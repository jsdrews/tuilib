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
   bundles a `pane.Pane` internally — `pkg/list`, `pkg/filter`, `pkg/input`,
   `pkg/toggle`, `pkg/logview`, `pkg/tree` all return a bordered, titled
   render from `View()`. To put a label on a component, set its `Title`
   field (which is rendered on the pane's top border) — don't render a label
   line above the component, and don't wrap a component in a second
   `pane.Pane`. The only things that *don't* own a pane are bars
   (`breadcrumb`, `statusbar`), the `help` key-hint renderer, the layout
   primitives, `pkg/runner` (which is not a UI component — it suspends the
   program to run a subprocess), and `pkg/form` itself, which is a vertical
   layout of bordered fields. New input-style components should follow the
   same shape: `Options.Title` + an internal `pane.Pane` + `View()` returns
   the bordered render.

10. **Components expose `Help() []key.Binding`.** Interactive components
    (`list`, `filter`, `input`, `toggle`, `logview`, `tree`, `form`) return
    the bindings they currently respond to. Screens compose these into their
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

17. **Color bubbles/table cells with `ansi.CellColor`, not lipgloss.**
    `bubbles/table` wraps each cell in its `Selected` style via
    `lipgloss.Render`, and lipgloss's full `\x1b[0m` reset clobbers the
    outer background mid-cell. `ansi.CellColor(n, text)` emits a
    foreground-only escape that closes with `\x1b[39m`, so the row's bg
    stays intact across colored cells. CellColor picks the shortest open
    form for the palette index (`\x1b[3Nm` for n<8, `\x1b[9Nm` for n<16,
    `\x1b[38;5;Nm` for 16+).

    Important sizing rule: `bubbles/table`'s truncation is **not**
    ANSI-aware (upstream uses `runewidth.Truncate`, which counts the
    escape bytes as visible width). If a cell exceeds its column width,
    the closing reset gets chopped off and the foreground bleeds into
    later cells. Size columns at least `visible_chars + 8` for n<16, or
    `+ ~14` for 256-color indices. The example's Status column uses 22
    cells for 10-char content with 8/16-color CellColor.

    This caveat is bubbles/table-only — every component that owns its
    own pane (`list`, `tree`, `logview`, …) renders ANSI-aware and you
    can keep using lipgloss inside them. See `examples/data/table/table.go`
    Status column.

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
- **Don't wrap `bubbles/table`.** We deliberately don't provide a table
  component — bubbles/table already owns rendering + scrolling + cursor,
  and wrapping it is passthrough bloat. For a filterable table, compose
  `bubbles/table` + `filter.Model` + `pane.Pane` directly. See
  `examples/data/table/main.go`.
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
| `logview.Model` (Searchable=false) | caller-controlled, all body |
| `logview.Model` (Searchable=true) | caller-controlled, internally splits 3 for filter + rest for body |
| `tree.Model` (Searchable=false) | caller-controlled, all body |
| `tree.Model` (Searchable=true) | caller-controlled, internally splits 3 for filter + rest for body |

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

When in doubt: read the nearest example and copy its structure. The
examples are maintained as the source of truth for idiomatic composition.
