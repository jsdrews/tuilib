# tuilib

A component library for building Bubble Tea TUIs quickly and reliably. Each
component is a small, well-documented `pkg/` with an `Options` struct, a
`New(Options)` constructor, and standard Bubble Tea `Init/Update/View`
methods. A central `theme` package collapses the color palette into one struct
so every component renders in the same palette without drift.

**Documentation: <https://jsdrews.github.io/tuilib/>**: install, quick start,
architecture, and a reference for every component. Build it locally with
`task docs:serve`.

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
    st := s.list.State() // cursor, filter, items… everything the user did
    opts := t.List()
    opts.Title = "Cities"
    opts.Items = []string{"London", "Tokyo", "Madrid", "Lima"}
    opts.Filterable = true
    s.list = list.New(opts)
    s.list.Restore(st)
}

func main() {
    m := app.New(app.Options{
        Root:     &cities{},
        Palettes: []palette.Palette{palette.Nord()},
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
| `pkg/menu` | Grouped, filterable list of things to open, such as an app's sections. `Item{ID, Label, Group, Description, Data}`: headers the cursor skips (only once there are two or more groups; ungrouped items last under `other`), a dimmed description cut off at the edge, `[`/`]` to jump between groups, `/` to filter on label and group. Enter or double-click emits `ChosenMsg`; `Chosen(msg)` matches it, and the screen decides what opening means. Groups follow the same rules as `Action.Group` in the action menu |
| `pkg/table` | Cursor-driven, optionally filterable tabular view inside a pane. `Column{Title, Width, Align, Sortable, Less, Flex, MaxWidth}` declares the layout; rows are `[]string` cells. Header pins to the top while scrolling horizontally with the body so columns stay aligned. ANSI-aware truncation via `x/ansi.Cut` — `Width` is the visible cell width. Sizing: `Width > 0` → fixed; `Width == 0` → content-auto (max of title + any cell, ANSI stripped, floor 4); `Flex > 0` → absorbs a share of leftover inner width by weight (Width is the min, `MaxWidth` is the cap; surplus from capped columns redistributes to uncapped flex columns). Recomputes on row/column/dimension changes, so flex columns reflow on resize. Same nav verbs as `pkg/list` (`g`/`G`, `ctrl+u/d`, `↑↓`/`j`/`k`); filter matches across all cells (ANSI stripped before matching) and accepts space-separated AND-ed terms; a `key:value` term scopes the match to the column whose Title starts with `key` (e.g. `region:europe pop:5`); a `~` prefix on the value makes that term a case-insensitive regex (e.g. `~^new`, `region:~^euro`). Mid-typing a `key:val` term shows the column's distinct matching values in the filter's bottom-left slot, and `tab` completes to the longest common prefix. `SetKeyedRows([]KeyedRow{Key,Cells})` + `SelectedKey()` snap the cursor to the same Key after a swap (the auto-refresh primitive — pair with `pkg/poll`). `Sortable` columns expose `[`/`]` (step active sort column) + `s` (toggle direction); supply `Column.Less` for numeric or unit-aware sort. `Options.Borders{Vertical, HeaderRule}` configures the inter-column glyph and the horizontal rule below the header — both are pre-styled glyph strings (`pkg/ansi.CellColor` keeps the selected-row bg intact); `theme.Table()` ships with subdued `│`/`─` defaults. `Options.Markable` adds a multi-selection: `space` toggles the cursor row, `X` extends from the last-marked row to the cursor in either direction (additive; shift+click does the same), `A` marks every row the filter shows, `D` clears outright, and clicking the `✓` gutter toggles; marks are held by Key, so they survive filtering, a keyed swap and a theme rebuild, and are inert on anonymous rows. Read `Selection()` — the marked keys, or the cursor's when nothing is marked — and `SelectionLabel()` ("3 items") (not available under `SetWindow`, which carries rows without keys) |
| `pkg/input` | Single-line text input in a pane; bare textbox without filter's commit/cancel keys |
| `pkg/toggle` | Yes/no selector in a pane — left/right/space/y/n |
| `pkg/confirm` | Modal yes/no dialog with title + message + confirm/cancel buttons; resolves via `ConfirmedMsg` / `CancelledMsg` so parent screens stay bubbletea-idiomatic. Designed for `layout.ZStack(base, layout.Center(w, h, ...))` |
| `pkg/alert` | Modal acknowledgement dialog with title + message + single OK button; resolves via `DismissedMsg`. Use for "stop and acknowledge" feedback (errors, blocking notices); for passive feedback prefer the lighter `app.Info` / `app.Error` statusbar messages. Override `ActiveColor` with `theme.ErrorBG` for an error-tinted look |
| `pkg/remote` | Binds a `table` or `eventlog` to a remote source so a screen writes only the fetch: `NewEventlog` / `NewTable` over `Seekable{Page, Find}` or `Anchored{Edge, Find}` — plain functions over typed requests — then `Init`, `Update`, `Restyle`. Paging, following, search landing and theme swaps are the binding's |
| `pkg/eventlog` | Events and lines from a paged remote source: items (key, lines, data) with a cursor per item, a merged range (Seekable) or a span grown at either edge (Anchored), follow with a new-items count, search that jumps past what is loaded, a filter that narrows the query, and the table's stale/loading/failed border signals. See CLAUDE.md rule 34 |
| `pkg/resume` | Exact resume for a stream whose "since" is coarser than its lines (pod logs): tracks the last second and the lines seen in it, and where a longer tail's older lines stop |
| `pkg/logview` | Streaming text viewer with `/`-search, n/N jump, g/G top/bottom, filter mode, current-line highlight, and a default `MaxLines` safety cap |
| `pkg/tree` | Searchable, expand/collapse hierarchical viewer over any `Node` (Label + Children); `/`-search highlights inline and `\` hides non-matching subtrees while keeping ancestors. `Options.Markable` adds a multi-selection keyed on each node's path — `x` toggles (space stays expand/collapse), `X` or shift+click extends a range, `A` marks every visible row, `D` clears; marking a branch marks that node alone, and paths are hierarchical so a caller can prefix-test for the subtree. Labels may contain lipgloss-styled ANSI (colored status icons, etc.) — the cursor's row highlight stays intact across colored segments |
| `pkg/inspector` | Two-column label/value viewer for structured records (k8s manifests, REST responses, Prefect run details). `Field{Label, Value, Children}` composes; `FromAny` / `FromMap` convert `json.Unmarshal` output into Fields. Sibling labels auto-align per group, ▸/▾ expand nested objects/arrays, `/` searches labels and values, `\` hides non-matching subtrees. `SetFields` preserves expansion state + cursor by row path across swaps — the auto-refresh primitive for inspector |
| `pkg/poll` | Interval-driven `tea.Cmd` ticker for auto-refresh. Emits a typed `RefreshMsg` parents match in their `Update` to kick off a refetch; `Pause`/`Resume`/`SetInterval`/`Refresh` (now) / `MarkRefreshed`. Pair with the keyed-row APIs on `pkg/list` and `pkg/table` (or path-keyed `SetFields` on `pkg/inspector`) so cursor + expansion survive every swap |
| `pkg/metrics` | Inline rendering primitives for list rows / table cells / inspector values: `Badge(ok, warn, down)` (status-count summary), `Ratio(done, total)` (severity-colored "N/M"), `Bar(value, max, width)` (fixed-width progress bar), `Spark(values, width)` (8-step block sparkline that resamples to fit). All return ANSI-safe foreground-only strings via `pkg/ansi.CellColor` so the selected-row background passes through. Rendering-only — caller owns history buffer for sparklines |
| `pkg/form` | Vertical layout of `input` + `toggle` (+ Select) fields with tab cycling and a submit button |
| `pkg/tab` | Tabbed container hosting multiple `screen.Screen` bodies behind a one-row strip. Each body keeps its own state across switches; `shift+left`/`shift+right` and `1`–`9` switch tabs (`tab`/`shift+tab` is left alone for inner pane focus cycling). Host screen forwards `Update`/`OnEnter`/`IsCapturingKeys`/`SetTheme`/`Help` to `tabs` |
| `pkg/runner` | Hand the terminal to an interactive subprocess (vim, htop, less, ssh) and resume the TUI on exit. Clears the screen on handoff by default; `RunWithNotice` prints a transitional line for slow handoffs (kubectl exec, ssh); `RunWith(Options{...})` for full control |
| `pkg/theme` | A palette drawn with a surface, plus the per-component `Options` builders. `app.New` composes it from the starting palette and surface (env var, then the user config file; `SkipConfig=true` opts out) |
| `pkg/palette` | The colours: the `Palette` struct and the 19 built-ins |
| `pkg/surface` | How edges and bodies are drawn, as data — `Outlined`, `Filled`, `Banded`, `Elevated`, `Accented` — and the colour roles derived from a palette. See `docs/surfaces.md` |
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
provide a root `screen.Screen`, a list of palettes and optionally of
surfaces; the shell does the rest.

```go
m := app.New(app.Options{
    Root:       newCityList(),
    Palettes:   []palette.Palette{palette.Nord(), palette.Dark()},
    Surfaces:   []surface.Surface{surface.Outlined, surface.Elevated},
    Version:    "v0.1.0",
    ThemeKey:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "palette")),
    SurfaceKey: key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "surface")),
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
See `examples/navigation/stack`.

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
lifetime. See `examples/workbench` for the full pattern.

## Theming

An app is drawn in a **palette** with a **surface**, which the user picks
independently. A `palette.Palette` is a struct of semantic colour tokens
(`BarBG`, `KeyFG`, `Accent`, `BorderActive`, …); built-ins include `Dark`,
`Light`, `Nord`, `Dracula`, `Gruvbox`, `Solarized`, `TokyoNight`,
`CatppuccinMocha/Latte`, `RosePine/Dawn`, `OneDark`, `Monokai`,
`EverforestDark`, four `Base16…` schemes, and a `Terminal()` that reads the
user's actual terminal colours at startup. A surface is how components'
edges and bodies are drawn: `Outlined` (a line border, the default),
`Filled`, `Banded`, `Elevated` or `Accented` — see `docs/surfaces.md`. The
shell composes the pair into a `theme.Theme`, whose builders hand every
component pre-styled options, so nothing drifts:

```go
th := theme.New(palette.Nord()) // Nord, drawn Outlined
bc := breadcrumb.New(th.Breadcrumb())
p  := pane.New(th.Pane())
fl := filter.New(th.Filter())
li := list.New(th.List())
sb := statusbar.New(th.Statusbar(helpModel.ShortView(), "v0.1.0"))
```

### Default palette and surface via env / config

`app.New` picks the starting palette by checking, in order, the env var
named by `Options.PaletteEnvVar`, the `palette:` field (or the older
`theme:`) in `$XDG_CONFIG_HOME/tuilib/config.yaml` (falls back to
`~/.config/tuilib/config.yaml`, via `pkg/config`), and `Palettes[0]`; and
the starting surface the same way, from `SurfaceEnvVar`, `surface:`, and
`Surfaces[0]`:

```go
m := app.New(app.Options{
    Root:          newRoot(),
    Palettes:      palette.All(),
    Surfaces:      surface.Builtins(),
    PaletteEnvVar: "MY_APP_PALETTE",
    // ...
})
```

User-side, `~/.config/tuilib/config.yaml`:

```yaml
palette: dracula
surface: elevated
block_glyphs: false   # this terminal draws ▄ ▀ ▌ with seams
```

The library never writes the file and never creates the directory — config
is opt-in. A missing file is the steady state. Unknown names fall through
silently (typo `dracla` just leaves `Palettes[0]` as the default);
malformed YAML surfaces as an error only if you call `config.Load`
directly. Set `Options.SkipConfig = true` to ignore the env vars and the
file entirely (useful for tests, or when the app should always pin to its
first palette and surface). When the terminal can't draw the chosen
surface, the shell draws its fallback and says why in the output console.

The shared file lives in `pkg/config` — as other components grow
user-tunable knobs (logview defaults, key remaps, custom palettes) their
fields will land on `config.Config` and any package can consult the same
file without going through `pkg/theme`.

## Examples

Run `task examples` to open a launcher with a menu of showcases. Select one
and press enter to drill in; esc pops back to the menu. The launcher itself
is just `pkg/app` hosting a `menu` — the same pattern every showcase uses.

Each entry is a showcase: many features on one screen, cooperating, so a
change that breaks an older feature shows up where you can see it.

| Showcase | What it puts together |
|---|---|
| Workbench | Every row component over one cluster, in one `focus.Group`: a filterable, sortable, markable `table`; a searchable, markable `tree`; a markable `list` following the table; an `inspector` following whichever pane last had focus; a grouped `menu`. The action menu (`a`, right-click) offers verbs for the focused pane's selection; the operations menu runs statusbar notices, `pkg/confirm` and `pkg/alert`; the mouse does everything |
| Live ops | Data that changes while you watch: row activity against demoapi's Argo-shaped API with one verb per operation shape, `pkg/metrics` cells (`Ratio`, `Badge`, `Bar`, `Spark`), an events log, one `pkg/poll` driving all three panes, and every pane loading until its first data |
| Remote data | Data too large for one request, one `pkg/tab` tab per shape: a paged table where scrolling is the pagination, an AWX job's events (running and finished), an Elasticsearch-style index read with `search_after`, and a pod log with exact reconnects |
| Processes | Commands three ways: `runner.Capture` streaming into a searchable logview while the TUI stays live, `runner.Run` handing the terminal to an editor or pager, and statusbar notices the output console (`o`) keeps — with the `OnEnter` guard on `app.OutputClosed` |
| Navigation | A grouped, filterable `menu` home that opens a screen stack (push, pop with a result, replace), a screen pushed in front of another, a master-detail drilldown, and a tab host; the breadcrumb names every level and a click on a crumb unwinds to it |
| Appearance | Palette, surface and glyph pickers that redraw the whole app live, beside what each kind of component is handed; then tabs for a titled form, a textview, pane variants, and one screen per layout primitive |

Each showcase is a package under `examples/` exporting
`New(theme.Theme) screen.Screen`, with tests that assert what its package
comment promises. The tabbed ones keep each tab in its own sub-package, which
can be read, copied and tested alone.

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
5. `examples/<name>/` — each showcase is a package exposing
   `New(theme.Theme) screen.Screen`. Read `examples/launcher/main.go` to
   see the composition pattern (the showcases as a menu screen).
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
- `examples/workbench/`, `liveops/`, `remotedata/`, `processes/`,
  `navigation/`, `appearance/` — the showcases, each a package exposing `New()`
- `docs/` — long-form usage notes

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs `go build`, `go vet`, `go
test`, and the integration tests on every push / PR.

The integration tests (`internal/integration`) drive whole screens against
`demoapi` on real timers, so they sit behind a build tag to keep `go test ./...`
fast. Run them with `task test:integration` or
`go test -tags integration ./internal/integration/...`; `task check` runs
everything. On merge to `master`, the release job auto-tags
with a semver bump: patch if `bug`/`bugfix`/`fix` appears in the branch name
or merge-commit message, minor otherwise. Starts at `0.1.0`, no `v` prefix.
