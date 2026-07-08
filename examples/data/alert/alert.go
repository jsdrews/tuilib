// Package alert demonstrates pkg/alert in the standard modal pattern,
// alongside the lighter app.Info / app.Error statusbar messages.
//
// A list of mock operations: enter runs the highlighted one. Successful
// operations report through the statusbar (app.Info) and auto-clear on
// the next keypress. Failures pop a modal alert with an error-tinted
// border that the user must dismiss before continuing — the right shape
// for "stop and acknowledge" feedback.
//
// The modal uses autosize (Options.Autosize = true): the pane word-wraps
// its message against 80% of the terminal width, grows vertically up to
// 60% of the terminal height, and scrolls internally when content
// overflows — see the "Merge upstream (long stderr)" op for the
// overflow case. The caller drops the outer layout.Center wrapper and
// composes with just layout.Sized(&s.modal) inside the ZStack.
//
// The result message uses a typed alert.DismissedMsg via tea.Cmd, the
// parent matches in its own Update to dismiss the modal, exactly the
// same pattern as pkg/confirm.
package alert

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	talert "github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the alert demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &alertScreen{}
	s.SetTheme(t)
	return s
}

type op struct {
	label string
	// errMsg is empty for successful ops (→ statusbar info) and non-empty
	// for failures (→ modal alert).
	errMsg string
	// okMsg is the statusbar message on success. Ignored when errMsg is set.
	okMsg string
}

var ops = []op{
	{"Save document", "", "Saved users.go."},
	{"Format file", "", "Formatted users.go."},
	{"Connect to server", "Connection refused: localhost:8080.\nIs the server running?", ""},
	{"Run migration 0042", "Migration failed: column \"email\" already exists\nin table \"users\".", ""},
	{"Deploy to staging", "Deploy aborted: 3 tests failed.\nRun `task test` for details.", ""},
	{
		"Merge upstream (long stderr)",
		"error: Your local changes to the following files would be committed by merge:\n" +
			"  services/api/handlers/users.go\n" +
			"  services/api/handlers/sessions.go\n" +
			"  services/api/handlers/webhooks.go\n" +
			"Please commit your changes or stash them before you merge.\n" +
			"Aborting.\n" +
			"Merge with strategy ort failed.\n" +
			"hint: Run `git status` to see the affected paths, then either\n" +
			"hint: stage + commit the wanted edits and drop the rest, or\n" +
			"hint: run `git stash push -m 'pre-merge'` to defer them.\n" +
			"hint: Rerun the merge once the working tree is clean.",
		"",
	},
}

type alertScreen struct {
	t       theme.Theme
	list    list.Model
	modal   talert.Model
	modalUp bool
}

func (s *alertScreen) Title() string         { return "Alert" }
func (s *alertScreen) Init() tea.Cmd         { return nil }
func (s *alertScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *alertScreen) IsCapturingKeys() bool { return s.modalUp || s.list.Filtering() }

func (s *alertScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if _, ok := msg.(talert.DismissedMsg); ok {
		s.modalUp = false
		return s, nil
	}

	if s.modalUp {
		var cmd tea.Cmd
		s.modal, cmd = s.modal.Update(msg)
		return s, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.list.Filtering() && k.String() == "enter" {
		if idx, ok := s.list.SelectedIndex(); ok && idx >= 0 && idx < len(ops) {
			o := ops[idx]
			if o.errMsg != "" {
				s.openModal(o.label, o.errMsg)
				return s, nil
			}
			return s, app.Info(o.okMsg)
		}
	}

	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *alertScreen) Layout() layout.Node {
	body := layout.Sized(&s.list)
	if !s.modalUp {
		return body
	}
	// Autosize handles centering and content sizing internally, so the
	// caller only needs layout.Sized inside the ZStack overlay.
	return layout.ZStack(body, layout.Sized(&s.modal))
}

func (s *alertScreen) Help() []key.Binding {
	if s.modalUp {
		return s.modal.Help()
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *alertScreen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.list.Cursor(), s.list.Value()
	lOpts := t.List()
	lOpts.Title = "operations"
	lOpts.Filterable = true
	lOpts.Filter.Placeholder = "filter…"
	lOpts.Items = opLabels()
	s.list = list.New(lOpts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)
}

// openModal builds an error-styled alert. The error tint comes from
// overriding theme.Alert()'s neutral chrome with t.ErrorBG — the
// component itself is palette-agnostic; semantics live in the override.
func (s *alertScreen) openModal(opLabel, message string) {
	s.modalUp = true

	aOpts := s.t.Alert()
	aOpts.Title = "Error · " + opLabel
	aOpts.Message = message
	aOpts.OK = "Dismiss"
	aOpts.ActiveColor = s.t.ErrorBG
	aOpts.OKStyle = lipgloss.NewStyle().Bold(true).Foreground(s.t.ErrorBG)
	aOpts.Autosize = true
	s.modal = talert.New(aOpts)
}

func opLabels() []string {
	out := make([]string, len(ops))
	for i, o := range ops {
		out[i] = o.label
	}
	return out
}
