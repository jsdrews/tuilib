package screen

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// stub records how many times it was activated and with what.
type stub struct {
	name    string
	enters  int
	lastArg any
}

func (s *stub) Title() string { return s.name }
func (s *stub) Init() tea.Cmd { return nil }
func (s *stub) OnEnter(v any) tea.Cmd {
	s.enters++
	s.lastArg = v
	return nil
}
func (s *stub) Update(tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s *stub) Layout() layout.Node              { return nil }
func (s *stub) Help() []key.Binding              { return nil }
func (s *stub) SetTheme(theme.Theme)             {}
func (s *stub) IsCapturingKeys() bool            { return false }

// fourDeep returns a stack of root → a → b → c and the four screens.
func fourDeep() (Stack, []*stub) {
	root := &stub{name: "root"}
	a, b, c := &stub{name: "a"}, &stub{name: "b"}, &stub{name: "c"}
	s := NewStack(root)
	for _, sc := range []*stub{a, b, c} {
		s, _ = s.Update(PushMsg{Screen: sc})
	}
	return s, []*stub{root, a, b, c}
}

func TestPopToUnwindsToDepth(t *testing.T) {
	s, screens := fourDeep()
	if s.Depth() != 4 {
		t.Fatalf("setup: Depth() = %d, want 4", s.Depth())
	}

	s, _ = s.Update(PopToMsg{Depth: 2})

	if s.Depth() != 2 {
		t.Errorf("Depth() = %d, want 2", s.Depth())
	}
	if got := s.Current().Title(); got != "a" {
		t.Errorf("Current() = %q, want %q", got, "a")
	}
	_ = screens
}

func TestPopToRootFromDepthFour(t *testing.T) {
	s, _ := fourDeep()

	s, _ = s.Update(PopToMsg{Depth: 1})

	if s.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", s.Depth())
	}
	if got := s.Current().Title(); got != "root" {
		t.Errorf("Current() = %q, want %q", got, "root")
	}
}

// Only the destination is activated. Firing OnEnter on the screens passed
// through would kick off fetches for views the user never sees.
func TestPopToActivatesOnlyTheDestination(t *testing.T) {
	s, screens := fourDeep()
	root, a, b := screens[0], screens[1], screens[2]
	beforeRoot, beforeA, beforeB := root.enters, a.enters, b.enters

	s, cmd := s.Update(PopToMsg{Depth: 2, Result: "payload"})
	if cmd != nil {
		cmd()
	}

	if a.enters != beforeA+1 {
		t.Errorf("destination activated %d times, want 1", a.enters-beforeA)
	}
	if root.enters != beforeRoot {
		t.Errorf("a screen below the destination was activated")
	}
	if b.enters != beforeB {
		t.Errorf("a screen passed through was activated")
	}
	if a.lastArg != "payload" {
		t.Errorf("destination received %v, want %q", a.lastArg, "payload")
	}
	_ = s
}

// PopTo can be called without first checking where the stack is.
func TestPopToCurrentOrDeeperIsNoop(t *testing.T) {
	for _, depth := range []int{4, 5, 0, -1} {
		s, screens := fourDeep()
		top := screens[3]
		before := top.enters

		s, _ = s.Update(PopToMsg{Depth: depth})

		if s.Depth() != 4 {
			t.Errorf("PopTo(%d): Depth() = %d, want 4 (unchanged)", depth, s.Depth())
		}
		if top.enters != before {
			t.Errorf("PopTo(%d) re-activated the current screen", depth)
		}
	}
}

func TestPopToCmdCarriesDepthAndResult(t *testing.T) {
	msg := PopTo(2, "x")()

	got, ok := msg.(PopToMsg)
	if !ok {
		t.Fatalf("PopTo returned %T, want PopToMsg", msg)
	}
	if got.Depth != 2 || got.Result != "x" {
		t.Errorf("PopToMsg = %+v, want {Depth:2 Result:x}", got)
	}
}
