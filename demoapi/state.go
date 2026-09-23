package demoapi

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jsdrews/tuilib/pkg/query"
)

// The world: a generated set of applications, plus jobs that run against them
// and finish after real time has passed.
//
// # No goroutine
//
// Nothing advances in the background. Every read calls advance, which applies
// whatever became due since the last one — completing finished jobs and firing
// any scheduled events whose turn has come.
//
// That is what makes a pinned clock work. A world with a ticker would advance
// on its own schedule regardless of what the test's clock said, so pinning it
// would change nothing; a world that advances lazily is a pure function of its
// seed and the elapsed time, so a test that steps the clock by four seconds
// and reads sees exactly what a real four seconds would have produced.

// App is one application.
type App struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Sync   string `json:"sync"`
	Health string `json:"health"`

	// Rev changes whenever a job against this app completes — a revision
	// whose contract is to move when the work does, the way finished_at or
	// resourceVersion does. Nothing in tuilib reads it today; it is on the
	// wire so a screen can, and so the "work that began and ended between two
	// polls" case has evidence a client could in principle notice.
	Rev int64 `json:"rev"`

	// The values a reader lagging behind this one would still be seeing, and
	// when they stopped being true. Not on the wire: they exist to serve
	// ?stale=, which is how this fixture reproduces a read from a replica or
	// a watch cache that has not caught up — the one thing a client cannot
	// fix from its own side, because the server genuinely said both.
	prevSync, prevHealth string
	prevRev              int64
	changedAt            time.Time
}

// asOf reports this app as a reader lagging by d would still be seeing it.
//
// One transition deep, deliberately. A replica lagging by less than the poll
// interval can only ever be hiding the most recent change, and storing a full
// history to model a deeper lag would buy a scenario nobody is asking about.
func (a App) asOf(now time.Time, d time.Duration) App {
	if d <= 0 || !a.changedAt.After(now.Add(-d)) {
		return a
	}
	a.Sync, a.Health, a.Rev = a.prevSync, a.prevHealth, a.prevRev
	return a
}

// Job is work running against an app.
type Job struct {
	ID       string    `json:"id"`
	App      string    `json:"app"`
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`

	// finishAt is when advance will complete this job. Not on the wire: a
	// real API does not tell a client when its work will be done, and a
	// screen that could read it would be testing against a promise no
	// server makes.
	finishAt time.Time

	// reflectAt is when the app starts reporting this job, and reflected is
	// whether that has happened. They are apart for ?reconcile=, which models
	// a server that accepts a request and takes a moment to show it — a
	// control loop rather than a handler that writes the status inline. The
	// window between them is the one where a client that just asked for work
	// reads back a status that has not heard about it.
	reflectAt time.Time
	reflected bool

	// phases is what the app says while this job runs, walked in order across
	// the job's duration. One entry is the ordinary case; several model work
	// that reports where it has got to.
	phases []string

	// result is what the app's Sync becomes when this job lands.
	result string
	health string
	fails  bool
}

// phase is what the app should be saying at now.
func (j *Job) phase(now time.Time) string {
	switch {
	case len(j.phases) == 0:
		return ""
	case len(j.phases) == 1:
		return j.phases[0]
	}
	total := j.finishAt.Sub(j.Started)
	if total <= 0 {
		return j.phases[len(j.phases)-1]
	}
	i := int(float64(len(j.phases)) * float64(now.Sub(j.Started)) / float64(total))
	return j.phases[max(0, min(i, len(j.phases)-1))]
}

// Sync states. The two the examples treat as in-flight are the two a server
// reports while it is working.
const (
	SyncSynced    = "Synced"
	SyncOutOfSync = "OutOfSync"
	SyncSyncing   = "Syncing"
	SyncRefresh   = "Refreshing"
)

// Job statuses.
const (
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
)

// Durations the fixture uses. Sync is slow enough that any reasonable poll
// observes it running; refresh finishes before the next poll can see it, which
// is the case docs/activity.md decision 19 exists for.
const (
	syncDuration = 4 * time.Second
	// A failing sync fails early, which is both what real ones do and what
	// keeps the client's error and the server's landing roughly together: the
	// log stream is what tells a client it failed, and a four-second job would
	// still be reporting Syncing for seconds after that.
	failDuration    = 1500 * time.Millisecond
	refreshDuration = 0
)

// appColumns is the filter and sort vocabulary, in wire order.
//
// The list endpoint resolves scoped terms against these through pkg/query —
// the same parser pkg/table's filter bar uses. Reimplementing the grammar here
// would let the fixture drift from it, and then every example would be
// teaching a syntax the library does not implement.
var appColumns = []string{"Name", "Region", "Sync", "Health"}

func (a App) cells() []string { return []string{a.Name, a.Region, a.Sync, a.Health} }

var (
	regions  = []string{"eu-west", "eu-central", "us-east", "us-west", "ap-south", "sa-east"}
	teams    = []string{"platform", "payments", "search", "identity", "media"}
	services = []string{"api", "worker", "gateway", "cache", "indexer", "scheduler", "web", "metrics"}
	healths  = []string{"Healthy", "Healthy", "Healthy", "Progressing", "Degraded"}
)

// launchOpts are the per-request knobs on one launch. Per-request rather than
// server-wide for the reason latency and failure already are: the orderings
// worth testing are the asymmetric ones, and one setting for the whole server
// cannot express "this row's work reconciles slowly while that row's does not".
type launchOpts struct {
	// reconcile delays the moment the app starts reporting the job. Models a
	// control loop rather than a handler that writes the status inline.
	reconcile time.Duration

	// blackhole accepts the request, returns an id, and starts nothing.
	blackhole bool

	// say overrides the status the app reports while the job runs — an
	// unfamiliar phase, different casing, a counter, a long or wide string.
	say string

	// phases is say with more than one value, walked across the duration.
	phases []string

	// duration overrides how long the work takes. Negative means the default
	// for the kind, which is what every caller that does not care passes.
	duration time.Duration
}

type world struct {
	mu    sync.Mutex
	now   func() time.Time
	rng   *rand.Rand
	apps  []App
	byID  map[string]int
	vocab []string

	jobs     map[string]*Job
	jobOrder []string

	seq          int64
	schedule     time.Duration
	nextSchedule time.Time
}

func newWorld(opts Options) *world {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	n := opts.Apps
	if n == 0 {
		n = DefaultApps
	}

	schedule := opts.Schedule
	if schedule <= 0 {
		schedule = DefaultSchedule
	}
	w := &world{
		now:      now,
		rng:      rand.New(rand.NewSource(seed)),
		byID:     make(map[string]int, n),
		jobs:     map[string]*Job{},
		schedule: schedule,
		vocab:    opts.Vocab,
	}
	w.apps = make([]App, n)
	for i := range w.apps {
		team := teams[w.rng.Intn(len(teams))]
		svc := services[w.rng.Intn(len(services))]
		sync := SyncSynced
		if w.rng.Intn(4) == 0 {
			sync = SyncOutOfSync
		}
		w.apps[i] = App{
			ID:     fmt.Sprintf("app-%05d", i),
			Name:   fmt.Sprintf("%s-%s-%02d", team, svc, i%100),
			Region: regions[w.rng.Intn(len(regions))],
			Sync:   sync,
			Health: healths[w.rng.Intn(len(healths))],
			Rev:    1,
		}
		w.byID[w.apps[i].ID] = i
	}
	w.nextSchedule = now().Add(schedule)
	return w
}

// advance brings the world up to the current clock, applying due events in
// timestamp order, one at a time.
//
// The ordering is the whole of it. An earlier version ran every job completion
// and then every scheduled event, which is correct only when the two do not
// interleave — and they always do: a scheduled sync fired inside this call
// starts a job that finishes inside it too. Advancing sixty seconds in one
// jump left that job running forever, while advancing a second at a time
// completed it, so the world depended on how often it was read. A pinned clock
// reproduces nothing under that rule.
//
// So: pick the earliest due event, apply it, repeat. The scan over jobs is
// linear per iteration and the job set is small; a heap here would be
// optimising a fixture.
//
// Callers hold w.mu.
func (w *world) advance() {
	now := w.now()

	for {
		var (
			next   *Job
			at     time.Time
			finish bool
		)
		consider := func(j *Job, t time.Time, isFinish bool) {
			if t.After(now) {
				return
			}
			if next == nil || t.Before(at) {
				next, at, finish = j, t, isFinish
			}
		}
		for _, id := range w.jobOrder {
			j := w.jobs[id]
			if j.Status != JobRunning {
				continue
			}
			// A job reflects before it finishes, even when ?reconcile= puts
			// the two out of order in wall time. Landing a result the app was
			// never seen to be working on is a legitimate thing to model —
			// it is work that began and ended between two reads — but it has
			// to happen in that order or complete() overwrites a status that
			// was never set.
			if !j.reflected {
				consider(j, j.reflectAt, false)
				continue
			}
			consider(j, j.finishAt, true)
		}

		scheduleDue := !w.nextSchedule.After(now)
		if next == nil && !scheduleDue {
			break
		}

		// Ties go to the schedule, which only matters for reproducibility:
		// the rule has to be fixed, not fair.
		if scheduleDue && (next == nil || !w.nextSchedule.After(at)) {
			w.fireScheduled(w.nextSchedule)
			w.nextSchedule = w.nextSchedule.Add(w.schedule)
			continue
		}
		if finish {
			w.complete(next)
			continue
		}
		w.reflect(next, at)
	}

	w.refreshPhases(now)
}

// reflect is the moment the app starts reporting a job that was accepted
// earlier. With no ?reconcile= it happens at the instant of the launch, which
// is what an API whose handler writes the status inline does.
func (w *world) reflect(j *Job, at time.Time) {
	j.reflected = true
	if i, ok := w.byID[j.App]; ok {
		w.mark(i, at)
		w.apps[i].Sync = j.phase(at)
	}
}

// refreshPhases re-reports where each running job has got to.
//
// Recomputed on every advance rather than scheduled as events: a phase is a
// pure function of elapsed time, so deriving it costs one pass and cannot
// drift out of step with a pinned clock the way a queue of transitions could.
func (w *world) refreshPhases(now time.Time) {
	for _, id := range w.jobOrder {
		j := w.jobs[id]
		if j.Status != JobRunning || !j.reflected || len(j.phases) < 2 {
			continue
		}
		i, ok := w.byID[j.App]
		if !ok {
			continue
		}
		if p := j.phase(now); p != w.apps[i].Sync {
			w.mark(i, now)
			w.apps[i].Sync = p
		}
	}
}

// mark records what this app was about to stop saying, so a lagging reader
// can still be told the old answer. Callers hold w.mu and pass the moment the
// change happens, not the moment it was noticed.
func (w *world) mark(i int, at time.Time) {
	a := &w.apps[i]
	a.prevSync, a.prevHealth, a.prevRev = a.Sync, a.Health, a.Rev
	a.changedAt = at
}

// complete lands one job on its app.
func (w *world) complete(j *Job) {
	j.Finished = j.finishAt
	j.Status = JobSucceeded
	if j.fails {
		j.Status = JobFailed
	}
	i, ok := w.byID[j.App]
	if !ok {
		return
	}
	// Always, success or failure. Skipping this for a failed job left the app
	// reporting the Syncing that start set, with nothing to ever clear it —
	// a row that spun forever because the server genuinely said it was
	// working. The job carries where the app lands either way; failing means
	// it lands OutOfSync rather than Synced, not that it lands nowhere.
	w.mark(i, j.finishAt)
	w.apps[i].Sync = j.result
	w.apps[i].Health = j.health

	// Rev moves on completion whether or not the job succeeded: something
	// happened to this row, which is what a revision reports.
	w.apps[i].Rev++
}

// scheduleWindow bounds which applications the world starts work on by itself:
// the first few, rather than any of them.
//
// Uniform across the whole set is what a real cluster does and what makes a
// fixture useless. With 5,000 applications, one event every few seconds
// anywhere is invisible by construction — a screen showing six rows sees one
// roughly never, which is exactly what happened: the server was busily syncing
// app-02998 while every demo watched the top of the list and concluded the
// feature did not work.
//
// A demo server should put its activity where the demo is looking. Paging
// screens still have the whole set to page through; screens that show the top
// of the list see background work constantly.
const scheduleWindow = 8

// fireScheduled invents one piece of background work, at the moment it was due
// rather than at the moment it was noticed.
func (w *world) fireScheduled(at time.Time) {
	if len(w.apps) == 0 {
		return
	}
	a := &w.apps[w.rng.Intn(min(len(w.apps), scheduleWindow))]
	if w.busy(a.ID) {
		return
	}

	if w.rng.Intn(2) == 0 {
		// A schedule fires a real sync: slow, so a poll sees it running and
		// a screen with ActivityWhen spins a row nobody touched.
		o := launchOpts{duration: -1}
		if len(w.vocab) > 0 {
			o.say = w.vocab[w.rng.Intn(len(w.vocab))]
		}
		w.start(a.ID, "sync", at, syncDuration, SyncSynced, "Healthy", false, o)
		return
	}

	// Or something completes entirely between two reads, leaving nothing
	// behind but a moved revision. No client in this repo notices, which is
	// the point of keeping it: it is the case the derived-only design
	// knowingly cannot see.
	i := w.byID[a.ID]
	w.mark(i, at)
	a.Rev++
	if a.Health == "Healthy" {
		a.Health = "Degraded"
	} else {
		a.Health = "Healthy"
	}
}

func (w *world) busy(appID string) bool {
	for _, id := range w.jobOrder {
		if j := w.jobs[id]; j.App == appID && j.Status == JobRunning {
			return true
		}
	}
	return false
}

// start registers a job. Callers hold w.mu and have already advanced.
func (w *world) start(appID, kind string, at time.Time, d time.Duration, result, health string, fails bool, o launchOpts) *Job {
	w.seq++
	phases := o.phases
	if len(phases) == 0 {
		say := o.say
		if say == "" {
			say = SyncSyncing
			if kind == "refresh" {
				say = SyncRefresh
			}
		}
		phases = []string{say}
	}
	j := &Job{
		ID:        fmt.Sprintf("job-%04d", w.seq),
		App:       appID,
		Kind:      kind,
		Status:    JobRunning,
		Started:   at,
		finishAt:  at.Add(d),
		reflectAt: at.Add(o.reconcile),
		phases:    phases,
		result:    result,
		health:    health,
		fails:     fails,
	}
	w.jobs[j.ID] = j
	w.jobOrder = append(w.jobOrder, j.ID)

	// The app reports the work as a status like any other, which is what lets
	// a screen derive its indicator from polled data alone — but only once the
	// server has caught up with itself. With no ?reconcile= that is now.
	if o.reconcile <= 0 {
		w.reflect(j, at)
	}
	return j
}

// page is one window of the app list.
type page struct {
	Total  int   `json:"total"`
	Offset int   `json:"offset"`
	Rows   []App `json:"rows"`
}

// listApps filters, sorts and windows.
//
// Two ways to filter, AND-ed together, because both are things a real client
// does. q takes pkg/query's grammar whole — bare substrings, key:value column
// scopes, ~regex — for a client that wants to forward what the user typed.
// scoped takes one resolved column per entry, which is what a client built on
// source.Query does: Term.Title is the resolved column title precisely so it
// can be used as a parameter name without a lookup.
func (w *world) listApps(q string, scoped map[string]string, sortCol string, desc bool, offset, limit int) page {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	terms := query.Parse(q, appColumns)
	for title, v := range scoped {
		col := query.ColumnByPrefix(title, appColumns)
		if col < 0 || v == "" {
			continue
		}
		terms = append(terms, query.Term{
			Column: col,
			Title:  appColumns[col],
			Value:  strings.ToLower(v),
			Raw:    appColumns[col] + ":" + v,
		})
	}
	matched := make([]App, 0, len(w.apps))
	for _, a := range w.apps {
		if query.MatchAll(a.cells(), terms) {
			matched = append(matched, a)
		}
	}

	if col := query.ColumnByPrefix(sortCol, appColumns); sortCol != "" && col >= 0 {
		sort.SliceStable(matched, func(i, j int) bool {
			x := strings.ToLower(matched[i].cells()[col])
			y := strings.ToLower(matched[j].cells()[col])
			if desc {
				return x > y
			}
			return x < y
		})
	}

	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}
	rows := append([]App(nil), matched[offset:end]...)
	return page{Total: total, Offset: offset, Rows: rows}
}

// facet is the whole distinct set for a field, not the current page's.
//
// That distinction is the entire reason the endpoint exists: completions
// scraped from one page of a paged source are wrong rather than merely
// incomplete, which is what CLAUDE.md's "don't scrape filter completions from
// a remote table's rows" anti-pattern is about.
func (w *world) facet(field string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	col := query.ColumnByPrefix(field, appColumns)
	if col < 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range w.apps {
		v := a.cells()[col]
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func (w *world) app(id string) (App, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	i, ok := w.byID[id]
	if !ok {
		return App{}, false
	}
	return w.apps[i], true
}

// listJobs returns jobs newest first, optionally scoped to one app.
func (w *world) listJobs(appID string) []Job {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	out := make([]Job, 0, len(w.jobOrder))
	for i := len(w.jobOrder) - 1; i >= 0; i-- {
		j := w.jobs[w.jobOrder[i]]
		if appID != "" && j.App != appID {
			continue
		}
		out = append(out, *j)
	}
	return out
}

func (w *world) job(id string) (Job, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	j, ok := w.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// Errors launch reports. They exist so the handler can answer 404 and 409
// distinctly, which is the whole of the conflict story: a client cannot avoid
// every race, so the server has to be the one that says no.
var (
	ErrNoSuchApp = errors.New("no such application")
	ErrBusy      = errors.New("an operation is already running")
)

// launch starts a job of the given kind against an app.
//
// Refuses a second operation while one is running, which is what a real API
// does and what makes the conflict reachable. Accepting both left two jobs
// racing on one application and the later completion overwriting the earlier
// one's result — a fixture cannot teach a client to handle a conflict it never
// produces.
func (w *world) launch(appID, kind string, o launchOpts) (Job, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	if _, ok := w.byID[appID]; !ok {
		return Job{}, ErrNoSuchApp
	}
	if w.busy(appID) {
		return Job{}, ErrBusy
	}
	now := w.now()

	// Accepted, given an id, and then nothing happens — the shape of a request
	// a queue drops, a worker never picks up, or a controller filters out. It
	// is worth modelling because it is indistinguishable, from the client's
	// side, from work that succeeded instantly: both leave the row exactly as
	// it was. Any client that can tell them apart does so by asking about the
	// id, which is the only reason this returns one.
	if o.blackhole {
		w.seq++
		return Job{ID: fmt.Sprintf("job-%04d", w.seq), App: appID, Kind: kind, Status: JobRunning, Started: now}, nil
	}

	d := o.duration
	switch kind {
	case "refresh":
		// Instant server-side by default: finished before any poll can see it.
		if d < 0 {
			d = refreshDuration
		}
		return *w.start(appID, kind, now, d, SyncOutOfSync, "Progressing", false, o), nil
	case "fail":
		if d < 0 {
			d = failDuration
		}
		return *w.start(appID, "sync", now, d, SyncOutOfSync, "Degraded", true, o), nil
	default:
		if d < 0 {
			d = syncDuration
		}
		return *w.start(appID, "sync", now, d, SyncSynced, "Healthy", false, o), nil
	}
}

// The churn operations. A set that only ever changes its rows' contents is
// half a fixture: rows also arrive, leave, and come back wearing an id
// something else used to have. Each of those lands somewhere different in a
// component that holds state by key.
//
// They are driven by explicit endpoints rather than by the schedule so a test
// can say when. A world that deleted rows on its own would make every other
// scenario flaky.

// remove deletes an application and any jobs against it.
//
// The indices move, so byID is rebuilt rather than patched. Linear per call on
// a fixture nobody calls in a loop.
func (w *world) remove(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	i, ok := w.byID[id]
	if !ok {
		return false
	}
	w.apps = append(w.apps[:i], w.apps[i+1:]...)
	w.byID = make(map[string]int, len(w.apps))
	for k := range w.apps {
		w.byID[w.apps[k].ID] = k
	}

	kept := w.jobOrder[:0]
	for _, jid := range w.jobOrder {
		if w.jobs[jid].App == id {
			delete(w.jobs, jid)
			continue
		}
		kept = append(kept, jid)
	}
	w.jobOrder = kept
	return true
}

// spawn adds an application, optionally already working.
//
// An explicit id is how key reuse is reproduced: remove app-00000 and spawn it
// again, and a component holding state by key is now holding it for something
// that only shares a name with what it was told about. Arriving already busy
// is the other half — a row whose first observation is mid-flight, which no
// amount of watching the previous poll would have predicted.
func (w *world) spawn(id string, busy bool) (App, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()

	if _, taken := w.byID[id]; taken {
		return App{}, false
	}
	now := w.now()
	a := App{
		ID:     id,
		Name:   fmt.Sprintf("%s-%s-%02d", teams[w.rng.Intn(len(teams))], services[w.rng.Intn(len(services))], len(w.apps)%100),
		Region: regions[w.rng.Intn(len(regions))],
		Sync:   SyncOutOfSync,
		Health: "Progressing",
		Rev:    1,
	}
	w.apps = append(w.apps, a)
	w.byID[id] = len(w.apps) - 1
	if busy {
		w.start(id, "sync", now, syncDuration, SyncSynced, "Healthy", false, launchOpts{duration: -1})
	}
	return w.apps[w.byID[id]], true
}

// asOf is the read a client lagging by d would have got.
//
// Applied to rows the world has already produced rather than threaded through
// listApps, because a lagging replica lags on the values it reports, not on
// which rows match a filter — and modelling the second would mean keeping a
// whole second index up to date to serve one query parameter.
func (w *world) asOf(rows []App, d time.Duration) []App {
	if d <= 0 {
		return rows
	}
	w.mu.Lock()
	now := w.now()
	w.mu.Unlock()

	out := make([]App, len(rows))
	for i, a := range rows {
		out[i] = a.asOf(now, d)
	}
	return out
}
