# Demo API — design

Status: **proposed.** Nothing here is built. It depends on nothing new: the
whole thing is `net/http` plus `encoding/json`, which is the property that
decides where it lives (decision 2).

## Problem

**Every example invents its own backend, and none of them can be borrowed.**
`examples/patterns/remote` hand-rolls a 5,000-row paged API with a 100-row page
size and a 250ms sleep. `examples/patterns/activity` hand-rolls a
mutex-guarded `backend` with jobs, revisions and scheduled work.
`examples/patterns/poll` mutates a synthetic job list on a tick. Three
different shapes of the same idea, each a private struct inside an example
package, each exercised by exactly one demo.

**The races the library is designed against cannot be reproduced.** This is the
sharper half. Two of the more careful pieces of behaviour in the library exist
for orderings that only a real server produces:

- `source.Deliver` carries a generation per request so a slow reply to the
  *previous* filter cannot paint itself under the current one.
- `activity`'s local-beats-derived rule exists so a poll already in flight when
  the user clicked cannot wipe their spinner with a pre-click value.

Both are currently asserted by handing the component a message sequence written
by the test. That proves the code handles an ordering *the test constructed*.
It does not prove that ordering is the one a real client produces, and it
cannot catch the case where the screen wires the pieces together correctly and
the sequence still comes out wrong. To do that you need something that will
answer one request slowly and another quickly, on purpose, at the same time.

**And there is a second project.** Another codebase uses tuilib and has the
same need. Today there is nothing to share, because every fixture is unexported
and every one of them is a `struct`, not a server.

## Shape

One `http.Handler`, standard library only, over two nouns and a clock. Four
consumers, all from the same code:

```go
// 1. tuilib's own tests
srv := httptest.NewServer(demoapi.New(demoapi.Options{Seed: 7, Now: clock.Now}))

// 2. tuilib's examples — in-process, no socket, so `task examples` needs
//    nothing running
api := demoapi.New(demoapi.Options{})
client := demoapi.Client(api)

// 3. a terminal
task server            // go run ./demoapi/cmd/demoapi --addr 127.0.0.1:8099

// 4. the other project
import "github.com/jsdrews/tuilib/demoapi"
```

A `main` cannot be shared. A handler can be shared four ways, and that is the
entire answer to "is there a way to share this".

---

## Decisions

### 1. A handler, not a program.

`demoapi.New(Options) http.Handler` is the artefact. The binary is a fifteen-line
`cmd/` wrapper around it, and is not where any logic lives.

Everything else in this document follows from that one choice. A program can be
run; a handler can be run, embedded in a test, embedded in an example, and
imported by another module. The moment the fixture's behaviour lives in `main`
it is reachable only by starting a process and talking to a port, which rules
out the two uses that matter most — unit tests and examples.

### 2. Standard library only, and that is what keeps it in the main module.

The cost of a package in the main module is not compile time — Go builds only
what is imported — and it is not binary size. It is **`go.mod`**. Anything
`demoapi` requires, every downstream consumer of tuilib requires, forever,
whether or not they ever import it.

tuilib's direct dependencies today are charmbracelet, termenv, `x/term` and
yaml. A TUI library that made everyone who depends on it also depend on an HTTP
router, or on testify, would be making a decision on their behalf that it has
no business making.

`net/http`, `encoding/json`, `math/rand`, `time` and `net/http/httptest` are
all standard library. A fake API needs nothing else, so the constraint costs
nothing to honour.

**The rule, written down so the next person does not have to re-derive it: the
day `demoapi` needs a non-stdlib import, it becomes a nested module.** That
means `demoapi/go.mod`, its own tags (`demoapi/v0.1.0`), and a separate
`require` line downstream — which also means version skew between the fixture
and the library it is a fixture for becomes possible, and someone will
eventually hit it. Real ceremony, worth avoiding, easy to avoid.

### 3. Top-level `demoapi/`, not `pkg/`.

`pkg/` is UI components — every directory in it is something a screen composes.
A fake API server there would be the first thing that is not, and the
categorisation is load-bearing: "read the nearest example, look in `pkg/` for
the component" is how this repo is navigated.

```
demoapi/
    demoapi.go        // New, Options, Client
    apps.go           // the list/detail/facet endpoints
    jobs.go           // status transitions, the log stream
    state.go          // the world, and the clock that advances it
    cmd/demoapi/      // the binary
```

**Rejected: `internal/demoapi`.** It would sit naturally beside
`internal/componenttest` and it cannot be imported by the other project, which
is the requirement that started this.

### 4. Examples use it in-process, over no socket at all.

A demo suite that needs a sidecar running is a demo suite people stop running,
and `task examples` currently needs nothing. So `demoapi.Client(h)` returns an
`*http.Client` whose `Transport` dispatches straight into the handler:

```go
type transport struct{ h http.Handler }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { … }
```

No port, no bind, no cleanup, no flake when a test runner starts twenty
examples at once, and no behaviour difference worth caring about — the screen
still builds a `*http.Request`, still gets an `*http.Response`, still parses
JSON out of a body.

**Implement it over `io.Pipe`, not `httptest.NewRecorder`.** The recorder
buffers the entire response and hands it back when the handler returns, which
is correct for the list endpoints and silently wrong for the log stream: a
chunked endpoint that never returns would hang, and one that does would deliver
its whole output in a single read, so a screen consuming it would look like it
worked while testing nothing. A pipe plus a goroutine running `ServeHTTP` is
about thirty lines and streams properly.

### 5. The world advances on wall-clock time, not on requests.

A job started at T finishes at T+3s because three seconds passed, not because
four requests arrived.

Two reasons, and the second is the one that bites. A `curl` in another terminal
must see the same world the TUI sees, or the fixture is useless for the "poke
it by hand while the TUI is open" workflow that is half its value. And a
request-counted clock makes the fixture's behaviour depend on the polling
interval of whatever is watching it, so turning `pkg/poll` up to inspect
something changes the thing being inspected.

### 6. `Options{Seed, Now}` — determinism where it is wanted, liveliness where it is not.

```go
type Options struct {
    Seed int64             // 0 = time-seeded
    Now  func() time.Time  // nil = time.Now
    // …
}
```

A test pins both and gets a world that transitions on exactly the tick it
chooses. The binary passes neither and gets something that looks alive. Same
code path, and the fixture never grows a "test mode" whose behaviour diverges
from the mode people actually look at.

### 7. Latency and failure are per-request, not server configuration.

```
GET /apps?offset=200&limit=100&latency=900ms
GET /apps?offset=0&limit=100&fail=503
GET /jobs?flaky=0.3
```

This is the decision that makes the fixture worth building rather than merely
tidy. **Server-wide latency cannot express the race the library is designed
against**, which is "the reply to the *previous* query is slow and the reply to
the *current* one is fast". One knob for the whole server makes every request
slow together, and the out-of-order arrival never happens. Per-request, a test
constructs it in two lines:

```go
slow := get("/apps?q=eu&latency=900ms")   // the abandoned filter
fast := get("/apps?q=euro")                // what the user actually typed
// fast lands first; slow must be dropped by source.Deliver's generation check
```

`fail` returns that status with a JSON error body. `flaky` fails that fraction
of requests, for exercising a retry policy or just watching a screen's error
path under load.

**Rejected: a control endpoint** (`POST /_control/latency`). It is server-wide
state by another name, it makes tests order-dependent, and two tests running
against one fixture would fight.

### 8. Two nouns, and resist the third.

`apps` and `jobs`. Between them they cover paging, filtering, faceting, sorting,
a detail document deep enough for `inspector`, a mutation that returns 202, a
status that transitions over time, and a stream.

| Endpoint | What it is for |
|---|---|
| `GET /apps` | `pkg/source`, `table` `FilterRemote`/`SortRemote`, `SetWindow` placeholders |
| `GET /apps/facets?field=` | `SetDistinct` — the "don't scrape completions from one page" anti-pattern |
| `GET /apps/{id}` | `inspector.FromMap`, `pkg/tree`, nested JSON |
| `POST /apps/{id}/sync` | `action.Run`; returns `202` and a job id |
| `GET /jobs` | `pkg/poll`, activity's derived layer, `ActivityRevision` |
| `GET /jobs/{id}/log` | `logview` streaming, `MaxLines`, chained reads |

A fixture that grows a noun per demo becomes a second product to maintain. If a
demo needs a shape these two do not have, the question to ask first is whether
the demo is really about that shape.

### 9. Errors are HTTP status codes and a JSON body.

```json
{"error": "application \"nope\" not found"}
```

Not a Go error type, not a sentinel the caller compares against. The consumer
of this is a screen holding an `*http.Client`, and the thing it must learn to
handle is the thing a real API sends it. A typed error would be a nicer Go API
and would teach the wrong lesson.

### 10. The log endpoint streams, slowly, and bounded.

`GET /jobs/{id}/log` writes chunked output at a few lines a second while the
job runs, then closes. Fast enough to watch, slow enough that a screen has to
actually handle partial arrival rather than receiving one blob.

It is the only endpoint that has to work over the in-process transport as a
*stream* (decision 4), and the only one where `logview`'s `MaxLines` cap means
anything.

**A `?rate=` knob** turns it into the "unbounded producer" case rule 13 warns
about, which is otherwise tedious to construct.

### 11. Bound to localhost, no auth, and it says so.

The binary defaults to `127.0.0.1:8099` and does not take `0.0.0.0` without an
explicit flag. It has no authentication and it has deliberate failure-injection
knobs — a combination that is fine on a loopback interface and an obvious
foot-gun on a shared host.

The package doc says, in the first paragraph, that this is a fixture and must
not be deployed. Not because anyone plans to, but because "the demo server" is
exactly the sort of thing that ends up in a compose file.

### 12. The Go API is stable; the wire shape is not.

Worth being explicit, because putting this in the main module makes it public
surface under the module's normal compatibility expectations.

`New`, `Options` and `Client` are covered by that. **The JSON shapes and paths
are not** — they will move as the examples that consume them move, and the
examples move in the same commit. A downstream project pins a tuilib version
and gets a matched pair; one that wants to skip a version and keep its client
code is on its own, and the release notes should say so the first time a shape
changes.

### 13. The examples that already fake a backend migrate onto it.

`examples/patterns/remote` loses its 5,000-row generator, its page arithmetic
and its `time.Sleep`. `examples/patterns/activity` loses its `backend` type,
its mutex and its job model. `examples/patterns/poll` loses its synthetic job
churn.

This is the argument that the fixture is not only for the other project. Three
demos that each exercise their own private simulation become three clients of
one, which means the fixture is exercised from three directions and a bug in it
shows up somewhere.

**It also makes the demos more honest.** `examples/patterns/remote`'s package
comment claims it demonstrates the windowed-source loop against a paged API;
what it currently demonstrates is that loop against a slice and a sleep in the
same goroutine. Nothing in it crosses a request boundary, which is where the
interesting failures live.

### 14. It does not replace `internal/componenttest`.

Component contracts stay where they are: synchronous, no I/O, no clock. A test
that asserts "a marked row survives a keyed swap" has no business starting a
server, and would be slower and flakier for it.

`demoapi` is for the layer above — a *screen* wired to a *source* talking to
something that behaves like a server. Different question, different tool. The
line: if the assertion can be written by calling a setter, it belongs in
`componenttest`.

---

## API surface

### `demoapi` (new)

```go
// New returns the fixture as an http.Handler.
func New(opts Options) http.Handler

type Options struct {
    // Seed makes the generated world and its scheduled events reproducible.
    // Zero seeds from the clock.
    Seed int64

    // Now is the clock the world advances on. Nil means time.Now; a test
    // passes a controllable one and drives transitions exactly.
    Now func() time.Time

    // Apps is how many applications to generate. Zero means 5000, which is
    // enough for the windowed table to be windowed.
    Apps int

    // Latency is the floor applied to every request, before any per-request
    // ?latency=. Zero means none.
    Latency time.Duration
}

// Client returns an *http.Client that dispatches into h without a socket.
// Streaming endpoints stream; see decision 4.
func Client(h http.Handler) *http.Client
```

### Wire shape (not stable — decision 12)

```
GET  /apps?offset=&limit=&q=&sort=&desc=&latency=&fail=&flaky=
     → {"total": 5000, "offset": 200, "rows": [{"id","name","region","sync","health","rev"}]}

GET  /apps/facets?field=region        → {"values": ["eu-west", "us-east", …]}
GET  /apps/{id}                       → a nested document, several levels deep
POST /apps/{id}/sync                  → 202 {"job": "j-1f4"}
POST /apps/{id}/refresh               → 202 {"job": "j-1f5"}   (completes at once)

GET  /jobs?app=                       → [{"id","app","kind","status","started","finished","rev"}]
GET  /jobs/{id}/log?rate=             → chunked text/plain
```

`rev` on an app changes whenever a job against it completes. That is the field
`activity`'s `ActivityRevision` watches, and the reason it is on the wire
rather than derived: a fixture that could not demonstrate the
finished-between-two-polls case would miss the one thing that case needs.

### `Taskfile.yml` (addition)

```yaml
  server:
    desc: Run the demo API on 127.0.0.1:8099
    cmds:
      - go run ./demoapi/cmd/demoapi
```

---

## Implementation order

1. **`demoapi/state.go`** — the world, the clock, seeded generation, job
   transitions. No HTTP. Unit-testable on its own, and the part most likely to
   be wrong.
2. **`demoapi/apps.go`** — list with offset/limit/filter/sort, facets, detail.
   Paging arithmetic and filter semantics have to match what `pkg/query` parses,
   or the fixture teaches a grammar the library does not implement.
3. **`demoapi/demoapi.go`** — `New`, `Options`, the mux, the `latency`/`fail`/
   `flaky` middleware.
4. **`demoapi/client.go`** — the in-process transport, over `io.Pipe`.
5. **`demoapi/jobs.go`** — POST endpoints, job list, the log stream.
6. **`demoapi/cmd/demoapi`** + the `task server` entry.
7. **Migrate `examples/patterns/remote`**, which is the demo whose current
   simulation is furthest from what it claims to demonstrate.
8. **Migrate `examples/patterns/activity`** and **`poll`**.
9. **The race tests** — the two orderings in the Problem section, which are the
   reason to build any of this.

Steps 1-6 are the fixture. 7-9 are the payoff, and 9 is the one that finds
bugs.

## Rejected, worth remembering

- **A recorded fixture — golden JSON files replayed.** Cannot express a
  transition over time, cannot accept a POST, cannot be slow on purpose. Every
  interesting thing here is about *when*, not *what*.
- **A control endpoint for latency and failure.** Server-wide state by another
  name; makes tests order-dependent and mutually hostile. Per-request instead
  (decision 7).
- **`internal/demoapi`.** Natural neighbour to `internal/componenttest`, and
  unimportable by the project that asked for this.
- **A nested module from day one.** The right answer *if* a third-party
  dependency is ever needed, and pure ceremony until then: separate tags, a
  separate `require`, and version skew between a fixture and the library it is
  a fixture for.
- **A typed Go error surface.** Nicer to consume and teaches the wrong lesson;
  the screen must learn to handle a status code (decision 9).
- **Growing a noun per demo.** A fixture with eight resources is a second
  product. Two, and push back on the third (decision 8).
- **`httptest.NewRecorder` for the in-process client.** Buffers whole
  responses, so the streaming endpoint would appear to work while testing
  nothing (decision 4).
- **Replacing `internal/componenttest` with it.** Different question, and a
  component contract that needs a server to assert is a component contract
  written wrong (decision 14).

## Tests

**The fixture needs its own**, because everything downstream trusts it:

- Paging arithmetic at the boundaries: `offset` past `total`, a final short
  page, `limit=0`.
- Filter and sort semantics matching `pkg/query`'s grammar — a bare substring,
  a `key:value` scope, a `~regex`. If the fixture and the parser disagree, the
  examples teach a grammar the library does not implement.
- Facet values are the *whole* distinct set, not the current page's — the
  entire point of the endpoint.
- Job transitions at a pinned clock: a job started at T is running at T+1 and
  finished at T+4, exactly.
- `rev` changes when and only when a job completes.
- `latency`, `fail` and `flaky` each do what they say, and `latency` on one
  request does not delay a concurrent one.
- The in-process client streams the log endpoint incrementally rather than in
  one read.

**The tests the fixture makes possible**, which are the reason for it:

- A slow reply to an abandoned filter arriving after a fast reply to the
  current one is dropped, and the table shows the current filter's rows —
  `source.Deliver`'s generation check, against a real out-of-order arrival
  rather than a hand-built message sequence.
- A poll already in flight when an action is launched returns the pre-click
  value and does not clear the row's indicator — activity's local-beats-derived
  rule, decision 18 of `docs/activity.md`.
- An action that dispatches and returns before the next poll keeps its row
  moving until that poll lands — the handoff, decision 19.
- A job that starts and finishes between two polls flashes rather than passing
  unnoticed — decision 20, which needs a server whose clock does not wait for
  the client.

## Open questions

**1. Should the other project pin tuilib as a whole, or is it worth the nested
module now?** Pinning the whole library is simplest and is what decision 2
assumes: one `require`, one version, no skew. The nested module buys a cleaner
boundary and the freedom to give the fixture dependencies, at the cost of
another tag to remember on every release and the possibility of a fixture built
against a different tuilib than the one importing it. Recommendation: single
module until something forces the split, since the split is easy later and
un-splitting is not.

**2. Argo/AWX-shaped, or deliberately generic?** The concrete shape makes
better demos — `sync`/`health`/`OutOfSync` reads as a real screen, and it
matches what these TUIs are actually built for. The generic shape
(`items`/`status`) ages better and does not imply the library is for
deployments. Leaning concrete, on the grounds that every example in the repo is
already concrete about something and none of them has been mistaken for a
claim about scope.

**3. Does the fixture need a WebSocket or SSE endpoint?** Rule 12 describes the
chained-`tea.Cmd` pattern for "an SSE response, a tailed file, a websocket" and
nothing demonstrates it. Chunked HTTP (decision 10) exercises the same client
shape with none of the protocol, so this is probably a no — but if a demo is
ever written for rule 12 specifically, this is where the server side would go.

**4. Should `task examples` be able to point at a real server?** An env var
(`TUILIB_DEMO_API=http://…`) would let the same examples run against the
in-process handler by default and a real one when set, which is a nice way to
watch two clients share a world. Cheap to add, easy to skip, and nothing else
depends on the answer.
