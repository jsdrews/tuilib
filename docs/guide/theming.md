# Theming and state

## One palette, many components

An app is drawn in a **palette** (the colours) with a **surface** (how each
component's edge and body are drawn: a line border, a filled card, a header
band, a drop shadow). The user picks the two independently, and the app shell
composes them, with the glyphs, into the `theme.Theme` every screen receives
in `SetTheme`. Every component has a builder method on it that returns
pre-styled Options:

```go
opts := t.Table()          // colors, borders, glyphs, keys, filter options
opts.Title = "Pods"
opts.Columns = cols
s.table = table.New(opts)
```

Start from the builder every time. Set only what's specific to the instance,
and don't put colors in Options literals unless you're deliberately
overriding one; a per-instance value always wins over the theme.

| Builder | Returns |
|---|---|
| `t.Pane()` | `pane.Options` |
| `t.List()` | `list.Options` (horizontal scrollbar on, filter styled) |
| `t.Table()` | `table.Options` (bold header, subtle row highlight, `│`/`─` separators) |
| `t.Tree()` | `tree.Options` |
| `t.Inspector()` | `inspector.Options` |
| `t.TextView()` | `textview.Options` |
| `t.Logview()` | `logview.Options` (horizontal scrollbar on) |
| `t.Eventlog()` | `eventlog.Options` |
| `t.Filter()` | `filter.Options` |
| `t.Input()` | `input.Options` |
| `t.Toggle()` | `toggle.Options` |
| `t.Form()` | `form.Options` (chain `.With(fields)`) |
| `t.Confirm()` | `confirm.Options` |
| `t.Alert()` | `alert.Options` |
| `t.Actions()` | `action.Options` |
| `t.HelpOverlay()` | `help.OverlayOptions` |
| `t.Help()` | `help.Options` (footer) |
| `t.Breadcrumb()` | `breadcrumb.Options` |
| `t.Statusbar(left, right)` | `statusbar.Options` |
| `t.Activity()` | `activity.Options` |
| `output.OptionsFrom(t)` | `output.Options`, inverted to avoid an import cycle |

## Built-in palettes

Palettes live in `pkg/palette`:

| Function | `Name` |
|---|---|
| `Dark()` | `dark` |
| `Accent()` | `accent` (colored header strip) |
| `Light()` | `light` |
| `Solarized()` | `solarized` |
| `Nord()` | `nord` |
| `Dracula()` | `dracula` |
| `Gruvbox()` | `gruvbox` |
| `CatppuccinMocha()` / `CatppuccinLatte()` | `catppuccin-mocha` / `catppuccin-latte` |
| `TokyoNight()` | `tokyo-night` |
| `RosePine()` / `RosePineDawn()` | `rose-pine` / `rose-pine-dawn` |
| `OneDark()` | `one-dark` |
| `Monokai()` | `monokai` |
| `EverforestDark()` | `everforest-dark` |
| `Base16Ocean()`, `Base16Eighties()`, `Base16Railscasts()`, `Base16TomorrowNight()` | `base16-…` |
| `Terminal()` | `terminal`: reads your terminal's own palette |

`palette.All()` returns every palette except `Terminal()`, in display order.
`Terminal()` queries the terminal's foreground and background (OSC 10/11) and
uses ANSI indices 0–15 for the rest, so a user with Catppuccin configured in
their terminal gets a Catppuccin UI. Call it **before**
`tea.NewProgram(...).Run()`, which takes over stdin. It falls back to `Dark()`
or `Light()` when the terminal doesn't answer.

### Your own palette

Write a `palette.Palette` literal, or convert any [Base16](https://github.com/chriskempson/base16)
scheme with `palette.FromBase16(name, palette.Base16{Base00: "1d1f21", …})`.
Hex values have no leading `#`. Set `Background` and `Foreground` to the
scheme's own pair: every surface tone is derived from them. Every token is
listed in the [theme token reference](../reference/theme.md).

## User defaults

`app.New` picks the starting palette from, in order, the env var named by
`app.Options.PaletteEnvVar`, the user's config file, and `Palettes[0]`; and
the starting surface the same way, from `SurfaceEnvVar`, the config file, and
`Surfaces[0]`:

```yaml
# ~/.config/tuilib/config.yaml (or $XDG_CONFIG_HOME/tuilib/config.yaml)
palette: dracula        # theme: is still read when palette: is unset
surface: elevated
block_glyphs: false     # this terminal draws ▄ ▀ ▌ with seams
double_click_ms: 400
```

The library never writes this file. A missing file is normal; an unknown name
is ignored. Call `config.Load()` yourself if you want malformed YAML reported.

## Surfaces and glyphs

Five surfaces ship in `pkg/surface`: `Outlined` (the default, a line border),
`Filled`, `Banded`, `Elevated` and `Accented`. Offer them with
`app.Options.Surfaces` and let the user cycle with `SurfaceKey`. A surface is
data, so a custom one is a struct literal; see [Surfaces](../surfaces.md).

Border shape is surface data. `surface.OutlinedWith(lipgloss.RoundedBorder())`
is Outlined drawn rounded; its `Overlay` form (thick lines by default) is what
confirm, alert and the action menu draw, so an overlay reads as floating above
the pane it covers. Its `Brackets` decide how titles meet the border:
`BracketsNone` (`── title ──`), `BracketsCorners` (`┐ title ┌`) or
`BracketsTees` (`─┤ title ├─`). Its `Divider` is the rule under an inline
filter. To pin one component to a different surface:

```go
opts := t.Logview()
opts.Surface = t.Resolve(surface.Outlined)
```

`Glyphs` (a `glyph.Set`) holds every mark components draw: cursor `▸`, mark
`✓`, expand `▾`/`▸`, scrollbar thumb and track, sort `▲`/`▼`, column
separator `│`, placeholder `·`. Empty fields fall back to `glyph.Default()`,
so setting one doesn't blank the rest.

## Rebuilding on a theme swap

When the palette or surface changes, the shell calls `SetTheme(t)` on every screen on the
stack, including covered ones, so they're already in the new palette when
uncovered. A screen rebuilds each themed component and carries its state
across with `State()` and `Restore()`:

```go
func (s *Screen) SetTheme(t theme.Theme) {
    s.t = t

    st := s.table.State()
    opts := t.Table()
    opts.Title = "Pods"
    opts.Columns = podColumns
    s.table = table.New(opts)
    s.table.Restore(st)
}
```

`State` is opaque and covers everything a component holds that Options
doesn't set. For a table that's rows (or the remote window), cursor and
scroll, filter, sort (including a staged remote sort), marks, row activity,
the query its rows answer and whether it failed. For a tree it includes which
branches are open; for a logview the lines, follow mode and current match.
`Restore` replays it in the order that keeps each part valid: rows before the
filter over them, marks after the rows they mark, the cursor last.

Don't restore field by field with individual setters. That list grows every
time a component gains state, and getting the order wrong is easy: a cursor
restored before the sort lands on the wrong row.

A `State` taken from a component that was never built restores nothing, so
the same lines work on the very first `SetTheme` call, from the constructor.
Components that support `State`/`Restore`: `list`, `table`, `tree`,
`inspector`, `logview`, `textview`, `eventlog`, `filter`, `input`, `toggle`.

### Special cases

- **A focus group** over rebuilt components: rebuild it over the same field
  addresses and restore the index:
  ```go
  i := s.focus.Index()
  s.focus = focus.NewGroup(&s.query, &s.results)
  s.focus.SetIndex(i)
  ```
- **Tabs**: `s.tabs.SetTheme(t)` fans the theme out to every body.
- **A `pkg/remote` view**: call `s.view.Restyle(opts)`, which carries the
  state for you.
- **Modals** (`confirm`, `alert`, `action.Menu`) have no `State`; rebuild them
  and set their message or cursor again.

Under the app shell you need no resize handler; the layout engine re-sizes
every component on every render.
