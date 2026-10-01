package demoapi

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// # AWX job events
//
// Under /api/v2/ the fixture answers like AWX's job endpoints, closely
// enough that a client written against these is shaped like one written
// against the real thing — which is the point: pkg/remote's examples talk
// to this over HTTP instead of to an in-process fake.
//
// What is reproduced, because a timeline component has to survive it:
//
//   - Events are numbered by counter, 1, 2, 3… with no gaps at the source,
//     so a client can fetch any range with counter__gt / counter__lte.
//   - Page mode is page and page_size, capped at 200 as AWX caps it (larger
//     is clamped, not refused). The envelope is DRF's: count, next,
//     previous, results.
//   - A running job keeps emitting events, and some are saved late: a range
//     read now can miss a counter that appears on the next read. The
//     highest counter is the total a client should use; count is not.
//   - event_processing_finished trails the job's own status, so a client
//     that stops polling on status alone misses the last events.
//   - Events carry 0–n lines of stdout; some (runner_on_start) carry none.
//   - Filters: stdout__icontains, failed, changed, host_name, task__icontains.
//
//	GET /api/v2/jobs/
//	GET /api/v2/jobs/4242/                         // running, growing
//	GET /api/v2/jobs/4187/                         // finished, 8,000 events
//	GET /api/v2/jobs/4242/job_events/?order_by=counter&counter__gt=100&counter__lte=200
//	GET /api/v2/jobs/4187/job_events/?stdout__icontains=fatal&counter__gt=37&order_by=counter&page_size=1
//	GET /api/v2/jobs/4187/job_events/?failed=true&page=2&page_size=50

const (
	awxMaxPageSize = 200
	// awxRate is how many events a running job emits a second.
	awxRate = 8
	// awxLiveLength is how many events the running job emits in all.
	awxLiveLength = 3000
	// awxLate is how long a late-saved event takes to appear.
	awxLate = 2 * time.Second
	// awxTrail is how long event processing trails the job's finish.
	awxTrail = 3 * time.Second
)

type awxJob struct {
	id      int
	name    string
	started time.Time
	// events is the full count for a finished job; for the running one it
	// is where it stops.
	events int
	live   bool
}

type awxEvent struct {
	ID        int    `json:"id"`
	Counter   int    `json:"counter"`
	Event     string `json:"event"`
	Stdout    string `json:"stdout"`
	HostName  string `json:"host_name"`
	Task      string `json:"task"`
	Failed    bool   `json:"failed"`
	Changed   bool   `json:"changed"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Created   string `json:"created"`
}

var (
	awxHosts = []string{"web-1", "web-2", "db-1", "cache-1"}
	awxTasks = []string{"Gathering Facts", "install packages", "render config", "restart service", "wait for health"}
)

// awxJobs are the fixture's jobs, the running one started when the
// fixture was built.
func (s *server) awxJobs() []awxJob {
	return []awxJob{
		{id: 4187, name: "deploy-web (nightly)", started: s.started.Add(-2 * time.Hour), events: 8000},
		{id: 4242, name: "deploy-web", started: s.started, events: awxLiveLength, live: true},
	}
}

func (s *server) awxJob(id string) (awxJob, bool) {
	n, _ := strconv.Atoi(strings.TrimSuffix(id, "/"))
	for _, j := range s.awxJobs() {
		if j.id == n {
			return j, true
		}
	}
	return awxJob{}, false
}

// emittedAt is when event n (1-based) of a job happened.
func (j awxJob) emittedAt(n int) time.Time {
	if !j.live {
		return j.started.Add(time.Duration(n) * time.Second / awxRate)
	}
	return j.started.Add(time.Duration(n-40) * time.Second / awxRate)
}

// late reports whether event n is one the callback receiver saves late.
func late(n int) bool { return n%13 == 0 }

// saved reports whether event n is readable at now.
func (j awxJob) saved(n int, now time.Time) bool {
	if n < 1 || n > j.events {
		return false
	}
	if !j.live {
		return true
	}
	at := j.emittedAt(n)
	if late(n) {
		at = at.Add(awxLate)
	}
	return !at.After(now)
}

// emitted is the highest counter emitted by now, saved or not.
func (j awxJob) emitted(now time.Time) int {
	if !j.live {
		return j.events
	}
	n := int(now.Sub(j.started)*awxRate/time.Second) + 40
	return max(0, min(j.events, n))
}

func (j awxJob) finishedAt() time.Time { return j.emittedAt(j.events) }

func (j awxJob) status(now time.Time) (status string, processed bool) {
	if !j.live || !now.Before(j.finishedAt()) {
		return "successful", !now.Before(j.finishedAt().Add(awxTrail))
	}
	return "running", false
}

// awxEventOf is event n of job j: a task header every five, a result per
// host otherwise, a failure now and then, and some with no output at all.
func awxEventOf(j awxJob, n int) awxEvent {
	task := awxTasks[(n/5)%len(awxTasks)]
	host := awxHosts[n%len(awxHosts)]
	e := awxEvent{ID: j.id*100000 + n, Counter: n, Task: task, HostName: host,
		Created: j.emittedAt(n).UTC().Format(time.RFC3339Nano)}
	switch {
	case n%5 == 0:
		e.Event, e.HostName = "playbook_on_task_start", ""
		e.Stdout = fmt.Sprintf("\r\nTASK [%s] %s", task, strings.Repeat("*", 30))
	case n%11 == 0:
		e.Event = "runner_on_start"
	case n%37 == 0:
		e.Event, e.Failed = "runner_on_failed", true
		e.Stdout = fmt.Sprintf("fatal: [%s]: FAILED! => {\"msg\": \"timeout waiting for %s\"}\r\n  retrying in 5s", host, task)
	case n%7 == 0:
		e.Event, e.Changed = "runner_on_ok", true
		e.Stdout = fmt.Sprintf("changed: [%s]", host)
	default:
		e.Event = "runner_on_ok"
		e.Stdout = fmt.Sprintf("ok: [%s]", host)
	}
	lines := 0
	if e.Stdout != "" {
		lines = strings.Count(e.Stdout, "\n") + 1
	}
	e.StartLine, e.EndLine = n*2, n*2+lines
	return e
}

func (s *server) awxListJobs(w http.ResponseWriter, r *http.Request) {
	now := s.w.now()
	var out []map[string]any
	for _, j := range s.awxJobs() {
		out = append(out, s.awxJobJSON(j, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "next": nil, "previous": nil, "results": out})
}

func (s *server) awxGetJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.awxJob(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	writeJSON(w, http.StatusOK, s.awxJobJSON(j, s.w.now()))
}

func (s *server) awxJobJSON(j awxJob, now time.Time) map[string]any {
	status, processed := j.status(now)
	out := map[string]any{
		"id": j.id, "name": j.name, "status": status,
		"started":                   j.started.UTC().Format(time.RFC3339),
		"event_processing_finished": processed,
	}
	if status != "running" {
		out["finished"] = j.finishedAt().UTC().Format(time.RFC3339)
	}
	return out
}

// awxEvents answers GET /api/v2/jobs/{id}/job_events/.
func (s *server) awxEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := s.awxJob(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	q := r.URL.Query()
	now := s.w.now()

	lo, hi := 1, j.emitted(now)
	bound := func(key string, adj int, lower bool) {
		if v := q.Get(key); v != "" {
			n := atoi(v, 0) + adj
			if lower {
				lo = max(lo, n)
			} else {
				hi = min(hi, n)
			}
		}
	}
	bound("counter__gt", 1, true)
	bound("counter__gte", 0, true)
	bound("counter__lt", -1, false)
	bound("counter__lte", 0, false)

	match := awxFilter(q)
	var hits []awxEvent
	for n := lo; n <= hi; n++ {
		if !j.saved(n, now) {
			continue
		}
		if e := awxEventOf(j, n); match(e) {
			hits = append(hits, e)
		}
	}
	switch q.Get("order_by") {
	case "-counter":
		sort.Slice(hits, func(a, b int) bool { return hits[a].Counter > hits[b].Counter })
	case "", "counter":
	default:
		// AWX's default order is start_line, which has ties; a client
		// should always ask for counter. Anything else is refused here
		// rather than silently ordered some other way.
		writeErr(w, http.StatusBadRequest, "order_by: only counter and -counter are supported")
		return
	}

	size := atoi(q.Get("page_size"), 25)
	size = max(1, min(size, awxMaxPageSize))
	page := max(1, atoi(q.Get("page"), 1))
	from := (page - 1) * size
	if from > len(hits) && len(hits) > 0 {
		writeErr(w, http.StatusNotFound, "Invalid page.")
		return
	}
	to := min(len(hits), from+size)
	results := hits[min(from, to):to]
	if results == nil {
		results = []awxEvent{}
	}
	link := func(p int) any {
		u := *r.URL
		v := u.Query()
		v.Set("page", strconv.Itoa(p))
		u.RawQuery = v.Encode()
		return u.String()
	}
	var next, prev any
	if to < len(hits) {
		next = link(page + 1)
	}
	if page > 1 {
		prev = link(page - 1)
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(hits), "next": next, "previous": prev, "results": results})
}

// awxFilter builds the predicate for the query's event filters.
func awxFilter(q url.Values) func(awxEvent) bool {
	text := strings.ToLower(q.Get("stdout__icontains"))
	task := strings.ToLower(q.Get("task__icontains"))
	host := q.Get("host_name")
	failed, changed := q.Get("failed"), q.Get("changed")
	return func(e awxEvent) bool {
		switch {
		case text != "" && !strings.Contains(strings.ToLower(e.Stdout), text):
		case task != "" && !strings.Contains(strings.ToLower(e.Task), task):
		case host != "" && e.HostName != host:
		case failed != "" && strconv.FormatBool(e.Failed) != strings.ToLower(failed):
		case changed != "" && strconv.FormatBool(e.Changed) != strings.ToLower(changed):
		default:
			return true
		}
		return false
	}
}
