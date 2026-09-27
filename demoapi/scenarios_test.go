package demoapi

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// The scenario knobs: the behaviours a per-row activity indicator has to cope
// with, each reproducible on demand.
//
// These are not tests of the fixture's realism — they pin the contract each
// knob offers, so that a screen test asserting "the indicator flickers here"
// is asserting something about the screen rather than about a fixture that
// quietly changed shape underneath it.
//
// Everything runs on a pinned clock. The world advances lazily, so stepping
// the clock and reading produces exactly what the equivalent real interval
// would have.

// phaseOf reads one application's operation phase — the field that says work
// is in progress — optionally through a lag.
func phaseOf(t *testing.T, c *http.Client, id, query string) string {
	t.Helper()
	var p page
	if code := getJSON(t, c, "/apps?limit=5000"+query, &p); code != http.StatusOK {
		t.Fatalf("list = %d, want 200", code)
	}
	for _, a := range p.Rows {
		if a.ID == id {
			return a.Phase
		}
	}
	return ""
}

func hasApp(t *testing.T, c *http.Client, id string) bool {
	t.Helper()
	var p page
	getJSON(t, c, "/apps?limit=5000", &p)
	for _, a := range p.Rows {
		if a.ID == id {
			return true
		}
	}
	return false
}

// --- reconcile lag -------------------------------------------------------

// The window a client that just asked for work reads back a status that has
// not heard about it — a control loop rather than a handler writing inline.
// It is the only thing an expected-entry floor can be measured against.
func TestReconcileDelaysWhenTheAppReportsTheWork(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	before := phaseOf(t, c, "app-00000", "")
	if code := post(t, c, "/apps/app-00000/sync?reconcile=2s", nil); code != http.StatusAccepted {
		t.Fatalf("launch = %d, want 202", code)
	}

	// Accepted, and the server still says what it said before.
	if got := phaseOf(t, c, "app-00000", ""); got != before {
		t.Errorf("immediately after launch = %q, want the pre-launch %q", got, before)
	}
	clk.advance(1900 * time.Millisecond)
	if got := phaseOf(t, c, "app-00000", ""); got != before {
		t.Errorf("inside the reconcile window = %q, want the pre-launch %q", got, before)
	}

	// And then it catches up with itself.
	clk.advance(200 * time.Millisecond)
	if got := phaseOf(t, c, "app-00000", ""); got != PhaseRunning {
		t.Errorf("past the reconcile window = %q, want %q", got, PhaseRunning)
	}
}

// With no knob the status is written inline, which is the ordinary API and the
// default every other test runs under.
func TestWithoutReconcileTheStatusIsImmediate(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	post(t, c, "/apps/app-00000/sync", nil)
	if got := phaseOf(t, c, "app-00000", ""); got != PhaseRunning {
		t.Errorf("sync = %q, want %q with no reconcile lag", got, PhaseRunning)
	}
}

// Reconcile longer than the work models the sharpest version: the server
// finishes before it ever reports starting, so no read can catch it running.
func TestReconcileLongerThanTheWorkIsNeverSeenRunning(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	post(t, c, "/apps/app-00000/sync?reconcile=9s&takes=1s", nil)
	for i := 0; i < 12; i++ {
		clk.advance(time.Second)
		if got := phaseOf(t, c, "app-00000", ""); got == PhaseRunning {
			t.Fatalf("observed %q running at step %d; the reconcile lag outlasts the work", got, i)
		}
	}
	if got := phaseOf(t, c, "app-00000", ""); got != PhaseSucceeded {
		t.Errorf("final = %q, want %q — it should still have landed", got, PhaseSucceeded)
	}
}

// --- stale reads ---------------------------------------------------------

// A read served by a replica that has not caught up. The client cannot fix
// this from its own side: the server genuinely said both things.
func TestStaleReadsServeThePreviousValue(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	before := phaseOf(t, c, "app-00000", "")
	post(t, c, "/apps/app-00000/sync", nil)

	if got := phaseOf(t, c, "app-00000", ""); got != PhaseRunning {
		t.Fatalf("fresh read = %q, want %q", got, PhaseRunning)
	}
	if got := phaseOf(t, c, "app-00000", "&stale=2s"); got != before {
		t.Errorf("stale read = %q, want the pre-launch %q", got, before)
	}

	// Once the change is older than the lag, even a lagging reader has it.
	clk.advance(3 * time.Second)
	if got := phaseOf(t, c, "app-00000", "&stale=2s"); got != PhaseRunning {
		t.Errorf("stale read past the lag = %q, want %q", got, PhaseRunning)
	}
}

// Alternating fresh and lagging reads is the read path going *backwards* —
// busy, settled, busy — which is the one no client-side generation check can
// repair, because neither reply overtook the other.
func TestAlternatingStaleReadsGoBackwards(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	before := phaseOf(t, c, "app-00000", "")
	post(t, c, "/apps/app-00000/sync", nil)

	got := []string{
		phaseOf(t, c, "app-00000", ""),
		phaseOf(t, c, "app-00000", "&stale=2s"),
		phaseOf(t, c, "app-00000", ""),
	}
	want := []string{PhaseRunning, before, PhaseRunning}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("read %d = %q, want %q (sequence %v)", i, got[i], want[i], got)
		}
	}
}

// One transition deep, and the limit is worth pinning rather than discovering.
// However far a reader lags, it sees the value before the most recent change —
// never the one before that.
func TestStaleIsOneTransitionDeep(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	before := phaseOf(t, c, "app-00000", "")
	post(t, c, "/apps/app-00000/sync?phases=one,two&takes=10s", nil)
	if got := phaseOf(t, c, "app-00000", ""); got != "one" {
		t.Fatalf("first phase = %q, want %q", got, "one")
	}
	if got := phaseOf(t, c, "app-00000", "&stale=1h"); got != before {
		t.Fatalf("one change back = %q, want the pre-launch %q", got, before)
	}

	// Second transition. The pre-launch value is now two changes back and is
	// no longer recoverable at any lag.
	clk.advance(6 * time.Second)
	if got := phaseOf(t, c, "app-00000", ""); got != "two" {
		t.Fatalf("second phase = %q, want %q", got, "two")
	}
	if got := phaseOf(t, c, "app-00000", "&stale=1h"); got != "one" {
		t.Errorf("two changes back = %q, want %q — the lag is one transition, not a history", got, "one")
	}
}

// --- blackhole -----------------------------------------------------------

// Accepted, given an id, and nothing happens — indistinguishable from instant
// success unless the client asks about the id it was handed.
func TestBlackholeAcceptsAndDoesNothing(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	before := phaseOf(t, c, "app-00000", "")
	var body struct {
		Job string `json:"job"`
	}
	if code := post(t, c, "/apps/app-00000/sync?blackhole=1", &body); code != http.StatusAccepted {
		t.Fatalf("launch = %d, want 202", code)
	}
	if body.Job == "" {
		t.Error("no job id returned; a client has nothing to ask about")
	}

	for i := 0; i < 10; i++ {
		clk.advance(time.Second)
		if got := phaseOf(t, c, "app-00000", ""); got != before {
			t.Fatalf("status moved to %q at step %d; a blackholed request starts nothing", got, i)
		}
	}

	// The id is the only evidence, and it resolves to nothing.
	var jobs []Job
	getJSON(t, c, "/jobs?app=app-00000", &jobs)
	for _, j := range jobs {
		if j.ID == body.Job {
			t.Errorf("job %s exists; a blackholed request should register none", j.ID)
		}
	}
}

// A blackholed request does not take the busy lock, so the next real one is
// accepted rather than 409'd forever.
func TestBlackholeDoesNotHoldTheApp(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	post(t, c, "/apps/app-00000/sync?blackhole=1", nil)
	if code := post(t, c, "/apps/app-00000/sync", nil); code != http.StatusAccepted {
		t.Errorf("second launch = %d, want 202", code)
	}
}

// --- outage --------------------------------------------------------------

// A client polling into a wall, and then recovering. Distinct from ?fail=,
// which is one request: what matters here is what a screen does with state it
// has stopped being able to refresh.
func TestOutageRefusesReadsThenRecovers(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	if code := post(t, c, "/chaos/down?for=1h", nil); code != http.StatusOK {
		t.Fatalf("chaos/down = %d, want 200", code)
	}
	for i := 0; i < 3; i++ {
		if code := getJSON(t, c, "/apps?limit=4", nil); code != http.StatusServiceUnavailable {
			t.Fatalf("read %d during outage = %d, want 503", i, code)
		}
	}

	// Chaos itself stays reachable, or an outage could never be called off.
	if code := post(t, c, "/chaos/down?for=0", nil); code != http.StatusOK {
		t.Fatalf("ending the outage = %d, want 200", code)
	}
	if code := getJSON(t, c, "/apps?limit=4", nil); code != http.StatusOK {
		t.Errorf("read after recovery = %d, want 200", code)
	}
}

// The world keeps turning behind an outage, so a screen that comes back finds
// what actually happened rather than where it left off.
func TestTheWorldAdvancesBehindAnOutage(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	post(t, c, "/apps/app-00000/sync", nil)
	if got := phaseOf(t, c, "app-00000", ""); got != PhaseRunning {
		t.Fatalf("sync = %q, want %q", got, PhaseRunning)
	}

	post(t, c, "/chaos/down?for=1h", nil)
	clk.advance(10 * time.Second)
	post(t, c, "/chaos/down?for=0", nil)

	if got := phaseOf(t, c, "app-00000", ""); got != PhaseSucceeded {
		t.Errorf("after the outage = %q, want %q — the work finished while nobody could look", got, PhaseSucceeded)
	}
}

// --- vocabulary ----------------------------------------------------------

// One knob covers every "the status is not the word you were expecting" case:
// an unfamiliar phase, other casing, a counter, a wide rune, a long string.
func TestSayOverridesTheReportedStatus(t *testing.T) {
	for _, want := range []string{
		"Superseded",    // a terminal state no Settled list knows
		"syncing",       // the same word, other casing
		"Syncing (3/7)", // detail the server supplies for free
		"同期中",           // wide runes
		"Reconciling an unusually long status that no column will ever fit",
	} {
		t.Run(want, func(t *testing.T) {
			c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})
			post(t, c, "/apps/app-00000/sync?say="+url.QueryEscape(want), nil)
			if got := phaseOf(t, c, "app-00000", ""); got != want {
				t.Errorf("sync = %q, want %q", got, want)
			}
		})
	}
}

// Phases walk in order across the job's duration, so a status that changes
// while the work runs needs no client-side invention to demonstrate.
func TestPhasesWalkInOrder(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	post(t, c, "/apps/app-00000/sync?takes=3s&phases=submitting,applying,verifying", nil)

	for i, want := range []string{"submitting", "applying", "verifying"} {
		if got := phaseOf(t, c, "app-00000", ""); got != want {
			t.Errorf("phase at second %d = %q, want %q", i, got, want)
		}
		clk.advance(time.Second)
	}
	if got := phaseOf(t, c, "app-00000", ""); got != PhaseSucceeded {
		t.Errorf("after the last phase = %q, want %q", got, PhaseSucceeded)
	}
}

// --- churn ---------------------------------------------------------------

// A row that leaves the set. Anything holding state by key has to notice.
func TestDeleteRemovesTheApp(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 6, Schedule: time.Hour})

	if code := post(t, c, "/chaos/delete/app-00002", nil); code != http.StatusOK {
		t.Fatalf("delete = %d, want 200", code)
	}
	if hasApp(t, c, "app-00002") {
		t.Error("app-00002 is still in the list")
	}
	if code := post(t, c, "/chaos/delete/app-00002", nil); code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", code)
	}
}

// Deleting a busy application takes its jobs with it, so nothing is left
// pointing at a row that no longer exists.
func TestDeleteTakesTheJobsWithIt(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 6, Schedule: time.Hour})

	post(t, c, "/apps/app-00002/sync", nil)
	post(t, c, "/chaos/delete/app-00002", nil)

	var jobs []Job
	getJSON(t, c, "/jobs", &jobs)
	for _, j := range jobs {
		if j.App == "app-00002" {
			t.Errorf("job %s still names the deleted app", j.ID)
		}
	}
}

// A row arriving already working: its first observation is mid-flight, which
// nothing about the previous read would have predicted.
func TestSpawnCanArriveBusy(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	if code := post(t, c, "/chaos/spawn?id=app-90001&busy=1", nil); code != http.StatusCreated {
		t.Fatalf("spawn = %d, want 201", code)
	}
	if got := phaseOf(t, c, "app-90001", ""); got != PhaseRunning {
		t.Errorf("spawned busy = %q, want %q", got, PhaseRunning)
	}
}

// Key reuse: the same id, a different thing. Delete and re-spawn is how a
// client holding per-key state is handed something that only shares a name
// with what it was told about.
func TestSpawnReusesADeletedID(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	if code := post(t, c, "/chaos/spawn?id=app-00000", nil); code != http.StatusConflict {
		t.Fatalf("spawn onto a live id = %d, want 409", code)
	}
	post(t, c, "/chaos/delete/app-00000", nil)
	if code := post(t, c, "/chaos/spawn?id=app-00000&busy=1", nil); code != http.StatusCreated {
		t.Fatalf("spawn onto the freed id = %d, want 201", code)
	}
	if got := phaseOf(t, c, "app-00000", ""); got != PhaseRunning {
		t.Errorf("reused id = %q, want %q", got, PhaseRunning)
	}
}

// --- the busy set as its own collection ----------------------------------

// The operations-endpoint shape: who is working, asked directly, rather than
// read off a field on each row.
func TestJobsFilterByStatus(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 6, Schedule: time.Hour})

	post(t, c, "/apps/app-00000/sync", nil)
	post(t, c, "/apps/app-00001/sync", nil)

	var running []Job
	getJSON(t, c, "/jobs?status=running", &running)
	if len(running) != 2 {
		t.Fatalf("running = %d jobs, want 2", len(running))
	}
	for _, j := range running {
		if j.Status != JobRunning {
			t.Errorf("job %s has status %q in a running-only list", j.ID, j.Status)
		}
	}

	clk.advance(2 * syncDuration)
	getJSON(t, c, "/jobs?status=running", &running)
	if len(running) != 0 {
		t.Errorf("running = %d jobs after they finished, want 0", len(running))
	}
}

// --- vocabulary from the schedule ----------------------------------------

// Options.Vocab is the interactive-demo counterpart to ?say=: work nobody
// asked for, reporting a status nobody wrote a predicate against.
func TestScheduledWorkUsesTheConfiguredVocabulary(t *testing.T) {
	c, clk := fixture(t, Options{
		Apps:     6,
		Schedule: time.Second,
		Vocab:    []string{"Reconciling"},
	})

	seen := false
	for i := 0; i < 20 && !seen; i++ {
		clk.advance(time.Second)
		var p page
		getJSON(t, c, "/apps?limit=6", &p)
		for _, a := range p.Rows {
			if a.Phase == "Reconciling" {
				seen = true
			}
		}
	}
	if !seen {
		t.Error("no scheduled work ever reported the configured vocabulary")
	}
}

// --- the Argo shape ------------------------------------------------------

// Sync is a comparison, not an activity. A client that watches it for work in
// progress sees nothing — the trap the first activity example taught.
func TestSyncStatusNeverReportsWorkInProgress(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	var p page
	getJSON(t, c, "/apps?limit=4", &p)
	before := p.Rows[0].Sync

	post(t, c, "/apps/app-00000/sync", nil)
	getJSON(t, c, "/apps?limit=4", &p)
	if p.Rows[0].Sync != before {
		t.Errorf("sync moved to %q while running; it is a comparison, not an activity", p.Rows[0].Sync)
	}
	if p.Rows[0].Phase != PhaseRunning {
		t.Errorf("phase = %q, want %q", p.Rows[0].Phase, PhaseRunning)
	}
	clk.advance(2 * syncDuration)
	getJSON(t, c, "/apps?limit=4", &p)
	if p.Rows[0].Sync != SyncSynced || p.Rows[0].Phase != PhaseSucceeded {
		t.Errorf("after = %s/%s, want %s/%s", p.Rows[0].Sync, p.Rows[0].Phase, SyncSynced, PhaseSucceeded)
	}
}

// A sync shorter than a poll interval, on an app already Synced, leaves no
// trace a poll could see but the phase it ended in — the "nothing happened"
// case activity.UnobservedMsg reports.
func TestAQuickSyncOfASyncedAppIsInvisibleToAPoll(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	var p page
	getJSON(t, c, "/apps?limit=4", &p)
	a := p.Rows[0]
	if a.Sync != SyncSynced {
		t.Skipf("seed puts app-00000 at %s", a.Sync)
	}
	post(t, c, "/apps/app-00000/sync?takes=300ms", nil)
	clk.advance(2 * time.Second) // one poll later
	getJSON(t, c, "/apps?limit=4", &p)
	if p.Rows[0].Sync != SyncSynced || p.Rows[0].Phase == PhaseRunning {
		t.Errorf("after = %s/%s; the poll should have missed it entirely", p.Rows[0].Sync, p.Rows[0].Phase)
	}
}

// Refresh holds the connection and shows nothing while it does. Every read
// taken during it reads exactly as before; only the response says it ended.
func TestRefreshBlocksAndLeavesNoTrace(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	var before page
	getJSON(t, c, "/apps?limit=4", &before)

	done := make(chan App, 1)
	go func() {
		var a App
		getJSON(t, c, "/apps/app-00001?refresh=normal&takes=150ms", &a)
		done <- a
	}()

	time.Sleep(50 * time.Millisecond)
	var mid page
	getJSON(t, c, "/apps?limit=4", &mid)
	if mid.Rows[1].Sync != before.Rows[1].Sync || mid.Rows[1].Phase != before.Rows[1].Phase {
		t.Errorf("mid-refresh = %s/%s, want the app unchanged", mid.Rows[1].Sync, mid.Rows[1].Phase)
	}
	select {
	case <-done:
		t.Fatal("refresh answered before its hold")
	default:
	}

	a := <-done
	if a.ID != "app-00001" {
		t.Fatalf("refresh returned %+v", a)
	}
	if !a.ReconciledAt.After(before.Rows[1].ReconciledAt) && !a.ReconciledAt.Equal(before.Rows[1].ReconciledAt) {
		t.Errorf("reconciledAt went backwards")
	}
}

// What a refresh is for: drift the server had not noticed. app-00001 starts
// drifted by construction.
func TestRefreshDiscoversDrift(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	var p page
	getJSON(t, c, "/apps?limit=4", &p)
	if p.Rows[1].Sync != SyncSynced {
		t.Skipf("seed puts app-00001 at %s", p.Rows[1].Sync)
	}
	var a App
	if code := getJSON(t, c, "/apps/app-00001?refresh=normal&takes=0", &a); code != http.StatusOK {
		t.Fatalf("refresh = %d", code)
	}
	if a.Sync != SyncOutOfSync {
		t.Errorf("refreshed sync = %q, want %q", a.Sync, SyncOutOfSync)
	}
}

// A client that gives up on a refresh is not answered.
func TestRefreshHonoursCancellation(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 4, Schedule: time.Hour})
	c.Timeout = 50 * time.Millisecond
	if _, err := c.Get("http://demo/apps/app-00000?refresh=normal&takes=5s"); err == nil {
		t.Error("a refresh held past the client's timeout still answered")
	}
}

// The job handle resolves, so a client can ask what became of the thing it
// started rather than inferring it from the app.
func TestJobHandleResolves(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 4, Schedule: time.Hour})

	var body struct {
		Job string `json:"job"`
	}
	post(t, c, "/apps/app-00000/sync", &body)
	var j Job
	if code := getJSON(t, c, "/jobs/"+body.Job, &j); code != http.StatusOK || j.Status != JobRunning {
		t.Fatalf("job = %d %+v, want running", code, j)
	}
	clk.advance(2 * syncDuration)
	getJSON(t, c, "/jobs/"+body.Job, &j)
	if j.Status != JobSucceeded {
		t.Errorf("job = %q, want %q", j.Status, JobSucceeded)
	}

	// A blackholed request's handle resolves to nothing, which is how a
	// client tells it apart from instant success.
	post(t, c, "/apps/app-00001/sync?blackhole=1", &body)
	if code := getJSON(t, c, "/jobs/"+body.Job, nil); code != http.StatusNotFound {
		t.Errorf("blackholed handle = %d, want 404", code)
	}
}
