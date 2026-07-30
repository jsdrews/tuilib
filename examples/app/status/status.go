// Package status demonstrates how a screen sends transient feedback to the
// app's statusbar. Pick an action with enter; the screen returns one of the
// app-level message commands (app.Info, app.Error, app.ClearStatus) and the
// shell mirrors it into the bar's center slot. Any subsequent keypress
// auto-clears the message — the same behavior as pug's footer.
package status

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the status demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &statusScreen{}
	s.SetTheme(t)
	return s
}

type action struct {
	label string
	run   func() tea.Cmd
}

var actions = []action{
	{"Report success", func() tea.Cmd { return app.Info("Run completed successfully.") }},
	{"Report failure", func() tea.Cmd { return app.Error("Error: API request failed — connection refused.") }},
	{"Long info", func() tea.Cmd {
		return app.Info("Deployment triggered — watching pipeline-7 for 12 services.")
	}},
	{"Clear", func() tea.Cmd { return app.ClearStatus() }},
}

type statusScreen struct {
	t    theme.Theme
	menu list.Model
	hint pane.Pane
}

func (s *statusScreen) Title() string         { return "Status" }
func (s *statusScreen) Init() tea.Cmd         { return nil }
func (s *statusScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *statusScreen) IsCapturingKeys() bool { return s.menu.Filtering() }

func (s *statusScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if s.menu.IsActivate(msg) {
		idx := s.menu.Cursor()
		if idx >= 0 && idx < len(actions) {
			return s, actions[idx].run()
		}
	}
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)
	return s, cmd
}

func (s *statusScreen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(1, layout.Sized(&s.menu)),
		layout.Flex(1, layout.Sized(&s.hint)),
	)
}

func (s *statusScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "fire")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *statusScreen) SetTheme(t theme.Theme) {
	s.t = t

	cursor := s.menu.Cursor()
	opts := t.List()
	opts.Title = "actions"
	labels := make([]string, len(actions))
	for i, a := range actions {
		labels[i] = a.label
	}
	opts.Items = labels
	s.menu = list.New(opts)
	s.menu.SetCursor(cursor)

	s.hint = pane.New(t.Pane())
	s.hint.SetTitle("about")
	s.hint.SetContent(
		"Each action returns a tea.Cmd from the screen's\n" +
			"Update — app.Info, app.Error, or app.ClearStatus.\n" +
			"The app shell intercepts the resulting message and\n" +
			"updates the statusbar's center slot.\n\n" +
			"Auto-clear: any keypress wipes the message before\n" +
			"the screen handles it, so a new message can be set\n" +
			"by the same key (try enter on a populated row).\n\n" +
			"Look down at the statusbar after each enter.",
	)
}
