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
| `pkg/help` | Key hints: `Model` (inline footer) + `Overlay` (the `?` modal — grouped by function, scrollable, searchable). `Section` + `Sectioned` are the contract every component implements; `Group` / `Sections` / `SectionsOf` build them, `Flatten` derives `Help()` from them, `Qualify` prefixes a group with its pane and `Suppress` drops what the shell's globals already claim |
| `pkg/filter` | Textinput in a pane; "/" to focus, enter commits, esc clears |
| `pkg/list` | Cursor-driven, optionally filterable list inside a pane. `SelectedIndex()` returns the underlying source-slice index even when items are formatted display strings and a filter is active. `SetKeyedItems([]KeyedItem{Key,Display})` + `SelectedKey()` snap the cursor to the same Key after a swap (the auto-refresh primitive — pair with `pkg/poll`). Vim-style nav: `g`/`G` top/bottom, `ctrl+u`/`ctrl+d` half-page, plus `↑↓` per row. `Options.Markable` adds a multi-selection: `space` toggles the cursor row, `X` extends from the last-marked row to the cursor in either direction (additive; shift+click does the same), `A` marks every row the filter shows, `D` clears outright, and clicking the `✓` gutter toggles; marks are held by Key, so they survive filtering, a keyed swap and a theme rebuild, and are inert on anonymous rows. Read `Selection()` — the marked keys, or the cursor's when nothing is marked — and `SelectionLabel()` ("3 items") |
| `pkg/table` | Cursor-driven, optionally filterable tabular view inside a pane. `Column{Title, Width, Align, Sortable, Less, Flex, MaxWidth}` declares the layout; rows are `[]string` cells. Header pins to the top while scrolling horizontally with the body so columns stay aligned. ANSI-aware truncation via `x/ansi.Cut` — `Width` is the visible cell width. Sizing: `Width > 0` → fixed; `Width == 0` → content-auto (max of title + any cell, ANSI stripped, floor 4); `Flex > 0` → absorbs a share of leftover inner width by weight (Width is the min, `MaxWidth` is the cap; surplus from capped columns redistributes to uncapped flex columns). Recomputes on row/column/dimension changes, so flex columns reflow on resize. Same nav verbs as `pkg/list` (`g`/`G`, `ctrl+u/d`, `↑↓`/`j`/`k`); filter matches across all cells (ANSI stripped before matching) and accepts space-separated AND-ed terms; a `key:value` term scopes the match to the column whose Title starts with `key` (e.g. `region:europe pop:5`); a `~` prefix on the value makes that term a case-insensitive regex (e.g. `~^new`, `region:~^euro`). Mid-typing a `key:val` term shows the column's distinct matching values in the filter's bottom-left slot, and `tab` completes to the longest common prefix. `SetKeyedRows([]KeyedRow{Key,Cells})` + `SelectedKey()` snap the cursor to the same Key after a swap (the auto-refresh primitive — pair with `pkg/poll`). `Sortable` columns expose `[`/`]` (step active sort column) + `s` (toggle direction); supply `Column.Less` for numeric or unit-aware sort. `Options.Borders{Vertical, HeaderRule}` configures the inter-column glyph and the horizontal rule below the header — both are pre-styled glyph strings (`pkg/ansi.CellColor` keeps the selected-row bg intact); `theme.Table()` ships with subdued `│`/`─` defaults. `Options.Markable` adds a multi-selection: `space` toggles the cursor row, `X` extends from the last-marked row to the cursor in either direction (additive; shift+click does the same), `A` marks every row the filter shows, `D` clears outright, and clicking the `✓` gutter toggles; marks are held by Key, so they survive filtering, a keyed swap and a theme rebuild, and are inert on anonymous rows. Read `Selection()` — the marked keys, or the cursor's when nothing is marked — and `SelectionLabel()` ("3 items") (not available under `SetWindow`, which carries rows without keys) |
| `pkg/input` | Single-line text input in a pane; bare textbox without filter's commit/cancel keys |
| `pkg/toggle` | Yes/no selector in a pane — left/right/space/y/n |
| `pkg/confirm` | Modal yes/no dialog with title + message + confirm/cancel buttons; resolves via `ConfirmedMsg` / `CancelledMsg` so parent screens stay bubbletea-idiomatic. Designed for `layout.ZStack(base, layout.Center(w, h, ...))` |
| `pkg/alert` | Modal acknowledgement dialog with title + message + single OK button; resolves via `DismissedMsg`. Use for "stop and acknowledge" feedback (errors, blocking notices); for passive feedback prefer the lighter `app.Info` / `app.Error` statusbar messages. Override `ActiveColor` with `theme.ErrorBG` for an error-tinted look |
| `pkg/logview` | Streaming text viewer with `/`-search, n/N jump, g/G top/bottom, filter mode, current-line highlight, and a default `MaxLines` safety cap |
| `pkg/tree` | Searchable, expand/collapse hierarchical viewer over any `Node` (Label + Children); `/`-search highlights inline and `\` hides non-matching subtrees while keeping ancestors. `Options.Markable` adds a multi-selection keyed on each node's path — `x` toggles (space stays expand/collapse), `X` or shift+click extends a range, `A` marks every visible row, `D` clears; marking a branch marks that node alone, and paths are hierarchical so a caller can prefix-test for the subtree. Labels may contain lipgloss-styled ANSI (colored status icons, etc.) — the cursor's row highlight stays intact across colored segments |
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
- `Sized(&c)` adapts any `SetRect(geom.Rect) + View()` component (pane, list, …).
- `Bar(&c)` is `Sized` under a name that says "this renders one row"
  (breadcrumb, statusbar).
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
See `examples/shell/stack`.

`IsCapturingKeys()` tells the shell when a screen owns input (e.g. filter
is focused) so global keys like `q`, `t`, and esc-pop are suppressed.

**Key hints and the `?` overlay.** The statusbar's left slot shows a
`? help` affordance; pressing `?` opens `help.Overlay`, a scrollable,
searchable modal listing every binding the active screen exposes.

Bindings are **grouped by what they do, not by who owns them**. Every
interactive component implements `HelpSections() []help.Section` and
derives its flat `Help()` from it, so the footer strip and the overlay can
never disagree:

```go
func (m Model) Help() []key.Binding { return help.Flatten(m.HelpSections()) }
```

Group names come from one shared vocabulary — `help.SectionNavigate` /
`Scroll` / `Filter` / `Search` / `Select` / `Sort` / `Expand` / `View` /
`Edit` / `Submit` / `Tabs` — so "Navigate" means the same thing in a list,
a table and a tree. A screen composes its components' groups with its own
verbs via `help.SectionsOf`:

```go
func (s *Screen) HelpSections() []help.Section {
    return help.SectionsOf(&s.table, help.Group("Deployments", s.verbs()...))
}
```

Name that group after the *subject* ("Deployments", "Cities"), never after
the screen or component that owns it — an owner's name ends up standing
over every binding it holds, including the scroll keys it doesn't
describe. The owner is a qualifier instead, applied by `help.Qualify`
("files · Navigate") only when more than one pane is on screen.

A screen with several interactive components forwards
`focus.Group.HelpSections()`; a `pkg/tab` host forwards
`tabs.HelpSections()`, which carries the strip's keys plus the *active*
body's groups. A screen that implements nothing still works — it falls
back to a single group titled with its own name, which is exactly the flat
list the footer would have shown.

The shell writes the **Global** group itself (quit or esc-back depending on
stack depth, theme, suspend, and the opt-in output and actions keys a
screen has no reason to know about), then `help.Suppress` drops from the
screen's own groups anything those globals already claim — so a screen that
lists `q` and `t` in its `Help()` isn't punished for it. Repeats *across*
panes are kept: two panes binding `↑/k` are two different verbs.

Configure with `app.Options.HelpKey` (the key), `HelpVerbose` (spell as
many bindings as fit inline in the footer) and `DisableHelpSearch`.

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
lifetime. See `examples/shell/status` for the full pattern.

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

Entries are grouped three ways, and the directories match, so the path tells
you what kind of thing you are about to open. Filtering in the launcher
searches the group as well as the name, so `/patterns` narrows to one
section.

### `examples/components/` — one per component, in isolation

| Entry | Demonstrates |
|---|---|
| Pane | Border styles, title positions, and slot-bracket variants in one 2×2 grid. Every interactive component owns one of these internally, which is why none of them need a wrapper |
| List | A filterable `list.Model` with 372 numbered rows, so every scroll affordance is reachable: drag the thumb, click the track, wheel over an unfocused pane, `g`/`G`, `ctrl+u/d`, double-click to open |
| Table | `table.Model` with a sticky header and all three column sizing modes side by side (City `Flex:1` + `MaxWidth:28`, Region/Population fixed, Status content-auto). `[`/`]` step the sort column, `s` toggles direction, Population sorts numerically via a custom `Less` that parses "8.3M". Status uses `ansi.CellColor` so the selected-row background passes through colored cells; Wiki wraps each URL in `ansi.Hyperlink` so shift-click launches the full link even when the column truncates it |
| Tree | Synthetic project tree with cursor, expand/collapse (`space`), `/`-search, and `\` filter mode that hides non-matching subtrees while keeping ancestors. Leaves carry colored status icons so the row highlight survives ANSI segments |
| Inspector | Two-column label/value viewer over a synthetic k8s-pod payload, fed through `inspector.FromAny` — the typical `json.Unmarshal` → `FromMap` path. Sibling labels auto-align per group; `/` searches labels and values |
| TextView | Two documents (README + git diff) cycled with `d`. `/`-search with `n`/`N`, `w` toggles wrap, `g`/`G` and `ctrl+u/d` scroll. No follow, no `MaxLines` — the read-static-text counterpart to logview |
| LogView | Streaming log tail with `/`-search, `n`/`N` jump, `\` filter-mode toggle, current-line highlight, and a `MaxLines` cap so an open-ended producer can't grow memory without limit |
| Form | `form.Model` with Text / Select / Confirm fields + submit. Validation lives in the form, not the screen: errors render on the field's own border so nothing reflows while you correct it |
| Metrics | `pkg/metrics` inline primitives in a live deployments table — `Ratio` for replica counts, `Badge` for pod-state breakdown, `Bar`+percent for CPU, `Spark` for CPU history. The point is that they drop into existing components as ordinary cells: no new screens, no custom layout. Polls every 2s with a keyed cursor |

### `examples/shell/` — what `pkg/app` owns

| Entry | Demonstrates |
|---|---|
| Layouts | One sub-screen per layout primitive: `HStack`+`Fixed`/`Flex`, nested stacks, `ZStack` modal, and more — the answer to every "how do I leave room for a bar" question that would otherwise be written as `m.h - 2` |
| Stack | Navigation plus all three directions data moves: parent→child through the constructor, child→parent through `Pop(result)` landing in `OnEnter`, and self→self through `Replace`. `r` on either screen swaps it for a fresh instance — filter and visit counter reset, depth stays put, and the screen underneath is never reactivated |
| Tabs | Three sub-screens (filterable list / streaming logview / counter) behind one strip; switch via shift+arrows, `1`/`2`/`3`, or a click on a label. Each body keeps its own state, and the logs keep streaming while you are on another tab |
| Status | Screens emit `app.Info` / `app.Error` / `app.ClearStatus` as `tea.Cmd`s and the shell paints the statusbar's center slot. Auto-clears on the next keypress, which is why anything worth reading twice belongs in the console |
| Output | The app-wide console: `app.Info` / `app.ErrorDetail` / `app.ErrorOf`, plus `runner.Capture` streaming a live subprocess. `o` opens it, `c` clears, `x` kills a running capture, `w` exports. Note the `OnEnter` guard on `app.OutputClosed` — closing the console must not look like a fresh activation |
| Themes | Live palette picker — moving the cursor re-skins the whole app, enter shows a theme's field palette. Also the package every other example's theme list comes from |
| Chrome | The two border-shape slots a `Theme` carries, one for ordinary components and one for overlays. Pick a shape and every component on screen is rebuilt from the modified theme. Set both pickers the same to see why the overlay slot is separate: the modal flattens into the content behind it |
| Prescreen | The "log in before you can use this" shape — a root screen pushes a child from `OnEnter`, takes its result on `Pop`, and can re-push it later (`L` logs out) without the child living permanently on the stack |

### `examples/patterns/` — composition idioms

| Entry | Demonstrates |
|---|---|
| Focus | `input` + `list` + `toggle` behind one `focus.Group`: tab cycles, only the focused component takes keys, and a clicked component gets focus because the Group sees the request it sends |
| Filters | A list and a table, each with its own filter, on one screen — the focus states a single filterable pane can't reach: one region highlighted at a time, clicking a body taking input back from its filter, switching panes clearing the filter you left, `tab` completing a `key:value` term instead of cycling panes |
| Mouse | Click to focus, click a row to select, double-click to open, click a table header to sort or a tree `▸` to expand, wheel over any pane focused or not, drag a scrollbar. Every rect comes from `pkg/layout` — nothing is injected into the rendered string |
| Loading | `list`, `logview`, and `tree` all start in `SetLoading(true)`; staggered `tea.Tick` delays simulate fetches that resolve at different times. `r` refetches, and the spinner replaces the previous result rather than overlaying it |
| Drilldown | Master-detail with async fetches at every level. Enter on either pane "opens the focused selection": left-enter loads the detail (reqID-tagged so stale results drop) and shifts focus right, right-enter pushes a child screen |
| Poll | `pkg/poll` drives a 2s tick that mutates a synthetic job list; `SetKeyedItems` keeps the cursor on the same job ID across every refresh even as statuses flip and the list reorders. `p` pauses, `r` refreshes now, `+`/`-` adjust cadence |
| Remote | The whole windowed-source loop: `pkg/source` coordinating a table in `FilterRemote`/`SortRemote` over a simulated 5,000-row API that answers one 100-row page at a time with 250ms of latency. Scroll faster than it answers and you see the `·` placeholders; the cursor stays put and data arrives under it |
| Actions | The verb menu. `a` or right-click opens `action.Menu`, sized to its widest row and anchored where you asked. Single-target on purpose. Start a Restart and reopen the menu to see the `Exclusive` gate; Delete confirms first |
| Multi-select | `table.Model` with `Options.Markable` + `pkg/action`: `x` marks, `X` (or shift+click) extends a range in either direction, `A` marks everything the filter shows, `D` drops. Marks are held by Key, so they survive filtering and a theme swap. The menu is titled with what it will act on ("3 items"), and the action that did not declare `Multi` dims itself with a reason |
| Tree actions | `pkg/tree` marking + `pkg/action` on a cluster hierarchy. Marking a branch marks that branch alone; the screen resolves it to the pods a verb should touch with a prefix test on the path, which is why Restart cascades into a marked namespace and Describe does not |
| Modals | The three weights of feedback on one list of operations: `app.Info` for a success, `pkg/confirm` for a destructive op, `pkg/alert` for a failure the user must acknowledge. "Force push main" goes through both modals in sequence. Note the two hosting shapes — fixed-size inside `layout.Center` vs. an autosizing alert that centers itself |
| Runner | Handing the terminal to an interactive subprocess: the TUI suspends, `$EDITOR` / `less` / `man` / `htop` gets the real TTY, and it resumes on exit. The last entry uses `RunWithNotice` to print "connecting…" during the handoff |
| Capture | The counterpart: `runner.Capture` streams a subprocess into an on-screen logview while the TUI stays live. The shell chains the reads and forwards every message, so the screen just matches `CaptureStarted` / `CapturedLine` / `Captured` — no `io.Pipe`, no goroutine, no scanner |

Each entry is a package under `examples/<area>/<name>/` exporting
`New(theme.Theme) screen.Screen`. The launcher imports them all and pushes the
chosen one onto its stack. When adding a demo, pick the area by what a reader
is looking for rather than by which package it imports: a screen that exists
to show one component's options is a component demo, one that shows two
components cooperating is a pattern.

## Learning the library

### For humans

1. **Run the launcher.** `task examples`, then drill into Shell · Stack (nav
   + data flow), Shell · Layouts (layout primitives), or Shell · Themes
   (live palette preview). Each entry is self-contained and shows one idiom,
   and the about pane names the directory its code lives in.
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
- `examples/components/`, `examples/shell/`, `examples/patterns/` — each demo
  as a package exposing `New()`
- `docs/` — long-form usage notes

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs `go build`, `go vet`, and
`go test` on every push / PR. On merge to `master`, the release job auto-tags
with a semver bump: patch if `bug`/`bugfix`/`fix` appears in the branch name
or merge-commit message, minor otherwise. Starts at `0.1.0`, no `v` prefix.
