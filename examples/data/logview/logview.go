// Package logview demonstrates pkg/logview as a single-screen app: a
// synthetic log stream you can search ("/"), jump between matches (n/N),
// scroll (pgup/pgdn/arrows), and pin (G to follow, g to top).
package logview

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	lv "github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the logview demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{seq: 1}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t   theme.Theme
	log lv.Model
	seq int
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (s *Screen) Title() string         { return "Logview" }
func (s *Screen) Init() tea.Cmd         { return tea.Batch(textinput.Blink, tick()) }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.log.Searching() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if _, ok := msg.(tickMsg); ok {
		s.log.AppendLines(s.nextBatch())
		return s, tick()
	}
	var cmd tea.Cmd
	s.log, cmd = s.log.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.log) }

func (s *Screen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	return append(base, s.log.Help()...)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	q := s.log.Query()
	opts := t.Logview()
	opts.Title = "tail -f /var/log/synthetic"
	opts.Searchable = true
	opts.MaxLines = 5000
	opts.Filter.Placeholder = "search…"
	s.log = lv.New(opts)
	if q != "" {
		s.log.SetQuery(q)
	}
}

var (
	levels   = []string{"INFO", "INFO", "INFO", "DEBUG", "WARN", "ERROR"}
	services = []string{"api", "worker", "scheduler", "cache", "auth"}
	verbs    = []string{
		"started", "completed", "failed", "retrying", "queued", "rate-limited",
		"cache miss", "cache hit", "rolled back", "committed", "evicted",
	}
)

func (s *Screen) nextBatch() []string {
	n := 1 + rand.Intn(3)
	out := make([]string, n)
	for i := range n {
		now := time.Now().Format("15:04:05.000")
		level := levels[rand.Intn(len(levels))]
		svc := services[rand.Intn(len(services))]
		verb := verbs[rand.Intn(len(verbs))]
		out[i] = fmt.Sprintf("%s  %-5s  %s  job#%-5d  %s",
			now, level, svc, s.seq, verb)
		s.seq++
	}
	return out
}
