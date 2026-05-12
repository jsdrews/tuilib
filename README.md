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
| `pkg/pane` | Bordered, titled, scrollable region with slot metadata around the border — the primitive every other component wraps. Truncates long lines to inner width and supports horizontal scroll (←→ / h / l) with an optional thin scrollbar. Built-in loading state replaces the body with a centered spinner via `SetLoading(true)`. |
| `pkg/statusbar` | Three-slot footer (left/middle/right) with info/error middle states |
| `pkg/help` | Key-hint renderer (`ShortView` inline, `FullView` overlay) |
| `pkg/filter` | Textinput in a pane; "/" to focus, enter commits, esc clears |
| `pkg/list` | Cursor-driven, optionally filterable list inside a pane. `SelectedIndex()` returns the underlying source-slice index even when items are formatted display strings and a filter is active. `SetKeyedItems([]KeyedItem{Key,Display})` + `SelectedKey()` snap the cursor to the same Key after a swap (the auto-refresh primitive — pair with `pkg/poll`). Vim-style nav: `g`/`G` top/bottom, `ctrl+u`/`ctrl+d` half-page, plus `↑↓` per row |
| `pkg/table` | Cursor-driven, optionally filterable tabular view inside a pane. `Column{Title, Width, Align, Sortable, Less, Flex, MaxWidth}` declares the layout; rows are `[]string` cells. Header pins to the top while scrolling horizontally with the body so columns stay aligned. ANSI-aware truncation via `x/ansi.Cut` — `Width` is the visible cell width. Sizing: `Width > 0` → fixed; `Width == 0` → content-auto (max of title + any cell, ANSI stripped, floor 4); `Flex > 0` → absorbs a share of leftover inner width by weight (Width is the min, `MaxWidth` is the cap; surplus from capped columns redistributes to uncapped flex columns). Recomputes on row/column/dimension changes, so flex columns reflow on resize. Same nav verbs as `pkg/list` (`g`/`G`, `ctrl+u/d`, `↑↓`/`j`/`k`); filter matches across all cells (ANSI stripped before matching) and accepts space-separated AND-ed terms; a `key:value` term scopes the match to the column whose Title starts with `key` (e.g. `region:europe pop:5`); a `~` prefix on the value makes that term a case-insensitive regex (e.g. `~^new`, `region:~^euro`). Mid-typing a `key:val` term shows the column's distinct matching values in the filter's bottom-left slot, and `tab` completes to the longest common prefix. `SetKeyedRows([]KeyedRow{Key,Cells})` + `SelectedKey()` snap the cursor to the same Key after a swap (the auto-refresh primitive — pair with `pkg/poll`). `Sortable` columns expose `[`/`]` (step active sort column) + `s` (toggle direction); supply `Column.Less` for numeric or unit-aware sort. `Options.Borders{Vertical, HeaderRule}` configures the inter-column glyph and the horizontal rule below the header — both are pre-styled glyph strings (`pkg/ansi.CellColor` keeps the selected-row bg intact); `theme.Table()` ships with subdued `│`/`─` defaults |
| `pkg/input` | Single-line text input in a pane; bare textbox without filter's commit/cancel keys |
| `pkg/toggle` | Yes/no selector in a pane — left/right/space/y/n |
| `pkg/confirm` | Modal yes/no dialog with title + message + confirm/cancel buttons; resolves via `ConfirmedMsg` / `CancelledMsg` so parent screens stay bubbletea-idiomatic. Designed for `layout.ZStack(base, layout.Center(w, h, ...))` |
| `pkg/alert` | Modal acknowledgement dialog with title + message + single OK button; resolves via `DismissedMsg`. Use for "stop and acknowledge" feedback (errors, blocking notices); for passive feedback prefer the lighter `app.Info` / `app.Error` statusbar messages. Override `ActiveColor` with `theme.ErrorBG` for an error-tinted look |
| `pkg/logview` | Streaming text viewer with `/`-search, n/N jump, g/G top/bottom, filter mode, current-line highlight, and a default `MaxLines` safety cap |
| `pkg/tree` | Searchable, expand/collapse hierarchical viewer over any `Node` (Label + Children); `/`-search highlights inline and `\` hides non-matching subtrees while keeping ancestors. Labels may contain lipgloss-styled ANSI (colored status icons, etc.) — the cursor's row highlight stays intact across colored segments |
| `pkg/inspector` | Two-column label/value viewer for structured records (k8s manifests, REST responses, Prefect run details). `Field{Label, Value, Children}` composes; `FromAny` / `FromMap` convert `json.Unmarshal` output into Fields. Sibling labels auto-align per group, ▸/▾ expand nested objects/arrays, `/` searches labels and values, `\` hides non-matching subtrees. `SetFields` preserves expansion state + cursor by row path across swaps — the auto-refresh primitive for inspector |
| `pkg/poll` | Interval-driven `tea.Cmd` ticker for auto-refresh. Emits a typed `RefreshMsg` parents match in their `Update` to kick off a refetch; `Pause`/`Resume`/`SetInterval`/`Refresh` (now) / `MarkRefreshed`. Pair with the keyed-row APIs on `pkg/list` and `pkg/table` (or path-keyed `SetFields` on `pkg/inspector`) so cursor + expansion survive every swap |
| `pkg/metrics` | Inline rendering primitives for list rows / table cells / inspector values: `Badge(ok, warn, down)` (status-count summary), `Ratio(done, total)` (severity-colored "N/M"), `Bar(value, max, width)` (fixed-width progress bar), `Spark(values, width)` (8-step block sparkline that resamples to fit). All return ANSI-safe foreground-only strings via `pkg/ansi.CellColor` so the selected-row background passes through. Rendering-only — caller owns history buffer for sparklines |
| `pkg/form` | Vertical layout of `input` + `toggle` (+ Select) fields with tab cycling and a submit button |
| `pkg/tab` | Tabbed container hosting multiple `screen.Screen` bodies behind a one-row strip. Each body keeps its own state across switches; `shift+left`/`shift+right` and `1`–`9` switch tabs (`tab`/`shift+tab` is left alone for inner pane focus cycling). Host screen forwards `Update`/`OnEnter`/`IsCapturingKeys`/`SetTheme`/`Help` to `tabs` |
| `pkg/runner` | Hand the terminal to an interactive subprocess (vim, htop, less, ssh) and resume the TUI on exit. Clears the screen on handoff by default; `RunWithNotice` prints a transitional line for slow handoffs (kubectl exec, ssh); `RunWith(Options{...})` for full control |
| `pkg/theme` | Single palette struct + per-component `Options` builders. `app.New` resolves the initial theme from `Options.ThemeEnvVar` + the user config file automatically (set `SkipConfig=true` to opt out) |
| `pkg/config` | YAML user-config at `~/.config/tuilib/config.yaml`. Pure data + I/O; opt-in (library never writes). Today carries `Theme`; expands as components grow user-tunable knobs |
| `pkg/ansi` | `CellColor(n, text)` for foreground-only ANSI in table cells (or any context that wraps content in its own SGR with a background); the foreground-only `\x1b[39m` reset preserves the outer background where lipgloss's full `\x1b[0m` would clobber it. `Hyperlink(url, text)` wraps a label in an OSC 8 hyperlink so shift-click in alacritty/tmux/kitty/iTerm2 opens the full URL — and `x/ansi.Cut` preserves the envelope across truncation, so narrow columns don't break the launched URL. `ExtractHyperlink(cell)` pulls the URL back out for programmatic open-in-browser |

> **Components own their pane.** Every interactive component (`pane`,
> `filter`, `list`, `table`, `input`, `toggle`, `logview`, `tree`) bundles
> a `pane.Pane` internally and returns a bordered render from `View()`. To
> label one, set its `Title` field — it renders on the border. Don't wrap a
> component in a second pane; don't render a label line above it.

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

**Atomic replace.** When you want a fresh instance of the current view
(reset filter, reset scroll, refetch from scratch) without disturbing the
parent below, use `screen.Replace(s)`:

```go
return s, screen.Replace(newCityDetail(s.city, s.t))
```

Replace swaps the top of the stack in one tick — no pop+push flicker, and
the parent's `OnEnter` doesn't fire spuriously. Pass the new screen with
its theme already applied (`s.t` in the example), same convention as Push.
See `examples/app/replace`.

`IsCapturingKeys()` tells the shell when a screen owns input (e.g. filter
is focused) so global keys like `q`, `t`, and esc-pop are suppressed.

**Statusbar messages.** Screens push transient feedback into the
statusbar's center slot via `tea.Cmd`s the shell intercepts:

```go
// inside Update, after a successful run:
return s, app.Info("Run completed successfully.")
// or, on failure:
return s, app.Error("Error: API request failed — connection refused.")
// to wipe an existing message without setting a new one:
return s, app.ClearStatus()
```

The shell mirrors the message into the bar with the appropriate style
(green for info, red for error). Messages auto-clear on the next keypress —
the same behavior as pug's footer — so you don't have to manage their
lifetime. See `examples/app/status` for the full pattern.

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

### Default theme via env / config

`app.New` resolves the initial theme by checking, in order, the env var
named by `Options.ThemeEnvVar`, the `theme:` field in
`$XDG_CONFIG_HOME/tuilib/config.yaml` (falls back to
`~/.config/tuilib/config.yaml`, via `pkg/config`), and `Themes[0]`. Hand
the raw theme list to `app.Options.Themes` — the shell reorders it for
you:

```go
m := app.New(app.Options{
    Root:        newRoot(),
    Themes:      theme.All(),
    ThemeEnvVar: "MY_APP_THEME",
    // ...
})
```

User-side, `~/.config/tuilib/config.yaml`:

```yaml
theme: dracula
```

The library never writes the file and never creates the directory — config
is opt-in. A missing file is the steady state. Unknown theme names fall
through silently (typo `dracla` just leaves `Themes[0]` as the default);
malformed YAML surfaces as an error only if you call `config.Load`
directly. Leave `ThemeEnvVar` empty to skip the env-var step and rely on
the config file alone, or set `Options.SkipConfig = true` to disable
resolution entirely (useful for tests, or when the app should always pin
to `Themes[0]`). The underlying `theme.Resolve(themes, envVar)` is still
exported if you need the resolved order outside `app.New`.

The shared file lives in `pkg/config` — as other components grow
user-tunable knobs (logview defaults, key remaps, custom palettes) their
fields will land on `config.Config` and any package can consult the same
file without going through `pkg/theme`.

## Examples

Run `task examples` to open a launcher with a menu of demos. Select one
and press enter to drill in; esc pops back to the menu. The launcher itself
is just `pkg/app` hosting a filterable list — the same pattern every other
demo uses.

| Entry | Demonstrates |
|---|---|
| Panes   | Border styles, title positions, and slot-bracket variants in one 2×2 grid |
| List    | A filterable `list.Model` as a single-screen app |
| Logview | Streaming log tail with `/`-search, n/N jump, `\` filter-mode toggle, current-line highlight |
| Table   | `table.Model` with sticky header, per-column widths, `g`/`G`/`ctrl+u/d` nav, sort via `[`/`]`/`s` (City + Region default-string, Population uses a custom `Less` to parse "8.3M"), and a Status column using `ansi.CellColor` so the selected-row background passes through colored cells. ANSI-aware truncation means `Column.Width` is the visible cell width — no escape-byte budget |
| Form    | `form.Model` with Text / Select / Confirm fields + submit button |
| Loading | `list`, `logview`, and `tree` start in `SetLoading(true)`; staggered `tea.Tick` delays simulate fetches. Press `r` to refetch. |
| Drilldown | Master-detail with async fetches at both levels, plus a third level via push. Cities list loads on Init; enter on a city fetches its attributes into the right pane (reqID-tagged so stale results are dropped); enter on a focused attribute pushes a child screen. Esc pops back with parent state intact. |
| Runner  | Pick a command, hand the terminal to it (`$EDITOR`, less, man, htop), return on exit. Last entry uses `RunWithNotice` to print "connecting…" during the handoff |
| Runlog  | Stream a subprocess's stdout/stderr into a `logview` pane; tab focus between picker and log, `x` to kill |
| Tree    | Synthetic project tree with cursor, expand/collapse (←→/space), `/`-search, and `\` filter mode that hides non-matching subtrees. Leaves carry colored status icons (lipgloss-rendered) so the row highlight stays intact across ANSI segments |
| Themes  | Live palette picker — cursor re-skins the whole app via `app.SetTheme` |
| Layouts | Five sub-screens, each with a different `layout.Node` tree |
| Stack   | Parent→child via constructor, child→parent via `Pop(result)` + `OnEnter` |
| Focus   | Multi-component focus cycling — tab/shift-tab between input + list + toggle, with `Help()` updating per focused component |
| Gate    | A root screen that pushes a login form on first OnEnter; submit pops back with creds, `L` re-pushes for logout |
| Tabs    | Three sub-screens (filterable list / streaming logview / counter) behind one tab strip; switch via shift+arrows or 1/2/3. Each body keeps its own state across switches; the logview keeps streaming while you're on another tab |
| Status  | Screens emit `app.Info` / `app.Error` / `app.ClearStatus` as `tea.Cmd`s; the shell mirrors the result into the statusbar's center slot. Auto-clears on the next keypress |
| Confirm | A `pkg/confirm` modal overlaid on a list via `ZStack` + `Center`. Press `d` on a file to bring up the dialog; on confirm the file is removed and the outcome is reported via `app.Info`. Demonstrates the message-driven result flow |
| Alert | A list of mock operations; some succeed (statusbar info), some fail with an error-tinted `pkg/alert` modal whose border is overridden with `theme.ErrorBG`. Contrasts the lightweight statusbar pattern with the modal "stop and acknowledge" pattern |
| Poll  | `pkg/poll` drives a 2s tick that mutates a synthetic job list; `SetKeyedItems` keeps the cursor on the same job ID across every refresh even as statuses flip and the list reorders. `p` pauses, `r` refreshes now, `+/-` adjust cadence; the pane title shows "refreshed Xs ago" |
| PollTable | `pkg/poll` + `pkg/table` `SetKeyedRows` on a deployments table — Sync/Health/Replicas drift each tick and dirty rows float to the top, but the cursor sticks to the same deployment ID. Same `p`/`r`/`+/-` keys; standard filter (`env:prod`, `health:~degraded`) and sort (`[`/`]`/`s`) still work |
| Metrics | `pkg/metrics` inline primitives composed into a polled deployments table — `Ratio` for replica counts, `Badge` for pod-state breakdown, `Bar`+percent for CPU, `Spark` for 24s CPU history. Demonstrates the primitives plug into existing components without a custom layout |

Each entry is a package under `examples/<area>/<name>/` that exports
`New(theme.Theme) screen.Screen`. The launcher imports them all and pushes
the chosen one onto its stack.

## Learning the library

### For humans

1. **Run the launcher.** `task examples`, then drill into Stack (nav + data
   flow), Layouts (layout primitives), or Themes (live palette preview).
   Each entry is self-contained and shows one idiom.
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
5. `examples/<area>/<name>/<name>.go` — each example is a package exposing
   `New(theme.Theme) screen.Screen`. Read `examples/launcher/main.go` to
   see the composition pattern (list of examples as a menu screen).
6. `pkg/theme/theme.go` — the `Theme` struct's field comments are the
   semantic color vocabulary shared across every component.

A good first move for any new task is: find the closest example, read it
end-to-end, then adapt.

## Project layout

Follows [golang-standards/project-layout](https://github.com/golang-standards/project-layout):

- `pkg/` — public components (import surface for consumers)
- `internal/` — private helpers not exported
- `cmd/` — demo binaries
- `examples/launcher/` — the single entry point (`task examples`)
- `examples/<area>/<name>/` — each demo as a package exposing `New()`
- `docs/` — long-form usage notes

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs `go build`, `go vet`, and
`go test` on every push / PR. On merge to `master`, the release job auto-tags
with a semver bump: patch if `bug`/`bugfix`/`fix` appears in the branch name
or merge-commit message, minor otherwise. Starts at `0.1.0`, no `v` prefix.
