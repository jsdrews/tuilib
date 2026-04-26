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

	focus int // 0=list, 1=log, 2=tree
}

const focusCount = 3

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
func (s *Screen) IsCapturingKeys() bool {
	switch s.focus {
	case 0:
		return s.list.Filtering()
	case 1:
		return s.log.Searching()
	case 2:
		return s.tree.Searching()
	}
	return false
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
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
		if !s.IsCapturingKeys() {
			switch m.String() {
			case "tab":
				s.focus = (s.focus + 1) % focusCount
				s.applyFocus()
				return s, nil
			case "shift+tab":
				s.focus = (s.focus - 1 + focusCount) % focusCount
				s.applyFocus()
				return s, nil
			case "r":
				return s, s.startFetches()
			}
		}
		// Key messages only reach the focused component — otherwise "/"
		// would open three search inputs at once and h/l would scroll
		// every pane in sync.
		return s, s.routeKey(msg)
	}

	// Non-key messages (spinner.TickMsg, fetched results, resize) fan
	// out to all three so every spinner keeps animating.
	var cmds []tea.Cmd
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	cmds = append(cmds, cmd)
	s.log, cmd = s.log.Update(msg)
	cmds = append(cmds, cmd)
	s.tree, cmd = s.tree.Update(msg)
	cmds = append(cmds, cmd)
	return s, tea.Batch(cmds...)
}

func (s *Screen) routeKey(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch s.focus {
	case 0:
		s.list, cmd = s.list.Update(msg)
	case 1:
		s.log, cmd = s.log.Update(msg)
	case 2:
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

func (s *Screen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev pane")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refetch")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	switch s.focus {
	case 0:
		return append(base, s.list.Help()...)
	case 1:
		return append(base, s.log.Help()...)
	case 2:
		return append(base, s.tree.Help()...)
	}
	return base
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
	s.list.SetFocused(s.focus == 0)
	s.log.SetFocused(s.focus == 1)
	s.tree.SetFocused(s.focus == 2)
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
