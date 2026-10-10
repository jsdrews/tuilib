package eventlog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/internal/tick"
	"github.com/jsdrews/tuilib/pkg/geom"
)

func items(from, to int) []Item {
	var out []Item
	for i := from; i < to; i++ {
		k := fmt.Sprint(i)
		out = append(out, Item{Key: k, Lines: []string{"event " + k}, Data: i})
	}
	return out
}

func newLog(t *testing.T, opts Options) Model {
	t.Helper()
	tick.Instant(t) // debounces, spinners and fades cost nothing here
	opts.Title = "Events"
	m := New(opts)
	m.SetRect(geom.New(0, 0, 60, 14))
	return m
}

// run executes cmd once, flattening batches, without waiting on timers.
func run(cmd tea.Cmd) []tea.Msg { return cmdtest.Run(cmd) }

func find[T any](ms []tea.Msg) (T, bool) {
	for _, m := range ms {
		if v, ok := m.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(m Model, keys ...string) (Model, []tea.Msg) {
	var all []tea.Msg
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = m.Update(keyMsg(k))
		all = append(all, run(cmd)...)
	}
	return m, all
}

func TestLoadingUntilTheFirstAnswer(t *testing.T) {
	m := newLog(t, Options{})
	if !m.Loading() {
		t.Fatal("no answer yet: the body should be Loading")
	}
	m.SetPage(items(0, 100), 0, 1000, Answer{})
	if m.Loading() {
		t.Error("the first answer should end Loading")
	}
	if !m.Following() || m.Cursor() != 999 {
		t.Errorf("a fresh answer while following lands on the newest item: cursor %d", m.Cursor())
	}
}

func TestSeekablePagesMergeAndReportHeld(t *testing.T) {
	m := newLog(t, Options{})
	m.SetPage(items(0, 100), 0, 1000, Answer{})
	m.SetCursor(0)
	m.SetPage(items(100, 200), 100, 1000, Answer{})
	start, held := m.Items()
	if start != 0 || len(held) != 200 {
		t.Fatalf("held %d from %d, want the pages merged", len(held), start)
	}
	if m.vp.HeldStart != 0 || m.vp.HeldCount != 200 {
		t.Errorf("viewport msg = %+v", m.vp)
	}
}

func TestFollowAndUnseenCount(t *testing.T) {
	m := newLog(t, Options{})
	m.SetGrowing(true)
	m.SetPage(items(0, 50), 0, 50, Answer{})
	if !strings.Contains(m.body.View(), "FOLLOWING") {
		t.Fatalf("growing and on the newest: FOLLOWING\n%s", m.body.View())
	}
	m, _ = press(m, "k")
	if m.Following() {
		t.Fatal("moving up should leave follow")
	}
	m.SetPage(items(50, 57), 50, 57, Answer{})
	if m.Cursor() != 48 {
		t.Errorf("cursor = %d — the view must not move by itself", m.Cursor())
	}
	if !strings.Contains(m.body.View(), "↓ 7 new") {
		t.Errorf("want the new items counted\n%s", m.body.View())
	}
	m, _ = press(m, "G")
	if !m.Following() || m.Cursor() != 56 {
		t.Errorf("G should return to the newest and follow: cursor %d", m.Cursor())
	}
}

func TestMultiLineAndEmptyItems(t *testing.T) {
	m := newLog(t, Options{})
	m.SetPage([]Item{
		{Key: "1", Lines: []string{"TASK [deploy]", "ok: [web-1]", "ok: [web-2]"}},
		{Key: "2"}, // no output
		{Key: "3", Lines: []string{"PLAY RECAP"}},
	}, 0, 3, Answer{})
	m.SetCursor(0)
	view := m.body.View()
	if !strings.Contains(view, "ok: [web-2]") || !strings.Contains(view, "PLAY RECAP") {
		t.Fatalf("items should draw as their lines:\n%s", view)
	}
	if !strings.Contains(view, "2 │ · no output") {
		t.Errorf("an item with no output still gets a row:\n%s", view)
	}
	m, _ = press(m, "j")
	if m.Cursor() != 1 {
		t.Errorf("cursor = %d, want the no-output item — it can be opened", m.Cursor())
	}
}

func TestStaleItemsAndSuffix(t *testing.T) {
	m := newLog(t, Options{Filterable: true})
	m.SetPage(items(0, 20), 0, 20, Answer{})
	m, ms := press(m, "f", "e", "r", "r", "enter")
	q, ok := find[QueryChangedMsg](ms)
	if !ok || q.Raw != "err" {
		t.Fatalf("filter commit = %+v, %v", q, ok)
	}
	if !m.Stale() || m.Loading() {
		t.Fatal("a committed filter makes the items stale, not Loading")
	}
	if !strings.Contains(m.body.View(), "loading filter err") {
		t.Errorf("border should name the query loading\n%s", m.body.View())
	}
	m.SetPage(items(0, 3), 0, 3, Answer{Raw: "err"})
	if m.Stale() {
		t.Error("the committed query's answer clears staleness")
	}
}

func TestFailedFilterRetriesOnEnter(t *testing.T) {
	m := newLog(t, Options{Filterable: true})
	m.SetPage(items(0, 20), 0, 20, Answer{})
	m.SetGrowing(false)
	m.SetCursor(0)
	m, _ = press(m, "f", "x", "enter")
	m.SetFailed(Answer{Raw: "x"})
	if !m.Failed() || !strings.Contains(m.body.View(), "✗ failed filter x") {
		t.Fatalf("failed = %v\n%s", m.Failed(), m.body.View())
	}
	m, ms := press(m, "f", "enter")
	if q, ok := find[QueryChangedMsg](ms); !ok || q.Raw != "x" {
		t.Error("enter on a failed filter should retry it")
	}
}

func TestPollFailureWhileFollowing(t *testing.T) {
	m := newLog(t, Options{})
	m.SetGrowing(true)
	m.SetPage(items(0, 10), 0, 10, Answer{})
	m.SetFailed(Answer{})
	if m.Failed() || !strings.Contains(m.body.View(), "polls failing") {
		t.Fatalf("a failure while following is a failed poll\n%s", m.body.View())
	}
	m.SetPage(items(10, 11), 10, 11, Answer{})
	if strings.Contains(m.body.View(), "polls failing") {
		t.Error("the next success clears it")
	}
}

func TestSearchJumpsLocallyThenAsksTheSource(t *testing.T) {
	m := newLog(t, Options{Searchable: true})
	page := items(0, 100)
	page[10].Lines = []string{"timeout talking to db"}
	page[40].Lines = []string{"another TIMEOUT"}
	m.SetPage(page, 0, 1000, Answer{})
	m.SetCursor(0)
	m, _ = press(m, "/", "t", "i", "m", "e", "o", "u", "t", "enter")
	m, _ = press(m, "n")
	if m.Cursor() != 10 {
		t.Fatalf("cursor = %d, want the first resident match", m.Cursor())
	}
	m, _ = press(m, "n")
	if m.Cursor() != 40 {
		t.Fatalf("cursor = %d, want the second", m.Cursor())
	}
	if !strings.Contains(m.body.View(), "match 2 of 2+ here") {
		t.Errorf("want a resident count\n%s", m.body.View())
	}
	m, ms := press(m, "n")
	f, ok := find[FindMsg](ms)
	if !ok || f.Term != "timeout" || !f.Newer || f.From != 99 {
		t.Fatalf("find = %+v, %v — past what is held, ask the source", f, ok)
	}
	if !strings.Contains(m.body.View(), "searching newer") {
		t.Errorf("border should say it's searching\n%s", m.body.View())
	}
	m.Found(true, 612)
	if m.Cursor() == 612 {
		t.Error("the cursor should not land on a hit whose line isn't loaded")
	}
	if !strings.Contains(m.body.View(), "searching newer") {
		t.Errorf("until the hit's page arrives, it is still searching\n%s", m.body.View())
	}
	m.SetPage(items(600, 700), 600, 1000, Answer{})
	if m.Cursor() != 612 || m.Following() {
		t.Errorf("once its page is here the cursor lands on the hit: %d", m.Cursor())
	}
	if strings.Contains(m.body.View(), "searching") {
		t.Error("landing ends the search")
	}
}

func TestNoMoreMatches(t *testing.T) {
	m := newLog(t, Options{Searchable: true})
	m.SetPage(items(0, 5), 0, 5, Answer{})
	m.SetCursor(0)
	m, _ = press(m, "/", "z", "enter", "n")
	if !strings.Contains(m.body.View(), "no more matches ↓") {
		t.Errorf("want no-more with nothing beyond\n%s", m.body.View())
	}
}

func TestSpanAppendPrependKeepsCursorAndEdges(t *testing.T) {
	m := newLog(t, Options{Anchored: true})
	m.SetPage(nil, 0, 0, Answer{}) // no-op for spans; ignore
	m.Append(items(100, 150), Answer{})
	m.SetMore(true, false)
	m.SetCursor(10)
	key, _ := m.Selected()
	m.Prepend(items(50, 100), Answer{})
	if got, _ := m.Selected(); got.Key != key.Key {
		t.Errorf("prepend moved the cursor off %s onto %s", key.Key, got.Key)
	}
	if o, n := m.Edges(); o != "50" || n != "149" {
		t.Errorf("Edges = %s, %s", o, n)
	}
	m.SetCursor(0)
	if !strings.Contains(m.body.View(), "loading older") {
		t.Errorf("at an older edge with more, say so\n%s", m.body.View())
	}
}

func TestFoundAtLandsWhenTheItemArrives(t *testing.T) {
	m := newLog(t, Options{Anchored: true, Searchable: true})
	m.Append(items(0, 20), Answer{})
	m.FoundAt(true, "k-7")
	m.Append([]Item{{Key: "k-6", Lines: []string{"a"}}, {Key: "k-7", Lines: []string{"hit"}}}, Answer{})
	if it, _ := m.Selected(); it.Key != "k-7" {
		t.Errorf("selected %q, want the match", it.Key)
	}
}

func TestSpanNotFollowingLandsOnTheFirstItem(t *testing.T) {
	m := newLog(t, Options{Anchored: true})
	m.SetFollow(false)
	m.Append(items(0, 20), Answer{})
	if it, _ := m.Selected(); it.Key != "0" || m.Following() {
		t.Errorf("selected %q following %v, want the first item", it.Key, m.Following())
	}
}

func TestTrimAtMaxItems(t *testing.T) {
	m := newLog(t, Options{Anchored: true, MaxItems: 30})
	m.Append(items(0, 20), Answer{})
	m.Append(items(20, 40), Answer{})
	if _, held := m.Items(); len(held) != 30 {
		t.Errorf("held %d, want the cap", len(held))
	}
	if o, _ := m.Edges(); o != "10" {
		t.Errorf("following the newest end should trim the oldest: edge %s", o)
	}
}

func TestActivate(t *testing.T) {
	m := newLog(t, Options{})
	m.SetPage(items(0, 5), 0, 5, Answer{})
	_, ms := press(m, "enter")
	a, ok := find[ActivatedMsg](ms)
	if !ok || a.Key != "4" || a.Data != 4 {
		t.Errorf("activate = %+v, %v", a, ok)
	}
	if !m.IsActivate(a) {
		t.Error("IsActivate should match its own message")
	}
}

func TestSpanAnswerLandsAtTheNewestAnchor(t *testing.T) {
	m := newLog(t, Options{Anchored: true, Filterable: true})
	m.Prepend(items(100, 200), Answer{})
	m, _ = press(m, "g")
	m, _ = press(m, "f", "x", "enter")
	m.Prepend(items(0, 50), Answer{Raw: "x"})
	if !m.Following() || m.Cursor() != 49 {
		t.Errorf("a Newest answer should land on the newest item, following: cursor %d following %v", m.Cursor(), m.Following())
	}
}

func TestGutterShowsPositionsOnSeekable(t *testing.T) {
	m := newLog(t, Options{})
	m.SetPage([]Item{
		{Key: "a", Lines: []string{"first", "  continued"}},
		{Key: "b"},
		{Key: "c", Lines: []string{"third"}},
	}, 0, 3, Answer{})
	m.SetCursor(0)
	view := m.body.View()
	for _, want := range []string{"1▌│ first", "  │   continued", "2 │ · no output", "3 │ third"} {
		if !strings.Contains(view, want) {
			t.Errorf("gutter missing %q:\n%s", want, view)
		}
	}
}

func TestGutterShowsMarksAndNothingElseOnSpans(t *testing.T) {
	m := newLog(t, Options{Anchored: true})
	m.Append([]Item{{Key: "x", Lines: []string{"no mark"}}}, Answer{})
	if strings.Contains(m.body.View(), "│ no mark") {
		t.Error("a span has no positions: without marks, no gutter")
	}
	m.Append([]Item{{Key: "y", Mark: "12:04:31.600", Lines: []string{"marked"}}}, Answer{})
	if !strings.Contains(m.body.View(), "12:04:31.600▌│ marked") {
		t.Errorf("the item's mark should label it:\n%s", m.body.View())
	}
}

func TestNoGutter(t *testing.T) {
	m := newLog(t, Options{NoGutter: true})
	m.SetPage(items(0, 3), 0, 3, Answer{})
	if strings.Contains(m.body.View(), "│ event") {
		t.Error("NoGutter should hide it")
	}
}

func TestNWhileFindingSendsNothing(t *testing.T) {
	m := newLog(t, Options{Searchable: true})
	m.SetPage(items(0, 100), 0, 1000, Answer{})
	m.SetCursor(0)
	m, _ = press(m, "/", "z", "enter")
	m, ms := press(m, "n")
	if _, ok := find[FindMsg](ms); !ok {
		t.Fatal("the first n past what is held should ask the source")
	}
	m, ms = press(m, "n", "N", "n")
	if _, ok := find[FindMsg](ms); ok {
		t.Error("n while a find is in flight must not ask again")
	}
	m.Found(false, 0)
	if _, ms = press(m, "n"); len(ms) == 0 {
		t.Error("after the answer, n works again")
	}
	if _, ok := find[FindMsg](ms); !ok {
		t.Error("after the answer, n past the edge should ask again")
	}
}

func TestSetFollowFalseLandsAtTheTop(t *testing.T) {
	m := newLog(t, Options{})
	m.SetFollow(false)
	m.SetPage(items(0, 100), 0, 5000, Answer{})
	if m.Cursor() != 0 || m.Following() {
		t.Errorf("a finished job should open at its first event: cursor %d following %v", m.Cursor(), m.Following())
	}
}

func TestSetCursorOnAnEmptyLogDoesNotFollow(t *testing.T) {
	m := newLog(t, Options{})
	m.SetFollow(false)
	m.SetCursor(0) // a rebuild restoring the cursor before any data
	m.SetPage(items(0, 100), 0, 5000, Answer{})
	if m.Cursor() != 0 || m.Following() {
		t.Errorf("cursor %d following %v — restoring a cursor on an empty log turned follow back on", m.Cursor(), m.Following())
	}
}

func TestSearchJumpParksTheMatchWithContextBelow(t *testing.T) {
	m := newLog(t, Options{Searchable: true})
	page := items(0, 100)
	page[60].Lines = []string{"needle"}
	m.SetPage(page, 0, 100, Answer{})
	m.SetCursor(0)
	m, _ = press(m, "/", "n", "e", "e", "d", "l", "e", "enter", "n")
	first, last := m.visibleRange()
	if m.Cursor() != 60 || m.Cursor()-first > (last-first)/2 {
		t.Errorf("match at %d shown in [%d,%d]: it should sit in the upper part, with what follows it visible", m.Cursor(), first, last)
	}
}

func TestNextFindStartsPastAPreviousHit(t *testing.T) {
	m := newLog(t, Options{Searchable: true})
	m.SetPage(items(0, 100), 0, 1000, Answer{})
	m.SetCursor(0)
	m, _ = press(m, "/", "z", "enter")
	m, _ = press(m, "n")
	m.Found(true, 150) // beyond what is held; its page hasn't arrived
	m, ms := press(m, "n")
	if _, ok := find[FindMsg](ms); ok {
		t.Fatal("n while the hit's page is still coming should wait, not ask again")
	}
	// The page lands and the cursor lands on the hit — and stays: the
	// press made while waiting was answered by this hit, not a request to
	// go one further.
	m.SetPage(items(100, 200), 100, 1000, Answer{})
	if m.Cursor() != 150 {
		t.Fatalf("cursor = %d, want it on the hit", m.Cursor())
	}
	_, ms = press(m, "n")
	f, ok := find[FindMsg](ms)
	if !ok || f.From != 199 {
		t.Errorf("find = %+v — the next n searches on past what the page holds", f)
	}
}

func TestArrivalsAreMarkedNewThenFade(t *testing.T) {
	m := newLog(t, Options{NewFor: 50 * time.Millisecond})
	m.SetGrowing(true)
	m.SetPage(items(0, 10), 0, 10, Answer{})
	if m.isNew(5) {
		t.Fatal("the first load is not news")
	}
	m.SetPage(items(10, 13), 10, 13, Answer{})
	if !m.isNew(10) || !m.isNew(12) || m.isNew(9) {
		t.Fatal("exactly the arrivals should be marked")
	}
	time.Sleep(30 * time.Millisecond)
	m.SetPage(items(13, 15), 13, 15, Answer{})
	time.Sleep(30 * time.Millisecond)
	// The first batch has faded on its own clock; the second has not —
	// a steady stream must not keep everything since the first lit.
	if m.isNew(10) {
		t.Error("the first batch should have faded")
	}
	if !m.isNew(13) {
		t.Error("the second batch is still new")
	}
	time.Sleep(40 * time.Millisecond)
	m, cmd := m.Update(struct{}{})
	for _, msg := range run(cmd) {
		if e, ok := msg.(newExpiredMsg); ok {
			m, _ = m.Update(e)
		}
	}
	if len(m.arrivals) != 0 {
		t.Errorf("expired batches should be dropped: %d left", len(m.arrivals))
	}
}

func TestNewHighlightsTheNumberOnly(t *testing.T) {
	m := newLog(t, Options{})
	m.SetGrowing(true)
	m.SetPage(items(0, 10), 0, 10, Answer{})
	m.SetPage(items(10, 12), 10, 12, Answer{})
	if strings.Contains(m.body.View(), "+") {
		t.Error("no + marker: the highlighted number is the cue")
	}
}

func TestPagesLoadedBesideTheHeldOnesAreMarked(t *testing.T) {
	m := newLog(t, Options{})
	m.SetFollow(false)
	m.SetPage(items(100, 200), 100, 1000, Answer{})
	if m.isNew(150) {
		t.Fatal("the first load marks nothing: every row would light up")
	}
	m.SetPage(items(200, 300), 200, 1000, Answer{})
	if !m.isNew(200) || !m.isNew(299) || m.isNew(199) {
		t.Error("a page loaded after the held ones should be marked, and only it")
	}
	m.SetPage(items(0, 100), 0, 1000, Answer{})
	if !m.isNew(0) || !m.isNew(99) || m.isNew(100) {
		t.Error("a page loaded before the held ones should be marked too")
	}
	m.SetPage(items(800, 900), 800, 1000, Answer{})
	if m.isNew(850) {
		t.Error("a far jump replaces everything and marks nothing")
	}
}

func TestSpanEdgeLoadsAreMarked(t *testing.T) {
	m := newLog(t, Options{Anchored: true})
	m.Append(items(100, 150), Answer{})
	if m.isNew(10) {
		t.Fatal("the anchor's first page marks nothing")
	}
	m.Prepend(items(50, 100), Answer{})
	if !m.isNew(0) || !m.isNew(49) || m.isNew(50) {
		t.Error("the prepended rows should be marked")
	}
}

func TestGMarksWhatArrivedWhileAway(t *testing.T) {
	m := newLog(t, Options{})
	m.SetGrowing(true)
	m.SetPage(items(0, 20), 0, 20, Answer{})
	m, _ = m.Update(keyMsg("k")) // commands not run: no timers fire
	m.arrivals = nil
	m.SetPage(items(20, 23), 20, 23, Answer{})
	m.arrivals = nil
	m, _ = m.Update(keyMsg("G"))
	if !m.isNew(20) || m.isNew(19) {
		t.Errorf("G should mark what arrived while away")
	}
}
