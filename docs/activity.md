# Row activity — design

Status: **implemented.** All twelve steps are in: `pkg/glyph`'s outcome marks,
`pkg/activity` with both layers, `theme.Activity()`, the three components,
`runner.CaptureStatus`, `action.Set.Targets` / `Action.Busy`, the shell's three
broadcasts and its per-target `Exclusive` gate, and the shared contract in
`internal/componenttest`.

Three ways in, all working: `SetActivity` directly, `ActivityWhen` from a
polled source of truth, and — free, with no screen wiring at all — an
`action.Set` that supplies `Targets`.

Six places the build corrected the design are marked **Corrected during
implementation** (decisions 8, 9, 10, 17, 19 and 20). Decisions 18-20 were
added after the first six steps, from a case the original had no answer for.

## Problem

A TUI over a remote system spends most of its time displaying state that
somebody is in the middle of changing. Press Sync on an argocd app and three
things should happen: the **row** says it is syncing, the **console** says what
the sync is doing, and the **statusbar** says when it finished. Today the
middle one is free, the last one is free, and the first one is the author's
problem.

`pkg/action` and `pkg/output` got the *event* right. `runner.Go` opens the
event, every line the action writes lands in the console under its label, `x`
kills it, the badge carries `⟳` while it runs, and the outcome paints the
statusbar — decisions 6 through 8 of `docs/actions.md`. What none of that
reaches is **the row the verb was about**. The user selects `cache-redis`,
picks Sync, the menu closes, and the table looks exactly as it did: the same
`Synced` in the Status column, which is now a lie, for as long as the sync
takes. The only evidence that anything is happening is a badge in the corner
and whatever the console shows if you go and look at it.

The library does have a spinner and it is the wrong granularity.
`pane.SetLoading` (rule 17) means "this whole component has no data yet" — a
centered glyph, the body replaced, all-or-nothing. There is no way to say
"two of these forty rows are busy and the other thirty-eight are still true."

So authors do it by hand, and the hand-rolled version is worse than it first
looks:

- The status text has to be baked into the row cells, so showing it means
  rebuilding and re-setting the whole row set.
- The spinner needs a frame, so it needs a ticker, so the screen grows a
  `spinner.Model` and a `spinner.TickMsg` case and re-sets every row on **every
  frame**.
- `pkg/poll` (rule 24) is usually swapping those same rows every couple of
  seconds, so there are two writers for one row set and whichever ran last
  wins. The spinner flickers, or the refresh does.
- None of it is reusable. The same forty lines get rewritten for the list on
  the next screen, and again for the tree on the one after.

The state is already in the program. `pkg/app` holds a set of live action runs
keyed by `action.RunKey(a, target)` (`pkg/app/app.go:370`), filled in
`runAction` and released on `runner.Captured`. Components hold their rows by
stable key — `SetKeyedRows`, `SetKeyedItems`, and the tree's node paths.

**They are two key spaces that never meet.** `RunKey` is the action's `Ident()`
paired with `Set.Target`, and `Target` is a *display string* — "cache-redis",
"3 items" — not the keys `Selection()` returned. The shell knows a Sync is
running. The table knows which row is `cache-redis`. Nothing joins them.

## Shape

A keyed, per-row in-flight state that components render themselves and the
shell fills in for free.

The screen already computes the keys; it just throws them away today:

```go
func (s *Screen) Actions() action.Set {
    return action.Set{
        Target:  s.apps.SelectionLabel(), // "cache-redis" — for the menu title
        Targets: s.apps.Selection(),      // new: the keys, for the rows
        Count:   len(s.apps.Selection()),
        Actions: []action.Action{{
            Label: "Sync",
            Busy:  "syncing",             // new: what the rows say meanwhile
            Run: func(ctx context.Context, out io.Writer) error {
                fmt.Fprintln(out, "sync started")      // → console, as today
                activity.Progress(out, "syncing 3/7")  // → the rows
                return s.api.Sync(ctx, targets)
            },
        }},
    }
}
```

and one field says where it lands:

```go
opts := t.Table()
opts.ActivityColumn = "Status"   // this cell, while the row is busy
```

That is the entire author-facing surface for the common case. Everything
between — start, tick, label, progress, finish, clear — is the shell
broadcasting two messages that rule 6 already delivers to every component on
screen.

```
NAME            SYNC          HEALTH
api-server      Synced        Healthy
cache-redis     ⠹ syncing     Healthy
web-frontend    OutOfSync     Degraded
worker-pool     ⠹ syncing     Progressing
```

Screens that do not use `pkg/action` — a fetch kicked off by enter, a refresh
bound to a key — call the same thing directly:

```go
return s, s.apps.SetActivity(key, "refreshing")
```

---

## Decisions

### 1. Activity is held by key, and keys are mandatory.

Rule 32's argument for marking applies here without a word changed, and for a
sharper reason. Marks held by index drift onto the wrong row when a polled
refresh reorders the set; an activity indicator held by index does the same,
except that the set it drifts within is *guaranteed* to be churning — the whole
point of showing a spinner is that something is changing the data underneath.

So activity works on `SetKeyedItems` / `SetKeyedRows` / tree paths, and on
anonymous `SetItems` / `SetRows` every activity call is a **deliberate no-op**.
An inert indicator is recoverable; a spinner sitting on the wrong row is a user
watching the wrong thing, which is worse than watching nothing.

A windowed table (`SetWindow`) is inert for the same reason marking is: it
carries rows without keys, and a sparse paged set is exactly where an
index-held indicator would land somewhere else.

### 2. The state lives in the component, not in the screen.

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

**Rejected: a `Decorator func(key string) string` hook on Options.** Superficially
more general — the screen returns whatever it likes per row. In practice it is
called once per visible row per frame, it invites I/O and allocation in a hot
path the way `Help()` does not, and it hands back a *string* that the component
must then place, style and truncate without knowing what it means. A typed
state the component understands can be right-aligned, can shrink to just its
glyph when the column is narrow (decision 9), and can be tested.

### 3. `pkg/activity` is a leaf package.

Three components need identical behaviour — a keyed map, a spinner, a hold
timer, terminal outcomes — and none of them should import another to get it.
That is the same argument that produced `pkg/geom`, `pkg/query` and
`pkg/glyph`, and it lands the same way: `pkg/activity` imports `bubbletea`,
`bubbles/spinner`, `lipgloss` and `pkg/glyph`, and nothing else from tuilib.

It matters more than usual here because the *shell* needs the message types
too. If activity lived inside `pkg/list`, then `pkg/app` broadcasting a start
would import `pkg/list` to name the message, and `pkg/table` would import
`pkg/list` to receive it.

### 4. The shell broadcasts; rule 6 delivers; the screen writes nothing.

`pkg/app` already forwards every message to the active screen, and rule 6
already obliges a screen to forward every message to its components. So the
shell posts `activity.StartMsg` when the run it launched reports itself
*started*, `activity.UpdateMsg` as that run reports progress, and
`activity.EndMsg` on the matching `runner.Captured` — and every component that
has activity enabled and holds one of those keys starts, relabels and stops on
its own.

Started, not launched. The distinction is not pedantry and it is not free; see
decision 17.

**No screen wiring at all.** This is the same payoff `OutputKey` had, where
every existing `app.Info` call became recoverable without a single call site
changing (rule 14): the plumbing is in the shell, and the author's contribution
is a label.

A component that holds none of the keys ignores the message, which is the
ordinary decline-what-isn't-yours behaviour it already performs for mouse
events outside its rect (rule 28).

**The known cost:** two components on one screen displaying the same key both
spin. See open question 1 — the fix is to scope the broadcast by
`focus.Token`, and the reason it is not decided here is that "the same object
shown in two panes" is a case where spinning both is arguably correct.

### 5. `action.Set` grows `Targets`; `Target` keeps its job.

```go
type Set struct {
    Target  string   // unchanged: the display label, titles the menu
    Targets []string // new: the keys the verbs will act on
    Count   int
    Actions []Action
}
```

Two fields rather than one because they answer different questions and only
one of them is renderable. `Target` is prose for a human — "3 items" is the
right menu title and a useless key. `Targets` is identity for a machine.
Deriving either from the other is impossible in both directions.

`Targets` is optional. A screen that does not set it gets exactly today's
behaviour: the menu titles itself from `Target`, the action runs, and no row
spins. `Count` stays authoritative for the arity gate (decision 21 of
`docs/actions.md`) rather than becoming `len(Targets)`, because a screen with
an expensive selection may legitimately report a count without materialising
the keys.

**`Validate` should report `len(Targets) > 0 && Count == 0`.** It already
collects dev-time mistakes into a `[]error` (`pkg/action/action.go:227`), and
that combination is a screen that filled in the new field and forgot the old
one — which silently disables every non-`Multi` action.

Filling the field on `Set` is only half of it: the keys then have to survive
the trip from the menu to the launch, through a confirm modal that may sit in
the middle. That is `ChosenMsg` and `pendingTgts`, in decision 17.

### 6. `Action.Busy` names the state, and defaults to the label.

```go
// Busy is what the target rows display while this action runs — "syncing",
// "refreshing", "deleting". Defaults to Label lowercased.
Busy string
```

A field rather than a derivation because the verb and the state are different
words in English and the gap is exactly where the user's attention is. "Sync"
is what you chose; "syncing" is what is happening. Defaulting to the label
keeps it free for the verbs where the distinction does not pay ("Refresh" →
"refresh" reads acceptably) and gets an author to a working spinner without
learning the field exists.

`Do` actions get no activity. A `Do` returns an opaque `tea.Cmd` — it may push
a screen or hand the terminal to `$EDITOR` — and the shell has no completion to
wait for, so a spinner it started could never be stopped. This is the same
boundary decision 7 of `docs/actions.md` drew for logging, for the same reason:
the library cannot narrate what it does not own.

### 7. Dynamic progress rides the writer the action already has.

The interesting half of the request is the label that *changes* — "syncing"
becoming "syncing 3/7" becoming "pruning". The action already holds a channel
back to the UI: the `io.Writer` from decision 5 of `docs/actions.md`. Reuse it,
via an optional interface, the way `http.Flusher` extends `http.ResponseWriter`:

```go
// in pkg/activity
type Progresser interface{ Progress(text string) }

// Progress updates the label on the rows this action is running against.
// A no-op when out does not support it, so an action written against a
// plain io.Writer keeps working.
func Progress(out io.Writer, text string) {
    if p, ok := out.(Progresser); ok {
        p.Progress(text)
    }
}
```

`runner.lineWriter` grows the method. It emits a neutral
`runner.CaptureStatus{RunID, Label, Tag, Text}` down the existing channel, and
`pkg/app` translates it into `activity.UpdateMsg` — the identical split
decision 6 of `docs/actions.md` established for log records, and the reason
`pkg/runner` still imports nothing from tuilib.

**Rejected: a second channel** — a `chan string`, a callback, a status field on
a result struct. The writer is already threaded everywhere the action's own
code runs, including into an `exec.Cmd`'s `Stdout`, and doubling the protocol
to carry one string is the trade decision 5 already refused.

**Rejected: inferring the label from the last line written.** Tempting — no new
API at all — and wrong twice: it makes every incidental log line a UI change,
and it forces authors to write log lines that read acceptably in a 12-cell
column.

**Corrected during implementation: `Progress` is narrower than this reads.**
The decision presents relabelling as the normal way to report a running action,
and building the example that way produced two bad outcomes. A single-target run
read "submitting" then "applying" while a multi-target run counted "1/3",
"2/3" — two vocabularies for one verb, so the row said something different
depending on how many rows were marked. And the counter was run-scoped but drawn
per row, so every marked row showed "2/3" as if that were its own progress,
which is not merely inconsistent but wrong.

The example now calls `Progress` nowhere. A row shows one word — the verb's
`Busy` text, set to the *server's own* status string — and holds it until an
observation says the work is settled. That is simpler to read, identical at
every arity, and makes the handoff invisible because both layers say the same
thing.

`Progress` stays, for an action with genuinely long and genuinely per-target
phases. It is not decoration for a two-second request, and a run-scoped value
must not be drawn as a per-row one.

`CaptureStatus` deliberately does **not** enter the log. It is a UI state
change, not news; a run that reports progress ten times would otherwise post
ten records the badge counts as one event but the reader has to scroll past.
An action that wants the progress in both places writes the line *and* calls
`Progress`, which is two lines and honest about being two things.

### 8. A finished row holds its outcome, then clears.

`EndMsg` does not erase the row's activity. It replaces the spinner with a
terminal glyph — `✓` or `✗` — holds it for `Hold` (default 2s), then clears.

A spinner that simply vanishes leaves no evidence of what happened. The
statusbar receipt (decision 8 of `docs/actions.md`) is the designed answer and
it is not enough here: it is one line for the whole run, it is wiped by the
next `tea.KeyMsg` (rule 20), and the user's eyes are on the row, not the
footer. When six rows sync and one fails, the per-row outcome is the only
surface that says *which*.

**Corrected during implementation.** `Hold: 0` means `DefaultHold`, not "clear
immediately". A zero value is what a caller who never heard of the field
supplies, and it should give them the considered default rather than silently
disable the feature. A negative `Hold` keeps the outcome until something else
clears it, which is what a screen wants when the underlying data will not
refresh on its own; clearing at once is `Hold: time.Nanosecond`, which nobody
has yet wanted.

The hold is a `tea.Tick` per finished key, not a global sweep — a sweep needs a
ticker running whenever anything might be holding, which is a timer the program
pays for while nothing is happening.

### 9. Where it renders, per component.

**`table` — a named column.** `Options.ActivityColumn string` is a column
*Title*, matched the way the filter's `key:value` scope matches (case-insensitive
prefix, via `query.ColumnByPrefix`). While a row is busy, that cell's content is
replaced. Titles rather than indices because a screen that reorders its columns
should not silently start decorating the wrong one.

This is the shape the request actually described, and it is the right one:
`Synced` → `⠹ syncing` → `✓ synced` → `Synced` reads as one cell changing its
mind, not as decoration appearing beside it.

Unset → a two-cell gutter, the same width and position as the mark gutter, with
the glyph alone.

**`list` and `tree` — a right-aligned trailing badge.**

```
worker-pool                              ⠹ syncing
```

Not a prefix: the leading cells are spoken for (list's gutter is two cells,
three with marking; the tree's is indent plus disclosure arrow), and pushing
rows sideways when a spinner appears is exactly the reflow that keeps `pkg/form`
drawing its validation errors on the border rather than under the field. When the row text and the badge cannot both fit, the
**row text truncates and the badge survives** — the badge is the news.

**When the space is too narrow for the label, the glyph wins.** A cell of width
8 renders `⠹ syncin`, which is worse than `⠹`. Below `len(label)+2` visible
cells the label is dropped rather than cut.

**Corrected during implementation: only `table` opts in.** The design implied a
symmetric switch on all three. There is nothing for `list` and `tree` to switch
on — a table has to be told *which column*, while a badge occupies no space at
all until something is running, so gating it would be a field whose "off"
position costs exactly as much as its "on" one. `ActivityColumn` therefore
gates the table and the other two are simply always ready.

### 10. Column width is the author's problem, and the doc has to say so.

**Corrected during implementation — the hazard is the opposite one.** The
design claimed a content-auto column would widen when `Synced` became
`⠹ syncing`, reflowing the table under the user. It cannot: `recomputeWidths`
sizes columns from the rows the table *holds*, and the indicator is substituted
at render time and never enters them. Activity cannot move a column, and
`TestTableActivityDoesNotReflowColumns` now holds that.

What actually goes wrong is the reverse. A column auto-sized to fit `Synced` is
six cells wide, the indicator is capped to the width it is given, and the label
is dropped in favour of the glyph (decision 9) — so the row spins but never
says *what* it is doing, and no amount of staring at it explains why.

So `ActivityColumn` still wants an explicit `Width`, for the opposite reason:
not to prevent a reflow, but to leave room for the longest label it will carry.
That belongs in the rule and in the field's doc comment, where it now is.

### 11. Multi-target: one event, many rows.

One action over three marked rows is one run, one log event and one statusbar
receipt (decision 7 of `docs/actions.md` settled that) — and three spinning
rows, because `StartMsg` carries all three keys.

They all clear together on `Captured`, which is right for a fan-out that
succeeds or fails as a unit and imprecise for one that does not. The refinement
is one function:

```go
func Done(out io.Writer, key string, err error)  // retire one key early
```

**Deferred, not refused.** The messages already carry keys, so it is additive
whenever someone has the case in front of them. Building it now means guessing
at how per-key failure should interact with the run's single `error` return,
and that guess is better made against a real action.

**Demonstrated as of `examples/patterns/activity` growing `Markable`.** Mark
several rows, pick Sync, and one run spins all of them, logs one event, and
retires them together. The last part is the deferral made visible rather than
hidden, and `TestOneRunSpinsEveryMarkedRow` asserts it — so if per-target
completion is ever built, that test is the one that should change.

### 12. `Exclusive` gets sharper for free.

`RunKey(a, target)` pairs the action's identity with the *display* target, so a
verb launched against "3 items" is held against the string `"Sync\x003 items"`.
Two overlapping selections that share a row can both run. Nobody has hit it
because marking-plus-Exclusive is rare, but it is a real hole.

With `Targets`, the shell can register one gate per key —
`Ident() + "\x00" + key` — so Sync on `{a, b}` and Sync on `{b, c}` correctly
refuses the second for `b` alone. The menu's `Disabled` reason gets more useful
too: "already running on cache-redis" rather than "already running".

This is a behaviour change to a shipped feature, so it is gated on `Targets`
being set: a screen that does not supply keys keeps the current semantics
exactly.

It needs a second index — `key → tag` — because the gate is now per key while
the release is still per run. That, and the run registry it hangs off, are in
decision 17.

### 13. Activity is not loading, not focus, and not marking.

Three neighbours, each of which someone will propose folding this into.

**Not `SetLoading`.** That is the pane's whole-body state and it *replaces* the
body — correct for "no data yet", nonsense for "thirty-eight of forty rows are
still true". They compose without interacting: a component can be loading with
activity keys pending, and when the body comes back the spinners are still on
the right rows because they are held by key.

**Not focus.** Activity never moves the cursor, never takes focus, and is not
in the tab order. It is a property of the data, not of the user's attention.
A row that finishes while the cursor is elsewhere must not pull the cursor to
itself — that is the auto-scroll bug every log viewer eventually grows.

**Not marking.** Marks are the user's selection and activity is the system's
state, and they are drawn in different places for that reason. They do overlap
usefully: mark three rows, sync them, watch three rows spin — which works
because both are keyed by the same key.

### 14. One spinner per component, and no help entries.

The component owns a `spinner.Model` distinct from the pane's loading spinner.
Safe because `spinner.TickMsg` carries the originating model's `ID` and bubbles
rejects ticks belonging to another spinner
(`bubbles@v1.0.0/spinner/spinner.go:141`) — so the two animate independently
without the component having to disambiguate.

Setters return `tea.Cmd` and the caller must batch it, exactly as
`pane.SetLoading` does (rule 17). The tick chain runs while any key is active
and stops when the last one clears, so an idle screen schedules nothing.

**Corrected during implementation: the tick chain has to heal itself.** The
chain lives in `tea.Cmd`s and survives only while the ticks it asks for come
back. `screen.Stack.Update` forwards to the **top screen only**, so pushing
anything over a screen with a spinner — the output console, a child view — has
its ticks delivered somewhere that drops them. The chain ends; `ticking` stays
true because nothing told the Set otherwise; `armTick` then refuses to start a
new one. The spinner is frozen for the rest of the session, on a row that is
still working.

`pkg/pane`'s loading spinner had the identical defect for the identical reason,
and so did `pkg/poll` — whose chain dying is worse, because then no observation
ever arrives to end a handoff (decision 19) and the row sits on a stale phase
until the user presses something. Three components, one cause.

**The first fix was per-component and insufficient.** Each Set learned to notice
a stalled chain and re-arm on the next message it saw. That is a real
improvement and it is not enough: after the action finishes, *nothing sends the
screen a message*, so "the next message" is the user's next keystroke. The
reported symptom was exactly that — the spinner resumed on a keypress and not
before, and the row stayed on "applying" indefinitely.

**The root fix is in `screen.Stack`, and it makes the library consistent with
itself.** `pkg/tab` already fans non-input messages out to every body, and rule
21 says why: a `tea.Tick` re-arm in an inactive tab has to keep working. The
screen stack delivered everything to the top screen alone. It now routes as tab
does — `tea.KeyMsg` and `mouse.Msg` to the top screen, everything else to every
screen — so the chains never die and there is nothing to revive. The cost is
that a covered screen keeps working, which is also the point: return from the
console and the data is current.

The per-component revival stays as defence for anyone driving components without
the stack, and because over-arming is provably harmless — bubbles tags each tick
and a spinner rejects one from a superseded chain, so two chains collapse into
one on the next frame.

**No new key bindings, so `Help()` and `HelpSections()` are untouched** (rule
10). Activity has no verbs: it is not clickable, not cancellable from the row,
and not navigable. Cancelling stays where it already is — `x` in the output
console, which kills the run and therefore ends the activity. Adding a second
kill affordance on the row would mean a second place to get the confirm, the
`Exclusive` release and the log record right.

### 15. State survives a theme rebuild, like every other state.

Rule 4 requires it, so the accessor pair exists:
`ActivityState() activity.Set` / `SetActivityState(activity.Set) tea.Cmd`. The
returned command is the tick — a rebuilt component starts a fresh spinner and
must be re-armed, or the theme swap leaves a frozen glyph on the row.

The spinner *frame* resets to zero across the swap. Accepted: the alternative
is serialising an animation phase through a state-restoration API, and nobody
can see it.

The keyed setters do **not** clear activity, for the same reason they do not
clear marks. A `pkg/poll` refresh that swaps every row leaves the spinners
exactly where they were, which is the entire point.

### 16. Theming follows rule 3, with the glyphs in `pkg/glyph`.

`t.Activity()` returns `activity.Options`, nested into each component's options
the way `Filter: t.Filter()` already is. The outcome marks join the shared
vocabulary as `glyph.Set.ActivityOK` / `ActivityFail` (`✓` / `✗`), so a theme
that sets its glyph set once gets these too.

Colors: the running spinner and label take `t.Accent`, matching every other
spinner in the library; `✓` takes `t.InfoBG` and `✗` takes `t.ErrorBG` as
*foregrounds*, which is the same reach rule 23 already makes for an
error-tinted alert.

Cell rendering inside a table goes through `pkg/ansi.CellColor`, never
`lipgloss.Render`, or the selected row's background is clobbered mid-cell
(rule 19). This is the one place the feature can produce a visible rendering
bug, and it is the one the library has already written down.

### 17. What the shell has to grow — and why the start is not where it looks.

Three pieces of plumbing that reading `runAction` does not suggest. All three
were found by tracing a two-row Sync end to end rather than by reading the
design, which is worth doing before writing any of it.

**The start broadcast cannot fire in `runAction`.** That is the obvious place —
the shell is right there holding the action and its targets — and the RunID
does not exist yet. `runner.GoWith` returns a `tea.Cmd`; the RunID is minted
*inside* that closure, when bubbletea executes it a cycle later. This is the
same fact decision 8 of `docs/actions.md` records about `Tag` — "the run ID is
minted inside the command, after the caller has returned" — and it is the
entire reason `Tag` exists.

So the start hangs off `runner.CaptureStarted`, which carries the RunID and the
Tag together. That is not a workaround; it is better than the launch site would
have been on two counts:

- It is the same branch that already registers the kill handle, increments the
  badge and appends the head record. The row spinner and the log line therefore
  appear in the **same frame** rather than one apart.
- It fails the way the rest of the run fails. If the goroutine never starts,
  nothing was broadcast, so no row is left spinning for a run that does not
  exist.

The cost is one tea cycle between the pick and the spinner — the latency the
badge already has, which nobody has noticed.

**`runAction` never sees the keys.** It takes `(a Action, target string)`, and
two paths reach it: `action.ChosenMsg` directly, and `confirm.ConfirmedMsg` by
way of `m.pendingAct` / `m.pendingTgt`. A destructive verb goes the long way
round, so both have to carry the keys or `Delete` is the one action that never
spins a row.

```go
// pkg/action
type ChosenMsg struct {
    Action  Action
    Target  string
    Targets []string   // new
}

// pkg/app
pendingTgts []string   // beside pendingTgt
func (m *Model) runAction(a action.Action, target string, targets []string) tea.Cmd
```

The menu holds the whole `Set` from `SetActions`, so filling `Targets` in
`chosen()` costs it nothing.

**`m.running` needs a value.** It is a `map[string]bool` used as a set
(`pkg/app/app.go:370`) — enough to grey out an `Exclusive` row, and not enough
once `CaptureStarted` has to ask what a tag was launched *for*:

```go
type actionRun struct {
    keys []string   // Set.Targets as of the launch
    busy string     // a.Busy, resolved
}

running  map[string]actionRun   // tag → run; released on Captured
busyKeys map[string]string      // key → tag; decision 12's per-key gate
```

`action.Menu.SetRunning` takes a `map[string]bool` today and can keep doing so
— the shell derives one from the richer map. Handing it the whole thing instead
would let a disabled row read "already running on cache-redis", which is
decision 12's better reason. That is a menu-rendering choice and blocks
nothing.

**Corrected during implementation: deriving that map needs the current `Set`.**
The menu asks whether `RunKey(a, Set.Target)` is running — one key, built from
the *selection's display label*. A per-target gate is held under
`RunKey(a, key)`, which that lookup can never match, so simply unioning
`busyKeys` into the set leaves the gate invisible on the very surface that
exists to explain it. The shell's helper is therefore `runningFor(set)`, not a
bare `runningKeys()`: for each action it tests the current `Set.Targets`
against `busyKeys` and, on a hit, reports it under the key the menu will
actually ask about. The two remaining call sites pass `m.menu.Set()`.

**Also corrected: `runner.Next` needs a `CaptureStatus` case.** Every message
that is not terminal has to chain the next read, and a status is not terminal.
Without the case the stream stalls after the first progress report — the run
keeps working, and nothing more is ever delivered from it, including its
`Captured`. A run that reports progress once would hang its own completion.

### 18. A polled source of truth can drive activity by itself.

Everything above assumes the TUI started the work. That covers a sync button
and misses an entire class of app.

An AWX job, an Argo rollout, a CI pipeline, a k8s deployment: the row's state
changes because a schedule fired, or a colleague clicked something, or the
previous step finished. `status: running` is a fact about the world that
arrives on a poll, and a screen showing it has exactly as much reason to spin
as one whose user pressed a key — more, since nobody in the room knows it is
happening. Under decisions 1-17 that screen gets nothing: no `StartMsg` was
broadcast, so no row moves.

The read-only half of the problem is also the easier half, because the answer
is a pure function of the data:

```go
o.ActivityColumn = "Status"
o.ActivityWhen = func(c table.Row) (label string, busy bool) {
    return activity.Busy("running", "pending", "waiting")(c[statusCol])
}
```

Every keyed swap walks the rows, builds `key → label` for the ones that match,
and hands the whole map to `Set.Derive`, which replaces the derived layer
wholesale. A row that matches spins; a row that stops matching stops. No
action, no shell, no `pkg/action` — a dashboard with a `pkg/poll` and a
predicate gets the whole feature.

**Derived entries have no hold and no outcome glyph.** When `running` becomes
`failed`, the cell simply goes back to rendering the row's own value, which
already says `failed`, in whatever colour the app gives it. A `✗` held over the
top of it for two seconds would be the library restating the data less
precisely than the data states itself. The hold exists for local entries
because *there* the outcome has nowhere else to appear.

**Two layers, and local wins.** A key can have both a local entry and a derived
one; `Render` prefers local. That ordering is what makes the mixed case a
handoff instead of a fight:

| | row shows | from |
|---|---|---|
| click Launch | `⠹ launching` | local |
| POST returns, first poll says `running` | `⠹ running` | derived |
| poll says `successful` | `successful` | the cell's own value |

and it is what stops a stale page wiping a live spinner. A poll that was
already in flight when the user clicked returns the *old* `successful`; derived
says "not busy"; the local entry is untouched, because derived does not retire
local. Without that rule every action would flicker off and on once, at a
moment determined by the poll phase.

Two helpers, and the second is usually the right one.
`activity.Busy(values...)` matches the statuses that mean work, labelled with
the matched value. `activity.Settled(values...)` inverts it: it names the
terminal states and treats everything else as work. Prefer `Settled` — it is
how these APIs document themselves, and it keeps spinning when a server learns
a new in-progress status, where `Busy` would treat that one as done. The mirror
risk is a new *settled* status spinning forever, so pick the list the server is
less likely to extend.

Either way the predicate takes the whole row rather than one cell, because the
status worth watching is not always the status worth showing, and a hidden
column or a second field is a normal thing to want.

### 19. A local entry hands off to the data, rather than expiring on a timer.

**This is the case that made decisions 18-20 necessary, and it is worth
following all the way through.**

An AWX launch is a POST that returns in 200ms and starts ten minutes of work on
a server. Under decision 8 alone:

```
t=0.0   click Launch          → local entry, "launching"
t=0.2   Run returns nil       → ✓, held
t=2.2   hold expires          → cleared
t=0.8   (server) job finishes
t=5.0   poll returns "successful"
```

The user saw a spinner for 200ms, a tick for two seconds, and then a cell that
already said `successful` before they clicked and says `successful` now.
**Nothing on screen distinguishes "it ran and succeeded" from "nothing
happened".** If the job had *failed* in that window, the cell changes to
`failed` five seconds later with no motion drawing the eye to it, and reads as
though it had been failed all along.

The instinct is to keep the spinner up for a few seconds and hope the poll
catches the job. That is a guess dressed as a feature: too short and it does
nothing, too long and the row lies about work that finished, and the right
number is the poll interval, which the component does not know.

The honest framing is that **the spinner should cover the observation gap, not
the work.** The TUI's problem in that window is not that the job is running —
it is that the TUI has nothing true to say, because the newest thing it knows
predates the user's click. That gap has a precise end: the next time the data
is observed.

So: **a local entry finished *successfully* is not cleared until the next
`Derive` after it finished.** The rendered outcome is deferred with it; the row
keeps its spinner and its label. Rewriting the trace:

```
t=0.0   click Launch          → local entry, "launching"
t=0.2   Run returns nil       → finished, awaiting confirmation; still spinning
t=5.0   poll observes         → Derive runs; "successful" is not busy
                              → local entry clears; the cell shows successful
```

The user sees continuous motion from the click until the answer arrives, then
the answer. The timing is fictional — the job was done at t=0.8 — but every
frame of it was the truth about *what the TUI knew*, which is the only thing a
UI can honestly report. And if the poll had returned `failed`, the same five
seconds of motion end on a cell that changed while they were watching it.

Four constraints keep this from becoming the timer it replaces:

- **Only on success.** A failed action already knows the outcome and reports it
  with `✗` and the normal hold: a dispatch that failed started nothing, so
  there is nothing for the data to confirm. (A POST that succeeded and whose
  response failed to parse is the edge this gets wrong, and it is the right way
  round — reporting the error the action returned.)
- **Only when the component has an `ActivityWhen`.** With no predicate there is
  no source of truth, nothing will ever call `Derive`, and decision 8's hold is
  the whole story. The rule switches itself off exactly where it cannot work.
- **Capped by `Options.Confirm` (default 30s).** Polling can be paused, the
  screen can be a tab nobody is looking at, the endpoint can be down. On expiry
  the entry clears through the normal hold, showing the outcome the action
  reported. A cap is not a guess about the work; it is a bound on how long the
  library will wait to be told something.
- **Derived may take over instead of clearing.** If the confirming `Derive`
  says the key *is* busy, the local entry retires and the derived one is
  already there — the handoff of decision 18, with no frame in between.

**Corrected during implementation.** A fifth constraint was missing, and it
only shows up once decision 20 is built alongside this one: the observation
that *ends* a handoff must also suppress the flash for that key. Otherwise the
ordinary happy path reports itself twice — the user watches the spinner from
click to completion, and then the revision change fires a `•` on top of it. The
`Set` therefore carries a `settled` set: keys the previous observation reported
busy, plus keys whose handoff this one just retired. See decision 20.

**What this does not fix, and cannot.** If the user never acted and the job
both starts and finishes between two polls, nothing in the library can know it
happened — see decision 20.

### 21. Conflict with the source of truth is three layers, and the server is the only authority.

What happens when the user acts on a row the server just started working on?
Nothing in decisions 1-20 answers that. `Exclusive` looks like the answer and is
not: it gates runs *this session* launched, held in the shell's own registry,
and it knows nothing about work that arrived on a poll.

The honest structure has three layers, and none of them is optional.

**The server refuses.** It is the only thing that knows, because the client's
newest information is an observation that may be a poll interval old. A real API
answers 409; `demoapi` now does too, and used to accept both commands and let
two jobs race on one row with the later completion overwriting the earlier
one's result. A fixture cannot teach a client to handle a conflict it never
produces.

**The action surfaces the refusal.** A non-2xx is an error, the action returns
it, and the existing path carries it: `EndMsg{Err}` puts ✗ on the row, the
statusbar says why, and the console keeps the body. No new machinery — this is
decision 19's failure branch doing its job.

**The screen pre-empts, best-effort.** `Action.Disabled` already exists for
"this verb does not apply right now", and the screen already holds what it needs:
a derived entry with no `RunID` means the last poll said the server was working
on that row. So `Actions()` dims the verbs and names the reason — "already
syncing" — and the ordinary case never reaches the server at all.

Best-effort is the accurate description, and calling it anything stronger would
be the mistake. The check reads the last observation; a schedule can fire in the
window between that observation and the POST. Which is precisely why the server
still has to reject and the action still has to report.

**The read side needs the same discipline, and a plainly polled screen gets no
help with it.** `source.Deliver` carries a generation so an overtaken reply
cannot paint stale rows under a newer one. A screen that just polls and calls
`SetKeyedRows` has nothing equivalent, and over a real network replies do arrive
out of order. `examples/patterns/activity` therefore stamps each fetch and drops
anything older than the newest applied — five lines, and the same idea the
coordinator encodes for windowed tables. A library helper here is an open
question rather than a gap with an obvious shape.

### 20. What no amount of polling can see, and the one escape hatch.

A busy phase entirely contained between two observations is invisible. The
value was `successful` at the last poll and is `successful` now; a run happened
in between; the data as sampled carries no evidence of it. This is not a
rendering problem or a state-machine problem — the information is genuinely
absent from what the screen was given.

Decision 19 covers the case where *the user acted*, because then the TUI has a
second source: it knows it asked. For work nobody in this session started,
there is nothing to reason from.

Unless the payload happens to carry a revision — and for this class of API it
usually does. `finished_at`, `last_job_run`, `resourceVersion`, an ETag, a
monotonic `id` of the latest run. When one exists:

```go
o.ActivityRevision = func(c table.Row) string { return c[finishedAtCol] }
```

On each swap, a key whose revision changed while it was **not** busy gets a
brief outcome flash — the same two-second hold a local entry uses, saying "this
row changed under you" rather than restating the value. Unset, nothing happens
and the row is as quiet as it is today.

Deliberately opt-in and deliberately separate from `ActivityWhen`. The two
answer different questions — "is this row working" versus "did this row's work
change" — and a payload very often supports the first and not the second.
Folding them into one predicate would make the honest answer ("I can tell you
it is running, I cannot tell you it ran") unexpressible.

**Rejected: diffing whole rows to detect change.** Every poll that reformats a
timestamp, reorders a list, or recomputes an age column would flash every row.
A revision is a field whose *contract* is that it changes when the work does;
a row diff is a heuristic that mistakes rendering for meaning.

**Corrected during implementation: a flash must never follow a spinner.** The
design described the flash purely as "revision changed while not busy", which
is true of every normal completion the moment the work stops being busy — so a
watched run ended in a spinner *and* a `•`, saying the same thing twice. A
flash is for work that was never visible at all, so it is now suppressed for
any key the previous observation reported busy, or whose handoff the current
one retired (decision 19). Without both halves the suppression has a hole: a
handoff that resolves straight to a resting status never appears in the
previous observation's busy set.

**Also corrected: construction counts as an observation.** `pkg/tree` takes its
data through `Options.Root`, so a tree that never calls `SetRoot` had never
derived, and decision 19 read that as "no source of truth" and reported its
first action's outcome immediately. `New` now observes when it is handed a
root. `list` and `table` need no equivalent: their `Options.Items` / `Rows` are
anonymous, and derived state is keyed.

**Rejected: an event stream instead.** Correct, and out of scope — the moment
an API offers one, the screen pushes `StartMsg`/`EndMsg` itself through the
direct API and needs none of this. These three decisions are for the far more
common case where all you have is a list endpoint and an interval.

---

## API surface

### `pkg/activity` (new)

```go
// State is one key's in-flight state.
type State struct {
    Label string    // "syncing", "syncing 3/7"
    RunID int64     // the run that owns it; 0 when set directly
    Since time.Time
    Done  bool      // terminal: holding its outcome before clearing
    Err   error     // non-nil on a failed outcome
}

// Set is the keyed collection plus the spinner that animates it. Components
// embed one; it is also the unit ActivityState/SetActivityState carry.
type Set struct{ /* map[string]State, spinner.Model, opts */ }

func New(opts Options) Set

func (s *Set) Start(key, label string) tea.Cmd
func (s *Set) Update(key, label string)
func (s *Set) Finish(key string, err error) tea.Cmd  // holds, then clears
func (s *Set) Clear(key string)
func (s *Set) Active() bool
func (s *Set) State(key string) (State, bool)
func (s *Set) Count() int
func (s Set) Render(key string, width int) (string, bool)  // "" + false when idle

// Handle applies the shell's broadcasts. Components call it from Update and
// return the command; it is a no-op for keys the component does not hold.
func (s *Set) Handle(msg tea.Msg, holds func(key string) bool) tea.Cmd

// Derive replaces the derived layer wholesale from an observation of the data
// (decision 18). Keys present are busy with that label; keys absent are not.
// Local entries are untouched, and win when both exist.
//
// A Derive is also the observation decision 19 waits for: it retires any local
// entry that finished successfully since the last one.
func (s *Set) Derive(busy map[string]string) tea.Cmd

// Busy builds the ordinary predicate: a case-insensitive match against values
// that mean "in progress", labelled with the matched value.
func Busy(values ...string) func(value string) (label string, busy bool)

type Options struct {
    Spinner    *spinner.Spinner // default spinner.Dot, matching pane
    Style      lipgloss.Style   // running: spinner + label
    OKStyle    lipgloss.Style
    ErrorStyle lipgloss.Style
    Glyphs     glyph.Set
    Hold       time.Duration    // default 2s; 0 means the default, <0 never
    Confirm    time.Duration    // default 30s; cap on decision 19's handoff
}

// Messages. The shell posts these; components match them via Handle.
type StartMsg  struct { Keys []string; Label string; RunID int64 }
type UpdateMsg struct { Keys []string; Label string; RunID int64 }
type EndMsg    struct { RunID int64; Err error }

// Progress reports a label change from inside an action's Run. A no-op when
// out does not implement Progresser.
type Progresser interface{ Progress(text string) }
func Progress(out io.Writer, text string)
```

### `pkg/list`, `pkg/table`, `pkg/tree` (additions)

```go
// Options
Activity activity.Options  // from t.Activity(), via the theme builders

// table only: the column whose cell is replaced while a row is busy.
// Matched on Title, case-insensitive prefix. Unset turns the feature off
// on a table; list and tree need no switch. Give it a Width wide enough
// for the longest label — see decision 10.
ActivityColumn string

// ActivityWhen derives in-flight state from the row's own data, for work
// nobody in this session started (decision 18). Evaluated on every keyed
// swap. Per-component signature, since each speaks its own data shape:
//   table: func(cells Row) (label string, busy bool)
//   list:  func(item string) (label string, busy bool)
//   tree:  func(n Node) (label string, busy bool)
ActivityWhen func(...) (string, bool)

// ActivityRevision, when set, is a per-row value that changes whenever the
// row's underlying work does — finished_at, resourceVersion, an ETag. A
// change observed while the row is not busy flashes an outcome (decision
// 20). Same per-component signature shape as ActivityWhen.
ActivityRevision func(...) string

// Model
func (m *Model) SetActivity(key, label string) tea.Cmd
func (m *Model) EndActivity(key string, err error) tea.Cmd
func (m *Model) ClearActivity(key string)
func (m Model) ActivityState() activity.Set
func (m *Model) SetActivityState(s activity.Set) tea.Cmd  // rule 4 rebuilds
```

### `pkg/action` (additions)

```go
// Set
Targets []string  // the keys Selection() returned; optional

// Action
Busy string       // row label while this runs; defaults to Label lowercased

// ChosenMsg
Targets []string  // carried from the Set, so the confirm detour keeps them
```

### `pkg/runner` (additions — still no tuilib imports)

```go
type CaptureStatus struct {
    RunID int64
    Label string
    Tag   string
    Text  string
}

func (w *lineWriter) Progress(text string)  // satisfies activity.Progresser
```

### `pkg/app` (additions)

No new options — the feature rides `ActionsKey`, which is already the gate for
everything it extends.

| Trigger | Broadcast |
|---|---|
| `runner.CaptureStarted` whose `Tag` names a run launched with keys | `activity.StartMsg{Keys, Label: busy, RunID}` |
| `runner.CaptureStatus` with a `Tag` | `activity.UpdateMsg{RunID, Label: msg.Text}` |
| `runner.Captured` with a `Tag` | `activity.EndMsg{RunID, Err}` |

Not at the launch site — see decision 17. The run registry changes shape to
support the first row:

```go
running  map[string]actionRun   // was map[string]bool
busyKeys map[string]string
```

### `pkg/theme` (addition)

```go
func (t Theme) Activity() activity.Options
```

nested into `List()`, `Table()` and `Tree()`, alongside the `SpinnerStyle` each
already sets.

### `pkg/glyph` (addition)

```go
ActivityOK   string // "✓"
ActivityFail string // "✗"
```

---

## Implementation order

1. **`pkg/glyph`** — the two glyphs. One commit, no dependents yet.
2. **`pkg/activity`** — `State`, `Set`, `Options`, the three messages,
   `Progress`. Unit tests for the hold timer, terminal outcomes, and `Render`'s
   narrow-width fallback. No component involvement.
3. **`pkg/theme`** — `t.Activity()`, nested into the three builders.
4. **`pkg/table`** — `Options.ActivityColumn`, column resolution, cell
   substitution through `ansi.CellColor`, the gutter fallback, the setters and
   the rule-4 pair. Table first because it is the shape the request asked for
   and the one with real placement problems (flex widths, windowed inertness).
5. **`pkg/list`** and **`pkg/tree`** — the trailing badge. Same setters.
6. **`internal/componenttest/activity_test.go`** — the shared contract, across
   all three. See Tests.
7. ~~**`pkg/runner`**~~ — `CaptureStatus`, `lineWriter.Progress`, and the
   `Next` case without which a status stalls its own run.
8. ~~**`pkg/action`**~~ — `Set.Targets`, `Action.Busy` / `BusyLabel()`,
   `ChosenMsg.Targets`, and `Validate`'s new check.
9. ~~**`pkg/app`**~~ — the keys threaded through `ChosenMsg` → `runAction` →
   the confirm detour, `m.running` growing a value plus `busyKeys`, the three
   broadcasts hung off the capture message family, and `runningFor(set)`.
   Decision 17: this step is bigger than it reads.
10. **`examples/patterns/actions`** grows a Status column and a `Busy` label, or
    — decision pending, see open question 3 — a new
    `examples/patterns/activity` whose subject this is.
11. ~~**`examples/patterns/activity`**~~ + launcher entry.
12. ~~**CLAUDE.md**~~ — rule 33, five anti-patterns, and an entry under "Where
    to learn more".

**All twelve steps are done.** 1-6 give any screen a per-row spinner through
the direct API, 7-9 make it free under `pkg/action`, and 10-12 let a polled
source of truth drive it with no actions at all.

10. ~~**`pkg/activity`**~~ — `Derive`, the local/derived precedence in
    `stateOf`, `Busy`, `Change`/`Revise`, `Options.Confirm`, and the handoff.
11. ~~**The three components**~~ — `ActivityWhen` and `ActivityRevision`,
    evaluated in an `observe()` called from the keyed setters (and from
    `tree.New`, which is where the data arrives for a tree). The commands they
    produce queue on `actCmd` and flush through the existing `flushMsgs`, since
    a setter has no return value.
12. ~~**`internal/componenttest`**~~ — the derived contract across all three:
    data-driven start and stop, the stale-observation guard, and the full
    handoff.

## Rejected, worth remembering

- **A `Decorator func(key string) string` hook.** More general, called in a hot
  path, and returns an untyped string the component must place blindly. See
  decision 2.
- **Reusing `pane.SetLoading` with a row filter.** Conflates "no data" with
  "some rows busy"; the pane has no notion of rows and should not acquire one.
- **A second channel for progress** (`chan string`, a callback, a status field
  on a result). The `io.Writer` is already threaded everywhere; doubling the
  protocol for one string is what decision 5 of `docs/actions.md` refused.
- **Inferring the row label from the last log line.** Makes every incidental
  log line a UI change and forces log prose to read well in a 12-cell column.
- **Logging `CaptureStatus` into the console.** Ten progress updates would post
  ten records in an event the badge counts as one. It is a UI state change, not
  news.
- **Activity state in the shell, keyed by `RunKey`.** The shell would then need
  to know which component holds which key and where that component draws its
  rows — rule 9 inverted.
- **Deriving `Targets` from `Target`.** "3 items" is not a key, and no amount
  of parsing makes it one.
- **A cancel affordance on the row.** A second place to get the confirm, the
  `Exclusive` release and the log record right. `x` in the console already
  kills the run, which ends the activity.
- **Auto-scrolling to a row that finishes.** The cursor belongs to the user.
- **Deriving the row label from the log line just written.** The activity
  example did exactly this, taking the first word of each line, and put
  "found", "waiting" and finally "sync" on the row — the last from "sync
  operation complete". Decision 7 had already rejected it in the abstract; a
  demo doing it anyway is how it came back. Phases are written deliberately, at
  the two or three points the client actually knows about.
- **Holding a local spinner for a fixed grace period** so a poll might catch
  the work (decision 19). Too short does nothing, too long lies, and the right
  number is the poll interval the component does not know. Waiting for the next
  observation is the same idea with a defined end.
- **Giving derived entries a hold and an outcome glyph.** The data already says
  `failed`, in the app's own colours. A `✗` over the top of it is the library
  restating the row less precisely than the row states itself.
- **Letting derived state retire a local entry.** A poll already in flight when
  the user clicks returns the pre-click value, so every action would flicker
  off and on once, at a moment set by the poll phase.
- **Diffing whole rows to detect change** (decision 20). Any poll that
  reformats a timestamp or recomputes an age column would flash every row. A
  revision field's contract is that it changes when the work does; a row diff
  mistakes rendering for meaning.

## Tests

The shared contract goes in `internal/componenttest`, not in whichever
component gets built first. The anti-pattern is on the record: the
click-to-blur behaviour was written for `pkg/list`, rolled out to five other
components by a script that omitted it, and covered by a test living in
`pkg/list` — so five components shipped broken and the suite stayed green.
`marking_test.go` is the precedent to copy.

`internal/componenttest/activity_test.go`, across `list`, `table` and `tree`:

- Activity on a keyed row renders; on an anonymous row it is inert.
- A keyed swap that reorders rows keeps the indicator on the same key.
- A filter that hides a busy row does not clear it; unfiltering shows it still
  running.
- `SetActivityState` across a rebuilt component restores the keys and returns a
  non-nil tick.
- `StartMsg` for a key the component does not hold changes nothing and returns
  no command.
- `EndMsg` shows the outcome glyph, and the key clears after `Hold`.
- No activity anywhere → `Update` returns no command, so an idle screen
  schedules no ticks.

Per-component:

- `table`: `ActivityColumn` resolves by title prefix; an unresolvable name
  falls back to the gutter rather than panicking or picking column 0; a windowed
  table is inert; the selected row's background survives an active cell
  (`lipgloss.SetColorProfile(termenv.TrueColor)` in `TestMain`, or the assertion
  is vacuous).
- `list` / `tree`: the badge is right-aligned and survives truncation of the row
  text; below `len(label)+2` cells only the glyph is drawn.

`pkg/app`, all of it decision 17's surface:

- An action with `Targets` broadcasts start and end around the run; one without
  broadcasts neither.
- The start rides `CaptureStarted`, not the launch — so a `runAction` that
  returns without the command ever executing broadcasts nothing, and no row is
  left spinning for a run that never began.
- An action with a `Confirm` still carries its keys: the assertion is on the
  `ChosenMsg` → `armConfirm` → `ConfirmedMsg` path, which is the one that drops
  them if `pendingTgts` is forgotten, and which every destructive verb takes.
- `CaptureStatus` with a `Tag` becomes `UpdateMsg` and appends no record.
- `Captured` releases both the tag and every `busyKeys` entry pointing at it,
  or an `Exclusive` verb is permanently disabled on a row that finished.

`pkg/runner`: `Progress` on the writer emits `CaptureStatus` and does not
disturb line assembly mid-write.

Decisions 18-20, in `internal/componenttest` across all three components:

- A keyed swap whose row matches `ActivityWhen` starts a spinner with no
  action, broadcast or setter call anywhere.
- A swap whose row stops matching clears it, with no outcome glyph and no hold.
- A local entry and a derived entry on one key render the local one.
- A derived "not busy" observation does **not** retire a live local entry —
  the stale-poll flicker, which is the one a real app hits within a minute of
  first use.
- A local entry finished successfully survives until the next `Derive`, then
  clears; the same entry finished with an error does not wait, and shows `✗`
  through the normal hold.
- With no `ActivityWhen`, a finished local entry clears on its hold as before:
  the handoff must switch itself off where nothing will ever confirm it.
- `Options.Confirm` expiring clears an unconfirmed entry through the normal
  hold rather than leaving it spinning.
- A confirming `Derive` that reports the key busy hands over with no idle frame
  between the two entries.
- `ActivityRevision` changing while a row is not busy flashes an outcome;
  changing while it *is* busy does not (the spinner already says so); unset
  does nothing at all.

## Open questions

**1. Should the broadcast be scoped to one component?** Two panes showing the
same key both spin. Scoping means `action.Set` carrying the `focus.Token` of
the component that produced the selection — `FocusToken()` is already public on
every component, and `pkg/action` importing `pkg/focus` closes no cycle. The
argument against is that the same object shown twice arguably *should* spin
twice, and the token adds a required field to a struct whose current version is
three plain values. Recommendation: ship unscoped, add `Set.Source` if a real
screen is bothered by it.

**2. Should `pkg/inspector` get this?** It has path-keyed fields and the same
`SetFields` preservation dance, so it would work. But a record viewer has no
verb that acts on a field (rule 32 declined marking there for exactly this
reason), so the only caller would be a screen animating something it fetched
per-field. Leaning no until someone has that screen.

**3. ~~Where does the example live?~~ Settled: `examples/patterns/activity`,
with `actions` left alone.** An argocd-shaped table — Name / Sync / Health, a
hidden Rev column, a `pkg/poll` refresh underneath — showing all three sources
at once. Its fake backend is mutex-guarded and called from the action
goroutine, which is not scaffolding: it is the discipline a real client needs,
and the reason an action is handed a context and a writer rather than the
model. Refresh is deliberately instant server-side so decision 19's wait is
visible; Sync is slow enough that the handoff to derived state happens
mid-flight.

**5. Should `Derive` be able to say "failed" rather than just "not busy"?**
Today the predicate is binary, so a derived run ending badly is communicated
entirely by the cell reverting to `failed` in the app's own styling. A
three-valued phase (resting / busy / failed) would let the library tint the
row for a moment on the way past. It is more API for something the data
already renders, and it needs the author to classify every resting value
rather than just the busy ones. Leaning no; revisit if a real screen reads
flat.

**4. Is `Hold` right at 2s, and should a failure hold longer?** A `✗` that
disappears while the user is reading the console is a lost outcome, and the
statusbar receipt is gone by then too. An asymmetric default (2s ok, 8s failed,
or failures holding until any keypress) is defensible and slightly surprising.
Wants a real screen to judge against.
