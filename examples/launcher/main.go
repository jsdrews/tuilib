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
	appreplace "github.com/jsdrews/tuilib/examples/app/replace"
	appstack "github.com/jsdrews/tuilib/examples/app/stack"
	appstatus "github.com/jsdrews/tuilib/examples/app/status"
	apptabs "github.com/jsdrews/tuilib/examples/app/tabs"
	dataalert "github.com/jsdrews/tuilib/examples/data/alert"
	dataconfirm "github.com/jsdrews/tuilib/examples/data/confirm"
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
	{"Table — filterable + sortable cities", "table.Model with sticky header, per-column widths, h-scroll for wide tables, [/] to step sort column + s to toggle direction (City + Region sort lexically; Population sorts numerically via a custom Less that parses \"8.3M\"). Status column uses ansi.CellColor so the selected-row bg passes through colored cells.", datatable.New},
	{"Form — text + select + confirm", "A form.Model with Text, Select, and Confirm fields; each field is its own bordered component, submit replaces with a result pane.", dataform.New},
	{"Loading — spinners while fetching", "List, logview, and tree all start in SetLoading(true); staggered tea.Tick delays simulate fetches that resolve at different times. Tab cycles focus so / and h/l only affect one pane. Press r to refetch.", dataloading.New},
	{"Drilldown — master-detail + push, with async fetches", "Cities list (loads on Init via tea.Tick) + detail list with focus cycling. Enter on either pane \"opens the focused selection\" — left-enter loads the detail (reqID tags drop stale results so hammering enter never races) and shifts focus right; right-enter pushes a child screen describing the attribute. Esc on the child pops back with parent state intact.", datadrilldown.New},
	{"Runner — interactive subprocess", "Pick a command, hand the terminal to it, return on exit. Demonstrates pkg/runner with $EDITOR, less, man, htop. Last entry uses RunWithNotice to print \"connecting…\" during the handoff.", datarunner.New},
	{"Runlog — stream stdout into logview", "Pick a command on the left; its stdout/stderr stream into a logview on the right. Tab cycles focus, x kills the running process.", datarunlog.New},
	{"Tree — searchable expand/collapse", "A synthetic project tree with cursor, expand/collapse (←→/space), search (/), and filter mode (\\) that hides non-matching subtrees. Leaves carry colored status icons (lipgloss-rendered) so the row highlight stays intact across ANSI segments.", datatree.New},
	{"Themes — live palette picker", "Cursor re-skins the whole app; enter shows a theme's field palette.", themecheck.New},
	{"Layouts — five layout.Node trees", "One screen per layout primitive: HStack+Fixed/Flex, nested stacks, ZStack modal, …", applayouts.New},
	{"Stack — data flow between screens", "Parent→child via constructor, child→parent via Pop(result) + OnEnter.", appstack.New},
	{"Focus — tab/shift-tab between components", "A screen with input + list + toggle; tab cycles focus, only the active component takes keys.", appfocus.New},
	{"Gate — login form on first entry", "A root screen that pushes a login form on top of itself via OnEnter; submit pops with creds, L re-pushes for logout. Form is on the stack only while interacting.", appgate.New},
	{"Tabs — three sub-screens behind one strip", "Cities (filterable list) + Logs (streaming logview) + Counter, switched via shift+left/right or 1/2/3. tab/shift+tab is left alone. Each body keeps its own state across switches; logs keep streaming while you're on another tab.", apptabs.New},
	{"Status — info/error messages from a screen", "Pick an action; the screen returns app.Info / app.Error / app.ClearStatus and the shell paints the statusbar's center slot. Auto-clears on any keypress.", appstatus.New},
	{"Confirm — modal yes/no dialog", "Press d on a file to bring up a confirm modal via pkg/confirm; ConfirmedMsg/CancelledMsg flow back as tea.Msg, the parent dismisses and reports the outcome via app.Info.", dataconfirm.New},
	{"Alert — modal acknowledgement dialog", "A list of mock operations; some succeed (statusbar info), some fail with an error-tinted modal alert via pkg/alert. DismissedMsg flows back as tea.Msg. Contrasts the lightweight statusbar pattern with the modal \"stop and acknowledge\" pattern.", dataalert.New},
	{"Replace — atomic top-of-stack swap", "Press r on the city list (filter set, cursor moved) to swap in a fresh instance — depth stays at 1, no flicker. Push a city detail, bump the visit counter, then r to reset the counter without re-firing the parent's OnEnter.", appreplace.New},
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
	prevIdx, prevOK := s.menu.SelectedIndex()
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)

	if curIdx, curOK := s.menu.SelectedIndex(); curIdx != prevIdx || curOK != prevOK {
		s.rebuildInfo()
	}

	if k, ok := msg.(tea.KeyMsg); ok && !s.menu.Filtering() && k.String() == "enter" {
		if idx, ok := s.menu.SelectedIndex(); ok && idx >= 0 && idx < len(entries) {
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
	idx, ok := s.menu.SelectedIndex()
	if !ok || idx < 0 || idx >= len(entries) {
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
		Root:        newRoot(),
		Themes:      themecheck.Themes(),
		ThemeEnvVar: "TUILIB_THEME",
		Version:     "examples",
		ThemeKey:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
