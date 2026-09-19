# Row activity — design

Status: **implemented.** `pkg/activity` (a leaf, ~165 lines of code),
`theme.Activity()`, the three components, and the shared contract in
`internal/componenttest`.

**One way in: the data.** A component observes the rows it holds on every keyed
swap, a predicate says which of them are working, and those rows spin. There is
no action broadcast, no shell involvement, no setter a screen calls, and no
second endpoint. A `pkg/poll` refresh and a predicate are the whole mechanism.

An earlier version of this design carried a second layer — state the TUI knew
about because it had started the work itself — and the machinery to reconcile
the two. That is recorded under [What was removed](#what-was-removed-and-why),
along with the three things it was reaching for, which are worth revisiting
deliberately rather than by accretion. The removed implementation is parked at
`pkg/activity/activity.go.old`.

## Problem

A TUI over a remote system spends most of its time displaying state that
somebody is in the middle of changing. The server already says so — the cell
reads `Syncing`, or `running`, or `Progressing` — and the table renders that
word as flat text, identical in weight and behaviour to the `Synced` above it.

So the row is telling the truth and still failing to communicate the one thing
that matters about it: **this will change without you doing anything.** A user
looking at a static `Syncing` has no way to distinguish "the server is working
on it right now" from "the server gave up mid-sync an hour ago and this string
is stale". Motion is what separates them, and motion is exactly what static
text cannot express.

The library does have a spinner and it is the wrong granularity.
`pane.SetLoading` (rule 17) means "this whole component has no data yet" — a
centered glyph, the body replaced, all-or-nothing. There is no way to say "two
of these forty rows are busy and the other thirty-eight are still true".

So authors do it by hand, and the hand-rolled version is worse than it first
looks:

- The indicator has to be baked into the row cells, so showing it means
  rebuilding and re-setting the whole row set.
- The spinner needs a frame, so it needs a ticker, so the screen grows a
  `spinner.Model` and a `spinner.TickMsg` case and re-sets every row on **every
  frame**.
- `pkg/poll` (rule 24) is usually swapping those same rows every couple of
  seconds, so there are two writers for one row set and whichever ran last
  wins. The spinner flickers, or the refresh does.
- None of it is reusable. The same forty lines get rewritten for the list on
  the next screen, and again for the tree on the one after.

Every one of those problems comes from the same mistake: treating the indicator
as *data*. It is not data. It is a rendering of data the component already
holds, and it belongs where every other rendering decision in this library
lives — inside the component, over the top of rows nobody rewrites.

## Shape

Two options on the component, and a `pkg/poll` you were going to have anyway:

```go
o := t.Table()
o.Columns = []table.Column{
    {Title: "Name", Width: 22},
    {Title: "Sync", Width: 14},   // explicit — see decision 6
    {Title: "Health", Width: 13},
}
o.ActivityColumn = "Sync"         // which cell the indicator replaces
settled := activity.Settled("Synced", "OutOfSync")
o.ActivityWhen = func(c table.Row) (string, bool) { return settled(c[colSync]) }
```

That is the entire author-facing surface. There is no third line, no message to
handle, no command to batch beyond the one every setter in this library already
returns, and nothing to remember to turn off.

```
NAME            SYNC          HEALTH
api-server      Synced        Healthy
cache-redis     ⣾ Syncing     Healthy
web-frontend    OutOfSync     Degraded
worker-pool     ⣾ Syncing     Progressing
```

The flow, end to end:

```
poll fires  →  screen fetches  →  SetKeyedRows(rows)
                                       │
                                       ├─ observe(): run the predicate over
                                       │  every keyed row, build key → label
                                       │
                                       └─ Set.Derive(busy): replace the map
                                          wholesale, arm the spinner if any
                                          key is busy

render      →  for each visible row, Set.Render(key, width)
                 busy → "⣾ Syncing", styled by the component
                 not  → false, and the row's own cell is drawn
```

**Nothing writes back.** The indicator is computed from the rows and drawn over
the rows; it never enters them. That is what makes the feature immune to the
two-writer problem above, and it is why a filter, a sort, a resize, a theme
swap and a cursor move all change nothing about which rows are spinning.

---

## Decisions

### 1. The data is the only source of truth.

The remote system knows whether it is working on something. The TUI does not,
and every mechanism by which the TUI *appears* to know is really a cached claim
with an expiry problem.

An earlier design had three ways in — an `action.Set` broadcast against its
`Targets`, a direct `SetActivity(key, label)` call, and this predicate — with a
precedence rule to settle the disagreements between them. It worked, and the
machinery it took to make it work is the argument against it:

| Mechanism | Existed to answer |
|---|---|
| local layer beats derived | a poll in flight when the user acted returns the pre-click value |
| handoff on next observation | a dispatching action returns long before the work ends |
| `Options.Confirm` expiry | …unless it never ends, and something has to cap the wait |
| `Options.Dwell` hysteresis | two replicas disagreeing between reads |
| outcome hold + `✓`/`✗` glyphs | a local entry's outcome has nowhere else to appear |
| `ActivityRevision` | work that starts and ends between two observations |

Six mechanisms, and every one of them is a rule for reconciling a local opinion
with the server's. Delete the local opinion and all six become unreachable.
What is left is a pure function:

```
the rows the component holds  →  which of them are busy
```

recomputed on a keyed swap and on nothing else. It cannot drift, cannot be
stale by more than one poll interval, cannot disagree with the cell beside it,
and cannot oscillate on its own — which is worth saying explicitly, because "the
spinner flaps" was the bug that prompted this reduction and the answer turned
out to be that nothing in this layer *can* flap. A flapping indicator is a
flapping read path: a doubled poll chain, replies landing out of order, or
`SetRows` on one path and `SetKeyedRows` on another. Fix it there.

**The cost, stated plainly, because it is a design choice and not an
oversight:**

- An action dispatched from the TUI shows nothing on the row until a later poll
  reports it. Press Sync and the row keeps saying `OutOfSync` for up to one
  poll interval.
- Work that begins and ends between two observations is never seen at all.

Both are real. Both were covered by the removed layer, at the cost of the table
above. Both are open questions rather than settled ones — see
[What was removed](#what-was-removed-and-why).

### 2. Activity is held by key, and keys are mandatory.

Rule 32's argument for marking applies here without a word changed, and for a
sharper reason. Marks held by index drift onto the wrong row when a polled
refresh reorders the set; an activity indicator held by index does the same,
except that the set it drifts within is *guaranteed* to be churning — the whole
point of showing a spinner is that something is changing the data underneath.

So activity works on `SetKeyedItems` / `SetKeyedRows` / tree paths, and is
inert on anonymous `SetItems` / `SetRows`. An inert indicator is recoverable; a
spinner sitting on the wrong row is a user watching the wrong thing, which is
worse than watching nothing.

A windowed table (`SetWindow`) is inert for the same reason marking is: it
carries rows without keys, and a sparse paged set is exactly where an
index-held indicator would land somewhere else.

`pkg/tree` needs no keyed setter, because a node's path already *is* its
identity — the same path used for expansion state, cursor restore and marks.
One node therefore has one key across all four features.

### 3. The state lives in the component, not in the screen.

This is rule 9 — a component owns its pane and its geometry — extended to the
obvious next thing: a component owns the *decoration of its own rows*. Nothing
outside `pkg/table` knows where row 12 is drawn, which cell the Status column
occupies, or how wide it ended up after flex resolution. A screen that wanted
to paint a spinner there would have to know all three.

Keeping it in the component also kills the two worst properties of the
hand-rolled version at once: the row data is never rewritten (activity is an
overlay on render, not a mutation of the cells), and the ticker lives next to
the thing it animates instead of in a screen that has to remember to re-push
rows on every frame.

**Rejected: a `Decorator func(key string) string` hook on Options.**
Superficially more general — the screen returns whatever it likes per row. In
practice it is called once per visible row per frame, it invites I/O and
allocation in a hot path the way `Help()` does not, and it hands back a
*string* the component must then place, style and truncate without knowing what
it means. A typed state the component understands can be right-aligned, can
shrink to its glyph when the column is narrow (decision 5), and can be tested.

### 4. `pkg/activity` is a leaf package.

Three components need identical behaviour — a keyed map, a spinner, a tick
chain that heals itself, a width-aware renderer — and none of them should
import another to get it. That is the same argument that produced `pkg/geom`
and `pkg/query`, and it lands the same way: `pkg/activity` imports `bubbletea`,
`bubbles/spinner` and `x/ansi`, and **nothing at all from tuilib**.

It imported `pkg/glyph` in the previous version, for the `✓`/`✗` outcome marks.
Decision 7 removed the marks, and the import went with them.

### 5. Where it renders, per component.

**`table` — a named column.** `Options.ActivityColumn string` is a column
*Title*, matched the way the filter's `key:value` scope matches
(case-insensitive prefix, via `query.ColumnByPrefix`). While a row is busy that
cell's content is replaced. Titles rather than indices because a screen that
reorders its columns should not silently start decorating the wrong one.

This is the shape the feature exists for: `Synced` → `⣾ Syncing` → `Synced`
reads as one cell changing its mind, not as decoration appearing beside it.

A name that resolves to no column — a typo, an ambiguous prefix, a hidden
column — falls back to a two-cell gutter after the mark gutter, as does
`ActivityWhen` set with no `ActivityColumn` at all. Degrading to something
visible beats a spinner nobody can see that animates anyway. (That last case
was a real bug: `actEnabled` keyed off `ActivityColumn` alone, so a predicate
with no column name built the whole derived set, ran a tick chain forever, and
rendered nothing.)

**`list` and `tree` — a right-aligned trailing badge.**

```
worker-pool                              ⣾ running
```

Not a prefix: the leading cells are spoken for (list's gutter is two cells,
three with marking; the tree's is indent plus disclosure glyph), and pushing
rows sideways when a spinner appears is exactly the reflow that keeps
`pkg/form` drawing its validation errors on the border rather than under the
field. When the row text and the badge cannot both fit, the **row text
truncates and the badge survives** — the badge is the news. The badge is itself
capped at half the width, so a pathological label cannot swallow the row
entirely.

**Only `table` opts in.** There is nothing for `list` and `tree` to switch on —
a table has to be told *which column*, while a badge occupies no space at all
until something is running, so gating it would be a field whose "off" position
costs exactly as much as its "on" one.

**When the space is too narrow for the label, the glyph wins.** A cell of width
8 rendering `⣾ syncin` is worse than one rendering `⣾`. Below
`len(glyph)+1+len(label)` visible cells the label is dropped rather than cut.
Width is measured and cuts are made ANSI-aware throughout, since both the row
text and the styled indicator carry escapes.

### 6. Column width is the author's problem, and the doc has to say so.

The intuition is that a content-auto column will *widen* when `Synced` becomes
`⣾ Syncing`, reflowing the table under the user. It cannot: `recomputeWidths`
sizes columns from the rows the table *holds*, and the indicator is substituted
at render time and never enters them. Activity cannot move a column, and
`TestTableActivityDoesNotReflowColumns` holds that.

What actually goes wrong is the reverse. A column auto-sized to fit `Synced` is
six cells wide, the indicator is capped to the width it is given, and the label
is dropped in favour of the glyph (decision 5) — so the row spins but never
says *what* it is doing, and no amount of staring at it explains why.

So an activity column wants an explicit `Width`, for the opposite reason to the
one you would guess: not to prevent a reflow, but to leave room for the longest
label it will carry.

### 7. There is no outcome, because the data already has one.

When `running` becomes `failed`, the cell goes back to rendering the row's own
value — which already says `failed`, in whatever colour the app gives it. A
`✓`/`✗` held over the top for two seconds is the library restating the row
*less precisely than the row states itself*, and in a vocabulary the app did
not choose.

So `State` has no `Done` and no `Err`, there is no hold timer, and `Render`
reports `false` the moment a key stops being busy. A key's whole lifecycle is:

```
absent  →  present (Label, Since)  →  absent
```

This is the decision that removes the most machinery for the least loss, and it
is only available *because* of decision 1. A locally-started entry genuinely
needs an outcome glyph — its outcome has nowhere else to appear, since the cell
underneath was never going to change. A derived entry's outcome *is* the cell
underneath.

### 8. `Settled` is usually the better predicate, and it takes the whole row.

Two helpers, and the second is usually the right one:

```go
activity.Busy("running", "pending", "waiting")        // these mean work
activity.Settled("successful", "failed", "canceled")  // everything else does
```

Prefer `Settled`. It is how these APIs document themselves — a finite set of
terminal states, an open set of intermediate ones — and it keeps working when
the server learns a new in-progress status, where `Busy` would quietly treat
that one as done and stop spinning. The mirror risk is a new *settled* status
spinning forever, so pick the list the server is less likely to extend.

Both compare with surrounding space and ANSI styling stripped, so a cell the
screen has already coloured still matches — otherwise the first thing an author
does after styling the status column is break their own indicator. Both label
the row with the value **as it appeared**, not as it was configured: the row
says the server's own word, with the server's own casing, so there is no
mapping to keep in step and no way for `syncing` to become `Syncing`
mid-flight.

`Settled` treats an empty value as settled. A blank cell is not work in
progress, and the alternative spins every row in a table that has not loaded.

**The predicate takes the whole row rather than one cell**, because the status
worth watching is not always the status worth showing — a hidden column, a
second field, or a pair that only means "busy" together are all normal things
to want. Each component's signature speaks its own data shape:

```go
table: func(cells Row) (label string, busy bool)
list:  func(item string) (label string, busy bool)
tree:  func(n Node) (label string, busy bool)
```

### 9. One spinner per component, and the tick chain heals itself.

The component owns a `spinner.Model` distinct from the pane's loading spinner.
Safe because `spinner.TickMsg` carries the originating model's `ID` and bubbles
rejects ticks belonging to another spinner — so the two animate independently
without the component having to disambiguate.

`Derive` and `SetActivityState` return `tea.Cmd` and the caller must batch it,
exactly as `pane.SetLoading` does (rule 17). The chain runs while any key is
busy and stops when the last one settles, so an idle screen schedules nothing.

**The chain has to heal itself.** It lives in `tea.Cmd`s and survives only while
the ticks it asks for come back. Anything that stops delivering messages to a
component — a screen stack routing to the top screen only, a tab hiding a body
— ends the chain, while `ticking` stays true because nothing told the `Set`
otherwise, and `armTick` then refuses to start another. The spinner is frozen
for the rest of the session, on a row that is still working.

`pkg/pane`'s loading spinner had the identical defect for the identical reason,
and so did `pkg/poll` — whose chain dying is worse, because then no observation
arrives at all and every row sits on a stale status indefinitely. Three
components, one cause.

**The root fix is in `screen.Stack`**, and it makes the library consistent with
itself: `pkg/tab` already fans non-input messages out to every body (rule 21),
and the stack now routes the same way — `tea.KeyMsg` and `mouse.Msg` to the top
screen, everything else to every screen (rule 6). The cost is that a covered
screen keeps working, which is also the point: return from the console and the
data is current.

**Per-component revival stays as defence**, for anyone driving components
without the stack. `Handle` checks on every non-tick message whether a tick has
actually arrived recently — generously, at four frame intervals, so ordinary
jitter does not look like a dead chain — and re-arms if not. Over-arming is
provably harmless: bubbles tags each tick and a spinner rejects one from a
superseded chain, so two chains collapse into one on the next frame.

**No new key bindings, so `Help()` and `HelpSections()` are untouched** (rule
10). Activity has no verbs: it is not clickable, not cancellable from the row,
and not navigable.

### 10. State survives a theme rebuild, like every other state.

Rule 4 requires it, so the accessor pair exists:
`ActivityState() activity.Set` / `SetActivityState(activity.Set) tea.Cmd`. The
returned command is the tick — a rebuilt component starts a fresh spinner and
must be re-armed, or the theme swap leaves a frozen glyph on the row.

`Adopt` **copies** the entries rather than aliasing the map, so the discarded
`Set` cannot write into the live one. (The old doc comment claimed copies share
their entries; they never did, since `Derive` replaces the map rather than
editing it. `TestAdoptDoesNotAliasTheOtherSet` now pins the real contract.)

The spinner *frame* resets to zero across the swap. Accepted: the alternative
is serialising an animation phase through a state-restoration API, and nobody
can see it.

The keyed setters do **not** clear activity, for the same reason they do not
clear marks — they re-derive it, which is strictly better.

### 11. The package emits no escapes, and that is why `Style` is a function.

`Options.Style` is `func(State, string) string`, not a `lipgloss.Style`.

A table cell must be coloured with a foreground-only escape (`\x1b[39m`) or the
selected row's background is clobbered mid-cell (rule 19); a list row has no
such constraint and can use lipgloss freely. **Only the component knows which
it is**, so the package hands back plain text and lets the caller colour it.
`theme.Activity()` supplies the lipgloss form for `list` and `tree`;
`theme.activityCell()` supplies the `ansi.CellColor` form for `table`.

A `lipgloss.Style` field would have forced one answer for both, and the answer
that is safe in a table is the one that looks wrong in a list. This is the one
place the feature can produce a visible rendering bug, and it is the one the
library has already written down.

The `State` is passed to `Style` even though nothing uses it today: it is what
a caller would need in order to colour by elapsed time or by label, and it
costs nothing to thread now rather than break the signature later.

### 12. Activity is not loading, not focus, and not marking.

Three neighbours, each of which someone will propose folding this into.

**Not `SetLoading`.** That is the pane's whole-body state and it *replaces* the
body — correct for "no data yet", nonsense for "thirty-eight of forty rows are
still true". They compose without interacting: a component can be loading with
activity keys held, and when the body comes back the spinners are still on the
right rows because they are held by key.

**Not focus.** Activity never moves the cursor, never takes focus, and is not
in the tab order. It is a property of the data, not of the user's attention. A
row that finishes while the cursor is elsewhere must not pull the cursor to
itself — that is the auto-scroll bug every log viewer eventually grows.

**Not marking.** Marks are the user's selection and activity is the system's
state, and they are drawn in different places for that reason. They do overlap
usefully: mark three rows, sync them, watch three rows spin — which works
because both are keyed by the same key.

---

## What was removed, and why

The previous design had a second layer for work the TUI started itself, fed by
three sources, and reconciled with the derived layer by the six mechanisms in
decision 1's table. All of it is deleted from `pkg/activity` and parked at
`pkg/activity/activity.go.old` (with its tests at `activity_test.go.old`).

Removed from `pkg/activity`: the `local` map; `StartMsg` / `UpdateMsg` /
`EndMsg`; `Start` / `StartRun` / `Finish` / `FinishRun` / `Clear` / `ClearAll`
/ `Relabel` / `RelabelRun`; the `awaiting` handoff and `Options.Confirm`;
`Options.Hold` and the outcome glyphs; `Revise` / `Change` and the revision
flash; `Progress` / `Progresser`; `Options.Dwell`; and the `pkg/glyph` import.
`Handle` lost its `holds func(string) bool` parameter, which existed only to
filter broadcasts.

Removed from the components: `SetActivity` / `EndActivity` / `ClearActivity` /
`Relabel` and `Options.ActivityRevision` on all three. Removed from `pkg/app`:
the three broadcasts in `CaptureStarted` / `CaptureStatus` / `Captured`.
`theme.Activity()` went from three styles plus a glyph set to one style.

**Three things that layer was reaching for remain genuinely unsolved**, and are
the subject of the next design pass rather than of this one:

1. **A separate handle.** Many systems answer "is this row busy" at a different
   endpoint from the one the list came from — an operations API, a job status
   resource, a `GET /operations/{id}` returned by the POST that started the
   work. Today a screen with one of those has nowhere to put the answer.
2. **A revision.** `finished_at`, `resourceVersion`, an ETag: a field whose
   contract is to change when the work does, which is the only way to notice
   work that began and ended between two polls. The previous
   `Options.ActivityRevision` was the right shape and arrived before the rest
   of the design was settled.
3. **A separate collection to query.** The busy set as its own list — `GET
   /operations?status=running` — joined to the rows by key, rather than derived
   from a field on each row.

All three are the same question wearing different clothes: **what does a screen
do when the busy-ness of a row is not a field on that row?** Answering it once,
rather than three times, is the point of doing it deliberately.

---

## API surface

### `pkg/activity`

```go
// State is one key's in-flight state.
type State struct {
    Label string     // the value the predicate matched — the server's own word
    Since time.Time  // first observed busy, not last: elapsed measures the work
}

type Options struct {
    Spinner *spinner.Spinner            // nil → spinner.Dot, matching pane
    Style   func(State, string) string  // nil → plain; see decision 11
}

// Set is the keyed collection of busy rows plus the spinner that animates
// them. Components embed one; it is also the unit ActivityState carries.
type Set struct{ /* map[string]State, spinner.Model, tick bookkeeping */ }

func New(opts Options) Set

// Derive replaces the whole collection from one observation. Keys present are
// busy with that label; keys absent are not. Since survives for a key that
// stays busy. The command is the animation's first tick.
func (s *Set) Derive(busy map[string]string) tea.Cmd

// Handle advances the animation, and on any other message takes the chance to
// notice the chain has stalled (decision 9).
func (s *Set) Handle(msg tea.Msg) tea.Cmd

// Adopt takes another Set's entries, keeping this Set's options — rule 4.
func (s *Set) Adopt(other Set) tea.Cmd

func (s Set) State(key string) (State, bool)
func (s Set) Active() bool
func (s Set) Count() int

// Render is the indicator fitted to width; false when the key is not busy.
func (s Set) Render(key string, width int) (string, bool)

// Badge right-aligns the indicator at the end of a row width cells wide,
// truncating the row rather than the badge. Returns row untouched when idle.
func (s Set) Badge(key, row string, width int) string

// Predicates. Both strip ANSI and surrounding space, and label with the value
// as it appeared.
func Busy(values ...string) func(value string) (label string, busy bool)
func Settled(values ...string) func(value string) (label string, busy bool)
```

No messages. No `Progress`. No timers beyond the spinner's own.

### `pkg/list`, `pkg/table`, `pkg/tree`

```go
// Options
Activity activity.Options  // from the theme builders

// table only: the column whose cell is replaced while a row is busy.
// Matched on Title, case-insensitive prefix. Give it an explicit Width —
// decision 6.
ActivityColumn string

// ActivityWhen derives in-flight state from the row's own data. Evaluated on
// every keyed swap. Per-component signature — decision 8.
ActivityWhen func(...) (label string, busy bool)

// Model
func (m Model) ActivityState() activity.Set
func (m *Model) SetActivityState(s activity.Set) tea.Cmd  // rule 4 rebuilds
func (m Model) ActivityCount() int
```

The table's feature is on when **either** `ActivityColumn` or `ActivityWhen` is
set; `list` and `tree` need only the predicate.

### `pkg/theme`

```go
func (t Theme) Activity() activity.Options      // lipgloss, for list and tree
func (t Theme) activityCell() activity.Options  // ansi.CellColor, for table
```

nested into `List()`, `Table()` and `Tree()` alongside the `SpinnerStyle` each
already sets. Both are one `Accent` style: with no outcome states there is
nothing else to colour.

### `pkg/app`, `pkg/action`, `pkg/runner`, `pkg/glyph`

Nothing. The feature touches none of them.

Two leftovers from the removed layer are now dead and should go with the next
pass at `pkg/action`: `Action.Busy` / `BusyLabel()`, which nothing reads, and
`pkg/app`'s `actionRun.busy` field, which is written in `runAction` and read
nowhere. `Set.Targets` is **not** dead — it still feeds the per-target
`Exclusive` gate.

---

## Rejected, worth remembering

- **A `Decorator func(key string) string` hook.** More general, called in a hot
  path, returns an untyped string the component must place blindly. Decision 3.
- **Reusing `pane.SetLoading` with a row filter.** Conflates "no data" with
  "some rows busy"; the pane has no notion of rows and should not acquire one.
- **An outcome glyph on a derived entry.** The data already says `failed`, in
  the app's own colours. Decision 7.
- **An incremental API** (`SetBusy` / `ClearBusy` per key) instead of wholesale
  `Derive`. It makes "this row stopped being busy" something the caller has to
  notice and report, which is the bookkeeping this feature exists to remove —
  and the failure mode is a spinner that never stops.
- **A `lipgloss.Style` for the indicator.** Forces one answer for table and
  list, and the answer that is safe in a table looks wrong in a list.
  Decision 11.
- **Animating a row by re-pushing the rows.** The screen and the poll become
  two writers for one row set; whichever ran last wins.
- **Inferring the row label from the last log line.** The activity example did
  exactly this, taking the first word of each line, and put "found", "waiting"
  and finally "sync" on the row — the last from "sync operation complete".
  Makes every incidental log line a UI change, and forces log prose to read
  well in a 12-cell column.
- **Reporting a run-scoped counter per row.** "2/3" drawn on every marked row,
  as though it were that row's own progress. A run has one label.
- **Diffing whole rows to detect change.** Any poll that reformats a timestamp
  or recomputes an age column would flash every row. A revision field's
  contract is that it changes when the work does; a row diff mistakes rendering
  for meaning. (The revision field itself is deferred, not rejected.)
- **Holding a spinner for a fixed grace period** so a poll might catch the
  work. Too short does nothing, too long lies, and the right number is the poll
  interval the component does not know.
- **A cancel affordance on the row.** A second place to get the confirm, the
  `Exclusive` release and the log record right. `x` in the console already
  kills a run.
- **Auto-scrolling to a row that finishes.** The cursor belongs to the user.
- **`Options.Dwell` as a general defence against flapping.** Hysteresis for a
  source of truth that disagrees with itself between reads is a real need, and
  it is *not* what a flapping indicator usually indicates. Every cause we
  actually found was in the read path (decision 1). Shipping the damper made it
  the first thing reached for and the last thing that would help. Parked with
  the rest of `activity.go.old`; bring it back when a screen demonstrates two
  replicas genuinely disagreeing.

---

## Tests

The shared contract goes in `internal/componenttest`, not in whichever
component gets built first. The anti-pattern is on the record: the
click-to-blur behaviour was written for `pkg/list`, rolled out to five other
components by a script that omitted it, and covered by a test living in
`pkg/list` — so five components shipped broken and the suite stayed green.
`marking_test.go` is the precedent.

### `pkg/activity/activity_test.go` — the state machine

*Observing.* A key goes busy from data alone. `Derive` is wholesale, so a key
absent from the observation settles. An empty observation clears everything —
the case that stops the last spinner, and the one an `if len(busy) > 0` guard
silently breaks. `Since` persists across observations of the same work and
restarts after a gap, because a key that settles and comes back is a second
piece of work. A changed label follows the data.

*Rendering.* Glyph then label. An unknown key renders nothing. A narrow cell
drops the word and keeps the glyph. **Width is never exceeded at any width from
1 to 40** — a loop rather than three cases, since the interesting failures are
at the boundaries between "glyph plus label", "glyph alone" and "cut". Zero
width renders nothing. The spinner frame's trailing space is trimmed, or every
label sits two spaces out. `Style` is applied, and optional.

*Badge.* Right-aligns to the full row width; truncates the row rather than the
badge; leaves settled rows untouched.

*Tick chain.* An idle `Set` schedules nothing. A second observation does not
start a second chain. The chain stops when the last row settles. **A starved
chain is revived** — decision 9's frozen spinner. A healthy chain is not
duplicated, which is the other half: revival must not fire on ordinary jitter.

*Rebuild and predicates.* `Adopt` carries the observation, re-arms, keeps the
new `Set`'s palette, and copies rather than aliases. `Busy` matches
case-insensitively and labels as the value appeared. Both predicates see
through ANSI styling. `Settled` treats an unknown status as work and a blank
one as settled.

### `internal/componenttest/activity_test.go` — the shared contract

Six properties, each asserted once and run across `list`, `table` and `tree`
through a `derivable` interface: a polled status drives the indicator; a
resting one stops it; rows are independent; the label is the data's own word;
the indicator survives a theme rebuild; an idle component schedules nothing.

### `internal/componenttest/activity_render_test.go` — the rendering hazards

Fourteen tests against a real table render, because "put a spinner in a column"
is exactly the change that opens rendering bugs: reflow (decision 6), narrow
cells, sort and filter identity, the mark gutter's interaction with the
fallback gutter, styled cells, rule 19's foreground-only escape surviving the
selected row's background, and the no-write-back invariant — that the rows the
component holds never contain an indicator, which is the one way this feature
could feed into its own input.

Three of these initially passed for the wrong reason and are worth naming, as
the pattern recurs: scanning a whole line for `\x1b[0m` hit the *pane border's*
reset; `ContainsAny(plain, "abcdefgh")` matched the pane **title**; and
`ContainsAny(view, "WORKING")` matched the `N` in the `Name` header. Assert
against the specific line carrying the indicator, and against the exact escape
pair — `"running\x1b[39m"` present, `"running\x1b[0m"` absent.

### Verification

Each behaviour was mutation-checked: removing the trailing-space trim, letting
the label draw past the width, dropping the empty-observation case, aliasing in
`Adopt`, skipping the revival, and reviving on every message each fail the test
written for it. Two mutations survived their first pass and both found
something real — one a missing test, one a piece of dead code that was deleted
rather than covered.

`lipgloss.SetColorProfile(termenv.TrueColor)` in `TestMain`, or every styling
assertion is vacuously true.

---

## Open questions

**1. Where does the busy-ness come from when it is not a field on the row?**
The three shapes in [What was removed](#what-was-removed-and-why) — a separate
handle, a revision, a separate collection. This is the next design pass, and
the reason the second layer came out rather than being patched: it was
answering this question implicitly, three times, in three different places.

**2. Should a dispatching action show anything at all before the next poll?**
Today it does not, and the row reads as untouched for up to a poll interval
after the user pressed Sync. The honest framing is that the TUI genuinely does
not know anything yet — but "the UI did not react to my keypress" is a strong
negative signal regardless of whether the UI has anything true to say. A
statusbar receipt (`Action.Receipt`, which still works) covers it partially.
Worth deciding deliberately, not by reinstating the layer.

**3. Should `pkg/inspector` get this?** It has path-keyed fields and the same
`SetFields` preservation dance, so it would work. But a record viewer has no
verb that acts on a field (rule 32 declined marking there for the same reason),
so the only caller would be a screen animating something it fetched per-field.
Leaning no until someone has that screen.

**4. Should two panes showing the same key both spin?** They do. With the
broadcast gone this is no longer a scoping question about `action.Set` — it is
simply what deriving from data means, and two components holding the same row
will both observe it busy. Arguably correct. Recorded because the previous
design had an open question here and the answer changed shape rather than going
away.

**5. Should `Derive` be able to say "failed" rather than just "not busy"?**
Today the predicate is binary, so a derived run ending badly is communicated
entirely by the cell reverting to `failed` in the app's own styling. A
three-valued phase (resting / busy / failed) would let the library tint the row
for a moment on the way past. It is more API for something the data already
renders, and it needs the author to classify every resting value rather than
just the busy ones. Leaning no; revisit if a real screen reads flat.
