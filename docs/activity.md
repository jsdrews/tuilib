# Row activity — design

Status: **implemented.** `pkg/activity` (a leaf, under 300 lines of code),
`theme.Activity()`, the three components, rule 33 in CLAUDE.md, and the shared
contract in `internal/componenttest`.

**The data is the source, and there is one map.** A component observes the rows
it holds on every keyed swap, a predicate says which of them are working, and
those rows spin. When the busy-ness is not a field on the row, the screen hands
the same map over with `SetBusy` instead — a second entrance, not a second map.
For the gap between a keypress and the next observation, `Expect` places a
claim that every observation retires. There is no action broadcast and no shell
involvement: a `pkg/poll` refresh and a predicate remain the whole mechanism
for a read-only screen.

An earlier version of this design carried a second layer — state the TUI knew
about because it had started the work itself — and the machinery to reconcile
the two. That is recorded under [What was removed](#what-was-removed-and-why),
along with the three things it was reaching for. The removed implementation is
parked at `pkg/activity/activity.go.old`.

Decisions 1-21 are built and green. **13-17 shipped in their amended form** —
[Work the user starts](#work-the-user-starts).
They put the user's own actions back on the row in five pieces rather than the
previous version's six reconciliation mechanisms, by moving the hard part out
of this package and into the fetch generation the screen already keeps. Whether
that trade holds is now a question with evidence behind it rather than an
argument: `demoapi` was widened to reproduce the orderings a row indicator
meets in the field, and running the plan against them found eight problems and
four gaps — [What the fixture found](#what-the-fixture-found). The problems are
decisions that are wrong; the gaps are capabilities nothing in 1-17 has. Three
problems were answered in the decisions before any of it was built; the four
gaps were not, and two of the whole set (F2, G1) apply to the read-only
feature as well.

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
precedence rule to settle the disagreements between them. (Decisions 13 and 14
add a second *entrance* and a *claim*, which is not the same thing: one map
still has one writer, and a claim is retired by the data rather than reconciled
against it.) It worked, and the
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

One cause on that list is not the client's to fix, and the list should not be
read as exhaustive: a read path served by a replica or a watch cache can report
busy, settled and busy again from three replies that each told the truth when
they were made, and no client-side generation check repairs it because neither
reply overtook the other. `demoapi`'s `?stale=` reproduces it. See F2 under
[What the fixture found](#what-the-fixture-found).

**The cost, stated plainly, because it is a design choice and not an
oversight:**

- An action dispatched from the TUI shows nothing on the row until a later poll
  reports it. Press Sync and the row keeps saying `OutOfSync` for up to one
  poll interval.
- Work that begins and ends between two observations is never seen at all.

Both are real, and both were covered by the removed layer at the cost of the
table above. Decisions 13-17 put the first one back — and most of the second —
for one line in the screen rather than six mechanisms here, by fixing it a
layer down. Nothing in decisions 1-12 changes.

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
`TestNoArrangementOfBusyRowsReflowsTheTable` holds that.

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
the subject of the next section rather than of this one:

1. **A separate handle.** Many systems answer "is this row busy" at a different
   endpoint from the one the list came from — an operations API, a job status
   resource, a `GET /operations/{id}` returned by the POST that started the
   work. Today a screen with one of those has nowhere to put the answer.
2. **A revision.** `finished_at`, `resourceVersion`, an ETag: a field whose
   contract is to change when the work does, which is the only way to notice
   work that began and ended between two polls.
3. **A separate collection to query.** The busy map as its own list — `GET
   /operations?status=running` — joined to the rows by key, rather than derived
   from a field on each row.

Items 1 and 3 look like the same question — *what does a screen do when the
busy-ness of a row is not a field on that row?* — and decision 13 was written
to answer both with one setter. The fixture shows they are not the same
question and it answers only item 3: see G2. Item 2 mostly dissolves: decision 14 covers the half of
it that matters in practice, and what is left is weak enough to stay dropped.

---

## Work the user starts

Status: **implemented.** Decisions 13-17 extend the feature above and are in
`pkg/activity`, forwarded through the three components, demonstrated in
`examples/patterns/activity`, and asserted in
`internal/componenttest/{expect,setbusy}_test.go` and
`internal/integration/activity_claim_test.go`.

They were designed before anything could be run against them, then run against
the widened fixture on paper, which found eight problems and four gaps — see
[What the fixture found](#what-the-fixture-found) and
[What the plan has no answer for](#what-the-plan-has-no-answer-for). The
decisions below are the amended versions that answer all eight; three more
things the build itself corrected are marked **Corrected during
implementation**. The four gaps are untouched — capabilities this design does
not have rather than mistakes in it, with G1 and G2 each large enough to be
their own pass. They are left standing rather than patched in place,
because the findings are what a build has to answer.

Everything the base does today is one pipeline with a single entrance:

```
poll → rows → ActivityWhen → observe() → Derive(map) → busy map → render
```

Each decision below is a change to that one picture, and it is worth reading
them that way: the previous design's mistake was not any single mechanism but
the decision to run a *second* pipeline alongside this one and arbitrate
between them.

### 13. `SetBusy` is a second entrance, not a second map.

**Problem.** The busy-ness of a row is not always a field on that row. An
operations API, a job-status resource, a `GET /operations?status=running`, a
handle returned by the POST that started the work — in all of them the screen
can find out who is busy, and has nowhere to put the answer, because the only
way in is a predicate over cells the component already holds.

The screen's workaround today is to fold the answer back into the rows — a
hidden column carrying a status the table never shows — so that `ActivityWhen`
can read it out again. That works, and it is silly: a value is written into a
cell purely so a predicate can recover it one call later.

**Decision.** Expose `Derive` one level up:

```go
s.table.SetBusy(map[string]string{"app-7": "Syncing"})
```

This is not a new mechanism. It is the same call `observe()` already makes,
with the screen supplying the map instead of the predicate computing it. One
map, one writer, replaced wholesale, exactly as before.

`ActivityWhen` is then correctly understood as **sugar for the common case**
where the status is on the row — not as the feature's definition. The two are
**mutually exclusive**: a component uses the predicate or it is told, never
both, because two writers for one map is the property decision 1 exists to
protect.

**Calling `SetBusy` on a component built with `ActivityWhen` panics** —
`table.SetBusy: built with Options.ActivityWhen; use one entrance or the
other`. Not a `Validate` error: there is no `Validate` in any of these
packages, and "`SetBusy` was called" is not an options-time fact in any case,
so options-time validation could not see it. A panic matches what the library
already does for a construction mistake that is always wrong regardless of data
(`app.New`, `tab.New`, `poll.New`), and the alternative — a silent no-op —
produces a component whose indicators simply never appear, which is the worst
of the three outcomes to debug.

**The entrance also drops an invariant that `ActivityWhen` held for free.** A
predicate runs over the rows the component holds, so every entry named a row on
screen. A map from `GET /jobs?status=running` need not: it can name a row on
another page of a paged table, or one deleted between the two reads. Left
alone, `Active()` goes true with nothing visible — so the tick chain animates
forever at a steady frame rate, drawing nothing — and `ActivityCount()`, which
all three components export, stops meaning "how many of my rows are working".

The fix is *scoping*, not filtering on the way in:

> **A `Set` keeps every key it is given, and answers `Active`, `Count` and
> `Render` over the intersection with the keys the component currently holds.**

Discarding unscoped keys at `SetBusy` time would be simpler and wrong: rows and
busy-ness arrive on separate cadences (below), so a key the component does not
hold *yet* is the normal case on a paged table, and a screen that scrolls a
known-busy row into view would find it inert until the next jobs poll happened
to come round. The component supplies its key set on every keyed swap; under
`ActivityWhen` that intersection is the identity, so nothing changes on the
predicate path. A screen that genuinely needs both merges them itself, with
`activity.Settled` available as the helper for its half:

```go
busy := map[string]string{}
for _, a := range s.apps {
    if label, ok := settled(a.Sync); ok { busy[a.ID] = label }
}
for id, op := range s.operations {       // the second source
    busy[id] = op.Phase
}
s.table.SetBusy(busy)
```

That merge is three lines and the precedence in it is the screen's to choose,
which is right: only the screen knows whether its operations endpoint or its
row status is the more current of the two.

**Two sources are two streams, and decision 15's counter has to split with
them.** A screen on this entrance polls `/apps` for rows and `/jobs` for
busy-ness, and the two replies arrive independently. One generation counter
cannot serialise both: an apps reply stamped 5 advances `seen` to 5, and the
jobs reply stamped 4 — the newest jobs data there is — is then dropped as
stale, permanently, because the next jobs request will race the next apps
request in exactly the same way.

> **Stamp per stream, not per screen. A write bumps every stream.**

```go
type stream struct{ gen, seen int }
s.apps, s.jobs    // one each; s.writing stays shared
```

Rule 33's existing instruction — stamp each fetch, ignore anything older than
the newest applied — reads as if a screen has one fetch. It has one *per
endpoint*, and the guard is per endpoint too. A dispatch bumps both, because a
write invalidates a read of either kind.

Which of the two then retires an expected entry is worth being explicit about:
**the jobs poll does**, because an expected entry is a claim about whether a
row is working and the jobs endpoint is what observes that. It is also the
endpoint most likely to lag a dispatch, since a separate operations collection
is usually a reconciler's output — which is not a reason to move the
responsibility, only the reason `Options.Settle` exists and the reason its
middle case is unavailable on this entrance (decision 16).

**This was written to answer removed-items 1 and 3 together**, on the argument
that "a separate handle" and "a separate collection" differ only in the shape
of the request, both ending at a `map[key]label` the screen holds and cannot
hand over. That is true of the collection and false of the handle, which is
keyed by dispatch rather than by row — G2.

### 14. An expected entry, deleted by the next observation.

**Problem.** Press Sync and nothing moves. The POST returns in 200ms, the next
poll is up to two seconds away, and for that whole window the row reads exactly
as it did before the keypress. The statusbar receipt covers it partially, but
the row — the thing the user is looking at, the thing the verb was *about* —
says nothing.

**Decision.** A second, temporary map of keys the screen has asked the server
to work on, rendered alongside the busy map:

```go
s.table.Expect(targets, "Syncing")
```

and one rule for its whole lifetime:

> **Every `Derive` deletes the expected map.**

No precedence rule, no handoff protocol, no expiry cap, no timer. An
observation that reaches `Derive` was taken after the request went out
(decision 15 is what guarantees that), so it is entitled to supersede the
claim — whether it confirms it or contradicts it. If it confirms, the key is in
the busy map and the row keeps spinning with no visible seam. If it
contradicts, the work is already over and the row should settle.

The two maps **union** rather than compete. A key in both renders from the busy
map, since that label is the observed word rather than the guess — though after
any `Derive` there is no overlap to resolve, and `Expect` on a key the last
observation already reported busy is a case `Actions()`' `Disabled` gate should
have prevented anyway.

**No timer is needed, which is worth stating because the previous design had
two.** An expected entry does not need to be aged out, because an observation
is always coming: the next poll clears it.

**When no observation is coming, the claim is unfounded and the screen says
so** — decision 17. The original text here said that a screen receiving no
observations has a dead poll chain and worse problems than a stuck indicator.
`/chaos/down?for=5s` is the counterexample: a healthy screen, polling, taking
503s, recovering cleanly, and holding a spinner for the whole window because a
fetch that never lands produces no `Derive`. That is a retraction, not a timer.

**The rule is only as good as decision 15's**, and it is worth being precise
about the dependency rather than leaving it implied. "Entitled to supersede"
means *issued after the write it would be superseding*. Nothing in the
component can check that — it sees rows arriving, never when they were asked
for — so the guarantee is entirely the screen's, and decision 15 is what makes
it. Two dispatches inside one poll interval is where a weaker version of that
rule shows: an observation that confirms the first and has never heard of the
second would retire both.

**This also covers most of removed-item 2.** `ActivityRevision` existed to
notice work that began and ended between two polls. The common instance of that
is work *the user just started* on a fast endpoint — a Refresh that the server
completes in 30ms — and an expected entry handles it exactly right: the row
spins for one poll interval, then the observation arrives saying settled and it
stops. What is left over is work *somebody else* started and finished inside
one interval, which is real, unobservable without a revision field, and worth a
`•` flash to approximately nobody. It stays dropped.

### 15. A screen does not apply a read taken across its own write.

**Problem, and it is the one that sank the previous design.** A poll already in
flight when the user pressed Sync returns the *pre-click* value. Under decision
14 that observation would clear the expected entry half a second after it
appeared, and the row would then sit dead until the next poll picked up the
real `Syncing`. Spinner, nothing, spinner — which reads as a glitch, and which
happens at a moment set by the poll phase rather than by anything the user did.

A poll *issued* after the keypress does it too, as long as it goes out before
the write lands — the same glitch, out of a read that looks perfectly current.

The previous design answered this by making the local entry *win* over the
derived one, and everything else followed from that: if derived cannot retire
local, then local needs its own retirement story, which needs a handoff, which
needs a cap, which needs something to show when the cap expires.

**Decision.** Recognise it as what it is — a stale read, not an activity
problem — and fix it where staleness is already handled:

> **A read is stale if it was issued before a write the screen has since had
> acknowledged, or while a write was outstanding. A screen drops both.**

```go
case action.ChosenMsg:
    s.gen++                                   // reads in flight predate the keypress
    s.writing++                               // and reads issued from here until the ack
    cmds = append(cmds, s.table.Expect(m.Targets, demoapi.SyncSyncing))

case dispatchedMsg:                           // the 202 came back
    s.writing--
    s.gen++                                   // reads issued during the POST predate the write

case fetchedMsg:
    if m.gen < s.seen || s.writing > 0 {
        return s, nil                         // stale, or taken mid-write
    }
```

Every polled screen already keeps the generation counter, because rule 33
requires it: replies arrive out of order over a real network, and one that
overtakes a newer one must be dropped. A write is simply another reason a read
is stale — it was taken before the thing the user just did. This is
read-your-own-writes, spelled in the counter the screen already has.

**Both bumps are needed, and the second is the one that matters.** An earlier
version of this decision bumped only at `action.ChosenMsg`, which drops reads
*in flight at the keypress* and nothing else. It leaves the POST's own latency
wide open: a read issued at t+50ms, landing at t+80ms, carries the newer
generation and a pre-write value, and clears the expected entry before the
write has even been accepted. That window is never zero, and
`POST /apps/{id}/sync?latency=3s` makes it three seconds. The `writing` count
closes it, and the ack-time bump catches the leftover case the count cannot —
a read issued long before the dispatch that happens to land after the ack, when
the count is already back to zero.

Both are two lines of bookkeeping in a screen that already keeps one of them.
The reply is then discarded before the component ever sees it, so `Derive` is
never called with a pre-write observation, and decision 14's one-line rule is
sound. **Six mechanisms collapse into three lines in the screen** — lines whose
absence is immediately visible (the row flickers once) rather than silently
wrong.

**The precondition, stated rather than assumed: the read path must be
monotonic.** All of this tracks when a read was *issued*; it assumes a server
whose replies get no older as time goes forward. A replica or a watch cache
breaks that assumption without breaking any rule — `?stale=` makes a server
report busy, settled and busy again from three replies that were each true when
they were made, and no client-side counter can repair it, because neither reply
overtook the other. Decision 16 is the only mitigation and it is a partial one.
A screen reading through a lagging replica gets a feature that flickers, and
that is a property of the deployment rather than a defect in the design.

**Rejected: having the library enforce it.** `pkg/activity` cannot; it has no
idea a fetch exists. The component cannot; `SetKeyedRows` tells it when rows
*arrived*, never when they were *requested*, and the gap between those is
precisely the window in question. A `SetKeyedRowsAsOf(rows, gen)` variant would
thread the number down to where it could be enforced, at the cost of a second
keyed setter on three components and a concept in the component that belongs to
the screen. The rule is cheaper and it is already half-written.

### 16. `Options.Settle`, counted in observations rather than seconds.

**Problem.** Decision 14's rule assumes the next observation knows about the
request. That is true of an API whose POST handler sets the status — and false
of a reconciler. Argo's controller, a Kubernetes watch cache, anything with a
control loop: the POST is accepted, the next poll honestly reports `OutOfSync`
because nothing has reconciled yet, the expected entry is deleted, and the row
flickers exactly as decision 15 was meant to prevent.

Worth knowing alongside it: firing `poll.Refresh()` on dispatch to shorten the
window makes this *worse*, not better. The sooner you read after a write, the
likelier you read before it lands.

**The first answer was a duration, and the fixture broke it.** A floor in
seconds has to be set from the server's reconcile lag, which the client cannot
know and the server does not publish. Worse, `?reconcile=9s&takes=1s` makes
every value wrong at once: the work is accepted at t=0, finished at t=1s, and
first reported at t=9s, so it is never observable running at any poll rate.
Any floor short enough not to outlive the work flickers, and any floor long
enough not to flicker holds a spinner eight seconds after the work is done.
Too short does nothing, too long lies — which is precisely the grace period
the Rejected section rejects, wearing a different name.

**Decision.** Count observations, not seconds, and let the data end the wait
whenever it can:

```go
Options.Settle int   // unchanged observations an unconfirmed claim survives; default 0
```

An expected entry meets one of three kinds of observation:

- **It reports the key busy.** The claim is confirmed; the key is in the busy
  map and the entry is redundant. This is the ordinary ending, and it has no
  visible seam.
- **It reports a *different* settled value than the key had when the claim was
  made.** The server has demonstrably acted — `OutOfSync` became `Synced` —
  so the entry is retired at once, whatever the allowance says.
- **It reports exactly what was there before the dispatch.** The server has
  said nothing new, so this observation is evidence of nothing. It consumes
  one of `Settle`'s allowance, and the entry is retired when the allowance
  runs out.

Only the third kind counts against the budget, which is what makes a small
number sufficient: an ordinary reconciler needs one or two unchanged reads of
patience, not a duration tuned to its control loop. And the number is in units
the screen actually owns — its own poll cadence — rather than in units of
server behaviour it is guessing at.

**Zero is the default and is decision 14's original rule exactly**: the next
observation retires the claim regardless. That is correct for any API whose
handler sets the status inline, which is most of them.

**A ceiling, not a floor, and that is the important inversion.** Every
observation still reduces the entry's remaining life, so an unconfirmed claim
always ends — after `Settle` unchanged reads at the very latest. `?blackhole=1`
is what that bound is for: a request accepted, given a job id, and never acted
on is indistinguishable from one still reconciling, and under a floor it would
spin until the floor expired while under an unbounded change-detection rule it
would spin forever.

**What it still cannot do**, named rather than mitigated: when the reconcile
lag exceeds the work's duration, no observation differs from the pre-dispatch
value until the work is already over, so the row spins for `Settle`
observations and learns nothing from any of them. The information is not in the
read path at all — recovering it needs the dispatch's own handle (G2) or a
revision field (removed-item 2), and a number here cannot substitute for
either.

**Under `SetBusy` the middle case is unavailable.** The screen hands over busy
keys only, so the component cannot see a settled value change and the allowance
does all the work. A screen on that entrance either sets `Settle` a little
higher or includes settled labels in what it reports.

It is still not `Dwell` renamed. `Dwell` held *derived* entries against a
source of truth that disagreed with itself between reads. `Settle` bounds an
*expected* entry against a source of truth that has not yet read its own write.
Different layer, different cause, and only one of them is being reinstated.

### 17. `Retract`, for a claim that has stopped being true.

**Problem.** The expected entry is optimistic by construction — it goes in when
the verb is dispatched, before the server has answered. Two things can then
make it false, and they are not equally forgiving.

The POST returns 409 or 500: the row spins for up to a poll interval after the
statusbar has already said it failed. Annoying, and self-correcting.

Or the observations stop. `/chaos/down?for=5s` refuses every read for a window
while the screen stays perfectly healthy — polling, taking 503s, recovering
afterwards. Decision 14 retires an expected entry on the next `Derive`, and
there is no next `Derive`, so the row spins for the length of the outage. The
same is true of a fetch that errors, a poll the user paused, and a screen whose
containing tab is hidden. Nothing self-corrects, because the thing that does
the correcting is what stopped.

**Decision.** Let the screen take it back:

```go
s.table.Retract(targets...)
```

and call it in both cases — on a write that failed, and on a read that failed:

```go
case dispatchedMsg:
    if m.err != nil {
        s.table.Retract(m.targets...)      // the request never landed
    }

case fetchedMsg:
    if m.err != nil {
        s.table.RetractAll()               // nothing is coming to retire them
    }
```

**On a failed write this is politeness; on a failed read it is required.** The
argument for optional — a screen that forgets is self-correcting within one
poll — holds only while observations are arriving. Where they have stopped, the
correction has stopped with them, and the failure mode is a stale spinner for
as long as the outage rather than for two seconds.

That asymmetry is the whole reason this decision is not "call `Retract` when
your POST fails". An expected entry is a claim that the next observation will
explain; when there is no next observation, the claim has nothing behind it and
the honest thing is to drop it. Which is also the narrow, in-scope half of
G1 — the row stops asserting something it cannot support, without this package
growing a notion of staleness it has no business holding.

### 18. Corrected during implementation: `Observe`, because a claim needs the value.

Decision 16's middle case — an observation reporting a *different* settled
value retires the claim outright — needs the key's value twice: when the claim
was made, and when the observation arrives. `Derive(busy)` carries neither. A
settled key is simply *absent* from that map, which is the whole point of its
shape, so there is nothing in it to compare.

So `Set` grew one method rather than a second map:

```go
func (s *Set) Observe(busy, values map[string]string) tea.Cmd
func (s *Set) Derive(busy map[string]string) tea.Cmd { return s.Observe(busy, nil) }
```

and `Expect` takes the same values at claim time. `Derive` keeps working
unchanged, and nil values read as "every observation is uninformative" — which
is the honest reading for a caller that cannot supply them rather than a
degraded one.

The cost is bounded by `Expecting()`: a component asks the Set which keys
carry a claim and collects values for those alone, so an ordinary poll on a
screen where nobody has dispatched anything gathers nothing at all.

### 19. Corrected during implementation: a tree has no value to compare.

The middle case cannot exist in `pkg/tree`, and the reason is worth keeping
because it is about identity rather than about convenience. The only string a
tree can read generically is `Node.Label()` — and a node's label *is* its
identity. The path that expansion state, cursor restore, marking and activity
all key on is built from it. A label that changed is not the same node
reporting something new; it is a different node, at a different path, which
this feature has never claimed to track.

So a tree passes nil values, and a claim there ends by being confirmed or by
spending its allowance. A tree backed by a reconciler wants a slightly larger
`Settle` than the same data in a table would.

That is the same shape as the `SetBusy` limitation in decision 16, arrived at
from the other direction: the middle case needs a value that is *about* the
key without *being* the key, and a tree node has none.

### 20. Corrected during implementation: under the shell, the acknowledgement is `runner.Captured`.

Decision 15 says the generation moves again when the write is acknowledged. In
a screen doing its own POST that is wherever the reply lands, and the example
in that decision reads correctly. Under the app shell it does not: the verb
runs through `action.Action.Run`, which the shell executes, so the screen never
sees the POST return at all. What it sees is `runner.Captured`, carrying the
run's `Tag` and its error.

The screen therefore records its claim against `action.RunKey(a, target)` when
it handles `ChosenMsg`, and matches the tag on the way back:

```go
case action.ChosenMsg:
    s.gen++
    s.writing++
    s.claimed[action.RunKey(m.Action, m.Target)] = keys
    cmds = append(cmds, s.table.Expect(keys, demoapi.SyncSyncing))

case runner.Captured:
    keys, ours := s.claimed[m.Tag]
    ...
    s.writing--
    s.gen++
    if m.Err != nil {
        s.table.Retract(keys...)
    }
```

One map on the screen, and it earns its place twice: it is also what makes
decision 17's `Retract` reachable, since the targets of the write that failed
are in it and nothing else on that message says which rows the run was about.

An action with no `Run` — a `Do` that pushes a screen — is skipped, or
`writing` would never come back down.

### 21. Corrected during implementation: the shell has to forward the dispatch.

Decision 20 above is written as if a screen under the app shell can see its own
verb being dispatched. It could not. `pkg/app` handled `action.ChosenMsg`,
called `runAction` and returned — the message never reached the stack, so the
`case action.ChosenMsg` in that decision was unreachable code.

This shipped, briefly, and is worth recording with the symptom rather than the
diagnosis: everything was right except the delivery, so `pkg/activity` was
correct, the component contract passed, the ordering rules were asserted
against a real server, and pressing Sync in the example still did nothing until
the next poll. Every test built its own screen for the occasion; none drove the
real one through the real shell.

**The shell now forwards it, from `runAction`** — the one place both the
direct path and the confirm path pass through:

```go
var fwd tea.Cmd
m.stack, fwd = m.stack.Update(action.ChosenMsg{Action: a, Target: target, Targets: targets})
```

From `runAction` rather than from where the menu's `ChosenMsg` arrives, because
those are different moments: a verb with `Confirm` is *armed* at the pick and
runs only if the modal says yes, and a screen told at the pick would claim rows
for work the user then cancelled. Forwarding from the shared path makes the
message mean "this is being dispatched now", which is what a screen can act on.
Both are asserted in `pkg/app/actions_test.go`.

It is also the smallest change that could work. The alternative — a new
message type for "an action started" — would have been a second thing meaning
what `ChosenMsg` already means, and rule 6 says a screen gets every message
anyway; this one was simply being swallowed.

`internal/integration/activity_example_test.go` now drives the example through
the shell and asserts the row claims on the keypress, which is the test whose
absence let this through.

### What this does not reinstate

The six mechanisms from decision 1's table, and what replaces each:

| Removed mechanism | Replaced by |
|---|---|
| local layer beats derived | nothing — the stale read never arrives (13, 15) |
| handoff on next observation | `Derive` deletes the expected map (14) |
| `Options.Confirm` expiry | nothing — every `Derive` clears, so the wait is bounded by the poll, or by a retraction when the polls stop (17) |
| `Options.Dwell` hysteresis | nothing — still a read-path problem (decision 1) |
| outcome hold + `✓`/`✗` glyphs | nothing — decision 7 stands unchanged |
| `ActivityRevision` | decision 14 for the common half; the rest stays dropped |

Also staying out: `Progress` and per-row progress counters, `StartMsg` /
`UpdateMsg` / `EndMsg`, and any `pkg/app` involvement. `pkg/activity` remains a
leaf with no tuilib imports.

### Open: who calls `Expect`?

Two answers, and the choice is not obvious.

**The screen**, explicitly, on the same line it bumps the generation
(recommended). The two halves of read-your-own-writes are then visibly
together, and `pkg/activity` stays a leaf. Costs the author two lines.

**The shell**, from `action.Set.Targets`, the way the removed design
broadcast — free for the author, and it puts the keys to use that `Set.Targets`
already carries for the `Exclusive` gate. But it re-couples `pkg/app` to
`pkg/activity` and brings back a message type, and it cannot do the job alone:
the shell has no access to the screen's fetch generation, so decision 15 still
has to be written by hand in the screen. Zero lines saved on the half that
matters, and a dependency restored on the half that does not.

The recommendation is the screen. Worth revisiting only if several real screens
end up writing the identical two lines.

### What landed in CLAUDE.md

Built as rule 33. The reasoning that shaped it:


Rule 33 currently says the data is the only source, and one of its
anti-patterns — "don't hold a row indicator open past what the data says" —
rejects grace periods, outcome holds and dampers in one sentence. Two of those
three stay rejected, and so is the fourth on a closer reading: `Options.Settle`
counts observations rather than seconds and is spent only by reads that say
nothing new, so it holds nothing open past what the data says — it declines to
treat a read that repeats the pre-dispatch value as having said anything. The
rule should say that, or the next reader deletes it as a violation.

The rule also grows decision 15, which is the only part of this design with
nothing but a compiler-free convention behind it — and it is now three lines in
the screen rather than one, so the anti-pattern it guards against needs naming
directly: *don't apply a read taken across your own write.* Rule 33 already
tells a polled screen to stamp each fetch and ignore anything older than the
newest applied; this extends that same sentence to writes, which is the
cheapest place for it to live. Decision 17's read-failure retraction goes in
alongside it, since a screen that retracts on a failed POST and not on a failed
fetch is the shape the fixture caught.

### Tests these needed

- An expected entry renders, and a subsequent `Derive` deletes it — including a
  `Derive` that reports the same key busy, which must produce no idle frame in
  between.
- `Expect` on a key already in the busy map renders the observed label, not the
  expected one.
- A screen that does **not** bump the generation flickers, asserted as an
  integration test against the example — this is the rule with no compiler
  behind it, so it needs the test that shows what its absence looks like.
- The same against a *slow* write: `?latency=` on the POST, a poll issued
  inside that window, and no idle frame on the row. A screen bumping only at
  the keypress fails this one and passes the previous one, which is why both
  are needed (F1).
- Two dispatches inside one poll interval, with an observation between them
  that knows about the first only: neither row goes idle (F3).
- A failed fetch retracts, asserted through `/chaos/down` — the row stops
  spinning inside the outage rather than at the end of it (F5).
- `Settle` spends its allowance only on observations that repeat the
  pre-dispatch value; one that reports the key busy confirms instead, and one
  that reports a different settled value retires the entry whatever the
  allowance says. Zero retires on the next observation.
- An unconfirmed claim always ends: `?blackhole=1`, accepted and never acted
  on, stops after `Settle` observations rather than spinning indefinitely.
- `Active`, `Count` and `Render` answer over the keys the component holds, and
  a `SetBusy` key for a row on another page survives the swap that brings the
  row into view (F6).
- Two streams with independent generations: a jobs reply is not dropped because
  an apps reply stamped later landed first (F7).
- `SetBusy` on a component built with `ActivityWhen` panics (F8).
- `Retract` clears one key and leaves its siblings.
- An expected entry survives a `SetTheme` rebuild, which means `Adopt` carries
  it, and so does the scope.
- A fetch that fails does not leave an expected entry spinning for the length
  of the outage (F5).
- An idle component with an expected entry still schedules exactly one tick
  chain, and stops when the entry goes.

All of them are written. The state machine is in
`pkg/activity/claim_test.go`; the contract that has to hold for all three
components is in `internal/componenttest/expect_test.go` (claims) and
`setbusy_test.go` (the second entrance), per the rule about not testing shared
behaviour in one component's package; and the two orderings that need a real
server are in `internal/integration/activity_claim_test.go`.

That last file is the one worth reading, because decision 15 is the rule with
no compiler behind it. It drives the same screen three ways — no guard, the
keypress bump alone, and the full rule — against a write held open with
`?latency=400ms`, and asserts that only the third keeps the row spinning. The
middle case is the point: a screen bumping at the keypress alone passes the
easy version of this test (a read already in flight when the user acts) and
fails this one, which is exactly why the first draft of the decision looked
sufficient.

### What the fixture found

Decisions 13-17 were written before anything could be run against them, so
`demoapi` was widened first: `?reconcile=`, `?stale=`, `?blackhole=`, `?say=`,
`?phases=`, `?takes=`, `GET /jobs?status=running`, and `/chaos/{down,delete,
spawn}` — each reproducing, on demand and on a pinned clock, one ordering a
per-row indicator meets in the field. `demoapi/scenarios_test.go` pins what
each knob promises, so a later screen test asserting "the row flickers here" is
asserting something about the screen rather than about a fixture that quietly
changed shape.

Running the plan against them, on paper, found two kinds of thing. **Eight
problems** (F1-F8): decisions that are wrong. **Four gaps** (G1-G4):
capabilities no decision covers, in
[What the plan has no answer for](#what-the-plan-has-no-answer-for). Two
smaller items are recorded rather than argued.

The distinction matters for what happens next. A problem is answered by
amending a decision before building it. A gap is answered by deciding whether
the feature wants the capability at all — and two of them (G1, G2) are large
enough to be their own design pass.

**All eight have been answered in the decisions**, and are kept here with their
original statements because the reasoning that produced them is the reason
those decisions now read as they do. Two of the answers are not fixes: F2 is a
precondition, stated as one in decision 15, and half of F7 was withdrawn as
wrong. What remains outstanding is the four gaps below, which are not defects
in 13-17 but capabilities it does not have.

**F1. The generation bump is in the wrong place (decision 15).** *Fixed in
place — decision 15 now bumps at the acknowledgement as well as the keypress,
and drops reads taken while a write is outstanding.* `gen++` on
`action.ChosenMsg` invalidates reads *in flight at the keypress*. It does
nothing about reads **issued after the keypress and before the write lands** —
those carry a newer generation and a pre-write value, so the screen accepts
them and `Derive` clears the expected entry. That window is the POST's own
latency and it is never zero; `POST /apps/{id}/sync?latency=3s` makes it
arbitrary. The bump belongs at the write's *acknowledgement*, not at the
keypress — or at both, since the dispatch-time bump is still what drops the
read already in flight. Decision 15's one line is two, and the second one is
the one that matters.

**F2. Read-your-own-writes assumes a monotonic read path.** Even a correctly
placed bump cannot help a reply that is newer in issue order and older in
content: `TestAlternatingStaleReadsGoBackwards` produces busy, settled, busy
from a server that genuinely said all three, and no client-side counter repairs
it. So decision 15's "`Derive` is never called with a pre-click observation" is
true of a server that reads its own writes and false of a replica, a watch
cache or anything fronted by one — and the six-into-one collapse rests on that
assumption rather than on the counter. Worth stating as a precondition.

This one reaches back into the **shipped** feature. Decision 1 says that when
an indicator flaps it is the read path, and lists four client-side causes — a
doubled tick chain, out-of-order replies, mixed setters, a component rebuilt
per fetch — under the instruction to fix the cause. A lagging replica is a
fifth, it is not the client's to fix, and a reader who takes the list as
exhaustive goes hunting for a defect in a poll chain that is working. The rule
needs the caveat whether or not 13-17 get built.

**F3. Wholesale clearing punishes the newest dispatch (decision 14).** *Fixed
by F1's correction rather than by a rule of its own: with the ack bump and the
outstanding-write check, an observation the screen accepts was issued after
every acknowledged write, so the wholesale clear is sound and decision 14 keeps
its one-line rule, and `Settle`'s allowance is per entry, which is what stops
one fresh dispatch preserving a stale claim elsewhere.*

Two syncs a second apart with one observation in between: the read confirms the
first and knows nothing of the second, and the second's expected entry — half a
second old — is deleted with it. As originally written this looked like a
reason to make the clear per-entry, and to promote `Settle` from a reconciler
knob to a requirement on any screen where a user can dispatch twice inside a
poll interval — which is every screen with a multi-select verb.

It is worth recording that this was the wrong conclusion, because the wrong
conclusion was the more elaborate one. The window it describes exists only
while a read issued *between* the second dispatch and its acknowledgement can
be applied, and that is the window F1 closes. With the ack bump and the
outstanding-write check there is nothing left for a per-entry clearing rule to
protect, and decision 14 keeps its single sentence.

**F4. `Settle` lies when the reconcile lag outlasts the work (decision 16).**
*Fixed in place — `Settle` is now an allowance of unchanged observations rather
than a duration, spent only by reads that say nothing new and bounded so an
unconfirmed claim always ends. The residual case is named and handed to G2.*
`TestReconcileLongerThanTheWorkIsNeverSeenRunning` — accepted at t=0, reported
at t=9s, finished at t=1s — is never observable running at any poll rate. A
`Settle` large enough to cover that reconcile held a spinner for eight seconds
after the work was done, which is the grace period the Rejected section
rejects, under a different name. The distinction the decision drew — a floor on
an expected entry, not a guess at how long work takes — was real and did not
survive a server whose reconcile lag exceeds its work duration.

The fix changed the units rather than the number. Nothing the client can
measure in seconds is the right length of that wait, but the client does know
whether a read told it anything: an observation repeating the pre-dispatch
value is evidence of nothing, one reporting the key busy confirms the claim,
and one reporting a different settled value proves the server acted. Counting
only the first kind makes a small allowance sufficient for an ordinary
reconciler and keeps every wait bounded. The case above remains unobservable —
that is a property of the server, not of the number — and is handed to G2.

**F5. An outage is not a dead poll chain (decision 14).** *Fixed in place —
decision 17 now covers a failed read as well as a failed write, and is required
rather than optional in that case.* The rule's escape
clause — if no observation is coming, the poll chain is dead and a stuck
indicator is the least of that screen's problems — describes a broken screen.
`/chaos/down?for=5s` describes a healthy one: polling, receiving 503s, and
recovering cleanly afterwards. There is simply no `Derive`, so an expected
entry set just before the window spins for the whole of it.

The screen is the only thing that knows a fetch failed, and `Retract` is
already the take-back primitive — so decision 17 generalises from "the POST
failed" to "nothing is coming", and stops being optional politeness. Its
absence is a lying spinner for the length of the outage rather than for one
poll interval.

**F6. `SetBusy` breaks the every-key-is-a-row invariant (decision 13).**
*Fixed in place — a `Set` keeps every key it is given and answers `Active`,
`Count` and `Render` over the intersection with the keys the component holds.
Scoped, not filtered on the way in, so paged rows still work.*
`ActivityWhen` runs over the rows the component holds, so every entry names a
row that exists. A map built from `GET /jobs?status=running` need not: it can
name a row on another page, or one `/chaos/delete/{id}` removed between the two
reads. Two exported surfaces change meaning — `Active()` goes true with nothing
on screen, so the tick chain animates nothing forever, and `ActivityCount()`
(on all three components) stops being "how many of my rows are working".
Intersecting the incoming map with the keys the component holds is a few lines
and restores the invariant; the alternative is to say `Count` counts claims.

**F7. Two endpoints are two cadences, and the clearing rule is attached to the
wrong one (13 + 14).** *Half fixed, half withdrawn — the real defect was one
generation counter serialising two independent streams, and decision 13 now
stamps per stream. The clearing rule was right where it was: an expected entry
is a claim about busy-ness, so the endpoint that observes busy-ness is what
retires it.* With rows from `/apps` and busy from `/jobs`, `SetBusy`
is the only `Derive`, so the expected map is cleared by the *jobs* poll — the
endpoint most likely to lag a dispatch, because a separate operations
collection is usually a reconciler's output. "One map, one writer, replaced
wholesale" assumed one observation; a separate collection makes an observation
two replies that interleave, each with its own staleness.

**F8. `Validate` cannot catch what decision 13 asks it to.** *Fixed in place —
`SetBusy` on a component built with `ActivityWhen` panics, matching what the
library already does for a construction mistake that is always wrong.* There is no
`Validate` in `pkg/activity`, `pkg/table`, `pkg/list` or `pkg/tree` today, and
"`SetBusy` was called" is not an options-time fact in any case. The exclusion
has to be a documented precedence or a panic at the call site — and note that
`Options.ActivityColumn` alone already enables the table's feature
(`actEnabled` is `ActivityColumn != "" || ActivityWhen != nil`), which is
exactly the configuration `SetBusy` wants.

Two smaller ones, recorded rather than argued: `Adopt` copies only the busy map,
so a theme rebuild would silently drop every expected entry unless it is on the
change list with `Render` / `Active` / `Count` / `State` (rule 4, decision 10).
And `State.Since` survives a key that is deleted and re-spawned between two
polls (`TestSpawnReusesADeletedID`), carrying one row's elapsed time onto a
different thing with the same name — latent today because nothing renders
`Since`, and sharper under `Expect`, where the same window lands a claim on a
row nobody dispatched against.

### What the plan has no answer for

The eight findings above are decisions that are wrong. These four are
capabilities the feature set does not have at all — no decision covers them and
none of 13-17 is a near miss. They came out of the knobs rather than out of
re-reading the plan, which is the argument for having built the knobs first.

**G1. There is no "I cannot see" state.** The whole feature has two states,
busy and not busy, plus `Expect`'s claim. `/chaos/down?for=5s` is a *healthy*
screen — polling, receiving 503s, recovering cleanly — and every row on it
renders as current, authoritative data for the whole window, because a fetch
that never lands produces no `Derive` and the last observation stands
unqualified. The same gap covers a fetch that errors, a paused poll, and a
screen whose tab is hidden. Staleness is not a row property and may well not
belong in this package at all — the pane title or the statusbar is the more
likely home — but nothing in the design says so, and the row currently lies
with a straight face. F5 is the expected-entry half of this; G1 is the whole of
it, and it applies to decisions 1-12 exactly as they ship today.

**G2. Nothing keeps the handle a dispatch returned.** `POST /apps/{id}/sync`
answers `202 {"job": "j-…"}`, and `GET /jobs?app=` resolves it. The plan drops
it: `Expect` is keyed by row and retired by observation, `SetBusy` takes a map
keyed by row, and neither can ask *is the thing I started still running?*
`?blackhole=1` — accepted, given an id, starts nothing — is the case that needs
exactly that, and its scenario test says so plainly: indistinguishable from
instant success unless the client asks about the id it was handed.

This falsifies a claim made above. Removed-items 1 (a separate handle) and 3 (a
separate collection) were declared the same question wearing different clothes,
answered together by decision 13's one setter. They are not the same question.
A collection is keyed by **row** and answers *who is busy*; a handle is keyed by
**dispatch** and answers *what became of the thing I started*. `SetBusy` does
the first. The second has no home in 13-17 — and it is the one of the two that
can tell a request that vanished from work that completed between two reads.

**G3. A status the predicate does not know has no escape hatch.**
`?say=Superseded` under `activity.Settled("Synced", "OutOfSync")` spins
forever: a terminal state the list does not name is in-flight by construction,
through every poll, until some other job moves the row. Decision 8 names this
as the trade a `Settled` list makes, and 13-17 add nothing to it — the only out
is abandoning the predicate for `SetBusy` and computing the whole map by hand,
which is a large price for one unrecognised value. `?say=` and `Options.Vocab`
turn that from a thought experiment into a one-line demo.

**G4. A key has no lifecycle.** `/chaos/delete/{id}` followed by
`/chaos/spawn?id=` hands back the same name attached to a different thing.
`State.Since` carries across it, and under decision 14 so does an expected
entry — a claim made about a row that no longer exists, landing on its
successor. Nothing distinguishes *the same key* from *the same row*, and no
observation can say a key **went away** as opposed to a key that is **not
busy**: both are absence from the map. Whether that matters depends entirely on
whether anything ever renders `Since`, which today nothing does.

### What the fixture confirms

Not everything it reproduced broke something. `?blackhole=1` — accepted, given
a job id, nothing started — retires on the next observation with no special handling,
which is the evidence that `Retract` is not needed on the success path.
`TestTheWorldAdvancesBehindAnOutage` shows the shipped feature trueing up on
recovery with nothing to reconcile, which is decision 1 working as designed. A
row spawned already busy needs nothing at all. And `?say=` / `?phases=` break
no rule: the observed label supersedes the expected one at the seam, as
decision 14 says it should — though it makes decision 6's explicit column width
load-bearing for `Expect` too, since the guess and the server's word have to
fit the same cell, and `Render` drops the label entirely below the width it
needs.

---

## API surface

### `pkg/activity`

```go
// State is one key's in-flight state.
type State struct {
    Label string     // the server's own word, or a claim's guess until observed
    Since time.Time  // first observed busy, not last: elapsed measures the work
}

type Options struct {
    Spinner *spinner.Spinner            // nil → spinner.Dot, matching pane
    Style   func(State, string) string  // nil → plain; see decision 11
    Settle  int                         // decision 16 — unchanged observations a claim survives; 0 = retire on the next
}

// Set is the keyed collection of busy rows plus the spinner that animates
// them. Components embed one; it is also the unit ActivityState carries.
type Set struct{ /* busy map, claims, scope, spinner.Model, tick bookkeeping */ }

func New(opts Options) Set

// Derive replaces the whole collection from one observation. Keys present are
// busy with that label; keys absent are not. Since survives for a key that
// stays busy. The command is the animation's first tick. It is Observe(busy, nil).
func (s *Set) Derive(busy map[string]string) tea.Cmd

// Observe is Derive plus the values each claim is judged against — 13/16.
func (s *Set) Observe(busy, values map[string]string) tea.Cmd

// Claims — decisions 14 and 17. at carries each key's value when claimed.
func (s *Set) Expect(keys []string, label string, at map[string]string) tea.Cmd
func (s Set) Expecting() []string   // which values are worth collecting
func (s *Set) Retract(keys ...string) // a write was refused
func (s *Set) RetractAll()            // reads have stopped

// Scope names the keys the component holds; entries outside it are kept but
// not rendered, counted or animated — decision 13.
func (s *Set) Scope(keys []string)

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

// The second entrance and the claim — decisions 13, 14, 17. The values a
// claim is judged against are gathered here rather than asked of the caller.
func (m *Model) SetBusy(busy map[string]string) tea.Cmd  // panics under ActivityWhen
func (m *Model) Expect(keys []string, label string) tea.Cmd
func (m *Model) Retract(keys ...string)
func (m *Model) RetractAll()
```

The table's feature is on when **either** `ActivityColumn` or `ActivityWhen` is
set, or when `SetBusy` or `Expect` is called; `list` and `tree` need only the
predicate, or neither on the `SetBusy` entrance.

### `pkg/theme`

```go
func (t Theme) Activity() activity.Options      // lipgloss, for list and tree
func (t Theme) activityCell() activity.Options  // ansi.CellColor, for table
```

nested into `List()`, `Table()` and `Tree()` alongside the `SpinnerStyle` each
already sets. Both are one `Accent` style: with no outcome states there is
nothing else to colour.

### `pkg/app`, `pkg/action`, `pkg/runner`, `pkg/glyph`

One line in `pkg/app`, added by decision 21: `runAction` forwards
`action.ChosenMsg` to the stack, so a screen learns that its own verb is being
dispatched. No new type, no new dependency — `pkg/app` already imports
`pkg/action`, and `pkg/activity` is still a leaf that has never heard of either.

`pkg/action`, `pkg/runner` and `pkg/glyph` are untouched. A screen reads the
acknowledgement off `runner.Captured`, which the shell already forwarded.

`Action.Busy` / `BusyLabel()` and `pkg/app`'s `actionRun.busy` went with the
removed layer — they existed only to carry a row label into the shell's
broadcast, and nothing read them afterwards. `Set.Targets` stays: it feeds the
per-target `Exclusive` gate, and under decision 17's recommendation it is also
what a screen passes to `Expect`.

### How decisions 13-17 changed the surface

`Set` grows three fields — `expected map[string]claim`, the scope, and the
allowance — and `Observe` grows a `retire` pass. `Render`, `Active`, `Count`,
`State` and `Adopt` all read the union, and `Adopt` carries the claims and the
scope as well as the observation, or a theme swap blanks the row the user just
acted on. The mutual exclusion is a panic at the `SetBusy` call site, not a
`Validate` error; everything else composes.

No new messages, no new timers, no new package dependencies.

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
- **Holding a spinner for a fixed grace period** so a poll might catch work
  the user dispatched. Too short does nothing, too long lies, and the right
  number is the poll interval the component does not know. Decision 14 waits
  for the next observation instead, which has a defined end. (`Options.Settle`
  in decision 16 is not this: it is an allowance of *observations*, spent only
  by reads that report nothing new, against a server that has not yet read its
  own write. It has no duration in it to be too short or too long.)
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

### `pkg/activity/claim_test.go` — the claim's state machine

The three endings side by side, because the difference between them *is*
`Settle`: confirmed, the value changed, or nothing new was said — and only the
last costs an observation. Zero retires on the next one whatever it says. A
claim nothing ever confirms still ends, which is the ceiling a request the
server accepted and dropped needs. Without values every observation is
uninformative, which is the honest reading for `SetBusy` and for `pkg/tree`
rather than a degraded one. `Since` measures from the claim, so elapsed time
starts at the keypress. `Retract` takes one, `RetractAll` takes the claims and
leaves the observations. An unscoped `Set` shows everything and one scoped to
nothing shows nothing — not the same state. An out-of-scope entry is kept and
comes back when its row arrives, and arms no tick chain while it cannot be
seen. `Adopt` carries claims and scope without aliasing them.

### `internal/componenttest/expect_test.go` and `setbusy_test.go` — across all three

The claim contract and the second entrance, asserted once for list, table and
tree rather than in whichever was edited last. The tree's exception is asserted
too (decision 19): a reader finding two components with a third ending and one
without should be able to find out here that it follows from identity rather
than from an omission.

### `internal/integration/activity_claim_test.go` — the rule with no compiler

Decision 15 driven three ways against a real server with the write held open by
`?latency=`: no guard, the keypress bump alone, and the full rule. Only the
third keeps the row spinning. The middle one is the point — it passes the easy
version (a read already in flight at the keypress) and fails this one — and a
failed read is asserted through `/chaos/down`, where nothing but the screen's
own retraction can end the claim.

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

*Questions 1 and 2 of the previous revision — where busy-ness comes from when
it is not a field on the row, and whether a dispatching action should show
anything before the next poll — are answered by decisions 13 and 14. What is
still open from that pass is only who calls `Expect`, which is recorded with
the decision rather than here.*

**1. Should `pkg/inspector` get this?** It has path-keyed fields and the same
`SetFields` preservation dance, so it would work. But a record viewer has no
verb that acts on a field (rule 32 declined marking there for the same reason),
so the only caller would be a screen animating something it fetched per-field.
Leaning no until someone has that screen.

**2. Should two panes showing the same key both spin?** They do. With the
broadcast gone this is no longer a scoping question about `action.Set` — it is
simply what deriving from data means, and two components holding the same row
will both observe it busy. Arguably correct. Recorded because the previous
design had an open question here and the answer changed shape rather than going
away.

**3. Should `Derive` be able to say "failed" rather than just "not busy"?**
Today the predicate is binary, so a derived run ending badly is communicated
entirely by the cell reverting to `failed` in the app's own styling. A
three-valued phase (resting / busy / failed) would let the library tint the row
for a moment on the way past. It is more API for something the data already
renders, and it needs the author to classify every resting value rather than
just the busy ones. Leaning no; revisit if a real screen reads flat.
