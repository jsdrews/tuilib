// Package workbench is the local-data showcase: every row component on one
// screen, cooperating, so a change that breaks an older feature shows up
// where you can see it.
//
//	┌ deployments ───────────────────┐┌ cluster ──────────┐
//	│ filterable, sortable, markable ││ searchable,       │
//	│ table                          ││ markable tree     │
//	└────────────────────────────────┘└───────────────────┘
//	┌ pods ─────────┐┌ detail ───────────┐┌ operations ───┐
//	│ markable list ││ filterable        ││ grouped menu  │
//	│               ││ inspector         ││               │
//	└───────────────┘└───────────────────┘└───────────────┘
//
// What it exercises:
//
//   - focus.Group over five panes: tab / shift+tab cycle, a click focuses,
//     keys go to the focused pane only and the mouse to all of them (rule 6).
//     The operations menu sits in the Group, so it leaves tab to it and jumps
//     its sections on [ / ].
//   - Master-detail: the pods list follows the table's cursor, and the detail
//     inspector follows whichever of table, tree or pods last had focus.
//     Enter (or a double click) on the table moves focus to its pods; on a
//     pod, to the detail (rule 16).
//   - Marking (rule 32): x, X, A, D and the ✓ gutter in the table, the tree
//     and the pods list. Counts sit on each border, so a mark filtered or
//     folded out of sight is still visible as a number.
//   - The action menu ("a" or right-click) offers verbs for the focused
//     pane's selection, filed read / write / danger. Describe is not Multi,
//     so a second mark dims it; Delete confirms first and is guarded. A marked
//     tree branch cascades to its pods for Restart but not for Describe.
//   - Feedback by weight: the operations menu runs statusbar notices, the
//     confirm modal and the alert modal.
//   - Mouse: header clicks sort, ▸/▾ glyphs toggle, the wheel scrolls the
//     pane under it without taking focus, scrollbars drag.
//   - SetTheme carries every pane's cursor, filter, marks and expansion across
//     a palette or surface swap, including a modal that is up (rule 4).
package workbench

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	talert "github.com/jsdrews/tuilib/pkg/alert"
	tconfirm "github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// New returns the workbench screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{deps: seed()}
	s.root = clusterTree(s.deps)
	s.SetTheme(t)
	return s
}

type Screen struct {
	t    theme.Theme
	deps []deployment
	root *node

	table  table.Model
	tree   tree.Model
	pods   list.Model
	detail inspector.Model
	ops    menu.Model
	focus  focus.Group

	// source is the pane the detail follows; podsOf is the deployment the
	// pods list shows, and shown what the detail shows.
	source      focus.Focusable
	podsOf      string
	shown       string
	detailTitle string

	confirm   tconfirm.Model
	confirmUp bool
	alert     talert.Model
	alertUp   bool
	pending   int
}

func (s *Screen) Title() string       { return "Workbench" }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }
func (s *Screen) Init() tea.Cmd       { return tea.Batch(textinput.Blink, s.focus.Init()) }

func (s *Screen) IsCapturingKeys() bool {
	return s.confirmUp || s.alertUp || s.focus.IsCapturingKeys()
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// Modal results first, so a dismissed modal never sees the key that
	// dismissed it a second time.
	switch msg.(type) {
	case tconfirm.ConfirmedMsg:
		s.confirmUp = false
		return s, s.finish()
	case tconfirm.CancelledMsg:
		s.confirmUp = false
		return s, nil
	case talert.DismissedMsg:
		s.alertUp = false
		return s, nil
	}
	// A modal that is up owns input.
	if s.confirmUp {
		var cmd tea.Cmd
		s.confirm, cmd = s.confirm.Update(msg)
		return s, cmd
	}
	if s.alertUp {
		var cmd tea.Cmd
		s.alert, cmd = s.alert.Update(msg)
		return s, cmd
	}

	var cmds []tea.Cmd
	var c tea.Cmd
	s.focus, c = s.focus.Update(msg)
	cmds = append(cmds, c)

	// Enter and a double click are one verb; each pane reports it and this
	// screen says what opening means there. An enter key counts only for
	// the pane it was typed into; a double click names its pane by token.
	_, isKey := msg.(tea.KeyMsg)
	opens := func(f focus.Focusable, activated func(tea.Msg) bool) bool {
		return (!isKey || s.focus.Is(f)) && activated(msg)
	}
	switch {
	case opens(&s.table, s.table.IsActivate):
		return s, tea.Batch(append(cmds, s.focus.SetIndex(2))...)
	case opens(&s.pods, s.pods.IsActivate):
		return s, tea.Batch(append(cmds, s.focus.SetIndex(3))...)
	}
	if it, ok := s.ops.Chosen(msg); ok {
		return s, tea.Batch(append(cmds, s.start(it.Data.(int)))...)
	}

	if isKey {
		cmds = append(cmds, s.forwardFocused(msg))
	} else {
		cmds = append(cmds, s.forwardAll(msg)...)
	}
	s.sync()
	return s, tea.Batch(cmds...)
}

func (s *Screen) forwardFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch {
	case s.focus.Is(&s.table):
		s.table, cmd = s.table.Update(msg)
	case s.focus.Is(&s.tree):
		s.tree, cmd = s.tree.Update(msg)
	case s.focus.Is(&s.pods):
		s.pods, cmd = s.pods.Update(msg)
	case s.focus.Is(&s.detail):
		s.detail, cmd = s.detail.Update(msg)
	case s.focus.Is(&s.ops):
		s.ops, cmd = s.ops.Update(msg)
	}
	return cmd
}

// forwardAll hands mouse events, timers and everything else to every pane:
// each tests a click against its own rect, and only the one it landed in
// acts.
func (s *Screen) forwardAll(msg tea.Msg) []tea.Cmd {
	cmds := make([]tea.Cmd, 5)
	s.table, cmds[0] = s.table.Update(msg)
	s.tree, cmds[1] = s.tree.Update(msg)
	s.pods, cmds[2] = s.pods.Update(msg)
	s.detail, cmds[3] = s.detail.Update(msg)
	s.ops, cmds[4] = s.ops.Update(msg)
	return cmds
}

// sync makes the dependent panes agree with the ones they follow.
func (s *Screen) sync() {
	if k, ok := s.table.SelectedKey(); ok && k != s.podsOf {
		s.podsOf = k
		s.pods.ClearMarks()
		s.pods.SetKeyedItems(s.podItems())
	}
	for _, f := range []focus.Focusable{&s.table, &s.tree, &s.pods} {
		if s.focus.Is(f) {
			s.source = f
		}
	}
	s.showDetail()
	s.refreshTitles()
}

func (s *Screen) deployment(k string) (deployment, bool) {
	for _, d := range s.deps {
		if d.key() == k {
			return d, true
		}
	}
	return deployment{}, false
}

func (s *Screen) podItems() []list.KeyedItem {
	d, _ := s.deployment(s.podsOf)
	out := make([]list.KeyedItem, len(d.pods))
	for i, p := range d.pods {
		out[i] = list.KeyedItem{Key: d.env + "/" + p.name, Display: phaseGlyph(p.phase) + " " + p.name}
	}
	return out
}

// showDetail puts the source pane's selection in the inspector, when it
// changed. SetFields keeps expansion and the cursor by path, so an unchanged
// record would be harmless, but the guard keeps the detail still while the
// user scrolls it.
func (s *Screen) showDetail() {
	id, title, fields := s.selected()
	if id == "" || id == s.shown {
		return
	}
	s.shown = id
	s.detailTitle = "detail · " + title
	s.detail.SetTitle(s.detailTitle)
	s.detail.SetFields(fields)
}

func (s *Screen) selected() (id, title string, fields []inspector.Field) {
	switch s.source {
	case &s.tree:
		n, ok := s.tree.Selected()
		if !ok {
			return "", "", nil
		}
		nd := n.(*node)
		switch nd.kind {
		case "pod":
			return "pod:" + nd.dep.env + "/" + nd.pod.name, nd.pod.name, podFields(*nd.dep, *nd.pod)
		case "workload":
			return "dep:" + nd.dep.key(), nd.dep.name, deploymentFields(*nd.dep)
		}
		return "ns:" + nd.label, nd.label, namespaceFields(nd)
	case &s.pods:
		k, ok := s.pods.SelectedKey()
		d, found := s.deployment(s.podsOf)
		if !ok || !found {
			return "", "", nil
		}
		for _, p := range d.pods {
			if d.env+"/"+p.name == k {
				return "pod:" + k, p.name, podFields(d, p)
			}
		}
		return "", "", nil
	}
	d, ok := s.deployment(s.podsOf)
	if !ok {
		return "", "", nil
	}
	return "dep:" + d.key(), d.name, deploymentFields(d)
}

// refreshTitles keeps each mark count on its pane's border, which is what
// makes a mark filtered or folded out of sight visible at all.
func (s *Screen) refreshTitles() {
	counted := func(title string, n int) string {
		if n == 0 {
			return title
		}
		return title + " · " + strconv.Itoa(n) + " marked"
	}
	s.table.SetTitle(counted("deployments", s.table.MarkCount()))
	s.tree.SetTitle(counted("cluster", s.tree.MarkCount()))
	if d, ok := s.deployment(s.podsOf); ok {
		s.pods.SetTitle(counted(fmt.Sprintf("pods · %s", d.name), s.pods.MarkCount()))
	}
}

func (s *Screen) Layout() layout.Node {
	body := layout.VStack(
		layout.Flex(1, layout.HStack(
			layout.Flex(3, layout.Sized(&s.table)),
			layout.Flex(2, layout.Sized(&s.tree)),
		)),
		layout.Flex(1, layout.HStack(
			layout.Flex(2, layout.Sized(&s.pods)),
			layout.Flex(3, layout.Sized(&s.detail)),
			layout.Flex(2, layout.Sized(&s.ops)),
		)),
	)
	switch {
	case s.confirmUp:
		return layout.ZStack(body, layout.Center(58, 8, layout.Sized(&s.confirm)))
	case s.alertUp:
		// An autosizing alert centers itself in the rect it is given.
		return layout.ZStack(body, layout.Sized(&s.alert))
	}
	return body
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections is the Group's, which names each pane's groups after it, or
// the modal's alone while one is up, since nothing else takes input then.
func (s *Screen) HelpSections() []help.Section {
	switch {
	case s.confirmUp:
		return help.SectionsOf(&s.confirm)
	case s.alertUp:
		return help.SectionsOf(&s.alert)
	}
	return s.focus.HelpSections()
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	tst := s.table.State()
	to := t.Table()
	to.Filterable = true
	to.Markable = true
	to.Filter.Placeholder = "filter, env:prod, health:~degraded…"
	to.Columns = []table.Column{
		{Title: "Deployment", Flex: 1, Width: 14, Sortable: true},
		{Title: "Env", Width: 8, Sortable: true},
		{Title: "Health", Width: 11, Sortable: true, Less: healthLess},
		{Title: "Ready", Width: 6, Align: lipgloss.Right},
		{Title: "Age", Width: 4, Sortable: true},
	}
	s.table = table.New(to)
	s.table.SetKeyedRows(s.rows())
	s.table.Restore(tst)

	trst := s.tree.State()
	tro := t.Tree()
	tro.Root = s.root
	tro.Searchable = true
	tro.Markable = true
	tro.InitialDepth = 2
	tro.Filter.Placeholder = "search…"
	s.tree = tree.New(tro)
	s.tree.Restore(trst)

	pst := s.pods.State()
	po := t.List()
	po.Filterable = true
	po.Markable = true
	po.Filter.Placeholder = "filter…"
	s.pods = list.New(po)
	s.pods.Restore(pst)

	dst := s.detail.State()
	do := t.Inspector()
	do.Title = "detail"
	if s.detailTitle != "" {
		do.Title = s.detailTitle
	}
	do.Filterable = true
	do.InitialDepth = 1
	do.Filter.Placeholder = "filter fields…"
	s.detail = inspector.New(do)
	s.detail.Restore(dst)

	ost := s.ops.State()
	oo := t.Menu()
	oo.Title = "operations"
	oo.Items = opItems()
	s.ops = menu.New(oo)
	s.ops.Restore(ost)

	// The panes behind these addresses were just replaced; the addresses
	// were not, so the rebuilt Group points at the right ones (rule 27).
	at := s.focus.Index()
	s.focus = focus.NewGroup(&s.table, &s.tree, &s.pods, &s.detail, &s.ops).WithKeys(focus.Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next pane")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev pane")),
	})
	s.focus.SetIndex(at)

	// A modal that is up survives the swap, on the side it was sitting on.
	if s.confirmUp {
		side := s.confirm.Value()
		s.openConfirm()
		s.confirm.SetValue(side)
	}
	if s.alertUp {
		s.openAlert()
	}

	// On the first build sync fills the pods and the detail; on a rebuild the
	// restored panes already hold them, and podsOf and shown say so.
	if s.source == nil {
		s.source = &s.table
	}
	s.sync()
}

func (s *Screen) rows() []table.KeyedRow {
	rows := make([]table.KeyedRow, len(s.deps))
	for i, d := range s.deps {
		rows[i] = table.KeyedRow{Key: d.key(), Cells: []string{
			d.name, d.env, healthCell(d.health()),
			fmt.Sprintf("%d/%d", d.ready(), len(d.pods)), d.age,
		}}
	}
	return rows
}
