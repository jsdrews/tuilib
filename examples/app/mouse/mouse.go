// Package mouse demonstrates the click, double-click, wheel and drag
// affordances tuilib components expose when the app shell enables mouse
// input (app.Options.Mouse = app.MouseClick).
//
// Nothing here is mouse-specific plumbing: the screen holds three ordinary
// components in a focus.Group and routes messages the usual way. Every
// behaviour below falls out of components knowing the rect layout gave them.
//
// What to try:
//
//   - Click any pane to focus it — the border flips to its active colour.
//   - Click a row to move the cursor there; double-click to open it, the
//     mouse spelling of enter (rule 14).
//   - Click a sortable table header to sort by it; click again to flip.
//   - Click a ▸/▾ glyph in the tree to expand or collapse that node. One
//     click, because the glyph is drawn to say "this opens".
//   - Roll the wheel over any pane — including an unfocused one. The wheel
//     does what ↓/j does for that component and never steals focus.
//   - Drag a scrollbar thumb, or click anywhere on its track to jump.
//   - Click a breadcrumb crumb to jump back up the stack, and the "? help"
//     affordance in the statusbar to open the panel.
package mouse

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// New returns the mouse demo's root screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t     theme.Theme
	files list.Model
	rows  table.Model
	nodes tree.Model

	focus focus.Group
}

func (s *Screen) Title() string       { return "Mouse" }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }
func (s *Screen) Init() tea.Cmd       { return s.focus.Init() }

func (s *Screen) IsCapturingKeys() bool { return s.focus.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmds []tea.Cmd

	// The group consumes tab / shift-tab and grants the focus requests that
	// clicked components send back up.
	var gcmd tea.Cmd
	s.focus, gcmd = s.focus.Update(msg)
	cmds = append(cmds, gcmd)

	// Double-click results. Each component reports its own activation; the
	// screen decides what "open" means, exactly as it would for enter.
	switch m := msg.(type) {
	case list.ActivatedMsg:
		return s, app.Info("opened file: " + m.Item)
	case table.ActivatedMsg:
		return s, app.Info(fmt.Sprintf("opened row: %s (%s)", m.Cells[0], m.Cells[1]))
	}

	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
		return s, screen.Pop(nil)
	}

	// Mouse events go to every component so each can test the position
	// against its own rect; keys go to the focused one alone.
	if _, isMouse := msg.(mouse.Msg); isMouse {
		cmds = append(cmds, s.forwardAll(msg)...)
	} else {
		cmds = append(cmds, s.forwardFocused(msg))
	}
	return s, tea.Batch(cmds...)
}

func (s *Screen) forwardAll(msg tea.Msg) []tea.Cmd {
	var cmds []tea.Cmd
	var c tea.Cmd
	s.files, c = s.files.Update(msg)
	cmds = append(cmds, c)
	s.rows, c = s.rows.Update(msg)
	cmds = append(cmds, c)
	s.nodes, c = s.nodes.Update(msg)
	return append(cmds, c)
}

func (s *Screen) forwardFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.files):
		s.files, cmd = s.files.Update(msg)
	case s.focus.Is(&s.rows):
		s.rows, cmd = s.rows.Update(msg)
	case s.focus.Is(&s.nodes):
		s.nodes, cmd = s.nodes.Update(msg)
	}
	return cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.files)),
		layout.Flex(3, layout.VStack(
			layout.Flex(1, layout.Sized(&s.rows)),
			layout.Flex(1, layout.Sized(&s.nodes)),
		)),
	)
}

func (s *Screen) Help() []key.Binding {
	out := s.focus.Help()
	return append(out,
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		// Mouse affordances carry sentinel keys so they flow through
		// help.Compile without colliding (keyless bindings all dedup to
		// one) and never match a real KeyMsg.
		key.NewBinding(key.WithKeys("mouse:click"), key.WithHelp("click", "focus + select")),
		key.NewBinding(key.WithKeys("mouse:dblclick"), key.WithHelp("2×click", "open")),
		key.NewBinding(key.WithKeys("mouse:wheel"), key.WithHelp("wheel", "scroll under pointer")),
		key.NewBinding(key.WithKeys("mouse:header"), key.WithHelp("click hdr", "sort column")),
		key.NewBinding(key.WithKeys("mouse:glyph"), key.WithHelp("click ▸", "expand node")),
		key.NewBinding(key.WithKeys("mouse:drag"), key.WithHelp("drag bar", "scroll")),
	)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor := s.files.Cursor()
	lOpts := t.List()
	lOpts.Title = "files"
	lOpts.Filterable = true
	lOpts.Items = fileNames
	s.files = list.New(lOpts)
	s.files.SetCursor(cursor)

	tblCursor, sortCol, sortDesc := s.rows.Cursor(), s.rows.SortColumn(), s.rows.SortDescending()
	tOpts := t.Table()
	tOpts.Title = "deployments · click a header to sort"
	tOpts.Columns = []table.Column{
		{Title: "Name", Flex: 2, Sortable: true},
		{Title: "Region", Width: 10, Sortable: true},
		{Title: "Replicas", Width: 9, Align: alignRight, Sortable: true},
	}
	tOpts.Rows = deployments
	s.rows = table.New(tOpts)
	s.rows.SetCursor(tblCursor)
	s.rows.SetSort(sortCol, sortDesc)

	nodeCursor := s.nodes.Cursor()
	trOpts := t.Tree()
	trOpts.Title = "services · click a ▸ to expand"
	trOpts.Root = serviceTree
	trOpts.InitialDepth = 1
	s.nodes = tree.New(trOpts)
	s.nodes.SetCursor(nodeCursor)

	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.files, &s.rows, &s.nodes).WithKeys(focus.Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev pane")),
	})
	s.focus.SetIndex(at)
}
