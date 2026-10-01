package list

import "github.com/jsdrews/tuilib/pkg/activity"

// State is what the user and the data did to a list — its items, cursor,
// filter, marks, row activity, loading and scroll — held across a rebuild.
// It carries nothing Options sets, so a list built from new Options (a
// theme swap, rule 4) takes it back with Restore:
//
//	st := s.list.State()
//	s.list = list.New(opts)
//	s.list.Restore(st)
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	keyed   []KeyedItem
	items   []string
	cursor  int
	value   string
	marks   []string
	anchor  string
	act     activity.Set
	loading bool
	xoff    int
}

// State captures the list's state. See State.
func (m Model) State() State {
	s := State{
		built:   m.token != nil,
		cursor:  m.cursor,
		value:   m.Value(),
		marks:   m.Marks(),
		anchor:  m.markAnchor,
		act:     m.act,
		loading: m.body.Loading(),
		xoff:    m.body.XOffset(),
	}
	if m.itemKeys != nil {
		s.keyed = m.KeyedItems()
	} else {
		s.items = append([]string(nil), m.items...)
	}
	return s
}

// Restore puts s back, in the order that keeps each part valid: items,
// then the filter over them, then marks and activity on the rows that
// exist, then the cursor. A spinner the restored activity needs re-arms on
// the next message, as SetActivityState's does.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	if s.keyed != nil {
		m.SetKeyedItems(s.keyed)
	} else {
		m.SetItems(s.items)
	}
	m.SetValue(s.value)
	m.SetMarks(s.marks)
	m.markAnchor = s.anchor
	m.SetActivityState(s.act)
	m.SetCursor(s.cursor)
	m.SetLoading(s.loading)
	m.body.SetXOffset(s.xoff)
}
