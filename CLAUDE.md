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
   `pkg/toggle`, `pkg/logview` all return a bordered, titled render from
   `View()`. To put a label on a component, set its `Title` field (which is
   rendered on the pane's top border) — don't render a label line above the
   component, and don't wrap a component in a second `pane.Pane`. The only
   things that *don't* own a pane are bars (`breadcrumb`, `statusbar`), the
   `help` key-hint renderer, the layout primitives, `pkg/runner` (which is
   not a UI component — it suspends the program to run a subprocess), and
   `pkg/form` itself, which is a vertical layout of bordered fields. New
   input-style components should follow the same shape: `Options.Title` +
   an internal `pane.Pane` + `View()` returns the bordered render.

10. **Components expose `Help() []key.Binding`.** Interactive components
    (`list`, `filter`, `input`, `toggle`, `logview`, `form`) return the
    bindings they currently respond to. Screens compose these into their
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

12. **Cap streaming buffers.** Components that accept open-ended input
    (`pkg/logview`) apply a default `MaxLines` cap so an unbounded
    producer can't grow memory without limit. When wiring up a streaming
    component, decide on an explicit cap if the default isn't right;
    only set `-1` (unbounded) when the producer is itself bounded.

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
- **Don't call `tea.ExecProcess` directly for subprocesses.** Use
  `runner.Run(*exec.Cmd)` — it sets `Stdin/Stdout/Stderr` + `LINES`/
  `COLUMNS` and returns a typed `runner.Result` your `Update` can match
  on. Bypassing it loses those defenses and the consistent result type.
- **Don't leave a streaming buffer uncapped.** `pkg/logview` defaults to
  `DefaultMaxLines = 10000`. Override only when you have a real reason —
  passing `-1` opts out of the cap entirely and makes the buffer grow
  with the stream.
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
| `pane.Pane` | caller-controlled, min 3 |
| `list.Model` (Filterable=false) | caller-controlled |
| `list.Model` (Filterable=true) | caller-controlled, internally splits 3 for filter + rest for body |
| `logview.Model` (Searchable=false) | caller-controlled, all body |
| `logview.Model` (Searchable=true) | caller-controlled, internally splits 3 for filter + rest for body |

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
