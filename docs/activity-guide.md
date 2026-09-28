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
    wasFailing := s.table.ReadsFailing()
    if s.table.ApplyRead(m.read, rows, m.err) {
        s.poll.MarkRefreshed()           // only a read that landed counts
    } else if m.err != nil && !wasFailing {
        return s, app.ErrorOf(m.err)     // once, when reads start failing
    }
```

Drive `fetch` from a `pkg/poll` tick (rule 24). Report an outage once, when
reads start failing, and let `ReadsFailing()` carry it on the title after that.
Posting the error on every failed poll puts a fresh error on the statusbar
every interval for as long as the server is down.

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
        // Multi: without it the menu disables the verb once a second row is
        // marked (action.Set.Arity is len(Targets)).
        {Label: "Sync", Multi: true, Do: func() tea.Cmd { return s.sync(keys) }},
    }}
}

type answeredMsg struct {
    op  activity.Op
    err error
}

func (s *Screen) sync(keys []string) tea.Cmd {
    // One operation per row: DispatchEach calls this once per key.
    return s.table.DispatchEach(keys, "Running", activity.Observed,
        func(op activity.Op, k string) tea.Cmd {
            return func() tea.Msg { return answeredMsg{op, s.client.Sync(k)} }   // 202 = nil error
        })
}

case answeredMsg:
    switch s.table.Done(m.op, m.err) {
    case activity.Withdrawn:     // refused: this row stops
        return s, app.ErrorOf(m.err)
    case activity.Acknowledged:  // accepted: still running, polls take over
        return s, app.Info("Sync requested")
    }
case activity.UnobservedMsg:
    return s, app.Info("Sync finished between polls")
```

**`Done` tells you what the reply means.** It returns `activity.Acknowledged`
for an Observed verb (the work has only started, so say "requested"),
`activity.Ended` for a Held one (the work is over), and `activity.Withdrawn`
when the request failed. Switch on it. A reply handler shared by a sync and a
refresh that says "completed" on every reply is wrong for the sync, and nothing
else will tell you.

**One operation per row.** `DispatchEach` gives each row its own operation
and its own request, so each gets its own `Done`. If one row's request is
refused, only that row stops. One operation over the whole selection puts every
row behind the first refusal; use `Dispatch` with several keys only when a
single request acts on all of them.

A `Do` verb gets no receipt from the shell (`Action.Receipt` applies only to
`Run` verbs), so post "requested" yourself, as above.

`UnobservedMsg` arrives through the table's `Update`, on the turn after the read
that ended the claim. Forward every message to the table (rule 6) and match it
in your `Update`. Its `Label` is the claim's label (`"Running"`), not the
verb, so name the verb yourself: `m.Label + " finished"` reads "Running
finished".

Claim with the server's own word (`"Running"`), so the row reads the same from
the keypress to the poll that confirms it.

### Held: a request that blocks until the work is done

```go
func (s *Screen) refresh(keys []string) tea.Cmd {
    return s.table.DispatchEach(keys, "Refreshing", activity.Held,
        func(op activity.Op, k string) tea.Cmd {
            return func() tea.Msg { return refreshedMsg{op, s.client.Refresh(k)} }   // blocks ~2s
        })
}

case refreshedMsg:
    switch s.table.Done(m.op, m.err) {   // the spinner ends here, not at a poll
    case activity.Withdrawn:
        return s, app.ErrorOf(m.err)
    case activity.Ended:
        return s, s.poll.Refresh()       // show what the refresh found now
    }
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

## The title during an outage

`ReadsFailing()` and `poll.LastRefresh()` answer different questions, so show
both. "reads failing" says the rows can't be refreshed; "refreshed Ns ago" says
how stale they are. The count comes from the last read that landed, so it keeps
climbing through an outage, which is when the user needs it most.

```go
func (s *Screen) titleText() string {
    title := "applications"
    if s.table.ReadsFailing() {
        title += " · reads failing"
    }
    if last := s.poll.LastRefresh(); !last.IsZero() {
        title += fmt.Sprintf(" · refreshed %ds ago", int(time.Since(last).Seconds()))
    }
    return title
}
```

Call `SetTitle(s.titleText())` after every read and on a one-second `tea.Tick`,
so N moves between polls.

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
| `table.DispatchEach(keys, label, mode, request) tea.Cmd` | one operation per key; `request(op, key)` returns that key's command |
| `table.Done(op, err) activity.Outcome` | the request answered; returns `Acknowledged` (Observed: still running), `Ended` (Held: over) or `Withdrawn` (failed) |
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
| The statusbar says "completed" while the row still spins | the reply handler ignores what `Done` returns; switch on it (`Acknowledged` is not finished) |
| One refused row stops every marked row's spinner | the selection was dispatched as one operation; dispatch one per row |
| An outage posts an error every poll | post only when `ReadsFailing()` goes from false to true |
| The menu says "one item at a time" with rows marked | the verb lacks `Multi: true`; a set with several `Targets` is a multi-selection |
| "refreshed Ns ago" vanishes during an outage | the title shows "reads failing" *instead of* the freshness count; show both |
| "Running finished between polls" | `UnobservedMsg.Label` is the claim's label; name the verb yourself |

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

## Migrating from the first API

The first version of this feature had the screen keep the read ordering itself
(generation counters, a count of outstanding writes, `RetractAll` on a failed
read) and offered two entrances (`ActivityWhen` or `SetBusy`). All of that is
gone. Screens written against it will not compile until they move over. Most
moves are mechanical.

### Call by call

| Before | Now | Notes |
|---|---|---|
| `Options.ActivityWhen func(Row) (label, busy)` (table) | `Options.BusyWhen func(KeyedRow) (status, busy)` | Reads the whole keyed row, `Data` included, so the field that means "working" needn't be a column. Return the status for **every** row, not only busy ones. |
| `Options.ActivityWhen func(string) (label, busy)` (list) | `Options.BusyWhen func(KeyedItem) (status, busy)` | `KeyedItem` gained `Data`. |
| `Options.ActivityWhen func(Node) (label, busy)` (tree) | `Options.BusyWhen func(Node) (status, busy)` | Same argument; the status now also ends claims when it changes. |
| `SetBusy(map[key]label)` | put the busy-ness in each row's `Data` and read it in `BusyWhen` | One entrance. The screen keeps the map (e.g. from `GET /jobs?status=running`) and folds it into the rows it pushes. |
| `Expect(keys, label)` | `DispatchEach(keys, label, mode, request)`, or `Dispatch(keys, label, mode)` | Choose `activity.Observed` (the server's status says when the work ends) or `activity.Held` (the request, or a job it returned, does). One operation per row unless a single request acts on all the keys. |
| `Retract(keys...)` after a refused write | `Done(op, err)` with the error | Returns `activity.Withdrawn`. |
| a successful write's reply (nothing to call before) | `Done(op, nil)` | **Required now.** Without it an Observed claim is never acknowledged and a Held one never ends. It returns `Acknowledged` (Observed: accepted, still running) or `Ended` (Held: over), so switch on it when writing the reply's message. |
| `RetractAll()` after a failed read | `ApplyRead(rd, nil, err)` | Withdraws the claims waiting on a read and marks observed work `Unknown`. |
| `s.gen++` / `if m.gen < s.seen \|\| s.writing > 0 { drop }` / `SetKeyedRows(rows)` | `rd := BeginRead()` when the fetch is issued, `ApplyRead(rd, rows, err)` when it lands | Delete the `gen`, `seen` and `writing` fields and the `claimed` map. A watch/SSE stream still uses plain `SetKeyedRows`. |
| `Options.Activity.Settle` compared the `ActivityColumn` cell | `Settle` compares `BusyWhen`'s status (plus `Revision`) | Behaviour only. Set `Options.Revision` if a quick operation can end at the status it started from. |
| `activity.Busy` / `Settled` returned `""` for a non-busy value | they return the value either way | Only matters if you compared the returned label to `""`. |
| `action.Set{Targets: keys}` with verbs lacking `Multi` | add `Multi: true` to verbs that act on several rows | The menu now counts `len(Targets)` (`Set.Arity`). A set with several `Targets` used to read as one target, so non-Multi verbs ran on every marked row. They are now disabled with "one item at a time". |
| `action.Set.Count` | optional when `Targets` is set | `Validate` now flags a `Count` that disagrees with `Targets`, not a missing one. |
| `activity.Set.Derive` / `Observe` / `Expect` / `Retract` / `RetractAll` / `Scope` / `Expecting` | removed | Only code that drove a `Set` directly is affected; screens go through their component. `Set.Dispatch` no longer takes an `at` map. |

New, with nothing to replace: `Options.Revision`, `ReadsFailing()`,
`activity.UnobservedMsg`, `KeyedRows()` / `KeyedItems()` / `Root()`, and the
`Unknown` state rows take on while reads fail.

### Before and after

A polled table with one Sync verb. Before:

```go
o.ActivityColumn = "Sync"
settled := activity.Settled("Synced", "OutOfSync")
o.ActivityWhen = func(c table.Row) (string, bool) { return settled(c[colSync]) }

case action.ChosenMsg:
    s.gen++; s.writing++
    s.claimed[action.RunKey(m.Action, m.Target)] = m.Targets
    cmds = append(cmds, s.table.Expect(m.Targets, "Syncing"))
case runner.Captured:
    s.writing--; s.gen++
    if m.Err != nil { s.table.Retract(s.claimed[m.Tag]...) }
case fetchedMsg:
    if m.gen < s.seen || s.writing > 0 { break }
    s.seen = m.gen
    if m.err != nil { s.table.RetractAll(); break }
    s.table.SetKeyedRows(rowsOf(m.apps))
```

After:

```go
o.ActivityColumn = "Sync"
o.BusyWhen = func(r table.KeyedRow) (string, bool) {
    phase := r.Data.(argoApp).Phase           // what is busy while work runs
    return phase, phase == "Running"
}
o.Revision = func(r table.KeyedRow) string { return r.Data.(argoApp).FinishedAt }

// the verb: {Label: "Sync", Multi: true, Do: func() tea.Cmd { return s.sync(keys) }}
func (s *Screen) sync(keys []string) tea.Cmd {
    return s.table.DispatchEach(keys, "Running", activity.Observed,
        func(op activity.Op, k string) tea.Cmd {
            return func() tea.Msg { return answeredMsg{op, s.client.Sync(k)} }
        })
}

case answeredMsg:
    switch s.table.Done(m.op, m.err) {
    case activity.Withdrawn:    return s, app.ErrorOf(m.err)
    case activity.Acknowledged: return s, app.Info("Sync requested")
    }
case activity.UnobservedMsg:
    return s, app.Info("Sync finished between polls")
case fetchedMsg:
    wasFailing := s.table.ReadsFailing()
    if s.table.ApplyRead(m.read, rowsOf(m.apps), m.err) {
        s.poll.MarkRefreshed()
    } else if m.err != nil && !wasFailing {
        return s, app.ErrorOf(m.err)
    }
```

**Check the predicate, not just the calls.** If the old `ActivityWhen` watched
a status that never says "working" while the work runs, such as Argo's
`sync.status`, a mechanical port keeps the bug: no row will ever spin. Point
`BusyWhen` at the field that is busy during the work
(`status.operationState.phase` for Argo). See Step 1.

## Examples

- `examples/patterns/activityrecipes`: one shape per tab (observe, sync,
  refresh, job handle), each small enough to copy.
- `examples/patterns/activity`: all of them on one screen, with marking and
  the output console.
