// The read path, asserted once across list, table and tree: ApplyRead drops a
// failed read, says so through ReadsFailing, and turns work the last good read
// reported busy Unknown rather than clearing it or leaving it animated.
package componenttest

import (
	"errors"
	"testing"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// reader is one component driven through BeginRead / ApplyRead.
type reader interface {
	read(status map[string]string, err error) bool
	failing() bool
	state(key string) (activity.State, bool)
}

type listReader struct{ m list.Model }

func (r *listReader) read(status map[string]string, err error) bool {
	items := make([]list.KeyedItem, 0, len(derivedKeys))
	for _, k := range derivedKeys {
		items = append(items, list.KeyedItem{Key: k, Display: k + " " + status[k]})
	}
	return r.m.ApplyRead(r.m.BeginRead(), items, err)
}
func (r *listReader) failing() bool                         { return r.m.ReadsFailing() }
func (r *listReader) state(k string) (activity.State, bool) { return r.m.ActivityState().State(k) }

type tableReader struct{ m table.Model }

func (r *tableReader) read(status map[string]string, err error) bool {
	rows := make([]table.KeyedRow, 0, len(derivedKeys))
	for _, k := range derivedKeys {
		rows = append(rows, table.KeyedRow{Key: k, Cells: []string{k, status[k]}})
	}
	return r.m.ApplyRead(r.m.BeginRead(), rows, err)
}
func (r *tableReader) failing() bool                         { return r.m.ReadsFailing() }
func (r *tableReader) state(k string) (activity.State, bool) { return r.m.ActivityState().State(k) }

type treeReader struct{ m tree.Model }

func (r *treeReader) read(status map[string]string, err error) bool {
	return r.m.ApplyRead(r.m.BeginRead(), derivedTree(status), err)
}
func (r *treeReader) failing() bool { return r.m.ReadsFailing() }
func (r *treeReader) state(k string) (activity.State, bool) {
	return r.m.ActivityState().State("cluster/" + k)
}

func eachReader(t *testing.T, fn func(t *testing.T, r reader)) {
	t.Helper()
	for name, mk := range map[string]func() reader{
		"list": func() reader {
			o := theme.Dark().List()
			o.BusyWhen = listBusyWhen
			return &listReader{m: list.New(o)}
		},
		"table": func() reader {
			o := theme.Dark().Table()
			o.Columns = []table.Column{{Title: "Name", Width: 16}, {Title: "Status", Width: 14}}
			o.ActivityColumn = "Status"
			o.BusyWhen = func(r table.KeyedRow) (string, bool) {
				return activity.Busy("running", "pending")(r.Cells[1])
			}
			return &tableReader{m: table.New(o)}
		},
		"tree": func() reader {
			o := theme.Dark().Tree()
			o.InitialDepth = 2
			o.Root = derivedTree(statusOf(nil))
			o.BusyWhen = treeBusyWhen
			return &treeReader{m: tree.New(o)}
		},
	} {
		t.Run(name, func(t *testing.T) { fn(t, mk()) })
	}
}

func TestAFailedReadMakesObservedWorkUnknownOnEveryComponent(t *testing.T) {
	eachReader(t, func(t *testing.T, r reader) {
		if !r.read(statusOf(map[string]string{"web": "running"}), nil) {
			t.Fatal("a good read was refused")
		}
		if r.read(nil, errors.New("503")) {
			t.Fatal("a failed read was applied")
		}
		if !r.failing() {
			t.Error("ReadsFailing is false after a failed read")
		}
		st, ok := r.state("web")
		if !ok || !st.Unknown {
			t.Errorf("web = %+v %v, want kept and Unknown", st, ok)
		}
		r.read(statusOf(map[string]string{"web": "running"}), nil)
		if st, _ := r.state("web"); st.Unknown || r.failing() {
			t.Error("a good read did not restore the row")
		}
	})
}
