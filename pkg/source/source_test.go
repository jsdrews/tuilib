package source

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/query"
)

// req runs cmd and returns the RequestMsg's Query, or nil when cmd is nil
// or carries something else.
func req(cmd tea.Cmd) *Query {
	if cmd == nil {
		return nil
	}
	if r, ok := cmd().(RequestMsg); ok {
		return &r.Query
	}
	return nil
}

func newSrc(t *testing.T, opts Options) Model {
	t.Helper()
	if opts.PageSize == 0 {
		opts.PageSize = 100
	}
	return New(opts)
}

func TestInitRequestsFirstPage(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	if q == nil {
		t.Fatal("Init emitted no request")
	}
	if q.Offset != 0 || q.Limit != 100 {
		t.Errorf("q = %+v, want offset 0 limit 100", *q)
	}
	if !m.Pending() {
		t.Error("Pending should be true with a request outstanding")
	}
}

func TestDefaultPageSizeApplied(t *testing.T) {
	m := New(Options{})
	if m.PageSize() != DefaultPageSize {
		t.Errorf("PageSize = %d, want %d", m.PageSize(), DefaultPageSize)
	}
}

func TestViewportInsideHeldAsksNothing(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	if got := req(m.Viewport(10, 39)); got != nil {
		t.Errorf("rows already held triggered %+v", *got)
	}
}

func TestViewportOutsideHeldRequestsAlignedWindow(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	got := req(m.Viewport(150, 170))
	if got == nil {
		t.Fatal("scrolling outside the held window emitted no request")
	}
	if got.Offset != 100 || got.Limit != 100 {
		t.Errorf("q = %+v, want the page-aligned window [100,200)", *got)
	}
}

func TestWindowSpansMultiplePagesWhenViewportDoes(t *testing.T) {
	m := newSrc(t, Options{})
	m.Init()
	m.Viewport(90, 210)
	got := req(m.Refresh())
	if got.Offset != 0 || got.Limit != 300 {
		t.Errorf("q = %+v, want [0,300) to cover rows 90..210", *got)
	}
}

func TestPrefetchExtendsWindow(t *testing.T) {
	m := newSrc(t, Options{Prefetch: 1})
	m.Init()
	got := req(m.Viewport(0, 29))
	if got == nil {
		t.Fatal("expected a request")
	}
	if got.Limit != 200 {
		t.Errorf("limit = %d, want 200 (one screen plus one prefetched page)", got.Limit)
	}
}

func TestDuplicateInFlightRequestSuppressed(t *testing.T) {
	m := newSrc(t, Options{})
	m.Init()
	// Same window, still outstanding: scrolling a row must not re-ask.
	if got := req(m.Viewport(0, 29)); got != nil {
		t.Errorf("re-requested an outstanding window: %+v", *got)
	}
	if got := req(m.Viewport(1, 30)); got != nil {
		t.Errorf("re-requested an outstanding window: %+v", *got)
	}
}

func TestStaleDeliveryRefused(t *testing.T) {
	m := newSrc(t, Options{})
	first := req(m.Init())
	second := req(m.SetQuery("oslo", nil, "", false))
	if first.Gen == second.Gen {
		t.Fatal("a new query must carry a new generation")
	}
	if m.Deliver(Page{Gen: first.Gen, Offset: 0, Count: 100, Total: 1000}) {
		t.Error("a reply to the superseded query was accepted")
	}
	if !m.Pending() {
		t.Error("refusing a stale reply must leave the live request outstanding")
	}
	if m.Total() != -1 {
		t.Errorf("Total = %d — a refused page must not update state", m.Total())
	}
}

func TestOnlyNewestRequestAccepted(t *testing.T) {
	m := newSrc(t, Options{})
	a := req(m.Init())
	m.Deliver(Page{Gen: a.Gen, Offset: 0, Count: 100, Total: 1000})
	b := req(m.Viewport(150, 170))
	c := req(m.Viewport(450, 470))
	if b == nil || c == nil {
		t.Fatal("expected two window requests")
	}
	if m.Deliver(Page{Gen: b.Gen, Offset: 100, Count: 100, Total: 1000}) {
		t.Error("an out-of-order reply overwrote the window the user is actually on")
	}
	if !m.Deliver(Page{Gen: c.Gen, Offset: 400, Count: 100, Total: 1000}) {
		t.Error("the newest reply should be accepted")
	}
	if start, count := m.Held(); start != 400 || count != 100 {
		t.Errorf("Held() = (%d, %d), want (400, 100)", start, count)
	}
}

func TestDeliverUpdatesTotalAndHeld(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	if !m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 80, Total: 80}) {
		t.Fatal("current-generation page refused")
	}
	if m.Pending() {
		t.Error("Pending should clear once the page lands")
	}
	if m.Total() != 80 {
		t.Errorf("Total = %d, want 80", m.Total())
	}
	if start, count := m.Held(); start != 0 || count != 80 {
		t.Errorf("Held() = (%d, %d)", start, count)
	}
}

func TestSetQueryResetsWindowAndCarriesTerms(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	m.Viewport(150, 170)

	terms := query.Parse("region:europe", []string{"Name", "Region"})
	got := req(m.SetQuery("region:europe", terms, "Name", true))
	if got == nil {
		t.Fatal("SetQuery emitted no request")
	}
	if got.Offset != 0 {
		t.Errorf("offset = %d, want 0 — a new query starts at the top", got.Offset)
	}
	if got.Raw != "region:europe" || len(got.Terms) != 1 {
		t.Errorf("q = %+v, want the filter carried through", *got)
	}
	if got.Terms[0].Title != "Region" {
		t.Errorf("term = %+v, want the resolved column title", got.Terms[0])
	}
	if got.Sort != "Name" || !got.Desc {
		t.Errorf("q = %+v, want the sort carried through", *got)
	}
	if m.Total() != -1 {
		t.Errorf("Total = %d, want -1 — the previous total describes a different set", m.Total())
	}
	if start, count := m.Held(); start != 0 || count != 0 {
		t.Errorf("Held() = (%d, %d), want the window discarded", start, count)
	}
}

func TestSetQueryAlwaysRequests(t *testing.T) {
	m := newSrc(t, Options{})
	m.Init()
	if req(m.SetQuery("", nil, "", false)) == nil {
		t.Error("SetQuery must always fetch, even when the query is unchanged")
	}
}

func TestEmptyPageNotRequestedTwice(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 0, Total: -1})
	if got := req(m.Viewport(0, 29)); got != nil {
		t.Errorf("re-requested a window the source answered with nothing: %+v", *got)
	}
}

func TestEmptyGuardClearedByRefresh(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 0, Total: -1})
	m.Viewport(0, 29)
	if req(m.Refresh()) == nil {
		t.Error("Refresh should retry a window that previously came back empty")
	}
}

func TestEmptyGuardClearedByNewQuery(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 0, Total: -1})
	m.Viewport(0, 29)
	q2 := req(m.SetQuery("x", nil, "", false))
	m.Deliver(Page{Gen: q2.Gen, Offset: 0, Count: 0, Total: -1})
	// A different query may legitimately have rows at the same window.
	if got := req(m.SetQuery("y", nil, "", false)); got == nil {
		t.Error("a new query should request regardless of the previous empty result")
	}
}

func TestRefreshKeepsQueryAndWindow(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	m.SetQuery("oslo", nil, "Name", false)
	m.Viewport(150, 170)
	got := req(m.Refresh())
	if got == nil {
		t.Fatal("Refresh emitted no request")
	}
	if got.Raw != "oslo" || got.Sort != "Name" {
		t.Errorf("q = %+v, want the query preserved", *got)
	}
	if got.Offset != 100 {
		t.Errorf("offset = %d, want the window on screen, not the top", got.Offset)
	}
}

func TestLimitClampedToTotal(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 150})
	got := req(m.Viewport(100, 120))
	if got == nil {
		t.Fatal("expected a request")
	}
	if got.Limit != 50 {
		t.Errorf("limit = %d, want 50 — never ask past a known total", got.Limit)
	}
}

func TestViewportBeforeAnyDataStillRequests(t *testing.T) {
	m := newSrc(t, Options{})
	if got := req(m.Viewport(0, 29)); got == nil {
		t.Error("a viewport with nothing held should request")
	}
}

// ---- ByCursor ----

func TestCursorInitHasEmptyCursor(t *testing.T) {
	m := newSrc(t, Options{Mode: ByCursor})
	q := req(m.Init())
	if q.Cursor != "" {
		t.Errorf("first cursor request carried %q, want empty", q.Cursor)
	}
}

func TestCursorAdvancesAndAccumulates(t *testing.T) {
	m := newSrc(t, Options{Mode: ByCursor})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 100, Total: -1, Next: "tok-1"})
	if _, count := m.Held(); count != 100 {
		t.Fatalf("held count = %d, want 100", count)
	}
	// Still well inside what has loaded.
	if got := req(m.Viewport(0, 29)); got != nil {
		t.Errorf("mid-window scroll asked for more: %+v", *got)
	}
	got := req(m.Viewport(70, 99))
	if got == nil {
		t.Fatal("reaching the end of the loaded rows should ask for more")
	}
	if got.Cursor != "tok-1" {
		t.Errorf("cursor = %q, want the token from the previous page", got.Cursor)
	}
	m.Deliver(Page{Gen: got.Gen, Count: 100, Total: -1, Next: "tok-2"})
	if _, count := m.Held(); count != 200 {
		t.Errorf("held count = %d, want 200 — cursor pages accumulate", count)
	}
}

func TestCursorExhaustionSetsTotalAndStops(t *testing.T) {
	m := newSrc(t, Options{Mode: ByCursor})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 40, Total: -1, Next: ""})
	if !m.Exhausted() {
		t.Error("an empty Next means the walk is over")
	}
	if m.Total() != 40 {
		t.Errorf("Total = %d, want 40 — exhaustion is what establishes it", m.Total())
	}
	if got := req(m.Viewport(20, 39)); got != nil {
		t.Errorf("asked for more after exhaustion: %+v", *got)
	}
}

func TestCursorRefreshRestartsWalk(t *testing.T) {
	m := newSrc(t, Options{Mode: ByCursor})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 100, Total: -1, Next: "tok-1"})
	got := req(m.Refresh())
	if got == nil {
		t.Fatal("Refresh emitted no request")
	}
	if got.Cursor != "" {
		t.Errorf("cursor = %q, want a restart — a cursor walk can't resume mid-way", got.Cursor)
	}
	if _, count := m.Held(); count != 0 {
		t.Errorf("held count = %d, want 0 after a restart", count)
	}
}

func TestCursorPrefetchAsksEarly(t *testing.T) {
	m := newSrc(t, Options{Mode: ByCursor, PageSize: 100, Prefetch: 1})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 200, Total: -1, Next: "tok-1"})
	// With one page of margin, row 99 is already inside the trigger zone.
	if got := req(m.Viewport(70, 99)); got == nil {
		t.Error("prefetch should ask a page ahead of the end")
	}
}

func TestExhaustedFalseUnderByOffset(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 10, Total: 10})
	if m.Exhausted() {
		t.Error("Exhausted is a ByCursor concept; the total says it under ByOffset")
	}
}
