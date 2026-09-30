// Package source coordinates a windowed view over a paged remote source —
// the bookkeeping between "the user scrolled here" and "ask the server for
// that range", without ever doing the I/O itself.
//
// It is shaped like pkg/poll: it owns no data and performs no requests. It
// tracks which window is held, which is in flight, and which query each
// request belongs to, and it emits RequestMsg when the rows on screen stop
// being rows it has. Your screen answers that with whatever HTTP, gRPC, or
// database call it likes, passing Query.Ctx so a superseded request is
// cancelled, then hands the result back through Deliver and pushes the rows
// into the component. Keeping the fetch in the screen is deliberate: every
// component in tuilib is synchronous, and a coordinator that owned a retry
// policy would drag it into places that have no business holding it.
//
// It deliberately does not import pkg/table. The table reports what
// happened (ViewportChangedMsg, QueryChangedMsg) and the screen translates
// those into Viewport and SetQuery calls here, which keeps this package
// usable for any component that can say which rows are on screen — and
// keeps the dependency pointing one way.
//
// The loop, in full:
//
//	func (s *Screen) Init() tea.Cmd { return s.src.Init() }
//
//	case table.ViewportChangedMsg:
//	    cmds = append(cmds, s.src.Viewport(msg.FirstVisible, msg.LastVisible))
//	case table.QueryChangedMsg:
//	    cmds = append(cmds, s.src.SetQuery(msg.Raw, msg.Terms, msg.Sort, msg.Desc))
//	case source.RequestMsg:
//	    return s, s.fetch(msg.Query)          // your call, under msg.Query.Ctx
//	case fetchedMsg:
//	    ok, cmd := s.src.Deliver(msg.Page)
//	    switch {
//	    case !ok:                             // superseded; drop it
//	    case msg.Page.Err != nil:
//	        s.table.SetFailed(answered(msg.Query))
//	    default:
//	        s.table.SetWindow(msg.Rows, msg.Page.Offset, msg.Page.Total, answered(msg.Query))
//	    }
//	    return s, cmd
//	}
//	cmds = append(cmds, s.src.Update(msg))     // every message, like pkg/poll
//
// Installing the window makes the component emit a fresh
// ViewportChangedMsg, which closes the loop: a short page that still
// doesn't cover the screen asks for the rest on its own.
//
// Three things keep a slow source bearable. Every request carries its own
// context, cancelled the moment a newer request supersedes it — a new
// query, or a newer window of the same one — so at most one request per
// source is live on the server. Scrolling asks only once the viewport has
// been still for Options.ViewportDelay, so a held key or a scrollbar drag
// is one request, not one per page crossed. And while a committed query
// has no answer yet, scrolling asks for nothing at all: the rows on screen
// answer the previous query, and their positions mean nothing in the next.
//
// The query history — each committed query answered, failed or cancelled —
// leaves as QueryAnsweredMsg, QueryFailedMsg and QueryCancelledMsg in the
// commands SetQuery and Deliver return. They are deliberately neutral, like
// pkg/runner's messages: pkg/app turns them into output-console records.
package source

import (
	"context"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/query"
)

// DefaultPageSize is the window size used when Options.PageSize is unset.
const DefaultPageSize = 100

// DefaultViewportDelay is how long the viewport must be still before a
// scroll asks for rows, when Options.ViewportDelay is zero.
const DefaultViewportDelay = 150 * time.Millisecond

// Mode selects how the source is addressed.
type Mode int

const (
	// ByOffset addresses rows by numeric offset ("?offset=200&limit=100").
	// Windows can jump anywhere, so scrolling to the middle of a large set
	// fetches exactly the range on screen. Zero value.
	ByOffset Mode = iota
	// ByCursor addresses rows by an opaque continuation token. Only
	// forward, sequential paging is possible, so the screen accumulates
	// rows and installs them as one growing window at offset 0 — pass
	// Total -1 to SetWindow until the source runs out.
	ByCursor
)

// Query is one request's worth of parameters. Everything the source needs
// to answer is here; nothing about how to reach it is.
type Query struct {
	// Offset is the first row wanted (ByOffset only).
	Offset int
	// Limit is how many rows are wanted.
	Limit int
	// Cursor is the continuation token to resume from (ByCursor only).
	// Empty on the first request of a query.
	Cursor string

	// Raw is the filter text the user committed, as typed.
	Raw string
	// Terms is Raw parsed. Scoped terms carry their resolved column Title,
	// so building "?region=europe" needs no lookup.
	Terms []query.Term
	// Sort is the column title to order by, "" when unsorted.
	Sort string
	// Desc reverses the order.
	Desc bool

	// Gen identifies this request. Echo it back in Page.Gen; anything
	// older is refused by Deliver, which is what stops an out-of-order
	// reply from painting a window the user has already scrolled past.
	Gen int

	// Ctx is cancelled as soon as this request is superseded — by a new
	// query, a newer window, Refresh, or Cancel — and once its page has
	// been delivered. Pass it to the call that does the fetch.
	Ctx context.Context
}

// RequestMsg asks the screen to fetch Query.
type RequestMsg struct {
	Query Query
}

// Page reports what a fetch returned. Rows themselves never come here —
// they go straight from the screen into the component.
type Page struct {
	// Gen must echo the Query.Gen that produced this page.
	Gen int
	// Offset is where these rows start (ByOffset). Ignored under ByCursor,
	// where pages always extend the accumulated window.
	Offset int
	// Count is how many rows arrived. A zero count is remembered, so an
	// offset the source has nothing for is not asked for twice.
	Count int
	// Total is the logical row count, or -1 when the source can't say.
	Total int
	// Next is the continuation token for the following page (ByCursor).
	// Empty means the source is exhausted, which is also what finally
	// establishes the total.
	Next string
	// Err is set when the fetch failed. Deliver it like any other page:
	// Deliver decides whether anyone is still waiting for it.
	Err error
}

// QueryAnsweredMsg reports that a committed query got its first answer —
// or its first answer after failing. One per committed query, never one
// per page.
type QueryAnsweredMsg struct {
	Query   Query
	Elapsed time.Duration
}

// QueryFailedMsg reports a fetch that failed while someone was still
// waiting for it. Window is true when the query already had an answer and
// only a page of it failed.
type QueryFailedMsg struct {
	Query   Query
	Err     error
	Elapsed time.Duration
	Window  bool
}

// QueryCancelledMsg reports a committed query abandoned before it was
// answered, because By was committed in its place.
type QueryCancelledMsg struct {
	Query Query
	By    Query
}

// Options configures a Model.
type Options struct {
	// Mode selects offset or cursor addressing. Defaults to ByOffset.
	Mode Mode
	// PageSize is how many rows one request asks for, and the boundary
	// windows align to. Defaults to DefaultPageSize.
	PageSize int
	// Prefetch is how many extra pages to pull beyond the range actually
	// on screen. Zero (the default) fetches only what is needed, so the
	// user sees placeholders briefly at each page boundary; one page of
	// prefetch usually hides that at the cost of an extra request.
	Prefetch int
	// ViewportDelay is how long the viewport must be still before a
	// scroll asks for rows. Zero means DefaultViewportDelay; negative
	// means ask at once. Init, SetQuery and Refresh never wait.
	ViewportDelay time.Duration
	// Context is the parent of every request's context, so cancelling it
	// cancels everything in flight. Defaults to context.Background().
	Context context.Context
}

// answerState is where the current query stands.
type answerState int

const (
	unanswered answerState = iota
	answered
	failed
)

var nextID atomic.Int64

// settleMsg fires when a scroll's delay has elapsed.
type settleMsg struct {
	id  int64
	seq int
}

// Model is the coordinator. Embed as a value; drive it through the methods.
type Model struct {
	id       int64
	mode     Mode
	pageSize int
	prefetch int
	delay    time.Duration
	parent   context.Context

	raw   string
	terms []query.Term
	sort  string
	desc  bool

	first, last int
	haveVP      bool
	vpSeq       int

	heldStart int
	heldCount int
	total     int
	next      string
	exhausted bool

	state answerState

	gen       int
	pending   bool
	wantStart int
	wantLimit int
	live      Query
	liveAt    time.Time
	cancel    context.CancelFunc

	// emptyStart/emptyLimit remember the last window the source answered
	// with nothing, so a range it has no rows for is not requested in a
	// loop. Cleared whenever the query or the held window changes.
	emptyStart int
	emptyLimit int
	emptySeen  bool
}

// New constructs a coordinator. Nothing is requested until Init.
func New(opts Options) Model {
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultPageSize
	}
	if opts.Prefetch < 0 {
		opts.Prefetch = 0
	}
	switch {
	case opts.ViewportDelay == 0:
		opts.ViewportDelay = DefaultViewportDelay
	case opts.ViewportDelay < 0:
		opts.ViewportDelay = 0
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	return Model{
		id:       nextID.Add(1),
		mode:     opts.Mode,
		pageSize: opts.PageSize,
		prefetch: opts.Prefetch,
		delay:    opts.ViewportDelay,
		parent:   opts.Context,
		total:    -1,
	}
}

// Init returns the first request — the opening page of the current query.
// Batch it into your screen's Init, the way poll.Init is batched. It is
// needed because an empty component reports no viewport, so nothing else
// would ever ask for the first page.
func (m *Model) Init() tea.Cmd {
	return m.request(0, m.pageSize)
}

// Update handles the source's own timer. Forward every message to it, the
// way pkg/poll is forwarded; anything that isn't the source's returns nil.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	s, ok := msg.(settleMsg)
	if !ok || s.id != m.id || s.seq != m.vpSeq {
		return nil
	}
	return m.maybeRequest()
}

// Viewport reports the logical row range now on screen, inclusive. Feed it
// from the component's viewport message. When those rows are not ones the
// source has supplied, it asks for them once the viewport has been still
// for Options.ViewportDelay — so calling it on every scroll tick is fine.
func (m *Model) Viewport(first, last int) tea.Cmd {
	if last < first {
		last = first
	}
	m.first, m.last, m.haveVP = first, last, true
	if _, _, ok := m.wanted(); !ok {
		return nil
	}
	if m.delay <= 0 {
		return m.maybeRequest()
	}
	m.vpSeq++
	msg := settleMsg{id: m.id, seq: m.vpSeq}
	return tea.Tick(m.delay, func(time.Time) tea.Msg { return msg })
}

// SetQuery installs a new filter and sort, discards the held window, and
// requests the first page of the new query. Everything in flight is
// cancelled, and Deliver refuses replies to the query that was just
// replaced, so a slow response to the previous filter cannot land under
// the new one. A query abandoned before it was answered is reported as
// QueryCancelledMsg.
//
// It always requests, even when the arguments match the current query —
// a caller that has gone to the trouble of calling it wants a fetch.
func (m *Model) SetQuery(raw string, terms []query.Term, sort string, desc bool) tea.Cmd {
	prev, abandoned := m.live, m.state == unanswered && m.pending
	m.raw, m.terms, m.sort, m.desc = raw, terms, sort, desc
	m.last -= m.first
	m.first = 0
	m.resetWindow()
	m.state = unanswered
	req := m.request(0, m.pageSize)
	if !abandoned {
		return req
	}
	ev := QueryCancelledMsg{Query: prev, By: m.live}
	return tea.Batch(func() tea.Msg { return ev }, req)
}

// Refresh re-requests the window currently on screen without changing the
// query — the poll-driven "same view, fresh data" case, and the retry after
// a failure. Under ByCursor it restarts from the first page, since a cursor
// walk cannot be resumed from the middle.
func (m *Model) Refresh() tea.Cmd {
	if m.mode == ByCursor {
		m.resetWindow()
		return m.request(0, m.pageSize)
	}
	m.emptySeen = false
	start, limit := m.wantWindow()
	return m.request(start, limit)
}

// Cancel cancels whatever is in flight. A reply that arrives anyway is
// refused by Deliver.
func (m *Model) Cancel() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
	m.pending = false
}

// Deliver records a fetched page — or a failed fetch, when p.Err is set —
// and reports whether it was accepted. A false return means the page
// answers a superseded request: drop it. The command carries the query
// history (QueryAnsweredMsg, QueryFailedMsg) and, when a query's first
// answer leaves part of the screen uncovered, the request for the rest.
func (m *Model) Deliver(p Page) (bool, tea.Cmd) {
	if p.Gen != m.gen {
		return false, nil
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.pending = false
	elapsed := time.Since(m.liveAt)

	if p.Err != nil {
		ev := QueryFailedMsg{Query: m.live, Err: p.Err, Elapsed: elapsed, Window: m.state == answered}
		if m.state != answered {
			m.state = failed
		}
		return true, func() tea.Msg { return ev }
	}

	if p.Count == 0 {
		m.emptyStart, m.emptyLimit, m.emptySeen = m.wantStart, m.wantLimit, true
	} else {
		m.emptySeen = false
	}

	if m.mode == ByCursor {
		m.heldStart = 0
		m.heldCount += p.Count
		m.next = p.Next
		m.exhausted = p.Next == ""
		if m.exhausted {
			m.total = m.heldCount
		} else {
			m.total = p.Total
		}
	} else {
		m.heldStart, m.heldCount = p.Offset, p.Count
		m.total = p.Total
	}

	if m.state == answered {
		return true, nil
	}
	// The first answer: the component moves its cursor to the top, so
	// wherever the stale rows had been scrolled to no longer applies.
	m.state = answered
	m.last -= m.first
	m.first = 0
	ev := QueryAnsweredMsg{Query: m.live, Elapsed: elapsed}
	return true, tea.Batch(func() tea.Msg { return ev }, m.maybeRequest())
}

// Total is the logical row count last reported, or -1 while unknown. Pass
// it straight to the component's window setter.
func (m Model) Total() int { return m.total }

// Pending reports whether a request is outstanding.
func (m Model) Pending() bool { return m.pending }

// Held reports the window the source has supplied: its first row's logical
// index and how many rows it holds.
func (m Model) Held() (start, count int) { return m.heldStart, m.heldCount }

// Exhausted reports whether a ByCursor walk has run out of pages. Always
// false under ByOffset, where the total says the same thing.
func (m Model) Exhausted() bool { return m.exhausted }

// PageSize is the configured window size.
func (m Model) PageSize() int { return m.pageSize }

// resetWindow forgets everything learned about the current result set.
func (m *Model) resetWindow() {
	m.heldStart, m.heldCount = 0, 0
	m.total = -1
	m.next = ""
	m.exhausted = false
	m.pending = false
	m.emptySeen = false
}

// maybeRequest requests the window wanted, if any.
func (m *Model) maybeRequest() tea.Cmd {
	start, limit, ok := m.wanted()
	if !ok {
		return nil
	}
	return m.request(start, limit)
}

// wanted reports the window to request when the rows on screen aren't
// covered by what the source has already supplied. Nothing is wanted while
// the current query is unanswered with its first request out, or failed:
// the rows on screen then answer another query, and scrolling them says
// nothing about which rows of this one are wanted.
func (m Model) wanted() (start, limit int, ok bool) {
	if !m.haveVP || m.state == failed || (m.state == unanswered && m.pending) {
		return 0, 0, false
	}
	if m.mode == ByCursor {
		if m.exhausted || m.pending {
			return 0, 0, false
		}
		// Ask for more once the screen reaches the end of what has loaded,
		// or the prefetch margin ahead of it.
		if m.last < m.heldCount-m.prefetch*m.pageSize-1 {
			return 0, 0, false
		}
		return m.heldCount, m.pageSize, true
	}
	if m.heldCount > 0 && m.first >= m.heldStart && m.last < m.heldStart+m.heldCount {
		return 0, 0, false
	}
	start, limit = m.wantWindow()
	if m.pending && start == m.wantStart && limit == m.wantLimit {
		return 0, 0, false
	}
	if m.emptySeen && start == m.emptyStart && limit == m.emptyLimit {
		return 0, 0, false
	}
	return start, limit, true
}

// wantWindow is the page-aligned range covering the rows on screen plus
// any prefetch. Aligning to page boundaries keeps requests repeatable, so
// scrolling within a page asks for nothing new.
func (m Model) wantWindow() (start, limit int) {
	p := m.pageSize
	start = (m.first / p) * p
	if start < 0 {
		start = 0
	}
	end := m.last + 1 + m.prefetch*p
	pages := (end - start + p - 1) / p
	if pages < 1 {
		pages = 1
	}
	limit = pages * p
	if m.total >= 0 && start+limit > m.total {
		limit = m.total - start
	}
	if limit < 1 {
		limit = 1
	}
	return start, limit
}

// request emits a fetch for [start, start+limit) under a fresh generation,
// cancelling whatever was in flight. Every request gets its own
// generation, so only the newest reply is ever accepted and out-of-order
// responses can't fight over the window.
func (m *Model) request(start, limit int) tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.parent)
	m.cancel = cancel
	m.gen++
	m.pending = true
	m.wantStart, m.wantLimit = start, limit
	q := Query{
		Limit: limit,
		Raw:   m.raw,
		Terms: m.terms,
		Sort:  m.sort,
		Desc:  m.desc,
		Gen:   m.gen,
		Ctx:   ctx,
	}
	if m.mode == ByCursor {
		q.Cursor = m.next
	} else {
		q.Offset = start
	}
	m.live, m.liveAt = q, time.Now()
	return func() tea.Msg { return RequestMsg{Query: q} }
}
