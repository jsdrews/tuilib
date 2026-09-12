// Package modals demonstrates the three weights of feedback a screen can
// give, on one list of operations, so the difference between them is
// visible side by side rather than across two demos:
//
//	app.Info       passive. A successful op puts one line in the statusbar
//	               and the next keypress wipes it (rule 20).
//	pkg/confirm    a gate. A destructive op asks first, and the cancel side
//	               starts highlighted (rule 22).
//	pkg/alert      an acknowledgement. A failure the user must see and
//	               dismiss before continuing (rule 23).
//
// Enter runs the highlighted op. "Force push main" goes through both
// modals in sequence — confirm the destructive thing, then acknowledge
// that it was rejected — which is the shape real tooling ends up in.
//
// Both modals resolve by message (confirm.ConfirmedMsg / CancelledMsg,
// alert.DismissedMsg) posted as tea.Cmds; this screen owns show/hide
// state, gates IsCapturingKeys while either is up, and overlays them in a
// ZStack. Note the two hosting shapes: the confirm modal is a fixed size
// inside layout.Center, while the autosizing alert centers itself and
// needs no Center wrapper.
package modals

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	talert "github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/app"
	tconfirm "github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the modals demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type op struct {
	label string
	// confirm, when set, makes the op destructive: nothing happens until the
	// confirm modal is answered. verb labels its affirmative button.
	confirm string
	verb    string
	// fails, when set, is what the op reports instead of succeeding. It goes
	// through the alert modal rather than the statusbar, because an error the
	// user might want to read twice does not belong in a slot the next
	// keypress wipes.
	fails string
	// ok is the statusbar line for a successful run.
	ok string
}

var ops = []op{
	{label: "Save document", ok: "Saved users.go."},
	{label: "Format users.go", ok: "Formatted users.go — 3 lines changed."},
	{
		label: "Connect to server",
		fails: "Connection refused: localhost:8080.\nIs the server running?",
	},
	{
		label: "Run migration 0042",
		fails: "Migration failed: column \"email\" already exists\nin table \"users\".",
	},
	{
		label:   "Delete build cache",
		confirm: "Delete the build cache?\n412 MB across 1,284 objects.",
		verb:    "Delete",
		ok:      "Deleted build cache — 412 MB freed.",
	},
	{
		label:   "Drop staging database",
		confirm: "Drop staging database \"app_staging\"?\nThis cannot be undone.",
		verb:    "Drop",
		ok:      "Dropped app_staging.",
	},
	{
		label:   "Force push main",
		confirm: "Force push main to origin?\n7 commits on the remote would be lost.",
		verb:    "Force push",
		// Long enough to need the alert's autosize + internal scroll.
		fails: "remote: error: GH006: Protected branch update failed for refs/heads/main.\n" +
			"remote: error: Cannot force-push to a protected branch.\n" +
			"remote:\n" +
			"remote: Required status checks are expected:\n" +
			"remote:   ci/build\n" +
			"remote:   ci/test\n" +
			"remote:   security/scan\n" +
			"remote: At least 1 approving review is required by reviewers with write access.\n" +
			"hint: Updates were rejected because the remote contains work that you do\n" +
			"hint: not have locally. Integrate the remote changes ('git pull') before\n" +
			"hint: pushing again, or open a pull request instead.\n" +
			"error: failed to push some refs to 'github.com:acme/app.git'",
	},
}

type Screen struct {
	t    theme.Theme
	list list.Model

	confirm   tconfirm.Model
	confirmUp bool
	alert     talert.Model
	alertUp   bool

	pending int // index into ops for the op a modal is asking about
}

func (s *Screen) Title() string       { return "Modals" }
func (s *Screen) Init() tea.Cmd       { return nil }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

func (s *Screen) IsCapturingKeys() bool {
	return s.confirmUp || s.alertUp || s.list.Filtering()
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// Result messages first, so a dismissed modal doesn't see the key that
	// dismissed it a second time.
	switch msg.(type) {
	case tconfirm.ConfirmedMsg:
		s.confirmUp = false
		return s, s.run(ops[s.pending])
	case tconfirm.CancelledMsg:
		s.confirmUp = false
		return s, app.Info("Cancelled — " + ops[s.pending].label + " did not run.")
	case talert.DismissedMsg:
		s.alertUp = false
		return s, nil
	}

	// While a modal is up it owns input: every message goes to it and
	// nothing reaches the list underneath.
	if s.confirmUp {
		var cmd tea.Cmd
		s.confirm, cmd = s.confirm.Update(msg)
		return s, cmd
	}
	if s.alertUp {
		var cmd tea.Cmd
		s.alert, cmd = s.alert.Update(msg)
		return s, cmd
	}

	if s.list.IsActivate(msg) {
		if idx, ok := s.list.SelectedIndex(); ok && idx >= 0 && idx < len(ops) {
			s.pending = idx
			o := ops[idx]
			if o.confirm != "" {
				s.openConfirm(o)
				return s, nil
			}
			return s, s.run(o)
		}
	}

	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// run is the op's actual effect: a line in the statusbar, or a modal the
// user has to dismiss.
func (s *Screen) run(o op) tea.Cmd {
	if o.fails != "" {
		s.openAlert(o)
		return nil
	}
	return app.Info(o.ok)
}

func (s *Screen) Layout() layout.Node {
	body := layout.Sized(&s.list)
	switch {
	case s.confirmUp:
		return layout.ZStack(body, layout.Center(58, 8, layout.Sized(&s.confirm)))
	case s.alertUp:
		// Autosize sizes and centers the alert against the rect it is given,
		// so the Center wrapper the confirm modal needs is redundant here.
		return layout.ZStack(body, layout.Sized(&s.alert))
	}
	return body
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections reflects the active context: while a modal is up its keys
// are the only ones that do anything, so they are the only ones listed.
func (s *Screen) HelpSections() []help.Section {
	switch {
	case s.confirmUp:
		return help.SectionsOf(&s.confirm)
	case s.alertUp:
		return help.SectionsOf(&s.alert)
	}
	return help.SectionsOf(&s.list, help.Group("Operations",
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
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

	// A modal that is up has to survive the swap too, with the side it was
	// sitting on (rule 4).
	if s.confirmUp {
		side := s.confirm.Value()
		s.openConfirm(ops[s.pending])
		s.confirm.SetValue(side)
	}
	if s.alertUp {
		s.openAlert(ops[s.pending])
	}
}

func (s *Screen) openConfirm(o op) {
	s.confirmUp = true

	cOpts := s.t.Confirm()
	cOpts.Title = o.label
	cOpts.Message = o.confirm
	cOpts.Confirm = o.verb
	cOpts.Cancel = "Cancel"
	// Initial defaults to false — the cancel side starts highlighted, which
	// is what every op reaching this path deserves.
	s.confirm = tconfirm.New(cOpts)
}

// openAlert builds an error-styled alert. The tint comes from overriding
// theme.Alert()'s neutral chrome with t.ErrorBG — the component is
// palette-agnostic, and the semantics live in the override.
func (s *Screen) openAlert(o op) {
	s.alertUp = true

	aOpts := s.t.Alert()
	aOpts.Title = "Error · " + o.label
	aOpts.Message = o.fails
	aOpts.OK = "Dismiss"
	aOpts.ActiveColor = s.t.ErrorBG
	aOpts.OKStyle = lipgloss.NewStyle().Bold(true).Foreground(s.t.ErrorBG)
	aOpts.Autosize = true
	s.alert = talert.New(aOpts)
}

func opLabels() []string {
	out := make([]string, len(ops))
	for i, o := range ops {
		out[i] = o.label
	}
	return out
}
