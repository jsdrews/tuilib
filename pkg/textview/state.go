package textview

// State is what the user did to a textview — its content, wrap, search
// and current match, loading and scroll — held across a rebuild. A
// textview built from new Options (a theme swap, rule 4) takes it back
// with Restore:
//
//	st := s.doc.State()
//	s.doc = textview.New(opts)
//	s.doc.Restore(st)
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	content    string
	wrap       bool
	query      string
	matchIdx   int
	loading    bool
	yoff, xoff int
}

// State captures the textview's state. See State.
func (m Model) State() State {
	return State{
		built:    m.token != nil,
		content:  m.raw,
		wrap:     m.wrap,
		query:    m.Query(),
		matchIdx: m.matchIdx,
		loading:  m.body.Loading(),
		yoff:     m.body.YOffset(),
		xoff:     m.body.XOffset(),
	}
}

// Restore puts s back: content and wrap first — they decide the lines —
// then the search and its match, then the scroll.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	m.SetWrap(s.wrap)
	m.SetContent(s.content)
	m.SetQuery(s.query)
	if s.matchIdx < len(m.matches) {
		m.matchIdx = s.matchIdx
	}
	m.SetLoading(s.loading)
	m.body.SetYOffset(s.yoff)
	m.body.SetXOffset(s.xoff)
}
