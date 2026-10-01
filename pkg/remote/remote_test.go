package remote

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/table"
)

// updater is either bound view.
type updater interface{ Update(tea.Msg) tea.Cmd }

// pump runs cmd and everything it leads to through the views, the way the
// app would. Timers (spinner, settle, poll) are left sleeping — see
// cmdtest — so the pump settles on the fetches and routing alone.
func pump(t *testing.T, cmd tea.Cmd, views ...updater) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			t.Fatal("did not settle")
		}
		c := queue[0]
		queue = queue[1:]
		for _, msg := range cmdtest.Run(c) {
			for _, v := range views {
				queue = append(queue, v.Update(msg))
			}
		}
	}
}

// ---- fakes ----------------------------------------------------------------

type fakeJob struct {
	mu      sync.Mutex
	n       int
	windows []Window
	finds   []Find
}

func (f *fakeJob) text(i int) string {
	if i%10 == 0 {
		return fmt.Sprintf("fatal: event %d", i)
	}
	return fmt.Sprintf("ok: event %d", i)
}

func (f *fakeJob) matches(raw string, i int) bool {
	return raw == "" || strings.Contains(f.text(i), raw)
}

func (f *fakeJob) ids(raw string) []int {
	var out []int
	for i := 0; i < f.n; i++ {
		if f.matches(raw, i) {
			out = append(out, i)
		}
	}
	return out
}

func (f *fakeJob) page(_ context.Context, w Window) ([]eventlog.Item, int, error) {
	f.mu.Lock()
	f.windows = append(f.windows, w)
	f.mu.Unlock()
	ids := f.ids(w.Raw)
	var items []eventlog.Item
	for j := w.Offset; j < min(len(ids), w.Offset+w.Limit); j++ {
		items = append(items, eventlog.Item{Key: strconv.Itoa(ids[j]), Lines: []string{f.text(ids[j])}})
	}
	return items, len(ids), nil
}

func (f *fakeJob) find(_ context.Context, q Find) (int, bool, error) {
	f.mu.Lock()
	f.finds = append(f.finds, q)
	f.mu.Unlock()
	ids := f.ids(q.Raw)
	step := -1
	if q.Newer {
		step = 1
	}
	for j := q.From + step; j >= 0 && j < len(ids); j += step {
		if strings.Contains(f.text(ids[j]), q.Term) {
			return j, true, nil
		}
	}
	return 0, false, nil
}

func (f *fakeJob) edge(_ context.Context, e Edge) ([]eventlog.Item, bool, error) {
	pos := f.n
	if e.Newer {
		pos = -1
	}
	if e.Cursor != "" {
		pos, _ = strconv.Atoi(e.Cursor)
	}
	var from, to int
	if e.Newer {
		from, to = pos+1, min(f.n, pos+1+e.Limit)
	} else {
		to = pos
		if e.Inclusive {
			to++
		}
		from = max(0, to-e.Limit)
	}
	var items []eventlog.Item
	for i := from; i < to; i++ {
		items = append(items, eventlog.Item{Key: strconv.Itoa(i), Lines: []string{f.text(i)}})
	}
	more := from > 0
	if e.Newer {
		more = to < f.n
	}
	return items, more, nil
}

func (f *fakeJob) findKey(_ context.Context, q Find) (string, bool, error) {
	pos, _ := strconv.Atoi(q.Cursor)
	step := -1
	if q.Newer {
		step = 1
	}
	for i := pos + step; i >= 0 && i < f.n; i += step {
		if strings.Contains(f.text(i), q.Term) {
			return strconv.Itoa(i), true, nil
		}
	}
	return "", false, nil
}

func newSeekLog(t *testing.T, job *fakeJob) *Eventlog {
	t.Helper()
	l := NewEventlog(eventlog.Options{Title: "job", Filterable: true, Searchable: true, NewFor: -1},
		Seekable[eventlog.Item]{Page: job.page, Find: job.find, ViewportDelay: -1, Follow: -1})
	l.SetRect(geom.New(0, 0, 60, 14))
	return l
}

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keys(t *testing.T, v updater, ks ...string) {
	t.Helper()
	for _, k := range ks {
		pump(t, v.Update(key(k)), v)
	}
}

// ---- tests ----------------------------------------------------------------

func TestSeekableEventlogLoadsFollowsAndPages(t *testing.T) {
	job := &fakeJob{n: 1000}
	l := newSeekLog(t, job)
	pump(t, l.Init(), l)
	if l.Loading() || l.Cursor() != 999 {
		t.Fatalf("after Init: loading %v cursor %d — the view should land on the newest item", l.Loading(), l.Cursor())
	}
	if _, ok := l.Selected(); !ok {
		t.Fatal("the newest item should be loaded")
	}
	keys(t, l, "g")
	if it, ok := l.Selected(); !ok || it.Key != "0" {
		t.Errorf("g should load and land on the first item: %+v %v", it, ok)
	}
}

func TestFilterReachesTheFetch(t *testing.T) {
	job := &fakeJob{n: 1000}
	l := newSeekLog(t, job)
	pump(t, l.Init(), l)
	keys(t, l, "f", "f", "a", "t", "a", "l", "enter")
	last := job.windows[len(job.windows)-1]
	if last.Raw != "fatal" {
		t.Fatalf("last window = %+v, want the committed filter", last)
	}
	if l.Stale() {
		t.Error("the answer should have landed")
	}
	if it, _ := l.Selected(); !strings.HasPrefix(it.Lines[0], "fatal") {
		t.Errorf("selected %+v", it)
	}
}

func TestRemoteFindLandsOnTheHit(t *testing.T) {
	job := &fakeJob{n: 1000}
	l := newSeekLog(t, job)
	pump(t, l.Init(), l)
	keys(t, l, "g", "/", "e", "v", "e", "n", "t", " ", "7", "7", "7", "enter", "n")
	if len(job.finds) == 0 {
		t.Fatal("a match beyond what is loaded should be asked for")
	}
	it, ok := l.Selected()
	if !ok || it.Key != "777" {
		t.Errorf("selected %+v %v, want event 777", it, ok)
	}
}

func TestAnchoredEventlog(t *testing.T) {
	job := &fakeJob{n: 1000}
	l := NewEventlog(eventlog.Options{Title: "logs", Searchable: true, NewFor: -1},
		Anchored[eventlog.Item]{Edge: job.edge, Find: job.findKey, PageSize: 100, ViewportDelay: -1, Follow: -1})
	l.SetRect(geom.New(0, 0, 60, 14))
	pump(t, l.Init(), l)
	if it, ok := l.Selected(); !ok || it.Key != "999" {
		t.Fatalf("a Newest anchor lands on the newest item: %+v %v", it, ok)
	}
	keys(t, l, "g")
	if o, _ := l.Edges(); o == "900" {
		t.Error("reaching the older edge should have extended it")
	}
	keys(t, l, "/", "e", "v", "e", "n", "t", " ", "1", "2", "3", "enter", "N")
	if it, ok := l.Selected(); !ok || it.Key != "123" {
		t.Errorf("a find past the span should re-anchor on the hit: %+v %v", it, ok)
	}
}

func TestTwoViewsDoNotAnswerEachOther(t *testing.T) {
	a, b := &fakeJob{n: 50}, &fakeJob{n: 70}
	la, lb := newSeekLog(t, a), newSeekLog(t, b)
	pump(t, tea.Batch(la.Init(), lb.Init()), la, lb)
	if la.Cursor() != 49 || lb.Cursor() != 69 {
		t.Errorf("cursors %d, %d — each view must take only its own pages", la.Cursor(), lb.Cursor())
	}
}

func TestRestyleKeepsState(t *testing.T) {
	job := &fakeJob{n: 1000}
	l := newSeekLog(t, job)
	pump(t, l.Init(), l)
	keys(t, l, "g", "j", "j")
	before, _ := l.Selected()
	l.Restyle(eventlog.Options{Title: "job", Filterable: true, Searchable: true, NewFor: -1})
	l.SetRect(geom.New(0, 0, 60, 14))
	if after, ok := l.Selected(); !ok || after.Key != before.Key {
		t.Errorf("restyle moved the cursor from %s to %+v", before.Key, after)
	}
}

func TestSeekableTable(t *testing.T) {
	var got []Window
	var mu sync.Mutex
	page := func(_ context.Context, w Window) ([]table.KeyedRow, int, error) {
		mu.Lock()
		got = append(got, w)
		mu.Unlock()
		var rows []table.KeyedRow
		for i := w.Offset; i < min(500, w.Offset+w.Limit); i++ {
			rows = append(rows, table.KeyedRow{Key: strconv.Itoa(i), Cells: []string{fmt.Sprint("app-", i), "eu"}})
		}
		return rows, 500, nil
	}
	tb := NewTable(table.Options{
		Title:      "apps",
		Filterable: true,
		Columns:    []table.Column{{Title: "Name", Width: 12, Sortable: true}, {Title: "Region", Width: 8}},
	}, Seekable[table.KeyedRow]{Page: page, ViewportDelay: -1, Follow: -1})
	tb.SetRect(geom.New(0, 0, 40, 14))
	pump(t, tb.Init(), tb)
	if row, ok := tb.Selected(); !ok || row[0] != "app-0" {
		t.Fatalf("first row = %v %v", row, ok)
	}
	keys(t, tb, "G")
	if row, ok := tb.Selected(); !ok || row[0] != "app-499" {
		t.Errorf("G should page to the last row: %v %v", row, ok)
	}
	keys(t, tb, "/", "e", "u", "enter")
	if last := got[len(got)-1]; last.Raw != "eu" || last.Offset != 0 {
		t.Errorf("filter should refetch from the top: %+v", last)
	}
}
