# tuilib

A component library for building Bubble Tea TUIs quickly and reliably. Each
component is a small, well-documented `pkg/` with an `Options` struct, a
`New(Options)` constructor, and standard Bubble Tea `Init/Update/View`
methods. A central `theme` package collapses the color palette into one struct
so every component renders in the same palette without drift.

## Quickstart

```go
package main

import (
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/jsdrews/tuilib/pkg/breadcrumb"
    "github.com/jsdrews/tuilib/pkg/list"
    "github.com/jsdrews/tuilib/pkg/statusbar"
    "github.com/jsdrews/tuilib/pkg/theme"
)

type model struct {
    w, h   int
    header breadcrumb.Model
    list   list.Model
    status statusbar.Model
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.w, m.h = msg.Width, msg.Height
        th := theme.Nord()

        bc := th.Breadcrumb()
        bc.Width = m.w
        bc.Crumbs = []string{"Cities"}
        m.header = breadcrumb.New(bc)

        lo := th.List()
        lo.Width, lo.Height = m.w, m.h-2
        lo.Title = "Cities"
        lo.Items = []string{"London", "Tokyo", "Madrid", "Lima"}
        lo.Filterable = true
        m.list = list.New(lo)

        sb := th.Statusbar("", "tuilib demo")
        sb.Width = m.w
        m.status = statusbar.New(sb)

    case tea.KeyMsg:
        if !m.list.Filtering() && msg.String() == "q" {
            return m, tea.Quit
        }
    }
    var cmd tea.Cmd
    m.list, cmd = m.list.Update(msg)
    return m, cmd
}

func (m model) View() string {
    if m.w == 0 { return "" }
    return m.header.View() + "\n" + m.list.View() + "\n" + m.status.View()
}

func main() {
    if _, err := tea.NewProgram(model{}, tea.WithAltScreen()).Run(); err != nil {
        fmt.Println(err); os.Exit(1)
    }
}
```

## Components

| Package | What it does |
|---|---|
| `pkg/breadcrumb` | One-line header strip with click-or-keyboard crumbs |
| `pkg/pane` | Bordered, titled, scrollable region with slot metadata around the border |
| `pkg/statusbar` | Three-slot footer (left/middle/right) with info/error middle states |
| `pkg/help` | Key-hint renderer (`ShortView` inline, `FullView` overlay) |
| `pkg/nav` | Drill-down stack of screens with automatic back-stack |
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
| `examples:pane:brackets` | Title slot-bracket styles (corners / none / tees) |
| `examples:footer:overlay` | Toggleable bordered help overlay above the statusbar |
| `examples:footer:inline` | All help hints inline in the statusbar |
| `examples:nav:breadcrumbs` | Breadcrumb header + nav.Stack drilldown (plain body) |
| `examples:nav:pane` | Same, with the body wrapped in `pane.Pane` |
| `examples:data:list` | Filterable `list.Model` with live theme cycling |
| `examples:data:table` | Filterable `bubbles/table` with live theme cycling |
| `examples:themecheck` | Interactive theme picker — cursor re-skins the TUI live |

Run `task` (no args) for the full list.

## Learning the library

### For humans

1. **Run the examples.** `task examples:data:list`, `task examples:nav:pane`,
   `task examples:themecheck`. Each is ~100–200 lines of self-contained
   code that shows one idiom.
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

1. `CLAUDE.md` — the five rules, anti-patterns, and layout math.
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
