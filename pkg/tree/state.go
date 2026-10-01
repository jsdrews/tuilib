package tree

import "github.com/jsdrews/tuilib/pkg/activity"

// State is what the user and the data did to a tree — its root, which
// branches are open, the cursor, search and filter mode, marks, row
// activity, loading and scroll — held across a rebuild. A tree built from
// new Options (a theme swap, rule 4) takes it back with Restore:
//
//	st := s.tree.State()
//	s.tree = tree.New(opts)
//	s.tree.Restore(st)
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	root       Node
	expanded   map[string]bool
	cursor     int
	query      string
	filterMode bool
	marks      []string
	anchor     string
	act        activity.Set
	loading    bool
	xoff       int
}

// State captures the tree's state. See State.
func (m Model) State() State {
	exp := make(map[string]bool, len(m.expanded))
	for k, v := range m.expanded {
		exp[k] = v
	}
	return State{
		built:      m.token != nil,
		root:       m.root,
		expanded:   exp,
		cursor:     m.cursor,
		query:      m.Query(),
		filterMode: m.filterMode,
		marks:      m.Marks(),
		anchor:     m.markAnchor,
		act:        m.act,
		loading:    m.body.Loading(),
		xoff:       m.body.XOffset(),
	}
}

// Restore puts s back: the root with its branches opened as they were,
// then the search over the rows that shows, marks and activity, then the
// cursor.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	m.root = s.root
	m.expanded = make(map[string]bool, len(s.expanded))
	for k, v := range s.expanded {
		m.expanded[k] = v
	}
	m.refresh()
	m.SetFilterMode(s.filterMode)
	m.SetQuery(s.query)
	m.SetMarks(s.marks)
	m.markAnchor = s.anchor
	m.SetActivityState(s.act)
	m.SetCursor(s.cursor)
	m.SetLoading(s.loading)
	m.body.SetXOffset(s.xoff)
}
