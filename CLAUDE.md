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
   and wrap components via `layout.Sized(&c)` (anything with
   `SetDimensions(w,h)`) or `layout.Bar(&c)` (anything with `SetWidth(w)`).
   Never write `height - 2` to leave room for a bar — put the bar in a
   `Fixed(1, …)` sibling.

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

6. **Forward every `tea.Msg` to embedded components.** Don't conditionally
   skip forwarding — each component decides what to act on. Even if you
   intercepted a key for your own shortcut, still forward it so focus +
   viewport behavior stays correct.

7. **Pass data through the stack, not globals.** Parent → child: construct
   the child with what it needs (`screen.Push(newDetail(city, s.t))`).
   Child → parent: `screen.Pop(result)`. The newly-active parent receives
   the value in `OnEnter(result any)` — that's where you rebuild UI that
   depends on it. On the initial push `OnEnter` fires with `nil`.

8. **Interaction should be menu-driven.** Prefer lists + enter over letter
   shortcuts for per-screen actions (`d` delete, `r` run, etc.). Reserve
   single-letter keys for app-wide affordances (`q`, `t`, `/`, `esc`).
   This keeps `Help()` honest and avoids shortcut collisions across
   screens.

9. **Components own their pane.** Every interactive component in `pkg/`
   bundles a `pane.Pane` internally — `pkg/list`, `pkg/table`, `pkg/filter`,
   `pkg/input`, `pkg/toggle`, `pkg/logview`, `pkg/tree`, `pkg/inspector` all return a
   bordered, titled render from `View()`. To put a label on a component,
   set its `Title` field (which is rendered on the pane's top border) —
   don't render a label line above the component, and don't wrap a
   component in a second `pane.Pane`. The only things that *don't* own a
   pane are bars (`breadcrumb`, `statusbar`), the `help` key-hint
   renderer, the layout primitives, `pkg/runner` (which is not a UI
   component — it suspends the program to run a subprocess), and
   `pkg/form` itself, which is a vertical layout of bordered fields. New
   input-style components should follow the same shape: `Options.Title` +
   an internal `pane.Pane` + `View()` returns the bordered render.

10. **Components expose `Help() []key.Binding`.** Interactive components
    (`list`, `table`, `filter`, `input`, `toggle`, `logview`, `tree`,
    `inspector`, `form`) return the bindings they currently respond to. Screens compose these into their
    own `Help()` so the hint strip updates as state changes — e.g. the
    focused field of a form, or whether a logview's filter is engaged.
    When state changes the relevant bindings (filter focused vs. blurred,
    a query is active in logview), `Help()` reflects it.

11. **Run interactive subprocesses through `pkg/runner`.** For editors,
    pagers, full-screen TUIs, or any command that needs the terminal
    (`$EDITOR`, `less`, `htop`, `ssh`), call `runner.Run(*exec.Cmd)` from
    your screen's `Update`. It suspends the alt-screen, hands the TTY to
    the subprocess (with `Stdin/Stdout/Stderr` and `LINES`/`COLUMNS` env
    pre-populated), and posts a `runner.Result` back when the subprocess
    exits. Don't call `tea.ExecProcess` directly — the wrapper handles
    fallback plumbing for terminals that miss the post-resume SIGWINCH.

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

14. **Enter means "open the focused selection."** In multi-pane screens
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

15. **Use `SetLoading(b bool) tea.Cmd` while data is in flight.** Any
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

16. **Trust the pane to handle long lines.** `pane.Pane` truncates each
    line to the inner width on `SetContent` (ANSI-aware via
    `x/ansi.Cut`) and exposes left/right (and `h`/`l`) for horizontal
    scroll, with an optional thin scrollbar via `Options.HScrollbar`.
    Don't pre-wrap with newlines or `wordwrap.WrapString` to avoid
    terminal-wrap glitches — the pane already prevents that. Pre-wrap
    only when you specifically want soft-wrapping (prose paragraphs);
    otherwise let truncation + horizontal scroll do the work.
    `theme.Logview()` enables `HScrollbar` by default since long log
    lines are common.

17. **Color cells in a table row with `ansi.CellColor`, not full lipgloss
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

18. **Send transient feedback through `app.Info` / `app.Error` /
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

19. **For tabbed sub-screens, use `pkg/tab`.** A `tab.Model` hosts multiple
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

    Message routing inside `tab.Model`: `tea.KeyMsg` goes to the active body
    only (otherwise `/` filters, `j/k` cursors, etc. race across hidden
    tabs); everything else (timers, async fetch results, custom messages)
    fans out to every body so a `tea.Tick` re-arm in an inactive tab keeps
    streaming. See `examples/app/tabs`, where the Logs tab keeps appending
    lines while you're on the Cities or Counter tab.

20. **For yes/no modals, use `pkg/confirm`.** A `confirm.Model` renders a
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

21. **For "stop and acknowledge" modals, use `pkg/alert`.** Same shape as
    `pkg/confirm` but with a single OK button and an `alert.DismissedMsg`
    result. Reach for it when the user *must* see and dismiss something
    before continuing — surfaced errors, destructive-action results,
    blocking notices. For passive feedback (success notices, transient
    warnings) prefer the lighter `app.Info` / `app.Error` statusbar
    messages from rule 18; the modal is heavier and breaks flow. The
    chrome from `theme.Alert()` is intentionally neutral: for an
    error-tinted look, override `ActiveColor` with `t.ErrorBG` (and
    optionally `OKStyle` foreground with the same) — the component is
    palette-agnostic and the semantics live in the override. Hosting,
    `IsCapturingKeys`, `Help()` composition, and ZStack placement all
    follow rule 20. See `examples/data/alert`.

22. **For auto-refresh, use `pkg/poll` + keyed rows.** When data backing a
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

    The `app.Info` / `app.Error` statusbar (rule 18) is the right place
    for transient refresh feedback ("refreshed 14 deployments"); the
    statusbar auto-clears so it doesn't accumulate. For a persistent
    "last refreshed Xs ago" indicator, mutate the component's title via
    `SetTitle` from a periodic UI tick — see `examples/data/poll`.

## Anti-patterns

- **Don't wire breadcrumb + statusbar by hand when you can use `pkg/app`.**
  The shell owns them, including theme-swap rebuilds and `SetWidth` on
  resize. The only reason to skip it is if you need something the shell's
  shape doesn't support.
- **Don't write `m.h - 2` / `m.h - 5` math inside a screen's `Layout()`.**
  Use `Fixed`/`Flex` siblings. The body gets whatever's left; it doesn't
  need to know about sibling sizes.
- **Don't set `Width`/`Height` on a component you're wrapping in
  `layout.Sized`.** The engine will overwrite it — passing literal sizes
  just misleads the reader.
- **Don't handle `q`, `t`, or esc-pop inside a screen** when running under
  `pkg/app`. The shell routes those. Return them from `Help()` so they
  appear in the hints, but don't re-implement them.
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
  `app.Info` / `app.Error` (rule 18) — modals break flow, statusbar
  doesn't.
- **Don't roll your own log viewer.** Use `pkg/logview` for any append-
  mostly text stream that needs search / jump / filter / auto-follow.
  Wrapping `viewport.Model` directly skips the search highlight, current-
  line indicator, and `MaxLines` cap that logview already gets right.
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
- `Bar(&c)` adapts any `SetWidth(int) + View()` component.
- `Sized(&c)` adapts any `SetDimensions(w,h int) + View()` component.
- `RenderFunc(func(w,h int) string)` — escape hatch; size and render inline.
- `ZStack(base, overlay)` composites overlay over base.
- `Center(w, h, node)` renders `node` at a fixed size, centered in the
  parent's rect — the standard modal pattern (use inside a `ZStack`).

## Manual layout reference (only when not using `pkg/app`)

If you genuinely can't use the app shell, here are the row costs:

| Component | Rows consumed |
|---|---|
| `breadcrumb.Model` | 1 |
| `statusbar.Model` | 1 |
| `filter.Model` | 3 (border + content + border) |
| `pane.Pane` | caller-controlled, min 3 (4 when `HScrollbar=true` — one inner row reserved for the bar) |
| `list.Model` (Filterable=false) | caller-controlled |
| `list.Model` (Filterable=true) | caller-controlled, internally splits 3 for filter + rest for body |
| `table.Model` (Filterable=false) | caller-controlled, all body (header consumes 1 inner row, leaving `VisibleRows()-1` data rows) |
| `table.Model` (Filterable=true) | caller-controlled, internally splits 3 for filter + rest for body (then header consumes 1) |
| `logview.Model` (Searchable=false) | caller-controlled, all body |
| `logview.Model` (Searchable=true) | caller-controlled, internally splits 3 for filter + rest for body |
| `tree.Model` (Searchable=false) | caller-controlled, all body |
| `tree.Model` (Searchable=true) | caller-controlled, internally splits 3 for filter + rest for body |
| `inspector.Model` (Filterable=false) | caller-controlled, all body |
| `inspector.Model` (Filterable=true) | caller-controlled, internally splits 3 for filter + rest for body |

Typical body height:
- Plain body pane: `m.h - 2`
- Body pane + standalone `filter.Model` above: `m.h - 5`
- `list.Model` filterable: `m.h - 2` (the filter is inside it)

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
- **Statusbar messages from a screen:** `app.Info(s)` / `app.Error(s)` /
  `app.ClearStatus()` return `tea.Cmd`s that the shell intercepts and
  paints into the statusbar's center slot. Auto-clears on the next
  `tea.KeyMsg`. See `examples/app/status` and rule 18.
- **Confirm modal:** `pkg/confirm` is a yes/no dialog meant to live in a
  ZStack overlay. Resolves via `confirm.ConfirmedMsg` / `confirm.CancelledMsg`
  as `tea.Cmd`s the parent matches in its own `Update`. See
  `examples/data/confirm` and rule 20.
- **Alert modal:** `pkg/alert` is the acknowledgement counterpart to
  confirm — a single OK button, `alert.DismissedMsg` result, identical
  hosting pattern. Override `ActiveColor` with `t.ErrorBG` for an
  error-tinted look. Use it for "stop and acknowledge" feedback; prefer
  `app.Info` / `app.Error` for passive notices. See `examples/data/alert`
  and rule 21.
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
  `SetRows`/`SetKeyedRows`/`SetColumns`/`SetDimensions`, so flex
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
  unbroken (rule 17). Internal separators are configurable via
  `Options.Borders{Vertical, HeaderRule}` — both fields are pre-styled
  glyph strings (use `pkg/ansi.CellColor` so the selected-row bg passes
  through). `Vertical` replaces the inter-column space with `" <glyph> "`;
  `HeaderRule` repeats its first visible rune as a horizontal rule
  between header and data rows. `theme.Table()` ships with subdued
  light-line defaults (`│` and `─` in palette index 240); override per
  screen for a different glyph or color, or set fields to `""` to
  disable. See `examples/data/table` and `theme.Table()`.
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
  `↑↓`/`j`/`k`, `right`/`l`/space to expand, `left`/`h` to collapse or
  ascend), same `Filterable` story (`/` searches both Label and Value;
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
  a `pkg/table` cell keeps the selected-row background intact (rule 17).
  The package is rendering-only — callers own any history buffer (e.g.
  for `Spark`, append each tick's value and trim to a fixed window). For
  non-severity colorization (e.g. CPU usage that should always be blue
  when low and only flush red at saturation), use `BarStyled` /
  `SparkStyled` with an explicit ANSI palette index. See
  `examples/data/metrics` and rule 22 (auto-refresh).
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
  rule 22.

When in doubt: read the nearest example and copy its structure. The
examples are maintained as the source of truth for idiomatic composition.
