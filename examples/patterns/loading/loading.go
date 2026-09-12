// Package loading demonstrates pkg/pane's loading state across a list,
// a logview, and a tree. Each component starts in SetLoading(true);
// after a staggered delay, simulated "fetched" data arrives and the
// spinners are dismissed via SetLoading(false). Press r to refetch.
//
// The screen also illustrates focus cycling across multiple interactive
// components: tab/shift-tab moves focus between list, logview, and tree.
// Key messages are routed only to the focused component, so "/" opens
// one search at a time and h/l scrolls only the focused pane. Spinner
// ticks and fetch results still fan out to all three so every spinner
// keeps animating regardless of focus.
package loading

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	lv "github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
	tw "github.com/jsdrews/tuilib/pkg/tree"
)

// New returns the loading-state demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t    theme.Theme
	list list.Model
	log  lv.Model
	tree tw.Model

	focus focus.Group
}

// fetchedMsg variants — one per component — let us deliver each result
// independently so the spinners disappear on different schedules.
type listFetchedMsg struct{ items []string }
type logFetchedMsg struct{ lines []string }
type treeFetchedMsg struct{ root tw.Node }

func fetchList() tea.Cmd {
	return tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg {
		return listFetchedMsg{items: cities}
	})
}

func fetchLog() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		out := make([]string, 0, 12)
		for i := 1; i <= 12; i++ {
			out = append(out, fmt.Sprintf(
				"%s  INFO  worker  job#%-3d  result fetched", time.Now().Format("15:04:05"), i))
		}
		return logFetchedMsg{lines: out}
	})
}

func fetchTree() tea.Cmd {
	return tea.Tick(2200*time.Millisecond, func(time.Time) tea.Msg {
		return treeFetchedMsg{root: sampleTree()}
	})
}

func (s *Screen) Title() string       { return "Loading" }
func (s *Screen) Init() tea.Cmd       { return tea.Batch(textinput.Blink, s.startFetches()) }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

// IsCapturingKeys claims keys whenever the focused component is in a
// text-input state — that's where global keys like q/t would otherwise
// steal printables from the search box.
func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// The group needs *every* message, not just tab: it also grants the
	// focus requests a clicked component sends. Feeding it only tab keys
	// drops those, so a click lights a pane while the keyboard stays put.
	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)

	switch m := msg.(type) {
	case listFetchedMsg:
		s.list.SetItems(m.items)
		s.list.SetLoading(false)
		return s, nil
	case logFetchedMsg:
		s.log.AppendLines(m.lines)
		s.log.SetLoading(false)
		return s, nil
	case treeFetchedMsg:
		s.tree.SetRoot(m.root)
		s.tree.SetLoading(false)
		return s, nil
	case tea.KeyMsg:
		if !s.IsCapturingKeys() && m.String() == "r" {
			return s, s.startFetches()
		}
		// Key messages only reach the focused component — otherwise "/"
		// would open three search inputs at once and h/l would scroll
		// every pane in sync. The group handled tab above.
		return s, tea.Batch(gcmd, s.routeKey(msg))
	}

	// Non-key messages (mouse, spinner.TickMsg, fetched results, resize)
	// fan out to all three, so every spinner keeps animating and every
	// component gets a chance to claim a click.
	return s, tea.Batch(gcmd, s.forwardAll(msg))
}

// forwardAll hands a message to every component. Each tests the
// position against its own rect and only the one it landed in acts, so
// fanning out is safe — and necessary: a component that never receives the
// click cannot claim focus or hand input back from its filter.
func (s *Screen) forwardAll(msg tea.Msg) tea.Cmd {
	var a, b, c tea.Cmd
	s.list, a = s.list.Update(msg)
	s.log, b = s.log.Update(msg)
	s.tree, c = s.tree.Update(msg)
	return tea.Batch(a, b, c)
}

func (s *Screen) routeKey(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.list):
		s.list, cmd = s.list.Update(msg)
	case s.focus.Is(&s.log):
		s.log, cmd = s.log.Update(msg)
	case s.focus.Is(&s.tree):
		s.tree, cmd = s.tree.Update(msg)
	}
	return cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(1, layout.Sized(&s.list)),
		layout.Flex(2, layout.VStack(
			layout.Flex(1, layout.Sized(&s.log)),
			layout.Flex(1, layout.Sized(&s.tree)),
		)),
	)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections forwards to the Group, which names each pane and its groups;
// tab/shift-tab come from the Group too, so this screen lists only its own.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.focus, help.Group("Loading",
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	// list
	cursor, value := s.list.Cursor(), s.list.Value()
	lopts := t.List()
	lopts.Title = "cities"
	lopts.Filterable = true
	lopts.Filter.Placeholder = "filter cities…"
	lopts.LoadingLabel = "loading cities…"
	s.list = list.New(lopts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)

	// logview
	q := s.log.Query()
	gopts := t.Logview()
	gopts.Title = "events"
	gopts.Searchable = true
	gopts.Filter.Placeholder = "search…"
	gopts.LoadingLabel = "fetching events…"
	s.log = lv.New(gopts)
	if q != "" {
		s.log.SetQuery(q)
	}

	// tree
	tcursor := s.tree.Cursor()
	topts := t.Tree()
	topts.Title = "modules"
	topts.Searchable = true
	topts.InitialDepth = 1
	topts.Filter.Placeholder = "search…"
	topts.LoadingLabel = "loading modules…"
	s.tree = tw.New(topts)
	s.tree.SetCursor(tcursor)

	s.applyFocus()
}

func (s *Screen) applyFocus() {
	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.list, &s.log, &s.tree)
	s.focus.SetIndex(at)
}

func (s *Screen) startFetches() tea.Cmd {
	cmds := []tea.Cmd{
		s.list.SetLoading(true),
		s.log.SetLoading(true),
		s.tree.SetLoading(true),
		fetchList(),
		fetchLog(),
		fetchTree(),
	}
	// Reset stale data so the spinner replaces the previous result on refetch.
	s.list.SetItems(nil)
	s.log.Clear()
	s.tree.SetRoot(nil)
	return tea.Batch(cmds...)
}

// ---- synthetic data ------------------------------------------------------

var cities = []string{
	"New York", "San Francisco", "Toronto", "Vancouver", "Chicago",
	"London", "Berlin", "Paris", "Madrid", "Amsterdam", "Lisbon", "Prague",
	"Tokyo", "Singapore", "Seoul", "Mumbai", "Bangkok",
	"Sydney", "Melbourne", "Auckland",
}

type node struct {
	label string
	kids  []*node
}

func (n *node) Label() string { return n.label }
func (n *node) Children() []tw.Node {
	out := make([]tw.Node, len(n.kids))
	for i, k := range n.kids {
		out[i] = k
	}
	return out
}

func sampleTree() tw.Node {
	leaf := func(s string) *node { return &node{label: s} }
	dir := func(s string, kids ...*node) *node { return &node{label: s, kids: kids} }
	return dir("workspace",
		dir("api",
			leaf("server.go"),
			leaf("router.go"),
			leaf("middleware.go"),
		),
		dir("ui",
			dir("components",
				leaf("button.tsx"),
				leaf("modal.tsx"),
			),
			leaf("App.tsx"),
		),
		dir("scripts",
			leaf("deploy.sh"),
			leaf("build.sh"),
		),
	)
}
