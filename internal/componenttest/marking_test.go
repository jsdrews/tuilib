package componenttest

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// Marking is implemented separately in pkg/list and pkg/table, which is
// exactly the shape that has bitten this repo before: behaviour written for
// one component, rolled out to the others by hand, and tested only where it
// was written. So the contract is asserted here, once, across both.
//
// See the "Don't test shared behaviour in one component's package"
// anti-pattern in CLAUDE.md.

// markable is the behaviour both components promise. Each adapter drives the
// real component through its real Update.
type markable interface {
	send(tea.Msg)
	setRect(geom.Rect)
	marks() []string
	markCount() int
	selection() []string
	selectionLabel() string
	setMarks([]string)
	clearMarks()
	cursor() int
	view() string
	// anonymous rebuilds the component with unkeyed data.
	anonymous() markable
	// filterTo types a query and commits it.
	filterTo(string)
}

type listMark struct{ m list.Model }

func newListMark(keyed bool) markable {
	o := theme.Dark().List()
	o.Markable = true
	o.Filterable = true
	l := list.New(o)
	if keyed {
		l.SetKeyedItems([]list.KeyedItem{
			{Key: "api", Display: "api-server"},
			{Key: "web", Display: "web-frontend"},
			{Key: "worker", Display: "worker-pool"},
			{Key: "cache", Display: "cache-redis"},
		})
	} else {
		l.SetItems([]string{"api-server", "web-frontend", "worker-pool", "cache-redis"})
	}
	lm := &listMark{m: l}
	lm.setRect(placed())
	return lm
}

func (l *listMark) send(msg tea.Msg)       { l.m, _ = l.m.Update(msg) }
func (l *listMark) setRect(r geom.Rect)    { l.m.SetRect(r) }
func (l *listMark) marks() []string        { return l.m.Marks() }
func (l *listMark) markCount() int         { return l.m.MarkCount() }
func (l *listMark) selection() []string    { return l.m.Selection() }
func (l *listMark) selectionLabel() string { return l.m.SelectionLabel() }
func (l *listMark) setMarks(k []string)    { l.m.SetMarks(k) }
func (l *listMark) clearMarks()            { l.m.ClearMarks() }
func (l *listMark) cursor() int            { return l.m.Cursor() }
func (l *listMark) view() string           { return l.m.View() }
func (l *listMark) anonymous() markable    { return newListMark(false) }
func (l *listMark) filterTo(q string) {
	l.m.SetValue(q)
	l.setRect(placed())
}

type tableMark struct{ m table.Model }

func newTableMark(keyed bool) markable {
	o := theme.Dark().Table()
	o.Markable = true
	o.Filterable = true
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Ready", Width: 6}}
	t := table.New(o)
	if keyed {
		t.SetKeyedRows([]table.KeyedRow{
			{Key: "api", Cells: []string{"api-server", "3/3"}},
			{Key: "web", Cells: []string{"web-frontend", "2/2"}},
			{Key: "worker", Cells: []string{"worker-pool", "0/4"}},
			{Key: "cache", Cells: []string{"cache-redis", "1/1"}},
		})
	} else {
		t.SetRows([]table.Row{
			{"api-server", "3/3"}, {"web-frontend", "2/2"},
			{"worker-pool", "0/4"}, {"cache-redis", "1/1"},
		})
	}
	tm := &tableMark{m: t}
	tm.setRect(placed())
	return tm
}

func (t *tableMark) send(msg tea.Msg)       { t.m, _ = t.m.Update(msg) }
func (t *tableMark) setRect(r geom.Rect)    { t.m.SetRect(r) }
func (t *tableMark) marks() []string        { return t.m.Marks() }
func (t *tableMark) markCount() int         { return t.m.MarkCount() }
func (t *tableMark) selection() []string    { return t.m.Selection() }
func (t *tableMark) selectionLabel() string { return t.m.SelectionLabel() }
func (t *tableMark) setMarks(k []string)    { t.m.SetMarks(k) }
func (t *tableMark) clearMarks()            { t.m.ClearMarks() }
func (t *tableMark) cursor() int            { return t.m.Cursor() }
func (t *tableMark) view() string           { return t.m.View() }
func (t *tableMark) anonymous() markable    { return newTableMark(false) }
func (t *tableMark) filterTo(q string) {
	t.m.SetValue(q)
	t.setRect(placed())
}

// plainMarkableList / plainMarkableTable are the click fixtures: markable,
// keyed, and *not* filterable, so no inline filter header shifts the rows.
func plainMarkableList() *list.Model {
	o := theme.Dark().List()
	o.Markable = true
	l := list.New(o)
	l.SetKeyedItems([]list.KeyedItem{
		{Key: "api", Display: "api-server"},
		{Key: "web", Display: "web-frontend"},
		{Key: "worker", Display: "worker-pool"},
	})
	return &l
}

func plainMarkableTable() *table.Model {
	o := theme.Dark().Table()
	o.Markable = true
	o.Columns = []table.Column{{Title: "Name", Width: 16}}
	tb := table.New(o)
	tb.SetKeyedRows([]table.KeyedRow{
		{Key: "api", Cells: []string{"api-server"}},
		{Key: "web", Cells: []string{"web-frontend"}},
		{Key: "worker", Cells: []string{"worker-pool"}},
	})
	return &tb
}

func pressAt(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

type treeMark struct{ m tree.Model }

// treeNode is a minimal tree.Node over a label + children.
type treeNode struct {
	label    string
	children []tree.Node
}

func (n treeNode) Label() string         { return n.label }
func (n treeNode) Children() []tree.Node { return n.children }

// newTreeMark mirrors the list/table fixtures: four addressable rows with the
// same keys, so the shared assertions read identically across all three.
//
// A tree's key is its node path, so the "keyed" argument means something
// different here: there is no anonymous tree. keyed=false turns marking off
// instead, which is the tree's version of "the feature is inert".
func newTreeMark(keyed bool) markable {
	o := theme.Dark().Tree()
	o.Markable = keyed
	o.Searchable = true
	o.InitialDepth = 9
	// The root is "api" rather than a wrapper node so the visible rows are
	// exactly api/web/worker/cache in that order — the same four rows, in the
	// same positions, as the list and table fixtures. A wrapper root would add
	// a phantom row 0 that the shared assertions know nothing about.
	o.Root = treeNode{label: "api", children: []tree.Node{
		treeNode{label: "web"},
		treeNode{label: "worker"},
		treeNode{label: "cache"},
	}}
	tm := &treeMark{m: tree.New(o)}
	tm.setRect(placed())
	return tm
}

func (t *treeMark) send(msg tea.Msg)       { t.m, _ = t.m.Update(msg) }
func (t *treeMark) setRect(r geom.Rect)    { t.m.SetRect(r) }
func (t *treeMark) markCount() int         { return t.m.MarkCount() }
func (t *treeMark) selectionLabel() string { return t.m.SelectionLabel() }
func (t *treeMark) setMarks(k []string)    { t.m.SetMarks(t.qualify(k)) }
func (t *treeMark) clearMarks()            { t.m.ClearMarks() }
func (t *treeMark) cursor() int            { return t.m.Cursor() }
func (t *treeMark) view() string           { return t.m.View() }
func (t *treeMark) anonymous() markable    { return newTreeMark(false) }
func (t *treeMark) filterTo(q string) {
	// SetQuery alone only searches (highlight + n/N). Hiding non-matching
	// subtrees is the separate filter mode the "\\" key toggles.
	t.m.SetQuery(q)
	t.m.SetFilterMode(true)
	t.setRect(placed())
}

// A tree's keys are node paths ("api/web"); the shared assertions speak in
// bare names, so translate at the boundary.
func (t *treeMark) marks() []string     { return strip(t.m.Marks()) }
func (t *treeMark) selection() []string { return strip(t.m.Selection()) }

func (t *treeMark) qualify(ks []string) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		if k == "api" {
			out[i] = "api"
			continue
		}
		out[i] = "api/" + k
	}
	return out
}

func strip(ks []string) []string {
	out := make([]string, 0, len(ks))
	for _, k := range ks {
		if i := strings.LastIndex(k, "/"); i >= 0 {
			k = k[i+1:]
		}
		out = append(out, k)
	}
	return out
}

func markables(t *testing.T) map[string]func() markable {
	t.Helper()
	return map[string]func() markable{
		"list":  func() markable { return newListMark(true) },
		"table": func() markable { return newTableMark(true) },
		"tree":  func() markable { return newTreeMark(true) },
	}
}

func space() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")} }

// markX is the mark key every markable component shares. space also marks in
// list and table, but means expand/collapse in a tree, so the shared
// assertions speak x.
func markX() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")} }
func down() tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyDown} }
func markAll() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")} }

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSpaceMarksTheCursorRow(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.send(markX())
			if got := c.marks(); !eq(got, []string{"api"}) {
				t.Errorf("marks = %v, want [api]", got)
			}
			c.send(markX())
			if got := c.marks(); len(got) != 0 {
				t.Errorf("marks = %v, want none — space toggles", got)
			}
		})
	}
}

func TestMarksAccumulateInRowOrder(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.send(down())
			c.send(down())
			c.send(markX()) // worker
			c.send(markX()) // untoggle
			c.send(markX()) // worker again
			c.send(down())
			c.send(markX()) // cache

			// Marked worker then cache; reported in row order regardless.
			if got := c.marks(); !eq(got, []string{"worker", "cache"}) {
				t.Errorf("marks = %v, want [worker cache] in row order", got)
			}
		})
	}
}

// Selection is the accessor screens use. It must collapse "marked set or
// cursor" so no caller has to write that branch.
func TestSelectionFallsBackToTheCursor(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			if got := c.selection(); !eq(got, []string{"api"}) {
				t.Errorf("selection = %v, want [api] from the cursor", got)
			}
			if got := c.selectionLabel(); got != "api" {
				t.Errorf("label = %q, want api", got)
			}

			c.send(down())
			c.send(markX())
			c.send(down())
			c.send(markX())
			if got := c.selection(); !eq(got, []string{"web", "worker"}) {
				t.Errorf("selection = %v, want the marked set [web worker]", got)
			}
			if got := c.selectionLabel(); got != "2 items" {
				t.Errorf("label = %q, want \"2 items\"", got)
			}
		})
	}
}

// The whole reason marks are keys: a swap that reorders and partially
// replaces the set must not move a mark onto a different row.
func TestMarksSurviveAKeyedSwap(t *testing.T) {
	l := newListMark(true).(*listMark)
	l.m.SetMarks([]string{"worker"})

	// Reorder, drop one, add one.
	l.m.SetKeyedItems([]list.KeyedItem{
		{Key: "cache", Display: "cache-redis"},
		{Key: "worker", Display: "worker-pool (scaled)"},
		{Key: "api", Display: "api-server"},
		{Key: "new", Display: "new-thing"},
	})
	if got := l.m.Marks(); !eq(got, []string{"worker"}) {
		t.Errorf("list marks = %v, want [worker] still on the same row", got)
	}

	tb := newTableMark(true).(*tableMark)
	tb.m.SetMarks([]string{"worker"})
	tb.m.SetKeyedRows([]table.KeyedRow{
		{Key: "cache", Cells: []string{"cache-redis", "1/1"}},
		{Key: "worker", Cells: []string{"worker-pool", "4/4"}},
		{Key: "api", Cells: []string{"api-server", "3/3"}},
	})
	if got := tb.m.Marks(); !eq(got, []string{"worker"}) {
		t.Errorf("table marks = %v, want [worker] still on the same row", got)
	}
}

// Marking an anonymous set would have to hold marks by index, which is the
// drift this design refuses. Inert beats approximate.
// A tree has no anonymous form — every node's path is already an identity —
// so this asserts the keyed-data precondition on the two components that have
// one. The tree's version of "inert" is Markable left off, covered below.
func TestMarkingIsInertOnAnonymousData(t *testing.T) {
	for name, build := range map[string]func() markable{
		"list":  func() markable { return newListMark(true) },
		"table": func() markable { return newTableMark(true) },
	} {
		t.Run(name, func(t *testing.T) {
			c := build().anonymous()
			c.send(markX())
			c.send(markAll())
			if n := c.markCount(); n != 0 {
				t.Errorf("markCount = %d, want 0 — anonymous data cannot be marked", n)
			}
			if got := c.selection(); len(got) != 0 {
				t.Errorf("selection = %v, want empty", got)
			}
		})
	}
}

func TestMarkAllMarksEveryVisibleRowThenClears(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.send(markAll())
			if n := c.markCount(); n != 4 {
				t.Fatalf("markCount = %d, want 4", n)
			}
			c.send(markAll())
			if n := c.markCount(); n != 0 {
				t.Errorf("markCount = %d, want 0 — a second mark-all clears", n)
			}
		})
	}
}

// Filter to a subset, mark it wholesale, act. Mark-all must not reach rows
// the filter is hiding.
func TestMarkAllRespectsTheFilter(t *testing.T) {
	for name, build := range map[string]func() markable{
		"list":  func() markable { return newListMark(true) },
		"table": func() markable { return newTableMark(true) },
	} {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.filterTo("w")
			c.send(markAll())

			got := c.marks()
			for _, k := range got {
				if k != "web" && k != "worker" {
					t.Errorf("marked %q, which the filter was hiding", k)
				}
			}
			if len(got) == 0 {
				t.Error("mark-all marked nothing under a filter")
			}
		})
	}
}

// Marks are keys, so they do not care whether a row is currently visible.
// Correct, and a real surprise — which is why the action menu names its
// target.
func TestMarksSurviveFiltering(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.send(markX()) // mark api
			c.filterTo("cache")
			if n := c.markCount(); n != 1 {
				t.Errorf("markCount = %d, want 1 — a filtered-out mark is still marked", n)
			}
			if got := c.selection(); !eq(got, []string{"api"}) {
				t.Errorf("selection = %v, want [api]", got)
			}
		})
	}
}

func TestSetMarksAndClearMarks(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.setMarks([]string{"api", "cache"})
			if got := c.marks(); !eq(got, []string{"api", "cache"}) {
				t.Errorf("marks = %v, want [api cache]", got)
			}
			c.clearMarks()
			if n := c.markCount(); n != 0 {
				t.Errorf("markCount = %d, want 0", n)
			}
		})
	}
}

func TestMarkedRowRendersTheGlyph(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			before := strings.Count(c.view(), "✓")
			c.send(down())
			c.send(markX())
			if after := strings.Count(c.view(), "✓"); after != before+1 {
				t.Errorf("✓ count went %d → %d, want one more", before, after)
			}
		})
	}
}

// Space must reach the filter as a literal space, not toggle a mark behind
// the user's back while they type.
func TestSpaceTypesIntoAFocusedFilter(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
			c.send(markX())
			if n := c.markCount(); n != 0 {
				t.Errorf("markCount = %d, want 0 — space belongs to the focused filter", n)
			}
		})
	}
}

// Clicking the ✓ gutter toggles the mark without opening the row, the way a
// tree's ▸ toggles on a single click.
//
// Built without the inline filter so the interior geometry is simple: for a
// list, content starts on the first inner row; for a table, the column header
// and its rule take the first two.
func TestClickingTheMarkGutterToggles(t *testing.T) {
	const contentX = 1 // rect.X + the pane's left border

	l := plainMarkableList()
	l.SetRect(placed())
	// list gutter: cursor glyph at contentX, ✓ at contentX+1.
	*l, _ = l.Update(pressAt(contentX+1, 1))
	if got := l.Marks(); !eq(got, []string{"api"}) {
		t.Errorf("list marks = %v, want [api] from a gutter click", got)
	}

	tb := plainMarkableTable()
	tb.SetRect(placed())
	// table gutter: ✓ at contentX (no cursor glyph); data starts below the
	// column header and its rule.
	*tb, _ = tb.Update(pressAt(contentX, 3))
	if got := tb.Marks(); !eq(got, []string{"api"}) {
		t.Errorf("table marks = %v, want [api] from a gutter click", got)
	}
}

// Clicking the row body selects as it always did — the gutter is one cell
// wide precisely so the rest of the row keeps its old behaviour.
func TestClickingTheRowBodyStillSelects(t *testing.T) {
	l := plainMarkableList()
	l.SetRect(placed())
	*l, _ = l.Update(pressAt(bodyX+4, 3))
	if n := l.MarkCount(); n != 0 {
		t.Errorf("markCount = %d, want 0 — a body click must not mark", n)
	}
	if l.Cursor() != 2 {
		t.Errorf("cursor = %d, want 2", l.Cursor())
	}
}

// Turning marking off must cost a row nothing.
func TestGutterIsAbsentWhenMarkingIsOff(t *testing.T) {
	o := theme.Dark().List()
	o.Markable = false
	l := list.New(o)
	l.SetItems([]string{"alpha"})
	l.SetRect(placed())
	if strings.Contains(l.View(), "✓") {
		t.Error("a non-markable list drew a mark gutter")
	}
}

// D drops the selection outright, from any state.
//
// The asymmetry it fixes: A is a toggle over the visible rows, so from a
// partial selection it marks the rest and only a second press clears. Without
// a dedicated key, "drop what I marked" routes through a state where every row
// is marked — which on a screen whose next keystroke might be a destructive
// verb is the wrong place to pass through.
func TestDropMarksKeyClearsFromAnyState(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()

			// Nothing marked: the key is harmless.
			m.send(dropMarks())
			if got := m.markCount(); got != 0 {
				t.Fatalf("D on an empty selection: MarkCount = %d, want 0", got)
			}

			// Partial selection — the case A cannot clear in one press.
			m.send(markX())
			m.send(down())
			m.send(markX())
			if got := m.markCount(); got != 2 {
				t.Fatalf("setup: MarkCount = %d, want 2", got)
			}

			m.send(dropMarks())
			if got := m.markCount(); got != 0 {
				t.Errorf("D on a partial selection: MarkCount = %d, want 0", got)
			}

			// Full selection clears too, without needing to know it was full.
			m.send(markAll())
			if m.markCount() == 0 {
				t.Fatal("setup: A marked nothing")
			}
			m.send(dropMarks())
			if got := m.markCount(); got != 0 {
				t.Errorf("D on a full selection: MarkCount = %d, want 0", got)
			}
		})
	}
}

// The cursor is not a mark, so clearing the selection must leave it alone —
// otherwise D on a marked row would also throw away where the user was.
func TestClearMarksLeavesTheCursorWhereItWas(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()
			m.send(down())
			m.send(down())
			m.send(markX())
			want := m.cursor()

			m.send(dropMarks())

			if got := m.cursor(); got != want {
				t.Errorf("cursor moved on clear: got %d, want %d", got, want)
			}
			// Selection falls back to the cursor row, so it is not empty.
			if len(m.selection()) != 1 {
				t.Errorf("Selection() = %v, want the cursor row alone", m.selection())
			}
		})
	}
}

// D must not steal the key from a focused filter's text input.
func TestClearMarksDoesNotFireWhileFiltering(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()
			m.send(markX())
			if m.markCount() != 1 {
				t.Fatalf("setup: nothing marked")
			}

			m.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
			m.send(dropMarks())

			if got := m.markCount(); got != 1 {
				t.Errorf("D fired while the filter had focus: MarkCount = %d, want 1", got)
			}
		})
	}
}

func dropMarks() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("D")} }

// V marks from the anchor — the most recently marked row — to the cursor.
func TestMarkRangeExtendsFromTheAnchor(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build() // api, web, worker, cache

			m.send(markX()) // anchor on row 0
			m.send(down())
			m.send(down()) // cursor on row 2
			m.send(shiftX())

			want := []string{"api", "web", "worker"}
			if got := m.marks(); !eq(got, want) {
				t.Errorf("Marks() = %v, want %v", got, want)
			}
		})
	}
}

// The spec's fallback: no anchor at all means X marks the cursor row alone,
// and that row becomes the anchor for the next X.
func TestMarkRangeWithoutAnAnchorMarksOnlyTheCursor(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()
			m.send(down())
			m.send(shiftX())

			if got := m.marks(); !eq(got, []string{"web"}) {
				t.Fatalf("Marks() = %v, want [web]", got)
			}

			// It anchored, so a second X from further down draws a range.
			m.send(down())
			m.send(shiftX())
			if got := m.marks(); !eq(got, []string{"web", "worker"}) {
				t.Errorf("Marks() = %v, want [web worker]", got)
			}
		})
	}
}

// Ranges run in either direction: an anchor below the cursor spans upward.
func TestMarkRangeRunsBackwardToo(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()

			m.send(down())
			m.send(down())
			m.send(markX()) // anchor on row 2 (worker)
			m.send(up())
			m.send(up()) // cursor back to row 0

			m.send(shiftX())

			want := []string{"api", "web", "worker"}
			if got := m.marks(); !eq(got, want) {
				t.Errorf("Marks() = %v, want %v — a backward range must span upward", got, want)
			}
		})
	}
}

// The anchor is fixed: ranging again from the same anchor spans to the new
// cursor rather than walking the anchor along behind it.
func TestMarkRangeKeepsTheAnchorFixed(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()

			m.send(down())
			m.send(markX()) // anchor row 1 (web)
			m.send(down())
			m.send(shiftX()) // web..worker
			m.send(down())
			m.send(shiftX()) // still from web -> web..cache

			want := []string{"web", "worker", "cache"}
			if got := m.marks(); !eq(got, want) {
				t.Errorf("Marks() = %v, want %v — the anchor moved between ranges", got, want)
			}
		})
	}
}

// A range adds to what is already marked rather than replacing it.
func TestMarkRangeIsAdditive(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()

			// Mark the last row, then range over the first two.
			m.send(down())
			m.send(down())
			m.send(down())
			m.send(markX()) // cache marked, anchored
			m.setMarks([]string{"cache"})

			// Re-anchor at row 0 and range to row 1.
			m.send(up())
			m.send(up())
			m.send(up())
			m.send(markX())
			m.send(down())
			m.send(shiftX())

			want := []string{"api", "web", "cache"}
			if got := m.marks(); !eq(got, want) {
				t.Errorf("Marks() = %v, want %v — the range dropped a pre-existing mark", got, want)
			}
		})
	}
}

// Unmarking the anchor row retires the anchor, so the next X does not draw a
// range from a row the user has just deselected.
func TestUnmarkingTheAnchorRetiresIt(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()

			m.send(markX()) // anchor row 0
			m.send(markX()) // unmark it again
			m.send(down())
			m.send(down())
			m.send(shiftX())

			if got := m.marks(); !eq(got, []string{"worker"}) {
				t.Errorf("Marks() = %v, want [worker] — a retired anchor still drew a range", got)
			}
		})
	}
}

func shiftX() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")} }
func up() tea.KeyMsg     { return tea.KeyMsg{Type: tea.KeyUp} }

// space must never mark. It is expand/collapse in a tree and plain navigation
// elsewhere; x is the mark verb library-wide. This is asserted rather than
// assumed because space *did* mark in list and table for a while, and a
// leftover binding would silently mark rows under a key nothing documents.
func TestSpaceNeverMarks(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			m := build()
			m.send(space())
			if n := m.markCount(); n != 0 {
				t.Errorf("markCount = %d, want 0 — space must not mark", n)
			}
		})
	}
}

func contains(ks []string, want string) bool {
	for _, k := range ks {
		if k == want {
			return true
		}
	}
	return false
}
