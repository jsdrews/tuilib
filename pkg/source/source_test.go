package source

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/query"
)

// req runs cmd and returns the RequestMsg's Query, or nil when cmd is nil
// or carries something else.
func req(cmd tea.Cmd) *Query {
	if cmd == nil {
		return nil
	}
	for _, msg := range msgs(cmd) {
		if r, ok := msg.(RequestMsg); ok {
			return &r.Query
		}
	}
	return nil
}

// msgs runs cmd, flattening batches.
func msgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, msgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// deliver is Deliver's verdict alone.
func deliver(m *Model, p Page) bool {
	ok, _ := m.Deliver(p)
	return ok
}

func newSrc(t *testing.T, opts Options) Model {
	t.Helper()
	if opts.PageSize == 0 {
		opts.PageSize = 100
	}
	if opts.ViewportDelay == 0 {
		opts.ViewportDelay = -1
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
	if deliver(&m, Page{Gen: first.Gen, Offset: 0, Count: 100, Total: 1000}) {
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
	if deliver(&m, Page{Gen: b.Gen, Offset: 100, Count: 100, Total: 1000}) {
		t.Error("an out-of-order reply overwrote the window the user is actually on")
	}
	if !deliver(&m, Page{Gen: c.Gen, Offset: 400, Count: 100, Total: 1000}) {
		t.Error("the newest reply should be accepted")
	}
	if start, count := m.Held(); start != 400 || count != 100 {
		t.Errorf("Held() = (%d, %d), want (400, 100)", start, count)
	}
}

func TestDeliverUpdatesTotalAndHeld(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	if !deliver(&m, Page{Gen: q.Gen, Offset: 0, Count: 80, Total: 80}) {
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

// ---- Latency: stale scrolling, cancellation, settle delay, history ----

func TestNoWindowRequestsWhileQueryUnanswered(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 5000})
	first := req(m.SetQuery("eu", nil, "", false))
	if got := req(m.Viewport(800, 829)); got != nil {
		t.Fatalf("scrolling stale rows requested %+v; it would supersede the first page", *got)
	}
	if !deliver(&m, Page{Gen: first.Gen, Offset: 0, Count: 12, Total: 12}) {
		t.Error("the new query's first page was superseded by scrolling")
	}
}

func TestFirstAnswerChecksViewportFromTheTop(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.SetQuery("eu", nil, "", false))
	m.Viewport(800, 949) // stale scroll, 150 rows tall
	_, cmd := m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 5000})
	got := req(cmd)
	if got == nil {
		t.Fatal("a first answer that leaves the screen uncovered should ask for the rest")
	}
	if got.Offset != 0 || got.Limit != 200 {
		t.Errorf("q = %+v, want [0,200) — the cursor is back at the top, not at the stale row 800", *got)
	}
}

func TestSupersededRequestIsCancelled(t *testing.T) {
	m := newSrc(t, Options{})
	a := req(m.Init())
	b := req(m.SetQuery("eu", nil, "", false))
	if a.Ctx.Err() == nil {
		t.Error("a new query must cancel the old one's request")
	}
	if b.Ctx.Err() != nil {
		t.Error("the live request was cancelled")
	}
	m.Deliver(Page{Gen: b.Gen, Offset: 0, Count: 100, Total: 1000})
	if b.Ctx.Err() == nil {
		t.Error("a delivered request's context should be released")
	}
	c := req(m.Viewport(150, 170))
	d := req(m.Viewport(450, 470))
	if c.Ctx.Err() == nil || d.Ctx.Err() != nil {
		t.Error("a newer window request must cancel the older one")
	}
}

func TestCancelCancelsAndRefusesLateReplies(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Cancel()
	if q.Ctx.Err() == nil {
		t.Error("Cancel left the request running")
	}
	if deliver(&m, Page{Gen: q.Gen, Err: context.Canceled}) {
		t.Error("a reply to a cancelled request was accepted")
	}
}

func TestParentContextIsInherited(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := newSrc(t, Options{Context: ctx})
	q := req(m.Init())
	cancel()
	if q.Ctx.Err() == nil {
		t.Error("cancelling Options.Context should cancel requests")
	}
}

func TestViewportWaitsForTheDelay(t *testing.T) {
	m := New(Options{PageSize: 100, ViewportDelay: time.Hour})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	if m.Viewport(150, 170) == nil {
		t.Fatal("a scroll off the held window should arm the delay")
	}
	stale := settleMsg{id: m.id, seq: m.vpSeq}
	if m.Viewport(450, 470) == nil {
		t.Fatal("expected the delay re-armed")
	}
	if got := m.Update(stale); got != nil {
		t.Error("an overtaken delay should ask for nothing")
	}
	got := req(m.Update(settleMsg{id: m.id, seq: m.vpSeq}))
	if got == nil || got.Offset != 400 {
		t.Fatalf("settled request = %+v, want offset 400", got)
	}
	if m.Viewport(420, 440) != nil {
		t.Error("a scroll inside what is wanted should arm nothing")
	}
}

func TestDelayMessageBelongsToItsSource(t *testing.T) {
	a := New(Options{})
	b := New(Options{})
	b.Viewport(0, 10)
	if a.Update(settleMsg{id: b.id, seq: b.vpSeq}) != nil {
		t.Error("one source acted on another's timer")
	}
}

func TestDefaultViewportDelay(t *testing.T) {
	if New(Options{}).delay != DefaultViewportDelay {
		t.Error("zero ViewportDelay should mean the default")
	}
	if New(Options{ViewportDelay: -1}).delay != 0 {
		t.Error("negative ViewportDelay should mean immediate")
	}
}

func TestHistoryAnsweredOncePerQuery(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	_, cmd := m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	if n := count[QueryAnsweredMsg](msgs(cmd)); n != 1 {
		t.Fatalf("first answer reported %d times", n)
	}
	w := req(m.Viewport(150, 170))
	_, cmd = m.Deliver(Page{Gen: w.Gen, Offset: 100, Count: 100, Total: 1000})
	if n := count[QueryAnsweredMsg](msgs(cmd)); n != 0 {
		t.Error("a page of an answered query was reported as history")
	}
}

func TestHistoryFailedThenRetried(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	f := req(m.SetQuery("eu", nil, "", false))
	ok, cmd := m.Deliver(Page{Gen: f.Gen, Err: errors.New("503")})
	if !ok {
		t.Fatal("a current failure must be accepted")
	}
	var failed []QueryFailedMsg
	for _, msg := range msgs(cmd) {
		if e, ok := msg.(QueryFailedMsg); ok {
			failed = append(failed, e)
		}
	}
	if len(failed) != 1 || failed[0].Window || failed[0].Query.Raw != "eu" {
		t.Fatalf("failed = %+v, want one query-level failure for eu", failed)
	}
	if m.Total() != -1 {
		t.Error("a failure must not install a window")
	}
	if got := req(m.Viewport(150, 170)); got != nil {
		t.Error("scrolling a failed query's stale rows should not retry it")
	}
	r := req(m.Refresh())
	if r == nil || r.Raw != "eu" {
		t.Fatalf("Refresh = %+v, want a retry of eu", r)
	}
	_, cmd = m.Deliver(Page{Gen: r.Gen, Offset: 0, Count: 5, Total: 5})
	if n := count[QueryAnsweredMsg](msgs(cmd)); n != 1 {
		t.Error("a successful retry should be reported")
	}
}

func TestHistoryWindowFailure(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	w := req(m.Viewport(150, 170))
	_, cmd := m.Deliver(Page{Gen: w.Gen, Err: errors.New("timeout")})
	for _, msg := range msgs(cmd) {
		if e, ok := msg.(QueryFailedMsg); ok && e.Window {
			return
		}
	}
	t.Error("a failed page should be reported, marked as a window failure")
}

func TestHistoryCancelledQuery(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	m.SetQuery("eu", nil, "", false)
	var cancelled []QueryCancelledMsg
	for _, msg := range msgs(m.SetQuery("eus", nil, "", false)) {
		if e, ok := msg.(QueryCancelledMsg); ok {
			cancelled = append(cancelled, e)
		}
	}
	if len(cancelled) != 1 || cancelled[0].Query.Raw != "eu" || cancelled[0].By.Raw != "eus" {
		t.Fatalf("cancelled = %+v, want eu superseded by eus", cancelled)
	}
	if n := count[QueryCancelledMsg](msgs(m.SetQuery("x", nil, "", false))); n != 1 {
		t.Error("each abandoned query is reported")
	}
}

func count[T any](ms []tea.Msg) int {
	n := 0
	for _, m := range ms {
		if _, ok := m.(T); ok {
			n++
		}
	}
	return n
}

// ---- MaxHeld, follow, find ----

func TestMaxHeldMergesAdjacentPages(t *testing.T) {
	m := newSrc(t, Options{MaxHeld: 5000})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	w := req(m.Viewport(150, 170))
	m.Deliver(Page{Gen: w.Gen, Offset: 100, Count: 100, Total: 1000})
	if start, count := m.Held(); start != 0 || count != 200 {
		t.Fatalf("Held = (%d, %d), want the pages merged into [0,200)", start, count)
	}
	if got := req(m.Viewport(20, 40)); got != nil {
		t.Errorf("scrolling back into the range refetched %+v", *got)
	}
	far := req(m.Viewport(800, 820))
	m.Deliver(Page{Gen: far.Gen, Offset: 800, Count: 100, Total: 1000})
	if start, count := m.Held(); start != 800 || count != 100 {
		t.Errorf("Held = (%d, %d), want a far jump to replace the range", start, count)
	}
	m.SetHeld(820, 80)
	if start, count := m.Held(); start != 820 || count != 80 {
		t.Errorf("SetHeld not adopted: (%d, %d)", start, count)
	}
}

func TestMaxHeldZeroKeepsOneWindow(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	w := req(m.Viewport(150, 170))
	m.Deliver(Page{Gen: w.Gen, Offset: 100, Count: 100, Total: 1000})
	if start, count := m.Held(); start != 100 || count != 100 {
		t.Errorf("Held = (%d, %d); a table holds one window", start, count)
	}
}

// pollOf fires the poll timer a command armed. A Tick runs once only, so
// tests that also inspect the command's messages use pollIn instead.
func pollOf(t *testing.T, m *Model, cmd tea.Cmd) *Query {
	t.Helper()
	return pollIn(m, msgs(cmd))
}

func pollIn(m *Model, ms []tea.Msg) *Query {
	for _, msg := range ms {
		if p, ok := msg.(pollMsg); ok {
			return req(m.Update(p))
		}
	}
	return nil
}

func TestFollowPollsTheTailWhileFollowing(t *testing.T) {
	m := newSrc(t, Options{MaxHeld: 5000, Follow: time.Millisecond})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 100})
	m.Viewport(80, 99) // the newest item is on screen: following
	p := pollOf(t, &m, m.SetGrowing(true))
	if p == nil || !p.Poll {
		t.Fatal("a Growing source should poll")
	}
	if p.Offset != 0 || p.Limit != 200 {
		t.Errorf("poll = [%d,+%d), want the tail page again plus what's past it", p.Offset, p.Limit)
	}
	_, cmd := m.Deliver(Page{Gen: p.Gen, Offset: 0, Count: 130, Total: 130})
	if start, count := m.Held(); start != 0 || count != 130 {
		t.Errorf("Held = (%d, %d)", start, count)
	}
	if pollOf(t, &m, cmd) == nil {
		t.Error("the next poll should be armed after a delivery")
	}
}

func TestProbeWhileNotFollowing(t *testing.T) {
	m := newSrc(t, Options{MaxHeld: 5000, Follow: time.Millisecond})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 100})
	m.Viewport(0, 19)
	p := pollOf(t, &m, m.SetGrowing(true))
	if p == nil || p.Offset != 100 || p.Limit != 1 {
		t.Fatalf("probe = %+v, want one item past the end", p)
	}
}

func TestPollWaitsForARequestInFlight(t *testing.T) {
	m := newSrc(t, Options{Follow: time.Millisecond})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	user := req(m.Viewport(150, 170))
	cmd := m.SetGrowing(true)
	if p := pollOf(t, &m, cmd); p != nil {
		t.Errorf("a poll superseded the user's request: %+v", *p)
	}
	if user.Ctx.Err() != nil {
		t.Error("the user's request was cancelled by a poll")
	}
}

func TestPollFailureReportedOnceThenRecovery(t *testing.T) {
	m := newSrc(t, Options{Follow: time.Millisecond})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 10, Total: 10})
	p := pollOf(t, &m, m.SetGrowing(true))
	_, cmd := m.Deliver(Page{Gen: p.Gen, Err: errors.New("down")})
	ms := msgs(cmd)
	if n := count[QueryFailedMsg](ms); n != 1 || !m.PollsFailing() {
		t.Fatalf("first failure reported %d times", n)
	}
	p = pollIn(&m, ms)
	if p == nil {
		t.Fatal("polling should continue through failures")
	}
	_, cmd = m.Deliver(Page{Gen: p.Gen, Err: errors.New("down")})
	ms = msgs(cmd)
	if n := count[QueryFailedMsg](ms); n != 0 {
		t.Error("a continuing outage was reported again")
	}
	p = pollIn(&m, ms)
	_, cmd = m.Deliver(Page{Gen: p.Gen, Offset: 10, Count: 1, Total: 11})
	if n := count[QueryRecoveredMsg](msgs(cmd)); n != 1 || m.PollsFailing() {
		t.Error("the first success should report a recovery")
	}
}

func TestStopGrowingDoesAFinalRead(t *testing.T) {
	m := newSrc(t, Options{Follow: time.Hour})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 10, Total: 10})
	m.SetGrowing(true)
	if f := req(m.SetGrowing(false)); f == nil || !f.Poll {
		t.Error("stopping should read once more for late items")
	}
}

func TestPokePollsNow(t *testing.T) {
	m := newSrc(t, Options{Follow: time.Hour})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 10, Total: 10})
	if m.Poke() != nil {
		t.Error("Poke on data that isn't Growing should do nothing")
	}
	m.SetGrowing(true)
	if p := req(m.Poke()); p == nil || !p.Poll {
		t.Error("Poke should poll immediately")
	}
}

func TestFindRequestsOneMatchAndInstallsNothing(t *testing.T) {
	m := newSrc(t, Options{})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	m.SetQuery("level:error", nil, "", false)
	f := req(m.Find("timeout", Newer, 99))
	if f == nil || !f.Find || f.Term != "timeout" || f.Offset != 99 || f.Limit != 1 || f.Raw != "level:error" {
		t.Fatalf("find = %+v", f)
	}
	ok, _ := m.Deliver(Page{Gen: f.Gen, Found: true, Offset: 612})
	if !ok || m.Total() != -1 {
		t.Error("a find answer is accepted but must not install a window")
	}
}

func TestAViewportRequestDoesNotSupersedeAFind(t *testing.T) {
	m := New(Options{PageSize: 100, ViewportDelay: -1, MaxHeld: 5000})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 1000})
	f := req(m.Find("ok", Newer, 99))
	// The jump that ran out of loaded matches also scrolled the view.
	if got := req(m.Viewport(95, 115)); got != nil {
		t.Errorf("a scroll during a find issued %+v, cancelling the find", *got)
	}
	if f.Ctx.Err() != nil {
		t.Fatal("the find was cancelled")
	}
	ok, cmd := m.Deliver(Page{Gen: f.Gen, Found: true, Offset: 100})
	if !ok {
		t.Fatal("the find's answer must still be accepted")
	}
	if got := req(cmd); got == nil || got.Offset > 100 || got.Offset+got.Limit <= 100 {
		t.Errorf("a found hit should fetch its page at once: %+v", got)
	}
}

func TestAnchoredEdgeRequestDoesNotSupersedeAFind(t *testing.T) {
	m := NewAnchored(AnchoredOptions{PageSize: 100, ViewportDelay: -1})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Count: 100, More: true})
	f := req(m.Find("ok", Older))
	if got := req(m.Viewport(5, 90)); got != nil {
		t.Errorf("an edge request during a find: %+v", *got)
	}
	if f.Ctx.Err() != nil {
		t.Error("the find was cancelled")
	}
}

func TestGrowingDoesNotClampToTheStaleTotal(t *testing.T) {
	m := New(Options{PageSize: 100, ViewportDelay: -1, MaxHeld: 5000, Follow: time.Hour})
	q := req(m.Init())
	m.Deliver(Page{Gen: q.Gen, Offset: 0, Count: 100, Total: 150})
	m.SetGrowing(true)
	got := req(m.Viewport(130, 149))
	if got == nil || got.Offset+got.Limit <= 150 {
		t.Errorf("q = %+v — a growing set may already be past 150; ask for the whole page", got)
	}
}
