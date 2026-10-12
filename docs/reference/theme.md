# Theme tokens

`theme.Theme` is the whole palette. Every field is a semantic slot used by one
or more components; writing a new theme is filling in this struct.

| Field | Type | Used for |
|---|---|---|
| `Name` | `string` | Identifier for pickers, the config file and `ThemeEnvVar`. |
| `BarBG`, `BarFG` | color | The breadcrumb, statusbar and anything embedded in them. One pair, so the strip reads as one band. |
| `Current` | color | The active crumb, current selection emphasis. |
| `Muted` | color | Past crumbs, secondary text, unselected buttons. |
| `Subtle` | color | Separators, faint chrome, selected-row background, disabled menu rows. |
| `KeyFG` | color | Key labels in help hints. |
| `BorderActive`, `BorderInactive` | color | Pane borders, focused and not. |
| `BorderShapeActive`, `BorderShapeInactive` | `lipgloss.Border` | Component border shapes. Zero means the normal border. |
| `BorderShapeOverlay` | `lipgloss.Border` | Confirm, alert, action menu. Zero means thick. |
| `SlotBrackets` | `pane.SlotBracketStyle` | How titles meet borders. |
| `Glyphs` | `glyph.Set` | Every drawn mark; empty fields use `glyph.Default()`. |
| `InfoBG`, `InfoFG` | color | Info messages on the statusbar. |
| `ErrorBG`, `ErrorFG` | color | Error messages; also the conventional override for an error-tinted alert. |
| `Accent` | color | Bold highlight for body content: cursors, matches, spinners, the selected button. |

Colors are `lipgloss.TerminalColor` (e.g. `lipgloss.Color("62")` or
`lipgloss.Color("#7aa2f7")`).

## Glyphs

| Field | Default | Drawn as |
|---|---|---|
| `Cursor` | `▸` | the cursor row in list, logview, the action menu |
| `Mark` | `✓` | a marked row |
| `ExpandOpen`, `ExpandClosed` | `▾`, `▸` | tree and inspector disclosure |
| `Rule` | `─` | the line under an inline filter |
| `ScrollThumb`, `ScrollTrack` | `█`, `░` | vertical scrollbar |
| `HScrollThumb`, `HScrollTrack` | `━`, `─` | horizontal scrollbar |
| `SortAsc`, `SortDesc` | `▲`, `▼` | the sorted table column |
| `ColumnSep` | `│` | table column separator |
| `Placeholder` | `·` | a row a windowed table hasn't received |
| `ActivityOK`, `ActivityFail`, `ActivityChanged` | `✓`, `✗`, `•` | activity outcome marks |

## From Base16

`theme.FromBase16(name, theme.Base16{Base00: "…", …, Base0F: "…"})` maps a
[Base16](https://github.com/chriskempson/base16) scheme (hex without `#`):

| Token | Base16 slot |
|---|---|
| `BarBG` | base01 |
| `BarFG` | base05 |
| `Current` | base06 |
| `Muted` | base04 |
| `Subtle` | base03 |
| `KeyFG` | base0D |
| `InfoFG` on `InfoBG` | base0B on base00 |
| `ErrorFG` on `ErrorBG` | base08 on base07 |
| `Accent` | base0E |

If a scheme reads wrong through this mapping, write the `Theme` by hand.

## Helpers

| Function | Does |
|---|---|
| `theme.All()` | Every built-in theme except `Terminal()`. |
| `theme.ByName(themes, name)` | Look one up. |
| `theme.Resolve(themes, envVar)` | Reorder so the env var's or config file's choice is first. |
| `theme.Terminal()` | Build from the terminal's own palette; call before `Run`. |
