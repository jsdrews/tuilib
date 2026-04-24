# tuilib

A component library for building Bubble Tea TUIs quickly and reliably. Each
component is a small, well-documented `pkg/` with an `Options` struct, a
`New(Options)` constructor, and standard Bubble Tea `Init/Update/View`
methods. A central `theme` package collapses the color palette into one struct
so every component renders in the same palette without drift.

## Quickstart

The fastest path to a working TUI is `pkg/app` + one `screen.Screen`. The
shell handles breadcrumb + statusbar + theme cycling; your screen returns
a `layout.Node` tree and local state.

```go
package main

import (
    "fmt"
    "os"

    "github.com/charmbracelet/bubbles/key"
    "github.com/charmbracelet/bubbles/textinput"
    tea "github.com/charmbracelet/bubbletea"

    "github.com/jsdrews/tuilib/pkg/app"
    "github.com/jsdrews/tuilib/pkg/layout"
    "github.com/jsdrews/tuilib/pkg/list"
    "github.com/jsdrews/tuilib/pkg/screen"
    "github.com/jsdrews/tuilib/pkg/theme"
)

type cities struct {
    t    theme.Theme
    list list.Model
}

func (s *cities) Title() string            { return "Cities" }
func (s *cities) Init() tea.Cmd            { return textinput.Blink }
func (s *cities) OnEnter(any) tea.Cmd      { return nil }
func (s *cities) IsCapturingKeys() bool    { return s.list.Filtering() }
func (s *cities) Layout() layout.Node      { return layout.Sized(&s.list) }
func (s *cities) Help() []key.Binding {
    return []key.Binding{
        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
    }
}

func (s *cities) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    var cmd tea.Cmd
    s.list, cmd = s.list.Update(msg)
    return s, cmd
}

func (s *cities) SetTheme(t theme.Theme) {
    s.t = t
    cursor, value := s.list.Cursor(), s.list.Value()
    opts := t.List()
    opts.Title = "Cities"
    opts.Items = []string{"London", "Tokyo", "Madrid", "Lima"}
    opts.Filterable = true
    s.list = list.New(opts)
    if value != "" { s.list.SetValue(value) }
    s.list.SetCursor(cursor)
}

func main() {
    m := app.New(app.Options{
        Root:   &cities{},
        Themes: []theme.Theme{theme.Nord()},
    })
    if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
        fmt.Println(err); os.Exit(1)
    }
}
```

No `m.h-2` math, no breadcrumb/statusbar wiring, no resize handler — the
app shell owns that. The screen just declares shape (via `Layout()`) and
handles its own state in `Update`.

## Components

| Package | What it does |
|---|---|
| `pkg/app` | Standard shell — breadcrumb + body + statusbar, theme cycling, global-key routing, auto esc→pop |
| `pkg/screen` | `Screen` interface + `Stack` with push/pop and result passing via `OnEnter(result)` |
| `pkg/layout` | Declarative layout engine: `VStack`/`HStack`/`ZStack` + `Fixed`/`Flex` — no `m.h-2` math |
| `pkg/breadcrumb` | One-line header strip with click-or-keyboard crumbs |
| `pkg/pane` | Bordered, titled, scrollable region with slot metadata around the border |
| `pkg/statusbar` | Three-slot footer (left/middle/right) with info/error middle states |
| `pkg/help` | Key-hint renderer (`ShortView` inline, `FullView` overlay) |
| `pkg/nav` | Drill-down stack of screens (legacy `Screen` interface — prefer `pkg/screen`) |
| `pkg/filter` | Textinput in a pane; "/" to focus, enter commits, esc clears |
| `pkg/list` | Cursor-driven, optionally filterable list inside a pane |
| `pkg/theme` | Single palette struct + per-component `Options` builders |

All components follow the same shape:

```go
opts := somecomp.Options{ /* zero-value fields use defaults */ }
m := somecomp.New(opts)
// in your parent's Update:
m, cmd = m.Update(msg)
// in your parent's View:
s := m.View()
```

## Layouts

`pkg/layout` is a tiny declarative engine — a `Node` knows how to render
itself at a given `(w, h)`. Containers divide their allotment among
children:

```go
layout.VStack(
    layout.Fixed(1,  layout.Bar(&m.breadcrumb)),           // 1 row
    layout.Flex(1,   layout.HStack(                        // flex middle
        layout.Fixed(24, layout.Sized(&m.sidebar)),        // 24 cols
        layout.Flex(1,   layout.Sized(&m.body)),           // rest
    )),
    layout.Fixed(1,  layout.Bar(&m.statusbar)),            // 1 row
)
```

- `Fixed(n, node)` reserves exactly n cells on the main axis.
- `Flex(weight, node)` takes a share of what's left; sibling weights set the ratio.
- `Bar(&c)` adapts any `SetWidth(int) + View()` component (breadcrumb, statusbar).
- `Sized(&c)` adapts any `SetDimensions(w,h int) + View()` component (pane, list).
- `RenderFunc(func(w,h int) string)` is the escape hatch — size and render inline.
- `ZStack(base, overlay)` composites overlay on top; `Center(w, h, node)` renders `node` at its natural size centered within the parent's rect — the typical "modal" pattern.

Layout is pure render plumbing — it doesn't own focus or key routing.

## App shell and screens

`pkg/app` is the standard shell for a tuilib TUI: breadcrumb + flex body
+ statusbar + theme cycling + global-key routing + auto-esc-pop. You
provide a root `screen.Screen` and a list of themes; the shell does the rest.

```go
m := app.New(app.Options{
    Root:     newCityList(),
    Themes:   []theme.Theme{theme.Nord(), theme.Dark()},
    Version:  "v0.1.0",
    ThemeKey: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
})
tea.NewProgram(m, tea.WithAltScreen()).Run()
```

A `Screen` is a small interface — `Title / Init / Update / Layout / Help
/ OnEnter / SetTheme / IsCapturingKeys`. Each screen declares its own
layout tree; the shell renders it inside the body rect and never asks
the screen about terminal dimensions.

**Nav is a stack with result passing.** A child screen pops with a value:

```go
// inside the picker's Update, on enter:
return s, screen.Pop(s.list.Selected())   // child → parent data flow
```

The unblocked parent receives it in `OnEnter(result any)`:

```go
func (s *cityDetail) OnEnter(result any) tea.Cmd {
    if tz, ok := result.(string); ok && tz != "" {
        s.chosen = tz
        s.rebuildInfo()
    }
    return nil
}
```

Parent → child flows the other way: construct the child with whatever
arguments it needs (`screen.Push(newCityDetail(city))`). No special method.

`IsCapturingKeys()` tells the shell when a screen owns input (e.g. filter
is focused) so global keys like `q`, `t`, and esc-pop are suppressed.

## Theming

`theme.Theme` is a single struct of semantic color tokens (`BarBG`, `KeyFG`,
`Accent`, `BorderActive`, …). Swapping themes is a one-liner — every
component reads from the same palette, so nothing drifts. Built-ins include
`Dark`, `Light`, `Nord`, `Dracula`, `Gruvbox`, `Solarized`, `TokyoNight`,
`CatppuccinMocha/Latte`, `RosePine/Dawn`, `OneDark`, `Monokai`,
`EverforestDark`, four `Base16…` schemes, and a `Terminal()` that reads
the user's actual terminal palette at startup.

```go
th := theme.Nord()
bc := breadcrumb.New(th.Breadcrumb())
p  := pane.New(th.Pane())
fl := filter.New(th.Filter())
li := list.New(th.List())
sb := statusbar.New(th.Statusbar(helpModel.ShortView(), "v0.1.0"))
```

See all themes live with `task examples:themecheck`.

## Examples

Each demo is a single `main.go` you can run with `task examples:<name>`:

| Task | Demonstrates |
|---|---|
| `examples:pane:basic` | Pane with scrollbar wrapping plain text |
| `examples:pane:styles` | Border + title-position variants |
| `examples:pane:brackets` | Title slot-bracket styles (none / corners / tees) |
| `examples:footer:overlay` | Toggleable bordered help overlay above the statusbar |
| `examples:footer:inline` | All help hints inline in the statusbar |
| `examples:nav:breadcrumbs` | Breadcrumb header + nav.Stack drilldown (plain body) |
| `examples:nav:pane` | Same, with the body wrapped in `pane.Pane` |
| `examples:data:list` | Filterable `list.Model` with live theme cycling |
| `examples:data:table` | Filterable `bubbles/table` with live theme cycling |
| `examples:app:layouts` | Five screens in one app, each with a different layout tree |
| `examples:app:stack` | Screen stack with two-way data flow (constructor + `OnEnter`) |
| `examples:themecheck` | Interactive theme picker — cursor re-skins the TUI live |

Run `task` (no args) for the full list.

## Learning the library

### For humans

1. **Run the examples.** `task examples:app:stack` (for nav + data flow),
   `task examples:app:layouts` (for layout primitives), `task examples:data:list`,
   `task examples:themecheck`. Each is self-contained and shows one idiom.
2. **Read the package doc comment.** Every `pkg/*/*.go` opens with a
   paragraph explaining what the component is and when to use it. `go doc
   ./pkg/pane` prints it.
3. **Read the `Options` fields.** Every field on every `Options` struct has
   a comment describing its default and when to override. `go doc
   ./pkg/list.Options` is the fastest way to see the full configuration
   surface.
4. **Copy an example, then delete.** Start from the closest example, strip
   what you don't need, and the idioms come along for the ride.

### For agents

Read [`CLAUDE.md`](./CLAUDE.md) first — it's the rules-and-anti-patterns
brief that keeps generated code consistent with the library's design.
Claude Code auto-loads it; other agents should read it before writing any
tuilib code.

Beyond that, the library is structured to be discoverable by reading, not by
convention. In order of signal density:

1. `CLAUDE.md` — the rules, anti-patterns, and layout/nav idioms.
2. `go doc ./pkg/<name>` — package overview + every exported symbol with
   its doc comment. The most complete single source.
3. `pkg/<name>/<name>.go` top-of-file comment — the "what and why."
4. `Options` struct field comments — the configurability surface.
5. `examples/<area>/<name>/main.go` — the wiring patterns, including
   theme-swap, resize, and focus-handoff flows.
6. `pkg/theme/theme.go` — the `Theme` struct's field comments are the
   semantic color vocabulary shared across every component.

The examples are intentionally flat (no subpackages, no hidden helpers) so
a single `Read` call fits them in context. A good first move for any new
task is: find the closest example, read it end-to-end, then adapt.

## Project layout

Follows [golang-standards/project-layout](https://github.com/golang-standards/project-layout):

- `pkg/` — public components (import surface for consumers)
- `internal/` — private helpers not exported
- `cmd/` — demo binaries
- `examples/` — runnable example TUIs that exercise the components
- `docs/` — long-form usage notes

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs `go build`, `go vet`, and
`go test` on every push / PR. On merge to `master`, the release job auto-tags
with a semver bump: patch if `bug`/`bugfix`/`fix` appears in the branch name
or merge-commit message, minor otherwise. Starts at `0.1.0`, no `v` prefix.
