// Package source coordinates a windowed view over a paged remote source —
// the bookkeeping between "the user scrolled here" and "ask the server for
// that range", without ever doing the I/O itself.
//
// It is shaped like pkg/poll: it owns no data and performs no requests. It
// tracks which window is held, which is in flight, and which generation
// each request belongs to, and it emits RequestMsg when the rows on screen
// stop being rows it has. Your screen answers that with whatever HTTP,
// gRPC, or database call it likes, then hands the result back through
// Deliver and pushes the rows into the component. Keeping the fetch in the
// screen is deliberate: every component in tuilib is synchronous, and a
// coordinator that owned a context and a retry policy would drag both into
// places that have no business holding them.
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
//	    return s, s.src.Viewport(msg.FirstVisible, msg.LastVisible)
//	case table.QueryChangedMsg:
//	    return s, s.src.SetQuery(msg.Raw, msg.Terms, msg.Sort, msg.Desc)
//	case source.RequestMsg:
//	    return s, s.fetch(msg.Query)          // your call, your context
//	case fetchedMsg:
//	    if !s.src.Deliver(msg.Page) {
//	        return s, nil                     // a stale reply; drop it
//	    }
//	    s.table.SetWindow(msg.Rows, msg.Page.Offset, msg.Page.Total)
//	    return s, s.table.SetLoading(false)
//
// Installing the window makes the component emit a fresh
// ViewportChangedMsg, which closes the loop: a short page that still
// doesn't cover the screen asks for the rest on its own.
package source

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/query"
)

// DefaultPageSize is the window size used when Options.PageSize is unset.
const DefaultPageSize = 100

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
}

// RequestMsg asks the screen to fetch Query. It is the only message this
// package emits.
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
}

// Model is the coordinator. Embed as a value; drive it through the methods.
type Model struct {
	mode     Mode
	pageSize int
	prefetch int

	raw   string
	terms []query.Term
	sort  string
	desc  bool

	first, last int
	haveVP      bool

	heldStart int
	heldCount int
	total     int
	next      string
	exhausted bool

	gen       int
	pending   bool
	wantStart int
	wantLimit int

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
	return Model{
		mode:     opts.Mode,
		pageSize: opts.PageSize,
		prefetch: opts.Prefetch,
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

// Viewport reports the logical row range now on screen, inclusive. Feed it
// from the component's viewport message. It returns a request when those
// rows are not ones the source has already supplied, and nil otherwise —
// so calling it on every scroll tick is fine.
func (m *Model) Viewport(first, last int) tea.Cmd {
	if last < first {
		last = first
	}
	m.first, m.last, m.haveVP = first, last, true
	return m.maybeRequest()
}

// SetQuery installs a new filter and sort, discards the held window, and
// requests the first page of the new query. Everything in flight is
// abandoned: Deliver refuses replies to the query that was just replaced,
// so a slow response to the previous filter cannot land under the new one.
//
// It always requests, even when the arguments match the current query —
// a caller that has gone to the trouble of calling it wants a fetch.
func (m *Model) SetQuery(raw string, terms []query.Term, sort string, desc bool) tea.Cmd {
	m.raw, m.terms, m.sort, m.desc = raw, terms, sort, desc
	m.last -= m.first
	m.first = 0
	m.resetWindow()
	return m.request(0, m.pageSize)
}

// Refresh re-requests the window currently on screen without changing the
// query — the poll-driven "same view, fresh data" case. Under ByCursor it
// restarts from the first page, since a cursor walk cannot be resumed from
// the middle.
func (m *Model) Refresh() tea.Cmd {
	if m.mode == ByCursor {
		m.resetWindow()
		return m.request(0, m.pageSize)
	}
	m.emptySeen = false
	start, limit := m.wantWindow()
	return m.request(start, limit)
}

// Deliver records a fetched page and reports whether it was accepted. A
// false return means the page answers a superseded request — the screen
// must drop those rows rather than install them.
func (m *Model) Deliver(p Page) bool {
	if p.Gen != m.gen {
		return false
	}
	m.pending = false
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
		return true
	}

	m.heldStart, m.heldCount = p.Offset, p.Count
	m.total = p.Total
	return true
}

// Total is the logical row count last reported, or -1 while unknown. Pass
// it straight to the component's window setter.
func (m Model) Total() int { return m.total }

// Pending reports whether a request is outstanding — the cue for a
// loading indicator.
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

// maybeRequest returns a request when the rows on screen aren't covered by
// what the source has already supplied.
func (m *Model) maybeRequest() tea.Cmd {
	if !m.haveVP {
		return nil
	}
	if m.mode == ByCursor {
		if m.exhausted || m.pending {
			return nil
		}
		// Ask for more once the screen reaches the end of what has loaded,
		// or the prefetch margin ahead of it.
		if m.last < m.heldCount-m.prefetch*m.pageSize-1 {
			return nil
		}
		return m.request(m.heldCount, m.pageSize)
	}
	if m.heldCount > 0 && m.first >= m.heldStart && m.last < m.heldStart+m.heldCount {
		return nil
	}
	start, limit := m.wantWindow()
	if m.pending && start == m.wantStart && limit == m.wantLimit {
		return nil
	}
	if m.emptySeen && start == m.emptyStart && limit == m.emptyLimit {
		return nil
	}
	return m.request(start, limit)
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

// request emits a fetch for [start, start+limit) under a fresh generation.
// Every request gets its own generation, so only the newest reply is ever
// accepted and out-of-order responses can't fight over the window.
func (m *Model) request(start, limit int) tea.Cmd {
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
	}
	if m.mode == ByCursor {
		q.Cursor = m.next
	} else {
		q.Offset = start
	}
	return func() tea.Msg { return RequestMsg{Query: q} }
}
