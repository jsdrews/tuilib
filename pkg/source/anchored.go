package source

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/query"
)

// Anchor is where a view of Anchored data begins.
type Anchor struct {
	kind   anchorKind
	cursor string
}

type anchorKind int

const (
	anchorNewest anchorKind = iota
	anchorOldest
	anchorAt
)

// Newest anchors at the newest item — the tail. The zero Anchor.
func Newest() Anchor { return Anchor{kind: anchorNewest} }

// Oldest anchors at the oldest item.
func Oldest() Anchor { return Anchor{kind: anchorOldest} }

// At anchors at the item whose cursor is c: a search hit, a time, a link.
func At(c string) Anchor { return Anchor{kind: anchorAt, cursor: c} }

// IsNewest reports whether a is the Newest anchor.
func (a Anchor) IsNewest() bool { return a.kind == anchorNewest }

// AnchoredOptions configures an Anchored source.
type AnchoredOptions struct {
	// PageSize is how many items one request asks for, and how close to an
	// edge the viewport must come before that edge is extended. Defaults to
	// DefaultPageSize.
	PageSize int
	// ViewportDelay, Context and Follow mean what they do on Options.
	ViewportDelay time.Duration
	Context       context.Context
	Follow        time.Duration
}

// Anchored coordinates a view over Anchored data — items reachable only by
// walking forwards or backwards from an anchor, with no offsets and no
// reliable total: Elasticsearch search_after, a log API filtered by
// timestamp. It is Model's counterpart and shares its request plumbing,
// cancellation, viewport delay and query history, but its surface is edges
// rather than offsets.
//
// The component holds a Span: items keyed by their cursors, grown at either
// edge and trimmed at the one furthest from the viewport. The source never
// stores or interprets a cursor. When it asks for more at an edge, the
// screen reads that edge's cursor from the component and encodes it for its
// API:
//
//	case source.RequestMsg:
//	    older, newer := s.log.Edges()
//	    return s, s.fetch(m.Query, older, newer)   // m.Query.Dir says which
//
// The request that starts a view has Query.FromAnchor set: page from the
// anchor, not from an edge — the span may still hold the previous query's
// items. Every other request extends the edge Query.Dir names.
//
// One request is in flight at a time. The viewport reports how far it is
// from each edge; an edge with more beyond it is extended once the viewport
// comes within PageSize of it, nearest edge first.
type Anchored struct {
	core

	pageSize int
	delay    time.Duration
	follow   time.Duration

	raw    string
	terms  []query.Term
	sort   string
	desc   bool
	anchor Anchor

	state     answerState
	moreOlder bool
	moreNewer bool

	toOlder, toNewer int
	haveVP           bool
	vpSeq            int

	growing      bool
	pollSeq      int
	pollArmed    bool
	pollsFailing bool
}

// NewAnchored constructs an Anchored source anchored at Newest. Nothing is
// requested until Init.
func NewAnchored(opts AnchoredOptions) Anchored {
	m := New(Options{
		PageSize:      opts.PageSize,
		ViewportDelay: opts.ViewportDelay,
		Context:       opts.Context,
		Follow:        opts.Follow,
	})
	return Anchored{
		core:     m.core,
		pageSize: m.pageSize,
		delay:    m.delay,
		follow:   m.follow,
	}
}

// Init requests the anchor's first page.
func (m *Anchored) Init() tea.Cmd { return m.reanchor() }

// SetQuery installs a new filter and sort and re-anchors at the current
// anchor. Everything in flight is cancelled; a query abandoned before it
// was answered is reported as QueryCancelledMsg.
func (m *Anchored) SetQuery(raw string, terms []query.Term, sort string, desc bool) tea.Cmd {
	prev, abandoned := m.live, m.state == unanswered && m.pending
	m.raw, m.terms, m.sort, m.desc = raw, terms, sort, desc
	req := m.reanchor()
	if !abandoned {
		return req
	}
	ev := QueryCancelledMsg{Query: prev, By: m.live}
	return tea.Batch(func() tea.Msg { return ev }, req)
}

// SetAnchor moves the view: the span is replaced by one starting at a. A
// search hit re-anchors with At(hit).
func (m *Anchored) SetAnchor(a Anchor) tea.Cmd {
	m.anchor = a
	return m.reanchor()
}

// Anchor returns the current anchor.
func (m Anchored) Anchor() Anchor { return m.anchor }

// Refresh re-anchors at the current anchor under the same query.
func (m *Anchored) Refresh() tea.Cmd { return m.reanchor() }

// Cancel cancels whatever is in flight.
func (m *Anchored) Cancel() { m.abandon() }

func (m *Anchored) reanchor() tea.Cmd {
	m.state = unanswered
	m.haveVP = false
	q := m.newQuery()
	q.FromAnchor = true
	switch m.anchor.kind {
	case anchorNewest:
		m.moreOlder, m.moreNewer = true, false
		q.Dir = Older
	case anchorOldest:
		m.moreOlder, m.moreNewer = false, true
		q.Dir = Newer
	case anchorAt:
		// Older from the anchor, inclusive; the newer side follows as an
		// ordinary edge request once the anchor is on screen.
		m.moreOlder, m.moreNewer = true, true
		q.Dir, q.Cursor, q.Inclusive = Older, m.anchor.cursor, true
	}
	return m.issue(q)
}

func (m Anchored) newQuery() Query {
	return Query{Limit: m.pageSize, Raw: m.raw, Terms: m.terms, Sort: m.sort, Desc: m.desc}
}

// Viewport reports how many items lie between the viewport and each edge
// of the span: toOlder above the first visible item, toNewer below the
// last. Feed it from the component's viewport message. An edge with more
// beyond it is extended once the viewport comes within PageSize of it,
// after Options.ViewportDelay.
func (m *Anchored) Viewport(toOlder, toNewer int) tea.Cmd {
	m.toOlder, m.toNewer, m.haveVP = max(0, toOlder), max(0, toNewer), true
	if _, ok := m.wanted(); !ok {
		return nil
	}
	if m.delay <= 0 {
		return m.maybeRequest()
	}
	m.vpSeq++
	msg := settleMsg{id: m.id, seq: m.vpSeq}
	return tea.Tick(m.delay, func(time.Time) tea.Msg { return msg })
}

// wanted reports which edge to extend: the nearer of those with more
// beyond them within PageSize of the viewport. Nothing is wanted until the
// anchor has an answer — the items on screen answer another query.
func (m Anchored) wanted() (Dir, bool) {
	if !m.haveVP || m.state != answered || (m.pending && m.live.Find) {
		return 0, false
	}
	older := m.moreOlder && m.toOlder < m.pageSize
	newer := m.moreNewer && m.toNewer < m.pageSize
	switch {
	case older && newer:
		if m.toNewer < m.toOlder {
			return Newer, true
		}
		return Older, true
	case older:
		return Older, true
	case newer:
		return Newer, true
	}
	return 0, false
}

// maybeRequest extends the wanted edge. A request already out for that
// edge is left alone; one for the other edge is cancelled in its favour.
func (m *Anchored) maybeRequest() tea.Cmd {
	dir, ok := m.wanted()
	if !ok {
		return nil
	}
	if m.pending && !m.live.Poll && !m.live.Find && m.live.Dir == dir {
		return nil
	}
	q := m.newQuery()
	q.Dir = dir
	return m.issue(q)
}

// Update handles the source's timers. Forward every message to it.
func (m *Anchored) Update(msg tea.Msg) tea.Cmd {
	switch s := msg.(type) {
	case settleMsg:
		if s.id != m.id || s.seq != m.vpSeq {
			return nil
		}
		return m.maybeRequest()
	case pollMsg:
		if s.id != m.id || s.seq != m.pollSeq {
			return nil
		}
		m.pollArmed = false
		return m.poll()
	}
	return nil
}

// SetGrowing says whether items are arriving at the newest end. While they
// are, the source polls every Follow: a fetch of what is newer than the
// newest edge while the viewport shows it (following), a Probe otherwise.
// Setting it false stops after one final read.
func (m *Anchored) SetGrowing(b bool) tea.Cmd {
	if m.growing == b {
		return nil
	}
	m.growing = b
	m.pollSeq++
	m.pollArmed = false
	if !b {
		return m.poll()
	}
	return m.armPoll()
}

// Poke makes a Growing source poll now — for a push channel saying there
// is news.
func (m *Anchored) Poke() tea.Cmd {
	if !m.growing {
		return nil
	}
	m.pollSeq++
	m.pollArmed = false
	return m.poll()
}

// PollsFailing reports whether follow polls are failing.
func (m Anchored) PollsFailing() bool { return m.pollsFailing }

// Find asks for the first item matching term beyond the edge in direction
// dir, within the committed filter. The screen fills the Cursor from the
// component's edge, as for any edge request. A reply with Page.Found and
// Page.Cursor is the hit: re-anchor with SetAnchor(At(cursor)).
func (m *Anchored) Find(term string, dir Dir) tea.Cmd {
	q := m.newQuery()
	q.Find, q.Term, q.Dir, q.Limit = true, term, dir, 1
	return m.issue(q)
}

func (m *Anchored) armPoll() tea.Cmd {
	if !m.growing || m.follow <= 0 || m.pollArmed {
		return nil
	}
	m.pollArmed = true
	msg := pollMsg{id: m.id, seq: m.pollSeq}
	return tea.Tick(m.follow, func(time.Time) tea.Msg { return msg })
}

func (m *Anchored) poll() tea.Cmd {
	if m.pending || m.state != answered {
		return m.armPoll()
	}
	q := m.newQuery()
	q.Dir, q.Poll = Newer, true
	q.Probe = !(m.haveVP && m.toNewer == 0 && !m.moreNewer)
	return m.issue(q)
}

// Deliver records a fetched page — or a failure, in p.Err — and reports
// whether it was accepted. For an edge request, p.More says whether that
// edge has more beyond it. The command carries the query history and the
// next request, when the page left an edge still wanted.
func (m *Anchored) Deliver(p Page) (bool, tea.Cmd) {
	if !m.accept(p) {
		return false, nil
	}
	elapsed := time.Since(m.liveAt)
	if p.Err != nil {
		if m.live.Poll {
			if m.pollsFailing {
				return true, m.armPoll()
			}
			m.pollsFailing = true
		}
		ev := QueryFailedMsg{Query: m.live, Err: p.Err, Elapsed: elapsed, Window: m.state == answered}
		if m.state != answered {
			m.state = failed
		}
		return true, tea.Batch(func() tea.Msg { return ev }, m.armPoll())
	}
	if m.live.Find {
		var next tea.Cmd
		if !p.Found {
			next = m.maybeRequest()
		}
		return true, tea.Batch(next, m.armPoll())
	}
	var recovered tea.Cmd
	if m.live.Poll && m.pollsFailing {
		m.pollsFailing = false
		ev := QueryRecoveredMsg{Query: m.live}
		recovered = func() tea.Msg { return ev }
	}
	if !m.live.Probe && !m.live.Poll {
		if m.live.Dir == Older {
			m.moreOlder = p.More
		} else {
			m.moreNewer = p.More
		}
	}
	if m.state == answered {
		return true, tea.Batch(recovered, m.maybeRequest(), m.armPoll())
	}
	m.state = answered
	ev := QueryAnsweredMsg{Query: m.live, Elapsed: elapsed}
	return true, tea.Batch(func() tea.Msg { return ev }, m.maybeRequest(), m.armPoll())
}

// More reports whether each edge has more beyond it.
func (m Anchored) More() (older, newer bool) { return m.moreOlder, m.moreNewer }

// Pending reports whether a request is outstanding.
func (m Anchored) Pending() bool { return m.pending }

// PageSize is the configured page size.
func (m Anchored) PageSize() int { return m.pageSize }
