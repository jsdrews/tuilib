package eventlog

import "github.com/jsdrews/tuilib/internal/remoteview"

// State is what the source and the user did to an eventlog — the items it
// holds (a range or a span), the query they answer and whether it failed,
// the cursor and scroll, follow and growth, the filter and search — held
// across a rebuild. An eventlog built from new Options (a theme swap, rule
// 4) takes it back with Restore:
//
//	st := s.log.State()
//	s.log = eventlog.New(opts)
//	s.log.Restore(st)
//
// A search or a hit still on its way is not carried: the rebuild answers
// neither, and a press after it asks again.
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	rng        remoteview.Range[Item]
	st         remoteview.Status
	cursor     int
	top        int
	following  bool
	growing    bool
	unseenFrom int
	qRaw       string
	term       string
	matchTotal int
	xoff       int
}

// State captures the eventlog's state. See State.
func (m Model) State() State {
	return State{
		built:      m.token != nil,
		rng:        m.rng,
		st:         m.st,
		cursor:     m.cursor,
		top:        m.top,
		following:  m.following,
		growing:    m.growing,
		unseenFrom: m.unseenFrom,
		qRaw:       m.qRaw,
		term:       m.term,
		matchTotal: m.matchTotal,
		xoff:       m.body.XOffset(),
	}
}

// Restore puts s back onto an eventlog built with the same Anchored
// setting. Items, query and failure come back as they were; the new
// Options keep the say over everything they set.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	if s.rng.IsSpan() == m.rng.IsSpan() {
		m.rng = s.rng
	}
	m.st = s.st
	m.st.Rearm()
	m.cursor, m.top = s.cursor, s.top
	m.following, m.growing, m.unseenFrom = s.following, s.growing, s.unseenFrom
	m.SetValue(s.qRaw)
	m.SetTerm(s.term)
	m.matchTotal = s.matchTotal
	m.refresh()
	m.body.SetXOffset(s.xoff)
}
