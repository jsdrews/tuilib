# Theme tokens

`palette.Palette` holds the colours. Every field is a semantic slot used by
one or more components; writing a new palette is filling in this struct.
`theme.Theme` embeds it, so `t.Accent` reads the palette's accent, and adds
the `Glyphs` and the resolved `Surface`.

| Field | Type | Used for |
|---|---|---|
| `Name` | `string` | Identifier for pickers, the config file and `PaletteEnvVar`. |
| `BarBG`, `BarFG` | color | The breadcrumb, statusbar and anything embedded in them. One pair, so the strip reads as one band. |
| `Current` | color | The active crumb, current selection emphasis. |
| `Muted` | color | Past crumbs, secondary text, unselected buttons. |
| `Subtle` | color | Separators, faint chrome, selected-row background, disabled menu rows. |
| `KeyFG` | color | Key labels in help hints. |
| `BorderActive`, `BorderInactive` | color | Pane borders, focused and not. |
| `InfoBG`, `InfoFG` | color | Info messages on the statusbar. |
| `ErrorBG`, `ErrorFG` | color | Error messages; also the conventional override for an error-tinted alert. |
| `Accent` | color | Bold highlight for body content: cursors, matches, spinners, the selected button. |
| `Background`, `Foreground` | color | The palette's canonical pair: the screen, and text on a filled surface. Every surface tone is derived from them; unset, `BarBG` and `BarFG` stand in. |
| `Surface` | `map[surface.Role]color` | Overrides for derived surface tones, by role. A missing role is derived. |

On `theme.Theme`, alongside the embedded palette:

| Field | Type | Used for |
|---|---|---|
| `Glyphs` | `glyph.Set` | Every drawn mark; empty fields use `glyph.Default()`. |
| `Surface` | `surface.Resolved` | How components are drawn: border shape, slot brackets, fill, overlay and field forms. See [Surfaces](../surfaces.md). |

Colors are `lipgloss.TerminalColor` (e.g. `lipgloss.Color("62")` or
`lipgloss.Color("#7aa2f7")`).

## Glyphs

| Field | Default | Drawn as |
|---|---|---|
| `Cursor` | `▸` | the cursor row in list, logview, the action menu |
| `Mark` | `✓` | a marked row |
| `ExpandOpen`, `ExpandClosed` | `▾`, `▸` | tree and inspector disclosure |
| `ScrollThumb`, `ScrollTrack` | `█`, `░` | vertical scrollbar |
| `HScrollThumb`, `HScrollTrack` | `━`, `─` | horizontal scrollbar |
| `SortAsc`, `SortDesc` | `▲`, `▼` | the sorted table column |
| `ColumnSep` | `│` | table column separator |
| `Placeholder` | `·` | a row a windowed table hasn't received |
| `ActivityOK`, `ActivityFail`, `ActivityChanged` | `✓`, `✗`, `•` | activity outcome marks |

## From Base16

`palette.FromBase16(name, palette.Base16{Base00: "…", …, Base0F: "…"})` maps a
[Base16](https://github.com/chriskempson/base16) scheme (hex without `#`):

| Token | Base16 slot |
|---|---|
| `Background` | base00 |
| `Foreground` | base05 |
| `BarBG` | base01 |
| `BarFG` | base05 |
| `Current` | base06 |
| `Muted` | base04 |
| `Subtle` | base03 |
| `KeyFG` | base0D |
| `InfoFG` on `InfoBG` | base0B on base00 |
| `ErrorFG` on `ErrorBG` | base08 on base07 |
| `Accent` | base0E |

If a scheme reads wrong through this mapping, write the `Palette` by hand.

## Helpers

| Function | Does |
|---|---|
| `palette.All()` | Every built-in palette except `Terminal()`. |
| `palette.ByName(palettes, name)` | Look one up. |
| `palette.Terminal()` | Build from the terminal's own colours; call before `Run`. |
| `theme.New(p)` | `p` drawn Outlined, for code outside the app shell and tests. |
| `theme.Compose(p, s, caps)` | `p` drawn with surface `s` on a terminal offering `caps`; what the shell calls. |
| `t.Resolve(s)` | Resolve another surface against `t`'s palette, to pin one component to it. |
