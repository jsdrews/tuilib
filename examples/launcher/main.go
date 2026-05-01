// Launcher: the single entry point for tuilib examples. Shows a menu of
// available demos; enter pushes the selected one onto the app stack, esc
// pops back.
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"

	appfocus "github.com/jsdrews/tuilib/examples/app/focus"
	appgate "github.com/jsdrews/tuilib/examples/app/gate"
	applayouts "github.com/jsdrews/tuilib/examples/app/layouts"
	appstack "github.com/jsdrews/tuilib/examples/app/stack"
	apptabs "github.com/jsdrews/tuilib/examples/app/tabs"
	datadetail "github.com/jsdrews/tuilib/examples/data/detail"
	datadrilldown "github.com/jsdrews/tuilib/examples/data/drilldown"
	dataform "github.com/jsdrews/tuilib/examples/data/form"
	dataloading "github.com/jsdrews/tuilib/examples/data/loading"
	datalist "github.com/jsdrews/tuilib/examples/data/list"
	datalogview "github.com/jsdrews/tuilib/examples/data/logview"
	datarunlog "github.com/jsdrews/tuilib/examples/data/runlog"
	datarunner "github.com/jsdrews/tuilib/examples/data/runner"
	datatable "github.com/jsdrews/tuilib/examples/data/table"
	datatree "github.com/jsdrews/tuilib/examples/data/tree"
	paneshowcase "github.com/jsdrews/tuilib/examples/pane/showcase"
	"github.com/jsdrews/tuilib/examples/themecheck"
)

type entry struct {
	name string
	blurb string
	build func(theme.Theme) screen.Screen
}

var entries = []entry{
	{"Panes — border + title showcase", "Four panes demonstrating border styles, title positions, and slot-bracket variants.", paneshowcase.New},
	{"List — filterable cities", "A filterable list.Model as a single-screen app.", datalist.New},
	{"Logview — streaming with search", "A synthetic log stream with /-search, n/N to jump matches, g/G top/bottom, and pause/follow.", datalogview.New},
	{"Table — filterable bubbles/table", "bubbles/table composed with filter.Model and pane, filtered inline. Status column uses ansi.CellColor so the selected-row bg stays intact across colored cells.", datatable.New},
	{"Form — text + select + confirm", "A form.Model with Text, Select, and Confirm fields; each field is its own bordered component, submit replaces with a result pane.", dataform.New},
	{"Loading — spinners while fetching", "List, logview, and tree all start in SetLoading(true); staggered tea.Tick delays simulate fetches that resolve at different times. Tab cycles focus so / and h/l only affect one pane. Press r to refetch.", dataloading.New},
	{"Detail — selection triggers async fetch", "Master-detail. The cities list itself loads on Init via tea.Tick; once populated, pressing enter on a city fires another async fetch into the right pane. Stale results are dropped via reqID tagging so hammering enter never displays an out-of-order body. Press r to refetch.", datadetail.New},
	{"Drilldown — three-level stack with focus", "Cities list + detail list with focus cycling; enter \"opens the focused selection\" on either pane — left-enter loads the detail and shifts focus right, right-enter pushes a child screen describing the attribute. Esc on the child screen pops back with the parent's state intact.", datadrilldown.New},
	{"Runner — interactive subprocess", "Pick a command, hand the terminal to it, return on exit. Demonstrates pkg/runner with $EDITOR, less, man, htop. Last entry uses RunWithNotice to print \"connecting…\" during the handoff.", datarunner.New},
	{"Runlog — stream stdout into logview", "Pick a command on the left; its stdout/stderr stream into a logview on the right. Tab cycles focus, x kills the running process.", datarunlog.New},
	{"Tree — searchable expand/collapse", "A synthetic project tree with cursor, expand/collapse (←→/space), search (/), and filter mode (\\) that hides non-matching subtrees. Leaves carry colored status icons (lipgloss-rendered) so the row highlight stays intact across ANSI segments.", datatree.New},
	{"Themes — live palette picker", "Cursor re-skins the whole app; enter shows a theme's field palette.", themecheck.New},
	{"Layouts — five layout.Node trees", "One screen per layout primitive: HStack+Fixed/Flex, nested stacks, ZStack modal, …", applayouts.New},
	{"Stack — data flow between screens", "Parent→child via constructor, child→parent via Pop(result) + OnEnter.", appstack.New},
	{"Focus — tab/shift-tab between components", "A screen with input + list + toggle; tab cycles focus, only the active component takes keys.", appfocus.New},
	{"Gate — login form on first entry", "A root screen that pushes a login form on top of itself via OnEnter; submit pops with creds, L re-pushes for logout. Form is on the stack only while interacting.", appgate.New},
	{"Tabs — three sub-screens behind one strip", "Cities (filterable list) + Logs (streaming logview) + Counter, switched via shift+left/right or 1/2/3. tab/shift+tab is left alone. Each body keeps its own state across switches; logs keep streaming while you're on another tab.", apptabs.New},
}

type rootScreen struct {
	t    theme.Theme
	menu list.Model
	info pane.Pane
}

func newRoot() *rootScreen {
	s := &rootScreen{}
	s.SetTheme(themecheck.Themes()[1]) // start on Dark (index 0 is Terminal())
	return s
}

func (s *rootScreen) Title() string          { return "Examples" }
func (s *rootScreen) Init() tea.Cmd          { return textinput.Blink }
func (s *rootScreen) OnEnter(any) tea.Cmd    { return nil }
func (s *rootScreen) IsCapturingKeys() bool  { return s.menu.Filtering() }

func (s *rootScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	prev := s.menu.Cursor()
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)

	if s.menu.Cursor() != prev {
		s.rebuildInfo()
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.menu.Filtering() && k.String() == "enter" {
		idx := s.menu.Cursor()
		if idx >= 0 && idx < len(entries) {
			return s, tea.Batch(cmd, screen.Push(entries[idx].build(s.t)))
		}
	}
	return s, cmd
}

func (s *rootScreen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.menu)),
		layout.Flex(3, layout.Sized(&s.info)),
	)
}

func (s *rootScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (s *rootScreen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.menu.Cursor(), s.menu.Value()
	opts := t.List()
	opts.Title = "examples"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter…"
	opts.Items = entryNames()
	s.menu = list.New(opts)
	if value != "" {
		s.menu.SetValue(value)
	}
	s.menu.SetCursor(cursor)

	s.rebuildInfo()
}

func (s *rootScreen) rebuildInfo() {
	s.info = pane.New(s.t.Pane())
	s.info.SetTitle("about")
	idx := s.menu.Cursor()
	if idx < 0 || idx >= len(entries) {
		s.info.SetContent("Pick an example on the left and press enter.")
		return
	}
	e := entries[idx]
	s.info.SetContent(e.name + "\n\n" + e.blurb)
}

func entryNames() []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.name
	}
	return out
}

func main() {
	m := app.New(app.Options{
		Root:     newRoot(),
		Themes:   theme.Resolve(themecheck.Themes(), "TUILIB_THEME"),
		Version:  "examples",
		ThemeKey: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
