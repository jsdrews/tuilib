// The second entrance, asserted once across list, table and tree.
//
// SetBusy is for busy-ness that is not a field on the row — an operations API,
// a job status resource, GET /jobs?status=running. The contract that matters is
// that it is still one map with one writer, and that a key naming a row the
// component does not hold is kept without being drawn, counted or animated.
package componenttest

import (
	"testing"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// busyable is one component told which of its rows are working.
type busyable interface {
	// hold replaces the data, so the component holds exactly these keys.
	hold(keys []string)
	setBusy(busy map[string]string)
	state(key string) (activity.State, bool)
	count() int
}

type listBusy struct{ m list.Model }

func newListBusy() busyable {
	o := theme.Dark().List()
	b := &listBusy{m: list.New(o)}
	b.hold(derivedKeys)
	return b
}

func (b *listBusy) hold(keys []string) {
	items := make([]list.KeyedItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, list.KeyedItem{Key: k, Display: k})
	}
	b.m.SetKeyedItems(items)
	b.m.SetRect(placed())
}
func (b *listBusy) setBusy(busy map[string]string)        { b.m.SetBusy(busy) }
func (b *listBusy) state(k string) (activity.State, bool) { return b.m.ActivityState().State(k) }
func (b *listBusy) count() int                            { return b.m.ActivityCount() }

type tableBusy struct{ m table.Model }

func newTableBusy() busyable {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	b := &tableBusy{m: table.New(o)}
	b.hold(derivedKeys)
	return b
}

func (b *tableBusy) hold(keys []string) {
	rows := make([]table.KeyedRow, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, table.KeyedRow{Key: k, Cells: []string{k, "successful"}})
	}
	b.m.SetKeyedRows(rows)
	b.m.SetRect(placed())
}
func (b *tableBusy) setBusy(busy map[string]string)        { b.m.SetBusy(busy) }
func (b *tableBusy) state(k string) (activity.State, bool) { return b.m.ActivityState().State(k) }
func (b *tableBusy) count() int                            { return b.m.ActivityCount() }

type treeBusy struct{ m tree.Model }

func newTreeBusy() busyable {
	o := theme.Dark().Tree()
	o.InitialDepth = 2
	o.Root = busyTree(derivedKeys)
	b := &treeBusy{m: tree.New(o)}
	b.m.SetRect(placed())
	return b
}

func busyTree(keys []string) tree.Node {
	kids := make([]tree.Node, 0, len(keys))
	for _, k := range keys {
		kids = append(kids, statusNode{label: k})
	}
	return statusNode{label: "cluster", children: kids}
}

func (b *treeBusy) hold(keys []string) {
	b.m.SetRoot(busyTree(keys))
	b.m.SetRect(placed())
}

// A tree's keys are paths; translate at the boundary, as treeDerive does.
func (b *treeBusy) setBusy(busy map[string]string) {
	paths := make(map[string]string, len(busy))
	for k, label := range busy {
		paths["cluster/"+k] = label
	}
	b.m.SetBusy(paths)
}
func (b *treeBusy) state(k string) (activity.State, bool) {
	return b.m.ActivityState().State("cluster/" + k)
}
func (b *treeBusy) count() int { return b.m.ActivityCount() }

func eachBusyable(t *testing.T, fn func(t *testing.T, b busyable)) {
	t.Helper()
	for name, mk := range map[string]func() busyable{
		"list":  newListBusy,
		"table": newTableBusy,
		"tree":  newTreeBusy,
	} {
		t.Run(name, func(t *testing.T) { fn(t, mk()) })
	}
}

// The entrance itself: no predicate anywhere, and the rows still spin.
func TestSetBusyDrivesTheIndicator(t *testing.T) {
	eachBusyable(t, func(t *testing.T, b busyable) {
		b.setBusy(map[string]string{"web": "Syncing"})

		st, ok := b.state("web")
		if !ok {
			t.Fatal("the row the screen reported busy is not working")
		}
		if st.Label != "Syncing" {
			t.Errorf("label = %q, want %q", st.Label, "Syncing")
		}
		if _, ok := b.state("api"); ok {
			t.Error("a row nobody reported busy is working")
		}
	})
}

// One map, one writer — the same property the predicate path has. An
// observation is the whole truth as of that moment, so the previous one goes.
func TestSetBusyReplacesTheMapWholesale(t *testing.T) {
	eachBusyable(t, func(t *testing.T, b busyable) {
		b.setBusy(map[string]string{"web": "Syncing", "api": "Syncing"})
		b.setBusy(map[string]string{"api": "Syncing"})

		if _, ok := b.state("web"); ok {
			t.Error("a key absent from the newest observation is still working")
		}
		if _, ok := b.state("api"); !ok {
			t.Error("a key present in the newest observation stopped working")
		}
	})
}

// The invariant the predicate held for free: every entry names a row on
// screen. A map from somewhere else need not, and an entry nothing can see
// must not animate a spinner or inflate a count a screen might put in a title.
func TestSetBusyKeysTheComponentDoesNotHoldAreNotCounted(t *testing.T) {
	eachBusyable(t, func(t *testing.T, b busyable) {
		b.setBusy(map[string]string{"web": "Syncing", "elsewhere": "Syncing"})

		if got := b.count(); got != 1 {
			t.Errorf("count = %d, want 1 — only one of the two is a row here", got)
		}
		if _, ok := b.state("elsewhere"); ok {
			t.Error("a key the component does not hold is being rendered")
		}
	})
}

// Kept, not discarded. Rows and busy-ness arrive on separate cadences, so a key
// the component does not hold *yet* is the ordinary case on a paged table —
// dropping it on arrival would leave that row inert when it scrolls into view.
func TestABusyKeyOutsideTheDataSurvivesUntilItsRowArrives(t *testing.T) {
	eachBusyable(t, func(t *testing.T, b busyable) {
		b.setBusy(map[string]string{"later": "Syncing"})
		if _, ok := b.state("later"); ok {
			t.Fatal("a key with no row is visible before its row exists")
		}

		b.hold(append(append([]string{}, derivedKeys...), "later"))
		if _, ok := b.state("later"); !ok {
			t.Error("the key was discarded on arrival, so the row is inert now it is here")
		}
	})
}

// Two writers for one map is the property that makes this feature unable to
// contradict itself, so the mistake is refused rather than raced. Loudly: a
// silent no-op produces a component whose indicators never appear, which is
// the worst of the three outcomes to debug.
func TestSetBusyRefusesAComponentThatHasAPredicate(t *testing.T) {
	for name, call := range map[string]func(){
		"list":  func() { c := newListClaim(0).(*listClaim); c.m.SetBusy(nil) },
		"table": func() { c := newTableClaim(0).(*tableClaim); c.m.SetBusy(nil) },
		"tree":  func() { c := newTreeClaim(0).(*treeClaim); c.m.SetBusy(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("SetBusy on a component built with ActivityWhen was allowed")
				}
			}()
			call()
		})
	}
}
