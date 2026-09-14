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

	// Rev changes whenever a job against this app completes. It is on the
	// wire rather than derived because it is what activity.ActivityRevision
	// watches: without it a fixture cannot demonstrate work that began and
	// ended between two polls, which is the case that needs it most.
	Rev int64 `json:"rev"`
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

	// result is what the app's Sync becomes when this job lands.
	result string
	health string
	fails  bool
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

type world struct {
	mu   sync.Mutex
	now  func() time.Time
	rng  *rand.Rand
	apps []App
	byID map[string]int

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
			next *Job
			at   time.Time
		)
		for _, id := range w.jobOrder {
			j := w.jobs[id]
			if j.Status != JobRunning || j.finishAt.After(now) {
				continue
			}
			if next == nil || j.finishAt.Before(at) {
				next, at = j, j.finishAt
			}
		}

		scheduleDue := !w.nextSchedule.After(now)
		if next == nil && !scheduleDue {
			return
		}

		// Ties go to the schedule, which only matters for reproducibility:
		// the rule has to be fixed, not fair.
		if scheduleDue && (next == nil || !w.nextSchedule.After(at)) {
			w.fireScheduled(w.nextSchedule)
			w.nextSchedule = w.nextSchedule.Add(w.schedule)
			continue
		}
		w.complete(next)
	}
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
		w.start(a.ID, "sync", at, syncDuration, SyncSynced, "Healthy", false)
		return
	}

	// Or something completes entirely between two polls. The only evidence is
	// the revision, which is what ActivityRevision watches for.
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
func (w *world) start(appID, kind string, at time.Time, d time.Duration, result, health string, fails bool) *Job {
	w.seq++
	j := &Job{
		ID:       fmt.Sprintf("job-%04d", w.seq),
		App:      appID,
		Kind:     kind,
		Status:   JobRunning,
		Started:  at,
		finishAt: at.Add(d),
		result:   result,
		health:   health,
		fails:    fails,
	}
	w.jobs[j.ID] = j
	w.jobOrder = append(w.jobOrder, j.ID)

	// The app reports the work as a status like any other, which is what lets
	// a screen derive its indicator from polled data alone.
	if i, ok := w.byID[appID]; ok {
		switch kind {
		case "refresh":
			w.apps[i].Sync = SyncRefresh
		default:
			w.apps[i].Sync = SyncSyncing
		}
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
func (w *world) launch(appID, kind string) (Job, error) {
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
	switch kind {
	case "refresh":
		// Instant server-side: finished before any poll can observe it.
		return *w.start(appID, kind, now, refreshDuration, SyncOutOfSync, "Progressing", false), nil
	case "fail":
		return *w.start(appID, "sync", now, failDuration, SyncOutOfSync, "Degraded", true), nil
	default:
		return *w.start(appID, "sync", now, syncDuration, SyncSynced, "Healthy", false), nil
	}
}
