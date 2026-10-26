package workbench

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	talert "github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/app"
	tconfirm "github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/menu"
)

// The operations pane is the three weights of feedback a screen can give,
// side by side:
//
//	notices   app.Info / app.Error / app.ClearStatus: one line in the
//	          statusbar that the next keypress wipes (rule 20).
//	confirm   pkg/confirm: a gate before a destructive op, the cancel side
//	          highlighted first (rule 22).
//	fails     pkg/alert: a failure the user must see and dismiss (rule 23).
//
// "Force push main" goes through both modals in sequence: confirm the
// destructive thing, then acknowledge that it was rejected.

type op struct {
	group, label string
	// run is what a notice does.
	run func() tea.Cmd
	// confirm, when set, gates the op behind the confirm modal; verb labels
	// its affirmative button.
	confirm, verb string
	// fails is what the op reports instead of succeeding, in the alert modal:
	// an error the user may want to read twice does not belong in a slot the
	// next keypress wipes.
	fails string
	ok    string
}

var ops = []op{
	{group: "notices", label: "Save document", run: func() tea.Cmd { return app.Info("Saved users.go.") }},
	{group: "notices", label: "Report an error", run: func() tea.Cmd { return app.Error("Error: lint found 3 problems") }},
	{group: "notices", label: "Clear status", run: app.ClearStatus},
	{
		group: "confirm", label: "Delete build cache",
		confirm: "Delete the build cache?\n412 MB across 1,284 objects.", verb: "Delete",
		ok: "Deleted build cache — 412 MB freed.",
	},
	{
		group: "confirm", label: "Force push main",
		confirm: "Force push main to origin?\n7 commits on the remote would be lost.", verb: "Force push",
		// Long enough to need the alert's autosize and internal scroll.
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
	{group: "fails", label: "Connect to server", fails: "Connection refused: localhost:8080.\nIs the server running?"},
	{group: "fails", label: "Run migration 0042", fails: "Migration failed: column \"email\" already exists\nin table \"users\"."},
}

func opItems() []menu.Item {
	out := make([]menu.Item, len(ops))
	for i, o := range ops {
		out[i] = menu.Item{ID: o.label, Label: o.label, Group: o.group, Data: i}
	}
	return out
}

// start runs op i, or opens the modal it asks for first.
func (s *Screen) start(i int) tea.Cmd {
	s.pending = i
	if ops[i].confirm != "" {
		s.openConfirm()
		return nil
	}
	return s.finish()
}

// finish is the op's effect once nothing stands in its way.
func (s *Screen) finish() tea.Cmd {
	o := ops[s.pending]
	switch {
	case o.fails != "":
		s.openAlert()
		return nil
	case o.run != nil:
		return o.run()
	}
	return app.Info(o.ok)
}

func (s *Screen) openConfirm() {
	o := ops[s.pending]
	s.confirmUp = true
	opts := s.t.Confirm()
	opts.Title = o.label
	opts.Message = o.confirm
	opts.Confirm = o.verb
	opts.Cancel = "Cancel"
	s.confirm = tconfirm.New(opts)
}

// openAlert tints theme.Alert()'s neutral chrome with ErrorBG: the component
// is palette-agnostic, and the meaning lives in the override.
func (s *Screen) openAlert() {
	o := ops[s.pending]
	s.alertUp = true
	opts := s.t.Alert()
	opts.Title = "Error · " + o.label
	opts.Message = o.fails
	opts.OK = "Dismiss"
	opts.ActiveColor = s.t.ErrorBG
	opts.OKStyle = lipgloss.NewStyle().Bold(true).Foreground(s.t.ErrorBG)
	if opts.Chips {
		// A chip carries the tint as its fill.
		opts.OKStyle = lipgloss.NewStyle().Bold(true).Foreground(s.t.ErrorFG).Background(s.t.ErrorBG)
	}
	opts.Autosize = true
	s.alert = talert.New(opts)
}
