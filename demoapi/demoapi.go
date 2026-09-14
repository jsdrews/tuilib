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

	s.mux.ServeHTTP(w, r)
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

	writeJSON(w, http.StatusOK, s.w.listApps(
		q.Get("q"),
		scoped,
		q.Get("sort"),
		q.Get("desc") == "true" || q.Get("desc") == "1",
		atoi(q.Get("offset"), 0),
		atoi(q.Get("limit"), 100),
	))
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
	j, err := s.w.launch(id, kind)
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

func (s *server) listJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.w.listJobs(r.URL.Query().Get("app")))
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
