# Row activity: developer guide

Show a spinner on the rows of a `pkg/table`, `pkg/list` or `pkg/tree` that
are being worked on, whether the work was started by the user or by something
else. The examples use a table; [list and tree](#list-and-tree) differ only in
the row type and where the spinner draws.

```
  Name                     │ Sync           │ Health
  ──────────────────────────────────────────────────
▸ payments-metrics-00      │ ⣾ Running      │ Healthy
  search-web-01            │ OutOfSync      │ Degraded
  media-web-02             │ ⣽ Refreshing   │ Progressing
  media-gateway-03         │ Synced         │ Healthy
```

## How it works

1. **The server is the source of truth.** A row spins when the data you
   fetched says work is running on it, and stops when the data says the work
   is over.
2. **Work the user starts shows immediately.** Between the keypress and the
   next poll, the data can't know yet. The table covers that gap with a
   *claim*: the row spins at once, and the claim ends when the truth can take
   over.

Everything below is one of those two, plus the plumbing that keeps them
honest on a real network.

## Step 1: show work the server reports

This is all a read-only dashboard needs.

```go
type argoApp struct {
    ID, Name, Sync, Health, Phase string
    Rev                           int64
}

o := th.Table()
o.Columns = []table.Column{
    {Title: "Name", Flex: 1},
    {Title: "Sync", Width: 14},   // wide enough for "⣾ Refreshing"
    {Title: "Health", Width: 12},
}
o.ActivityColumn = "Sync"         // where the spinner draws
o.BusyWhen = func(r table.KeyedRow) (string, bool) {
    phase := r.Data.(argoApp).Phase
    return phase, phase == "Running"
}
s.table = table.New(o)
```

Fetch with a token taken when the request is issued, and apply the reply with
`ApplyRead`:

```go
type fetchedMsg struct {
    read activity.Read
    apps []argoApp
    err  error
}

func (s *Screen) fetch() tea.Cmd {
    rd := s.table.BeginRead()
    return func() tea.Msg {
        apps, err := s.client.ListApps()   // check the HTTP status in here
        return fetchedMsg{rd, apps, err}
    }
}

case fetchedMsg:
    rows := make([]table.KeyedRow, len(m.apps))
    for i, a := range m.apps {
        rows[i] = table.KeyedRow{Key: a.ID, Cells: []string{a.Name, a.Sync, a.Health}, Data: a}
    }
    if !s.table.ApplyRead(m.read, rows, m.err) && m.err != nil {
        return s, app.ErrorOf(m.err)
    }
```

Drive `fetch` from a `pkg/poll` tick (rule 24).

**Choosing the field for `BusyWhen`.** It has to be a field that says
"working" *while the work runs*. It doesn't have to be a column: it's read from
`Data`.

| API | Busy while | Not this |
|---|---|---|
| Argo CD | `status.operationState.phase == Running` | `sync.status`: that's a comparison, and it never says Syncing |
| Kubernetes Deployment | `observedGeneration < generation`, or a rollout still in progress | `status.conditions` alone |
| A jobs API | `status in (queued, running)` | the resource the job acts on |
| Busy-ness from another endpoint (`GET /jobs?status=running`) | merge it into `Data` before you push rows | (there is no second entrance) |

`BusyWhen` returns the status for **every** row, busy or not. The table shows it
as the label while the row is busy, and compares it to notice when the server
acted. `activity.Busy("running", "pending")` and
`activity.Settled("succeeded", "failed")` build the common predicates.

## Step 2: add verbs the user can run

For each verb, answer one question: **who knows when the work ends?**

| The work ends when… | Mode | Example |
|---|---|---|
| the server's status says so; the request only acknowledges | `activity.Observed` | Argo sync `POST` → `202` at once |
| the request answers | `activity.Held` | Argo refresh `GET` that holds the connection |
| a job id the request returned says so | `activity.Held` | `POST` → `{job}`, then poll `GET /jobs/{id}` |

Every verb has the same shape: `Dispatch` when the user picks it, and `Done`
when the request answers.

### Observed: a POST that answers at once

```go
func (s *Screen) Actions() action.Set {
    keys := s.table.Selection()
    return action.Set{Targets: keys, Actions: []action.Action{
        {Label: "Sync", Do: func() tea.Cmd { return s.sync(keys) }},
    }}
}

type answeredMsg struct {
    op  activity.Op
    err error
}

func (s *Screen) sync(keys []string) tea.Cmd {
    op, spin := s.table.Dispatch(keys, "Running", activity.Observed)
    return tea.Batch(spin, func() tea.Msg {
        return answeredMsg{op, s.client.Sync(keys)}   // 202 = nil error
    })
}

case answeredMsg:
    s.table.Done(m.op, m.err)   // error: the spinner stops; nil: polls take over
    if m.err != nil { return s, app.ErrorOf(m.err) }
    return s, app.Info("Sync requested")   // not "completed": the work has only started
case activity.UnobservedMsg:
    return s, app.Info("Sync finished between polls")
```

A `Do` verb gets no receipt from the shell (`Action.Receipt` applies only to
`Run` verbs), so post "requested" yourself, as above.

`UnobservedMsg` arrives through the table's `Update`, on the turn after the read
that ended the claim. `ApplyRead` and `Done` are setters and return nothing.
Forward every message to the table (rule 6) and match it in your `Update`.

Claim with the server's own word (`"Running"`), so the row reads the same from
the keypress to the poll that confirms it.

### Held: a request that blocks until the work is done

```go
func (s *Screen) refresh(keys []string) tea.Cmd {
    op, spin := s.table.Dispatch(keys, "Refreshing", activity.Held)
    return tea.Batch(spin, func() tea.Msg {
        return refreshedMsg{op, s.client.Refresh(keys)}   // blocks ~2s
    })
}

case refreshedMsg:
    s.table.Done(m.op, m.err)   // the spinner ends here, not at a poll
    return s, s.poll.Refresh()  // show what the refresh found now
```

### Held: a job handle

The same as a blocking request. The command that follows the job only returns
once the job has finished:

```go
return tea.Batch(spin, func() tea.Msg {
    id, err := s.client.StartSync(key)
    if err == nil {
        err = s.client.WaitJob(id)   // poll GET /jobs/{id} until terminal
    }
    return jobDoneMsg{op, err}
})
```

## Theme swaps

`SetTheme` rebuilds the table (rule 4), so carry the rows and the activity
across it:

```go
func (s *Screen) SetTheme(t theme.Theme) {
    rows, act := s.table.KeyedRows(), s.table.ActivityState()
    o := t.Table()
    // … the same options as before …
    s.table = table.New(o)
    s.table.SetKeyedRows(rows)         // rows first
    _ = s.table.SetActivityState(act)  // then the spinners that belong on them
}
```

`SetActivityState` returns the spinner's first tick, and `SetTheme` has nowhere
to return it. Dropping it is safe: the table re-arms a stalled spinner on the
next message it receives.

## What the user sees

| Situation | Row shows |
|---|---|
| The server reports the row working | `⣾ Running` (the server's word), animated |
| The user just picked a verb, no read yet | `⣾ Running` / `⣾ Refreshing` (your label), animated |
| A read confirms the work | the server's label; your claim steps aside silently |
| The work ends | the cell's own value again (`Synced`, `OutOfSync`, …) |
| Work finished before any poll saw it | the row stops, and you get `UnobservedMsg` |
| Reads are failing | rows busy on the last good read show a static `? Running` in the muted colour; open requests keep spinning |
| The request was refused (`Done` with an error) | the row stops immediately |

The table draws no outcome glyph (✓ / ✗). When the spinner stops, the cell
shows the row's own value, which already says `Failed` in your app's colours.

## Settings

| Option | Default | Set it when |
|---|---|---|
| `ActivityColumn` | none (a 2-cell gutter) | always; name the column the spinner replaces |
| `Column.Width` on that column | content-auto | always; the indicator never widens a column |
| `BusyWhen` | none | always |
| `Revision` | none | your API has a value that moves when an operation *finishes* (Argo `operationState.finishedAt`). Without it, a sync that starts and ends at the same status (Succeeded → Succeeded) waits out `Settle` and reports `Changed: false`. |
| `Activity.Settle` | 0 | the server reports new work after a lag (a reconciler such as a real Argo controller): 1–2. Leave it at 0 when the server reports the work inline, as demoapi does; the examples leave it at 0. It counts reads that repeat the pre-dispatch status before the claim gives up. It's a number of reads, not a duration. |

## Messages and methods

| | |
|---|---|
| `table.Dispatch(keys, label, mode) (activity.Op, tea.Cmd)` | start a claim; batch the returned command (it's the spinner's first tick) |
| `table.Done(op, err)` | the request answered |
| `table.BeginRead() activity.Read` | stamp a fetch when it is issued |
| `table.ApplyRead(rd, rows, err) bool` | apply a reply; false means it was dropped (stale or failed) |
| `table.SetKeyedRows(rows)` | apply a *stream* event (watch/SSE); no token needed |
| `table.ReadsFailing() bool` | the newest read failed; put it on the title |
| `table.ActivityCount() int` | how many rows are working; for the title |
| `table.KeyedRows()` | the rows as last applied, `Data` included; for a `SetTheme` rebuild (list: `KeyedItems()`, tree: `Root()`) |
| `table.ActivityState()` / `SetActivityState(s)` | carry state across `SetTheme` (rule 4) |
| `activity.UnobservedMsg{Op, Keys, Label, Changed}` | an Observed verb ended without any read seeing it busy |

## What the table does for you

You don't need to write any of this:

- **Out-of-order replies.** A reply overtaken by a newer one is dropped.
- **Reads that race your own write.** A poll issued before your POST answered
  is applied, but it can't end that POST's spinner. That's where the "row
  blinks off and back on" bug came from.
- **Outages.** A failed read ends claims that were waiting on a read, and marks
  observed work Unknown instead of leaving it animating.
- **Refused requests.** `Done` with an error stops the spinner at once.

## What it can't do

- **See work somebody else started and finished between two polls.** With
  `Revision` set, the row's revision moves, but the table only reports that
  for operations *you* dispatched.
- **Repair a read path that goes backwards.** A replica or cache that answers
  busy, then settled, then busy again is telling the truth each time; no
  client-side ordering fixes that.
- **Windowed tables.** Under `SetWindow` rows have no keys, and activity is
  inert.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| No row ever spins | `BusyWhen` reads a field that is never busy while work runs (Argo `sync.status`), or rows were pushed with `SetRows` instead of keyed rows |
| The row spins, blinks off, then on again | the fetch skipped `BeginRead` / `ApplyRead` and used `SetKeyedRows` |
| Refresh stops spinning after the first poll | it was dispatched `Observed`; a blocking request is `Held` |
| Sync stops the moment the 202 lands | it was dispatched `Held`; a POST that hands work off is `Observed` |
| A quick sync looks ignored | handle `activity.UnobservedMsg`; set `Revision` if the status ends where it began |
| The spinner shows `⣾` with no word | the activity column is too narrow for the label; give it a `Width` |
| The table blanks during an outage | the fetch decoded an error body as an empty list; check the HTTP status and return an error |

## List and tree

The same methods, with the component's own row type:

| | `pkg/table` | `pkg/list` | `pkg/tree` |
|---|---|---|---|
| Row type | `table.KeyedRow{Key, Cells, Data}` | `list.KeyedItem{Key, Display, Data}` | your `tree.Node` |
| Key | `Key` | `Key` | the node's path (`"cluster/web"`) |
| `BusyWhen` reads | `KeyedRow` | `KeyedItem` | `tree.Node` |
| Apply a read | `ApplyRead(rd, rows, err)` | `ApplyRead(rd, items, err)` | `ApplyRead(rd, root, err)` |
| Apply a stream event | `SetKeyedRows` | `SetKeyedItems` | `SetRoot` |
| Where the spinner draws | the `ActivityColumn` cell | a badge at the row's right edge | a badge at the row's right edge |

```go
o := th.List()
o.BusyWhen = func(it list.KeyedItem) (string, bool) {
    phase := it.Data.(argoApp).Phase
    return phase, phase == "Running"
}

o := th.Tree()
o.BusyWhen = func(n tree.Node) (string, bool) {
    app, ok := n.(appNode)   // your Node carries what the server reported
    return app.phase, ok && app.phase == "Running"
}
```

A tree evaluates every node, collapsed ones included, so expanding a branch
reveals a spinner that is already turning.

## Examples

- `examples/patterns/activityrecipes`: one shape per tab (observe, sync,
  refresh, job handle), each small enough to copy.
- `examples/patterns/activity`: all of them on one screen, with marking and
  the output console.
