// Package processes is the subprocess showcase: running commands, streaming
// their output, handing the terminal over, and the app-wide console that
// keeps everything said — on one screen, so a change that breaks an older
// feature shows up where you can see it.
//
//	┌ commands ───────────┐┌ output ─────────────────────────────────┐
//	│ grouped menu:       ││ a searchable, capped logview: what a     │
//	│ capture / terminal  ││ capture streams, and the synthetic tail  │
//	│ / console           ││                                          │
//	├ about ──────────────┤│                                          │
//	│ the highlighted one ││                                          │
//	└─────────────────────┘└──────────────────────────────────────────┘
//
// What it exercises:
//
//   - runner.Capture (rule 15): the TUI stays live while a command streams.
//     Under the shell there is no plumbing: the screen matches the three
//     messages a capture emits — CaptureStarted, one CapturedLine per line,
//     Captured — and the shell chains the reads. The same stream feeds the
//     console (o) at the same time, with the exit status. Kill is on the
//     action menu (a) while a capture runs.
//   - runner.Run and RunWithNotice (rule 11): the terminal handed to an
//     editor, a pager or a full-screen TUI, the result shown on return.
//   - The console (rule 14): app.Info / Error, ErrorDetail's body and
//     ErrorOf's %w chain, all kept after the statusbar wipes them.
//   - A logview following a stream (rules 13, 18): search, n / N, G to
//     follow, a cap on what is kept.
//   - OnEnter's OutputClosed guard: closing the console pops back here, and
//     the activation count in the about pane must not move when it does.
//   - pkg/menu inside a focus.Group: tab cycles panes, [ / ] jump the
//     menu's groups, enter or a double click runs the highlighted command.
package processes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
)

const outputCap = 5000

// New returns the processes screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{seq: 1}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t     theme.Theme
	menu  menu.Model
	out   logview.Model
	about textview.Model
	focus focus.Group

	// started is the capture running, kept so Kill can stop it.
	started runner.CaptureStarted
	running bool
	// tailing is the synthetic tail; tailGen drops the ticks of one stopped
	// and restarted, so a stop really stops.
	tailing bool
	tailGen int
	seq     int
	// last is what the last terminal command returned.
	last *runner.Result
	// activations counts OnEnter calls that were real activations, so the
	// OutputClosed guard is observable rather than merely asserted.
	activations int
	shown       string
}

func (s *Screen) Title() string         { return "Processes" }
func (s *Screen) Init() tea.Cmd         { return tea.Batch(textinput.Blink, s.focus.Init()) }
func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

// OnEnter guards against the console. A pop hands OnEnter a result, and
// nothing but the result tells "I was just pushed" from "someone glanced at
// the log". A screen that fetches on activation needs this branch, or opening
// the console silently refetches whatever was underneath.
func (s *Screen) OnEnter(result any) tea.Cmd {
	if _, closed := result.(app.OutputClosed); closed {
		return nil
	}
	s.activations++
	s.showAbout(true)
	return nil
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// The three messages a capture emits. Nothing chains the next read here:
	// the app shell does, because a capture nobody drains stalls its process.
	switch m := msg.(type) {
	case runner.CaptureStarted:
		s.started, s.running = m, true
		return s, nil
	case runner.CapturedLine:
		// Stderr is a stream, not a severity, so the marker is informational.
		if m.Stderr {
			s.out.Append("2> " + m.Text)
		} else {
			s.out.Append(m.Text)
		}
		return s, nil
	case runner.Captured:
		s.running = false
		if m.Err != nil {
			s.out.AppendMarker(m.Label + " failed: " + m.Err.Error())
		} else {
			s.out.AppendMarker(m.Label + " completed")
		}
		return s, nil
	case runner.Result:
		s.last = &m
		s.showAbout(true)
		return s, nil
	case tailMsg:
		if !s.tailing || m.gen != s.tailGen {
			return s, nil
		}
		s.out.AppendLines(s.tailBatch())
		return s, tailTick(s.tailGen)
	}

	var cmds []tea.Cmd
	var c tea.Cmd
	s.focus, c = s.focus.Update(msg)
	cmds = append(cmds, c)

	if it, ok := s.menu.Chosen(msg); ok {
		cmds = append(cmds, s.start(commands[it.Data.(int)]))
	}

	if _, isKey := msg.(tea.KeyMsg); isKey {
		switch {
		case s.focus.Is(&s.menu):
			s.menu, c = s.menu.Update(msg)
		case s.focus.Is(&s.out):
			s.out, c = s.out.Update(msg)
		case s.focus.Is(&s.about):
			s.about, c = s.about.Update(msg)
		}
		cmds = append(cmds, c)
	} else {
		var a, b tea.Cmd
		s.menu, a = s.menu.Update(msg)
		s.out, b = s.out.Update(msg)
		s.about, c = s.about.Update(msg)
		cmds = append(cmds, a, b, c)
	}
	s.showAbout(false)
	return s, tea.Batch(cmds...)
}

// start runs a command the way its kind says.
func (s *Screen) start(c command) tea.Cmd {
	switch c.kind {
	case capture:
		if s.running {
			return app.Info("A capture is already running — Kill it from the action menu first.")
		}
		s.out.AppendMarker("$ " + c.label)
		return runner.CaptureWith(runner.CaptureOptions{Cmd: c.build(), Label: c.label})
	case tail:
		s.tailing = !s.tailing
		s.tailGen++
		if !s.tailing {
			s.out.AppendMarker("tail stopped")
			return nil
		}
		s.out.AppendMarker("tail -f /var/log/synthetic")
		return tailTick(s.tailGen)
	case terminal:
		if c.notice != "" {
			return runner.RunWithNotice(c.build(), c.notice)
		}
		return runner.Run(c.build())
	}
	return c.run()
}

// Actions satisfies action.Provider: what can be done to the output.
func (s *Screen) Actions() action.Set {
	kill := action.Action{Label: "Kill", Desc: "stop the running capture", Group: "run", Do: func() tea.Cmd {
		_ = runner.Kill(s.started)
		s.out.AppendMarker("killed")
		return nil
	}}
	if !s.running {
		kill.Disabled = "nothing is running"
	}
	set := action.Set{Target: "output", Actions: []action.Action{kill}}
	if s.tailing {
		set.Actions = append(set.Actions, action.Action{Label: "Stop the tail", Group: "run", Do: func() tea.Cmd {
			return s.start(command{kind: tail})
		}})
	}
	set.Actions = append(set.Actions, action.Action{Label: "Clear output", Group: "output", Do: func() tea.Cmd {
		s.out.Clear()
		return nil
	}})
	return set
}

// showAbout describes the highlighted command, when it changed or when what
// the pane reports did.
func (s *Screen) showAbout(force bool) {
	it, ok := s.menu.Selected()
	if !ok || (it.ID == s.shown && !force) {
		return
	}
	s.shown = it.ID
	c := commands[it.Data.(int)]
	lines := []string{c.blurb, ""}
	if s.last != nil {
		lines = append(lines, "last terminal command: "+result(*s.last))
	}
	lines = append(lines,
		fmt.Sprintf("activations: %d — closing the console must not move it", s.activations))
	s.about.SetTitle(c.label)
	s.about.SetContent(strings.Join(lines, "\n"))
}

func result(r runner.Result) string {
	desc := strings.Join(r.Cmd.Args, " ")
	switch {
	case r.Err != nil:
		return desc + " — error: " + r.Err.Error()
	case r.Cmd.ProcessState != nil:
		return fmt.Sprintf("%s — exit %d", desc, r.Cmd.ProcessState.ExitCode())
	}
	return desc + " — exited"
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.VStack(
			layout.Flex(3, layout.Sized(&s.menu)),
			layout.Flex(2, layout.Sized(&s.about)),
		)),
		layout.Flex(3, layout.Sized(&s.out)),
	)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections is the Group's. It carries no "o": the shell advertises the
// output key itself, so a screen restating it would show it twice.
func (s *Screen) HelpSections() []help.Section { return s.focus.HelpSections() }

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	mst := s.menu.State()
	mo := t.Menu()
	mo.Title = "commands"
	mo.Items = items()
	s.menu = menu.New(mo)
	s.menu.Restore(mst)

	ost := s.out.State()
	oo := t.Logview()
	oo.Title = "output"
	oo.Searchable = true
	oo.MaxLines = outputCap
	oo.Filter.Placeholder = "search…"
	s.out = logview.New(oo)
	s.out.Restore(ost)

	ast := s.about.State()
	ao := t.TextView()
	ao.Wrap = true
	s.about = textview.New(ao)
	s.about.Restore(ast)
	s.showAbout(true)

	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.menu, &s.out, &s.about).WithKeys(focus.Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev pane")),
	})
	s.focus.SetIndex(at)
}

func items() []menu.Item {
	out := make([]menu.Item, len(commands))
	for i, c := range commands {
		out[i] = menu.Item{ID: c.label, Label: c.label, Group: groupOf[c.kind], Data: i}
	}
	return out
}
