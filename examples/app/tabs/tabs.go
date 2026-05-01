// Package tabs demonstrates pkg/tab as the layout primitive of a host
// screen. Three independent sub-screens — a filterable list, a streaming
// logview, and a counter — coexist behind a single-row tab strip. Each tab
// preserves its own state across switches: the logview keeps appending
// lines while the user is on the cities tab, the list keeps its cursor and
// filter, and the counter keeps its tally.
//
// shift+left / shift+right and 1/2/3 switch tabs. tab/shift+tab is left
// alone (reserved for inner pane focus cycling, per the library convention).
// "/" only triggers the active body's filter or search; the tab strip
// suppresses its switch keys while the active body is capturing input.
package tabs

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/tab"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// New returns the tabs demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &host{}
	s.SetTheme(t)
	return s
}

// ---- host screen -----------------------------------------------------------

// host holds pointers to each body so theme rebuilds reuse the same bodies
// (and their cursors / query strings / counters) instead of constructing
// fresh ones. The tab.Model itself is recreated so its strip styling picks
// up the new palette; the bodies' own SetTheme preserves their state.
type host struct {
	t      theme.Theme
	cities *citiesBody
	logs   *logsBody
	info   *infoBody
	tabs   tab.Model
}

func (s *host) Title() string         { return "Tabs › " + s.tabs.ActiveLabel() }
func (s *host) Init() tea.Cmd         { return tea.Batch(textinput.Blink, s.tabs.Init()) }
func (s *host) OnEnter(r any) tea.Cmd { return s.tabs.OnEnterActive(r) }
func (s *host) IsCapturingKeys() bool { return s.tabs.IsCapturingKeys() }
func (s *host) Layout() layout.Node   { return layout.Sized(&s.tabs) }

func (s *host) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tabs, cmd = s.tabs.Update(msg)
	return s, cmd
}

func (s *host) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	return append(base, s.tabs.Help()...)
}

func (s *host) SetTheme(t theme.Theme) {
	s.t = t
	if s.cities == nil {
		s.cities = newCitiesBody(t)
		s.logs = newLogsBody(t)
		s.info = newInfoBody(t)
	} else {
		s.cities.SetTheme(t)
		s.logs.SetTheme(t)
		s.info.SetTheme(t)
	}

	active := s.tabs.ActiveTab() // 0 on first call (zero-value tab.Model)
	s.tabs = tab.New(tab.Options{
		Theme:   t,
		Initial: active,
		Tabs: []tab.Tab{
			{Label: "Cities", Body: s.cities},
			{Label: "Logs", Body: s.logs},
			{Label: "Counter", Body: s.info},
		},
	})
}

// ---- cities body -----------------------------------------------------------

type citiesBody struct {
	t    theme.Theme
	list list.Model
}

func newCitiesBody(t theme.Theme) *citiesBody {
	b := &citiesBody{}
	b.SetTheme(t)
	return b
}

var citiesData = []string{
	"Amsterdam", "Auckland", "Bangkok", "Berlin", "Bogotá",
	"Buenos Aires", "Cairo", "Cape Town", "Chicago", "Hong Kong",
	"Lagos", "Lima", "Lisbon", "London", "Madrid",
	"Melbourne", "Mexico City", "Mumbai", "Nairobi", "New York",
	"Paris", "Prague", "San Francisco", "São Paulo", "Seoul",
	"Singapore", "Sydney", "Taipei", "Tokyo", "Toronto", "Vancouver",
}

func (b *citiesBody) Title() string         { return "Cities" }
func (b *citiesBody) Init() tea.Cmd         { return nil }
func (b *citiesBody) OnEnter(any) tea.Cmd   { return nil }
func (b *citiesBody) IsCapturingKeys() bool { return b.list.Filtering() }

func (b *citiesBody) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	b.list, cmd = b.list.Update(msg)
	return b, cmd
}

func (b *citiesBody) Layout() layout.Node { return layout.Sized(&b.list) }

func (b *citiesBody) Help() []key.Binding { return b.list.Help() }

func (b *citiesBody) SetTheme(t theme.Theme) {
	b.t = t
	cursor, value := b.list.Cursor(), b.list.Value()
	opts := t.List()
	opts.Title = "cities"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter cities…"
	opts.Items = citiesData
	b.list = list.New(opts)
	if value != "" {
		b.list.SetValue(value)
	}
	b.list.SetCursor(cursor)
}

// ---- logs body -------------------------------------------------------------

type logsBody struct {
	t   theme.Theme
	log logview.Model
	seq int
}

type logTickMsg time.Time

func logTick() tea.Cmd {
	return tea.Tick(180*time.Millisecond, func(t time.Time) tea.Msg { return logTickMsg(t) })
}

func newLogsBody(t theme.Theme) *logsBody {
	b := &logsBody{seq: 1}
	b.SetTheme(t)
	return b
}

func (b *logsBody) Title() string         { return "Logs" }
func (b *logsBody) Init() tea.Cmd         { return logTick() }
func (b *logsBody) OnEnter(any) tea.Cmd   { return nil }
func (b *logsBody) IsCapturingKeys() bool { return b.log.Searching() }

func (b *logsBody) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if _, ok := msg.(logTickMsg); ok {
		b.log.AppendLines(b.nextBatch())
		return b, logTick()
	}
	var cmd tea.Cmd
	b.log, cmd = b.log.Update(msg)
	return b, cmd
}

func (b *logsBody) Layout() layout.Node { return layout.Sized(&b.log) }

func (b *logsBody) Help() []key.Binding { return b.log.Help() }

func (b *logsBody) SetTheme(t theme.Theme) {
	b.t = t
	q := b.log.Query()
	opts := t.Logview()
	opts.Title = "tail -f /var/log/synthetic"
	opts.Searchable = true
	opts.MaxLines = 5000
	opts.Filter.Placeholder = "search…"
	b.log = logview.New(opts)
	if q != "" {
		b.log.SetQuery(q)
	}
}

var (
	logLevels = []string{"INFO", "INFO", "INFO", "DEBUG", "WARN", "ERROR"}
	logVerbs  = []string{
		"GET /api/cities",
		"POST /api/orders",
		"cache miss",
		"cache hit",
		"timeout reaching upstream",
		"db connection acquired",
		"worker started",
		"job completed",
	}
)

func (b *logsBody) nextBatch() []string {
	n := 1 + rand.Intn(2)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		level := logLevels[rand.Intn(len(logLevels))]
		verb := logVerbs[rand.Intn(len(logVerbs))]
		out = append(out, fmt.Sprintf("%s [%s] req-%04d %s", time.Now().Format("15:04:05.000"), level, b.seq, verb))
		b.seq++
	}
	return out
}

// ---- counter body ----------------------------------------------------------

type infoBody struct {
	t     theme.Theme
	count int
	body  pane.Pane
}

func newInfoBody(t theme.Theme) *infoBody {
	b := &infoBody{}
	b.SetTheme(t)
	return b
}

func (b *infoBody) Title() string         { return "Counter" }
func (b *infoBody) Init() tea.Cmd         { return nil }
func (b *infoBody) OnEnter(any) tea.Cmd   { return nil }
func (b *infoBody) IsCapturingKeys() bool { return false }

func (b *infoBody) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "+", "=":
			b.count++
			b.rebuild()
		case "-", "_":
			b.count--
			b.rebuild()
		}
	}
	return b, nil
}

func (b *infoBody) Layout() layout.Node { return layout.Sized(&b.body) }

func (b *infoBody) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "inc")),
		key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "dec")),
	}
}

func (b *infoBody) SetTheme(t theme.Theme) {
	b.t = t
	b.body = pane.New(t.Pane())
	b.body.SetTitle("counter")
	b.rebuild()
}

func (b *infoBody) rebuild() {
	b.body.SetContent(fmt.Sprintf(
		"count = %d\n\nThis tab keeps its own count across switches —\nflip to Cities or Logs and back; it persists.\nThe Logs tab keeps streaming the whole time.",
		b.count,
	))
}
