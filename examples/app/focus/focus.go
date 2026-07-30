// Package focus demonstrates a screen with multiple components sharing one
// keyboard, using pkg/focus to decide which of them has it.
//
// A focus.Group holds the components in tab order and owns three things a
// hand-rolled index cannot do as well:
//
//   - tab / shift-tab cycling, including the blur-everything-focus-one dance
//   - granting focus to a component that was clicked, since the click arrives
//     at the component rather than at the screen that knows the ordering
//   - answering IsCapturingKeys from whichever component currently has focus
//
// The screen still routes messages itself, because focus is about the
// keyboard and not about content: keys go to the focused component only —
// otherwise typing in the query would also drive the list's cursor — while
// mouse events go to every component, so each can test the click against its
// own rect and claim it if it landed inside.
package focus

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
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

	focus focus.Group
}

func (s *focusScreen) Title() string       { return "Focus" }
func (s *focusScreen) OnEnter(any) tea.Cmd { return nil }

func (s *focusScreen) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, s.focus.Init())
}

// IsCapturingKeys defers to whichever component holds focus. The input says
// yes whenever it's focused; the list says yes only while its filter is
// engaged; the toggle never does, since ←/→ and space don't collide with the
// shell's globals.
func (s *focusScreen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *focusScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	// The group consumes tab / shift-tab and grants any focus request a
	// clicked component sent back up.
	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)
	cmds = append(cmds, gcmd)

	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
		// While capturing, the shell suppresses its auto-esc-pop; pop
		// explicitly so esc backs out from any focus state.
		return s, screen.Pop(nil)
	}

	if _, isMouse := msg.(mouse.Msg); isMouse {
		cmds = append(cmds, s.forwardAll(msg)...)
	} else {
		cmds = append(cmds, s.forwardFocused(msg))
	}
	return s, tea.Batch(cmds...)
}

// forwardAll hands a message to every component. Used for mouse events: each
// component tests the position against its own rect, and only the one it
// landed in acts.
func (s *focusScreen) forwardAll(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	var c tea.Cmd
	s.query, c = s.query.Update(msg)
	cmds = append(cmds, c)
	s.results, c = s.results.Update(msg)
	cmds = append(cmds, c)
	s.caseTgl, c = s.caseTgl.Update(msg)
	return append(cmds, c)
}

// forwardFocused hands a message to the focused component alone.
func (s *focusScreen) forwardFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.query):
		s.query, cmd = s.query.Update(msg)
	case s.focus.Is(&s.results):
		s.results, cmd = s.results.Update(msg)
	case s.focus.Is(&s.caseTgl):
		s.caseTgl, cmd = s.caseTgl.Update(msg)
	}
	return cmd
}

func (s *focusScreen) Layout() layout.Node {
	return layout.VStack(
		layout.Fixed(3, layout.Bar(&s.query)),
		layout.Flex(1, layout.Sized(&s.results)),
		layout.Fixed(3, layout.Bar(&s.caseTgl)),
	)
}

// Help composes the screen's own binding with the group's, which already
// carries tab / shift-tab plus the focused component's keys.
func (s *focusScreen) Help() []key.Binding {
	out := s.focus.Help()
	return append(out, key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")))
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

	// Rebuild the group over the same field addresses and restore the index.
	// The components behind those addresses were just replaced, but the
	// addresses themselves are stable, so the group keeps pointing at the
	// right panes across a theme swap.
	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.query, &s.results, &s.caseTgl)
	s.focus.SetIndex(at)
}
