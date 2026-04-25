// Package runner demonstrates pkg/runner: pick an interactive subprocess
// from a list, hand the terminal to it, and return to the TUI on exit.
// The status pane shows the last result (exit code or error).
package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/runner"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the runner demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t      theme.Theme
	list   list.Model
	status pane.Pane
	last   *runner.Result
}

type option struct {
	label string
	build func() *exec.Cmd
}

var commands = []option{
	{"$EDITOR /tmp/tuilib-scratch.txt", func() *exec.Cmd {
		ed := os.Getenv("EDITOR")
		if ed == "" {
			ed = "vi"
		}
		return exec.Command(ed, "/tmp/tuilib-scratch.txt")
	}},
	{"less /etc/hosts", func() *exec.Cmd { return exec.Command("less", "/etc/hosts") }},
	{"man ls", func() *exec.Cmd { return exec.Command("man", "ls") }},
	{"htop", func() *exec.Cmd { return exec.Command("htop") }},
	{"sh -c 'echo hello; sleep 1'", func() *exec.Cmd { return exec.Command("sh", "-c", "echo hello; sleep 1") }},
}

func (s *Screen) Title() string         { return "Runner" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.list.Filtering() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if r, ok := msg.(runner.Result); ok {
		s.last = &r
		s.refreshStatus()
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	if k, ok := msg.(tea.KeyMsg); ok && !s.list.Filtering() && k.String() == "enter" {
		idx := s.list.Cursor()
		if idx >= 0 && idx < len(commands) {
			return s, tea.Batch(cmd, runner.Run(commands[idx].build()))
		}
	}
	return s, cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.VStack(
		layout.Flex(1, layout.Sized(&s.list)),
		layout.Fixed(4, layout.Sized(&s.status)),
	)
}

func (s *Screen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.list.Cursor(), s.list.Value()
	opts := t.List()
	opts.Title = "interactive subprocess"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter…"
	opts.Items = labels()
	s.list = list.New(opts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)

	s.status = pane.New(t.Pane())
	s.status.SetTitle("last result")
	s.refreshStatus()
}

func (s *Screen) refreshStatus() {
	if s.last == nil {
		s.status.SetContent("Press enter to run the highlighted command.\nThe TUI suspends; on exit you return here.")
		return
	}
	desc := strings.Join(s.last.Cmd.Args, " ")
	if s.last.Err != nil {
		s.status.SetContent(fmt.Sprintf("%s\nerror: %s", desc, s.last.Err))
		return
	}
	if state := s.last.Cmd.ProcessState; state != nil {
		s.status.SetContent(fmt.Sprintf("%s\nexit code: %d", desc, state.ExitCode()))
		return
	}
	s.status.SetContent(desc + "\nexited cleanly")
}

func labels() []string {
	out := make([]string, len(commands))
	for i, c := range commands {
		out[i] = c.label
	}
	return out
}
