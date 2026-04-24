// Demo: one app shell, several screens — each with a different layout tree.
// Proves that pkg/layout can describe arbitrary compositions without the
// caller doing any "m.h-2" math.
//
// Interaction is menu-driven: no per-screen letter shortcuts. Arrow keys
// move the cursor, enter opens the selection, esc pops.
//
// Global keys:
//
//	t     cycle theme
//	q     quit (at root)
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
)

var themes = []theme.Theme{
	theme.Nord(), theme.Dark(), theme.Dracula(), theme.Gruvbox(),
	theme.TokyoNight(), theme.Light(),
}

// ---- Home (root) -----------------------------------------------------------
//
// Layout: HStack(Flex(1, info), Flex(1, menu list)). The menu is a plain
// list — arrows move, enter opens the selected demo.

type homeScreen struct {
	t    theme.Theme
	info pane.Pane
	menu list.Model
}

const (
	menuSidebar   = "Sidebar     — HStack(Fixed + Flex)"
	menuDashboard = "Dashboard   — nested VStack of HStacks"
	menuFilter    = "Filter+list — list.Model inside a single Flex"
	menuModal     = "Modal       — ZStack(base + centered overlay)"
	menuColumns   = "Columns     — HStack of five equal Flex panes"
)

var menuItems = []string{menuSidebar, menuDashboard, menuFilter, menuModal, menuColumns}

func newHomeScreen() *homeScreen {
	s := &homeScreen{}
	s.SetTheme(themes[0])
	return s
}

func (s *homeScreen) Title() string         { return "Home" }
func (s *homeScreen) Init() tea.Cmd         { return nil }
func (s *homeScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *homeScreen) IsCapturingKeys() bool { return s.menu.Filtering() }

func (s *homeScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !s.menu.Filtering() && k.String() == "enter" {
		if item, ok := s.menu.Selected(); ok {
			return s, screen.Push(screenFor(item, s.t))
		}
	}
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)
	return s, cmd
}

func screenFor(item string, t theme.Theme) screen.Screen {
	switch item {
	case menuSidebar:
		return newSidebarScreen(t)
	case menuDashboard:
		return newDashboardScreen(t)
	case menuFilter:
		return newFilterableScreen(t)
	case menuModal:
		return newModalScreen(t)
	case menuColumns:
		return newColumnsScreen(t)
	}
	return nil
}

func (s *homeScreen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(1, layout.Sized(&s.info)),
		layout.Flex(1, layout.Sized(&s.menu)),
	)
}

func (s *homeScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (s *homeScreen) SetTheme(t theme.Theme) {
	s.t = t
	s.info = pane.New(t.Pane())
	s.info.SetTitle("layouts demo")
	s.info.SetContent(
		"Every demo body is a different layout.Node tree.\n\n" +
			"No window-math lives inside the screens — they\n" +
			"declare shape, the stacker divides the pixels.\n\n" +
			"Pick a demo on the right and press enter.",
	)

	cursor := s.menu.Cursor()
	opts := t.List()
	opts.Title = "demos"
	opts.Items = menuItems
	s.menu = list.New(opts)
	s.menu.SetCursor(cursor)
}

// ---- Sidebar: HStack with fixed + flex -------------------------------------

type sidebarScreen struct {
	t       theme.Theme
	sidebar pane.Pane
	body    pane.Pane
}

func newSidebarScreen(t theme.Theme) *sidebarScreen {
	s := &sidebarScreen{}
	s.SetTheme(t)
	return s
}

func (s *sidebarScreen) Title() string                           { return "Sidebar" }
func (s *sidebarScreen) Init() tea.Cmd                           { return nil }
func (s *sidebarScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *sidebarScreen) OnEnter(any) tea.Cmd                     { return nil }
func (s *sidebarScreen) IsCapturingKeys() bool                   { return false }

func (s *sidebarScreen) Layout() layout.Node {
	return layout.HStack(
		layout.Fixed(24, layout.Sized(&s.sidebar)),
		layout.Flex(1, layout.Sized(&s.body)),
	)
}

func (s *sidebarScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *sidebarScreen) SetTheme(t theme.Theme) {
	s.t = t
	s.sidebar = pane.New(t.Pane())
	s.sidebar.SetTitle("folders")
	s.sidebar.SetContent("• Inbox  (42)\n• Drafts (3)\n• Sent\n• Trash\n• Archive\n• Spam")
	s.body = pane.New(t.Pane())
	s.body.SetTitle("body")
	s.body.SetContent(
		"HStack splits width between a Fixed(24) sidebar and\n" +
			"a Flex(1) body. Resize the terminal — sidebar stays\n" +
			"24 cols, body absorbs the rest.\n\n" +
			"layout.HStack(\n" +
			"  layout.Fixed(24, layout.Sized(&s.sidebar)),\n" +
			"  layout.Flex(1,   layout.Sized(&s.body)),\n" +
			")\n",
	)
}

// ---- Dashboard: nested VStack of HStacks -----------------------------------

type dashboardScreen struct {
	t                  theme.Theme
	tl, tr, bl, bm, br pane.Pane
}

func newDashboardScreen(t theme.Theme) *dashboardScreen {
	s := &dashboardScreen{}
	s.SetTheme(t)
	return s
}

func (s *dashboardScreen) Title() string                           { return "Dashboard" }
func (s *dashboardScreen) Init() tea.Cmd                           { return nil }
func (s *dashboardScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *dashboardScreen) OnEnter(any) tea.Cmd                     { return nil }
func (s *dashboardScreen) IsCapturingKeys() bool                   { return false }

func (s *dashboardScreen) Layout() layout.Node {
	return layout.VStack(
		layout.Flex(1, layout.HStack(
			layout.Flex(1, layout.Sized(&s.tl)),
			layout.Flex(2, layout.Sized(&s.tr)),
		)),
		layout.Flex(1, layout.HStack(
			layout.Flex(1, layout.Sized(&s.bl)),
			layout.Flex(1, layout.Sized(&s.bm)),
			layout.Flex(1, layout.Sized(&s.br)),
		)),
	)
}

func (s *dashboardScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *dashboardScreen) SetTheme(t theme.Theme) {
	s.t = t
	mk := func(title, body string) pane.Pane {
		p := pane.New(t.Pane())
		p.SetTitle(title)
		p.SetContent(body)
		return p
	}
	s.tl = mk("cpu", "user  21%\nsys    4%\nidle  75%")
	s.tr = mk("timeline", "12:00  build\n12:03  deploy\n12:05  smoke-test\n12:07  rolled out\n12:12  healthy")
	s.bl = mk("db", "conns 12/32\nqps   47\np95   18ms")
	s.bm = mk("cache", "hit 92%\nmem 412M\nkeys 18k")
	s.br = mk("queue", "depth 3\nrate 9/s")
}

// ---- Filterable: list inside a single Flex ---------------------------------

type filterableScreen struct {
	t    theme.Theme
	list list.Model
}

func newFilterableScreen(t theme.Theme) *filterableScreen {
	s := &filterableScreen{}
	s.SetTheme(t)
	return s
}

func (s *filterableScreen) Title() string          { return "Filter" }
func (s *filterableScreen) Init() tea.Cmd          { return textinput.Blink }
func (s *filterableScreen) OnEnter(any) tea.Cmd    { return nil }
func (s *filterableScreen) IsCapturingKeys() bool  { return s.list.Filtering() }

func (s *filterableScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *filterableScreen) Layout() layout.Node { return layout.Sized(&s.list) }

func (s *filterableScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *filterableScreen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, value := s.list.Cursor(), s.list.Value()
	opts := t.List()
	opts.Title = "Fruits"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter…"
	opts.Items = []string{
		"apple", "apricot", "banana", "blueberry", "cherry", "cranberry",
		"date", "elderberry", "fig", "grape", "grapefruit", "guava",
		"kiwi", "lemon", "lime", "mango", "melon", "nectarine",
		"orange", "papaya", "peach", "pear", "pineapple", "plum",
		"pomegranate", "quince", "raspberry", "strawberry", "tangerine",
		"watermelon",
	}
	s.list = list.New(opts)
	if value != "" {
		s.list.SetValue(value)
	}
	s.list.SetCursor(cursor)
}

// ---- Modal: actions list + ZStack(base, overlay) ---------------------------
//
// The menu on the right triggers a modal. While the modal is up, the screen
// captures keys — enter confirms and dismisses, esc cancels and dismisses.
// That way neither the app-wide quit nor esc-pop interferes with the modal.

type modalScreen struct {
	t       theme.Theme
	base    pane.Pane
	modal   pane.Pane
	actions list.Model

	modalUp bool
	message string // what the base shows after confirm/cancel
}

func newModalScreen(t theme.Theme) *modalScreen {
	s := &modalScreen{}
	s.SetTheme(t)
	return s
}

func (s *modalScreen) Title() string         { return "Modal" }
func (s *modalScreen) Init() tea.Cmd         { return nil }
func (s *modalScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *modalScreen) IsCapturingKeys() bool { return s.modalUp || s.actions.Filtering() }

func (s *modalScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if s.modalUp && isKey {
		switch k.String() {
		case "enter":
			s.modalUp = false
			s.message = "✓ confirmed"
			s.rebuildBase()
			return s, nil
		case "esc":
			s.modalUp = false
			s.message = "✗ cancelled"
			s.rebuildBase()
			return s, nil
		}
		return s, nil // swallow other keys while modal up
	}
	if isKey && !s.actions.Filtering() && k.String() == "enter" {
		if _, ok := s.actions.Selected(); ok {
			s.modalUp = true
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.actions, cmd = s.actions.Update(msg)
	return s, cmd
}

func (s *modalScreen) Layout() layout.Node {
	body := layout.HStack(
		layout.Flex(1, layout.Sized(&s.base)),
		layout.Flex(1, layout.Sized(&s.actions)),
	)
	if !s.modalUp {
		return body
	}
	return layout.ZStack(body, layout.Center(48, 7, layout.Sized(&s.modal)))
}

func (s *modalScreen) Help() []key.Binding {
	if s.modalUp {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "confirm")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *modalScreen) SetTheme(t theme.Theme) {
	s.t = t
	s.rebuildBase()

	s.modal = pane.New(t.Pane())
	s.modal.SetTitle("confirm")
	s.modal.SetContent(
		"Delete file.txt permanently?\n\n" +
			"enter   yes\n" +
			"esc     no",
	)

	cursor := s.actions.Cursor()
	opts := t.List()
	opts.Title = "actions"
	opts.Items = []string{"Delete file.txt"}
	s.actions = list.New(opts)
	s.actions.SetCursor(cursor)
}

func (s *modalScreen) rebuildBase() {
	s.base = pane.New(s.t.Pane())
	s.base.SetTitle("base")
	body := "Trigger an action from the menu on the right.\n" +
		"A confirmation modal appears on top.\n\n" +
		"The modal is a ZStack layer above the rest of\n" +
		"the body — nothing else moves."
	if s.message != "" {
		body = "Last action: " + s.message + "\n\n" + body
	}
	s.base.SetContent(body)
}

// ---- Columns: HStack of five equal Flex panes ------------------------------

type columnsScreen struct {
	t  theme.Theme
	ps [5]pane.Pane
}

func newColumnsScreen(t theme.Theme) *columnsScreen {
	s := &columnsScreen{}
	s.SetTheme(t)
	return s
}

func (s *columnsScreen) Title() string                           { return "Columns" }
func (s *columnsScreen) Init() tea.Cmd                           { return nil }
func (s *columnsScreen) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *columnsScreen) OnEnter(any) tea.Cmd                     { return nil }
func (s *columnsScreen) IsCapturingKeys() bool                   { return false }

func (s *columnsScreen) Layout() layout.Node {
	items := make([]layout.Item, 5)
	for i := range s.ps {
		items[i] = layout.Flex(1, layout.Sized(&s.ps[i]))
	}
	return layout.HStack(items...)
}

func (s *columnsScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *columnsScreen) SetTheme(t theme.Theme) {
	s.t = t
	titles := []string{"one", "two", "three", "four", "five"}
	for i := range s.ps {
		p := pane.New(t.Pane())
		p.SetTitle(titles[i])
		p.SetContent(fmt.Sprintf("col %d\n\nFlex(1)\neach", i+1))
		s.ps[i] = p
	}
}

// ---- main -----------------------------------------------------------------

func main() {
	m := app.New(app.Options{
		Root:     newHomeScreen(),
		Themes:   themes,
		Version:  "layouts",
		ThemeKey: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
