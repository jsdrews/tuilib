// Package output demonstrates the app-wide output console: everything the
// app has said, kept, and readable long after the statusbar wiped it.
//
// The problem it solves is visible from this screen. Pick "Deploy fails with
// a wrapped error" and watch the footer: you get one truncated line, and it
// is gone on the next keypress. The full %w chain never had anywhere to go.
// Press "o" and it is all still there.
//
// Four things feed the console, and this screen exercises all of them:
//
//   - app.Info / app.Error — captured automatically, no code change needed.
//   - app.InfoDetail / app.ErrorDetail — summary to the bar, body to the log.
//   - app.ErrorOf — the %w chain, one wrap per line, without hand-formatting.
//   - runner.Capture — a subprocess whose stdout/stderr stream into the log
//     while the TUI stays live. Start the 30-second one and keep navigating;
//     the badge shows ⟳ while it runs, and "x" on the console kills it.
//
// The console is opt-in: none of it exists unless app.Options.OutputKey is
// set. See examples/launcher/main.go, which sets it for the whole suite.
//
// Note OnEnter: closing the console pops with app.OutputClosed, and a screen
// whose OnEnter starts a fetch has to say so, or glancing at a log silently
// refetches whatever was underneath.
package output

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the output-console demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t     theme.Theme
	menu  list.Model
	notes pane.Pane

	// fetches counts OnEnter activations that actually did work, so the
	// OutputClosed guard is observable rather than merely asserted.
	fetches int
}

type action struct {
	label string
	blurb string
	run   func() tea.Cmd
}

var actions = []action{
	{
		"Info — one short line",
		"app.Info(\"…\"). Paints the statusbar and is captured verbatim. Nothing " +
			"about existing call sites has to change for them to become recoverable.",
		func() tea.Cmd { return app.Info("deployment triggered") },
	},
	{
		"Error — one short line",
		"app.Error(\"…\"). Same, at error level, which is what tints the badge red.",
		func() tea.Cmd { return app.Error("deployment rejected: quota exceeded") },
	},
	{
		"Error with a body — 12 lines of stderr",
		"app.ErrorDetail(summary, body). The summary paints the bar exactly as " +
			"app.Error does; the body goes only to the console. This is the case the " +
			"statusbar physically cannot serve.",
		func() tea.Cmd {
			return app.ErrorDetail("apply failed: 3 resources rejected", stderrBlob)
		},
	},
	{
		"Deploy fails with a wrapped error",
		"app.ErrorOf(err). The bar gets err.Error(); the console gets the unwrapped " +
			"%w chain, one wrap per line. Without this the chain is flattened to its " +
			"outermost message, because hand-formatting it at each call site is work " +
			"nobody does.",
		func() tea.Cmd { return app.ErrorOf(wrappedFailure()) },
	},
	{
		"Capture — 40 lines, stdout and stderr",
		"runner.Capture(cmd). No terminal handoff and no suspend: the TUI stays " +
			"live and the output streams into the console. stderr lines get a heavier " +
			"gutter (┃) but are not treated as errors — the tint comes from the exit " +
			"status, since plenty of tools log progress to stderr.",
		func() tea.Cmd {
			return runner.Capture(exec.Command("sh", "-c",
				`for i in $(seq 1 40); do
				   if [ $((i % 7)) -eq 0 ]; then echo "warn: retry $i" 1>&2; else echo "step $i ok"; fi
				   sleep 0.05
				 done`))
		},
	},
	{
		"Capture — fails with exit 2",
		"The completion line lands at error level and turns the badge red. It is a " +
			"continuation of the run, not a new event, so a 40-line failure still " +
			"counts as one thing that happened.",
		func() tea.Cmd {
			return runner.Capture(exec.Command("sh", "-c",
				`echo "resolving dependencies"; echo "error: no matching version" 1>&2; exit 2`))
		},
	},
	{
		"Capture — 30s, so you can watch it run",
		"Start it and navigate away. The badge carries ⟳ while anything is in " +
			"flight. Open the console and press x to kill it — with two running you " +
			"get a picker first, then a confirmation.",
		func() tea.Cmd {
			return runner.CaptureWith(runner.CaptureOptions{
				Label: "slowbuild",
				Cmd: exec.Command("sh", "-c",
					`for i in $(seq 1 60); do echo "building chunk $i"; sleep 0.5; done`),
			})
		},
	},
	{
		"Burst — 500 lines, to watch the ring trim",
		"Exceeds nothing by default (the cap is 10k) but shows the console under " +
			"a fast producer. Set a small output.Options.MaxRecords to watch trimming " +
			"cut on event boundaries instead of mid-run.",
		func() tea.Cmd {
			return runner.Capture(exec.Command("sh", "-c", "seq 1 500"))
		},
	},
}

const stderrBlob = `+ kubectl apply -f manifests/
deployment.apps/api configured
error: unable to recognize "manifests/rollout.yaml": no matches for kind "Rollout" in version "argoproj.io/v1alpha1"
error: unable to recognize "manifests/scaler.yaml": no matches for kind "ScaledObject" in version "keda.sh/v1alpha1"
error: unable to recognize "manifests/policy.yaml": no matches for kind "PodMonitor" in version "monitoring.coreos.com/v1"

The cluster is missing three CRDs. Install them with:
  kubectl apply -f https://.../argo-rollouts/crds.yaml
  kubectl apply -f https://.../keda/crds.yaml
  kubectl apply -f https://.../prometheus-operator/crds.yaml

3 resources rejected, 1 configured.`

func wrappedFailure() error {
	base := errors.New("dial tcp 10.0.4.12:443: connect: connection refused")
	api := fmt.Errorf("GET /api/v1/deployments: %w", base)
	client := fmt.Errorf("cluster \"prod-eu\" unreachable: %w", api)
	return fmt.Errorf("deploy aborted: %w", client)
}

// Title is "Console" rather than "Output" so the breadcrumb reads
// "Examples › Console › Output" when the log is open, instead of the same
// word twice.
func (s *Screen) Title() string         { return "Console" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) IsCapturingKeys() bool { return s.menu.IsCapturingKeys() }

// OnEnter guards against the console.
//
// A pop hands OnEnter a result, and there is no way to tell "I was just
// pushed" from "someone glanced at a log" except by looking at it. Screens
// that fetch on activation — most of them — need this branch, or opening the
// console becomes an accidental refresh of whatever was underneath.
func (s *Screen) OnEnter(result any) tea.Cmd {
	if _, closed := result.(app.OutputClosed); closed {
		return nil
	}
	s.fetches++
	s.rebuildNotes()
	return nil
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	prevIdx, prevOK := s.menu.SelectedIndex()

	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)

	if idx, ok := s.menu.SelectedIndex(); idx != prevIdx || ok != prevOK {
		s.rebuildNotes()
	}

	if s.menu.IsActivate(msg) {
		if idx, ok := s.menu.SelectedIndex(); ok && idx >= 0 && idx < len(actions) {
			return s, tea.Batch(cmd, actions[idx].run())
		}
	}
	return s, cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.menu)),
		layout.Flex(3, layout.Sized(&s.notes)),
	)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections carries no "o" binding: the shell advertises the output key
// itself, so a screen restating it would show it twice.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.menu, help.Group("Console demo",
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run action")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.menu.Cursor(), s.menu.Value()
	opts := t.List()
	opts.Title = "actions"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter…"
	opts.Items = actionLabels()
	s.menu = list.New(opts)
	if value != "" {
		s.menu.SetValue(value)
	}
	s.menu.SetCursor(cursor)

	s.rebuildNotes()
}

func (s *Screen) rebuildNotes() {
	s.notes = pane.New(s.t.Pane())
	s.notes.SetTitle("about")

	idx, ok := s.menu.SelectedIndex()
	if !ok || idx < 0 || idx >= len(actions) {
		s.notes.SetContent("Pick an action and press enter, then press o to read the log.")
		return
	}
	a := actions[idx]
	s.notes.SetContent(strings.Join([]string{
		a.label,
		"",
		a.blurb,
		"",
		fmt.Sprintf("OnEnter activations that fetched: %d", s.fetches),
		"(open and close the console — this must not move)",
	}, "\n"))
}

func actionLabels() []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = a.label
	}
	return out
}
