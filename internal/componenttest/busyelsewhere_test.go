// Busy-ness that is not a field on the row, asserted once across list, table
// and tree.
//
// An operations API, a job status resource, GET /jobs?status=running: the
// screen folds it into each row's data and the predicate reads it, so there is
// still one entrance and one map. The contract that matters is that a key the
// component does not hold is neither drawn nor counted, and lights up when its
// row arrives.
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

// Busy-ness from another endpoint rides in each row's data and the predicate
// reads it: one entrance on every component. The screen holds the map — as
// these adapters do — which is what keeps a key for a row not yet held.
type listBusy struct {
	m    list.Model
	keys []string
	busy map[string]string
}

func newListBusy() busyable {
	o := theme.Dark().List()
	o.BusyWhen = func(it list.KeyedItem) (string, bool) {
		label, _ := it.Data.(string)
		return label, label != ""
	}
	b := &listBusy{m: list.New(o)}
	b.hold(derivedKeys)
	return b
}

func (b *listBusy) hold(keys []string) {
	b.keys = keys
	b.push()
}
func (b *listBusy) setBusy(busy map[string]string) {
	b.busy = busy
	b.push()
}
func (b *listBusy) push() {
	items := make([]list.KeyedItem, 0, len(b.keys))
	for _, k := range b.keys {
		items = append(items, list.KeyedItem{Key: k, Display: k, Data: b.busy[k]})
	}
	b.m.SetKeyedItems(items)
	b.m.SetRect(placed())
}
func (b *listBusy) state(k string) (activity.State, bool) { return b.m.ActivityState().State(k) }
func (b *listBusy) count() int                            { return b.m.ActivityCount() }

// The table's version of the same adapter: Data on each row, BusyWhen reading it. The screen holds the map — as
// this adapter does — which is what keeps a key for a row not yet held.
type tableBusy struct {
	m    table.Model
	keys []string
	busy map[string]string
}

func newTableBusy() busyable {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	o.BusyWhen = func(r table.KeyedRow) (string, bool) {
		label, _ := r.Data.(string)
		return label, label != ""
	}
	b := &tableBusy{m: table.New(o)}
	b.hold(derivedKeys)
	return b
}

func (b *tableBusy) hold(keys []string) {
	b.keys = keys
	b.push()
}
func (b *tableBusy) setBusy(busy map[string]string) {
	b.busy = busy
	b.push()
}
func (b *tableBusy) push() {
	rows := make([]table.KeyedRow, 0, len(b.keys))
	for _, k := range b.keys {
		rows = append(rows, table.KeyedRow{Key: k, Cells: []string{k, "successful"}, Data: b.busy[k]})
	}
	b.m.SetKeyedRows(rows)
	b.m.SetRect(placed())
}
func (b *tableBusy) state(k string) (activity.State, bool) { return b.m.ActivityState().State(k) }
func (b *tableBusy) count() int                            { return b.m.ActivityCount() }

type treeBusy struct {
	m    tree.Model
	keys []string
	busy map[string]string
}

func newTreeBusy() busyable {
	o := theme.Dark().Tree()
	o.InitialDepth = 2
	o.Root = busyTree(derivedKeys, nil)
	o.BusyWhen = func(n tree.Node) (string, bool) {
		sn, _ := n.(statusNode)
		return sn.status, sn.status != ""
	}
	b := &treeBusy{m: tree.New(o), keys: derivedKeys}
	b.m.SetRect(placed())
	return b
}

func busyTree(keys []string, busy map[string]string) tree.Node {
	kids := make([]tree.Node, 0, len(keys))
	for _, k := range keys {
		kids = append(kids, statusNode{label: k, status: busy[k]})
	}
	return statusNode{label: "cluster", children: kids}
}

func (b *treeBusy) hold(keys []string) {
	b.keys = keys
	b.m.SetRoot(busyTree(b.keys, b.busy))
	b.m.SetRect(placed())
}
func (b *treeBusy) setBusy(busy map[string]string) {
	b.busy = busy
	b.hold(b.keys)
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
func TestBusyFromElsewhereDrivesTheIndicator(t *testing.T) {
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
func TestBusyFromElsewhereReplacesTheMapWholesale(t *testing.T) {
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
func TestBusyFromElsewhereKeysTheComponentDoesNotHoldAreNotCounted(t *testing.T) {
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
