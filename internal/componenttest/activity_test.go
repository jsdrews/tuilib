package componenttest

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// Row activity is implemented separately in pkg/list, pkg/table and pkg/tree
// — the shape that has bitten this repo before, where behaviour written for
// one component is rolled out to the others by hand and tested only where it
// was written. So the contract is asserted here, once, across all three.
//
// See the "Don't test shared behaviour in one component's package"
// anti-pattern in CLAUDE.md, and marking_test.go for the precedent.

// busy is the behaviour all three components promise.
type busy interface {
	send(tea.Msg)
	setRect(geom.Rect)
	setActivity(key, label string) tea.Cmd
	endActivity(key string, err error) tea.Cmd
	clearActivity(key string)
	relabel(key, label string)
	activityCount() int
	state() activity.Set
	adopt(activity.Set) tea.Cmd
	view() string
	// reorder swaps the underlying data around without changing the keys,
	// standing in for a pkg/poll refresh.
	reorder()
	// rebuilt is a fresh instance of the same component, as a theme swap
	// would produce.
	rebuilt() busy
	// anonymous rebuilds with unkeyed data, where activity must be inert.
	anonymous() busy
}

// --- list ---------------------------------------------------------------

type listBusy struct{ m list.Model }

func newListBusy(keyed, reversed bool) busy {
	o := theme.Dark().List()
	l := list.New(o)
	if keyed {
		items := []list.KeyedItem{
			{Key: "api", Display: "api-server"},
			{Key: "web", Display: "web-frontend"},
			{Key: "worker", Display: "worker-pool"},
		}
		if reversed {
			items[0], items[2] = items[2], items[0]
		}
		l.SetKeyedItems(items)
	} else {
		l.SetItems([]string{"api-server", "web-frontend", "worker-pool"})
	}
	b := &listBusy{m: l}
	b.setRect(placed())
	return b
}

func (b *listBusy) send(msg tea.Msg)                      { b.m, _ = b.m.Update(msg) }
func (b *listBusy) setRect(r geom.Rect)                   { b.m.SetRect(r) }
func (b *listBusy) setActivity(k, l string) tea.Cmd       { return b.m.SetActivity(k, l) }
func (b *listBusy) endActivity(k string, e error) tea.Cmd { return b.m.EndActivity(k, e) }
func (b *listBusy) clearActivity(k string)                { b.m.ClearActivity(k) }
func (b *listBusy) relabel(k, l string)                   { b.m.Relabel(k, l) }
func (b *listBusy) activityCount() int                    { return b.m.ActivityCount() }
func (b *listBusy) state() activity.Set                   { return b.m.ActivityState() }
func (b *listBusy) adopt(s activity.Set) tea.Cmd          { return b.m.SetActivityState(s) }
func (b *listBusy) view() string                          { return b.m.View() }
func (b *listBusy) rebuilt() busy                         { return newListBusy(true, false) }
func (b *listBusy) anonymous() busy                       { return newListBusy(false, false) }
func (b *listBusy) reorder() {
	b.m.SetKeyedItems([]list.KeyedItem{
		{Key: "worker", Display: "worker-pool"},
		{Key: "api", Display: "api-server"},
		{Key: "web", Display: "web-frontend"},
	})
	b.setRect(placed())
}

// --- table --------------------------------------------------------------

type tableBusy struct{ m table.Model }

func newTableBusy(keyed bool, activityCol string) busy {
	o := theme.Dark().Table()
	o.ActivityColumn = activityCol
	o.Columns = []table.Column{
		{Title: "Name", Width: 16},
		{Title: "Status", Width: 14},
	}
	t := table.New(o)
	if keyed {
		t.SetKeyedRows([]table.KeyedRow{
			{Key: "api", Cells: []string{"api-server", "Synced"}},
			{Key: "web", Cells: []string{"web-frontend", "OutOfSync"}},
			{Key: "worker", Cells: []string{"worker-pool", "Synced"}},
		})
	} else {
		t.SetRows([]table.Row{
			{"api-server", "Synced"},
			{"web-frontend", "OutOfSync"},
			{"worker-pool", "Synced"},
		})
	}
	b := &tableBusy{m: t}
	b.setRect(placed())
	return b
}

func (b *tableBusy) send(msg tea.Msg)                      { b.m, _ = b.m.Update(msg) }
func (b *tableBusy) setRect(r geom.Rect)                   { b.m.SetRect(r) }
func (b *tableBusy) setActivity(k, l string) tea.Cmd       { return b.m.SetActivity(k, l) }
func (b *tableBusy) endActivity(k string, e error) tea.Cmd { return b.m.EndActivity(k, e) }
func (b *tableBusy) clearActivity(k string)                { b.m.ClearActivity(k) }
func (b *tableBusy) relabel(k, l string)                   { b.m.Relabel(k, l) }
func (b *tableBusy) activityCount() int                    { return b.m.ActivityCount() }
func (b *tableBusy) state() activity.Set                   { return b.m.ActivityState() }
func (b *tableBusy) adopt(s activity.Set) tea.Cmd          { return b.m.SetActivityState(s) }
func (b *tableBusy) view() string                          { return b.m.View() }
func (b *tableBusy) rebuilt() busy                         { return newTableBusy(true, "Status") }
func (b *tableBusy) anonymous() busy                       { return newTableBusy(false, "Status") }
func (b *tableBusy) reorder() {
	b.m.SetKeyedRows([]table.KeyedRow{
		{Key: "worker", Cells: []string{"worker-pool", "Synced"}},
		{Key: "api", Cells: []string{"api-server", "Synced"}},
		{Key: "web", Cells: []string{"web-frontend", "OutOfSync"}},
	})
	b.setRect(placed())
}

// --- tree ---------------------------------------------------------------

type treeBusy struct{ m tree.Model }

func newTreeBusy() busy {
	o := theme.Dark().Tree()
	o.InitialDepth = 2
	o.Root = treeNode{label: "cluster", children: []tree.Node{
		treeNode{label: "api"},
		treeNode{label: "web"},
		treeNode{label: "worker"},
	}}
	b := &treeBusy{m: tree.New(o)}
	b.setRect(placed())
	return b
}

func (b *treeBusy) send(msg tea.Msg)                      { b.m, _ = b.m.Update(msg) }
func (b *treeBusy) setRect(r geom.Rect)                   { b.m.SetRect(r) }
func (b *treeBusy) setActivity(k, l string) tea.Cmd       { return b.m.SetActivity(b.key(k), l) }
func (b *treeBusy) endActivity(k string, e error) tea.Cmd { return b.m.EndActivity(b.key(k), e) }
func (b *treeBusy) clearActivity(k string)                { b.m.ClearActivity(b.key(k)) }
func (b *treeBusy) relabel(k, l string)                   { b.m.Relabel(b.key(k), l) }
func (b *treeBusy) activityCount() int                    { return b.m.ActivityCount() }
func (b *treeBusy) state() activity.Set                   { return b.m.ActivityState() }
func (b *treeBusy) adopt(s activity.Set) tea.Cmd          { return b.m.SetActivityState(s) }
func (b *treeBusy) view() string                          { return b.m.View() }
func (b *treeBusy) rebuilt() busy                         { return newTreeBusy() }
func (b *treeBusy) anonymous() busy                       { return nil } // paths are always keys
func (b *treeBusy) reorder()                              {}             // no keyed swap to make

// key maps a shared test key onto this tree's real node path. A tree's keys
// are paths, so the translation happens at the boundary — exactly as
// marking_test.go does it.
func (b *treeBusy) key(k string) string { return "cluster/" + k }

// --- the contract -------------------------------------------------------

func busyComponents() map[string]func() busy {
	return map[string]func() busy{
		"list":  func() busy { return newListBusy(true, false) },
		"table": func() busy { return newTableBusy(true, "Status") },
		"tree":  newTreeBusy,
	}
}

func eachBusy(t *testing.T, fn func(t *testing.T, b busy)) {
	t.Helper()
	for name, mk := range busyComponents() {
		t.Run(name, func(t *testing.T) { fn(t, mk()) })
	}
}

func TestActivityRendersOnAKeyedRow(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		if cmd := b.setActivity("api", "syncing"); cmd == nil {
			t.Fatal("SetActivity returned no tick command")
		}
		b.setRect(placed())
		if !strings.Contains(b.view(), "syncing") {
			t.Errorf("view does not show the label:\n%s", b.view())
		}
		if b.activityCount() != 1 {
			t.Errorf("ActivityCount = %d, want 1", b.activityCount())
		}
	})
}

func TestActivityIsInertOnAnonymousRows(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		anon := b.anonymous()
		if anon == nil {
			t.Skip("this component's rows are always keyed")
		}
		anon.setActivity("api", "syncing")
		anon.setRect(placed())
		if strings.Contains(anon.view(), "syncing") {
			t.Errorf("an unkeyed row drew an indicator:\n%s", anon.view())
		}
	})
}

// The reason activity is keyed at all: a poll refresh reorders the set
// mid-flight and the spinner has to stay on its own row.
func TestActivitySurvivesAKeyedReorder(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		b.setActivity("api", "syncing")
		b.reorder()
		if b.activityCount() != 1 {
			t.Fatalf("ActivityCount = %d after a reorder, want 1", b.activityCount())
		}
		if !strings.Contains(b.view(), "syncing") {
			t.Errorf("the indicator did not survive the swap:\n%s", b.view())
		}
	})
}

func TestActivityOutcomeGlyphs(t *testing.T) {
	g := glyph.Default()
	eachBusy(t, func(t *testing.T, b busy) {
		b.setActivity("api", "syncing")
		b.endActivity("api", nil)
		b.setRect(placed())
		if !strings.Contains(b.view(), g.ActivityOK) {
			t.Errorf("no success glyph after a nil error:\n%s", b.view())
		}
		if b.activityCount() != 0 {
			t.Errorf("ActivityCount = %d, want 0 — a held outcome is not running", b.activityCount())
		}

		b.setActivity("api", "syncing")
		b.endActivity("api", errors.New("boom"))
		b.setRect(placed())
		if !strings.Contains(b.view(), g.ActivityFail) {
			t.Errorf("no failure glyph after an error:\n%s", b.view())
		}
	})
}

func TestActivityClears(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		b.setActivity("api", "syncing")
		b.clearActivity("api")
		b.setRect(placed())
		if strings.Contains(b.view(), "syncing") {
			t.Errorf("ClearActivity left the indicator up:\n%s", b.view())
		}
	})
}

func TestActivityRelabels(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		b.setActivity("api", "syncing")
		b.relabel("api", "pruning")
		b.setRect(placed())
		v := b.view()
		if !strings.Contains(v, "pruning") || strings.Contains(v, "syncing") {
			t.Errorf("Relabel did not take:\n%s", v)
		}
	})
}

// Rule 4: a theme swap rebuilds the component, and the work in flight has to
// come across with the cursor and the marks.
func TestActivitySurvivesAThemeRebuild(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		b.setActivity("api", "syncing")

		fresh := b.rebuilt()
		if cmd := fresh.adopt(b.state()); cmd == nil {
			t.Fatal("SetActivityState returned no command; the spinner is stranded")
		}
		fresh.setRect(placed())
		if !strings.Contains(fresh.view(), "syncing") {
			t.Errorf("the rebuilt component lost the work:\n%s", fresh.view())
		}
	})
}

// The broadcast a component does not hold keys for must change nothing. Two
// panes on one screen both see every StartMsg (rule 6); only the one holding
// the key may act.
func TestActivityBroadcastForForeignKeysIsIgnored(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		b.send(activity.StartMsg{Keys: []string{"nothing-here"}, Label: "syncing", RunID: 3})
		b.setRect(placed())
		if b.activityCount() != 0 {
			t.Errorf("ActivityCount = %d, want 0 for a key the component does not hold", b.activityCount())
		}
		if strings.Contains(b.view(), "syncing") {
			t.Errorf("a foreign key drew an indicator:\n%s", b.view())
		}
	})
}

// An idle component must schedule no ticks, or every table in the app pays a
// timer for a feature it is not using.
func TestIdleComponentSchedulesNothing(t *testing.T) {
	eachBusy(t, func(t *testing.T, b busy) {
		var got tea.Cmd
		switch c := b.(type) {
		case *listBusy:
			c.m, got = c.m.Update(tea.KeyMsg{Type: tea.KeyDown})
		case *tableBusy:
			c.m, got = c.m.Update(tea.KeyMsg{Type: tea.KeyDown})
		case *treeBusy:
			c.m, got = c.m.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
		_ = got // a nav key may legitimately emit its own message
		if b.activityCount() != 0 {
			t.Errorf("an idle component reported %d running", b.activityCount())
		}
	})
}

// --- table-specific -----------------------------------------------------

// A name that resolves to no column must degrade to a visible gutter rather
// than to silence or to column 0.
func TestTableUnresolvableActivityColumnFallsBackToGutter(t *testing.T) {
	b := newTableBusy(true, "Nonexistent").(*tableBusy)
	b.setActivity("api", "syncing")
	b.setRect(placed())

	v := b.view()
	if !strings.Contains(v, "api-server") {
		t.Fatalf("the row vanished:\n%s", v)
	}
	if strings.Contains(v, "Synced") == false {
		t.Errorf("column 0 or the status cell was overwritten:\n%s", v)
	}
	if b.activityCount() != 1 {
		t.Errorf("ActivityCount = %d, want 1", b.activityCount())
	}
}

// Under SetWindow the rows carry no keys, so an indicator could only be held
// by index into a sparse paged set. Inert, not approximate.
func TestTableWindowedIsInert(t *testing.T) {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	tbl := table.New(o)
	tbl.SetWindow([]table.Row{{"api-server", "Synced"}}, 0, 40)
	tbl.SetRect(placed())

	if cmd := tbl.SetActivity("api", "syncing"); cmd != nil {
		t.Error("SetActivity armed a spinner on a windowed table")
	}
	if strings.Contains(tbl.View(), "syncing") {
		t.Errorf("a windowed table drew an indicator:\n%s", tbl.View())
	}
}

// The activity cell must not clobber the selected row's background: the table
// draws its cursor row as one styled run, and a lipgloss render inside a cell
// would close it at the first reset (rule 19).
func TestTableActivityCellKeepsSelectedRowIntact(t *testing.T) {
	b := newTableBusy(true, "Status").(*tableBusy)
	b.setActivity("api", "syncing") // "api" is the cursor row
	b.setRect(placed())

	for _, line := range strings.Split(b.view(), "\n") {
		if !strings.Contains(line, "syncing") {
			continue
		}
		// The cell must close with a foreground-only reset. A full \x1b[0m
		// here would end the selected row's background mid-row; the one at
		// the end of the line is the row's own style closing, which is fine.
		if !strings.Contains(line, "syncing\x1b[39m") {
			t.Errorf("the activity cell did not close foreground-only:\n%q", line)
		}
		if strings.Contains(line, "syncing\x1b[0m") {
			t.Errorf("the activity cell emitted a full reset inside the selected row:\n%q", line)
		}
		return
	}
	t.Fatalf("no row showed the indicator:\n%s", b.view())
}

// Widths come from the rows the table holds, never from the indicator, so a
// run cannot reflow the columns under the user.
func TestTableActivityDoesNotReflowColumns(t *testing.T) {
	b := newTableBusy(true, "Status").(*tableBusy)
	b.setRect(placed())
	before := widestLine(b.view())

	b.setActivity("api", "a very long status label indeed")
	b.setRect(placed())

	if after := widestLine(b.view()); after != before {
		t.Errorf("table width moved from %d to %d under an activity label", before, after)
	}
}

// widestLine measures visible width. Counting runes would count the escapes
// the indicator adds and report a reflow that never happened.
func widestLine(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if n := xansi.StringWidth(line); n > w {
			w = n
		}
	}
	return w
}

// --- list/tree-specific -------------------------------------------------

// The badge is the news: when the row text and the indicator cannot both fit,
// the row is what gives way.
func TestBadgeSurvivesALongRow(t *testing.T) {
	o := theme.Dark().List()
	l := list.New(o)
	l.SetKeyedItems([]list.KeyedItem{{Key: "api", Display: strings.Repeat("x", 200)}})
	l.SetRect(placed())
	l.SetActivity("api", "syncing")
	l.SetRect(placed())

	if !strings.Contains(l.View(), "syncing") {
		t.Errorf("a long row squeezed out the badge:\n%s", l.View())
	}
}

func TestActivityHoldExpires(t *testing.T) {
	o := theme.Dark().List()
	o.Activity.Hold = time.Millisecond
	l := list.New(o)
	l.SetKeyedItems([]list.KeyedItem{{Key: "api", Display: "api-server"}})
	l.SetRect(placed())

	l.SetActivity("api", "syncing")
	cmd := l.EndActivity("api", nil)
	if cmd == nil {
		t.Fatal("EndActivity armed no hold")
	}
	l.SetRect(placed())
	if !strings.Contains(l.View(), glyph.Default().ActivityOK) {
		t.Fatal("the outcome did not render during its hold")
	}

	l, _ = l.Update(cmd())
	l.SetRect(placed())
	if strings.Contains(l.View(), glyph.Default().ActivityOK) {
		t.Errorf("the outcome outlived its hold:\n%s", l.View())
	}
}

// --- derived activity (decisions 18-20) ----------------------------------

// A source of truth drives the indicator without anyone calling a setter, so
// the contract is asserted across all three components the same way the
// imperative half is.
type derivable interface {
	setRect(geom.Rect)
	view() string
	// observe swaps in data where each named key carries that status, and
	// every other key carries "successful".
	observe(status map[string]string)
	// pending drains whatever the setter queued, as Update would.
	pending() tea.Cmd
	setActivity(key, label string) tea.Cmd
	finish(key string, err error) tea.Cmd
	state(key string) (activity.State, bool)
	count() int
}

var derivedKeys = []string{"api", "web", "worker"}

// statusOf builds the full status map for one observation.
func statusOf(busy map[string]string) map[string]string {
	out := map[string]string{}
	for _, k := range derivedKeys {
		if s, ok := busy[k]; ok {
			out[k] = s
			continue
		}
		out[k] = "successful"
	}
	return out
}

// lastField is the predicate shape for components whose row is free text: the
// status is the last word of it.
func lastField(values ...string) func(string) (string, bool) {
	pred := activity.Busy(values...)
	return func(s string) (string, bool) {
		f := strings.Fields(s)
		if len(f) == 0 {
			return "", false
		}
		return pred(f[len(f)-1])
	}
}

type listDerive struct{ m list.Model }

func newListDerive() derivable {
	o := theme.Dark().List()
	o.ActivityWhen = lastField("running", "pending")
	o.ActivityRevision = func(item string) string { return item }
	d := &listDerive{m: list.New(o)}
	d.observe(statusOf(nil))
	d.setRect(placed())
	return d
}

func (d *listDerive) setRect(r geom.Rect) { d.m.SetRect(r) }
func (d *listDerive) view() string        { return d.m.View() }
func (d *listDerive) observe(status map[string]string) {
	items := make([]list.KeyedItem, 0, len(derivedKeys))
	for _, k := range derivedKeys {
		items = append(items, list.KeyedItem{Key: k, Display: k + " " + status[k]})
	}
	d.m.SetKeyedItems(items)
	d.setRect(placed())
}
func (d *listDerive) pending() tea.Cmd {
	var cmd tea.Cmd
	d.m, cmd = d.m.Update(struct{}{})
	return cmd
}
func (d *listDerive) setActivity(k, l string) tea.Cmd       { return d.m.SetActivity(k, l) }
func (d *listDerive) finish(k string, e error) tea.Cmd      { return d.m.EndActivity(k, e) }
func (d *listDerive) state(k string) (activity.State, bool) { return d.m.ActivityState().State(k) }
func (d *listDerive) count() int                            { return d.m.ActivityCount() }

type tableDerive struct{ m table.Model }

func newTableDerive() derivable {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy("running", "pending")(c[1])
	}
	o.ActivityRevision = func(c table.Row) string { return c[1] }
	d := &tableDerive{m: table.New(o)}
	d.observe(statusOf(nil))
	d.setRect(placed())
	return d
}

func (d *tableDerive) setRect(r geom.Rect) { d.m.SetRect(r) }
func (d *tableDerive) view() string        { return d.m.View() }
func (d *tableDerive) observe(status map[string]string) {
	rows := make([]table.KeyedRow, 0, len(derivedKeys))
	for _, k := range derivedKeys {
		rows = append(rows, table.KeyedRow{Key: k, Cells: []string{k, status[k]}})
	}
	d.m.SetKeyedRows(rows)
	d.setRect(placed())
}
func (d *tableDerive) pending() tea.Cmd {
	var cmd tea.Cmd
	d.m, cmd = d.m.Update(struct{}{})
	return cmd
}
func (d *tableDerive) setActivity(k, l string) tea.Cmd       { return d.m.SetActivity(k, l) }
func (d *tableDerive) finish(k string, e error) tea.Cmd      { return d.m.EndActivity(k, e) }
func (d *tableDerive) state(k string) (activity.State, bool) { return d.m.ActivityState().State(k) }
func (d *tableDerive) count() int                            { return d.m.ActivityCount() }

// statusNode carries its status beside its label rather than in it, so a node
// keeps the same path across observations — which is what lets expansion state
// and activity keys survive a swap.
type statusNode struct {
	label    string
	status   string
	children []tree.Node
}

func (n statusNode) Label() string         { return n.label }
func (n statusNode) Children() []tree.Node { return n.children }

type treeDerive struct{ m tree.Model }

func newTreeDerive() derivable {
	o := theme.Dark().Tree()
	o.InitialDepth = 2
	// Root at construction, so New's preExpand opens it — SetRoot alone
	// leaves a fresh tree collapsed, and a spinner on a hidden child proves
	// nothing.
	o.Root = derivedTree(statusOf(nil))
	o.ActivityWhen = func(n tree.Node) (string, bool) {
		sn, ok := n.(statusNode)
		if !ok {
			return "", false
		}
		return activity.Busy("running", "pending")(sn.status)
	}
	o.ActivityRevision = func(n tree.Node) string {
		sn, _ := n.(statusNode)
		return sn.status
	}
	d := &treeDerive{m: tree.New(o)}
	d.setRect(placed())
	return d
}

func derivedTree(status map[string]string) tree.Node {
	kids := make([]tree.Node, 0, len(derivedKeys))
	for _, k := range derivedKeys {
		kids = append(kids, statusNode{label: k, status: status[k]})
	}
	return statusNode{label: "cluster", children: kids}
}

func (d *treeDerive) setRect(r geom.Rect) { d.m.SetRect(r) }
func (d *treeDerive) view() string        { return d.m.View() }
func (d *treeDerive) observe(status map[string]string) {
	d.m.SetRoot(derivedTree(status))
	d.setRect(placed())
}
func (d *treeDerive) pending() tea.Cmd {
	var cmd tea.Cmd
	d.m, cmd = d.m.Update(struct{}{})
	return cmd
}

// A tree's keys are node paths; translate at the boundary.
func (d *treeDerive) path(k string) string             { return "cluster/" + k }
func (d *treeDerive) setActivity(k, l string) tea.Cmd  { return d.m.SetActivity(d.path(k), l) }
func (d *treeDerive) finish(k string, e error) tea.Cmd { return d.m.EndActivity(d.path(k), e) }
func (d *treeDerive) state(k string) (activity.State, bool) {
	return d.m.ActivityState().State(d.path(k))
}
func (d *treeDerive) count() int { return d.m.ActivityCount() }

func eachDerivable(t *testing.T, fn func(t *testing.T, d derivable)) {
	t.Helper()
	for name, mk := range map[string]func() derivable{
		"list":  newListDerive,
		"table": newTableDerive,
		"tree":  newTreeDerive,
	} {
		t.Run(name, func(t *testing.T) { fn(t, mk()) })
	}
}

func TestDerivedActivityNeedsNoSetterCall(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(map[string]string{"web": "running"}))
		if d.count() != 1 {
			t.Fatalf("count = %d, want 1 from the data alone", d.count())
		}
		if !strings.Contains(d.view(), "running") {
			t.Errorf("no indicator from a polled status:\n%s", d.view())
		}
		if d.pending() == nil {
			t.Error("the setter queued no tick, so the spinner would never animate")
		}
	})
}

func TestDerivedActivityStopsOnARestingStatus(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(map[string]string{"web": "running"}))
		d.observe(statusOf(nil))

		if d.count() != 0 {
			t.Errorf("count = %d, want 0 once the data says it is resting", d.count())
		}
		if strings.Contains(d.view(), glyph.Default().ActivityOK) {
			t.Errorf("a derived entry left an outcome glyph behind:\n%s", d.view())
		}
	})
}

// The flicker every real app hits within a minute: a poll already in flight
// when the user acted returns the pre-click value.
func TestStaleObservationDoesNotWipeALocalIndicator(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.setActivity("web", "launching")
		d.observe(statusOf(nil)) // the stale page

		st, ok := d.state("web")
		if !ok || st.Done || st.Label != "launching" {
			t.Errorf("state = %+v, %v; the local indicator was wiped", st, ok)
		}
	})
}

func TestLocalIndicatorHandsOffToTheData(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.setActivity("web", "launching")
		d.finish("web", nil)

		// Still moving: the data has not been observed since it finished.
		if st, _ := d.state("web"); st.Done {
			t.Fatalf("state = %+v; want the row still moving while it waits", st)
		}

		d.observe(statusOf(map[string]string{"web": "running"}))
		st, ok := d.state("web")
		if !ok || st.Label != "running" {
			t.Errorf("state = %+v, %v; want the derived entry to have taken over", st, ok)
		}

		d.observe(statusOf(nil))
		if _, ok := d.state("web"); ok {
			t.Error("the row kept an indicator after the work finished")
		}
	})
}
