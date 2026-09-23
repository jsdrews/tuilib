// Package demoapi is a fake API to build TUIs against: applications that can
// be listed, filtered, paged and sorted, and jobs that run against them and
// finish after real time has passed.
//
// It is a fixture. It has no authentication, it will fail or stall a request
// on request, and the binary binds loopback on purpose. Do not deploy it.
//
// # Why it is a handler
//
// New returns an http.Handler rather than running a server, which is what lets
// one fixture serve four uses:
//
//	srv := httptest.NewServer(demoapi.New(demoapi.Options{Seed: 7}))  // tests
//	client := demoapi.Client(demoapi.New(demoapi.Options{}))          // examples, no socket
//	go run ./demoapi/cmd/demoapi                                      // a terminal
//	import "github.com/jsdrews/tuilib/demoapi"                        // another project
//
// A main cannot be shared. A handler can be shared four ways.
//
// # Why it has no dependencies
//
// Standard library only, plus pkg/query from this module. The cost of a
// package here is not compile time — Go builds what is imported — it is
// go.mod: anything demoapi required, every consumer of tuilib would require.
// The day this needs a third-party import it should become a nested module.
//
// # Latency and failure are per-request
//
// Every endpoint honours ?latency=, ?fail= and ?flaky=. Per-request rather
// than server-wide because the orderings worth testing are the asymmetric
// ones: a slow reply to an abandoned filter arriving after a fast reply to the
// current one is what source.Deliver's generation check exists for, and one
// knob for the whole server cannot express it.
//
//	GET /apps?q=eu&latency=900ms    // the abandoned query
//	GET /apps?q=euro                // what the user actually typed
//
// # Scenario knobs
//
// Beyond latency and failure, the fixture can reproduce the behaviours a
// per-row activity indicator has to survive. Each is a query parameter on the
// request it applies to, or — where the effect has to outlive one request — an
// endpoint under /chaos/.
//
//	GET  /apps?stale=2s                    // a replica that has not caught up
//	POST /apps/{id}/sync?reconcile=2s      // accepted now, reported later
//	POST /apps/{id}/sync?blackhole=1       // accepted, given an id, dropped
//	POST /apps/{id}/sync?say=Superseded    // a status no predicate was written for
//	POST /apps/{id}/sync?phases=a,b,c      // a status that moves while it works
//	POST /apps/{id}/sync?takes=800ms       // how long the work runs
//	GET  /jobs?status=running              // the busy set as its own collection
//	POST /chaos/down?for=5s                // every read 503s until it passes
//	POST /chaos/delete/{id}                // a row leaves the set
//	POST /chaos/spawn?id=X&busy=1          // a row arrives, already working
//
// Alternating ?stale= with a fresh read is how a read path that goes
// *backwards* is reproduced — busy, settled, busy — which no client-side
// generation check can repair, because neither reply overtook the other.
// Deleting an id and spawning it again is how key reuse is reproduced.
//
// # The world advances on the clock
//
// A job started at T finishes at T+4s because four seconds passed, not because
// four requests arrived — so a curl in another terminal sees the same world a
// TUI does. Nothing runs in the background; each read applies whatever became
// due. Options.Now pins the clock for a test, and the same code path serves
// both.
package demoapi

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultApps is enough rows that a windowed table is actually windowed.
const DefaultApps = 5000

// DefaultSchedule is how often the world starts work of its own.
const DefaultSchedule = 6 * time.Second

// Options configures the fixture.
type Options struct {
	// Seed makes the generated world and its scheduled events reproducible.
	// Zero seeds from the clock.
	Seed int64

	// Now is the clock the world advances on. Nil means time.Now. A test
	// passes a controllable one and drives transitions exactly.
	Now func() time.Time

	// Apps is how many applications to generate. Zero means DefaultApps.
	Apps int

	// Latency is a floor applied to every request, before any per-request
	// ?latency=. Zero means none.
	Latency time.Duration

	// Schedule is how often the world starts work nobody asked for. Zero means
	// DefaultSchedule.
	//
	// A demo wants this short: background work is the thing a screen with
	// ActivityWhen exists to show, and a viewer who has to wait out two
	// intervals to see one concludes the feature does not work.
	Schedule time.Duration

	// Vocab is the set of status strings scheduled work reports while
	// running. Nil means "Syncing" for everything, which is what a demo
	// wants; a list is how a screen is shown statuses it was not written
	// against — an unfamiliar phase, other casing, a counter, a wide rune.
	//
	// Per-request ?say= is the sharper tool and the one tests should use.
	// This exists so an interactive demo can be odd without a client asking.
	Vocab []string
}

type server struct {
	w   *world
	mux *http.ServeMux
	lat time.Duration

	// rng backs ?flaky=. Separate from the world's, and mutex-guarded, so a
	// coin flip on a concurrent request cannot perturb the sequence the
	// world's scheduled events are drawn from — which would make a seeded
	// world depend on how many requests happened to be in flight.
	mu  sync.Mutex
	rng *rand.Rand

	// downUntil is when an injected outage ends. A window rather than a
	// per-request ?fail= because the scenario worth reproducing is a client
	// that keeps polling into a wall and then recovers — the failure of a
	// single request is already covered, and says nothing about what a screen
	// does with state it stopped being able to refresh.
	downUntil time.Time
}

// New returns the fixture as an http.Handler.
func New(opts Options) http.Handler {
	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	s := &server{
		w:   newWorld(opts),
		mux: http.NewServeMux(),
		lat: opts.Latency,
		rng: rand.New(rand.NewSource(seed ^ 0x5eed)),
	}
	s.routes()
	return s
}

func (s *server) routes() {
	s.mux.HandleFunc("GET /apps", s.listApps)
	s.mux.HandleFunc("GET /apps/facets", s.facets)
	s.mux.HandleFunc("GET /apps/{id}", s.getApp)
	s.mux.HandleFunc("POST /apps/{id}/{action}", s.launch)
	s.mux.HandleFunc("GET /jobs", s.listJobs)
	s.mux.HandleFunc("GET /jobs/{id}/log", s.jobLog)

	// Chaos is stateful, so it is endpoints rather than query parameters: a
	// test says when the world changes shape, and the change outlives the
	// request that asked for it.
	s.mux.HandleFunc("POST /chaos/down", s.chaosDown)
	s.mux.HandleFunc("POST /chaos/delete/{id}", s.chaosDelete)
	s.mux.HandleFunc("POST /chaos/spawn", s.chaosSpawn)
}

// ServeHTTP applies the per-request knobs, then routes.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if d := s.lat + duration(q.Get("latency")); d > 0 {
		select {
		case <-time.After(d):
		case <-r.Context().Done():
			// A client that gave up mid-latency must not be answered. This is
			// the path a screen exercises when it abandons a query, and a
			// fixture that answered anyway would hide a leak.
			return
		}
	}

	if code := atoi(q.Get("fail"), 0); code >= 400 {
		writeErr(w, code, "injected failure")
		return
	}
	if p := parseFloat(q.Get("flaky")); p > 0 && s.roll() < p {
		writeErr(w, http.StatusBadGateway, "injected flake")
		return
	}

	// The outage gate is last, and /chaos/ is exempt from it, or an outage
	// could never be cut short and a test that wanted one would have to wait
	// it out in real time.
	if !strings.HasPrefix(r.URL.Path, "/chaos/") && s.isDown() {
		writeErr(w, http.StatusServiceUnavailable, "injected outage")
		return
	}

	s.mux.ServeHTTP(w, r)
}

func (s *server) isDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.downUntil)
}

// chaosDown refuses every read for a window. POST /chaos/down?for=5s, or
// ?for=0 to end one early.
func (s *server) chaosDown(w http.ResponseWriter, r *http.Request) {
	d := duration(r.URL.Query().Get("for"))
	s.mu.Lock()
	s.downUntil = time.Now().Add(d)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"down_for": d.String()})
}

// chaosDelete removes an application from the set.
func (s *server) chaosDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.w.remove(id) {
		writeErr(w, http.StatusNotFound, "application "+strconv.Quote(id)+" not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// chaosSpawn adds an application. ?id= reuses a name something else had;
// ?busy=1 has it arrive already working.
func (s *server) chaosSpawn(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "spawn needs an id")
		return
	}
	a, ok := s.w.spawn(id, q.Get("busy") == "1" || q.Get("busy") == "true")
	if !ok {
		writeErr(w, http.StatusConflict, "application "+strconv.Quote(id)+" already exists")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *server) roll() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rng.Float64()
}

func (s *server) listApps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// A parameter named after a column scopes to it — ?region=eu-west — which
	// is what a client driving source.Query sends, since Term.Title is already
	// the resolved column title. ?q= takes the raw grammar instead, for a
	// client forwarding what the user typed. Both, AND-ed, is fine.
	scoped := map[string]string{}
	for _, col := range appColumns {
		if v := q.Get(strings.ToLower(col)); v != "" {
			scoped[col] = v
		}
	}

	p := s.w.listApps(
		q.Get("q"),
		scoped,
		q.Get("sort"),
		q.Get("desc") == "true" || q.Get("desc") == "1",
		atoi(q.Get("offset"), 0),
		atoi(q.Get("limit"), 100),
	)
	p.Rows = s.w.asOf(p.Rows, duration(q.Get("stale")))
	writeJSON(w, http.StatusOK, p)
}

func (s *server) facets(w http.ResponseWriter, r *http.Request) {
	field := r.URL.Query().Get("field")
	values := s.w.facet(field)
	if values == nil {
		writeErr(w, http.StatusBadRequest, "unknown field "+strconv.Quote(field))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"values": values})
}

func (s *server) getApp(w http.ResponseWriter, r *http.Request) {
	a, ok := s.w.app(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "application "+strconv.Quote(r.PathValue("id"))+" not found")
		return
	}
	if rows := s.w.asOf([]App{a}, duration(r.URL.Query().Get("stale"))); len(rows) == 1 {
		a = rows[0]
	}
	writeJSON(w, http.StatusOK, describe(a, s.w.listJobs(a.ID)))
}

func (s *server) launch(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("action")
	switch kind {
	case "sync", "refresh", "fail":
	default:
		writeErr(w, http.StatusNotFound, "no such action "+strconv.Quote(kind))
		return
	}
	id := r.PathValue("id")
	q := r.URL.Query()
	o := launchOpts{
		reconcile: duration(q.Get("reconcile")),
		blackhole: q.Get("blackhole") == "1" || q.Get("blackhole") == "true",
		say:       q.Get("say"),
		duration:  -1,
	}
	if v := q.Get("phases"); v != "" {
		o.phases = strings.Split(v, ",")
	}
	if v := q.Get("takes"); v != "" {
		o.duration = duration(v)
	}
	j, err := s.w.launch(id, kind, o)
	switch {
	case errors.Is(err, ErrNoSuchApp):
		writeErr(w, http.StatusNotFound, "application "+strconv.Quote(id)+" not found")
		return
	case errors.Is(err, ErrBusy):
		// 409, not 202. A client that asked at the same moment as a schedule
		// has to learn it lost, and this is the only place that can know.
		writeErr(w, http.StatusConflict,
			"application "+strconv.Quote(id)+" already has an operation in progress")
		return
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": j.ID})
}

// listJobs is the busy set as its own collection — the shape of an operations
// endpoint, as opposed to a status field on each row. ?status=running is what
// a screen deriving its indicators from this rather than from the app list
// would ask for.
func (s *server) listJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	jobs := s.w.listJobs(q.Get("app"))
	if want := q.Get("status"); want != "" {
		kept := jobs[:0]
		for _, j := range jobs {
			if j.Status == want {
				kept = append(kept, j)
			}
		}
		jobs = kept
	}
	writeJSON(w, http.StatusOK, jobs)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr is the only error shape. A status code and a JSON body, because
// that is what a screen has to learn to handle; a typed Go error would be a
// nicer API and would teach the wrong lesson.
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// duration accepts "250ms" and a bare integer read as milliseconds, because
// hand-typing a URL is half of what this fixture is for.
func duration(s string) time.Duration {
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return time.Duration(n) * time.Millisecond
	}
	return 0
}
