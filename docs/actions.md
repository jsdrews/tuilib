# Actions and reserved keys — design

Status: **implemented**, except `pkg/keys` and rule 31, which are independent
and still proposed. `pkg/action`, `geom.AnchorIn`, `mouse.IsRightPress` /
`IsPointPress`, `theme.Actions()`, marking in `pkg/list` and `pkg/table`,
`runner.Go` / `runner` correlation tags, the app shell wiring
(`Options.ActionsKey`, the overlay routing mode, right-click, the statusbar
receipt), `examples/data/actions` and `examples/data/multiselect` are all
in. Marking is demonstrated in `examples/data/multiselect` and summarised
as CLAUDE.md rule 32 — rules 30 and 31 below are still unwritten, so the
marking rule took the next free number rather than one they had
reserved. `examples/data/actions` stays single-target on purpose.
Decisions 6, 7, 15 and 19
record where the built thing corrected the design.

## Problem

Three complaints, one shape underneath.

**1. Feedback is inconsistent, because only two kinds of work have a channel.**
`pkg/output` solved this for the two things that already flowed through the
shell: `app.Info`/`app.Error` and `runner.Capture`. Everything else — an HTTP
POST, a k8s API call, a file write, anything that is *Go work* rather than a
subprocess — has no channel at all. The author picks between `app.Info` (one
line, wiped by the next keypress), an `alert` modal (breaks flow for a success
notice), or nothing. Nothing wins, because nothing is the least work. So the
console is excellent at reporting the two things it was wired to and silent
about the verbs an app actually exists to perform.

**2. Nothing pushes an author onto the async path.** `Update` has a value
receiver and returns a `tea.Cmd`; the correct shape is right there. But
writing the blocking version is *fewer characters* — call the API, get the
result, set the field — and it freezes the TUI for the duration with no
warning at authoring time. The library offers `runner.Capture` for
subprocesses and, for everything else, a recipe.

**3. The footer is full, and rule 8 has no mechanism.** Rule 8 already says
per-screen verbs should be menu-driven rather than letter shortcuts. But the
only menu the library ships is "build a `list.Model` and push it as a screen,"
which costs a breadcrumb crumb and a round trip through `OnEnter` — so authors
bind letters instead. Then they collide, because nothing in the library states
which letters are already spoken for. Rule 25 reserves the scroll vocabulary
in prose; the rest is folklore recoverable only by grepping every
`DefaultKeys()` in the tree.

The three are the same problem seen from different sides: **the library has no
concept of an action.** It has components that display things and a shell that
routes keys, and the verb in between is left as an exercise.

## Shape

An `Action` is a named verb over the current selection. A screen declares the
ones that apply right now; the shell owns the key, the right-click, the menu
overlay, the goroutine, the cancellation, and the logging. The author writes a
label and a function.

```go
func (s *Screen) Actions() action.Set {
    // Selection is the marked set, or the cursor row when nothing is marked.
    targets := s.table.Selection()
    if len(targets) == 0 {
        return action.Set{}
    }
    return action.Set{
        Target: s.table.SelectionLabel(),   // "cache-redis" or "3 items"
        Actions: []action.Action{
            {
                Label:     "Restart",
                Multi:     true,   // three marked pods, three restarts
                Exclusive: true,   // but not twice on the same one
                Confirm:   fmt.Sprintf("Restart %s?", s.table.SelectionLabel()),
                Run: func(ctx context.Context, out io.Writer) error {
                    for _, name := range targets {
                        fmt.Fprintf(out, "restarting %s\n", name)
                        if err := s.api.Restart(ctx, name); err != nil {
                            return err
                        }
                    }
                    return nil
                },
            },
            {
                // No Multi: there is one screen to push, so the menu disables
                // this by itself the moment a second row is marked.
                Label: "Logs",
                Do:    func() tea.Cmd { return screen.Push(newLogScreen(targets[0])) },
            },
            {
                Label:    "Delete",
                Multi:    true,
                Disabled: guard(targets),
                Confirm:  fmt.Sprintf("Delete %s? This cannot be undone.", s.table.SelectionLabel()),
                Run:      func(ctx context.Context, out io.Writer) error { … },
            },
        },
    }
}
```

Mark rows with `space`, then press the actions key or right-click: a bordered
menu opens anchored where you asked for it, titled `Actions · 3 items`. Enter
picks one; the menu closes immediately; `Restart` runs in a goroutine with a
cancellable context; every line it writes lands in the console attributed to
`Restart`, grouped as one event, killable with `x` like any capture; the
outcome paints the statusbar. The TUI never blocked.

Plus `pkg/keys`: a registry of what the library has claimed, so "which letters
may I bind" has an answer, and a test that keeps the answer true.

---

## Decisions

### 1. The screen declares actions; components do not.

An action is a domain verb — "restart deployment", "cancel run", "promote". A
`list.Model` is a generic renderer of strings; it has no domain and cannot
acquire one without dragging the caller's types into `pkg/list`. Hanging an
action registry off `Options` would also mean a screen with three components
wires the same closures into three places and then reconciles which one is
focused anyway.

The screen already holds the answer to both questions the menu needs — which
component has focus (`focus.Group.Is`) and what is selected in it — so the
scoping the user wants ("actions for *this* component") is a switch the screen
writes once:

```go
func (s *Screen) Actions() action.Set {
    switch {
    case s.focus.Is(&s.deployments):
        return s.deploymentActions()
    case s.focus.Is(&s.events):
        return s.eventActions()
    }
    return action.Set{}
}
```

This also matches rule 7: parent → child data goes through construction, child
→ parent through `Pop`, and nothing reaches around the stack. An action
registry living on components would be the first thing in the library that
does.

**Rejected: components contribute built-in actions** (copy value, expand all,
open link). Tempting, and it would make the menu non-empty for free — but it
refills the menu with library verbs, which is the clutter problem relocated.
The one exception worth revisiting later is `pkg/table`'s hyperlink cells,
where "open in browser" is genuinely component knowledge (`ansi.ExtractHyperlink`
already exists for it). Deferred, not refused.

### 2. An optional interface, not a change to `screen.Screen`.

```go
// in pkg/action
type Provider interface {
    Actions() Set
}

// Set is a screen's actions plus what they will act on.
type Set struct {
    // Target names the object of the verbs — "cache-redis", "3 items".
    // It titles the menu and it is the whole reason Set is a struct rather
    // than a bare slice: once marking exists (decision 19), "Delete" can
    // mean one row or twelve, and the menu is the last surface that can say
    // which before it happens.
    Target string

    // Count is how many targets Actions will act on. 0 and 1 both mean a
    // single target; above that, actions without Multi are disabled (21).
    Count int

    Actions []Action
}
```

The shell type-asserts the active screen. Every existing screen keeps
compiling and simply has no actions, which is the honest state of affairs for
a screen that hasn't declared any. `focus.Capturer` is the precedent: an
optional interface answering a shell question that most implementations don't
need.

`Actions()` is called on menu open, on key dispatch, and on every `apply()`
that needs to know whether to advertise the key — i.e. on roughly the same
cadence as `Help()`, which screens already rebuild per message. Same contract:
**cheap, allocation-light, no I/O.** Anything expensive belongs behind a field
the screen already maintains.

### 3. The menu is an overlay the shell composites, not a pushed screen.

`pkg/output` chose a pushed screen and named the cost: a breadcrumb crumb for
a place the user didn't navigate to. For a console that's a fair trade — you
go there and read for a while. For a context menu it is the wrong trade three
times over:

- The row you are acting on scrolls out of view at the moment you choose the
  verb that acts on it.
- Popping fires `OnEnter` on the screen underneath, so the menu would need its
  own sentinel and every fetching screen would need a second guard branch next
  to the `OutputClosed` one. One sentinel per transient surface does not scale.
- A crumb reading `Deployments › Actions` describes a location. The menu is not
  a location.

So the shell draws it as `ZStack(body, at(x, y, …))` inside its own `View`, and
`app.Update` grows a routing mode: while the menu is up it takes every
`tea.KeyMsg` and the active screen sees none.

**That new routing mode is the real cost of this decision**, and it is the
thing the output console was designed to avoid. It is accepted here because
the alternative costs more, and because the mode is small and total — the menu
owns the keyboard completely while it is up, exactly as `pkg/confirm` owns it
inside a host screen, so there is no partial-routing state to reason about.
Mouse still fans out to everything (rule 6): a click outside the menu closes
it, and the component under the pointer is entitled to see that click.

### 4. The menu sizes and places itself; no new layout primitive.

An earlier draft proposed `layout.At(x, y, w, h, child)` as a positioned
counterpart to `Center`. It isn't needed. `pkg/alert`'s autosize mode already
does exactly what a context menu needs, without a new node type:

- `SetRect` treats the rect it is handed as **outer bounds** rather than as
  the component's own box.
- The component measures its content and computes its natural size against
  caps derived from those bounds.
- It places itself with `geom.CenterIn(r, w, h)` and **retains the placed
  rect**, so it hit-tests where it actually landed rather than against the
  bounds it was given.
- `View` renders the box padded to the outer bounds, so the host composes with
  a bare `layout.Sized(&m)` and no `layout.Center` wrapper.

The menu is that, with the anchor swapped from "centered" to "at this point,
clamped." That is one function in `pkg/geom`:

```go
func AnchorIn(outer Rect, x, y, w, h int) Rect
```

Placing a `w`×`h` box with its top-left at `(x, y)`, pushed back inside
`outer` when it would overflow — a menu opened on the bottom-right row flips
up and left instead of hanging off the edge. Eight lines in a leaf package
beats a new node type in `pkg/layout`, and it keeps rule 9's "a component owns
its geometry" intact: the menu decides where it lands and hit-tests where it
landed, exactly as `alert` already does.

### 5. Execution is `func(ctx context.Context, out io.Writer) error`.

```go
type Func func(ctx context.Context, out io.Writer) error
```

Three deliberate choices in one signature.

**`ctx` first**, so cancellation is not optional. The console already has `x`
for killing an in-flight capture; an action that cannot be stopped would be a
second class of in-flight work with a worse story than subprocesses have.

**`io.Writer` rather than a channel or a `log(string)` closure**, because it
composes with everything that already writes. `fmt.Fprintf` works. `io.Copy(out,
resp.Body)` works. An action that shells out can point `cmd.Stdout` straight at
it. A logger can wrap it. A closure of our own invention composes with nothing
and would be re-wrapped at every call site. The writer splits on newlines
internally and pushes one record per line into the same bounded channel
`runner.Capture` uses, so backpressure behaves identically — a producer that
outruns the UI blocks rather than growing memory.

**`error` return** drives the finish level. No status enum, no completion
struct: an action either worked or it didn't, and the reason is an error.
`ctx.Err()` on cancellation reads as a failure, which is correct — the user
stopped it.

**Rejected: `Func` returning a result value** for the screen to consume. Two
channels back (log + typed result) doubles the protocol, and the case is
already served: an action that needs to hand data back returns a `tea.Cmd` via
`Do` and posts its own message, exactly as a fetch does today.

### 6. Background actions reuse the capture message family.

`pkg/runner` grows one function:

```go
func Go(label string, fn func(ctx context.Context, out io.Writer) error) tea.Cmd
```

emitting the *existing* `CaptureStarted` → `CapturedLine`… → `Captured`
sequence, over the existing `stream`, with the existing bounded channel and
`Next`-driven read.

The payoff is that **`pkg/app` needs no new logging code whatsoever.** It
already appends a head record on `CaptureStarted`, registers the run so the
badge shows `⟳` and `x` can kill it, appends a continuation per line, and
posts a levelled completion on `Captured`. Point a Go func at that pipeline
and requirement 1 — every action reports through the console, consistently —
is satisfied by construction rather than by convention. Kill becomes
`cancel()` instead of `killProcess(cmd)`, which the run registry already
models as an opaque `kill func() error` for precisely this reason.

`pkg/runner` stays dependency-free: `context` and `io` are stdlib, and the
messages stay neutral. Its package doc widens from "runs an interactive
subprocess" to "runs work whose output is captured rather than given the
terminal" — which is what `Capture` already meant.

One small addition is needed: `CaptureStarted` gains a `Detail string` for the
head line. `Capture` fills it with `$ cmd args` (what `commandLine` builds
today); `Go` fills it with the label. Without it the head for Go work renders
`(no command)`, which is the shell's current fallback for a nil `*exec.Cmd`.

**Two things the build added.** `GoOptions.Detail` turned out to be needed
twice over, not once: a Go run has no `Cmd` to derive a head line from, *and*
the log's source column is ten cells wide, so an action label carrying its
target ("Restart · 2 items") truncates to `Restart ·` there. Splitting them —
short `Label` for the source column, full `Detail` for the head line — is what
makes the log read properly, and it is not something the design anticipated.

The second is a `recover` around fn. A panic in background work would
otherwise kill the program *and* leave the run in flight forever, with the
badge showing `⟳` for something that can never finish; it is reported as the
failure it is instead.

**Naming wart, named:** a `Captured` message for a Go function reads oddly.
The alternative is renaming the family to `runner.Started/Line/Done` with
aliases, which churns a shipped API and every screen matching on it. The
reading that "capture" describes *where the output goes* — captured, rather
than handed to the terminal — holds for both producers, so the names stay.

### 7. The shell logs the invocation — for `Do` actions only.

**Corrected during implementation.** The decision originally said "whatever
the action does", and the built version did exactly that for one round of
testing, which is how the mistake surfaced: the badge read `2 output` for a
single Restart.

A `Run` action already opens its own event — `runner.Go` emits a head record
carrying the action's `Detail` — so a separate invocation line makes every
action report twice in a badge whose whole premise is *events, not lines*
(rule 17). The original text even said the invocation line "is the head of the
run's event", which is true; it just isn't a *second* record.

So a `Run` action gets one head and its output beneath it:

```
14:23:01 INF Restart › Restart · 2 items
14:23:01 INF Restart │ restarting api-server
14:23:02 INF Restart │ api-server rolled out
```

A `Do` action gets the invocation line, because it is the only line the shell
can promise — the returned `tea.Cmd` might push a screen or hand the terminal
to `$EDITOR`, and the library cannot narrate what it does not own.

Attribution still works as decision 9 wanted: records from a background action
are sourced to the action rather than to whatever screen the user wandered to
by the time it finished.

This is what makes "all actions communicate through the console" true rather
than aspirational. It also fixes attribution for the `Run` path: decision 9 of
the output-log design accepted that `app.Info` cannot know who created it,
because a `tea.Cmd` cannot introspect its creator. An action *can* — the shell
launched it and knows its label — so every record from a background action is
correctly sourced even if the user has navigated three screens away by the
time it fails.

### 8. The outcome paints the statusbar; the detail stays in the log.

On `Captured` for an action-launched run, the shell already appends a levelled
completion record. It additionally sets the statusbar message — `Restart
completed` / `Restart failed: <err>` — which is `InfoDetail` semantics applied
automatically.

Every action produces exactly one statusbar flash and one console event. No
`Quiet` flag: an action that shouldn't tell the user it happened is not an
action, it's a keystroke, and rule 20 already covers keystrokes.

**Scoped to actions, not to every capture.** The worry when this was written
was that painting the bar on `runner.Captured` would change behaviour for every
existing `runner.Capture` caller — a shipped feature with callers of its own.
It doesn't have to: an action the shell launched carries a correlation `Tag`,
and the receipt is scoped to runs that have one. A bare `runner.Capture`
behaves exactly as it always did.

That tag is the one addition `pkg/runner` needed for the shell. Neither `Label`
nor `RunID` could do the job — labels collide across targets, and the run ID is
minted inside the command, after the caller has returned — so a caller
correlating a finished run back to what it launched it *for* had no handle at
all. The shell uses it to release the right `Exclusive` gate as well.

### 9. Right-click focuses, moves the cursor, then opens the menu — in that order.

`mouse.Msg` gains `IsRightPress()`. Components treat a right press exactly as
they treat a left single press — take focus via `focus.RequestSelf`, move the
cursor to the row under the pointer — and never as an activation. The shell
forwards the event to the stack **first**, then opens the menu, so `Actions()`
is asked after the cursor has moved and reports on the row the user pointed
at, not the one that happened to be selected before.

No new bubbling mechanism is needed: the shell already forwards then acts, and
components already decline events outside their rect.

The `Tracker` is untouched — it counts left presses only, and a right
double-click means nothing in this library.

**Known limitation, and the reason the key is primary:** several terminals
never deliver a right press to the application. macOS Terminal.app shows its
own context menu; iTerm2 and others forward it once mouse reporting is on, but
this is emulator policy, not something the library can fix. Mouse is opt-in
already (`app.Options.Mouse`), so right-click is an accelerator on top of an
accelerator. The keyboard path must be complete on its own, and the menu is
fully navigable with the list vocabulary users already know.

### 10. Opt-in via a single `ActionsKey`, conventionally `a`.

```go
ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
```

Following `OutputKey` exactly, and for the reason spelled out there: a letter
the shell claims is a letter taken from every component in every downstream
app, permanently. That should be a line an app author writes on purpose. `a`
is the recommended value and is otherwise unclaimed (see the registry below);
the library does not default to it.

The shell advertises the key itself rather than leaving it to screens — same
exception as `OutputKey`, same reasoning (rule 14): the key exists only in
apps that opted in, so screen authors have no reason to know about it, and
left to them it would be advertised on the one screen whose author remembered.

**It is advertised only when the active screen actually has actions, and is
inert otherwise.** The precedent is `?`: `helpOverflow` gates both the
affordance and whether the key does anything, so the hint is the source of
truth. A key that opens an empty box teaches users the feature is broken.

### 11. An action may carry a key; `Help()` must not list them.

```go
{Label: "Restart", Key: key.NewBinding(key.WithKeys("r"), …), Run: …}
```

The shell dispatches it when the screen is not capturing keys, so declaring an
action gets you both the menu entry and the shortcut from one declaration —
the screen's `Update` grows no branch per verb.

The menu renders the key in a right-hand column, and that is the *only* place
it is advertised. This is the whole answer to the footer-clutter complaint:
discovery moves from a strip that must fit one row into a surface built to
hold a list. A screen with nine verbs contributes one binding to `Help()`, not
nine.

**Action keys are checked against the registry and dropped if reserved.** The
shell dispatches action keys before forwarding to the screen, so an action
bound to `j` would silently kill scrolling on that screen. Rather than let
that ship, a reserved binding is ignored for dispatch and the menu shows the
action with no shortcut. `action.Validate([]Action) []error` makes it a test
failure instead of a mystery — the repo's own lesson (see the anti-patterns on
`internal/componenttest`) is that documentation does not prevent this and a
test does.

### 12. Disabled actions are shown with a reason, not hidden.

```go
{Label: "Delete", Disabled: "pod is already terminating"}
```

Rendered dimmed, unselectable, reason in the row. Hiding it is worse: the user
learns the verb doesn't exist rather than that it doesn't apply *right now*,
and they go looking for it in the docs. The screen is still free to omit an
action entirely when it is meaningless rather than merely unavailable.

### 13. `Confirm` for destructive verbs, using `pkg/confirm`.

A non-empty `Confirm` string puts the existing modal between the pick and the
run. It lives in the shell's overlay slot, replacing the menu, so the routing
mode already built in decision 3 covers it with no second mode. Without this
every app hand-rolls the same two-modal sequence, which the anti-patterns
section already forbids for good reason.

### 14. `Run` and `Do` are two fields, and exactly one may be set.

```go
type Action struct {
    Label, Desc string
    Key         key.Binding
    Confirm     string

    // Disabled, when non-empty, renders the row dimmed and unselectable with
    // this as the reason. The menu also sets it automatically — for a
    // non-Multi action under a multi-selection (21), and for an Exclusive
    // action already in flight (16).
    Disabled string

    // Multi accepts a selection of more than one. Defaults to one (21).
    Multi bool

    // Exclusive refuses a second concurrent run for the same target (16).
    Exclusive bool
    ID        string // scopes Exclusive; defaults to Label

    // Run is background work. Prefer it: the shell runs it under a
    // cancellable context, streams what it writes into the console under
    // Label as one event, and reports the outcome. Concurrent by default.
    Run Func

    // Do is the escape hatch for verbs that are not background work —
    // pushing a child screen, handing the terminal to $EDITOR via
    // runner.Run, opening a form.
    //
    // Navigational Do actions are one-at-a-time by construction rather than
    // by declaration: pushing a screen replaces what is on top, so there is
    // no second one to push and nothing for Exclusive to prevent. It is a Do
    // that *doesn't* navigate — one that fires a request and returns — that
    // may still want Exclusive.
    Do func() tea.Cmd
}
```

A single `Do func() tea.Cmd` with an `action.Background(…)` helper was the
tidier-looking option, but it hides the difference that matters: `Run` gets
attribution, grouping, cancellation and automatic reporting, and `Do` gets
none of those because it cannot. Two fields put that choice in front of the
author at the moment they make it. `Validate` enforces the "exactly one".

Note that `Run`/`Do` and `Multi`/`Exclusive` are **orthogonal**, and it is
worth resisting the urge to collapse them. The temptation is to infer arity
and exclusivity from which function field is set — `Do` pushes a screen, so
surely it's single and exclusive? But a `Do` that opens a browser tab per
target is legitimately `Multi`, and a `Run` holding a cluster-wide lock is
legitimately `Exclusive`. Inferring either would be right most of the time,
which is the worst frequency for a wrong default: often enough to be relied
on, rare enough that the exception ships broken.

### 15. `theme.Actions()` — rule 3 holds here after all.

An earlier draft of this decision said the opposite: that `pkg/action` would
need `OptionsFrom(t)` because `theme` could not import it, the way
`pkg/output` cannot be imported by `theme`. That was pattern-matching, not
checking. **There is no cycle.**

`pkg/output`'s problem is specific: `output.Screen` implements
`screen.Screen`, `pkg/screen` imports `pkg/theme` for `SetTheme`, so
`theme` → `output` closes the loop. `action.Menu` is a plain component, not a
screen — it imports `pane`, `geom` and `mouse`, none of which import `theme` —
so `theme` importing `action` closes nothing.

So the `th.Component()` convention of rule 3 applies unchanged, and
`pkg/output` remains the *only* place it bends. Worth recording because the
wrong version of this decision was written down confidently, and the check
that settled it took one `grep` of `pkg/pane`'s imports.

The one visible consequence is in the tests: `pkg/action`'s own test files
cannot import `pkg/theme` (that *would* be a cycle, in the test binary), so
they build `Options` locally. Which is the right dependency for a component
test to have anyway.

### 16. Concurrency is the default; `Exclusive` opts out, and the menu shows why.

Most background actions are safely concurrent: each is a run with its own ID,
its own context, and its own line in the kill picker, and the badge already
counts them. Firing three restarts and watching them interleave in the console
is a feature.

But some verbs genuinely cannot overlap — a deploy, a migration, anything that
takes a lock — and "the action guards itself" is the wrong answer for them,
because the guard has to live in a closure that has no way to tell the user it
declined. So:

```go
// Exclusive refuses a second run while one is in flight for the same
// target. The menu renders it disabled with "already running" rather than
// silently dropping the press.
Exclusive bool

// ID scopes that check. Defaults to Label.
ID string
```

The identity is `ID` + `Set.Target`, which gives per-target exclusivity for
free: restarting `web` while `api` restarts is fine, restarting `web` twice is
not. The shell can answer this without new bookkeeping — `output.Buffer`
already holds the in-flight run registry that the badge's `⟳` and the `x` kill
picker read from.

Rendering it as a *disabled row with a reason* rather than a rejected keypress
is the whole point. This supersedes an earlier draft of this decision, which
said the library would not dedupe and left it to the author; that was written
before decision 12 established that "visible but unavailable, with a reason"
is a thing the menu can express. Once the menu can say "already running,"
declining to say it is just a worse product.

What the second press does is therefore *nothing*, visibly — not queue, not
cancel-and-restart. Those are policies a verb might want, and an author who
needs one writes it inside `Run`, where the domain knowledge is.

### 17. The menu is a dedicated component, not a configured `list.Model`.

The reflex is to reach for `pkg/list` — it already has a pane, a cursor, a
filter, and the nav vocabulary. It is the wrong shape, for one reason that
then produces several: **a list item is a `string`, and a menu row has
structure.** `refresh()` builds `"▸ " + it` and hands the pane one string, so
label / key-hint / disabled-reason have to be flattened into it before the
list ever sees them. That breaks twice:

- **Right-aligning the key column** needs the pane's inner width *before* the
  items exist — circular with decision 4, where the width is measured *from*
  the items.
- **Dimming a disabled row** needs ANSI inside the item, but the cursor row
  gets `selectedStyle.Render(...)` wrapped around the whole string, and a
  nested reset closes the outer style. Rule 19's hazard, relocated from
  `pkg/table` to `pkg/list`.

And a disabled row must be **visible but unselectable**, which `list` has no
concept of — its cursor walks `visible` with no skip. Teaching it one puts a
field on every list in the library to serve a single caller.

So `action.Menu` owns a `pane.Pane` (rule 9) and renders its own rows, its own
cursor, and its own anchor. What it gives up is the embedded `filter.Model`,
which it re-adds when a menu grows long enough to want `/` — about thirty
lines, and the same thing `list` itself does.

**This does not block the hosting decision.** The component emits
`ChosenMsg` / `CancelledMsg` exactly as `pkg/confirm` does, so it can be
built, tested and demoed against a host screen (as `examples/data/confirm`
already does) before anything in `pkg/app` changes.

### 9b. A right press points; only a left press acts.

Decision 9 said components should treat a right press "exactly as they treat a
left single press". Implementing it showed that is too broad — the press
branch of several components does more than point:

- `pkg/tree` and `pkg/inspector` toggle the `▸`/`▾` glyph on a single click.
- `pkg/table` sorts when the press lands on a column header.
- `pkg/list` and `pkg/table` toggle a mark when it lands on the `✓` gutter.

All of those are *acting*, and a right-click that expands a branch or re-sorts
a table on its way to opening a menu is a surprise the user did not ask for.
So the rule sharpened into two verbs: `mouse.IsPointPress` (either button)
means "focus this and put the cursor here", and `IsPress` (left only) is what
any chrome that *does* something checks. Activation was already safe, since
`IsDoubleClick` implies `IsPress`.

`pkg/toggle`, `pkg/confirm` and `pkg/alert` were left untouched for the same
reason: every press they handle is an action, so a right-click on them
correctly does nothing at all.

### 18b. A right-press outside an open menu retargets it.

The menu is opened by right-clicking a row. Right-clicking a *different* row
while it is open is the same gesture aimed somewhere else — "not that one,
this one" — and charging two gestures for it (dismiss, then click again) is
the kind of friction that makes a feature feel unfinished.

The menu cannot do the retarget itself: it has no view of what is under the
pointer in the layer beneath. But it is the only thing that knows whether the
press landed on it (rule 28 — nothing outside a component decides a click was
its own), so it declines the event and reports where it went as
`RetargetMsg{Event}`, carrying the press untouched. The host routes that to
whatever is underneath and reopens against the result — the same path a fresh
right-click takes, which is what keeps the two entry points from drifting.

Three properties fall out and are worth stating, because each is a way this
could have gone subtly wrong:

- A **left** press outside still dismisses. Retargeting and dismissing are
  different gestures and must not collapse into one.
- A right press **inside** the menu does nothing. There is no second level of
  menu to ask about.
- A host that ignores `RetargetMsg` is left exactly where it was — the menu
  does not close itself, because it does not know whether the host will find
  anything to reopen against. That is what made this safe to add after the
  component had shipped.

### 18. A single click commits a menu row.

Rule 28 says single click focuses and moves the cursor, double click
activates. A menu row is exempt, and the exemption is already the library's
rule rather than a new one: `confirm` and `alert` commit their buttons on
`IsPress()`, and `pkg/tab` switches tabs on a single press, under rule 28's
own "library-drawn chrome is clickable" clause. **A menu row is a button, not
a data row.** Worth stating explicitly, or it reads as a bug and someone
"fixes" it.

### 19. Marking lives in `list` and `table`, keyed by `KeyedItem`/`KeyedRow`.

Acting on one row at a time is the case the menu obviously serves; acting on
several is the case that makes it worth building. Marking belongs in the data
component the user is already looking at — not in the menu, and not in a
separate picker modal — because the rows the user wants are the rows in front
of them, and a modal that re-lists them is a second copy to keep in sync.

**Marks are keys, not indices,** and that is not a detail. Rule 24's whole
point is that a polled refresh reorders and partially replaces the row set;
index-based marks would drift onto unrelated rows between the mark and the
verb, which on `Delete` is the worst bug this library could ship. The
substrate already exists — `KeyedItem` / `KeyedRow` were added so the *cursor*
could survive the same swap — so marking is that primitive with a
`map[string]bool` in place of one index. Marking is therefore **available only
on keyed data**; on anonymous `SetItems([]string)` the mark keys are no-ops
and `Marks()` is empty.

- **Opt-in per component** via `Options.Markable`, matching `Filterable` and
  `HScrollbar`. Off, there is no marker column, no binding, and no width cost.
- **`space` toggles the cursor row.** It is unbound in both `list` and
  `table`, and already means "toggle" in `tree`, `inspector` and `toggle` — so
  it extends the existing vocabulary instead of inventing one. `pkg/tree` is
  excluded for exactly that reason: `space` is already spent there on
  expand/collapse, and marking a tree raises a question this design does not
  answer (does marking a branch mark its children?).
- **`A` marks every currently visible row** — visible meaning post-filter,
  which makes "filter to `region:eu`, mark all, restart" a three-keystroke
  flow. On a windowed table (`SetWindow`) it marks the resident window only;
  the count stays honest because it is a count of what is actually marked.
- **Marks survive filtering**, since they are keys. That means you can mark a
  row, filter it out of sight, and still act on it — which is why decision 2's
  `Set.Target` is not cosmetic. The menu title is the disclosure.
- **The marker column is two cells in `list`** (cursor `▸` then mark `✓`) and
  **one in `table`**, which shows its cursor with a background rather than a
  glyph and so needs a column only for the `✓`.
- **A windowed table cannot be marked.** Found while building, not while
  designing: `SetWindow` carries `[]Row` with no keys, so a mark there could
  only be held by index in a sparse paged set — the exact drift this decision
  refuses. Marking is inert under `SetWindow` rather than approximate. Giving
  remote sources marks needs a keyed window setter, which is its own change.

### 19b. The marking tests live in `internal/componenttest`, not in either component.

`list` and `table` implement marking separately — their models differ too much
to share code, and the repo's grain is parallel implementations per component.
That is precisely the shape the "Don't test shared behaviour in one
component's package" anti-pattern was written about: click-to-blur was written
for `pkg/list`, rolled out to five others by a script that omitted it, and
tested only where it was written, so five components shipped broken with a
green suite.

So the contract — space toggles, marks are keyed and survive both a swap and a
filter, mark-all respects the filter, anonymous data is inert, the gutter
click toggles without opening, `Selection` falls back to the cursor — is
asserted once against both components through a small adapter interface. When
`tree` eventually grows marking, it joins the table rather than getting its
own copy of the assertions.

### 20. `Selection()` collapses "the marked set, or the cursor".

```go
func (m Model) Selection() []string      // marks if any, else the cursor's key
func (m Model) SelectionLabel() string   // "cache-redis" or "3 items"
```

Without this, every screen writes `if marks := t.Marks(); len(marks) > 0 { … }
else { … }` — a branch that is easy to write once and easy to forget, and
whose failure mode is a verb silently acting on one row when the user marked
six. One accessor makes the single-selection case the zero-mark case of the
multi-selection case, so screens have one path.

`SelectionLabel` exists so the count reaches the confirm string and the menu
title without every app re-inventing the pluralization.

### 21. Arity is declared, and defaults to one.

Marking creates a question every action now has to answer: **does this verb
accept more than one target?** "Restart" does — three pods, three restarts.
"View logs" does not, because it pushes a screen and there is only one screen
to push. Nothing about `Run` vs `Do` settles it: a `Do` that opens a browser
tab per target is perfectly sensible, and a `Run` that acquires a single
cluster-wide lock is not.

```go
// Multi reports whether this action accepts a selection of more than one.
// The zero value is false: an action acts on exactly one target unless it
// says otherwise.
Multi bool
```

With more than one target marked, the menu renders every non-`Multi` action
disabled with a stock reason ("acts on one item at a time"). No screen writes
the guard.

**The default is the safe direction, and that is the whole reason it is a
field rather than an inference.** If the default were "accepts many," an author
who forgot would ship a logs action that, on a three-row selection, either
pushes three screens or silently picks one arbitrarily — a bug you find in
production. With the default at one, forgetting produces a *disabled row with
an explanation*, which is a bug you find in the first five seconds of using
your own screen. Same reasoning as `confirm.Options.Initial` defaulting to the
cancel side and `app.Options.Mouse` defaulting off: when omission has to mean
something, it should mean the harmless thing.

This replaces the hand-written `Disabled: onlyOne(targets)` guard that an
earlier draft of this document's example needed, which is the usual sign that
a field was missing.

---

## Reserved keys

The second half of the request, and the part that pays off immediately even
if none of the above ships.

### The registry

A new leaf package, `pkg/keys`, depending on nothing in tuilib:

```go
type Scope int
const (
    Global    Scope = iota // the app shell owns it — never bind, anywhere
    Universal              // rule 25's scroll + search vocabulary — no component may bind it
    Component              // a specific component owns it while focused
)

type Reservation struct {
    Key   string  // "ctrl+u", "g", "/"
    Scope Scope
    Owner string  // "pkg/app", "rule 25", "pkg/table"
    Verb  string  // "quit", "half-page down", "sort direction"
}

func Reserved() []Reservation
func Lookup(k string) (Reservation, bool)
func Check(bindings ...key.Binding) []Conflict
func Free() []string   // unclaimed single characters, for authors picking one
```

`Check` is what an app author calls in their own test. `Reserved()` is what
generates the table below, so the doc cannot drift from the code.

### What is claimed today

**Global — the shell owns these; nothing else may bind them.**

| Key | Owner | Verb |
|---|---|---|
| `q`, `ctrl+c` | `pkg/app` | quit (at stack depth 1) |
| `esc` | `pkg/app` | pop the stack |
| `?` | `pkg/app` | expanded help panel |
| `ctrl+z` | `pkg/app` | suspend |
| `t` | `pkg/app` | cycle theme (opt-in, conventional) |
| `o` | `pkg/app` | output console (opt-in, conventional) |
| `a` | `pkg/app` | actions menu (opt-in, conventional) |

**Universal — rule 25's vocabulary. No component and no screen may rebind these.**

| Keys | Verb |
|---|---|
| `↑` `k` / `↓` `j` | line up / down |
| `←` `h` / `→` `l` | scroll left / right |
| `g` / `G` | top / bottom |
| `ctrl+u` / `ctrl+d` | half-page up / down |
| `pgup` / `pgdown` | page up / down |
| `0` `home` / `$` `end` | line start / end |
| `/` | search or filter |
| `\` | filter mode |
| `n` / `N` | next / previous match |
| `tab` / `shift+tab` | focus cycling (`pkg/focus`) |

**Component-claimed — owned by a specific component while it has focus.** An
app-level binding that collides here is a latent bug that surfaces only when
that component happens to be focused, so treat them as spoken for.

| Keys | Owner | Verb |
|---|---|---|
| `[` `]` `s` | `pkg/table` | sort column −/+, direction |
| `shift+←` `shift+→` | `pkg/table` | previous / next column edge |
| `shift+←` `shift+→`, `1`–`9` | `pkg/tab` | switch tab |
| `space` | `pkg/tree`, `pkg/inspector`, `pkg/toggle` | toggle (expand/collapse in tree — never marking) |
| `x` | `pkg/list`, `pkg/table`, `pkg/tree` | mark row (when `Markable`) |
| `space` | `pkg/list`, `pkg/table` | mark row (when `Markable`) |
| `A` | `pkg/list`, `pkg/table`, `pkg/tree` | mark all visible (when `Markable`) |
| `X` | `pkg/list`, `pkg/table`, `pkg/tree` | mark anchor↔cursor, either direction (when `Markable`) |
| `D` | `pkg/list`, `pkg/table`, `pkg/tree` | clear marks (when `Markable`) |
| `E` `C` | `pkg/tree`, `pkg/inspector` | expand / collapse all |
| `{` `}` `J` `K` | `pkg/tree`, `pkg/inspector` | sibling / leaf navigation |
| `w` | `pkg/textview` | toggle wrap |
| `c` `x` `w` | `pkg/output` | clear, kill run, export |
| `y` `n` | `pkg/confirm` | yes / no |
| `enter` | everywhere | activate the focused selection (rule 16) |

### What is free

After `a` goes to the actions menu, the unclaimed lowercase letters are:

> **b  d  e  f  i  m  p  r  u  v  z**

Most uppercase letters are free (`B F H I L M O P Q R S T U V W Y Z`),
as is most punctuation and most `ctrl+` combinations outside
`A`/`c`/`d`/`u`/`z`.

The point of publishing this is not to encourage spending it. It is that an
author who needs a key can now find one in five seconds instead of grepping,
and that the answer to "may I bind `s`?" stops being "probably, unless there's
a table on screen."

### Enforcement

Documentation did not prevent the last five components from shipping a bug
that was written down as a rule; a test in `internal/componenttest` did. Same
shape here:

1. A test walks every component's `DefaultKeys()` and asserts no binding
   claims a `Global` key or rebinds a `Universal` one to a different verb.
2. `keys.Check` is exported so an app's own test asserts the same about its
   `Options` overrides and its `Action.Key` values.
3. `action.Validate` folds `keys.Check` in, so a screen's action set is
   checkable in one call.

---

## What lands where

| Package | Change |
|---|---|
| `pkg/action` | **new.** `Action`, `Set`, `Func`, `Provider`, `Menu` (self-sizing anchored overlay component), `OptionsFrom`, `Validate`. |
| `pkg/keys` | **new.** Registry, scopes, `Check`, `Free`. Leaf, no tuilib imports. |
| `pkg/geom` | `AnchorIn(outer, x, y, w, h)`. |
| `pkg/list`, `pkg/table` | `Options.Markable`; `space` / `A`; marker column; `Marks`/`SetMarks`/`ClearMarks`/`Selection`/`SelectionLabel`. |
| `pkg/runner` | `Go(label, fn)`; `CaptureStarted.Detail`; package doc widened. Still dependency-free. |
| `pkg/mouse` | `Msg.IsRightPress()`. |
| `pkg/app` | `Options.ActionsKey`; overlay slot + routing mode; right-press handling; invocation logging; outcome → statusbar; hint gating. |
| components | right press treated as focus + cursor move (`list`, `table`, `tree`, `inspector`, `logview`, `textview`, `form`, `input`, `toggle`). |
| `internal/componenttest` | reserved-key conformance; right-press-focuses conformance; marks-survive-a-keyed-swap conformance across `list` and `table`. |
| `examples/app/actions` | **new.** Marking, menu from key and right-click, a background action over a marked set, a `Confirm` action, a `Disabled` action, a `Do` action that pushes a screen. |
| `CLAUDE.md` | rules 30, 31, 32 (drafted below). |

**Build order.** `action.Menu` first, hosted by a screen in an example the way
`pkg/confirm` is — it is the piece with the most design risk and the least
coupling, and per decision 17 it does not depend on the shell wiring. Marking
second, since it is independent of the menu and useful on its own. The shell
wiring (`ActionsKey`, overlay routing, right-click) last, once the component
has been used enough to know it is right. `pkg/keys` can land at any point and
is worth doing early — it is the piece that pays off even if the rest stalls.

### Proposed rule 30 — Actions

> **Give a screen's verbs to `pkg/action`, not to the footer.** A screen
> declares what can be done to the current selection by implementing
> `Actions() action.Set`; the shell owns the key, the right-click, the menu,
> the goroutine and the reporting. Prefer `Action.Run` — background work under
> a cancellable context, streamed into the output console under the action's
> label as one event, with the outcome on the statusbar — and reach for
> `Action.Do` only for verbs that are not background work (pushing a screen,
> handing the terminal to `$EDITOR`). Never do the work inline in `Update`: a
> value receiver that blocks freezes the whole TUI, and nothing about the
> signature warns you.
>
> Declare the two things the menu cannot infer. `Multi` means the verb accepts
> a selection of more than one — omit it and the menu disables the action the
> moment a second row is marked, which is the right way round: forgetting
> costs you a disabled row you notice immediately, not a "view logs" that
> picks one of three targets at random. `Exclusive` means it must not overlap
> a run of itself on the same target, and the menu says "already running"
> instead of dropping the press. Neither is inferable from `Run` vs `Do` — a
> `Do` opening one browser tab per target is `Multi`, a `Run` holding a lock
> is `Exclusive`.
>
> Set `Confirm` on destructive verbs rather than hand-rolling a modal, and
> `Disabled` (with a reason) rather than hiding an action the user is looking
> for. Scope actions by focus — a `switch` over `s.focus.Is(&…)` — so the menu
> describes the pane the user is pointing at, and set `Set.Target` from
> `SelectionLabel()` so a destructive verb states its blast radius before it
> runs.

### Proposed rule 31 — Reserved keys

> **Check `pkg/keys` before binding a letter.** The shell owns `q`, `esc`,
> `?`, `ctrl+z` and, where the app opts in, `t`, `o` and `a`; rule 25's scroll
> and search vocabulary is reserved library-wide; the rest belongs to whichever
> component is focused. `keys.Free()` lists what is actually unclaimed, and
> `keys.Check` turns a collision into a test failure instead of a bug that
> appears only when a table happens to be on screen. If you find yourself
> shopping for a letter, that is usually the signal the verb belongs in the
> actions menu (rule 30) rather than on a key.

### Proposed rule 32 — Marking

> **Multi-selection is keyed, or it is wrong.** Set `Options.Markable` on a
> `list` or `table` and feed it through `SetKeyedItems` / `SetKeyedRows` —
> never `SetItems` / `SetRows`. Marks are held by `Key` for the same reason
> the cursor is (rule 24): a polled refresh reorders and partially replaces
> the row set, and index-held marks drift onto unrelated rows between the
> mark and the verb. On anonymous items, marking is inert rather than
> approximate.
>
> Read the result with `Selection()`, not `Marks()` — it returns the marked
> set when there is one and the cursor's key when there isn't, so a screen has
> one path instead of a branch it can forget. `SelectionLabel()` is its
> display counterpart, for confirm strings and `Set.Target`.
>
> Marks are keys, so they survive filtering: a user can mark a row, filter it
> out of view, and still act on it. That is the correct behaviour and a
> genuine surprise, which is why the menu title always names the target.

---

### 21b. The keyboard centers the menu. A top anchor was tried and reverted.

Recorded because the reasoning that produced it was wrong in an instructive
way, not because the outcome is interesting.

The observation was that on a tall terminal a centered menu sits well below
the rows it acts on, and might therefore go unnoticed. That was a
misdiagnosis: it came from cropped terminal captures during verification, not
from anyone failing to find the menu. The report that triggered it was about
marking being invisible, and the reporter had plainly seen the menu — they
described it running a verb and closing.

Anchoring at the top did put the box nearer the rows, and made things worse:
it covered the host pane's filter row and its rule, so the frame read as
broken. Centering leaves the pane's chrome intact, and the middle of the pane
is where a modal is expected anyway.

The lesson worth keeping is about the evidence, not the placement — a fix was
designed, approved and shipped for a problem that only existed in the
measuring instrument.

### 21c. One subject per example.

`examples/data/actions` originally demonstrated the menu *and* marking. Both
worked; neither read. A reader met it as a menu demo, found a picker that runs
one verb and closes — correct behaviour — and never discovered that rows could
be marked first, because nothing on screen said so.

Marking is now out of that example, which demonstrates the menu, `Confirm`,
`Do` versus `Run`, and background reporting. Marking belongs in an example
whose subject it is, ideally a polling table where keyed marks visibly survive
a refresh — which would also cover `table` marking, currently demonstrated
nowhere.

### 22. The shell claims a modal's results only while it is the one showing it.

`pkg/confirm` reports through `ConfirmedMsg` / `CancelledMsg`, and the shell
now hosts a confirm of its own for `Action.Confirm`. Those messages are
neither keys nor mouse, so they do not travel through the overlay's input
routing — they arrive at the top of `Update` like any other message, and the
shell has to claim them there.

Claiming them unconditionally would break every screen that hosts its own
confirm modal (`examples/data/confirm`, `pkg/output`'s kill picker): the shell
would swallow the result and the screen would never dismiss. So both branches
are guarded on the shell actually having a dialog up. It is one `if`, and the
failure it prevents is silent — the modal simply stops responding.

This surfaced as a bug during implementation, not in review: the first version
handled the results inside the overlay's input router, where a message that is
neither a key nor a click never reaches.

## Open questions

1. **Does the actions key deserve a statusbar affordance** the way the output
   console got a badge? A badge counts unread events; an actions menu has no
   equivalent state to report, so probably not — but "this screen has verbs"
   is arguably worth a glyph. Currently proposed as help-hint only.

2. **Should `Do` actions get a completion line?** They can't be tracked (the
   returned `tea.Cmd` is opaque), so today they get the invocation line and
   nothing else. An action that pushes a screen needs nothing more; one that
   fires off a fetch looks like it vanished. Possible answer: `Do` actions
   that want reporting use `app.Info`/`ErrorOf` as they do today, and the
   invocation line is what ties them together.

3. **Right-click on a component that isn't the focused one** currently focuses
   it, which changes keyboard focus as a side effect of asking a question. That
   matches left-click (rule 28) and it is what makes `Actions()` report on the
   right pane, so it's proposed as-is — but it is the one place where "look at
   this" and "work here" are conflated.

4. **Does right-click on an unmarked row clear the marks?** Every desktop file
   manager says yes: right-clicking outside a selection replaces it. But marks
   here survive filtering and refreshes deliberately, and silently discarding
   six of them because the pointer landed on a seventh row is the kind of loss
   that has no undo. Proposed as-is (right-click moves the cursor, marks
   untouched, menu title discloses the real target), but it is the decision
   most likely to feel wrong in use.

5. **What kills a `Run` action from the menu rather than the console?** An
   `Exclusive` action already in flight renders as "already running", and the
   only way to stop it is `o` then `x`. A "Cancel" row in the menu, replacing
   the disabled one, is the obvious affordance — deferred until the base
   flow is real, since it is additive.

6. **A `Markable` component does not advertise that it is markable.** Nothing
   in the pane says rows can be marked, so the feature is invisible until
   someone presses space on a hunch. The pane's bottom-right slot is free and
   `space: mark` / `3 marked` would fill it — the single change most likely to
   make marking discoverable. Not done.

7. **Should the marker column render when nothing is marked?** Always-on
   costs two cells of width on every row for a feature most rows never use;
   appearing on the first mark reflows the content under the user at the worst
   moment. Leaning toward always-on when `Markable` is set, since `Markable`
   is already opt-in per component.
