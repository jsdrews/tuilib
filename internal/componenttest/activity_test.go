// The derived contract, asserted once across list, table and tree.
//
// Shared behaviour belongs here rather than in whichever component was edited
// last: the click-to-blur behaviour was written for pkg/list, rolled out to
// five other components by a script that omitted it, and covered by a test
// that lived in pkg/list — so five components shipped broken and the suite
// stayed green. marking_test.go is the precedent; this is the same shape.
package componenttest

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// derivable is one component configured for activity, reduced to what the
// contract is about: put data in, look at what comes out.
type derivable interface {
	setRect(geom.Rect)
	view() string
	// observe swaps in data where each named key carries that status, and
	// every other key carries "successful".
	observe(status map[string]string)
	// pending drains whatever the setter queued, as Update would.
	pending() tea.Cmd
	state(key string) (activity.State, bool)
	count() int
	// rebuilt is a fresh instance, for the theme-swap carry.
	rebuilt() derivable
	adopt(activity.Set) tea.Cmd
	activityState() activity.Set
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
func (d *listDerive) state(k string) (activity.State, bool) { return d.m.ActivityState().State(k) }
func (d *listDerive) count() int                            { return d.m.ActivityCount() }
func (d *listDerive) rebuilt() derivable                    { return newListDerive() }
func (d *listDerive) adopt(s activity.Set) tea.Cmd          { return d.m.SetActivityState(s) }
func (d *listDerive) activityState() activity.Set           { return d.m.ActivityState() }

type tableDerive struct{ m table.Model }

func newTableDerive() derivable {
	o := theme.Dark().Table()
	o.ActivityColumn = "Status"
	o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
	o.ActivityWhen = func(c table.Row) (string, bool) {
		return activity.Busy("running", "pending")(c[1])
	}
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
func (d *tableDerive) state(k string) (activity.State, bool) { return d.m.ActivityState().State(k) }
func (d *tableDerive) count() int                            { return d.m.ActivityCount() }
func (d *tableDerive) rebuilt() derivable                    { return newTableDerive() }
func (d *tableDerive) adopt(s activity.Set) tea.Cmd          { return d.m.SetActivityState(s) }
func (d *tableDerive) activityState() activity.Set           { return d.m.ActivityState() }

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
	// Root at construction, so New's preExpand opens it — SetRoot alone leaves
	// a fresh tree collapsed, and a spinner on a hidden child proves nothing.
	o.Root = derivedTree(statusOf(nil))
	o.ActivityWhen = func(n tree.Node) (string, bool) {
		sn, ok := n.(statusNode)
		if !ok {
			return "", false
		}
		return activity.Busy("running", "pending")(sn.status)
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
func (d *treeDerive) path(k string) string { return "cluster/" + k }
func (d *treeDerive) state(k string) (activity.State, bool) {
	return d.m.ActivityState().State(d.path(k))
}
func (d *treeDerive) count() int                   { return d.m.ActivityCount() }
func (d *treeDerive) rebuilt() derivable           { return newTreeDerive() }
func (d *treeDerive) adopt(s activity.Set) tea.Cmd { return d.m.SetActivityState(s) }
func (d *treeDerive) activityState() activity.Set  { return d.m.ActivityState() }

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

// The whole feature: data arrives, a row matches, it spins. No setter call
// anywhere, no action, no shell.
func TestAPolledStatusDrivesTheIndicator(t *testing.T) {
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

func TestARestingStatusStopsTheIndicator(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(map[string]string{"web": "running"}))
		d.observe(statusOf(nil))

		if d.count() != 0 {
			t.Errorf("count = %d, want 0 once the data says it is resting", d.count())
		}
		if strings.Contains(d.view(), "running") {
			t.Errorf("the indicator outlived the status it was reporting:\n%s", d.view())
		}
	})
}

// An observation is the whole truth, so one row settling must not disturb
// another that is still working.
func TestRowsAreIndependent(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(map[string]string{"api": "running", "web": "pending"}))
		if d.count() != 2 {
			t.Fatalf("count = %d, want 2", d.count())
		}

		d.observe(statusOf(map[string]string{"api": "running"}))
		if _, ok := d.state("api"); !ok {
			t.Error("a row still working lost its indicator")
		}
		if _, ok := d.state("web"); ok {
			t.Error("a row that settled kept its indicator")
		}
	})
}

// The label is the server's own word, carried through unchanged — there is no
// mapping to keep in step, which is the reason nothing can drift.
func TestTheLabelIsTheDataSOwnWord(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(map[string]string{"web": "pending"}))

		st, ok := d.state("web")
		if !ok || st.Label != "pending" {
			t.Errorf("state = %+v, %v; want the status as the data spelled it", st, ok)
		}
	})
}

// Rule 4: a theme swap rebuilds the component, and the indicator has to
// survive it or every spinner blanks until the next poll.
func TestActivitySurvivesAThemeRebuild(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(map[string]string{"web": "running"}))

		fresh := d.rebuilt()
		cmd := fresh.adopt(d.activityState())
		fresh.setRect(placed())

		if _, ok := fresh.state("web"); !ok {
			t.Errorf("the rebuilt component lost the indicator:\n%s", fresh.view())
		}
		if cmd == nil {
			t.Error("SetActivityState re-armed no tick, so the carried row sits frozen")
		}
	})
}

// An idle component must schedule nothing, or every screen in an app pays for
// a feature it is not using.
func TestAnIdleComponentSchedulesNothing(t *testing.T) {
	eachDerivable(t, func(t *testing.T, d derivable) {
		d.observe(statusOf(nil))
		d.pending()

		if cmd := d.pending(); cmd != nil {
			t.Error("a component with nothing busy queued a command")
		}
	})
}
