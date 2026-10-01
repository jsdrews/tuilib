package table

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func spanRows(from, to int) []KeyedRow {
	var out []KeyedRow
	for i := from; i < to; i++ {
		k := fmt.Sprint(i)
		out = append(out, KeyedRow{Key: k, Cells: []string{"item " + k}})
	}
	return out
}

func newSpanTable(t *testing.T, max int) Model {
	t.Helper()
	m := New(Options{
		Title:      "Apps",
		Columns:    []Column{{Title: "Name", Width: 20}},
		Filterable: true,
		FilterMode: FilterRemote,
		Anchored:   true,
		MaxItems:   max,
	})
	m.SetRect(geom.New(0, 0, 40, 14))
	return m
}

func TestSpanAppendAndPrependKeepTheCursorsRow(t *testing.T) {
	m := newSpanTable(t, 0)
	if !m.Loading() {
		t.Fatal("an empty remote span is Loading")
	}
	m.AppendRows(spanRows(100, 150), Answer{})
	m.SetMore(true, false)
	m.SetCursor(5)
	before, _ := m.SelectedKey()
	m.PrependRows(spanRows(50, 100), Answer{})
	if after, _ := m.SelectedKey(); after != before {
		t.Errorf("cursor moved from %s to %s", before, after)
	}
	if o, n := m.Edges(); o != "50" || n != "149" {
		t.Errorf("Edges = %s,%s", o, n)
	}
	m.SetCursor(0)
	if !strings.Contains(m.body.Title(), "loading older") {
		t.Errorf("at the older edge with more: %q", m.body.Title())
	}
}

func TestSpanTrimsTheFarEnd(t *testing.T) {
	m := newSpanTable(t, 60)
	m.AppendRows(spanRows(0, 50), Answer{})
	m.SetCursor(0)
	m.AppendRows(spanRows(50, 100), Answer{})
	if o, n := m.Edges(); o != "0" || n != "59" {
		t.Errorf("viewing the top, the newest end should be trimmed: %s..%s", o, n)
	}
}

func TestSpanStaleThenFresh(t *testing.T) {
	m := newSpanTable(t, 0)
	m.AppendRows(spanRows(0, 20), Answer{})
	m, _ = commitFilter(m, "x")
	if !m.Stale() {
		t.Fatal("a committed filter makes the span stale")
	}
	m.AppendRows(spanRows(500, 510), Answer{Raw: "x"})
	if m.Stale() || m.rowCount() != 10 {
		t.Errorf("the new answer replaces the span: stale=%v rows=%d", m.Stale(), m.rowCount())
	}
}
