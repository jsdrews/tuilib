package remote

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/table"
)

func TestRefusedWraps(t *testing.T) {
	base := errors.New("HTTP 400: cannot order by owner")
	err := Refused(base)
	if !errors.Is(err, ErrRefused) || !errors.Is(err, base) {
		t.Error("errors.Is does not reach both the mark and the cause")
	}
	if err.Error() != base.Error() {
		t.Errorf("message %q, want the cause's own", err.Error())
	}
	if Refused(nil) != nil {
		t.Error("Refused(nil) is not nil")
	}
	if errors.Is(base, ErrRefused) {
		t.Error("an unmarked error reads as refused")
	}
}

// books is a source that refuses to sort by Owner, and fails outright for
// the filter "down".
type books struct {
	mu   sync.Mutex
	asks []Window
}

func (b *books) page(_ context.Context, w Window) ([]table.KeyedRow, int, error) {
	b.mu.Lock()
	b.asks = append(b.asks, w)
	b.mu.Unlock()
	switch {
	case w.Sort == "Owner":
		return nil, 0, Refused(errors.New("HTTP 400: order_by owner not allowed"))
	case w.Raw == "down":
		return nil, 0, errors.New("HTTP 503")
	}
	var rows []table.KeyedRow
	for i := w.Offset; i < min(50, w.Offset+w.Limit); i++ {
		rows = append(rows, table.KeyedRow{Key: fmt.Sprint(i), Cells: []string{fmt.Sprint("owner-", i), fmt.Sprint("book-", i)}})
	}
	return rows, 50, nil
}

func (b *books) sorts() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, w := range b.asks {
		out = append(out, w.Sort)
	}
	return out
}

// recorder keeps every message the pump delivers.
type recorder struct{ msgs []tea.Msg }

func (r *recorder) Update(msg tea.Msg) tea.Cmd { r.msgs = append(r.msgs, msg); return nil }

func (r *recorder) refusals() []table.SortRefusedMsg {
	var out []table.SortRefusedMsg
	for _, m := range r.msgs {
		if s, ok := m.(table.SortRefusedMsg); ok {
			out = append(out, s)
		}
	}
	return out
}

// press sends keys to tb, pumping what follows through tb and rec.
func press(t *testing.T, tb *Table, rec *recorder, ks ...string) {
	t.Helper()
	for _, k := range ks {
		pump(t, tb.Update(key(k)), tb, rec)
	}
}

func bookOpts() table.Options {
	return table.Options{
		Title:        "books",
		Filterable:   true,
		SortDebounce: -1,
		Columns: []table.Column{
			{Title: "Owner", Width: 10, Sortable: true},
			{Title: "Title", Width: 10, Sortable: true},
		},
	}
}

func newBooks(t *testing.T) (*books, *Table, *recorder) {
	t.Helper()
	b := &books{}
	tb := NewTable(bookOpts(), Seekable[table.KeyedRow]{Page: b.page, ViewportDelay: -1, Follow: -1})
	tb.SetRect(geom.New(0, 0, 50, 14))
	return b, tb, &recorder{}
}

func TestRefusedSortGoesBackWithoutARequest(t *testing.T) {
	b, tb, rec := newBooks(t)
	pump(t, tb.Init(), tb, rec)
	press(t, tb, rec, "]") // Owner — refused

	if got := strings.Join(b.sorts(), ","); got != ",Owner" {
		t.Errorf("requests %q, want the load and the refused sort, then nothing", got)
	}
	rs := rec.refusals()
	if len(rs) != 1 || rs[0].Column != 0 || rs[0].Title != "Owner" || !errors.Is(rs[0].Err, ErrRefused) {
		t.Fatalf("SortRefusedMsg = %+v", rs)
	}
	if tb.SortColumn() != -1 || tb.Stale() || tb.Failed() {
		t.Errorf("sort %d stale %v failed %v; want back on the unsorted answer, current", tb.SortColumn(), tb.Stale(), tb.Failed())
	}
	if !tb.SortUnavailable(0) {
		t.Error("Owner still available")
	}
	if row, ok := tb.Selected(); !ok || row[0] != "owner-0" {
		t.Errorf("rows lost: %v %v", row, ok)
	}
	if !strings.Contains(tb.View(), "\x1b[38;5;240mOwner") {
		t.Error("Owner's header title is not dimmed")
	}

	press(t, tb, rec, "]") // skips Owner, lands on Title
	if tb.SortColumn() != 1 {
		t.Errorf("] went to column %d, want 1", tb.SortColumn())
	}
}

func TestRefusedSortStaysUnavailableAcrossRestyle(t *testing.T) {
	_, tb, rec := newBooks(t)
	pump(t, tb.Init(), tb, rec)
	press(t, tb, rec, "]")
	tb.Restyle(bookOpts())
	if !tb.SortUnavailable(0) {
		t.Error("Restyle forgot the refused column")
	}
}

func TestRefusedFirstLoadRetriesUnsortedOnce(t *testing.T) {
	b, tb, rec := newBooks(t)
	tb.SetSort(0, false) // start sorted by Owner
	pump(t, tb.Init(), tb, rec)

	if got := strings.Join(b.sorts(), ","); got != "Owner," {
		t.Errorf("requests %q, want Owner then one unsorted retry", got)
	}
	if len(rec.refusals()) != 1 {
		t.Errorf("refusals = %+v", rec.refusals())
	}
	if row, ok := tb.Selected(); !ok || row[0] != "owner-0" || tb.SortColumn() != -1 {
		t.Errorf("first load did not recover unsorted: %v %v sort %d", row, ok, tb.SortColumn())
	}
}

func TestOrdinaryFailureIsNotARefusal(t *testing.T) {
	_, tb, rec := newBooks(t)
	pump(t, tb.Init(), tb, rec)
	press(t, tb, rec, "/", "d", "o", "w", "n", "enter")
	if len(rec.refusals()) != 0 || !tb.Failed() {
		t.Errorf("a 503 was handled as a refusal: refusals %v failed %v", rec.refusals(), tb.Failed())
	}
}

// A refusal of a query that changed more than the sort is a failure: going
// back would undo the user's filter too.
func TestRefusalBeyondTheSortIsAFailure(t *testing.T) {
	b := &books{}
	page := func(ctx context.Context, w Window) ([]table.KeyedRow, int, error) {
		if w.Raw == "x" {
			return nil, 0, Refused(errors.New("HTTP 422"))
		}
		return b.page(ctx, w)
	}
	tb := NewTable(bookOpts(), Seekable[table.KeyedRow]{Page: page, ViewportDelay: -1, Follow: -1})
	tb.SetRect(geom.New(0, 0, 50, 14))
	rec := &recorder{}
	pump(t, tb.Init(), tb, rec)
	press(t, tb, rec, "/", "x", "enter")
	if len(rec.refusals()) != 0 || !tb.Failed() {
		t.Errorf("refused filter: refusals %v failed %v", rec.refusals(), tb.Failed())
	}
}
