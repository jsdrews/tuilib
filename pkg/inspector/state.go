package inspector

// State is what the user and the data did to an inspector — its fields,
// which are open, the cursor, search and filter mode, loading and scroll —
// held across a rebuild. An inspector built from new Options (a theme
// swap, rule 4) takes it back with Restore:
//
//	st := s.ins.State()
//	s.ins = inspector.New(opts)
//	s.ins.Restore(st)
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	fields     []Field
	expanded   map[string]bool
	cursor     int
	query      string
	filterMode bool
	loading    bool
	xoff       int
}

// State captures the inspector's state. See State.
func (m Model) State() State {
	exp := make(map[string]bool, len(m.expanded))
	for k, v := range m.expanded {
		exp[k] = v
	}
	return State{
		built:      m.token != nil,
		fields:     m.fields,
		expanded:   exp,
		cursor:     m.cursor,
		query:      m.Query(),
		filterMode: m.filterMode,
		loading:    m.body.Loading(),
		xoff:       m.body.XOffset(),
	}
}

// Restore puts s back: the fields with their objects opened as they were,
// then the search, then the cursor.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	m.fields = s.fields
	m.expanded = make(map[string]bool, len(s.expanded))
	for k, v := range s.expanded {
		m.expanded[k] = v
	}
	m.refresh()
	m.SetFilterMode(s.filterMode)
	m.SetQuery(s.query)
	m.SetCursor(s.cursor)
	m.SetLoading(s.loading)
	m.body.SetXOffset(s.xoff)
}
