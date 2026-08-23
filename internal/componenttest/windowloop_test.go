package componenttest

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/source"
	"github.com/jsdrews/tuilib/pkg/table"
)

// The windowed-source loop spans two packages that deliberately don't know
// about each other: the table reports which logical rows are on screen, the
// coordinator decides what to fetch, and installing the result makes the
// table report again. Each half is unit-tested in its own package; what
// neither can prove alone is that the cycle *converges* — that a delivered
// window satisfies the viewport that asked for it instead of provoking
// another request forever.
//
// These tests drive both halves against a fake source and count requests.

type fakeSource struct {
	total    int
	requests []source.Query
}

func (f *fakeSource) rows(offset, limit int) []table.Row {
	end := offset + limit
	if f.total >= 0 && end > f.total {
		end = f.total
	}
	if offset >= end {
		return nil
	}
	out := make([]table.Row, 0, end-offset)
	for i := offset; i < end; i++ {
		out = append(out, table.Row{rowName(i), "europe"})
	}
	return out
}

func rowName(i int) string {
	digits := ""
	n := i
	if n == 0 {
		digits = "0"
	}
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return "row-" + digits
}

// rig wires a table to a coordinator the way a screen would, and pumps
// messages until everything settles.
type rig struct {
	t     *testing.T
	tbl   table.Model
	src   source.Model
	fake  *fakeSource
	steps int
}

func newRig(t *testing.T, total, pageSize, height int) *rig {
	t.Helper()
	tbl := table.New(table.Options{
		Columns: []table.Column{
			{Title: "Name", Width: 12, Sortable: true},
			{Title: "Region", Width: 8},
		},
		FilterMode: table.FilterRemote,
		SortMode:   table.SortRemote,
		Filterable: true,
	})
	tbl.SetRect(geom.New(0, 0, 40, height))
	return &rig{
		t:    t,
		tbl:  tbl,
		src:  source.New(source.Options{PageSize: pageSize}),
		fake: &fakeSource{total: total},
	}
}

// pump runs cmd and every message it produces to completion, routing each
// one the way a screen's Update would. It fails the test rather than
// spinning if the loop doesn't settle.
func (r *rig) pump(cmd tea.Cmd) {
	r.t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		r.steps++
		if r.steps > 200 {
			r.t.Fatalf("loop did not converge after %d steps; requests so far: %d",
				r.steps, len(r.fake.requests))
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		queue = append(queue, r.route(msg)...)
	}
}

func (r *rig) route(msg tea.Msg) []tea.Cmd {
	switch m := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Cmd
		for _, sub := range m {
			out = append(out, sub)
		}
		return out

	case source.RequestMsg:
		r.fake.requests = append(r.fake.requests, m.Query)
		rows := r.fake.rows(m.Query.Offset, m.Query.Limit)
		page := source.Page{
			Gen:    m.Query.Gen,
			Offset: m.Query.Offset,
			Count:  len(rows),
			Total:  r.fake.total,
		}
		if !r.src.Deliver(page) {
			return nil
		}
		r.tbl.SetWindow(rows, page.Offset, page.Total)
		return nil

	case table.ViewportChangedMsg:
		return []tea.Cmd{r.src.Viewport(m.FirstVisible, m.LastVisible)}

	case table.QueryChangedMsg:
		return []tea.Cmd{r.src.SetQuery(m.Raw, m.Terms, m.Sort, m.Desc)}
	}
	return nil
}

// key drives a keypress through the table and pumps whatever it produces.
func (r *rig) key(k tea.KeyMsg) {
	r.t.Helper()
	var cmd tea.Cmd
	r.tbl, cmd = r.tbl.Update(k)
	r.pump(cmd)
}

// typeKey drives a key without pumping the resulting cmd. Focusing a
// filter and typing into it return the textinput's cursor-blink cmd, which
// is a tea.Tick that really sleeps — and pumping it would add half a second
// per keystroke for no coverage. Neither focusing nor typing emits anything
// the coordinator reacts to (pkg/table proves that separately: filters
// report on commit), so dropping those cmds loses nothing.
func (r *rig) typeKey(k tea.KeyMsg) {
	r.t.Helper()
	r.tbl, _ = r.tbl.Update(k)
}

func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestWindowLoopConvergesOnInit(t *testing.T) {
	r := newRig(t, 1000, 100, 14)
	r.pump(r.src.Init())
	if n := len(r.fake.requests); n != 1 {
		t.Errorf("initial load made %d requests, want exactly 1", n)
	}
	if row, ok := r.tbl.Selected(); !ok || row[0] != "row-0" {
		t.Errorf("Selected() = %v, %v after the first window landed", row, ok)
	}
}

func TestWindowLoopConvergesOnJumpToBottom(t *testing.T) {
	r := newRig(t, 1000, 100, 14)
	r.pump(r.src.Init())
	before := len(r.fake.requests)

	r.key(runes("G"))

	if r.tbl.Cursor() != 999 {
		t.Fatalf("cursor = %d, want 999", r.tbl.Cursor())
	}
	row, ok := r.tbl.Selected()
	if !ok {
		t.Fatal("the last row should be resident once the loop settles")
	}
	if row[0] != "row-999" {
		t.Errorf("Selected() = %v, want row-999", row)
	}
	if n := len(r.fake.requests) - before; n != 1 {
		t.Errorf("jumping to the bottom made %d requests, want 1", n)
	}
}

func TestWindowLoopDoesNotRefetchWithinAPage(t *testing.T) {
	r := newRig(t, 1000, 100, 14)
	r.pump(r.src.Init())
	before := len(r.fake.requests)
	for i := 0; i < 20; i++ {
		r.key(runes("j"))
	}
	if n := len(r.fake.requests) - before; n != 0 {
		t.Errorf("scrolling inside the held page made %d requests, want 0", n)
	}
}

func TestWindowLoopHandlesShortFinalPage(t *testing.T) {
	// 250 rows with a page size of 100: the last window is 50 short.
	r := newRig(t, 250, 100, 14)
	r.pump(r.src.Init())
	r.key(runes("G"))
	row, ok := r.tbl.Selected()
	if !ok || row[0] != "row-249" {
		t.Errorf("Selected() = %v, %v, want the final row of a short page", row, ok)
	}
}

func TestWindowLoopConvergesWhenTotalIsSmallerThanAPage(t *testing.T) {
	r := newRig(t, 7, 100, 14)
	r.pump(r.src.Init())
	if got := r.tbl.Cursor(); got != 0 {
		t.Errorf("cursor = %d", got)
	}
	r.key(runes("G"))
	row, ok := r.tbl.Selected()
	if !ok || row[0] != "row-6" {
		t.Errorf("Selected() = %v, %v", row, ok)
	}
}

func TestWindowLoopEmptyResultConverges(t *testing.T) {
	r := newRig(t, 0, 100, 14)
	r.pump(r.src.Init())
	if _, ok := r.tbl.Selected(); ok {
		t.Error("an empty result should select nothing")
	}
	if n := len(r.fake.requests); n != 1 {
		t.Errorf("an empty result made %d requests, want 1 — a source with no rows must not be polled in a loop", n)
	}
}

func TestWindowLoopFilterRefetchesFromTheTop(t *testing.T) {
	r := newRig(t, 1000, 100, 14)
	r.pump(r.src.Init())
	r.key(runes("G")) // scroll away from the top
	before := len(r.fake.requests)

	r.typeKey(runes("/"))
	for _, c := range "region:europe" {
		r.typeKey(runes(string(c)))
	}
	r.key(tea.KeyMsg{Type: tea.KeyEnter})

	reqs := r.fake.requests[before:]
	if len(reqs) == 0 {
		t.Fatal("committing a filter fetched nothing")
	}
	first := reqs[0]
	if first.Offset != 0 {
		t.Errorf("first request after a filter had offset %d, want 0", first.Offset)
	}
	if first.Raw != "region:europe" {
		t.Errorf("request carried Raw = %q", first.Raw)
	}
	if len(first.Terms) != 1 || first.Terms[0].Title != "Region" {
		t.Errorf("request carried terms %+v, want the resolved column", first.Terms)
	}
}

func TestWindowLoopSortRefetches(t *testing.T) {
	r := newRig(t, 1000, 100, 14)
	r.pump(r.src.Init())
	before := len(r.fake.requests)
	r.key(runes("]"))
	reqs := r.fake.requests[before:]
	if len(reqs) == 0 {
		t.Fatal("requesting a sort fetched nothing")
	}
	if reqs[0].Sort != "Name" {
		t.Errorf("request carried Sort = %q, want Name", reqs[0].Sort)
	}
}
