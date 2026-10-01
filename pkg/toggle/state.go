package toggle

// State is which side of a toggle the user picked, held across a rebuild
// (rule 4). The same shape as every component's:
//
//	st := s.dry.State()
//	s.dry = toggle.New(opts)
//	s.dry.Restore(st)
type State struct {
	built bool
	value bool
}

// State captures the toggle's state.
func (m Model) State() State { return State{built: m.token != nil, value: m.Value()} }

// Restore puts s back.
func (m *Model) Restore(s State) {
	if s.built {
		m.SetValue(s.value)
	}
}
