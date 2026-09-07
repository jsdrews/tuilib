package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// stubScreen is a minimal screen with a few bindings, so the footer has
// something to overflow with.
type stubScreen struct{ name string }

func (s *stubScreen) Title() string                           { return s.name }
func (s *stubScreen) Init() tea.Cmd                           { return nil }
func (s *stubScreen) OnEnter(any) tea.Cmd                     { return nil }
func (s *stubScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *stubScreen) Layout() layout.Node {
	return layout.RenderFunc(func(geom.Rect) string { return "" })
}
func (s *stubScreen) SetTheme(theme.Theme)  {}
func (s *stubScreen) IsCapturingKeys() bool { return false }
func (s *stubScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alpha")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bravo")),
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "charlie")),
	}
}

const (
	termW = 80
	termH = 24
)

func newApp(t *testing.T, depth int) Model {
	t.Helper()
	m := New(Options{
		Root:       &stubScreen{name: "root"},
		Themes:     []theme.Theme{theme.Dark()},
		Mouse:      MouseClick,
		SkipConfig: true,
	})
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)
	for i := 1; i < depth; i++ {
		tm, _ = m.Update(screen.PushMsg{Screen: &stubScreen{name: "child"}})
		m = tm.(Model)
	}
	// A render establishes the frame generation the rects are stamped in.
	_ = m.View()
	return m
}

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

// The shell's own chrome must be clickable. This regressed once because
// View has a value receiver, so the layout.Bar wrappers there size a copy of
// the Model that is thrown away — leaving the real breadcrumb and statusbar
// with a zero rect that declines every click.
func TestClickingHelpAffordanceOpensAndClosesOverlay(t *testing.T) {
	m := newApp(t, 1)
	if !m.helpOverflow {
		t.Fatalf("setup: expected the footer to show a help affordance")
	}
	if m.helpUp {
		t.Fatalf("setup: overlay should start closed")
	}

	m.placeChrome()
	slot := m.sb.LeftContentRect()
	at, _, ok := m.help.AffordanceSpan(m.shortViewBudget())
	if !ok {
		t.Fatalf("setup: no affordance span reported")
	}

	tm, _ := m.Update(press(slot.X+at, slot.Y))
	m = tm.(Model)

	if !m.helpUp {
		t.Errorf("clicking the help affordance did not open the overlay")
	}

	// And again to close it: the affordance is outside the modal's bounds,
	// which is the overlay's own dismissal gesture rather than a second
	// case in the shell.
	_ = m.View()
	m.placeChrome()
	at, _, _ = m.help.AffordanceSpan(m.shortViewBudget())
	tm, cmd := m.Update(press(slot.X+at, slot.Y))
	m = tm.(Model)
	if cmd == nil {
		t.Fatalf("no dismissal command from a press outside the overlay")
	}
	tm, _ = m.Update(cmd())
	if tm.(Model).helpUp {
		t.Errorf("clicking outside the overlay did not close it")
	}
}

func TestClickingElsewhereOnStatusbarDoesNotToggle(t *testing.T) {
	m := newApp(t, 1)

	m.placeChrome()
	slot := m.sb.LeftContentRect()
	at, w, _ := m.help.AffordanceSpan(m.shortViewBudget())

	tm, _ := m.Update(press(slot.X+at+w+2, slot.Y))

	if tm.(Model).helpUp {
		t.Errorf("a click beside the affordance opened the overlay")
	}
}

// Clicking a crumb unwinds the stack to that depth.
func TestClickingCrumbPopsToThatDepth(t *testing.T) {
	m := newApp(t, 3)
	if got := m.stack.Depth(); got != 3 {
		t.Fatalf("setup: Depth() = %d, want 3", got)
	}

	m.placeChrome()
	// The root crumb sits just inside the bar's left padding.
	x := m.bc.Rect().X + 1
	if _, ok := m.bc.CrumbAt(x, 0); !ok {
		t.Fatalf("setup: no crumb at x=%d", x)
	}

	tm, cmd := m.Update(press(x, 0))
	m = tm.(Model)
	if cmd != nil {
		cmd()
	}

	if got := m.stack.Depth(); got != 1 {
		t.Errorf("Depth() = %d, want 1 after clicking the root crumb", got)
	}
}

func TestMouseOffIgnoresClicks(t *testing.T) {
	m := New(Options{
		Root:       &stubScreen{name: "root"},
		Themes:     []theme.Theme{theme.Dark()},
		SkipConfig: true,
	})
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)
	_ = m.View()

	m.placeChrome()
	slot := m.sb.LeftContentRect()
	at, _, _ := m.help.AffordanceSpan(m.shortViewBudget())

	tm, _ = m.Update(press(slot.X+at, slot.Y))

	if tm.(Model).helpUp {
		t.Errorf("clicks were handled with MouseOff")
	}
}

// Handing the terminal to a subprocess kills mouse reporting, and
// bubbletea's RestoreTerminal doesn't bring it back — it restores the alt
// screen, bracketed paste and focus reporting, but has no notion of mouse
// state. Enabling mouse from Init (rather than as a tea.NewProgram option)
// means its startup flags don't record it either, so nothing would ever turn
// it back on and the TUI returns mouse-dead.
func TestMouseIsReenabledAfterASubprocess(t *testing.T) {
	m := newApp(t, 1)

	_, cmd := m.Update(runner.Result{})

	if cmd == nil {
		t.Fatalf("runner.Result produced no command; mouse was not re-enabled")
	}
	if !emits(cmd, isEnableMouse) {
		t.Errorf("no enable-mouse command after a subprocess returned")
	}
}

func TestMouseStaysOffAfterASubprocessWhenDisabled(t *testing.T) {
	m := New(Options{
		Root:       &stubScreen{name: "root"},
		Themes:     []theme.Theme{theme.Dark()},
		SkipConfig: true,
	})
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)

	_, cmd := m.Update(runner.Result{})

	if emits(cmd, isEnableMouse) {
		t.Errorf("an app with MouseOff turned mouse on after a subprocess")
	}
}

// emits reports whether cmd produces a message satisfying pred, flattening
// batches the way bubbletea would.
func emits(cmd tea.Cmd, pred func(tea.Msg) bool) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if emits(c, pred) {
				return true
			}
		}
		return false
	}
	return pred(msg)
}

// Suspending hands the terminal back to the shell exactly as a subprocess
// does, so the same restore applies. bubbletea's RestoreTerminal runs before
// ResumeMsg is sent, so re-enabling on that message lands in the right order.
func TestMouseIsReenabledAfterSuspend(t *testing.T) {
	m := newApp(t, 1)

	_, cmd := m.Update(tea.ResumeMsg{})

	if !emits(cmd, isEnableMouse) {
		t.Errorf("no enable-mouse command after resuming from suspend")
	}
}

func TestMouseStaysOffAfterSuspendWhenDisabled(t *testing.T) {
	m := New(Options{
		Root:       &stubScreen{name: "root"},
		Themes:     []theme.Theme{theme.Dark()},
		SkipConfig: true,
	})
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)

	_, cmd := m.Update(tea.ResumeMsg{})

	if emits(cmd, isEnableMouse) {
		t.Errorf("an app with MouseOff turned mouse on after resuming")
	}
}

// bubbletea delivers ctrl+z as an ordinary key and expects the app to ask
// for the suspend, so without the shell binding it the key does nothing.
func TestSuspendKeyRequestsSuspend(t *testing.T) {
	m := newApp(t, 1)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})

	if !emits(cmd, func(msg tea.Msg) bool { _, ok := msg.(tea.SuspendMsg); return ok }) {
		t.Errorf("ctrl+z did not request a suspend")
	}
}

// A screen taking keyboard input owns ctrl+z along with everything else —
// suspending mid-filter would strand the query.
func TestSuspendKeySuppressedWhileCapturing(t *testing.T) {
	m := newApp(t, 1)
	m.stack, _ = m.stack.Update(screen.PushMsg{Screen: &capturingScreen{}})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})

	if emits(cmd, func(msg tea.Msg) bool { _, ok := msg.(tea.SuspendMsg); return ok }) {
		t.Errorf("ctrl+z suspended while a screen was capturing keys")
	}
}

func TestDisableSuspendTurnsTheKeyOff(t *testing.T) {
	m := New(Options{
		Root:           &stubScreen{name: "root"},
		Themes:         []theme.Theme{theme.Dark()},
		DisableSuspend: true,
		SkipConfig:     true,
	})
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m = tm.(Model)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlZ})

	if emits(cmd, func(msg tea.Msg) bool { _, ok := msg.(tea.SuspendMsg); return ok }) {
		t.Errorf("DisableSuspend did not turn the suspend key off")
	}
}

// capturingScreen claims the keyboard, as a focused filter would.
type capturingScreen struct{ stubScreen }

func (s *capturingScreen) IsCapturingKeys() bool { return true }

func isEnableMouse(msg tea.Msg) bool {
	return msg != nil && strings.Contains(fmt.Sprintf("%T", msg), "enableMouse")
}
