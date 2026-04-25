// Package focus demonstrates a screen with multiple components and
// tab/shift-tab focus cycling. Only the focused component receives
// keystrokes; the others sit dim with their inactive border color.
//
// The pattern is: keep an int focus index, intercept tab/shift-tab in
// Update, call Blur on every component and Focus on the active one, and
// forward all other messages to the focused component only — otherwise
// typing in the input would also move list cursors.
package focus

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// New returns the focus demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &focusScreen{}
	s.SetTheme(t)
	return s
}

type focusScreen struct {
	t       theme.Theme
	query   input.Model
	results list.Model
	caseTgl toggle.Model

	focus int // 0=query, 1=results, 2=caseTgl
}

const focusCount = 3

func (s *focusScreen) Title() string       { return "Focus" }
func (s *focusScreen) Init() tea.Cmd       { return tea.Batch(textinput.Blink, s.query.Focus()) }
func (s *focusScreen) OnEnter(any) tea.Cmd { return nil }

// IsCapturingKeys claims keys only while the input is focused — that's
// the one component that needs every printable (q, t, …) routed to it.
// The list (j/k/↑↓) and toggle (←→/space) don't conflict with the shell's
// global keys, so they let q-quit and esc-pop pass through normally.
func (s *focusScreen) IsCapturingKeys() bool { return s.query.Focused() }

func (s *focusScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "tab":
			s.focus = (s.focus + 1) % focusCount
			return s, s.applyFocus()
		case "shift+tab":
			s.focus = (s.focus - 1 + focusCount) % focusCount
			return s, s.applyFocus()
		case "esc":
			// While capturing, the shell suppresses its auto-esc-pop; pop
			// explicitly so esc backs out from any focus state.
			return s, screen.Pop(nil)
		}
	}

	var cmd tea.Cmd
	switch s.focus {
	case 0:
		s.query, cmd = s.query.Update(msg)
	case 1:
		s.results, cmd = s.results.Update(msg)
	case 2:
		s.caseTgl, cmd = s.caseTgl.Update(msg)
	}
	return s, cmd
}

func (s *focusScreen) Layout() layout.Node {
	return layout.VStack(
		layout.Fixed(3, layout.Bar(&s.query)),
		layout.Flex(1, layout.Sized(&s.results)),
		layout.Fixed(3, layout.Bar(&s.caseTgl)),
	)
}

// Help composes the screen-wide bindings (tab/shift-tab/esc) with the
// focused component's own — each component owns its keys via Help().
func (s *focusScreen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next field")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev field")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
	switch s.focus {
	case 0:
		return append(base, s.query.Help()...)
	case 1:
		return append(base, s.results.Help()...)
	case 2:
		return append(base, s.caseTgl.Help()...)
	}
	return base
}

func (s *focusScreen) SetTheme(t theme.Theme) {
	s.t = t

	qOpts := t.Input()
	qOpts.Title = "query"
	qOpts.Placeholder = "search…"
	qOpts.Initial = s.query.Value()
	s.query = input.New(qOpts)

	cursor := s.results.Cursor()
	lOpts := t.List()
	lOpts.Title = "results"
	lOpts.Items = []string{
		"users.go", "users_test.go", "user_repo.go", "user_handler.go",
		"auth.go", "auth_test.go", "session.go", "middleware.go",
		"router.go", "config.go", "logging.go", "metrics.go",
	}
	s.results = list.New(lOpts)
	s.results.SetCursor(cursor)

	tOpts := t.Toggle()
	tOpts.Title = "case-sensitive"
	tOpts.Initial = s.caseTgl.Value()
	s.caseTgl = toggle.New(tOpts)

	s.applyFocus()
}

func (s *focusScreen) applyFocus() tea.Cmd {
	s.query.Blur()
	s.results.SetFocused(false)
	s.caseTgl.Blur()
	switch s.focus {
	case 0:
		return s.query.Focus()
	case 1:
		s.results.SetFocused(true)
	case 2:
		return s.caseTgl.Focus()
	}
	return nil
}
