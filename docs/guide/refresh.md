# Auto-refresh and row activity

## Polling with `pkg/poll`

`poll.Model` is an interval ticker that never touches data. It says "now is a
good time to refetch"; your screen fetches.

```go
s.poll = poll.New(poll.Options{Interval: 2 * time.Second})

func (s *Screen) Init() tea.Cmd { return tea.Batch(s.poll.Init(), s.fetch()) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    var cmds []tea.Cmd
    switch m := msg.(type) {
    case poll.RefreshMsg:
        cmds = append(cmds, s.fetch())
    case fetchedMsg:
        if m.err != nil {
            return s, app.ErrorOf(m.err)
        }
        s.table.SetKeyedRows(rowsOf(m.items))
        s.poll.MarkRefreshed()
    }
    cmds = append(cmds, s.poll.Update(msg)) // every message
    var cmd tea.Cmd
    s.table, cmd = s.table.Update(msg)
    return s, tea.Batch(append(cmds, cmd)...)
}
```

| Method | Does |
|---|---|
| `Init()` | the first tick; `nil` when constructed with `Paused: true` |
| `Update(msg) tea.Cmd` | on its own tick, returns `RefreshMsg` plus the next tick |
| `Refresh()` | a one-off `RefreshMsg` now, without touching the schedule (bind it to `r`) |
| `Pause()` / `Resume() tea.Cmd` | stop and restart; `Resume` waits one interval |
| `SetInterval(d) tea.Cmd` | change cadence and reschedule |
| `MarkRefreshed()` / `LastRefresh()` | record and read the last successful refresh |
| `Paused()`, `Interval()` | state |

Every scheduled tick carries a serial number and only the latest is honored, so
a tick delivered twice can't double the chain, and a tick from an old cadence
is dropped after `SetInterval` or `Pause`.

Because non-key messages reach every screen on the stack, a covered screen
keeps polling and is current when you come back to it.

## Keyed rows keep the user's place

Swap rows with the keyed setters so the cursor follows the same item across a
refresh, even when the set reorders or partially changes:

```go
rows := make([]table.KeyedRow, len(apps))
for i, a := range apps {
    rows[i] = table.KeyedRow{Key: a.Name, Cells: []string{a.Name, a.Region, a.Status}, Data: a}
}
s.table.SetKeyedRows(rows)
```

| Component | Keyed setter | Read the key |
|---|---|---|
| `list` | `SetKeyedItems([]list.KeyedItem{Key, Display, Data})` | `SelectedKey()` |
| `table` | `SetKeyedRows([]table.KeyedRow{Key, Cells, Data})` | `SelectedKey()` |
| `tree` | `SetRoot(node)` (node paths are keys) | `Selected()` |
| `inspector` | `SetFields(fields)` (row paths are keys) | `Selected()` |

When the previous key is gone, the cursor falls back to the nearest index.
`SetItems` / `SetRows` clear keys, so use the keyed setter consistently across
a screen. Filter, sort and marks are preserved across a keyed swap.

### "Last refreshed" indicators

Use `app.Info` for transient feedback ("refreshed 14 deployments"). For a
persistent "refreshed 3s ago", set the component's title from a one-second
tick:

```go
s.table.SetTitle(fmt.Sprintf("Deployments · %s ago", time.Since(s.poll.LastRefresh()).Round(time.Second)))
```

See `examples/liveops`.

## Row activity

`pkg/activity` shows a spinner and status label on just the rows that are
busy: "two of these forty rows are syncing". It's the row-scale counterpart
to `SetLoading`, which replaces a whole body. `list`, `table` and `tree` carry
it; a screen rarely touches `pkg/activity` directly.

The model in two sentences: **the server is the source of truth. When the user
starts work locally, the row is told at once, and that claim lasts until the
truth can take over.**

### 1. Observation: `BusyWhen`

Put the record in `KeyedRow.Data` and say which field means "working":

```go
opts := t.Table()
opts.ActivityColumn = "Sync"   // where the spinner draws
opts.BusyWhen = func(r table.KeyedRow) (string, bool) {
    phase := r.Data.(argoApp).Phase
    return phase, phase == "Running"
}
```

Return the status for every row, busy or not. Pick the field that is busy
**while the work runs** (Argo's `operationState.phase`, not `sync.status`).
`activity.Busy("Running", "Pending")` and `activity.Settled("Succeeded",
"Failed")` build common predicates. Give the activity column a `Width` wide
enough for its longest label. In `list` and `tree` the spinner draws as a
right-aligned badge, so there's no column to name.

### 2. Reads: `BeginRead` and `ApplyRead`

```go
func (s *Screen) fetch() tea.Cmd {
    rd := s.table.BeginRead()
    return func() tea.Msg {
        apps, err := s.api.List()
        return fetchedMsg{rd, apps, err}
    }
}

case fetchedMsg:
    wasFailing := s.table.ReadsFailing()
    if s.table.ApplyRead(m.read, rowsOf(m.apps), m.err) {
        s.poll.MarkRefreshed()
    } else if m.err != nil && !wasFailing {
        return s, app.ErrorOf(m.err)   // once, when reads start failing
    }
```

`ApplyRead` is `SetKeyedRows` with ordering: it drops a reply overtaken by a
newer one, and on a failed read it withdraws spinners waiting on a read and
draws rows that were busy as Unknown (static, with `?`) until a read succeeds.
`ReadsFailing()` says the same for the title. A watch or SSE stream needs none
of this; push its events with `SetKeyedRows`.

### 3. Work the user starts: `Dispatch` and `Done`

One decision per verb: **who knows when the work ends?**

| Mode | When | Example |
|---|---|---|
| `activity.Observed` | the server's status says when it ends; the request only acknowledges | Argo sync `POST` → 202 at once |
| `activity.Held` | the request, or a job it returns, says when it ends | a `GET` that blocks until done; a job id polled to completion |

```go
Do: func() tea.Cmd {
    return s.table.DispatchEach(s.table.Selection(), "Running", activity.Observed, s.postSync)
},

// postSync(op, key) returns a command that does the request and replies answeredMsg{op, err}.

case answeredMsg:
    switch s.table.Done(m.op, m.err) {
    case activity.Withdrawn:     // the request failed
        return s, app.ErrorOf(m.err)
    case activity.Acknowledged:  // Observed: accepted, still running
        return s, app.Info("Sync requested")
    case activity.Ended:         // Held: the work is over
        return s, app.Info("Refresh finished")
    }
case activity.UnobservedMsg:     // started and ended between two polls
    return s, app.Info("Sync finished between polls")
```

- **Switch on what `Done` returns.** After an `Observed` request the work is
  only accepted; say "requested", never "completed".
- **Dispatch one operation per row** (`DispatchEach`), and call `Done` per
  row, so one row's refusal doesn't stop the others' spinners.
- **Claim with the server's own word** (`"Running"`) so the row reads the same
  from the keypress to the confirming poll.
- An `Observed` spinner ends when a read after `Done` reports the work over,
  or after `Options.Activity.Settle` reads that repeat the pre-dispatch status
  (0 for APIs that set status inline, 1–2 for reconcilers).
- `Options.Revision`, a value that moves when work *finishes* (Argo's
  `finishedAt`, a job counter), lets a sync that ends where it began
  (Succeeded → Succeeded) end on the first read instead of waiting out
  `Settle`.
- `UnobservedMsg` reports work no read ever saw running. `Changed` is true
  when the status moved. Its `Label` is the claim's label, not the verb.

There are no outcome glyphs and no hold timers: a row stops spinning when the
data says so and goes back to rendering its own cell. Rows must be keyed;
under `SetWindow` activity is inert. `State()`/`Restore` carry it across a
theme swap.

Full detail: the [row activity guide](../activity-guide.md), and
`examples/liveops`, which offers every shape as a verb on one table.
