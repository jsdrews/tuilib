package logview

// State is what the stream and the user did to a logview — its lines and
// which are markers, follow, the search and filter mode, the current match,
// loading and scroll — held across a rebuild. A logview built from new
// Options (a theme swap, rule 4) takes it back with Restore:
//
//	st := s.log.State()
//	s.log = logview.New(opts)
//	s.log.Restore(st)
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	lines      []string
	markers    []bool
	follow     bool
	query      string
	filterMode bool
	matchIdx   int
	loading    bool
	yoff, xoff int
}

// State captures the logview's state. See State.
func (m Model) State() State {
	return State{
		built:      m.token != nil,
		lines:      append([]string(nil), m.lines...),
		markers:    append([]bool(nil), m.markers...),
		follow:     m.follow,
		query:      m.Query(),
		filterMode: m.filterMode,
		matchIdx:   m.matchIdx,
		loading:    m.body.Loading(),
		yoff:       m.body.YOffset(),
		xoff:       m.body.XOffset(),
	}
}

// Restore puts s back: the lines, then the search over them and the match
// it was on, then where the view was — the bottom when following.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	m.lines = append([]string(nil), s.lines...)
	m.markers = append([]bool(nil), s.markers...)
	m.trim()
	m.filterMode = s.filterMode
	m.SetQuery(s.query)
	if s.matchIdx < len(m.matches) {
		m.matchIdx = s.matchIdx
	}
	m.refresh()
	m.SetLoading(s.loading)
	m.follow = s.follow
	if m.follow {
		m.body.GotoBottom()
	} else {
		m.body.SetYOffset(s.yoff)
	}
	m.body.SetXOffset(s.xoff)
	m.refreshStatus()
}
