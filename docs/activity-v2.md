# Row activity v2 — proposal

Status: **prototyped on `pkg/table`**, alongside v1. It restates the feature
around the model a developer already has, and moves the parts a screen can get
wrong into the library. See [Prototype notes](#prototype-notes) for what the
build changed.

## The model

> The server is the source of truth. When the user triggers work locally, the
> row is told immediately, and that claim lasts until the truth can take over.

Everything below is one of two concepts:

1. **Observation** — a predicate over each row's data says which rows are busy.
2. **Operation** — a local action opens a claim on some keys. *How the claim
   ends* depends on the shape of the remote API, and that is the one thing the
   developer has to decide.

## Operation shapes

v1 assumes every local action is fire-and-observe. Real APIs aren't:

| Shape | Example | Who knows when the work ends | Claim mode |
|---|---|---|---|
| Fire-and-observe | Argo `POST /applications/{n}/sync` — returns at once, the controller runs the operation | the server's status field | `activity.Observed` |
| Blocking | Argo `GET /applications/{n}?refresh=normal` — the connection is held until the refresh is reconciled; a synchronous RPC; a `runner.Capture` | the request itself | `activity.Held` |
| Job handle | `POST` → `202 {job: "j-1"}`, then `GET /jobs/j-1` | the job resource | `activity.Held` |
| External | auto-sync, someone else's CLI, a cron job | the server's status field | none — observation only |

**Observed**: the claim ends when observations say so — confirmed by a busy
read and then settled, or ended by a read showing a different settled value, or
running out after `ConfirmWithin` reads that showed nothing. The request's
return is just the acknowledgement.

**Held**: the claim ends when the screen calls `Done`. Observations cannot
retire it. This is not a grace timer: an open request *is* the server not
having answered yet. A job handle is the same mode, but the screen calls `Done`
when the job reaches a terminal state instead of when the request returns.

In both modes, if a read reports the key busy, the row shows the server's label
instead of the local guess. After a held claim ends, a server that still
reports the key busy keeps it spinning. For example, a refresh can trigger an
auto-sync.

v1 cannot express Held at all. A blocking refresh under v1 gets its claim
retired by the next poll, which still says `Synced`, while the request is still
open. The spinner stops half way through the refresh.

## API difference

| v1 (today) | v2 |
|---|---|
| `ActivityWhen func(Row) (label, busy)` — sees display cells only; a status you don't show needs a `Hidden` column | `BusyWhen func(KeyedRow) (status, busy)` — sees `KeyedRow.Data any`, the screen's own record |
| predicate's `label` only meaningful when busy | `status` is always the current value; `busy` says whether it's in progress |
| `Settle` compares the **ActivityColumn cell** | `Settle` compares the **predicate's status** — whatever it reads |
| `SetBusy(map)` — second entrance, panics with `ActivityWhen` | removed — put `/jobs` results in `Data`; one predicate |
| `Expect(keys, label)` | `op, cmd := Dispatch(keys, label, activity.Observed \| activity.Held)` |
| screen tracks `claimed map[tag]keys` | `activity.Op` rides your message |
| `Retract(keys...)` on a failed write | `Done(op, err)` — err retracts, nil acknowledges |
| screen's `gen`, `seen`, `writing` counters | `rd := BeginRead()` |
| `if m.gen < s.seen \|\| s.writing > 0 { drop }` + `SetKeyedRows` | `if !ApplyRead(m.read, rows, m.err) { drop }` |
| `RetractAll()` on a failed read | done by `ApplyRead(rd, nil, err)` — observed claims retract, held claims stay |
| a claim that retires unseen is silent | `activity.UnobservedMsg{Keys, Label}` |
| `ActivityColumn`, column `Width`, `ActivityState`/`SetActivityState` | unchanged |

**Rule moved into the library.** v1 drops a read taken across an unacked
write *entirely* (`writing > 0`), which freezes every row for the POST's
latency. v2 applies the read. It just doesn't let the read count against a
claim whose write was unacknowledged when the read began. `BeginRead` records
that point, which is why the library can enforce the rule and the screen
doesn't need to know it.

**Streams need no token.** A watch/SSE event is taken when it arrives, and
events arrive in order. Call `SetKeyedRows` without `BeginRead`/`ApplyRead`, and
the library treats the observation as taken now.

## Examples

All examples are for a `pkg/table` Argo applications screen. `list` and `tree`
get the same methods.

### Setup, shared by every example

```go
type argoApp struct {
    Name   string
    Sync   string // status.sync.status: Synced | OutOfSync | Unknown
    Health string // status.health.status
    Phase  string // status.operationState.phase: Running | Terminating | Succeeded | Failed | Error | ""
}

o := th.Table()
o.Columns = []table.Column{
    {Title: "Name", Flex: 1},
    {Title: "Sync", Width: 14},   // wide enough for "⣾ Refreshing"
    {Title: "Health", Width: 12},
}
o.ActivityColumn = "Sync"
o.Activity.Settle = 2 // the Argo controller takes a read or two to pick an op up
o.BusyWhen = func(r table.KeyedRow) (string, bool) {
    a := r.Data.(argoApp)
    return a.Phase, a.Phase == "Running" || a.Phase == "Terminating"
}

func (s *Screen) rows() []table.KeyedRow {
    out := make([]table.KeyedRow, 0, len(s.apps))
    for _, a := range s.apps {
        out = append(out, table.KeyedRow{Key: a.Name, Cells: []string{a.Name, a.Sync, a.Health}, Data: a})
    }
    return out
}
```

The predicate watches `operationState.phase` and the row draws in Sync. That
split is the one v1 made you discover.

### Polling, shared by every example that polls

v1:

```go
func (s *Screen) fetch() tea.Cmd {
    s.gen++
    gen := s.gen
    return func() tea.Msg { apps, err := s.api.List(); return fetchedMsg{gen, apps, err} }
}

case fetchedMsg:
    if m.gen < s.seen || s.writing > 0 { break }
    s.seen = m.gen
    if m.err != nil { s.table.RetractAll(); cmds = append(cmds, app.ErrorOf(m.err)); break }
    s.apps = m.apps
    s.table.SetKeyedRows(s.rows())
```

v2:

```go
func (s *Screen) fetch() tea.Cmd {
    rd := s.table.BeginRead()
    return func() tea.Msg { apps, err := s.api.List(); return fetchedMsg{rd, apps, err} }
}

case fetchedMsg:
    if !s.table.ApplyRead(m.read, rowsOf(m.apps), m.err) {  // out of order, or failed
        if m.err != nil { cmds = append(cmds, app.ErrorOf(m.err)) }
        break
    }
    s.apps = m.apps
```

### 1. Sync — fire-and-observe

```go
case action.ChosenMsg: // "Sync"
    op, cmd := s.table.Dispatch(m.Targets, "Syncing", activity.Observed)
    return s, tea.Batch(cmd, s.postSync(op, m.Targets))

func (s *Screen) postSync(op activity.Op, names []string) tea.Cmd {
    return func() tea.Msg { return syncedMsg{op, s.api.Sync(names)} }
}

case syncedMsg:
    s.table.Done(m.op, m.err) // nil: acknowledged, observations take over
                              // err: claim retracted
    if m.err != nil { return s, app.ErrorOf(m.err) }
    return s, app.Info("Sync requested")

case activity.UnobservedMsg:
    // The claim ran out of reads and never saw Running: the sync finished
    // between two polls. This is the case that looked like nothing happened.
    return s, app.Info(m.Label + " finished before it was observed")
```

The v1 version of this case is the four counters and the `claimed` map in
`examples/patterns/activity` lines 194–248.

### 2. Refresh — blocking request

```go
case action.ChosenMsg: // "Refresh"
    op, cmd := s.table.Dispatch(m.Targets, "Refreshing", activity.Held)
    return s, tea.Batch(cmd, s.refresh(op, m.Targets))

func (s *Screen) refresh(op activity.Op, names []string) tea.Cmd {
    return func() tea.Msg {
        // GET /api/v1/applications/{n}?refresh=normal — returns once reconciled
        apps, err := s.api.Refresh(names)
        return refreshedMsg{op, apps, err}
    }
}

case refreshedMsg:
    s.table.Done(m.op, m.err)
    if m.err != nil { return s, app.ErrorOf(m.err) }
    return s, s.poll.Refresh() // pick up the reconciled state now
```

Polls keep landing during the refresh. They update every row, including this
one's Health, but can't end the claim. Don't apply `m.apps` directly: the
response is one app out of a set another poll may already have overtaken, and
`poll.Refresh()` puts it back on the ordered path. (The alternative is
`Accept(op.Read(), nil)`; see the open questions.)

### 3. Job handle

```go
case action.ChosenMsg: // "Run playbook"
    op, cmd := s.table.Dispatch(m.Targets, "Queued", activity.Held)
    return s, tea.Batch(cmd, s.launch(op, m.Targets))

case launchedMsg: // POST returned 202 {job}
    if m.err != nil { s.table.Done(m.op, m.err); return s, app.ErrorOf(m.err) }
    s.jobs[m.job] = m.op
    return s, nil

case jobsMsg: // the screen's own poll of GET /jobs?id=...
    for id, st := range m.jobs {
        if op, ok := s.jobs[id]; ok && st.Terminal() {
            s.table.Done(op, st.Err())
            delete(s.jobs, id)
        }
    }
```

The row can say more than "Queued" by putting the job's phase in `Data`, so the
predicate reports it busy with the job's own label. `Held` only decides who
ends the claim.

### 4. Read-only dashboard — external work only

```go
o.ActivityWhen = /* the predicate above */
// poll + BeginRead/ApplyRead. No Dispatch, nothing else.
```

### 5. Watch stream

```go
case appEventMsg: // one event from /api/v1/stream/applications
    s.apps[m.app.Name] = m.app
    s.table.SetKeyedRows(s.rows()) // taken now; no BeginRead/ApplyRead
    return s, s.nextEvent()
```

## What a developer has to decide

1. **What the predicate reads**: the field that is busy while work runs. For
   Argo that is `operationState.phase`, not `sync.status`.
2. **For each verb, who knows when it ends**: the status (`Observed`) or the
   request/job (`Held`).
3. **`Settle`**, only for `Observed` on a reconciler: 1–2.

Everything else is either unchanged (`ActivityColumn`, `Width`) or done by the
library.

## Still not covered

- **Work that ends between polls under `Observed`** is still invisible on the
  row. v2 *tells the screen* (`UnobservedMsg`) instead of staying silent. The
  only other fix is to make the verb `Held` by following it with a blocking
  wait or a job poll.
- **Staleness during an outage (G1).** `Accept` knows reads are failing, so
  it could expose `Stale() bool` for a title suffix. That is proposed, not
  designed.
- **A settled status the predicate doesn't know (G3)** still spins under a
  `Settled(...)` list. v2's predicate is plain code over `Data`, which makes
  the escape a line of Go instead of a switch to `SetBusy`.

## Fixture changes (`demoapi`)

The demo has to model the shapes above, or the example teaches the wrong one
again:

- Split `sync` (`Synced`/`OutOfSync`) from `operationState.phase`. Delete the
  invented `Syncing` sync status.
- `GET /apps/{id}?refresh=1` holds the connection until reconciled, with a
  `?takes=` knob and a server-side timeout.
- Keep `/jobs` for the job-handle example.
- A `?quick=1` sync that finishes inside one poll interval, so `UnobservedMsg`
  has a scenario test.

## Open questions

1. **Should a Held op's response be accepted as an observation?** It's a
   read taken after the write, so it is the freshest data the screen has. But
   it's usually a subset of the rows. `Accept(op.Read(), nil)` plus a per-key
   upsert would allow it; example 2 avoids the question with `poll.Refresh()`.
2. **Should `pkg/action` dispatch for you?** An `Action.Claim` label plus
   `Action.Mode` would let the shell call `Dispatch`/`Done` around `Run`. That
   would remove example 1's two cases, but the shell would need to know which
   component holds the targets.

## Prototype notes

What was built, and where it differs from the proposal above.

**Where it lives.**

- `pkg/activity/op.go`: `Mode`, `Op`, `Read`, `UnobservedMsg`, and
  `Set.Dispatch`/`Done`/`BeginRead`/`Accept`.
- `pkg/table/activity.go`: the table wrappers.
- `pkg/table/table.go`: `Options.BusyWhen` and `KeyedRow.Data`.

v1 (`ActivityWhen`, `Expect`, `Retract`, `RetractAll`, `SetBusy`, `Derive`) has
since been removed. `pkg/list` and `pkg/tree` carry the same surface as the
table; the shared logic moved into `activity.Set.ObserveRows`, and the `Set`
now keeps each key's last status itself, so `Dispatch` takes no `at` map.

**Names.**

- The predicate is `BusyWhen`, not a changed `ActivityWhen`, so the two could
  coexist during the prototype.
- `ConfirmWithin` stayed `Settle`. Only its comparison changed: it now reads
  the predicate's status.

**One clock.** Every `BeginRead` and every `Done` ticks a counter in the
`Set`. A read counts against an Observed claim only if it was issued at or
after that claim's `Done`. A stream observation (`SetKeyedRows` with no
`Accept`) is taken at the current tick. That comparison replaces the
screen's `gen`, `seen` and `writing` counters.

**Held needs less than expected.** A Held claim is never acknowledged: `Done`
deletes it. The ack-ordering rule therefore already stops observations from
ending it. The explicit `mode == Held` skip in `retire` is redundant, and
removing it breaks no test. In practice the mode only decides two things:
whether `Done` acknowledges or ends the claim, and whether a busy read
supersedes the claim or sits on top of it.

**`UnobservedMsg` is reported for every Observed `Dispatch`** that ends
without a read seeing it busy.

**A tree can now see a claim's work happen.** Under v1 a tree compared nothing,
since a node's label is its identity. `BusyWhen` returns a status per node, so
a tree ends a claim on a changed status the way list and table do.

**Tests.**

- `pkg/activity/op_test.go`: the state machine.
- `internal/integration/activity_ops_test.go`: v1's ordering cases rewritten
  with no guard in the screen, plus a blocking refresh and a sync that
  finishes between polls, run against `demoapi`. Removing the ordering rule
  from `retire` fails the two ordering tests.

**`demoapi`** now models Argo:

- `sync` is Synced or OutOfSync only. Work in progress shows in `phase`
  (Running, then Succeeded or Failed).
- `GET /apps/{id}?refresh=normal` holds the connection (`?takes=`, default
  2s) and changes nothing on the app until it answers.
- `GET /jobs/{id}` resolves a job handle.
- Some apps start drifted, and scheduled events drift more, so a refresh
  has something to find.
- The fixture used to invent a `Syncing` status. Two v1 integration tests
  had been passing because of it: their "read in flight" came back after the
  server had already reported the work. They now read `phase`.

**The example** (`examples/patterns/activity`) has one verb per shape:

- Sync: Observed.
- Refresh: Held, blocking GET.
- Sync and follow: Held, job handle.
- Quick sync: Observed, finishes between polls, so the screen gets an
  `UnobservedMsg`.
- Fail a sync: Observed.

It holds no counters. Its only per-screen state is `ops`, a map from each
run's tag to its `Op`.

**Not done yet:**

- `list` and `tree`.
- Removing v1.
- Rewriting CLAUDE.md rule 33 and `docs/activity.md`.
- Open questions 1–2 above.
