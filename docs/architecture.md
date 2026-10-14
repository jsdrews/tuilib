# Architecture

tuilib is a set of layers. Each one owns a single concern, and the rules that
keep the library consistent fall out of which layer owns what.

```
tea.Program
└── app.Model                   the shell: chrome, global keys, theme, mouse, console
    ├── breadcrumb.Model        1 row: the stack's titles
    ├── screen.Stack            push / pop / replace, result passing
    │   └── your Screen         state + Update + a layout.Node tree
    │       └── layout.Node     VStack / HStack / ZStack of Fixed / Flex items
    │           └── component   list, table, tree, … each owning a pane.Pane
    └── statusbar.Model         1 row: "? help", messages, console badge, version
```

## The app shell

`app.New(app.Options{Root: …, Palettes: …, Surfaces: …})` returns a `tea.Model` you hand to
`tea.NewProgram`. It owns everything around your screens:

- the **breadcrumb** (one crumb per screen on the stack, clickable),
- the **statusbar** (help affordance, transient messages, console badge,
  version),
- **global keys**: quit, esc-to-pop, palette and surface cycling, suspend, the `?` overlay,
  and the opt-in output-console and action-menu keys,
- **theme resolution** from an env var and the user's config file,
- **mouse**: enabling reporting, counting double clicks, routing clicks on its
  own chrome,
- the **output console**, the **action menu** and the **help overlay**.

A screen never sees the terminal size and never draws chrome. See
[The app shell](guide/app-shell.md).

## Screens and the stack

A `screen.Screen` is your unit of UI: it holds its components, handles its
messages, and describes its layout. Screens live on a `screen.Stack`. Data
moves through the stack in three directions:

| Direction | How |
|---|---|
| parent → child | construct the child with what it needs: `screen.Push(newDetail(item, t))` |
| child → parent | `screen.Pop(result)`; the uncovered parent receives it in `OnEnter(result)` |
| self → self | `screen.Replace(fresh)` swaps the top without disturbing the parent |

See [Screens and navigation](guide/screens.md).

## Layout

`Layout()` returns a tree of `layout.Node`. Containers split their rect among
`Fixed(n, …)` and `Flex(weight, …)` children, and leaves are components
wrapped with `layout.Sized(&c)`. Each component is handed a `geom.Rect`: its
**absolute** position and size. The position is what makes mouse support
work without injecting anything into the rendered string; a component stores
its rect and hit-tests clicks against it. See [Layout](guide/layout.md).

## Components own their pane

Every interactive component bundles a `pane.Pane`: a bordered, titled,
scrollable region with a scrollbar, slots on its border, horizontal scroll and
a loading spinner. So:

- **label a component with its `Title` option**, which draws on the border;
  never render a label line above it, and never wrap it in another pane;
- **a filterable component draws its filter inside its own pane**, as a
  pinned row above a rule, so it's unambiguous which pane a filter belongs to;
- **a component owns its geometry**: it knows where its rows, glyphs and
  buttons are, so it's the one that decides a click was its own.

The exceptions are the bars (`breadcrumb`, `statusbar`), the help footer, the
layout primitives, and `pkg/form`, which is a vertical stack of bordered
fields.

## The component shape

Every component follows the same pattern:

```go
opts := t.Table()            // 1. start from the theme builder
opts.Title = "Deployments"   // 2. set what's specific to this instance
opts.Columns = cols
s.table = table.New(opts)    // 3. construct

s.table, cmd = s.table.Update(msg)   // 4. forward every message
layout.Sized(&s.table)               // 5. place it
```

Stateful components add `State()` / `Restore(State)` for rebuilding,
`Help()` / `HelpSections()` for key hints, and `Focus` / `Blur` / `Focused`
for focus. See [Components](components/index.md).

## How messages flow

Bubble Tea delivers every message to `app.Model.Update`. From there:

1. **Shell-level messages** are handled and consumed: window resize, theme
   swap, statusbar requests (`app.Info`, `app.Error`), stack operations,
   console and action-menu traffic.
2. **Global keys** are checked next, unless the active screen reports
   `IsCapturingKeys()`. A focused text field gets `q` as a letter, not as
   quit.
3. **Everything else goes to the stack**, which routes by kind:
    - `tea.KeyMsg` and `mouse.Msg` go to the **top screen only**. A covered
      screen must not act on input meant for what covers it.
    - All other messages (timers, fetch results, spinner ticks) **fan out to
      every screen on the stack**, so a polling screen keeps polling while a
      child is pushed over it and its data is current when you return.
4. **Inside a screen**, forward every message to every component. Each
   component decides what to act on. The exceptions are about user input
   aimed at what's on screen:
    - with a `focus.Group`, keys go to the **focused** component only, while
      mouse events go to **all** components (each tests its own rect);
    - `pkg/tab` sends keys and mouse to the **active** tab body only; other
      messages fan out to every body.

## Theme swaps and state

`theme.Theme` is one struct of semantic color tokens, border shapes and
glyphs. Each component has a builder on it (`t.List()`, `t.Table()`, …)
returning pre-styled `Options`. When the theme changes, the shell calls
`SetTheme(t)` on **every** screen on the stack, and each screen rebuilds its
components from the new builders.

Rebuilding loses nothing because each stateful component exposes an opaque
`State()` covering everything Options doesn't set: rows, cursor, scroll,
filter, sort, marks, expansion, follow, remote query state, row activity.
`Restore` replays it in the order that keeps each part valid. See
[Theming and state](guide/theming.md).

## Focus, keys and mouse

- **Keys**: arrows and `hjkl` are reserved for scrolling everywhere; `tab` /
  `shift+tab` cycle focus between panes; `enter` means "open the selection".
  Every component's bindings live in an overridable `Options.Keys` struct.
- **Focus**: a screen with several interactive components holds a
  `focus.Group` over them, which owns cycling and grants focus to a clicked
  component.
- **Mouse** is opt-in (`app.Options.Mouse = app.MouseClick`). Click focuses
  and selects; double-click is enter; the wheel scrolls whatever is under the
  pointer; library-drawn chrome (headers, glyphs, scrollbars, tabs, crumbs,
  buttons) is clickable.

See [Keys, focus and mouse](guide/input.md).

## Async work

Every component is synchronous; nothing in the library spawns a goroutine
that touches a model. Long-running work runs in a `tea.Cmd` and comes back as
a message:

- `pkg/poll` emits `RefreshMsg` on an interval; your screen fetches.
- `pkg/source` and `pkg/remote` decide *what* to fetch for paged data; your
  fetch function does the I/O under a context the library cancels.
- `pkg/activity` (through `table`, `list` and `tree`) shows per-row spinners
  driven by the server's status.
- `pkg/runner` hands the terminal to a subprocess, captures one in the
  background, or streams a Go function's output into the console.

## Package map

| Layer | Packages |
|---|---|
| Shell and navigation | `app`, `screen`, `tab` |
| Layout and geometry | `layout`, `geom` |
| Look | `theme`, `glyph`, `ansi`, `config` |
| Input | `focus`, `mouse`, `help`, `action` |
| Containers and text | `pane`, `textview`, `logview` |
| Data views | `list`, `table`, `tree`, `inspector`, `eventlog`, `metrics` |
| Inputs and modals | `filter`, `input`, `toggle`, `form`, `confirm`, `alert` |
| Bars | `breadcrumb`, `statusbar` |
| Data plumbing | `poll`, `source`, `remote`, `query`, `activity`, `resume` |
| Processes and logs | `runner`, `output` |

Leaf packages (`geom`, `query`, `glyph`, `activity`, `runner`) import
nothing else from tuilib, so any package can depend on them without a cycle.
