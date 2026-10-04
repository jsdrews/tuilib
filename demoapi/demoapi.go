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
// # Why it is generated from a spec
//
// The wire contract is api/openapi.yaml. oapi-codegen turns it into package
// api — models, a typed client, and the strict server interface this package
// implements — so the server cannot drift from the document a client is
// generated from. Edit the spec, run go generate ./demoapi/api, and the
// compiler names every handler the change broke.
//
// That costs one third-party import, oapi-codegen's runtime, which reaches
// only binaries that import demoapi: Go fetches and builds the modules an
// import graph needs, so a program built on tuilib's components never sees
// it. The generator itself runs through go run at a pinned version and is
// not a requirement of this module at all.
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
// The shape follows Argo CD, because that is the API the activity feature was
// found wanting against. Sync is a comparison (Synced, OutOfSync) and never an
// activity; work in progress is Phase. The three ways a client learns an
// operation ended are all here, one per verb:
//
//	POST /apps/{id}/sync                   // fire-and-observe: 202 at once, Phase says Running
//	GET  /apps/{id}?refresh=normal         // blocking: held until reconciled, the app shows nothing
//	GET  /jobs/{job}                       // job handle: the id the 202 returned
//
//	GET  /apps/{id}?refresh=1&takes=5s     // how long the refresh holds the connection
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
// # Elasticsearch search
//
// POST /logs-app/_search answers like Elasticsearch over a growing log
// index: the 10,000-hit window, search_after with a required tiebreaker,
// totals that stop counting, refresh, and documents ingested late. See
// es.go.
//
// # AWX job events
//
// Under /api/v2/jobs/ the fixture answers like AWX's job endpoints — counter
// ranges, page mode capped at 200, stdout and outcome filters, a running
// job whose late-saved events leave holes and whose event processing trails
// its status. See awx.go.
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
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jsdrews/tuilib/demoapi/api"
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
	// BusyWhen exists to show, and a viewer who has to wait out two
	// intervals to see one concludes the feature does not work.
	Schedule time.Duration

	// Vocab is the set of phases scheduled work reports while running. Nil
	// means "Running" for everything, as Argo reports it, which is what a demo
	// wants; a list is how a screen is shown statuses it was not written
	// against — an unfamiliar phase, other casing, a counter, a wide rune.
	//
	// Per-request ?say= is the sharper tool and the one tests should use.
	// This exists so an interactive demo can be odd without a client asking.
	Vocab []string
}

type server struct {
	w   *world
	h   http.Handler
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

	// started is when the fixture was built, on the world's clock: when
	// the AWX corner's running job began, and where the search corner's
	// seeded documents end and live ones begin.
	started time.Time
}

// New returns the fixture as an http.Handler.
func New(opts Options) http.Handler {
	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	s := &server{
		w:   newWorld(opts),
		lat: opts.Latency,
		rng: rand.New(rand.NewSource(seed ^ 0x5eed)),
	}
	s.started = s.w.now()
	s.h = api.HandlerWithOptions(
		api.NewStrictHandlerWithOptions(s, []api.StrictMiddlewareFunc{withURL}, api.StrictHTTPServerOptions{
			RequestErrorHandlerFunc: badRequest,
			ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeErr(w, http.StatusInternalServerError, err.Error())
			},
		}),
		api.StdHTTPServerOptions{ErrorHandlerFunc: badRequest},
	)
	return s
}

// badRequest answers a request the generated layer could not bind — a
// parameter of the wrong type, a body that is not JSON — in the fixture's one
// error shape, or Elasticsearch's under the search path, rather than the
// plain text oapi-codegen writes by default.
func badRequest(w http.ResponseWriter, r *http.Request, err error) {
	if strings.HasPrefix(r.URL.Path, "/logs-app/") {
		esErr(w, http.StatusBadRequest, "parse_exception", err.Error())
		return
	}
	writeErr(w, http.StatusBadRequest, err.Error())
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

	s.h.ServeHTTP(w, r)
}

func (s *server) isDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.downUntil)
}

// ChaosDown refuses every read for a window. POST /chaos/down?for=5s, or
// ?for=0 to end one early.
func (s *server) ChaosDown(_ context.Context, r api.ChaosDownRequestObject) (api.ChaosDownResponseObject, error) {
	d := duration(deref(r.Params.For))
	s.mu.Lock()
	s.downUntil = time.Now().Add(d)
	s.mu.Unlock()
	return api.ChaosDown200JSONResponse{DownFor: d.String()}, nil
}

// ChaosDelete removes an application from the set.
func (s *server) ChaosDelete(_ context.Context, r api.ChaosDeleteRequestObject) (api.ChaosDeleteResponseObject, error) {
	if !s.w.remove(r.Id) {
		return api.ChaosDeletedefaultJSONResponse{StatusCode: http.StatusNotFound, Body: notFound("application", r.Id)}, nil
	}
	return api.ChaosDelete200JSONResponse{Deleted: r.Id}, nil
}

// ChaosSpawn adds an application. ?id= reuses a name something else had;
// ?busy=1 has it arrive already working.
func (s *server) ChaosSpawn(_ context.Context, r api.ChaosSpawnRequestObject) (api.ChaosSpawnResponseObject, error) {
	id := r.Params.Id
	if id == "" {
		return api.ChaosSpawndefaultJSONResponse{StatusCode: http.StatusBadRequest, Body: api.Error{Error: "spawn needs an id"}}, nil
	}
	a, ok := s.w.spawn(id, deref(r.Params.Busy))
	if !ok {
		return api.ChaosSpawndefaultJSONResponse{StatusCode: http.StatusConflict,
			Body: api.Error{Error: "application " + strconv.Quote(id) + " already exists"}}, nil
	}
	return api.ChaosSpawn201JSONResponse(a.wire()), nil
}

func (s *server) roll() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rng.Float64()
}

func (s *server) ListApps(_ context.Context, r api.ListAppsRequestObject) (api.ListAppsResponseObject, error) {
	p := r.Params

	// A parameter named after a column scopes to it — ?region=eu-west — which
	// is what a client driving source.Query sends, since Term.Title is already
	// the resolved column title. ?q= takes the raw grammar instead, for a
	// client forwarding what the user typed. Both, AND-ed, is fine.
	scoped := map[string]string{}
	for col, v := range map[string]*string{
		"Name": p.Name, "Region": p.Region, "Sync": p.Sync, "Health": p.Health, "Phase": p.Phase,
	} {
		if v != nil && *v != "" {
			scoped[col] = *v
		}
	}

	limit := 100
	if p.Limit != nil {
		limit = *p.Limit
	}
	pg := s.w.listApps(deref(p.Q), scoped, deref(p.Sort), deref(p.Desc), deref(p.Offset), limit)
	rows := s.w.asOf(pg.Rows, duration(deref(p.Stale)))
	out := api.ListApps200JSONResponse{Total: pg.Total, Offset: pg.Offset, Rows: make([]api.App, len(rows))}
	for i, a := range rows {
		out.Rows[i] = a.wire()
	}
	return out, nil
}

func (s *server) ListFacets(_ context.Context, r api.ListFacetsRequestObject) (api.ListFacetsResponseObject, error) {
	values := s.w.facet(r.Params.Field)
	if values == nil {
		return api.ListFacetsdefaultJSONResponse{StatusCode: http.StatusBadRequest,
			Body: api.Error{Error: "unknown field " + strconv.Quote(r.Params.Field)}}, nil
	}
	return api.ListFacets200JSONResponse{Values: values}, nil
}

func (s *server) GetApp(ctx context.Context, r api.GetAppRequestObject) (api.GetAppResponseObject, error) {
	if deref(r.Params.Refresh) != "" {
		return s.refreshApp(ctx, r)
	}
	a, ok := s.w.app(r.Id)
	if !ok {
		return api.GetAppdefaultJSONResponse{StatusCode: http.StatusNotFound, Body: notFound("application", r.Id)}, nil
	}
	if rows := s.w.asOf([]App{a}, duration(deref(r.Params.Stale))); len(rows) == 1 {
		a = rows[0]
	}
	var out api.GetApp200JSONResponse
	err := out.FromAppDetail(describe(a, s.w.listJobs(a.ID)))
	return out, err
}

// refreshApp is Argo's refresh: a GET that holds the connection until the
// server has re-compared desired and live state, then answers with the app.
//
// It runs no job and sets no Phase. For its whole duration the app reads
// exactly as it did before, so nothing a poll returns can say a refresh is
// happening or has ended — only the open request knows. That is the shape
// activity.Held exists for.
//
// The hold is real time, not the world's clock, for the reason ?latency= is:
// it is the connection that waits. ?takes= sets it; a test passes ?takes=0.
func (s *server) refreshApp(ctx context.Context, r api.GetAppRequestObject) (api.GetAppResponseObject, error) {
	if _, ok := s.w.app(r.Id); !ok {
		return api.GetAppdefaultJSONResponse{StatusCode: http.StatusNotFound, Body: notFound("application", r.Id)}, nil
	}
	d := refreshDuration
	if r.Params.Takes != nil {
		d = duration(*r.Params.Takes)
	}
	if d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return abandoned{}, nil
		}
	}
	a, ok := s.w.refresh(r.Id)
	if !ok {
		// Deleted while the connection was held.
		return api.GetAppdefaultJSONResponse{StatusCode: http.StatusNotFound, Body: notFound("application", r.Id)}, nil
	}
	var out api.GetApp200JSONResponse
	err := out.FromApp(a.wire())
	return out, err
}

func (s *server) GetJob(_ context.Context, r api.GetJobRequestObject) (api.GetJobResponseObject, error) {
	j, ok := s.w.job(r.Id)
	if !ok {
		return api.GetJobdefaultJSONResponse{StatusCode: http.StatusNotFound, Body: notFound("job", r.Id)}, nil
	}
	return api.GetJob200JSONResponse(j.wire()), nil
}

func (s *server) LaunchJob(_ context.Context, r api.LaunchJobRequestObject) (api.LaunchJobResponseObject, error) {
	fail := func(code int, msg string) (api.LaunchJobResponseObject, error) {
		return api.LaunchJobdefaultJSONResponse{StatusCode: code, Body: api.Error{Error: msg}}, nil
	}
	kind := r.Action
	switch kind {
	case "sync", "fail":
	default:
		return fail(http.StatusNotFound, "no such action "+strconv.Quote(kind))
	}
	p := r.Params
	o := launchOpts{
		reconcile: duration(deref(p.Reconcile)),
		blackhole: deref(p.Blackhole),
		say:       deref(p.Say),
		duration:  -1,
	}
	if p.Phases != nil {
		o.phases = *p.Phases
	}
	if p.Takes != nil {
		o.duration = duration(*p.Takes)
	}
	j, err := s.w.launch(r.Id, kind, o)
	switch {
	case errors.Is(err, ErrNoSuchApp):
		return fail(http.StatusNotFound, "application "+strconv.Quote(r.Id)+" not found")
	case errors.Is(err, ErrBusy):
		// 409, not 202. A client that asked at the same moment as a schedule
		// has to learn it lost, and this is the only place that can know.
		return fail(http.StatusConflict,
			"application "+strconv.Quote(r.Id)+" already has an operation in progress")
	case err != nil:
		return fail(http.StatusInternalServerError, err.Error())
	}
	return api.LaunchJob202JSONResponse{Job: j.ID}, nil
}

// ListJobs is the busy set as its own collection — the shape of an operations
// endpoint, as opposed to a status field on each row. ?status=running is what
// a screen deriving its indicators from this rather than from the app list
// would ask for.
func (s *server) ListJobs(_ context.Context, r api.ListJobsRequestObject) (api.ListJobsResponseObject, error) {
	jobs := s.w.listJobs(deref(r.Params.App))
	out := api.ListJobs200JSONResponse{}
	for _, j := range jobs {
		if r.Params.Status != nil && j.Status != string(*r.Params.Status) {
			continue
		}
		out = append(out, j.wire())
	}
	return out, nil
}

type urlKey struct{}

// withURL hands a handler the URL it was asked on. The strict interface gives
// a handler typed parameters and nothing else, and DRF's next and previous
// links are the request's own URL with the page changed — every parameter the
// client sent, the failure knobs included.
func withURL(f api.StrictHandlerFunc, _ string) api.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, req any) (any, error) {
		return f(context.WithValue(ctx, urlKey{}, *r.URL), w, r, req)
	}
}

func requestURL(ctx context.Context) url.URL {
	u, _ := ctx.Value(urlKey{}).(url.URL)
	return u
}

// abandoned answers a request whose client has gone: it writes nothing, as a
// handler that returned without a response would.
type abandoned struct{}

func (abandoned) VisitGetAppResponse(http.ResponseWriter) error    { return nil }
func (abandoned) VisitGetJobLogResponse(http.ResponseWriter) error { return nil }

func notFound(what, id string) api.Error {
	return api.Error{Error: what + " " + strconv.Quote(id) + " not found"}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
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
	writeJSON(w, code, api.Error{Error: msg})
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
