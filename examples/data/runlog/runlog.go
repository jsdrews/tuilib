// Package runlog demonstrates streaming a subprocess's stdout/stderr
// into pkg/logview, with tab-cycling focus between a command picker on
// the left and the logview on the right.
//
// The streaming pattern: cmd.Stdout/Stderr point at a shared io.Pipe;
// a goroutine waits on the process and closes the pipe when it exits;
// a tea.Cmd reads one line at a time from the pipe and posts logLineMsg,
// chaining itself for the next line until EOF (logDoneMsg). No goroutine
// touches the model directly — every mutation flows through Update.
package runlog

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	lv "github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the runlog demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t       theme.Theme
	cmds    list.Model
	log     lv.Model
	focus   focus.Group
	running *exec.Cmd
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
	{"ping -c 5 8.8.8.8", func() *exec.Cmd { return exec.Command("ping", "-c", "5", "8.8.8.8") }},
}

type startedMsg struct {
	cmd     *exec.Cmd
	scanner *bufio.Scanner
	waitErr chan error
}
type logLineMsg struct {
	line    string
	scanner *bufio.Scanner
	waitErr chan error
}
type logDoneMsg struct{ err error }

func (s *Screen) Title() string         { return "Runlog" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case startedMsg:
		s.running = m.cmd
		return s, readLine(m.scanner, m.waitErr)
	case logLineMsg:
		s.log.Append(m.line)
		return s, readLine(m.scanner, m.waitErr)
	case logDoneMsg:
		s.running = nil
		suffix := "exited"
		if m.err != nil {
			suffix = fmt.Sprintf("exited: %s", m.err)
		}
		s.log.Append("─── " + suffix)
		return s, nil
	}

	// The group needs *every* message, not just tab: it also grants the
	// focus requests a clicked component sends. Feeding it only tab keys
	// drops those, so a click lights a pane while the keyboard stays put.
	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)

	// Enter and double-click both mean "run the selected command", so they
	// resolve through one predicate rather than two branches that can drift.
	if s.focus.Is(&s.cmds) && s.running == nil && s.cmds.IsActivate(msg) {
		if idx := s.cmds.Cursor(); idx >= 0 && idx < len(entries) {
			s.log.Clear()
			s.log.Append("$ " + entries[idx].label)
			return s, tea.Batch(gcmd, startCmd(entries[idx].build()))
		}
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.log.Searching() {
		switch k.String() {
		case "c":
			if s.running != nil {
				_ = s.running.Process.Signal(syscall.SIGINT)
				s.log.Append("─── SIGINT sent")
			}
			return s, nil
		case "x":
			if s.running != nil {
				_ = s.running.Process.Kill()
				s.log.Append("─── SIGKILL sent")
			}
			return s, nil
		}
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
// groups — and adds this screen's own verbs, the process-control pair
// appearing only while something is running.
func (s *Screen) HelpSections() []help.Section {
	own := []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	if s.running != nil {
		own = append(own,
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "interrupt (SIGINT)")),
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "force-kill (SIGKILL)")),
		)
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

// startCmd starts cmd with stdout+stderr merged into a single pipe and
// returns a tea.Cmd whose first message is startedMsg (carrying a scanner
// over the pipe). A background goroutine waits on the process and closes
// the pipe when it exits — that's what lets readLine see EOF and post
// logDoneMsg.
func startCmd(cmd *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		pr, pw := io.Pipe()
		cmd.Stdout = pw
		cmd.Stderr = pw
		if err := cmd.Start(); err != nil {
			return logDoneMsg{err: err}
		}
		waitErr := make(chan error, 1)
		go func() {
			err := cmd.Wait()
			_ = pw.Close()
			waitErr <- err
		}()
		return startedMsg{cmd: cmd, scanner: bufio.NewScanner(pr), waitErr: waitErr}
	}
}

// readLine pulls one line from the scanner. If the scanner is at EOF it
// drains the wait error and returns logDoneMsg; otherwise it returns
// logLineMsg carrying the line and the same scanner+waitErr so Update
// can chain readLine again for the next line.
func readLine(scanner *bufio.Scanner, waitErr chan error) tea.Cmd {
	return func() tea.Msg {
		if scanner.Scan() {
			return logLineMsg{line: scanner.Text(), scanner: scanner, waitErr: waitErr}
		}
		select {
		case err := <-waitErr:
			return logDoneMsg{err: err}
		default:
			return logDoneMsg{}
		}
	}
}
