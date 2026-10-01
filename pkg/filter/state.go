package filter

// State is what the user typed into a filter, held across a rebuild (rule
// 4). The same shape as every component's, so a screen restores them all
// the same way:
//
//	st := s.f.State()
//	s.f = filter.New(opts)
//	s.f.Restore(st)
type State struct{ value string }

// State captures the filter's state.
func (m Model) State() State { return State{value: m.Value()} }

// Restore puts s back.
func (m *Model) Restore(s State) { m.SetValue(s.value) }
