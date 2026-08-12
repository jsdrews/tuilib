# Output log — design

Status: **implemented**. See "Changed during implementation" at the bottom for
the four places the built thing differs from what was agreed here.

## Problem

`app.Info` / `app.Error` are the only feedback channel a screen has, and they
land in the statusbar's center slot — one line, width `w - left - right`,
wiped by `statusbar.Update` on the **next `tea.KeyMsg`**. There is no buffer.
The string is overwritten and gone.

Worse, the output worth reading usually never reaches that call at all.
`runner.Run` hands the real TTY to the subprocess and returns
`Result{Cmd, Err}` — it captures **nothing**. `examples/data/runlog` is the
only capturing path in the codebase, and it is ~40 lines of `io.Pipe` +
goroutine + `bufio.Scanner` + chained `tea.Cmd` that every author re-types per
command.

So: run a command, watch a sliver of a footer flash by, press a key, lose it.

## Shape

An opt-in, app-wide console. One shell-owned ring buffer of records, fed
automatically from every `Info`/`Error`, from an explicit detail channel, and
from a new non-suspending `runner.Capture`. A right-slot statusbar badge shows
unread events and tints red on unread errors. Pressing the configured key
pushes a `logview` screen over the stack; it pops with a sentinel.

---

## Decisions

**1. Scope — one app-wide buffer.** Not per-screen. A screen-scoped buffer
loses the output the moment you navigate, which is the same failure one
keystroke later. Screens wanting an in-view stream still have `pkg/logview`;
that is a different thing.

**2. Feeds — automatic *and* explicit.** The shell intercepts every
`StatusInfoMsg` / `StatusErrorMsg` and appends it, so existing apps gain a
recoverable history with zero code changes. Plus a detail channel, because the
footer sliver is truncated *precisely because it was authored to fit a footer*
— auto-capture alone yields a scrollable history of slivers.

**3. Entry shape — flat lines.** Not collapsible records. The dominant use is
"the thing I just ran failed, show me why": open, follow has you at the bottom,
read the dump. Collapsed-by-default rows make that case worse. Accepted cost: a
400-line dump cannot be folded away.

**4. Surface — a pushed `screen.Screen`.** Not a drawer, not a `ZStack`
overlay. The stack already does the routing: `IsCapturingKeys` propagates when
the logview's `/` search is focused, `SetTheme` reaches it, `Help()` composes,
mouse routes, and auto-esc-pop closes it. No new routing mode in `app.Update`.
Accepted cost: it takes a breadcrumb crumb, and you cannot see the output and
the screen that produced it simultaneously.

**5. Closing — pops with a sentinel.** `screen.Pop` fires `OnEnter(nil)` on the
uncovered screen, and rule 7 makes `OnEnter` the documented place to kick off
fetches — so glancing at a log would silently refetch. The output screen pops
with a typed value so a parent can early-return. Sentinel over a
non-activating pop: `PopTo` already carries a `Result`, so the machinery
exists, and a sentinel is honest — something *did* happen, the parent just gets
to decide it doesn't care.

**6. Affordance — right slot, not left.** The statusbar's left slot is rendered
entirely by `pkg/help` as one opaque string, and locating the clickable region
needs `help.AffordanceSpan` mapped through `sb.LeftContentRect()`. A second
left-slot affordance means either teaching `pkg/help` what a log is (wrong
layering) or the shell composing the slot from two pieces plus its own span
arithmetic, which perturbs `shortViewBudget()` and the overflow gate on `?`.
The right slot is a plain string. Needs a new `statusbar.RightContentRect()`
mirroring the existing left one.

**7. Badge state — unread since last open, tinted on unread error.** Not a
total (a counter climbing toward the cap is not a signal), not errors-only
(that makes successful 40-line output invisible, which is the current bug).
Two-state so the door never disappears: `output` bare when everything's been
seen, `3 output` when it hasn't, nothing at all only while the buffer is
empty. Read-marker set on **pop**, so lines arriving while you sit on the
screen don't come back unread as you leave.

**8. Key — no library default.** Superseded by 15. The letter is spent by the
app, not by the shell.

**9. Source — stamped from `stack.Current().Title()` at arrival.** `app.Info`
is a closure with no idea who created it, and a `tea.Cmd` cannot introspect its
creator. Known failure mode, named deliberately: a screen kicks off a fetch,
you navigate away, the fetch fails, and the entry is stamped with the screen
you're standing on *now* — which is exactly the case the console exists for.
Accepted because a misattributed line still carries its time, level, and full
text: you lose a hint, not the content. Optional provenance was rejected —
provenance that's optional is absent exactly when someone was in a hurry.

**10. Line format — full prefix on every line.**

```
14:23:01 ERR Deploy › deploy failed: exit status 1
14:23:01 ERR Deploy │ + kubectl apply -f manifest.yaml
14:23:01 ERR Deploy │ error: unable to recognize "manifest.yaml"
```

`›` marks a head line, `│` a continuation. Every line stands alone, so
`logview`'s filter mode (`\`, which shows *only* matching lines) never orphans
a body line from the command that produced it. Cost: ~20 columns of every
line, leaving ~55 for content at 80 cols before h-scroll.

Colors are **foreground-only via `pkg/ansi.CellColor`**. `logview`'s
`CurrentLineStyle` pads a row to the pane's inner width to paint a background;
a full `lipgloss.Render` with a `\x1b[0m` reset in the level tag would punch a
hole in it — rule 17's reasoning, applied outside `pkg/table`.

Timestamps are wall-clock `HH:MM:SS`, not relative — relative ages would
require re-rendering the buffer on a tick, and a console is read after the
fact.

**11. Subprocess capture — both halves.** (a) The shell auto-logs
`runner.Result` exit status, which it already intercepts for the mouse re-enable;
a failing subprocess currently produces *nothing* unless the author checked
`Result.Err`. (b) A new `runner.Capture` that pipes stdout/stderr into the log.
Not a flag on `Run` — teeing a full-screen `htop` is meaningless, so it is a
separate function with different semantics: no terminal handoff, no alt-screen
suspend, output to the console. Without this the console ships empty by
default, because the missing output isn't in a variable someone forgot to log,
it's in a pipe nobody opened.

**12. Concurrent captures — source is the command label.** `Capture` doesn't
suspend, so parallel runs are possible and would otherwise braid
indistinguishably. For captured lines `Source` is `filepath.Base(cmd.Path)`
(overridable) rather than the screen title. Costs zero extra columns — for
captured output the honest answer to "what produced this line" *is* `kubectl`.
Loses which screen initiated the run; the completion summary carries both.
Two concurrent runs of the same binary still braid; accepted.

**13. The calling screen also sees the stream.** The shell logs each line then
forwards the message. Under console-only, an author wanting a live in-view
logview has to hand-roll the pipe anyway, leaving the library with **two
subprocess pipelines that do the same thing** — one in `pkg/runner`, one as a
recipe everyone copies. One pipeline; "do I also want it on screen?" is
answered by whether the screen bothers to match the message.

**14. Packaging — new `pkg/output`; `pkg/runner` stays dependency-free.**
`runner` currently imports nothing from tuilib, and that is why it is safe for
anything to use. It posts its own neutral messages; `pkg/app` translates them
into records, exactly as it already intercepts `runner.Result`. The translation
hop is where `Source` is decided, and that is shell knowledge, not runner
knowledge. Matches the grain of the repo (`pkg/geom` exists as a leaf
*specifically* to avoid a dependency). Known seam: the buffer lives in
`output`, the read-marker and badge live in `app` — data vs chrome.

**15. Opt-in via a single `OutputKey`.** Not on by default. `app.Options`
already encodes the principle: `ThemeKey` is opt-in *because it claims a
letter*, and `Mouse` defaults off with the rationale spelled out — taking
something from the user is "a trade the app author should make." The output log
claims a letter permanently in every app that links the shell. Set `OutputKey`
and you get buffer, badge, screen, capture logging; leave it zero and none of
it exists. `runner.Capture` still runs and the calling screen still receives
every line when it's off — you lose the console, not the pipeline.

**16. Detail channel — paired functions.** `InfoDetail(summary, body)` /
`ErrorDetail(summary, body)`, not a level argument and not a struct. A level
constant would drag `pkg/output` into every screen file that wants to log a
stack trace; `app.Error("boom")` needs only `pkg/app` today and the verbose
variant should too. Body splits on `\n`; an empty body degrades to exactly
`Info`/`Error`; the summary still paints the statusbar, so `ErrorDetail` is a
strict superset of `Error`. No source argument — consistent with 9.

Plus `ErrorOf(err error)`: summary is `err.Error()`, body is the unwrapped
`%w` chain one wrap per line. Without it authors flatten the chain to its
outermost message, because hand-formatting it at every site is work nobody
does.

**17. Badge unit — events, not lines.** A `go build` streaming 3,000 lines
would render `3000 output`, which is a wall counter, not a signal. One
`Info`/`Error`/`*Detail` call is one event; **one capture run is one event**
regardless of line count. A 200-line dump is not 200 pieces of news.

- Increment at **start**, not completion — otherwise a five-minute build gives
  no footer signal until it's done, and "keep working while it runs" becomes
  "keep working, blind." It also means opening mid-run shows live output.
- In-flight runs get a **static** marker (`⟳ 2 output`). Animating it needs its
  own tick loop repainting for the whole build, and `apply()` already rebuilds
  the bar on every update.

**18. Storage — `[]Record` rendered at view time, not pre-rendered strings.**
Decision 3 was about *shape* (one line per row, no folding), not storage. Under
pre-rendered strings every line is baked with the palette active when it was
written, so a theme swap turns the log into a stratigraphy of old themes —
which is what rule 4 exists to prevent, and which cannot be honored without
storing enough structure to re-render. Everything else falls out instead of
needing side-machinery: unread events count `Head` records, in-flight captures
are `RunID`s with no terminal record, the `⟳` marker is derived.

Cost: `output` maintains its own ring and feeds `logview` rendered lines,
rather than using `logview.Append` as the buffer. The cap logic has to live in
`output` regardless, because trimming must be **event-aware** — cutting a
3,000-line build in half should not strand the survivors from the head record
naming the command. Default 10,000 records, matching
`logview.DefaultMaxLines`; the screen's logview runs `MaxLines: -1` so two caps
don't fight.

**19. Screen actions — clear, kill, export.** Not read-only.

- `c` clear. No confirm — the log is a convenience, not a record, and a modal
  on the cheap action is friction. (`c` verified free; nothing in `pkg/` binds
  it outside tests.)
- `x` kill, through `pkg/confirm` per rule 20 — destructive, and triggered from
  a viewer behind the back of whichever screen started the process. With more
  than one run in flight it needs a pick step first, which is rule 8's
  menu-driven case arriving naturally. **Not `k`** — that is scroll-up, and
  rule 23 is absolute.
- `w` export. This is the least optional of the three: `app.Options.Mouse`'s
  own doc says mouse reporting takes the terminal's click-drag text selection
  away, so in a mouse-enabled app — the app you'd want this in — you **cannot
  select and copy a stack trace out of the console**. Write-to-file is the only
  exit.

**20. Export — auto-named, filter-scoped, self-describing.** Writes to
`output.Options.ExportDir` when set, `$TMPDIR` otherwise, as
`output-YYYYMMDD-HHMMSS.log`, and reports the path via `app.Info`. Prompting
would add a text field, a focus region and a capture state to an otherwise pure
viewer, for a path nobody cares about; what they care about is knowing where it
went. (Pleasing closure: that `Info` is itself auto-captured, so the path
survives the next-keypress wipe — the original complaint.)

Exports exactly what the pane is showing: filter mode (`\`) narrows it, a bare
`/` query does not, which is already the distinction those keys make. Scroll
position is irrelevant — the whole filtered set, not the visible window. First
line is a `# filter: <query>` header when narrowed, so a truncated export
attached to a bug report says so.

**21. Never auto-opens.** Not on error, not opt-in-on-error. Decision 4 made
this a pushed screen, so auto-open mutates the nav stack asynchronously at a
moment the user didn't choose: mid-`/`-filter on a table when a two-screens-ago
fetch fails, or on top of a live `confirm` modal. Both are worse than missing
the message. An opt-in flag is the foot-gun version — it looks safe because
someone consented, but the author consenting has no idea what screen the user
will be standing on. The badge solves "I didn't notice" without touching
navigation; that was the point of the tint.

---

## API surface

### `pkg/output` (new)

```go
type Level int
const (
    LevelInfo Level = iota
    LevelError
)

// Record is one rendered line's worth of structure. Flat — a body line is a
// Record with Head=false, not a field on its parent.
type Record struct {
    Time   time.Time
    Level  Level
    Source string
    Text   string
    Head   bool  // true = summary (›), false = continuation (│)
    RunID  int64 // non-zero for records belonging to a Capture run
}

type Buffer struct{ /* ring, cap, read-marker, in-flight set */ }

func (b *Buffer) Append(r Record)
func (b *Buffer) Records() []Record
func (b *Buffer) Clear()
func (b *Buffer) MarkRead()
func (b *Buffer) Unread() int      // Head records since the marker
func (b *Buffer) UnreadError() bool
func (b *Buffer) InFlight() int

type Options struct {
    MaxRecords int    // default 10000
    ExportDir  string // default $TMPDIR
    // logview/pane chrome pass-throughs, from theme.Output()
}

// Closed is the pop sentinel. It lives here, not in pkg/app: pkg/app imports
// pkg/output, so the reverse would be a cycle. pkg/app aliases it so screens
// matching it in OnEnter don't need a second import.
type Closed struct{}

type Screen struct{ /* implements screen.Screen */ }
func NewScreen(b *Buffer, t theme.Theme, opts Options) *Screen
```

### `pkg/app` (additions)

```go
// Options
OutputKey key.Binding    // zero = the whole feature is off
Output    output.Options

func InfoDetail(summary, body string) tea.Cmd
func ErrorDetail(summary, body string) tea.Cmd
func ErrorOf(err error) tea.Cmd

type OutputClosed = output.Closed // alias; see output.Closed
```

### `pkg/statusbar` (addition)

```go
func (m Model) RightContentRect() geom.Rect // mirrors LeftContentRect
```

### `pkg/runner` (additions — no tuilib imports)

```go
type CaptureOptions struct {
    Cmd   *exec.Cmd
    Label string // defaults to filepath.Base(cmd.Path)
}

func Capture(cmd *exec.Cmd) tea.Cmd
func CaptureWith(opts CaptureOptions) tea.Cmd

// CaptureStarted carries Cmd so the shell can retain a kill handle for
// decision 19 and increment the badge at start per decision 17.
type CaptureStarted struct {
    RunID int64
    Label string
    Cmd   *exec.Cmd
}

type CapturedLine struct {
    RunID  int64
    Label  string
    Text   string
    Stderr bool
}

type Captured struct {
    RunID int64
    Cmd   *exec.Cmd
    Label string
    Err   error
}
```

### Badge rendering (in `pkg/app`, styled from `theme.Output()`)

| State | Right slot |
|---|---|
| Buffer empty | *(nothing)* |
| All read | `output` |
| 3 unread | `3 output` |
| Any unread error | `3 output`, error tint |
| Any run in flight | `⟳ ` prefix |

Rendered before the version string. Clickable via `RightContentRect`, mirroring
how `clickChrome` handles the `? help` affordance.

---

## Implementation order

1. **`pkg/output`** — `Record`, `Buffer` (event-aware trim, read-marker,
   in-flight), `Options`, `Screen` (logview `Searchable: true`, follow on,
   `MaxLines: -1`, `Title() = "Output"`). Tests for trim boundaries and unread
   accounting.
2. **`pkg/statusbar`** — `RightContentRect()`.
3. **`pkg/theme`** — `theme.Output()`, covering the screen's logview chrome
   *and* the badge's normal/error styles. The badge is shell chrome, so its
   colors need a home in the palette rather than being set inline in `app`.
4. **`pkg/app`** — `OutputKey` gate, buffer ownership, auto-capture of
   `StatusInfoMsg`/`StatusErrorMsg`, `InfoDetail`/`ErrorDetail`/`ErrorOf`,
   badge in the right slot + `clickChrome` branch, push/pop toggle,
   `OutputClosed` alias, auto-log of `runner.Result`.
5. **`pkg/runner`** — `Capture`/`CaptureWith` + the three message types; `app`
   translation into records (label as source, `RunID` threading).
6. **Screen actions** — `c` clear, `x` kill (confirm + pick), `w` export with
   filter scoping and the `# filter:` header.
7. **`examples/app/output`** + launcher entry; a rule in `CLAUDE.md`; a
   `internal/componenttest` routing case so the screen isn't the fourth one to
   ship with mouse routed to the focused component only.

## Rejected, worth remembering

- **A drawer or `ZStack` overlay.** Needs a third key-routing mode in
  `app.Update` (keys to chrome, not to the screen) that does not exist today.
- **Collapsible entries.** Not a `logview` — a new component.
- **Required or optional `Source` on the API.** A `tea.Cmd` closure cannot
  name its creator; optional provenance is absent when it matters.
- **`app.Detail(level, ...)` or a struct call.** Drags `pkg/output` into every
  caller.
- **Pre-rendered `[]string`.** Breaks theme swap (rule 4).
- **Auto-open on error.** Mutates the nav stack asynchronously.
- **`runner` depending on `pkg/output`.** Costs runner its independence for one
  translation hop that belongs in the shell anyway.

## Changed during implementation

Four things the design got wrong, found by building it.

**1. `theme.Output()` can't exist — it's an import cycle.** `Screen` implements
`screen.Screen`, and `pkg/screen` imports `pkg/theme` for `SetTheme`. A `Theme`
method returning `output.Options` would close the loop
(`output → screen → theme → output`). The dependency is inverted instead:
`output.OptionsFrom(t theme.Theme)`, with `output` importing `theme` and `theme`
knowing nothing about `output`. This is the one place the library breaks rule 3's
`th.Component()` convention, and it is documented in the package doc. The
alternative — moving `Screen` into `pkg/app` — would have preserved the
convention at the cost of the thing decision 14 explicitly wanted, a screen
testable without an app shell.

**2. The pop sentinel had to live in `pkg/output`, not `pkg/app`.** Same reason,
one layer down: `app` imports `output`, so the console screen cannot pop a type
defined in `app`. It is `output.Closed`, with `type OutputClosed = output.Closed`
aliased in `app` so screens matching it in `OnEnter` still need only the one
import — which was the whole argument in decision 16.

**3. `Record` gained a `Stderr` field.** Decision 11 didn't say where captured
stderr goes, and the obvious answer — `LevelError` — is wrong: `go build`,
`git` and `curl` all write progress to stderr, so it would leave the badge
permanently red and the tint would stop meaning anything. Severity now comes
from the run's exit status alone, and `Stderr` carries the stream distinction
visually, as a heavier gutter glyph (`┃`). Without the extra field the console
would have lost the stdout/stderr distinction entirely.

**4. Two pre-existing statusbar bugs had to be fixed for the badge to work.**
Both come from the same root: the help line pads out to whatever budget it is
given, so the bar's center slot is routinely sized to zero, and lipgloss's
`Width` is a *minimum*.

- A message longer than its slot rendered anyway, overflowing and shoving the
  right slot off the end of the bar.
- A style with `Padding(0,1)` renders **two cells for empty content**, so a
  slot sized to zero still cost two — clipping the last two characters of the
  right slot on every frame where the panel was open (`? close` being two
  cells wider than `? help` is what pushed it over).

Both used to cost only a couple of characters of the version string, which is
presumably why they went unnoticed. With the badge in the right slot they cost
the badge — exactly backwards, since the badge is the thing that says the full
text is still recoverable. Three changes: `statusbar.View` cuts the message to
its slot, `renderMiddle` fills the slot in exactly `w` cells (including `w=0`),
and `app.apply` reserves the message's width (capped at half the bar) before
handing the rest to the help line.

`TestFooterFitsWithBadgeAndVersion` sweeps four widths × panel open/closed.
Worth noting the whole suite was green while this was happening — it was found
by rendering the shell and reading the output.

**5. The shell advertises the output key itself.** Not in the design at all.
Every other global follows the opposite convention: a screen lists `q` and `t`
in its own `Help()`. That works because those keys exist in every app, so an
author already knows to list them. The output key is opt-in, so it exists in
some apps and not others, and an author copying an existing screen has no
reason to add it — it would end up advertised on the one screen whose author
remembered. `app.helpBindings` adds it to whatever the active screen returns,
and flips the description to "close output" while the console is open, since
the same key does both.

It goes **first** in the list, which is not cosmetic. The expanded panel is
capped at `HelpMaxRows`, so its tail is simply not drawn; a binding appended
at the end is the first casualty on precisely the screens with enough bindings
to need a panel. Appending shipped broken — visible on a sparse screen at 120
columns, gone on a table at 80.
`TestOutputKeySurvivesACappedHelpPanel` sweeps four widths against a
24-binding screen.

**6. `Capture` needed process-group kill and a bounded drain.** Found by CI on
Linux, where `TestKillStopsALongRun` hung; macOS never reproduced it. Two
distinct defects behind one symptom, both about a descendant outliving the
process we started and keeping the pipe's write end open:

- **Kill only killed the process we started.** Captures are typically a shell
  wrapping real work, and the shell forwards nothing — so the build kept
  running, unreachable, with the consumer's only handle already dead. Fixed by
  putting the child in its own process group (`Setpgid`) and signalling the
  group. Windows has no portable equivalent; `capture_windows.go` says so.
- **`Captured` could never fire.** With `io.Pipe`, `os/exec` spawns a copy
  goroutine and `Wait` blocks on it until *every* writer closes — so one
  lingering grandchild wedged the run permanently: still counted by the badge,
  no longer killable. Fixed by using real `os.Pipe`s (exec hands an `*os.File`
  straight to the child, so `Wait` tracks only the process) plus a
  `drainGrace` after which the read ends are closed out from under whatever
  still holds them.

Also: killing a run that had just exited surfaced "process already finished"
as a failed kill. `ignoreDone` maps that to success — the user got what they
asked for and can't act on the difference.

## Tests

- `pkg/output/buffer_test.go` — trim cuts on event boundaries, oversized events
  keep their head, trimming is amortized, unread never exceeds what survives,
  tint from a continuation line, tint clears when the error is trimmed away.
- `pkg/output/render_test.go` — full prefix on every line, glyph column
  alignment, foreground-only resets (rule 19's property), badge states.
- `pkg/output/screen_test.go` — the buffer mirror, rebuild after trim, export
  scoping + `# filter:` header + plain text.
- `pkg/runner/capture_test.go` — real subprocesses: stdout/stderr split,
  non-zero exit, a binary that never starts, more lines than the channel
  buffer (backpressure), kill, kill-after-exit, and the two orphan cases from
  item 6 (a descendant holding the pipe, and a kill that has to reach
  descendants). Both orphan tests assert an elapsed-time bound, so a
  regression fails rather than hangs.
- `pkg/app/output_test.go` — opt-in, auto-capture, detail bodies, `ErrorOf`,
  badge states, toggle + sentinel on both `o` and esc, read-on-close, stderr
  not tinting, one-event-per-capture, footer fit.
- `internal/componenttest/screenrouting_test.go` — the example screen is in the
  mouse fan-out case.
