// Package capture demonstrates streaming a subprocess's output into an
// on-screen pkg/logview with runner.Capture, next to a command picker, with
// tab cycling focus between the two.
//
// Capture is the counterpart to runner.Run: Run hands the real terminal to
// a full-screen program and the TUI suspends, while Capture pipes
// stdout/stderr and the TUI stays live — so the user keeps scrolling,
// filtering and searching while the command runs (rule 15).
//
// Under the app shell there is no plumbing to write. The shell chains the
// reads and forwards every message on, so a screen that wants the output in
// place just matches the three messages a capture emits: CaptureStarted,
// one CapturedLine per line, then one Captured. The same stream is feeding
// the app-wide console at the same time — press o and the run is all there,
// with its exit status, after this screen has scrolled it away.
package capture

import (
	"os/exec"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	lv "github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the capture demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t     theme.Theme
	cmds  list.Model
	log   lv.Model
	focus focus.Group

	// started is the live run, kept so x can kill it. Zero value means
	// nothing is running — runner.Kill on a finished run is a no-op, but the
	// help strip still needs to know whether to advertise the key.
	started runner.CaptureStarted
	running bool
}

type entry struct {
	label string
	build func() *exec.Cmd
}

var entries = []entry{
	{"seq with sleep (50 lines)", func() *exec.Cmd {
		return exec.Command("sh", "-c", "for i in $(seq 1 50); do echo line $i; sleep 0.05; done")
	}},
	{"ls -la /usr/bin", func() *exec.Cmd { return exec.Command("ls", "-la", "/usr/bin") }},
	{"find /etc -maxdepth 2 -type f", func() *exec.Cmd {
		return exec.Command("find", "/etc", "-maxdepth", "2", "-type", "f")
	}},
	{"echo to stdout + stderr", func() *exec.Cmd {
		return exec.Command("sh", "-c", "echo OUT && echo ERR 1>&2 && echo MORE")
	}},
	{"exit 2 after a few lines", func() *exec.Cmd {
		return exec.Command("sh", "-c", "echo building…; echo 'cc: error: undefined symbol' 1>&2; exit 2")
	}},
	{"ping -c 5 8.8.8.8", func() *exec.Cmd { return exec.Command("ping", "-c", "5", "8.8.8.8") }},
}

func (s *Screen) Title() string         { return "Capture" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// The three messages a capture emits. Nothing chains the next read here:
	// the app shell does that, unconditionally, because a capture nobody
	// drains eventually stalls the subprocess.
	switch m := msg.(type) {
	case runner.CaptureStarted:
		// Only the handle is new here — the command line was echoed when the
		// user launched it, so the log doesn't wait on the process to show
		// that something is happening.
		s.started, s.running = m, true
		return s, nil
	case runner.CapturedLine:
		// Stderr is a stream, not a severity — a tool writing progress there
		// is well behaved, so the marker is informational.
		if m.Stderr {
			s.log.Append("2> " + m.Text)
		} else {
			s.log.Append(m.Text)
		}
		return s, nil
	case runner.Captured:
		s.running = false
		if m.Err != nil {
			s.log.Append("─── " + m.Label + " failed: " + m.Err.Error())
		} else {
			s.log.Append("─── " + m.Label + " completed")
		}
		return s, nil
	}

	// The group needs *every* message, not just tab: it also grants the
	// focus requests a clicked component sends. Feeding it only tab keys
	// drops those, so a click lights a pane while the keyboard stays put.
	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)

	// Enter and double-click both mean "run the selected command", so they
	// resolve through one predicate rather than two branches that can drift.
	if s.focus.Is(&s.cmds) && !s.running && s.cmds.IsActivate(msg) {
		if idx := s.cmds.Cursor(); idx >= 0 && idx < len(entries) {
			e := entries[idx]
			s.log.Clear()
			s.log.Append("$ " + e.label)
			return s, tea.Batch(gcmd, runner.CaptureWith(runner.CaptureOptions{
				Cmd:   e.build(),
				Label: e.label,
			}))
		}
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.log.Searching() && k.String() == "x" {
		if s.running {
			_ = runner.Kill(s.started)
			s.log.Append("─── killed")
		}
		return s, nil
	}

	// Mouse goes to every component so each can test the click against its
	// own rect; keys go to the focused one alone.
	if _, isMouse := msg.(mouse.Msg); isMouse {
		var a, b tea.Cmd
		s.cmds, a = s.cmds.Update(msg)
		s.log, b = s.log.Update(msg)
		return s, tea.Batch(gcmd, a, b)
	}
	var cmd tea.Cmd
	if s.focus.Is(&s.cmds) {
		s.cmds, cmd = s.cmds.Update(msg)
	} else {
		s.log, cmd = s.log.Update(msg)
	}
	return s, tea.Batch(gcmd, cmd)
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.cmds)),
		layout.Flex(5, layout.Sized(&s.log)),
	)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections forwards to the Group — which names both panes and their
// groups — and adds this screen's own verbs, the kill key appearing only
// while something is running.
func (s *Screen) HelpSections() []help.Section {
	own := []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	if s.running {
		own = append(own, key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "kill")))
	}
	return help.SectionsOf(&s.focus, help.Group("Run", own...))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.cmds.Cursor(), s.cmds.Value()
	listOpts := t.List()
	listOpts.Title = "commands"
	listOpts.Items = labels()
	s.cmds = list.New(listOpts)
	if value != "" {
		s.cmds.SetValue(value)
	}
	s.cmds.SetCursor(cursor)

	q := s.log.Query()
	prev := s.log.Lines()
	logOpts := t.Logview()
	logOpts.Title = "output"
	logOpts.Searchable = true
	logOpts.MaxLines = 5000
	logOpts.Filter.Placeholder = "search…"
	s.log = lv.New(logOpts)
	if len(prev) > 0 {
		s.log.AppendLines(prev)
	}
	if q != "" {
		s.log.SetQuery(q)
	}

	s.applyFocus()
}

func (s *Screen) applyFocus() {
	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.cmds, &s.log)
	s.focus.SetIndex(at)
}

func labels() []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.label
	}
	return out
}
