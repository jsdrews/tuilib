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

func markables(t *testing.T) map[string]func() markable {
	t.Helper()
	return map[string]func() markable{
		"list":  func() markable { return newListMark(true) },
		"table": func() markable { return newTableMark(true) },
	}
}

func space() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")} }
func down() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyDown} }
func ctrlA() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlA} }

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
			c.send(space())
			if got := c.marks(); !eq(got, []string{"api"}) {
				t.Errorf("marks = %v, want [api]", got)
			}
			c.send(space())
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
			c.send(space()) // worker
			c.send(space()) // untoggle
			c.send(space()) // worker again
			c.send(down())
			c.send(space()) // cache

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
			c.send(space())
			c.send(down())
			c.send(space())
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
func TestMarkingIsInertOnAnonymousData(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build().anonymous()
			c.send(space())
			c.send(ctrlA())
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
			c.send(ctrlA())
			if n := c.markCount(); n != 4 {
				t.Fatalf("markCount = %d, want 4", n)
			}
			c.send(ctrlA())
			if n := c.markCount(); n != 0 {
				t.Errorf("markCount = %d, want 0 — a second mark-all clears", n)
			}
		})
	}
}

// Filter to a subset, mark it wholesale, act. Mark-all must not reach rows
// the filter is hiding.
func TestMarkAllRespectsTheFilter(t *testing.T) {
	for name, build := range markables(t) {
		t.Run(name, func(t *testing.T) {
			c := build()
			c.filterTo("w")
			c.send(ctrlA())

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
			c.send(space()) // mark api
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
			c.send(space())
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
			c.send(space())
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
