package demoapi

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The fixture is trusted by every test and example that uses it, so it gets
// its own. The bugs that matter here are the quiet ones: paging arithmetic off
// by one at a boundary, a facet computed from the wrong set, a filter grammar
// that drifts from the one pkg/table's filter bar actually parses.

// clock is a pinned, movable clock. Options.Now takes one of these so a test
// can step four seconds and see exactly what four real seconds would produce.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock {
	return &clock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fixture returns a handler and an in-process client over it.
func fixture(t *testing.T, opts Options) (*http.Client, *clock) {
	t.Helper()
	c := newClock()
	if opts.Now == nil {
		opts.Now = c.Now
	}
	if opts.Seed == 0 {
		opts.Seed = 7
	}
	return Client(New(opts)), c
}

func getJSON(t *testing.T, c *http.Client, url string, into any) int {
	t.Helper()
	resp, err := c.Get("http://demo" + url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if into != nil {
		if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
			t.Fatalf("decoding %s: %v", url, err)
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode
}

func post(t *testing.T, c *http.Client, url string, into any) int {
	t.Helper()
	resp, err := c.Post("http://demo"+url, "", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if into != nil {
		if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
			t.Fatalf("decoding %s: %v", url, err)
		}
	}
	return resp.StatusCode
}

// --- paging --------------------------------------------------------------

func TestPagingBoundaries(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 250})

	for _, tc := range []struct {
		name           string
		url            string
		wantRows       int
		wantOffset     int
		wantTotalIs250 bool
	}{
		{"first page", "/apps?offset=0&limit=100", 100, 0, true},
		{"middle", "/apps?offset=100&limit=100", 100, 100, true},
		{"short final page", "/apps?offset=200&limit=100", 50, 200, true},
		{"offset at total", "/apps?offset=250&limit=100", 0, 250, true},
		{"offset past total clamps", "/apps?offset=9999&limit=100", 0, 250, true},
		{"negative offset clamps", "/apps?offset=-5&limit=10", 10, 0, true},
		{"limit zero means the rest", "/apps?offset=240&limit=0", 10, 240, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p page
			getJSON(t, c, tc.url, &p)
			if len(p.Rows) != tc.wantRows {
				t.Errorf("rows = %d, want %d", len(p.Rows), tc.wantRows)
			}
			if p.Offset != tc.wantOffset {
				t.Errorf("offset = %d, want %d", p.Offset, tc.wantOffset)
			}
			if tc.wantTotalIs250 && p.Total != 250 {
				t.Errorf("total = %d, want 250", p.Total)
			}
		})
	}
}

// Total is the size of the filtered set, not of the world — the table draws
// its scrollbar and its counter against it.
func TestTotalReflectsTheFilter(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 500})

	var all, filtered page
	getJSON(t, c, "/apps?limit=1", &all)
	getJSON(t, c, "/apps?limit=1&q=region:eu-west", &filtered)

	if filtered.Total >= all.Total {
		t.Errorf("filtered total %d, unfiltered %d — the filter did nothing", filtered.Total, all.Total)
	}
	if filtered.Total == 0 {
		t.Fatal("filter matched nothing; the fixture cannot demonstrate filtering")
	}
}

// --- the filter grammar --------------------------------------------------

// The fixture parses with pkg/query, the same parser pkg/table's filter bar
// uses. If these ever diverge, every example teaches a syntax the library does
// not implement — so the agreement is asserted, not assumed.
func TestFilterGrammarMatchesPkgQuery(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 500})

	var scoped page
	getJSON(t, c, "/apps?limit=500&q=region:eu-west", &scoped)
	for _, a := range scoped.Rows {
		if a.Region != "eu-west" {
			t.Fatalf("key:value scope leaked: %s has region %s", a.Name, a.Region)
		}
	}

	// A prefix of the column title resolves, exactly as the filter bar does it.
	var byPrefix page
	getJSON(t, c, "/apps?limit=500&q=reg:eu-west", &byPrefix)
	if byPrefix.Total != scoped.Total {
		t.Errorf("prefix scope matched %d, full title %d", byPrefix.Total, scoped.Total)
	}

	// A regex value.
	var re page
	getJSON(t, c, "/apps?limit=500&q=region:~%5Eeu", &re) // ~^eu
	if re.Total <= scoped.Total {
		t.Errorf("regex ~^eu matched %d, want more than eu-west's %d", re.Total, scoped.Total)
	}

	// Terms are AND-ed.
	var both page
	getJSON(t, c, "/apps?limit=500&q=region:eu-west%20sync:Synced", &both)
	for _, a := range both.Rows {
		if a.Region != "eu-west" || a.Sync != SyncSynced {
			t.Fatalf("AND-ed terms leaked: %+v", a)
		}
	}
	if both.Total >= scoped.Total {
		t.Error("adding a term did not narrow the set")
	}

	// An unresolvable key falls through as a literal rather than being
	// refused — Parse never fails, and neither may the fixture.
	if code := getJSON(t, c, "/apps?limit=1&q=nosuch:thing", nil); code != http.StatusOK {
		t.Errorf("status = %d for an unresolvable scope, want 200", code)
	}
}

func TestSort(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 200})

	var asc, desc page
	getJSON(t, c, "/apps?limit=200&sort=Name", &asc)
	getJSON(t, c, "/apps?limit=200&sort=Name&desc=true", &desc)

	for i := 1; i < len(asc.Rows); i++ {
		if asc.Rows[i-1].Name > asc.Rows[i].Name {
			t.Fatalf("ascending sort out of order at %d", i)
		}
	}
	if len(desc.Rows) > 0 && desc.Rows[0].Name != asc.Rows[len(asc.Rows)-1].Name {
		t.Error("desc is not the reverse of asc")
	}
}

// --- facets --------------------------------------------------------------

// The whole point of the endpoint: completions scraped from one page are wrong
// rather than merely incomplete.
func TestFacetsCoverTheWholeSetNotThePage(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 2000})

	var f struct {
		Values []string `json:"values"`
	}
	getJSON(t, c, "/apps/facets?field=Region", &f)
	if len(f.Values) != len(regions) {
		t.Errorf("got %d regions, want all %d", len(f.Values), len(regions))
	}

	var firstPage page
	getJSON(t, c, "/apps?limit=5", &firstPage)
	seen := map[string]bool{}
	for _, a := range firstPage.Rows {
		seen[a.Region] = true
	}
	if len(seen) >= len(f.Values) {
		t.Skip("this page happened to contain every region; the assertion needs a narrower page")
	}
}

func TestFacetsRejectUnknownField(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 10})
	if code := getJSON(t, c, "/apps/facets?field=nope", nil); code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
}

// --- jobs and the clock --------------------------------------------------

func TestJobTransitionsOnThePinnedClock(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 20})

	var before page
	getJSON(t, c, "/apps?limit=1", &before)
	app := before.Rows[0]

	var launched struct {
		Job string `json:"job"`
	}
	if code := post(t, c, "/apps/"+app.ID+"/sync", &launched); code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", code)
	}

	// Running immediately, and the app reports it as a status like any other
	// — which is what lets a screen derive an indicator from polled data.
	var jobs []Job
	getJSON(t, c, "/jobs?app="+app.ID, &jobs)
	if len(jobs) != 1 || jobs[0].Status != JobRunning {
		t.Fatalf("jobs = %+v, want one running", jobs)
	}
	var mid page
	getJSON(t, c, "/apps?limit=1", &mid)
	if mid.Rows[0].Sync != SyncSyncing {
		t.Errorf("sync = %q, want %q", mid.Rows[0].Sync, SyncSyncing)
	}

	// Still running one second in.
	clk.advance(1 * time.Second)
	getJSON(t, c, "/jobs?app="+app.ID, &jobs)
	if jobs[0].Status != JobRunning {
		t.Errorf("status = %q after 1s, want still running", jobs[0].Status)
	}

	// Done at four.
	clk.advance(3 * time.Second)
	getJSON(t, c, "/jobs?app="+app.ID, &jobs)
	if jobs[0].Status != JobSucceeded {
		t.Errorf("status = %q after 4s, want succeeded", jobs[0].Status)
	}

	var after page
	getJSON(t, c, "/apps?limit=1", &after)
	if after.Rows[0].Sync != SyncSynced {
		t.Errorf("sync = %q, want %q", after.Rows[0].Sync, SyncSynced)
	}
	if after.Rows[0].Rev <= app.Rev {
		t.Errorf("rev did not change: %d then %d", app.Rev, after.Rows[0].Rev)
	}
}

// Refresh is instant server-side. A poll never observes it running, which is
// the case docs/activity.md decision 19 exists for — so the fixture has to be
// able to produce it.
func TestRefreshFinishesBeforeAnyPollCouldSeeIt(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 20})

	var p page
	getJSON(t, c, "/apps?limit=1", &p)
	id := p.Rows[0].ID

	post(t, c, "/apps/"+id+"/refresh", nil)
	clk.advance(2 * time.Second) // one poll interval later

	var jobs []Job
	getJSON(t, c, "/jobs?app="+id, &jobs)
	if len(jobs) != 1 || jobs[0].Status == JobRunning {
		t.Fatalf("jobs = %+v, want it already finished", jobs)
	}
	var after page
	getJSON(t, c, "/apps?limit=1", &after)
	if after.Rows[0].Sync == SyncRefresh {
		t.Error("the first observation still saw it refreshing")
	}
}

// Rev is what ActivityRevision watches, so it must change when work completes
// and not otherwise.
func TestRevChangesOnlyOnCompletion(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 20})

	var p page
	getJSON(t, c, "/apps?limit=1", &p)
	id, rev := p.Rows[0].ID, p.Rows[0].Rev

	// Reading does not change it.
	for i := 0; i < 3; i++ {
		getJSON(t, c, "/apps?limit=1", &p)
	}
	if p.Rows[0].Rev != rev {
		t.Errorf("rev moved on a read: %d then %d", rev, p.Rows[0].Rev)
	}

	post(t, c, "/apps/"+id+"/sync", nil)
	getJSON(t, c, "/apps?limit=1", &p)
	if p.Rows[0].Rev != rev {
		t.Errorf("rev moved when work started rather than when it finished")
	}

	clk.advance(5 * time.Second)
	getJSON(t, c, "/apps?limit=1", &p)
	if p.Rows[0].Rev != rev+1 {
		t.Errorf("rev = %d after completion, want %d", p.Rows[0].Rev, rev+1)
	}
}

// A world advances the same way whether it is read once or forty times, or a
// pinned clock would not reproduce anything.
func TestAdvanceIsIndependentOfReadFrequency(t *testing.T) {
	coarse, cc := fixture(t, Options{Apps: 50, Seed: 99})
	fine, fc := fixture(t, Options{Apps: 50, Seed: 99})

	cc.advance(60 * time.Second)
	var a page
	getJSON(t, coarse, "/apps?limit=50", &a)

	for i := 0; i < 60; i++ {
		fc.advance(1 * time.Second)
		getJSON(t, fine, "/apps?limit=1", nil)
	}
	var b page
	getJSON(t, fine, "/apps?limit=50", &b)

	for i := range a.Rows {
		if a.Rows[i] != b.Rows[i] {
			t.Fatalf("row %d diverged:\n one read: %+v\nmany reads: %+v", i, a.Rows[i], b.Rows[i])
		}
	}
}

func TestSeedIsReproducible(t *testing.T) {
	one, _ := fixture(t, Options{Apps: 100, Seed: 42})
	two, _ := fixture(t, Options{Apps: 100, Seed: 42})

	var a, b page
	getJSON(t, one, "/apps?limit=100", &a)
	getJSON(t, two, "/apps?limit=100", &b)
	for i := range a.Rows {
		if a.Rows[i] != b.Rows[i] {
			t.Fatalf("row %d differs between two runs of seed 42", i)
		}
	}
}

func TestUnknownAppAndAction(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 5})
	if code := getJSON(t, c, "/apps/nope", nil); code != http.StatusNotFound {
		t.Errorf("GET unknown app = %d, want 404", code)
	}
	if code := post(t, c, "/apps/nope/sync", nil); code != http.StatusNotFound {
		t.Errorf("POST to unknown app = %d, want 404", code)
	}
	if code := post(t, c, "/apps/app-00000/frobnicate", nil); code != http.StatusNotFound {
		t.Errorf("POST unknown action = %d, want 404", code)
	}
}

// --- the knobs -----------------------------------------------------------

func TestFailInjection(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 5})
	if code := getJSON(t, c, "/apps?fail=503", nil); code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
	if code := getJSON(t, c, "/apps?limit=1", nil); code != http.StatusOK {
		t.Errorf("the next request also failed: %d", code)
	}
}

func TestLatencyIsPerRequestNotServerWide(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 50})

	done := make(chan time.Duration, 2)
	go func() {
		start := time.Now()
		getJSON(t, c, "/apps?limit=1&latency=400ms", nil)
		done <- time.Since(start)
	}()
	time.Sleep(20 * time.Millisecond)
	go func() {
		start := time.Now()
		getJSON(t, c, "/apps?limit=1", nil)
		done <- time.Since(start)
	}()

	// The fast one must land first. This is the whole reason latency is a
	// query parameter: a server-wide setting cannot produce a slow reply to
	// an abandoned query overtaken by a fast reply to the current one, which
	// is the ordering source.Deliver's generation check exists for.
	first := <-done
	second := <-done
	if first > 200*time.Millisecond {
		t.Errorf("the unencumbered request took %v; latency is not per-request", first)
	}
	if second < 300*time.Millisecond {
		t.Errorf("the slow request took %v, want ~400ms", second)
	}
}

func TestLatencyAcceptsBareMilliseconds(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 5})
	start := time.Now()
	getJSON(t, c, "/apps?limit=1&latency=120", nil)
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Errorf("took %v, want ~120ms — a bare integer should read as milliseconds", d)
	}
}

// --- streaming -----------------------------------------------------------

// The in-process client must stream, or the one endpoint whose point is
// partial arrival would be tested by a transport that buffers it away.
func TestLogStreamsIncrementally(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 5})

	var p page
	getJSON(t, c, "/apps?limit=1", &p)
	var launched struct {
		Job string `json:"job"`
	}
	post(t, c, "/apps/"+p.Rows[0].ID+"/sync", &launched)

	resp, err := c.Get("http://demo/jobs/" + launched.Job + "/log?rate=10ms")
	if err != nil {
		t.Fatalf("GET log: %v", err)
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	var lines int
	firstAt := time.Time{}
	for sc.Scan() {
		if lines == 0 {
			firstAt = time.Now()
		}
		lines++
	}
	if lines != len(logLines) {
		t.Errorf("read %d lines, want %d", lines, len(logLines))
	}
	if firstAt.IsZero() {
		t.Fatal("no lines at all")
	}
	// A buffered transport would deliver everything at once, so the gap
	// between the first line and the last would be ~0.
	if lines > 1 && time.Since(firstAt) < 20*time.Millisecond {
		t.Error("the whole log arrived in one read; the transport is buffering")
	}
}

func TestLogUnknownJob(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 5})
	if code := getJSON(t, c, "/jobs/nope/log", nil); code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

// A failed job must still land the app somewhere restful. It used to skip the
// assignment entirely, leaving the Syncing that starting it had set with
// nothing to clear it — so the app reported itself working forever and every
// client faithfully spun a row for it.
func TestFailedJobLandsInARestingState(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 5})

	var p page
	getJSON(t, c, "/apps?limit=1", &p)
	id := p.Rows[0].ID

	post(t, c, "/apps/"+id+"/fail", nil)

	var mid page
	getJSON(t, c, "/apps?limit=1", &mid)
	if mid.Rows[0].Sync != SyncSyncing {
		t.Fatalf("sync = %q while the job runs, want %q", mid.Rows[0].Sync, SyncSyncing)
	}

	clk.advance(10 * time.Second)

	var jobs []Job
	getJSON(t, c, "/jobs?app="+id, &jobs)
	if len(jobs) != 1 || jobs[0].Status != JobFailed {
		t.Fatalf("jobs = %+v, want one failed", jobs)
	}

	var after page
	getJSON(t, c, "/apps?limit=1", &after)
	if after.Rows[0].Sync == SyncSyncing {
		t.Error("a failed job left the app reporting Syncing with nothing to clear it")
	}
	if after.Rows[0].Sync != SyncOutOfSync {
		t.Errorf("sync = %q, want %q — a sync that failed did not reconcile",
			after.Rows[0].Sync, SyncOutOfSync)
	}
}

// Scheduled work has to land where a demo is looking.
//
// Uniform across 5,000 applications is what a real cluster does and what makes
// a fixture useless: a screen showing six rows sees an event roughly never. It
// shipped that way, and the symptom was a demo whose whole subject — a row
// spinning because the server said so — appeared not to work while the server
// was busily syncing app-02998.
func TestScheduledWorkLandsWhereADemoIsLooking(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 5000, Schedule: time.Second})

	clk.advance(90 * time.Second)

	var jobs []Job
	getJSON(t, c, "/jobs", &jobs)
	if len(jobs) == 0 {
		t.Fatal("the world scheduled no work at all in 90 seconds")
	}
	for _, j := range jobs {
		n, err := strconv.Atoi(strings.TrimPrefix(j.App, "app-"))
		if err != nil {
			t.Fatalf("unparseable app id %q", j.App)
		}
		if n >= scheduleWindow {
			t.Errorf("scheduled work on %s, outside the first %d — no screen showing "+
				"the top of the list would ever see it", j.App, scheduleWindow)
		}
	}
}

// Options.Schedule has to be honoured, or a demo cannot ask for a world lively
// enough to watch.
func TestScheduleOptionChangesTheRate(t *testing.T) {
	count := func(schedule time.Duration) int {
		c, clk := fixture(t, Options{Apps: 40, Schedule: schedule, Seed: 5})
		clk.advance(60 * time.Second)
		var jobs []Job
		getJSON(t, c, "/jobs", &jobs)
		return len(jobs)
	}

	slow, fast := count(10*time.Second), count(2*time.Second)
	if fast <= slow {
		t.Errorf("a 2s schedule produced %d jobs and a 10s one %d; the option does nothing",
			fast, slow)
	}
}

// A command sent to an application that is already working must be refused, not
// queued and not silently accepted.
//
// It used to return 202 for both, so two jobs raced on one application and
// whichever finished last set the result. A client cannot avoid every race —
// the schedule can fire in the window between its last poll and its POST — so
// the server has to be the one that says no.
func TestConflictingCommandIsRejected(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 5, Schedule: time.Hour})

	var p page
	getJSON(t, c, "/apps?limit=1", &p)
	id := p.Rows[0].ID

	if code := post(t, c, "/apps/"+id+"/sync", nil); code != http.StatusAccepted {
		t.Fatalf("first POST = %d, want 202", code)
	}
	if code := post(t, c, "/apps/"+id+"/sync", nil); code != http.StatusConflict {
		t.Errorf("second POST = %d, want 409", code)
	}

	var jobs []Job
	getJSON(t, c, "/jobs?app="+id, &jobs)
	if len(jobs) != 1 {
		t.Errorf("%d jobs on one app; a rejected command still started work", len(jobs))
	}

	// And once it is done, the app accepts work again.
	clk.advance(10 * time.Second)
	if code := post(t, c, "/apps/"+id+"/sync", nil); code != http.StatusAccepted {
		t.Errorf("POST after completion = %d, want 202", code)
	}
}

// The conflict body has to say what happened, since it is what a screen shows.
func TestConflictExplainsItself(t *testing.T) {
	c, _ := fixture(t, Options{Apps: 5, Schedule: time.Hour})

	var p page
	getJSON(t, c, "/apps?limit=1", &p)
	id := p.Rows[0].ID
	post(t, c, "/apps/"+id+"/sync", nil)

	resp, err := c.Post("http://demo/apps/"+id+"/sync", "", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !strings.Contains(body.Error, "in progress") {
		t.Errorf("error = %q, want it to say an operation is already running", body.Error)
	}
}

// Work the server starts on its own conflicts the same way, which is the case a
// client cannot see coming.
func TestScheduledWorkAlsoBlocksACommand(t *testing.T) {
	c, clk := fixture(t, Options{Apps: 8, Schedule: time.Second, Seed: 3})

	// Let a schedule fire and catch an app mid-job.
	var busy string
	for i := 0; i < 30 && busy == ""; i++ {
		clk.advance(time.Second)
		var jobs []Job
		getJSON(t, c, "/jobs", &jobs)
		for _, j := range jobs {
			if j.Status == JobRunning {
				busy = j.App
				break
			}
		}
	}
	if busy == "" {
		t.Fatal("no scheduled job was ever running")
	}

	if code := post(t, c, "/apps/"+busy+"/sync", nil); code != http.StatusConflict {
		t.Errorf("POST to an app the server is working on = %d, want 409", code)
	}
}
