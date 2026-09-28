package table

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/geom"
)

type revApp struct{ phase, rev string }

func revTable(withRevision bool) Model {
	o := Options{
		Columns:        []Column{{Title: "Name", Width: 10}, {Title: "Sync", Width: 14}},
		ActivityColumn: "Sync",
		BusyWhen: func(r KeyedRow) (string, bool) {
			p := r.Data.(revApp).phase
			return p, p == "Running"
		},
	}
	o.Activity.Settle = 1
	if withRevision {
		o.Revision = func(r KeyedRow) string { return r.Data.(revApp).rev }
	}
	m := New(o)
	m.SetRect(geom.New(0, 0, 40, 8))
	return m
}

func rowsAt(phase, rev string) []KeyedRow {
	return []KeyedRow{{Key: "a", Cells: []string{"a", "Synced"}, Data: revApp{phase, rev}}}
}

func unobservedIn(cmd tea.Cmd) []activity.UnobservedMsg {
	if cmd == nil {
		return nil
	}
	var out []activity.UnobservedMsg
	switch m := cmd().(type) {
	case activity.UnobservedMsg:
		out = append(out, m)
	case tea.BatchMsg:
		for _, c := range m {
			out = append(out, unobservedIn(c)...)
		}
	}
	return out
}

// A sync that runs and finishes between two reads, Succeeded → Succeeded. The
// status cannot say anything happened; the revision can, on the first read.
func TestRevisionEndsASamePhaseSyncOnTheFirstRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		revision bool
		ended    bool
	}{
		{"with a revision", true, true},
		{"without one, the allowance is spent instead", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := revTable(tc.revision)
			m.ApplyRead(m.BeginRead(), rowsAt("Succeeded", "1"), nil)

			op, _ := m.Dispatch([]string{"a"}, "Running", activity.Observed)
			m.Done(op, nil)
			m.ApplyRead(m.BeginRead(), rowsAt("Succeeded", "2"), nil)
			_, cmd := m.Update(struct{}{})

			_, busy := m.ActivityState().State("a")
			if busy == tc.ended {
				t.Errorf("still busy = %v, want %v", busy, !tc.ended)
			}
			got := unobservedIn(cmd)
			if tc.ended && (len(got) != 1 || !got[0].Changed) {
				t.Errorf("reports = %+v, want one with Changed", got)
			}
		})
	}
}

// ReadsFailing is the table's view of a failed ApplyRead.
func TestReadsFailingFollowsApplyRead(t *testing.T) {
	m := revTable(false)
	m.ApplyRead(m.BeginRead(), rowsAt("Running", "1"), nil)
	m.ApplyRead(m.BeginRead(), nil, errTest)
	if !m.ReadsFailing() {
		t.Fatal("ReadsFailing false after a failed read")
	}
	if st, _ := m.ActivityState().State("a"); !st.Unknown {
		t.Error("the row busy on the last good read is not Unknown")
	}
	m.ApplyRead(m.BeginRead(), rowsAt("Running", "1"), nil)
	if m.ReadsFailing() {
		t.Error("a good read did not clear ReadsFailing")
	}
}

type testErr struct{}

func (testErr) Error() string { return "503" }

var errTest = testErr{}

// KeyedRows hands back what was applied, Data included, for a SetTheme rebuild.
func TestKeyedRowsRoundTrips(t *testing.T) {
	m := revTable(true)
	m.ApplyRead(m.BeginRead(), rowsAt("Running", "3"), nil)
	got := m.KeyedRows()
	if len(got) != 1 || got[0].Key != "a" || got[0].Data.(revApp).rev != "3" {
		t.Fatalf("KeyedRows = %+v", got)
	}
	m.SetRows([]Row{{"x", "y"}})
	if m.KeyedRows() != nil {
		t.Error("anonymous rows reported as keyed")
	}
}
