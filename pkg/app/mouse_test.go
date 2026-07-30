package app

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/layout"
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
func TestClickingHelpAffordanceTogglesPanel(t *testing.T) {
	m := newApp(t, 1)
	if !m.helpOverflow {
		t.Fatalf("setup: expected the footer to show a help affordance")
	}
	if m.helpExpanded {
		t.Fatalf("setup: panel should start collapsed")
	}

	m.placeChrome()
	slot := m.sb.LeftContentRect()
	at, _, ok := m.help.AffordanceSpan(m.shortViewBudget())
	if !ok {
		t.Fatalf("setup: no affordance span reported")
	}

	tm, _ := m.Update(press(slot.X+at, slot.Y))
	m = tm.(Model)

	if !m.helpExpanded {
		t.Errorf("clicking the help affordance did not expand the panel")
	}

	// And again to close it.
	_ = m.View()
	m.placeChrome()
	at, _, _ = m.help.AffordanceSpan(m.shortViewBudget())
	tm, _ = m.Update(press(slot.X+at, slot.Y))
	if tm.(Model).helpExpanded {
		t.Errorf("clicking the affordance again did not collapse the panel")
	}
}

func TestClickingElsewhereOnStatusbarDoesNotToggle(t *testing.T) {
	m := newApp(t, 1)

	m.placeChrome()
	slot := m.sb.LeftContentRect()
	at, w, _ := m.help.AffordanceSpan(m.shortViewBudget())

	tm, _ := m.Update(press(slot.X+at+w+2, slot.Y))

	if tm.(Model).helpExpanded {
		t.Errorf("a click beside the affordance toggled the panel")
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

	if tm.(Model).helpExpanded {
		t.Errorf("clicks were handled with MouseOff")
	}
}
