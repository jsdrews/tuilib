package activity

import tea "github.com/charmbracelet/bubbletea"

// derive is one observation with no values to judge claims by — the shape
// most state-machine tests need.
func (s *Set) derive(busy map[string]string) tea.Cmd { return s.observe(busy, nil) }

// expect is an acknowledged Observed claim recorded against at: what a screen
// has once its POST answered.
func (s *Set) expect(keys []string, label string, at map[string]string) tea.Cmd {
	if s.statuses == nil {
		s.statuses = map[string]string{}
	}
	for k, v := range at {
		s.statuses[k] = v
	}
	op, cmd := s.Dispatch(keys, label, Observed)
	s.Done(op, nil)
	return cmd
}

// dispatchAt is Dispatch with each key's pre-dispatch status set first, as a
// component's last observation would have left it.
func (s *Set) dispatchAt(keys []string, label string, mode Mode, at map[string]string) (Op, tea.Cmd) {
	if s.statuses == nil {
		s.statuses = map[string]string{}
	}
	for k, v := range at {
		s.statuses[k] = v
	}
	return s.Dispatch(keys, label, mode)
}
