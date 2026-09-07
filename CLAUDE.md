# Agent guidance for tuilib

This file is read automatically when an agent enters this repo. It encodes
the rules that keep generated code consistent with the library's design. For
recipes and the full component reference, read `README.md` and the relevant
example in `examples/`.

## The rules

1. **Default to `pkg/app` + `pkg/screen` + `pkg/layout`.** A tuilib TUI is a
   root `screen.Screen` handed to `app.New(app.Options{...})`. The shell
   owns breadcrumb + statusbar + theme cycling + global-key routing +
   auto-esc-pop. Your screen declares a `layout.Node` tree in `Layout()`
   and handles its own state in `Update`. Only drop to a bare
   `tea.Model` + manual layout when you genuinely need something outside
   the shell's shape (rare). See `examples/app/stack/main.go` and
   `examples/app/layouts/main.go`.

2. **Describe layout declaratively, not with `m.h - N` math.** Compose
   `layout.VStack` / `HStack` / `ZStack` + `Fixed(n, …)` / `Flex(weight, …)`
   and wrap components via `layout.Sized(&c)` or `layout.Bar(&c)` —
   both take anything with `SetRect(geom.Rect)`, which every component
   satisfies. (`Bar` is `Sized`; the name just documents that a bar
   belongs in a `Fixed(1, …)` slot.) Never write `height - 2` to leave
   room for a bar — put the bar in a `Fixed(1, …)` sibling.

   A rect carries absolute position as well as size, which is what lets
   components answer mouse events without any marker injection — see
   rule 28.

3. **Always start from a `theme` builder.** Every component has a
   `th.Component()` method on `Theme` that returns a pre-styled `Options`.
   Set the task-specific fields (`Title`, data, `Filterable`, …) and pass
   it to `component.New()`. Don't set colors inline unless you're
   deliberately overriding one. Don't set `Width`/`Height` on components
   you're going to hand to `layout.Sized` — the layout engine sizes them.

4. **Rebuild themed components in `SetTheme(t)`, preserving state.** Use
   accessors (`m.list.Cursor()`, `m.list.Value()`, …) and setters
   (`m.list.SetCursor`, `m.list.SetValue`) to carry state across the
   rebuild. The app shell calls `SetTheme` on the current screen during
   theme swaps. Under the app shell you do *not* need a resize handler —
   layout takes care of it.

5. **Gate global keys via `IsCapturingKeys()`.** Return `true` from your
   screen's `IsCapturingKeys()` whenever an embedded component owns
   input (e.g. `s.list.Filtering()`, a focused filter, a visible modal).
   The shell then suppresses `q`, theme-cycle, and esc-pop so those keys
   flow to the component instead. Inside `Update`, don't add a parallel
   guard — check `Filtering()`/`Focused()` only where *your* screen's
   own shortcuts would otherwise collide with input.

   A screen using a `focus.Group` (rule 27) usually just delegates:
   `return s.focus.IsCapturingKeys()`. Every component that swallows
   printables implements `IsCapturingKeys()` itself — true while a text
   field is focused or a filter is engaged — so the Group can answer
   from whichever one currently holds focus.

6. **Forward every `tea.Msg` to embedded components.** Don't conditionally
   skip forwarding — each component decides what to act on. Even if you
   intercepted a key for your own shortcut, still forward it so focus +
   viewport behavior stays correct.

   Two deliberate exceptions, both about *user input aimed at what's on
   screen*. A screen with a focus.Group sends `tea.KeyMsg` to the
   focused component only — otherwise typing in an input would also
   drive a list's cursor — but sends `mouse.Msg` to **every** component,
   since each tests the position against its own rect and only the one
   it landed in acts. `pkg/tab` inverts the second half: mouse goes to
   the *active body only*, because hidden bodies aren't on screen
   (rule 21).

7. **Pass data through the stack, not globals.** Parent → child: construct
   the child with what it needs (`screen.Push(newDetail(city, s.t))`).
   Child → parent: `screen.Pop(result)`. The newly-active parent receives
   the value in `OnEnter(result any)` — that's where you rebuild UI that
   depends on it. On the initial push `OnEnter` fires with `nil`.

8. **Interaction should be menu-driven.** Prefer lists + enter over letter
   shortcuts for per-screen actions (`d` delete, `r` run, etc.). Reserve
   single-letter keys for app-wide affordances (`q`, `t`, `/`, `?`,
   `esc`). `?` opens the app shell's key overlay (see the help entry
   under "Where to learn more"). This keeps `Help()` honest and avoids
   shortcut collisions across screens.

9. **Components own their pane.** Every interactive component in `pkg/`
   bundles a `pane.Pane` internally — `pkg/list`, `pkg/table`, `pkg/filter`,
   `pkg/input`, `pkg/toggle`, `pkg/logview`, `pkg/textview`, `pkg/tree`, `pkg/inspector` all return a
   bordered, titled render from `View()`. To put a label on a component,
   set its `Title` field (which is rendered on the pane's top border) —
   don't render a label line above the component, and don't wrap a
   component in a second `pane.Pane`. The only things that *don't* own a
   pane are bars (`breadcrumb`, `statusbar`), `help.Model` (the footer
   strip — `help.Overlay` does own one), the layout primitives, `pkg/runner` (which is not a UI
   component — it suspends the program to run a subprocess), and
   `pkg/form` itself, which is a vertical layout of bordered fields. New
   input-style components should follow the same shape: `Options.Title` +
   an internal `pane.Pane` + `View()` returns the bordered render.

   A filterable component draws its filter *inside* its own pane, as a
   pinned header row above a rule (`pane.SetHeader`), not as a second
   pane stacked above it. Two equal-weight boxes read as siblings when
   the relationship is parent-child — people genuinely cannot tell which
   filter belongs to which pane once there are two components on screen.
   One border says it unambiguously, and costs 2 fewer rows.

   A component owns its *geometry* as well as its pane: it stores the
   rect `SetRect` gave it and does its own mouse hit-testing (rule 28).
   Nothing outside the component knows where its rows, glyphs or buttons
   are, which is exactly why the component has to be the one to decide a
   click was its own.

10. **Components expose `Help() []key.Binding`.** Interactive components
    (`list`, `table`, `filter`, `input`, `toggle`, `logview`, `textview`, `tree`,
    `inspector`, `form`) return the bindings they currently respond to. Screens compose these into their
    own `Help()` so the hint strip updates as state changes — e.g. the
    focused field of a form, or whether a logview's filter is engaged.
    When state changes the relevant bindings (filter focused vs. blurred,
    a query is active in logview), `Help()` reflects it.

    Mouse affordances are advertised the same way, as bindings with
    sentinel keys — `key.WithKeys("mouse:click")`, `WithHelp("click",
    "focus + select")`. The sentinel matters twice: `help.Compile`
    dedupes on the joined key string, so two *keyless* bindings would
    collapse into one, and a sentinel can never match a real
    `tea.KeyMsg`. Surface these in the `?` overlay rather than the
    inline strip, which is already short on room.

    **Components also expose `HelpSections() []help.Section`, and derive
    `Help()` from it** (`help.Flatten`), so the flat list and the grouped
    one can never disagree. Groups are named by what the keys *do*, from
    one shared vocabulary — `help.SectionNavigate` / `Scroll` / `Filter`
    / `Search` / `Select` / `Sort` / `Expand` / `View` / `Edit` /
    `Submit` / `Tabs` — so "Navigate" means the same thing in a list, a
    table and a tree. The groups are the `Keys` struct's own shape (rule 26); all
    `Help()` ever did was flatten them away.

    Never name a group after its owner. A heading named for the screen
    or the component ends up over every binding that owner has, which is
    how "Multi-select" came to sit above a table's scroll keys. The
    owner is a *qualifier*, applied by `help.Qualify` only when more
    than one of them is on screen ("files · Navigate"); `focus.Group`
    does this for the panes it holds.

    A screen composes with `help.SectionsOf`, which takes components,
    sections and binding lists in one call:

    ```go
    func (s *Screen) HelpSections() []help.Section {
        return help.SectionsOf(&s.table, help.Group("Deployments", s.verbs()...))
    }
    ```

    A screen with more than one interactive component forwards
    `focus.Group.HelpSections()` instead — one line, like
    `IsCapturingKeys`. A `pkg/tab` host forwards `tabs.HelpSections()`,
    which carries the strip's keys plus the *active* body's groups.

    **A screen that hosts a component and doesn't forward its groups
    falls back to one group named after the screen** — which is the bug
    this rule exists to prevent, with the screen's name standing over
    bindings it doesn't describe. Every example that hosts a component
    forwards; the ones that don't (a bare `pane`, say) have only their
    own verbs to list, which is what the fallback is for.

    The library's own screens are held to this too: `pkg/output`'s console
    forwards its logview's groups and adds `Log` for clear/export/kill,
    because a screen that ships *with* the library would otherwise put a
    heading named "Output" over the logview's scroll keys in every app
    that sets `OutputKey`.

    **`pkg/filter` is the one component with no `HelpSections`, and it
    stays that way**: `pkg/help` imports it for the overlay's own `/`
    search field, so `filter` implementing `help.Sectioned` is an import
    cycle. It is a leaf below `help`, not an oversight — its two bindings
    reach the overlay through whichever component embeds it, each of
    which groups them under `Filter` or `Search` itself.

11. **Run interactive subprocesses through `pkg/runner`.** For editors,
    pagers, full-screen TUIs, or any command that needs the terminal
    (`$EDITOR`, `less`, `htop`, `ssh`), call `runner.Run(*exec.Cmd)` from
    your screen's `Update`. It suspends the alt-screen, hands the TTY to
    the subprocess (with `Stdin/Stdout/Stderr` and `LINES`/`COLUMNS` env
    pre-populated), and posts a `runner.Result` back when the subprocess
    exits. Don't call `tea.ExecProcess` directly — the wrapper handles
    fallback plumbing for terminals that miss the post-resume SIGWINCH.

    Handing the terminal over also drops mouse reporting, and bubbletea's
    `RestoreTerminal` does not restore it — it knows about the alt screen,
    bracketed paste and focus reporting, but has no notion of mouse state.
    The app shell re-enables it on `runner.Result` *and* on
    `tea.ResumeMsg`, so neither a subprocess nor a ctrl+z suspend leaves
    the TUI mouse-dead. An app driving `tea.ExecProcess` or `tea.Suspend`
    outside the shell has to do that for itself.

    Suspend is a shell-owned global key like `q` and `t`
    (`app.Options.SuspendKey`, default ctrl+z, off via
    `DisableSuspend`). Bubbletea does not bind it: it delivers ctrl+z as
    an ordinary key and waits for the app to ask, so without the shell
    the key does nothing. It is suppressed while a screen is capturing
    keys — suspending mid-filter would strand the query — and ignored on
    Windows, where bubbletea has no suspend support.

12. **Stream subprocess output via a chained `tea.Cmd`.** When you want
    stdout/stderr in a logview rather than a terminal handoff, point
    `cmd.Stdout` and `cmd.Stderr` at one `io.Pipe`, call `cmd.Start()`,
    then spawn a goroutine that does `cmd.Wait()` + `pw.Close()`. A
    `tea.Cmd` reads one line via `bufio.Scanner` and posts a
    `logLineMsg`; on receipt, `Update` appends + re-dispatches the read.
    EOF posts `logDoneMsg`. No goroutine touches the model directly. To
    interrupt or kill, call `cmd.Process.Signal(syscall.SIGINT)` or
    `cmd.Process.Kill()` (SIGKILL). See `examples/data/runlog`.

13. **Cap streaming buffers.** Components that accept open-ended input
    (`pkg/logview`) apply a default `MaxLines` cap so an unbounded
    producer can't grow memory without limit. When wiring up a streaming
    component, decide on an explicit cap if the default isn't right;
    only set `-1` (unbounded) when the producer is itself bounded.

14. **Verbose output belongs in the console, not the statusbar.** The
    statusbar's center slot holds one truncated line and `statusbar.Update`
    wipes it on the next `tea.KeyMsg`. Anything a user might want to read
    twice — a command's stderr, a `%w` chain, an API response body — needs
    `pkg/output`, the app-wide console. Set `app.Options.OutputKey` to turn
    it on (the whole feature hangs off that one field; there is no default,
    because the shell claiming a letter takes it from every component in
    every downstream app, which is exactly why `ThemeKey` is opt-in too).

    Four things feed it, and the first is free: every `app.Info` /
    `app.Error` is captured automatically, so existing call sites become
    recoverable with no change. Then `app.InfoDetail(summary, body)` /
    `app.ErrorDetail` put the summary on the bar and the body only in the
    log; `app.ErrorOf(err)` does that with the unwrapped `%w` chain, one
    wrap per line; and `runner.Capture` streams a subprocess into it.

    Reading it is a pushed screen (`o`, esc, or the statusbar badge), which
    means **the pop carries a sentinel**. `screen.Pop` fires `OnEnter` on
    whatever it uncovers, and rule 7 makes `OnEnter` the place to start a
    fetch — so a screen that fetches must say it doesn't want this one:

    ```go
    func (s *Screen) OnEnter(result any) tea.Cmd {
        if _, closed := result.(app.OutputClosed); closed {
            return nil   // a glance at the log is not a fresh activation
        }
        return s.fetch()
    }
    ```

    The badge counts **events, not lines** — one `Info`, or one whole
    capture however many lines it emitted. It tints red from a run's exit
    status, never from stderr: plenty of well-behaved tools log progress
    there, and folding that into severity leaves the badge permanently red.

    There are three ways in: the key, the statusbar badge, and **clicking
    the status message itself** — the sliver is a truncated echo of what
    the log holds in full, so "show me the rest" is the obvious thing to
    want from it, and the colored band it renders as spans the whole
    center slot. The message opens but never toggles: the console raises
    messages of its own, and clicking one to close the view that produced
    it would be backwards.

    Opening the console **clears the statusbar message**. The message is a
    truncated echo of something the log is about to show in full, so
    leaving it up puts a stale sliver under the view that supersedes it.
    Closing does not clear, so a notice the console itself raised (the
    path `w` reports after an export) survives the trip out.

    **Don't list the output key in your screen's `Help()`.** This is the
    one global the shell advertises for you, and it would otherwise show
    up twice. It is the exception because it is *opt-in* — `q` and `t`
    exist in every app, so an author writing a screen already knows to
    list them, while the output key exists only where someone set
    `OutputKey`. Left to the screens it would be advertised on the one
    screen whose author happened to remember. The hint flips to "close
    output" while the console is open, since the same key does both.
    See `examples/app/output` and rule 15.

15. **Capture a subprocess with `runner.Capture`, hand it the terminal with
    `runner.Run`.** They are counterparts, not modes of each other.
    `Run` suspends the TUI and gives away the real TTY — right for `$EDITOR`,
    `less`, `htop`, and the reason its output is unrecoverable (the shell
    logs the exit status and nothing more). `Capture` pipes stdout/stderr
    while the TUI stays live, so the user keeps working while a build runs.
    Teeing a full-screen program would be meaningless, which is why one call
    cannot serve both.

    The sequence is `CaptureStarted`, a `CapturedLine` per line, then one
    `Captured`. Under the app shell you don't drive it — the shell chains
    the reads and forwards every message on to your screen, so a view that
    wants the output on screen *as well as* in the console just matches
    `CapturedLine`. Outside the shell, chain it yourself with
    `runner.Next(msg)`; a capture nobody drains eventually stalls the
    subprocess rather than growing without bound.

    Don't hand-roll the `io.Pipe` + goroutine + `bufio.Scanner` dance any
    more. `examples/data/runlog` still shows it because it predates this,
    but new code has no reason to repeat it.

16. **Enter means "open the focused selection."** In multi-pane screens
    with focus cycling, enter should have a single conceptual meaning
    across the whole screen — open whatever is highlighted in the
    focused pane. The *side effect* differs per pane: on a master list
    enter loads the adjacent detail (and typically transfers focus to
    it); on a detail list enter pushes a child screen via
    `screen.Push(...)`; on a form it submits. The user never has to
    remember "which pane uses enter for what" — same verb, different
    object. This matches the launcher's enter-to-push convention and
    avoids the alternative of overloading per-pane keys (e.g. `>`/`d`
    just to drill in). The pattern's payoff is most visible in
    `examples/data/drilldown` where enter on the cities list loads
    detail + shifts focus right, and enter on the focused detail
    pushes the level-3 attribute screen.

    Double-click is the mouse spelling of the same verb, so route both
    through one predicate rather than writing the open branch twice:

    ```go
    if s.menu.IsActivate(msg) {          // enter, or a double click
        return s, screen.Push(detailFor(s.menu.Cursor()))
    }
    ```

    `list.Model.IsActivate` / `table.Model.IsActivate` match enter (when
    the filter isn't taking input) *or* that component's own
    `ActivatedMsg`. The message carries a `focus.Token`, so two lists on
    one screen never claim each other's double clicks. Match
    `ActivatedMsg` directly only when you need its payload.

    A screen has to opt in: a component reports the activation, but what
    "open" means is the screen's call, and a screen that only checks for
    an enter `KeyMsg` will silently do nothing on double click. A
    *single* click only focuses and moves the cursor — it must always be
    safe to explore with.

17. **Use `SetLoading(b bool) tea.Cmd` while data is in flight.** Any
    component that owns a pane (`list`, `logview`, `tree`, …) inherits a
    loading state from `pane.Pane`. Calling `SetLoading(true)` returns a
    `tea.Cmd` you must batch into your screen's command stream — that's
    the spinner's first tick. The pane then renders a centered spinner
    (with optional `LoadingLabel`) in place of the body. On the fetched
    message, push the data into the component (`SetItems`/`AppendLines`/
    `SetRoot`) and call `SetLoading(false)`. Reset stale data before
    `SetLoading(true)` on refetch so the spinner replaces the previous
    result instead of overlaying it. Theme builders set `SpinnerStyle`
    from `Theme.Accent`; override only when you need a different color.
    See `examples/data/loading/loading.go`.

18. **Trust the pane to handle long lines.** `pane.Pane` truncates each
    line to the inner width on `SetContent` (ANSI-aware via
    `x/ansi.Cut`) and exposes left/right (and `h`/`l`) for horizontal
    scroll, with an optional thin scrollbar via `Options.HScrollbar`.
    Don't pre-wrap with newlines or `wordwrap.WrapString` to avoid
    terminal-wrap glitches — the pane already prevents that. Pre-wrap
    only when you specifically want soft-wrapping (prose paragraphs);
    otherwise let truncation + horizontal scroll do the work.
    `theme.Logview()` enables `HScrollbar` by default since long log
    lines are common.

19. **Color cells in a table row with `ansi.CellColor`, not full lipgloss
    backgrounds.** `pkg/table` applies the row's `SelectedStyle` (which
    typically sets `Background`) on top of cell content. A cell that
    contains its own `lipgloss.Render` with a full `\x1b[0m` reset will
    clobber the selected row's background mid-cell. `ansi.CellColor(n,
    text)` emits a foreground-only escape that closes with `\x1b[39m`,
    so the row's bg stays intact across colored cells. CellColor picks
    the shortest open form for the palette index (`\x1b[3Nm` for n<8,
    `\x1b[9Nm` for n<16, `\x1b[38;5;Nm` for 16+).

    Column truncation is ANSI-aware: `pkg/table` cuts cells via
    `x/ansi.Cut`, so `Column.Width` is the visible cell width — no
    escape-byte budget to add. The table example's Status column uses
    `Width: 12` for 10-char status text. (This is a change from the old
    bubbles/table-based example, which had to pad the Status column to
    22 to survive non-ANSI-aware truncation in upstream `runewidth`.)
    See `examples/data/table/table.go` Status column.

    For URL cells, wrap the visible label with `ansi.Hyperlink(url,
    text)` so shift-click / cmd-click in alacritty/tmux/kitty/iTerm2
    launches the full URL even when the column truncates the display
    text. Bare URL strings get cut mid-host on narrow columns and
    break the launched URL; OSC 8 decouples the underlying link from
    the visible label, and `x/ansi.Cut` preserves both ends of the
    OSC envelope across truncation. `ansi.ExtractHyperlink(cell)`
    pulls the URL back out when the screen needs to handle "open in
    browser" programmatically rather than via terminal click.

20. **Send transient feedback through `app.Info` / `app.Error` /
    `app.ClearStatus`.** Screens don't touch the statusbar directly — they
    return one of these `tea.Cmd`s from `Update` and the shell paints the
    bar's center slot. Use `Info` for successful operations ("Run
    completed", "Deployment triggered"), `Error` for surfaced failures
    ("Error: API request failed"), `ClearStatus` to wipe a stale message
    on a non-key event. Messages auto-clear on the next `tea.KeyMsg`, so
    don't try to manage their lifetime — set the message in response to
    the action that produced it and let the next keypress retire it. The
    auto-clear runs *before* the screen handles the key, so a screen can
    set a fresh message in response to the same key without it being
    immediately wiped. Don't read or mutate `m.sb` directly from outside
    `pkg/app`; the shell rebuilds the statusbar on every update and only
    preserves message state through `Message()` round-trips.

21. **For tabbed sub-screens, use `pkg/tab`.** A `tab.Model` hosts multiple
    `screen.Screen` bodies behind a one-row strip and lives entirely inside
    one host screen — it never touches the screen stack. The host forwards
    `Update` / `OnEnter` / `IsCapturingKeys` / `SetTheme` / `Help` to
    `tabs`, and surfaces the active tab in its breadcrumb via
    `Title() { return "Host › " + s.tabs.ActiveLabel() }`. Hold pointers to
    each body on the host (not just inside `tab.Model`) so theme rebuilds
    can reuse them and preserve cursors / queries / counters. Tab-switch
    keys default to `shift+left` / `shift+right` + `1`–`9`; do **not**
    rebind to `tab` / `shift+tab` — those are reserved across the library
    for inner pane focus cycling. If a tab body pushes a child screen via
    `screen.Push`, the cmd bubbles up through the host into the app stack
    and the host (with its tab pane intact) is preserved underneath. When
    that child later pops with a value, the host's `OnEnter(result)` should
    forward to `tabs.OnEnterActive(result)` so the body that initiated the
    push receives it (rather than a sibling tab).

    Message routing inside `tab.Model`: `tea.KeyMsg` and `mouse.Msg` go to
    the active body only (otherwise `/` filters, `j/k` cursors, etc. race
    across hidden tabs — and a click would be claimed by a hidden body
    still holding the rect it had when last drawn); everything else
    (timers, async fetch results, custom messages) fans out to every body
    so a `tea.Tick` re-arm in an inactive tab keeps streaming. See
    `examples/app/tabs`, where the Logs tab keeps appending lines while
    you're on the Cities or Counter tab.

    Clicking a label in the strip switches to that tab. The strip's own
    row is never part of the body, so this can't compete with whatever
    the active body draws.

    Strip position is configurable via `Options.StripPos`: `tab.StripTop`
    (default) puts the strip on the first row; `tab.StripBottom` renders
    the body first and the strip on the last row. Body height is the
    same either way — the tabs Model consumes exactly one row for the
    strip regardless of position. Reach for `StripBottom` in dashboard
    shapes where the primary content is above the fold and tab switching
    is a secondary affordance.

22. **For yes/no modals, use `pkg/confirm`.** A `confirm.Model` renders a
    bordered titled pane with two buttons and resolves via
    `confirm.ConfirmedMsg` / `confirm.CancelledMsg` posted as `tea.Cmd`s.
    The parent screen owns show/hide state, gates `IsCapturingKeys()`
    while the modal is up, and matches the result messages in its own
    `Update` to dismiss + act. The modal sits in a `ZStack` overlay on
    top of the base layout via `layout.Center(w, h, layout.Sized(&m))`.
    Default `Initial` is `false` (the cancel side starts highlighted),
    which matches the safer choice for destructive actions. While the
    modal is up, forward every `tea.Msg` to it; while down, don't.
    Compose the modal's `Help()` into the host's `Help()` only while
    the modal is up so the hint strip reflects the active context.
    Don't roll your own ZStack + toggle + key-routing — it re-implements
    what `pkg/confirm` already gets right (selection movement, y/n/esc
    shortcuts, theme-aware styling).

23. **For "stop and acknowledge" modals, use `pkg/alert`.** Same shape as
    `pkg/confirm` but with a single OK button and an `alert.DismissedMsg`
    result. Reach for it when the user *must* see and dismiss something
    before continuing — surfaced errors, destructive-action results,
    blocking notices. For passive feedback (success notices, transient
    warnings) prefer the lighter `app.Info` / `app.Error` statusbar
    messages from rule 20; the modal is heavier and breaks flow. The
    chrome from `theme.Alert()` is intentionally neutral: for an
    error-tinted look, override `ActiveColor` with `t.ErrorBG` (and
    optionally `OKStyle` foreground with the same) — the component is
    palette-agnostic and the semantics live in the override. For
    dynamic-length messages (subprocess stderr, `%w`-chained errors,
    multi-line API responses) set `Options.Autosize = true`: the modal
    word-wraps at 80% terminal width (40-col floor), caps height at
    60%, and scrolls the message region internally with the OK button
    pinned to the last inner row (scroll keys per rule 25:
    `↑↓`/`j`/`k`, `g`/`G`, `ctrl+u/d`, `PgUp/PgDn`). In autosize the
    host composes with `layout.Sized(&s.modal)` alone — the modal
    centers itself inside whatever bounds it gets, so the outer
    `layout.Center(w, h, ...)` wrapper is redundant. Hosting,
    `IsCapturingKeys`, `Help()` composition, and ZStack placement all
    follow rule 22. See `examples/data/alert`.

24. **For auto-refresh, use `pkg/poll` + keyed rows.** When data backing a
    view changes over time (k8s deployments, Prefect runs, REST endpoints),
    drive the cadence with `poll.New(poll.Options{Interval: 2*time.Second})`,
    batch its `Init()` into your screen's Init, and forward every `tea.Msg`
    to `m.poll.Update(msg)`. When the interval elapses, `Update` returns
    `poll.RefreshMsg` — your screen matches that and kicks off the fetch
    (typed result message, then `MarkRefreshed()` + push the data into the
    component). Pause/Resume/SetInterval/Refresh all return `tea.Cmd`s that
    handle the rescheduling — bump the cadence in response to a key, don't
    reach inside.

    Pair the poll with `SetKeyedItems([]list.KeyedItem{Key,Display})` (on
    `pkg/list`) or `SetKeyedRows([]table.KeyedRow{Key,Cells})` (on
    `pkg/table`) so the cursor sticks to the same row by Key after the
    swap, even when the underlying set has reordered or partially
    changed. Without keys, every refresh would either reset the cursor
    or drift it onto unrelated rows. `SelectedKey()` is the pre-swap
    side of the same primitive — read it before refetch, pass it back
    into the snap. `pkg/inspector` does the same dance internally on
    `SetFields` (preserves expansion + cursor by row path), so its
    auto-refresh is just "fetch new fields, call SetFields."

    The `app.Info` / `app.Error` statusbar (rule 20) is the right place
    for transient refresh feedback ("refreshed 14 deployments"); the
    statusbar auto-clears so it doesn't accumulate. For a persistent
    "last refreshed Xs ago" indicator, mutate the component's title via
    `SetTitle` from a periodic UI tick — see `examples/data/poll`.

25. **Reserve arrows + hjkl for scroll, library-wide.** Every component
    that scrolls in a given axis uses the same bindings on that axis,
    and arrow keys are aliases for hjkl (not separate verbs):

    - Vertical: `↑`/`k` up, `↓`/`j` down, `g`/`G` top/bottom,
      `ctrl+u`/`ctrl+d` half-page.
    - Horizontal: `←`/`h` left, `→`/`l` right, `0`/`home` to the start,
      `$`/`end` to the end. `pkg/table` additionally uses `shift+←` /
      `shift+→` to snap to the previous/next column edge.

    The mouse wheel is on the same axis and follows the same rule: it
    does exactly what `↓`/`j` does for the component **under the
    pointer** — moves the cursor in `list`/`table`/`tree`/`inspector`,
    scrolls the viewport in `logview`/`textview`/`pane`. It acts on
    whatever is under the pointer whether or not that pane has focus,
    and never takes focus: scrolling is navigation, not a claim on the
    keyboard.

    The pane (`pkg/pane`) owns the horizontal axis for every component
    that embeds it (`list`, `table`, `logview`, `tree`, `inspector`,
    `filter`, `input`); the cursor-owning component intercepts only the
    vertical keys and lets horizontal keys fall through. So no
    component-specific verb is allowed to consume an arrow or hjkl key.
    In particular: `pkg/tree` and `pkg/inspector` do **not** bind
    `←/→/h/l` to expand/collapse — use `space` to toggle the cursor row
    and `E`/`C` to expand/collapse all. Drilling into a child is two
    keystrokes (`space` then `↓`/`j`), which keeps the scroll convention
    consistent across the library.

26. **Override component keybindings via `Options.Keys`.** Every
    interactive component (`list`, `table`, `logview`, `tree`,
    `inspector`) exposes a `Keys` struct as `Options.Keys` plus a
    `DefaultKeys()` builder. Each binding carries both `WithKeys(...)`
    (dispatch) and `WithHelp(label, desc)` (hint text) so `Update` and
    `Help()` read from the same source — a custom binding propagates to
    the hint strip without restating anything. `theme.<Component>()`
    pre-populates `Options.Keys` with `DefaultKeys()`, so users who
    don't care get the stock bindings for free; users who do override
    individual fields on the returned options before passing to `New`,
    and any zero-valued bindings fall back to defaults via an internal
    `fillDefaults` field-by-field merge. Horizontal scroll lives on the
    embedded `pane.Keys` field at `Options.Keys.Pane` — mutate it to
    rebind `←/h/→/l/0/$` without touching the rest. Per-screen tweaks
    (one app's vim-only users get `dd` for delete) belong here; future
    user-level config will route through the same struct via
    `pkg/config`. Don't add a parallel package-level `var keys` — every
    binding the component dispatches against must live in its `Keys`
    struct. Mouse settings are the exception and deliberately do *not*
    live here: the double-click threshold is a per-machine preference,
    so it sits on `config.Config` (`double_click_ms`) with an
    `app.Options.DoubleClickInterval` override, and no component carries
    mouse configuration at all (rule 28).

27. **Compose multi-component focus with `pkg/focus`, not an int.** A
    screen with more than one interactive component holds a
    `focus.Group` over them in tab order:

    ```go
    s.focus = focus.NewGroup(&s.query, &s.results, &s.caseTgl)
    ```

    The Group owns tab/shift-tab cycling, the blur-everything-focus-one
    dance, and — the part a hand-rolled index can't do — granting focus
    to a component that was *clicked*, since that request arrives at the
    component rather than at the screen that knows the ordering. Batch
    `Group.Init()` into the screen's `Init`, forward every message to
    `Group.Update`, and route your own shortcuts with `Is`:

    ```go
    switch {
    case s.focus.Is(&s.cities): …
    case s.focus.Is(&s.detail): …
    }
    ```

    Every interactive component satisfies `focus.Focusable`
    (`Focus() tea.Cmd` / `Blur()` / `Focused() bool`). Components that
    swallow printable keys also implement `focus.Capturer`
    (`IsCapturingKeys() bool` — true while a text field is focused or a
    filter is engaged), so a screen's rule-5 gate is usually just
    `return s.focus.IsCapturingKeys()`. `Group.Help()` returns the
    cycling bindings plus the focused component's, so the hint strip
    tracks the active pane for free; relabel the cycling pair with
    `WithKeys` when "pane" reads better than the default "field".

    Rebuild the Group in `SetTheme` over the same field addresses and
    restore the index with `SetIndex` — the components behind those
    addresses are replaced, but the addresses are stable, so the Group
    keeps pointing at the right panes. See `examples/app/focus`.

    **A filterable component has two focusable regions behind one
    `Focusable`** — its filter and its body — and the two must stay
    reconciled or focus drifts across the screen:

    - `Focus()` lights the pane. It never blurs the filter, which
      matters because a click on the filter also asks the group for
      focus and that grant lands *after* — it must not undo what the
      click just did.
    - `Blur()` clears **both**. A filter left focused on a blurred
      component is invisible and still swallows keys — that is what
      breaks a second filterable pane.
    - `Focused()` is true if either region is active.
    - `FocusFilter()` / `BlurFilter()` move input between the two. The
      pane border stays lit throughout — it means "this component has
      focus". Which *region* has input is shown by the filter's prompt
      and the rule beneath it taking `ActiveColor`; an inline filter has
      no border of its own to light up, so those carry the signal. A
      cursor alone is not enough: it blinks, so half the time there is
      no cue at all.
    - **Any press inside the pane other than the filter row hands input
      back to the body** — the rule, the content, blank space below it,
      the borders. Leaving the filter focused after a click into the
      body is invisible and keeps swallowing keys. The exception is the
      scrollbar, which returns earlier: scrolling never claims the
      keyboard however it is expressed (rule 25), so dragging the bar
      leaves a half-typed query alive.

    `Group.Update` also declines to cycle while `IsCapturingKeys()` is
    true. Tabbing out of a half-typed filter would strand it, and
    `pkg/table` binds tab to complete a `key:value` term — leave the
    field with enter or esc, then cycle. `examples/app/filters` is the
    two-filterable-pane screen these rules exist for.

28. **Mouse support comes from rects, not from markers.** `pkg/layout`
    hands every node a `geom.Rect` — absolute position *and* size — and
    `layout.Sized(&c)` passes it to the component's `SetRect`. A
    component stores that rect and answers mouse events by testing
    against it. Nothing is injected into the rendered string, so there
    is no marker for `ansi.Cut` or `ZStack` to corrupt.

    Mouse is opt-in per app: `app.Options.Mouse = app.MouseClick`. The
    shell enables reporting from `Init`, so `tea.NewProgram` needs no
    mouse option of its own. It also translates each `tea.MouseMsg` into
    a `mouse.Msg` carrying a resolved click count — screens never see
    the raw event, and no component needs a double-click threshold
    plumbed into it.

    Inside a component, hit-test with `Rect.Hit(x, y)` rather than
    `Contains`: `Hit` also rejects events aimed at a component that
    wasn't drawn in the current frame, so a hidden tab body or a
    dismissed modal declines clicks without needing to know it's hidden.
    Decline anything outside your rect by returning the message
    untouched so a sibling can claim it.

    The verbs, library-wide:

    - **Click** focuses the component (via `focus.RequestSelf`) and
      moves the cursor to the clicked row.
    - **Double-click** does what enter does (rule 16) — components emit
      an `ActivatedMsg` and the screen decides what "open" means.
    - **Wheel** does what `↓`/`j` does for the component *under the
      pointer*, focused or not (rule 25). Scrolling is navigation, not a
      claim on the keyboard.
    - **Library-drawn chrome is clickable**: table headers sort, tree
      and inspector `▸`/`▾` glyphs toggle on a single click, modal
      buttons commit, tab labels switch, breadcrumb crumbs unwind via
      `screen.PopTo`, and scrollbars accept click-to-jump and drag.
    - **Input-style components too**: clicking an `input` focuses it,
      clicking a side of a `toggle` picks that side outright, and
      clicking a `form` field moves the form's focus there. A `toggle`
      handles the mouse *before* its focus gate, since clicking an
      unfocused one is how it gets focused.

    `pane.RowAt(x, y)` inverts the border inset and scroll offset for
    components that scroll the pane. Components that window their own
    rows (`table`, `inspector`) map through their own window start
    instead — and must keep that computation in one place, shared with
    rendering, or a click lands on a different row than the one under
    the pointer.

29. **Back a table with a remote source via `pkg/source`, and let
    scrolling be the pagination.** When the rows live behind an API,
    three pieces divide the work and none of them does the others' job:

    - **`pkg/table`** reports and displays. `FilterMode: FilterRemote`
      and `SortMode: SortRemote` stop it answering the filter and sort
      itself; it emits `QueryChangedMsg` instead. `SetWindow(rows,
      offset, total)` makes it sparse — it holds one window, draws
      `Placeholder` elsewhere, and runs the cursor, scrollbar and
      counters against `total`.
    - **`pkg/source`** decides *what to ask for*. It owns offset/limit
      (or the cursor token), the held window, the logical total, and a
      generation per request. It does no I/O, exactly as `pkg/poll`
      touches no data.
    - **Your screen** does the fetch. That is the whole reason the
      coordinator is a separate package: every component here is
      synchronous, and a component owning a context, a retry policy and
      an in-flight request would drag all three into a value-receiver
      `Update`.

    The loop is five lines of routing:

    ```go
    func (s *Screen) Init() tea.Cmd { return s.src.Init() }

    case table.ViewportChangedMsg:
        return s, s.src.Viewport(m.FirstVisible, m.LastVisible)
    case table.QueryChangedMsg:
        return s, s.src.SetQuery(m.Raw, m.Terms, m.Sort, m.Desc)
    case source.RequestMsg:
        return s, s.fetch(m.Query)          // your HTTP call
    case fetchedMsg:
        if !s.src.Deliver(m.page) {
            return s, nil                   // stale; drop the rows
        }
        s.tab.SetWindow(m.rows, m.page.Offset, m.page.Total)
    ```

    `SetWindow` makes the table emit a fresh `ViewportChangedMsg`, which
    is what closes the loop — a short page that still doesn't cover the
    screen asks for the rest by itself, and a covered viewport asks for
    nothing. `Init` exists because an empty component reports no
    viewport, so nothing else would ever request the first page.

    **Check what `Deliver` returns.** Every request carries its own
    generation and only the newest is accepted, which is what stops a
    slow reply to the previous filter painting itself under the current
    one, and stops out-of-order window replies fighting. A screen that
    ignores the bool re-introduces both races.

    Pagination is a wire protocol, not a UI. The user scrolls; windows
    arrive under them. Don't add `n`/`p` page keys — they collide with
    rule 25's reservations and impose a second navigation model on a
    component that already scrolls. See `examples/data/remote`.

32. **For multi-select, set `Options.Markable` and read `Selection()`.**
    `pkg/list`, `pkg/table` and `pkg/tree` carry a marked set the user
    builds with `x`, which toggles the cursor row both ways (`space` also
    marks in list and table; in a tree it stays expand/collapse). `X`
    (shift+x) extends the selection from the anchor — the most recently marked row —
    to the cursor; `A` marks every row the filter currently shows;
    `D` clears the selection outright; clicking the `✓` gutter toggles
    without opening the row. Off by default: no marker column, no
    bindings, no width cost.

    `D` is a separate key rather than a second `A` because
    `A` is a *toggle over the visible rows*: from a partial selection
    it marks the rest, and only a second press clears. "Undo my selection"
    would otherwise route through a state where everything is marked,
    which is the wrong place to pass through on a screen whose next
    keystroke might be a destructive verb.

    `X` spans in **either direction** from the anchor, which stays fixed —
    so repeated ranges grow and shrink against one end rather than walking
    it along. With no usable anchor it marks the cursor row alone and
    anchors there. Ranges are additive: they extend a selection rather
    than replacing it. **Shift+click is the same verb with the mouse.**
    (The range key is `X` — shift+x — and not `shift+space`, because
    terminals do not deliver shift+space distinguishably from space.
    Shift+click may never arrive at all, since some terminals reserve it,
    so it is an accelerator on top of `X` and never the only path.)

    **In `list` and `table`, marking requires keyed data** —
    `SetKeyedItems` / `SetKeyedRows`. Marks are held by key, never by
    index, so a polled refresh (rule 24) that reorders the set between the
    user marking rows and choosing a verb cannot slide the selection onto
    its neighbours. On anonymous `SetItems` / `SetRows` every mark
    operation is a deliberate no-op: an inert feature is recoverable, a
    silently wrong selection is not.

    `pkg/tree` needs no keyed setter, because a node's path already *is*
    its identity — the same path the tree uses for expansion state and
    cursor restore. **Marking a branch marks that node alone**, not its
    descendants. Paths are hierarchical strings, so a caller wanting the
    subtree can prefix-test what it got; the alternative needs a tri-state
    glyph and a rule for children that arrive on a later refresh, and buys
    nothing until something needs it. `A` there means every *visible*
    row — expanded, and surviving the filter — so it never marks what a
    collapsed branch is hiding.

    **Read `Selection()`, not `Marks()`.** It is "the marked keys, or the
    cursor row's key when nothing is marked", which is the branch every
    caller would otherwise write and some would forget — and whose
    failure mode is a verb quietly acting on one row when the user had
    marked six. `SelectionLabel()` names it for a confirm string or a
    menu title ("cache-redis", or "3 items").

    Marks survive filtering, because a key does not care whether its row
    is on screen. So a user can mark a row, filter it away, and still act
    on it — correct, and a genuine surprise, which is why `action.Set`
    puts `Target` on the menu's own border rather than trusting the user
    to remember. Carry them across a `SetTheme` rebuild with `SetMarks`
    the same way you carry the cursor (rule 4).

    A windowed table (`SetWindow`) cannot be marked: it carries rows
    without keys, so a mark there could only be held by index into a
    sparse paged set. Marking is inert there rather than approximate.
    `pkg/inspector` has no marking — it is a record viewer, and there is
    no verb that acts on a set of its fields.

    See `examples/data/multiselect` and rule 8: the verbs that act on a
    selection belong in an `action.Set`, not on letter keys. (Rules 30 and
    31 are reserved by `docs/actions.md` for `pkg/action` and the reserved-
    key table, both shipped but not yet written up here.)

## Anti-patterns

- **Don't wire breadcrumb + statusbar by hand when you can use `pkg/app`.**
  The shell owns them, including theme-swap rebuilds and re-placing them
  on resize. It also owns mouse: enabling reporting, resolving double
  clicks, and routing clicks on its own chrome (crumbs, the `? help`
  affordance). The only reason to skip it is if you need something the
  shell's shape doesn't support.
- **Don't write `m.h - 2` / `m.h - 5` math inside a screen's `Layout()`.**
  Use `Fixed`/`Flex` siblings. The body gets whatever's left; it doesn't
  need to know about sibling sizes.
- **Don't set `Width`/`Height` on a component you're wrapping in
  `layout.Sized`.** The engine will overwrite it — passing literal sizes
  just misleads the reader.
- **Don't handle `q`, `t`, or esc-pop inside a screen** when running under
  `pkg/app`. The shell routes those. Return them from `Help()` so they
  appear in the hints, but don't re-implement them.
- **Don't validate a form outside the form.** `pkg/form` owns it:
  `TextOptions.Required` for emptiness, `Validate func(any) error` on any
  field for everything else, and `SelectOptions.RequirePick` for "must be
  chosen deliberately" (a select always has a cursor, so required means
  something different there). Required is checked first, so a blank field
  reports "required" rather than whatever a format rule makes of `""`.
  Validation runs **on submit**, then live for fields already flagged —
  nothing nags mid-typing, but a correction is acknowledged at once. An
  invalid submit is *refused*: no `SubmittedMsg`, focus moves to the first
  offender, and `InvalidMsg{Keys}` lets the screen react optionally. A
  screen can therefore never act on bad data by forgetting to check.
  Errors render on the field's own border (tint + a short message in the
  top-right slot) so nothing reflows — see rule 9's note on why layout
  must not shift while the user is correcting it. Required fields carry a
  `*` on their label, which is a standing property distinct from the
  border, which is a complaint. Cross-field rules (password confirmation,
  date ranges) are not supported: they have no field to attach to.
- **Don't instantiate `textinput.New()` directly.** Use `input.Model` for a
  bare bordered text field, `filter.Model` for the "/-to-focus, enter-commits"
  pattern, or `list.Model` with `Filterable=true` for a filtered list.
- **Don't double-wrap a component in another `pane.Pane`.** Every
  interactive component already owns one. If you find yourself writing
  `pane.New(…).SetContent(list.View())`, set the component's `Title` instead
  and place it directly via `layout.Sized(&c)`.
- **Don't render a label line above an input/toggle/list/filter.** The
  component's `Title` field renders the label on the border itself. The old
  inline-label pattern is gone.
- **Don't use `bubbles/table` directly.** Use `pkg/table` — it owns the
  pane, has ANSI-aware cell truncation (so `Column.Width` is the visible
  width, no escape-byte padding), pins the header at line 0 while still
  scrolling horizontally with the body, and mirrors `pkg/list`'s
  ergonomics (cursor, filterable, `g`/`G`/`ctrl+u/d` nav, `SetLoading`,
  `SetTheme`-friendly setters). See `examples/data/table/table.go`.
- **Don't roll your own confirm modal.** `pkg/confirm` already handles
  selection movement, y/n/esc shortcuts, message-driven results, and
  theme-aware styling. Hand-rolling a `pane.Pane` + `toggle.Model` +
  ZStack tree skips theme swap robustness and the typed result messages
  the rest of the codebase expects.
- **Don't roll your own alert modal.** `pkg/alert` covers the
  acknowledgement case (single OK button, `DismissedMsg`); use it for
  blocking errors and notices. For passive feedback prefer
  `app.Info` / `app.Error` (rule 20) — modals break flow, statusbar
  doesn't.
- **Don't roll your own log viewer.** Use `pkg/logview` for any append-
  mostly text stream that needs search / jump / filter / auto-follow.
  Wrapping `viewport.Model` directly skips the search highlight, current-
  line indicator, and `MaxLines` cap that logview already gets right.
- **Don't use `pkg/logview` for static text.** Reach for `pkg/textview`
  when the payload is a document rather than a stream — a rendered
  README, a diff, a kubectl describe, an API response body. textview
  drops logview's streaming machinery (follow / filter mode / MaxLines)
  and keeps only what read-static-text needs: scroll + search + wrap
  toggle. `SetContent(s)` replaces the buffer and resets scroll to the
  top. Same key vocabulary (`/` search, `n`/`N` step, `g`/`G` bounds,
  `w` toggles wrap) so users don't relearn per component.
- **Don't roll your own tree viewer.** Use `pkg/tree` for any
  hierarchical data the user needs to expand/collapse and search. Provide
  a `Node` (Label + Children) over your own data shape — don't force
  tuilib's structs into your domain model. Pre-flattening into a list +
  manual indentation strings re-implements expand/collapse + filter-with-
  ancestors badly.
- **Don't call `tea.ExecProcess` directly for subprocesses.** Use
  `runner.Run(*exec.Cmd)` — it sets `Stdin/Stdout/Stderr` + `LINES`/
  `COLUMNS` and returns a typed `runner.Result` your `Update` can match
  on. Bypassing it loses those defenses and the consistent result type.
- **Don't hand-roll the `io.Pipe` + goroutine + `bufio.Scanner` dance.**
  `runner.Capture` is that pattern, moved out of an example and into the
  library, with the app shell already chaining the reads. Rolling it again
  per command gets you a second subprocess pipeline that the console can't
  see. `examples/data/runlog` still shows the manual form because it
  predates `Capture`; it is not the recommended path any more.
- **Don't use the statusbar as a log.** Its center slot is one truncated
  line and it is wiped by the next `tea.KeyMsg`. If the user might want to
  read it twice, it belongs in `pkg/output` — `app.ErrorDetail` for a
  summary plus a body, `app.ErrorOf` for a `%w` chain. Cramming a stack
  trace into `app.Error` produces exactly the sliver-of-footer problem the
  console exists to fix.
- **Don't paginate by swapping `SetRows` per page.** That is a page
  control bolted onto a component that already scrolls: two navigation
  models, `n`/`p` colliding with keys rule 25 reserves, and a cursor that
  resets to the top on every page. `SetWindow` makes scrolling *be* the
  pagination — the wire protocol stays offset/limit, but the user just
  scrolls and windows arrive under them. Discrete pages are the special
  case (a window that never prefetches), not the default.
- **Don't read a placeholder row as data.** Under `SetWindow`, `Selected`
  returns `ok=false` for an index the window doesn't hold, and
  double-click emits no `ActivatedMsg` there — check the bool rather than
  indexing the row, or a drilldown will happily open `·`. `RowFocusedMsg`
  reports `Empty` for the same reason: nothing is focused until the row
  actually arrives.
- **Don't give a windowed table content-auto column widths.** Widths are
  computed from the resident window, so every swap reflows the columns
  under the user. Use fixed `Width` or `Flex` when the rows are paged in.
- **Don't scrape filter completions from a remote table's rows.** Under
  `FilterRemote` the rows on screen are one page of a larger set, so
  values scraped from them complete to answers that are *wrong*, not
  merely incomplete — the user tabs to `region:eu` and gets `europe`
  because that page happened to hold no `eu-west`. The table stops
  scraping in remote mode for exactly this reason; feed `SetDistinct`
  from whatever the source can actually enumerate, or accept no hints.
- **Don't emit a query per keystroke.** A remote filter that fires on
  every character is a request storm, and the responses race — the reply
  to `eu` can land after the reply to `euro`. `pkg/table` reports on
  commit, and a screen driving its own filter should debounce or wait
  for enter rather than reacting to every `Value()` change.
- **Don't let a state-restoration setter look like a user action.**
  `SetSort` / `SetValue` exist so a `SetTheme` rebuild can replay state
  onto a fresh model (rule 4); if they emitted `QueryChangedMsg` every
  theme swap would refetch. They adopt their new state silently — call
  `Query()` yourself when you set a filter or sort programmatically and
  do want it fetched.
- **Don't treat stderr as an error level.** Plenty of well-behaved tools
  write progress there (`go build`, `git`, `curl`), so mapping stderr onto
  `LevelError` leaves the console badge permanently red and the tint stops
  meaning anything. Severity comes from the run's exit status;
  `Record.Stderr` carries the stream distinction on its own.
- **Don't forget the `OutputClosed` guard in `OnEnter`.** A screen that
  fetches on activation will silently refetch every time the user glances
  at the console, because `screen.Pop` fires `OnEnter` either way and
  nothing else distinguishes the two. It is one branch (rule 14) and the
  symptom — an invisible, possibly slow, possibly billable refresh — is
  the kind you find in a bill rather than in a test.
- **Don't leave a streaming buffer uncapped.** `pkg/logview` defaults to
  `DefaultMaxLines = 10000`. Override only when you have a real reason —
  passing `-1` opts out of the cap entirely and makes the buffer grow
  with the stream.
- **Don't pre-wrap content for the pane.** Pane already truncates lines
  to its inner width (so terminal wrap can't break row counting) and
  offers horizontal scroll. Pre-wrapping with `wordwrap.WrapString` or
  manual `\n` insertion just makes content harder to read at narrow
  widths. Pre-wrap only when the content is genuinely paragraph prose.
- **Don't set colors in `Options` literals.** Start from the theme builder.
- **Don't skip state preservation in `SetTheme`.** If you forget to carry
  cursor/value across rebuilds, theme-swap will silently reset the user's
  state.
- **Don't write per-component reset codes.** If bar colors drift between
  embedded segments, the fix is usually "make sure every embedded style
  sets the same `Background()`," not a manual `\x1b[0m`.
- **Don't branch on `Marks()` vs. the cursor by hand.** Writing
  `if ks := t.Marks(); len(ks) > 0 { … } else { … }` at every call site is
  easy to get right once and easy to forget the second time, and what a
  forgotten branch does is act on one row when the user marked six.
  `Selection()` is that branch, already written. Reach for `Marks()` only
  when you specifically need "marked, and not the cursor fallback".
- **Don't offer marking on anonymous rows.** `Options.Markable` with
  `SetRows` / `SetItems` renders a gutter the user can click and a `space`
  binding that does nothing, because marks need keys. Either key the data
  (`SetKeyedRows` / `SetKeyedItems`) or leave `Markable` off — a visible
  affordance that silently no-ops reads as a broken component.
- **Don't hand-roll a focus index.** `focus.Group` (rule 27) owns
  cycling, the blur-all-focus-one dance, click-to-focus grants, and
  `IsCapturingKeys` delegation. An int plus an `applyFocus` helper gets
  the first of those right and silently misses the rest — most visibly,
  a clicked component has no way to tell the screen it wants focus.
- **Don't reach for bubblezone or any marker-injection scheme.** Markers
  encode position *inside the rendered string*, and this library rewrites
  strings freely — `pane` truncates with `ansi.Cut`, `table` cuts cells,
  `ZStack` splices three fragments per row. `layout` already computes
  every rect; use it (rule 28). ZStack in particular duplicates any
  escape it carries across the splice, so a marked region on a row
  crossed by a modal registers twice and the wrong copy wins.
- **Don't compute a component's scroll window in two places.** If
  rendering derives the visible range one way and hit-testing derives it
  another, a click resolves to a different row than the one under the
  pointer — and it only shows up once the content is long enough to
  scroll. Extract the decision (`viewStart()`, `elideStart()`) and call
  it from both.
- **Don't scroll a cursor-owning component by moving its viewport.**
  `refresh` re-asserts "the cursor is visible" and `layout.Sized` calls
  `SetRect` — hence `refresh` — on every render, so a viewport moved on
  its own is undone one frame later and the view snaps back to wherever
  the cursor still was. Move the cursor as well: `list`, `table`, `tree`
  and `inspector` all expose `scrollTo(row)`, which sets the cursor *and*
  the window start so the next frame's cursor-visible pass is a no-op.
  The wheel already worked this way (rule 25); the scrollbar had to
  follow.
- **Don't let an event that ends a gesture report a position.** A mouse
  release finishes a scrollbar drag; it does not move anything.
  `pane.HandleScrollbar` returns the *current* offset on release rather
  than zero, because a caller that scrolls to the reported row would
  slam back to the top the instant the button came up.
- **Don't feed `focus.Group` only the keys it cycles on.** It needs
  *every* message (rule 27), because click-to-focus works by a
  `focus.RequestMsg` the clicked component sends back — a screen that
  calls `Group.Update` inside its tab branch alone silently drops those.
  The symptom is nasty: the clicked pane lights up, because the component
  sets its own border directly, while every keystroke keeps going to the
  pane focus never left. Typing into a filter and watching the characters
  appear in a different pane is what this looks like.
- **Don't route mouse events to the focused component only.** Keys go to
  the focused one; mouse goes to *all* of them (rule 6). This is the
  single easiest thing to get wrong when writing a screen, because the
  keyboard version is right there and looks symmetric — it was wrong in
  three shipped examples after the rule was already written down. The
  symptom is misleading: the component under the pointer never receives
  the press, so it can't take focus or release its filter, and it reads
  as a broken component rather than a routing mistake. Documentation did
  not prevent this; `internal/componenttest/screenrouting_test.go` does,
  by driving real screens and asserting a press somewhere lands.
- **Don't test shared behaviour in one component's package.** If
  something must hold for every filterable component, assert it once in
  `internal/componenttest` and run it across all of them. Testing the
  component you just edited proves only that you edited it: the
  click-to-blur behaviour was hand-written for `pkg/list`, rolled out to
  the other five by a script that omitted it, and covered by a test that
  lived in `pkg/list` — so five components shipped broken and the suite
  stayed green.
- **Force a colour profile in any test that asserts on styling.**
  Without a TTY lipgloss falls back to the Ascii profile and strips every
  style, so a render comparison silently passes no matter what the code
  does. `lipgloss.SetColorProfile(termenv.TrueColor)` in TestMain.
- **Don't add a comment explaining what well-named code does.** Component
  doc comments belong at the package and exported-symbol level; inline
  code should be self-describing.

## Layout primitives cheat sheet

```go
layout.VStack(                                   // stack children top-to-bottom
    layout.Fixed(1,  layout.Bar(&m.breadcrumb)), // 1 row, full width
    layout.Flex(1,   layout.HStack(              // middle takes the rest
        layout.Fixed(24, layout.Sized(&m.side)), // 24 cols
        layout.Flex(1,   layout.Sized(&m.body)), // whatever's left
    )),
    layout.Fixed(1,  layout.Bar(&m.statusbar)),
)
```

- `Fixed(n, node)` reserves exactly `n` cells on the main axis.
- `Flex(weight, node)` takes a share of what's left; sibling weights set
  the ratio.
- `Sized(&c)` adapts any `SetRect(geom.Rect) + View()` component.
- `Bar(&c)` is `Sized` under a name that says "this renders one row".
- `RenderFunc(func(r geom.Rect) string)` — escape hatch; render inline at
  `r.W` × `r.H`.
- `ZStack(base, overlay)` composites overlay over base. Both layers get
  the same rect; occluding the base is the host screen's job (rule 22).
- `Center(w, h, node)` renders `node` at a fixed size, centered in the
  parent's rect — the standard modal pattern (use inside a `ZStack`).
  The child's rect is offset to where it is actually drawn, so a
  centered modal hit-tests against its visible position.

A `geom.Rect` is `{X, Y, W, H, Gen}`: absolute terminal position, size,
and the render generation it was stamped in. The root of a render calls
`geom.NextGen()` once per frame and seeds its rect with `geom.New`;
children inherit. Use `Rect.Hit(x, y)` rather than `Contains` for click
routing — it also rejects a rect from an earlier frame, which is what
stops an undrawn component claiming a click.

## Manual layout reference (only when not using `pkg/app`)

If you genuinely can't use the app shell, here are the row costs. Size
components with `SetRect(geom.Rect{W: w, H: h})`; a zero X/Y is fine when
you aren't hit-testing, and mouse support needs the app shell anyway
(it's what translates `tea.MouseMsg` into `mouse.Msg`).

| Component | Rows consumed |
|---|---|
| `breadcrumb.Model` | 1 |
| `statusbar.Model` | 1 |
| `filter.Model` | 3 (border + content + border) |
| `pane.Pane` | caller-controlled, min 3 (4 when `HScrollbar=true` — one inner row reserved for the bar) |
| `list.Model` (Filterable=false) | caller-controlled |
| `list.Model` (Filterable=true) | caller-controlled; the filter is 2 inner rows (row + rule) inside the same pane |
| `table.Model` (Filterable=false) | caller-controlled, all body (header consumes 1 inner row, leaving `VisibleRows()-1` data rows) |
| `table.Model` (Filterable=true) | caller-controlled, the filter is 2 inner rows inside the same pane (then the column header consumes 1) |
| `logview.Model` (Searchable=false) | caller-controlled, all body |
| `logview.Model` (Searchable=true) | caller-controlled, the filter is 2 inner rows (row + rule) inside the same pane |
| `textview.Model` (Searchable=false) | caller-controlled, all body |
| `textview.Model` (Searchable=true) | caller-controlled, the filter is 2 inner rows (row + rule) inside the same pane |
| `tree.Model` (Searchable=false) | caller-controlled, all body |
| `tree.Model` (Searchable=true) | caller-controlled, the filter is 2 inner rows (row + rule) inside the same pane |
| `inspector.Model` (Filterable=false) | caller-controlled, all body |
| `inspector.Model` (Filterable=true) | caller-controlled, the filter is 2 inner rows (row + rule) inside the same pane |

Typical body height:
- Plain body pane: `m.h - 2`
- Any filterable component: `m.h - 2` — the filter is inside the pane, so
  there is no sibling to subtract for.

Prefer `pkg/layout` — this table exists for edge cases, not as the default
path.

## Where to learn more

- **Run the launcher:** `task examples`. Every demo is hosted there as a
  child screen. For code, each example lives at `examples/<area>/<name>/<name>.go`
  as a package exposing `New(theme.Theme) screen.Screen`.
- **Closest examples first:** `examples/app/stack/stack.go` for nav + data
  flow, `examples/app/layouts/layouts.go` for layout primitives across
  five sub-screens. Copy one and strip what you don't need.
- **Launcher pattern:** `examples/launcher/main.go` shows how to compose
  multiple screens into a single app — a filterable menu pushing the
  selected example onto the stack.
- **Package overview:** `go doc ./pkg/<name>` prints the package doc
  comment + every exported symbol's signature and doc.
- **Full config surface:** `go doc ./pkg/<name>.Options`.
- **Color vocabulary:** `pkg/theme/theme.go` — field comments on the
  `Theme` struct name every semantic slot.
- **User-default theme:** `app.New` runs `theme.Resolve` on `Options.Themes`
  by default, picking the initial theme from (1) `Options.ThemeEnvVar` if
  set, (2) `~/.config/tuilib/config.yaml`'s `theme:` field, (3) `Themes[0]`.
  Pass the raw theme list — the shell reorders it. Set `Options.SkipConfig
  = true` to disable resolution (tests, or apps pinned to a single theme).
  Config is opt-in: the library never writes the file. Unknown names fall
  through silently; malformed YAML surfaces only via `config.Load`
  directly. `theme.Resolve` is still exported for callers that need the
  resolved order outside `app.New`.
- **User config in general:** `pkg/config` owns the YAML file shape
  (`Config`, `Path`, `Load`, `LoadFrom`). Cross-component knobs go here
  as fields on `Config`. Other packages should import `pkg/config`
  (not `pkg/theme`) when they grow their own user-tunable defaults.
  Today it carries `theme:` and `double_click_ms:`.
- **Geometry:** `pkg/geom` is a leaf holding `Rect{X,Y,W,H,Gen}` plus
  `Contains` / `Hit` / `Fresh` / `Inset` / `CenterIn`, and the
  render-generation counter (`NextGen`, `Gen`). Both `pkg/layout` and
  every component depend on it; it depends on nothing in tuilib, which
  is why it exists as its own package rather than living in `layout`.
  `CenterIn` mirrors `lipgloss.Place`'s centering exactly, so a
  component that draws itself centered can hit-test where it landed.
- **Filter grammar:** `pkg/query` is the second leaf — `Term`, `Parse`,
  `ColumnByPrefix`, `Match`/`MatchAll`, plus the completion half
  (`Distinct`, `ActiveTerm`, `Candidates`, `Complete`). It owns the
  "bare substring / `key:value` column scope / `~regex`" syntax
  `pkg/table`'s filter bar exposes, and it imports nothing from tuilib
  for the same reason `pkg/geom` doesn't: the component that applies a
  filter to rows it holds and a coordinator that turns the same filter
  into a remote request need the identical parse, and neither should
  import the other to get it. `Term.Title` carries the resolved column
  title (not the prefix the user typed) so a scoped term maps onto a
  query parameter directly. `Parse` never fails — an unresolvable key or
  an uncompilable regex degrades to a literal substring, so the filter
  bar never refuses input. Feeding `Distinct` values scraped from one
  page of a paged source produces completions that are *wrong* rather
  than merely incomplete; a remote caller should pass facet values
  instead.
- **Remote paging:** `pkg/source` is the coordinator between "the user
  scrolled here" and "ask the server for that range" — `Query`,
  `RequestMsg`, `Page`, and a `Model` that owns the held window, the
  logical total, and a generation per request. `ByOffset` (default)
  addresses rows numerically and can jump anywhere; `ByCursor` walks an
  opaque continuation token forward only, accumulating into one growing
  window at offset 0. `Options.PageSize` sets the window size requests
  align to (so scrolling within a page asks for nothing); `Prefetch`
  pulls extra pages ahead of the screen to hide the placeholder flash at
  boundaries. It imports `pkg/query` and nothing else from tuilib —
  deliberately not `pkg/table`, so the dependency points one way and the
  screen does the translating. See rule 29 and `examples/data/remote`.
- **Focus composition:** `pkg/focus` is `Group` (ordered focusables,
  cycling, click grants), the `Focusable` interface every component
  satisfies, and the optional `Capturer` that answers rule 5. Components
  identify themselves in focus requests by `Token` rather than by
  pointer — bubbletea's value receiver on `Update` means a component
  cannot name its own address. See rule 27 and `examples/app/focus`.
- **Mouse:** `pkg/mouse` is the `Msg` components actually handle
  (bubbletea's event plus a resolved `Clicks` count) and the `Tracker`
  that counts rapid repeat presses in the same cell. The app shell owns
  one tracker, so no component carries mouse state or a threshold. See
  rule 28 and `examples/app/mouse`; run the launcher to try it, since
  `app.Options.Mouse` is set there for the whole suite.
- **Help footer + key overlay:** the statusbar's left slot shows a
  `? help` affordance (and, with `app.Options.HelpVerbose`, as many of
  the active screen's `Help()` bindings as fit inline before it).
  Pressing `?` — configurable via `app.Options.HelpKey`, and clickable —
  opens `help.Overlay`: a bordered modal listing every binding the
  screen exposes, **grouped into sections**, scrollable, and searchable
  (`/`, on by default; `app.Options.DisableHelpSearch` turns it off).
  Esc, `q`, `?` again, or a click outside close it.

  The grouping is where the value is, and it is **by function, not by
  owner** (rule 10): Navigate, Scroll, Sort, Filter, Select. The shell
  writes a **Global** group itself — quit or esc-back depending on stack
  depth, theme, suspend, and the opt-in output and actions keys a screen
  has no reason to know about — then each component contributes its own
  groups, qualified by pane ("files · Navigate") only when more than one
  pane is on screen. `help.Suppress` drops from a screen's groups
  anything the shell's globals already claim, so a screen that lists `q`
  and `t` in its own `Help()` is not punished for it; repeats *across*
  panes are kept, because two panes binding `↑/k` are two different
  verbs, not one duplicate.

  The list runs top to bottom in one column and scrolls when it
  overflows, so a group keeps a fixed place instead of moving between
  columns as the terminal changes width.
  A screen that doesn't implement `help.Sectioned` still works: it gets
  one group titled with its own name, which is the flat list the footer
  would have shown.

  The affordance is the source of truth for whether `?` does anything:
  no affordance, no overlay. Screens contribute by returning the right
  bindings from `Help()` (rule 10) and, when they have more than one
  component, by grouping them. See `pkg/help` for both renderers and
  `pkg/app` for the wiring.
- **Statusbar messages from a screen:** `app.Info(s)` / `app.Error(s)` /
  `app.ClearStatus()` return `tea.Cmd`s that the shell intercepts and
  paints into the statusbar's center slot. Auto-clears on the next
  `tea.KeyMsg`. See `examples/app/status` and rule 20.
- **Confirm modal:** `pkg/confirm` is a yes/no dialog meant to live in a
  ZStack overlay. Resolves via `confirm.ConfirmedMsg` / `confirm.CancelledMsg`
  as `tea.Cmd`s the parent matches in its own `Update`. See
  `examples/data/confirm` and rule 22.
- **Alert modal:** `pkg/alert` is the acknowledgement counterpart to
  confirm — a single OK button, `alert.DismissedMsg` result, identical
  hosting pattern. Override `ActiveColor` with `t.ErrorBG` for an
  error-tinted look. Use it for "stop and acknowledge" feedback; prefer
  `app.Info` / `app.Error` for passive notices. See `examples/data/alert`
  and rule 23.
- **Atomic screen swap:** `screen.Replace(s)` swaps the active top of the
  stack in one tick. Use it for "fresh instance of this view" (reset
  filter, reset scroll, refetch from scratch) — pop+push flickers and
  fires `OnEnter` on the parent below, which is wrong for a self-refresh.
  Pass the new screen with its theme already applied, same as Push.
- **List nav keys:** `pkg/list` handles `↑↓`/`j`/`k` per row, `g`/`G` for
  jump-to-top/bottom, and `ctrl+u`/`ctrl+d` for half-page jumps. Half-page
  is half the pane's `VisibleRows()` (floor 1) so it always moves at least
  one row even on tiny panes. The keys are gated on `Filtering()`, so
  typing `G` into the filter doesn't trigger the jump. Long rows scroll
  horizontally with `←→` / `h` / `l` when `HScrollbar` is enabled (default
  via `theme.List()`).
- **Multi-select (marking):** `pkg/list/mark.go`, `pkg/table/mark.go` and
  `pkg/tree/mark.go` —
  `Options.Markable`, `space` / `A`, `Marks` / `SetMarks` /
  `ClearMarks` / `MarkCount`, and `Selection` / `SelectionLabel`, which
  are the two a screen actually calls. Held by key, so marks survive a
  filter, a keyed swap and a theme rebuild; inert on anonymous data and
  under `SetWindow`. The contract is asserted once across both components
  in `internal/componenttest/marking_test.go` rather than in any one
  package, for the reason the "don't test shared behaviour in one
  component's package" anti-pattern gives. See rule 32,
  `examples/data/multiselect`, and `docs/actions.md` decisions 19-20.
- **Keyed items / rows:** `pkg/list` (`SetKeyedItems` + `KeyedItem{Key,
  Display}` + `SelectedKey`) and `pkg/table` (`SetKeyedRows` +
  `KeyedRow{Key, Cells}` + `SelectedKey`) are the auto-refresh primitive.
  When the data backing a list/table changes over time, swap rows via
  the keyed setter so the cursor snaps to the same Key after the swap;
  the previous-cursor index is the fallback only when the key is gone.
  `SetItems`/`SetRows` clear any keys, so reach for the keyed variant
  consistently across a screen — mixing them resets the keys mid-flight.
  Pair with `pkg/poll` for the cadence; see `examples/data/poll`.
- **Table component:** `pkg/table` is the cursor-driven tabular companion
  to `pkg/list`. `Column{Title, Width, Align, Sortable, Less, Flex,
  MaxWidth}` declares the layout; rows are `[]string` cells. The
  header pins to the top of the body (it does not scroll out as the
  cursor moves down) but scrolls horizontally with the body so column
  titles stay aligned with their data. Cell truncation is ANSI-aware
  via `x/ansi.Cut`, so `Width` is the visible cell width. Sizing
  modes: `Width > 0, Flex == 0` → fixed; `Width == 0, Flex == 0` →
  content-auto (sized to max of title and any cell value, ANSI
  stripped, floor 4); `Flex > 0` → column absorbs a share of leftover
  inner width by Flex weight after every column's base is accounted
  for (Width acts as a minimum). `MaxWidth` (when > 0) caps any
  column's effective width — when a flex column hits its cap, the
  surplus redistributes to remaining uncapped flex columns; when all
  flex columns are capped, leftover space stays unused on the right
  edge. Effective widths recompute on
  `SetRows`/`SetKeyedRows`/`SetColumns`/`SetRect`, so flex
  columns reflow on resize and content-auto re-fits when data swaps.
  When the table is narrower than the sum of base widths, flex
  columns don't grow — overflow goes to h-scroll, never to a column
  reflow surprise. Same nav verbs as list (`g`/`G`, `ctrl+u/d`,
  `↑↓`/`j`/`k`), same `Filterable` story (filter matches
  case-insensitively across all cells, with ANSI stripped before
  matching). Filter input is split on whitespace into AND-ed terms; a
  term shaped `key:value` scopes the match to the column whose Title
  case-insensitively starts with `key` (e.g. `region:europe pop:5`).
  Bare terms still match any cell, so existing single-word filters keep
  working. An ambiguous or unknown `key` falls through as a literal
  bare term, which is also how you'd search for a literal colon. A term
  whose value starts with `~` is compiled as a case-insensitive Go regex
  (`~^new`, `region:~^euro`); compile errors fall back to a literal
  substring including the tilde, so the parser never refuses input.
  While the user is mid-typing a `key:val` term, the filter pane's
  bottom-left slot lists the column's distinct values matching `val` so
  they don't have to guess what's there; `tab` completes `val` to the
  longest common prefix of the remaining candidates (regex terms skip
  the hint — enumerating regex matches isn't useful). Same `SetLoading`
  / `SetTheme` ergonomics. Set
  `Column.Sortable = true` to opt a column into sort: `[`/`]` step the
  active sort column among Sortable columns and `s` toggles direction
  (asc/desc indicator ▲/▼ renders after the active column's title).
  Default comparator is case-insensitive on the ANSI-stripped text;
  override with `Column.Less func(a, b string) bool` for numeric, date,
  or unit-aware columns ("8.3M") — see the table example's `popLess`.
  Carry `SortColumn()`/`SortDescending()` across `SetTheme` rebuilds via
  `SetSort(col, desc)` the same way you carry `Cursor()`/`Value()`. For
  colored cells inside a row, prefer `pkg/ansi.CellColor` over
  `lipgloss.Render` so the selected-row background passes through
  unbroken (rule 19). Internal separators are configurable via
  `Options.Borders{Vertical, HeaderRule}` — both fields are pre-styled
  glyph strings (use `pkg/ansi.CellColor` so the selected-row bg passes
  through). `Vertical` replaces the inter-column space with `" <glyph> "`;
  `HeaderRule` repeats its first visible rune as a horizontal rule
  between header and data rows. `theme.Table()` ships with subdued
  light-line defaults (`│` and `─` in palette index 240); override per
  screen for a different glyph or color, or set fields to `""` to
  disable. `Options.FilterMode` / `Options.SortMode` decide who answers
  the filter and the sort: the defaults (`FilterLocal` / `SortLocal`)
  keep today's behavior, while `FilterRemote` / `SortRemote` make the
  table display its rows exactly as given and emit `QueryChangedMsg`
  instead. The two are independent — sort remotely while filtering the
  page you hold, or the reverse. The message carries the raw text, the
  parsed `[]query.Term` (each scoped term already resolved to its column
  Title, so `?region=europe` needs no lookup), and the active sort;
  `Query()` returns the same value on demand, which is how the first
  fetch runs through the same path as every later change. Filters report
  on **commit** — enter, esc, or losing focus — never per keystroke, and
  tab completion is silent because it edits a term rather than
  submitting one. `SetDistinct(col, values)` feeds completion candidates
  from a facet endpoint, since remote mode deliberately stops scraping
  them from resident rows. `SetWindow(rows, offset, total)` makes the
  table sparse: it holds the logical indices `[offset, offset+len(rows))`
  of a set `total` rows long, while the cursor, the scrollbar and the
  counters all work against `total`. Pass `total < 0` when the source
  can't say (cursor-paginated APIs) — the counter then reads `20+` and
  grows as pages land. Indices outside the window render as
  `Options.Placeholder` (default `·`) and report `ok=false` from
  `Selected`, so scrolling ahead of the data shows filler instead of
  wrong rows and a screen cannot act on one it never received. The cursor
  is a logical index and does not move when a window arrives, so
  scrolling to row 800 and waiting leaves you on row 800.
  `Window()` reports `(offset, count, total)`; `ViewportChangedMsg`
  reports the logical range on screen, which is the signal to fetch.
  `SetRows` / `SetKeyedRows` leave windowed mode. See
  `examples/data/table` and `theme.Table()`.
- **TextView component:** `pkg/textview` is the read-static-text
  counterpart to `pkg/logview`. Feed it a document via `Options.Content`
  (or `SetContent(s)` at runtime — replaces + resets scroll to top).
  `Wrap` (default true) word-wraps against the pane's inner width via
  ANSI-aware `x/ansi.Wrap`; toggle at runtime with `w`. When wrap is
  off, the pane's built-in truncation + horizontal scroll takes over
  (rule 18). `Searchable=true` embeds the same filter pattern as
  logview: `/` focuses, typing highlights case-insensitive substring
  matches, enter blurs (query stays for `n`/`N` stepping), esc clears.
  No follow, no filter mode, no `MaxLines` — for the streaming case
  reach for `pkg/logview`. Same nav vocabulary as list/table/logview
  (`g`/`G` bounds, `ctrl+u`/`ctrl+d` half-page, `↑↓`/`j`/`k` line —
  rule 25). Carry `Content()`/`Query()`/`Wrap()` across `SetTheme`
  rebuilds via `SetContent`/`SetQuery`/`SetWrap` — the theme swap
  pattern from rule 4. See `examples/data/textview` and `theme.TextView()`.
- **Inspector component:** `pkg/inspector` is a two-column label/value
  viewer for structured records — k8s manifests, REST responses, Prefect
  run details. `Field{Label, Value, Children}` composes fields by
  value; nested objects/arrays render below their parent with indent +
  ▸/▾ expand glyphs. Sibling labels at the same parent auto-align so
  key:value rows read in a clean column. `inspector.FromMap` /
  `inspector.FromAny` convert `json.Unmarshal` output
  (`map[string]any` / `[]any` / scalars) into `[]Field` so REST and
  kubectl-style payloads land directly without a hand-written
  converter. Same nav verbs as `pkg/tree` (`g`/`G`, `ctrl+u`/`ctrl+d`,
  `↑↓`/`j`/`k`, `space` to toggle the cursor row, `E`/`C` to
  expand/collapse all — arrows + hjkl are reserved for scroll per
  rule 25), same `Filterable` story (`/` searches both Label and Value;
  `\` toggles filter mode that hides non-matching subtrees while
  keeping ancestors visible; `n`/`N` step matches). `SetFields` swaps
  the record while preserving expansion state by row path and pinning
  the cursor to its previous path when it survives the swap — the
  primitive that auto-refresh will lean on. Carry
  `Cursor()`/`Query()` across `SetTheme` rebuilds via
  `SetCursor`/`SetQuery`. See `examples/data/inspector` and
  `theme.Inspector()`.
- **Inline metrics:** `pkg/metrics` is a small set of cell-fitting renderers
  for the monitoring shape — `Badge(ok, warn, down)` for status-count
  summaries ("12✓ 3⚠ 1✗"), `Ratio(done, total)` for severity-colored
  "N/M" indicators ("6/6" green, "3/4" yellow, "0/4" red), `Bar(value,
  max, width)` for fixed-width progress bars, `Spark(values, width)` for
  8-step block sparklines that resample to fit. All four return ANSI
  foreground-only strings via `pkg/ansi.CellColor`, so embedding them in
  a `pkg/table` cell keeps the selected-row background intact (rule 19).
  The package is rendering-only — callers own any history buffer (e.g.
  for `Spark`, append each tick's value and trim to a fixed window). For
  non-severity colorization (e.g. CPU usage that should always be blue
  when low and only flush red at saturation), use `BarStyled` /
  `SparkStyled` with an explicit ANSI palette index. See
  `examples/data/metrics` and rule 24 (auto-refresh).
- **Poll component:** `pkg/poll` is a thin interval ticker for screens
  that auto-refresh remote state. Construct with `poll.New(poll.Options
  {Interval: d})`, batch `m.poll.Init()` into the screen's Init, and
  forward every `tea.Msg` to `m.poll.Update`. When the interval elapses
  the model emits `poll.RefreshMsg` (and re-arms the next tick); your
  screen matches that and runs the fetch, then calls `MarkRefreshed()`
  once the data lands so `LastRefresh()` reflects only successful
  refreshes. `Pause`/`Resume`/`SetInterval`/`Refresh` all return
  `tea.Cmd`s and bump an internal generation so any in-flight tick from
  the prior cadence is dropped on arrival — your screen never sees a
  stale RefreshMsg. Pair with the keyed-row APIs on `pkg/list` /
  `pkg/table` (or path-keyed `SetFields` on `pkg/inspector`) so cursor
  + expansion state survive every swap. See `examples/data/poll` and
  rule 24.
- **Output console:** `pkg/output` is the app-wide log — `Record` (one
  flat line's worth of structure: time, level, source, text, head/body,
  stderr, run id), `Buffer` (the ring, the unread accounting, the
  in-flight run registry), and `Screen` (a logview over the buffer with
  clear / kill / export). Turn it on with `app.Options.OutputKey`; there
  is no default key, because the shell claiming a letter takes it from
  every component in every downstream app.

  Two things about it are worth knowing before reading the code. Records
  are rendered at view time rather than stored pre-rendered, so a theme
  swap re-colors the whole log instead of leaving a stratigraphy of old
  palettes (rule 4). And trimming is event-aware: whole events go off the
  front, so a surviving body line always still has the head naming the
  command it came from — which is what makes logview's filter mode usable
  on it.

  Options come from `output.OptionsFrom(t)` rather than a `theme.Output()`
  method, the one place the library breaks the `th.Component()` convention
  of rule 3. `Screen` implements `screen.Screen`, `pkg/screen` imports
  `pkg/theme` for `SetTheme`, so a `Theme` method returning
  `output.Options` would close an import cycle. Inverting the dependency
  is what keeps the screen in its own package and testable without an app
  shell. See `examples/app/output` and rules 14 and 15.
- **Capturing subprocesses:** `runner.Capture(cmd)` /
  `runner.CaptureWith(runner.CaptureOptions{Cmd, Label})` run a
  subprocess without suspending the TUI, posting `CaptureStarted`, a
  `CapturedLine` per line, then one `Captured`. `runner.Next(msg)` asks
  for the next message and `runner.Kill(started)` stops the process — the
  app shell does both for you, and forwards every message to the active
  screen besides. `pkg/runner` imports nothing from tuilib, so these
  messages are deliberately neutral: `pkg/app` is what turns them into log
  records, because the log format, the source attribution and the
  read-marker are shell knowledge. See rule 15.

When in doubt: read the nearest example and copy its structure. The
examples are maintained as the source of truth for idiomatic composition.
