package input

// State is what the user typed into an input, held across a rebuild (rule
// 4). The same shape as every component's:
//
//	st := s.name.State()
//	s.name = input.New(opts)
//	s.name.Restore(st)
type State struct {
	built bool
	value string
}

// State captures the input's state.
func (m Model) State() State { return State{built: m.token != nil, value: m.Value()} }

// Restore puts s back.
func (m *Model) Restore(s State) {
	if s.built {
		m.SetValue(s.value)
	}
}
