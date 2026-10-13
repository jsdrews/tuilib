# Surface styles in popular TUIs

Research for #113 (map #111, "surfaces — card-style theming"). Vocabulary is
CONTEXT.md § Surfaces: **Outlined**, **Filled**, **Banded**, **Elevated**.
Sources are primary (source code, official docs) and were read on 2026-10-12;
links point at `main`/`master` unless a version is named.

## Answer in brief

- Every framework surveyed treats the surface as **a per-glyph table of edge
  characters**, and the frameworks that do filled looks (Textual, Ratatui)
  add one more fact per glyph: **which background it sits on** — the
  widget's, the parent's, or a reversed mix of the two. That second fact is
  what makes half-block and eighth-block edges work, and it is the part a
  plain "border chars" interface (lipgloss, superfile, btop) cannot express.
- Textual already ships tuilib's four surfaces under other names: `panel` is
  Banded, `block`/`outer`/`inner` are Filled variants, `tall`/`wide` are
  "filled with an eighth-block lip", and `round` is Outlined. Elevated (a drop
  shadow) is the only one Textual lacks; Ratatui added `Block::shadow` in
  v0.30.1 (June 2026).
- Focus is shown four ways across the field: **border colour** (lazygit, k9s,
  Zellij, Harlequin, Posting, superfile, Textual inputs), **fill tint**
  (Textual `DataTable`/`ListView`: `background-tint: $foreground 5%`),
  **a left accent bar** (Huh: thick left border vs. hidden; Posting/Harlequin
  `border-left: wide $accent`), and **band colour** (Zellij `frame_highlight`,
  Textual `panel` takes its border colour as the band). Apps choose; none
  combine more than two.
- Theme authors almost never define a new surface shape. They pick from a
  fixed enum (Textual `border:`, lazygit `border:`, Zellij
  `pane_frame_style`, Helix `popup-border`, btop `rounded_corners`) and set
  colours. Only Ratatui (`border::Set`, `Shadow::custom`), lipgloss
  (`Border{}` struct) and superfile (11 glyph keys in config) let you supply
  glyphs.
- Rendering pitfalls are real and well documented: block glyphs leave gaps
  unless the terminal draws them itself (kitty, WezTerm, Alacritty, Ghostty
  do; macOS Terminal.app does not and Textual's FAQ prescribes line spacing
  0.805), surface tones collapse in 256-colour (Nord's base and a 6% lift
  both land on xterm 237), and tmux hides truecolor unless told (`RGB`
  terminal feature) — termenv then picks ANSI256.

## Frameworks

### Textual (Python)

**Border types.** `VALID_BORDER` in
[`css/constants.py`](https://github.com/Textualize/textual/blob/main/src/textual/css/constants.py)
is a closed set: `ascii blank dashed double heavy hidden hkey inner none outer
panel round solid tall tab thick block vkey wide`. Glyphs are in
[`_border.py` `BORDER_CHARS`](https://github.com/Textualize/textual/blob/main/src/textual/_border.py),
a 3×3 table (top/middle/bottom × left/centre/right):

| Type | Top row | Sides | Bottom row | Reads as |
|---|---|---|---|---|
| `round` / `solid` / `heavy` / `double` / `dashed` / `ascii` | `╭─╮` etc. | `│` | `╰─╯` | Outlined |
| `outer` | `▛▀▜` | `▌ ▐` | `▙▄▟` | half-block line outside the fill |
| `inner` | `▗▄▖` | `▐ ▌` | `▝▀▘` | half-block line inside, drawn on the *parent* bg |
| `thick` | `█▀█` | `█ █` | `█▄█` | solid frame |
| `block` | `▄▄▄` | `█ █` | `▀▀▀` | **Filled** — half rows are the edge (top/bottom on parent bg) |
| `tall` | `▊▔▎` | `▊ ▎` | `▊▁▎` | fill with eighth-block lips top/bottom, heavy left |
| `wide` / `tab` | `▁▁▁` | `▎ ▊` | `▔▔▔` | eighth-block lips outside the fill |
| `hkey` | `▔▔▔` | (none) | `▁▁▁` | horizontal rules only |
| `vkey` | `▏ ▕` | `▏ ▕` | `▏ ▕` | vertical rules only |
| `panel` | `▊█▎` | `▊ ▎` | `▊▁▎` | **Banded** — a full-block top row is the title band |
| `blank` / `hidden` / `none` | spaces | | | reserve the cell / no border |

The part that matters for tuilib is the parallel table `BORDER_LOCATIONS`:
for every one of those nine cells it records whether the glyph is painted on
the **widget's** background (0), the **parent's** (1), or a **reversed
cross-combination** (2: parent bg as fg with widget fg, reversed; 3: the
converse). `inner` and `block` put their top/bottom rows on the parent;
`tall`/`panel` use 2 on the left column and 1 on the right. Half- and
eighth-block edges only look right because of this. `BORDER_TITLE_FLIP`
marks `panel` (top) and `tab` (both) as needing the title drawn reversed —
on `panel` that is what turns the block row into a band carrying the title.

**Titles.** `border-title` on the top edge, `border-subtitle` on the bottom,
each with `-align` (`left` default, `center`, `right`), `-color`,
`-background`, `-style`
([docs/styles/border_title_align.md](https://github.com/Textualize/textual/blob/main/docs/styles/border_title_align.md)).
Each side takes its own type and colour (`border-left: wide $accent`).

**Related rules.** `outline` draws the same types *over* content instead of
reserving cells
([docs/styles/outline.md](https://github.com/Textualize/textual/blob/main/docs/styles/outline.md));
`keyline` draws dividing lines in a container's gutter that may cross and
join ([docs/styles/keyline.md](https://github.com/Textualize/textual/blob/main/docs/styles/keyline.md)).
`textual borders` previews every type.

**Design tokens.** [`design.py`](https://github.com/Textualize/textual/blob/main/src/textual/design.py)
derives the surface tones from a few seeds: `background` (dark default
`#121212`), `surface` (`#1e1e1e`), `panel` (= `surface` blended 10% toward
`primary`, plus a 4% `boost`), each with `-lighten-1..3`/`-darken-1..3`
shades. Focus tokens are derived too: `border` = `primary`,
`border-blurred` = `surface.darken(0.025)`, `surface-active` =
`surface.lighten(spread/2.5)`. When the background is an ANSI colour, `panel`,
`surface` and `boost` become `transparent` — filled surfaces collapse to
nothing on an ANSI theme, by design.

**Focus.** Per widget, in CSS: `Input`, `TextArea`, `Button` use
`border: tall $border-blurred` → `&:focus { border: tall $border }`
([`_input.py`](https://github.com/Textualize/textual/blob/main/src/textual/widgets/_input.py));
`DataTable` and `ListView` change **no border** — `&:focus { background-tint:
$foreground 5% }`.

**Defining a new one.** Not possible: the set is validated, and the glyph and
location tables are private module constants.

### Ratatui (Rust)

[`Block`](https://github.com/ratatui/ratatui/blob/main/ratatui-widgets/src/block.rs)
is the one surface primitive: `borders(Borders)` (bitflags per side),
`border_type(BorderType)`, `border_set(border::Set)` for arbitrary glyphs,
`border_style` / `style` (edge vs body), `padding(Padding)`, `title_top` /
`title_bottom` (many titles, each a `Line` with its own alignment),
`title_position`, `merge_borders(MergeStrategy)` to join adjacent blocks
(`Replace`/`Exact`/`Fuzzy`), and `shadow(Shadow)`. `inner(area)` subtracts
borders, padding, and a title row even when that side has no border.

`BorderType`
([`borders.rs`](https://github.com/ratatui/ratatui/blob/main/ratatui-widgets/src/borders.rs)):
`Plain Rounded Double Thick`, four dashed weights (`Light/Heavy` ×
`Double/Triple/Quadruple`), `QuadrantInside`, `QuadrantOutside`. Extra
`border::Set` constants
([`symbols/border.rs`](https://github.com/ratatui/ratatui/blob/main/ratatui-core/src/symbols/border.rs))
include `ONE_EIGHTH_WIDE` / `ONE_EIGHTH_TALL` (doc-commented "based on
McGugan box technique", i.e. Textual's `wide`/`tall`), `PROPORTIONAL_WIDE`
(half rows top/bottom, full-block sides — Textual `block`, so visually equal
edge thickness), `PROPORTIONAL_TALL`, `FULL`, `EMPTY` ("apply the border
style to a block with a title without drawing a border").

`Padding` is per side (`new(l, r, t, b)`, `horizontal`, `vertical`,
`uniform`, `symmetric`, and `proportional(n)` = n vertical, 2n horizontal, to
compensate for cells being ~2:1).

**Shadow** (v0.30.1, 2026-06-05, [CHANGELOG](https://github.com/ratatui/ratatui/blob/main/CHANGELOG.md),
[`shadow.rs`](https://github.com/ratatui/ratatui/blob/main/ratatui-widgets/src/block/shadow.rs)):
an offset area behind the block; presets `overlay()` (style only — dims
what's there), `block()`, `light_shade()`/`medium_shade()`/`dark_shade()`
(`░▒▓`), `symbol(s)`, and `custom(impl CellEffect)` that gets the shadow and
base rects and writes the buffer.

Ratatui has **no glyph-location concept**: a `Set` is just glyphs, coloured
by one `border_style`. Half-block sets look right only if the caller sets the
style's bg to the parent colour and fg to the fill — every edge glyph shares
that one pairing, so `QuadrantInside` vs `Outside` is the caller's colour
choice, not the set's. Focus is entirely the app's (swap `border_style`).

### Charm: lipgloss, Huh

**lipgloss** v1
([`borders.go`](https://github.com/charmbracelet/lipgloss/blob/v1.1.0/borders.go))
and v2.0.6 ship `Normal Rounded Block OuterHalfBlock InnerHalfBlock Thick
Double Hidden Markdown ASCII`; `Border{}` is an exported 13-field struct
(edges, corners, middles), so custom glyphs are trivial. Per side:
`BorderTop(bool)`…, `BorderForeground(c...)` / `BorderBackground(c...)`
taking 1–4 colours CSS-style; v2 adds a blended gradient along each side.
No title-in-border, no shadow, no notion of parent background — a
`BorderBackground` is one colour for the whole edge.

**Huh** ([`theme.go`, v0.6.0](https://github.com/charmbracelet/huh/blob/v0.6.0/theme.go)):
a `Theme` holds `Focused` and `Blurred` `FieldStyles`. Focus is a **left bar**:
`Focused.Base = PaddingLeft(1).BorderStyle(ThickBorder()).BorderLeft(true)`,
and `Blurred.Base` swaps in `HiddenBorder()` so the column is kept and nothing
shifts. `ThemeCharm` colours the bar `238`. Themes are Go functions
(`ThemeCharm`, `ThemeDracula`, …) that mutate a copy of `ThemeBase()`.

## Apps

| App | Surface | Focus signal | How a theme picks it |
|---|---|---|---|
| **btop** | Outlined boxes, title notched into the top edge with `┐title┌` (`Symbols::title_left/right`), `├┤` dividers ([btop_draw.cpp](https://github.com/aristocratos/btop/blob/main/src/btop_draw.cpp)) | none per box; selection is `selected_bg` | colours per box (`cpu_box`, `mem_box`, `net_box`, `proc_box`, `div_line`); shape is a config bool `rounded_corners`, ignored in TTY mode; `theme_background=false` for transparency ([btop_config.cpp](https://github.com/aristocratos/btop/blob/main/src/btop_config.cpp)) |
| **lazygit** | Outlined | `activeBorderColor` (default green bold) vs `inactiveBorderColor`; `searchingActiveBorderColor` | `gui.border: rounded \| single \| double \| hidden \| bold` ([Config.md](https://github.com/jesseduffield/lazygit/blob/master/docs/Config.md)) |
| **k9s** | Outlined (tview), title in top edge with counter/filter | `frame.border.focusColor` vs `fgColor` | skin YAML: colours only; `frame.title` has `fg/bg/highlight/counter/filterColor` ([README](https://github.com/derailed/k9s/blob/master/README.md)) |
| **Posting** (Textual) | Outlined panes, accent left bars on items | `border: round $accent 40%` → `:focus-within` `round $accent 100%`, title right-aligned; option lists gain `border-left: inner $accent` on focus ([posting.scss](https://github.com/darrenburns/posting/blob/main/src/posting/posting.scss)) | YAML themes feeding Textual tokens |
| **Harlequin** (Textual) | Outlined panes | `$border-color-nofocus: $panel` → `$border-color-focus: $primary` on `:focus`/`:focus-within`; status items `border-left: wide $primary/$error` ([app.tcss](https://github.com/tconbeer/harlequin/blob/main/src/harlequin/app.tcss), [global.tcss](https://github.com/tconbeer/harlequin/blob/main/src/harlequin/global.tcss)) | Pygments style → Textual tokens |
| **Zellij** | `pane_frame_style`: `titles` (**default since 0.45.0**: one title line above each pane, no box), `full`, `none`; `rounded_corners` ([options.md](https://github.com/zellij-org/zellij-org.github.io/blob/main/docs/src/options.md)) | theme components `frame_unselected` / `frame_selected` / `frame_highlight` (the last for non-normal modes) ([themes.md](https://github.com/zellij-org/zellij-org.github.io/blob/main/docs/src/themes.md)) | KDL theme by UI component |
| **Helix** | Splits divided by single lines (`ui.window`); popups/menus **Filled, borderless by default** (`ui.popup`, `ui.menu` bg) | `ui.statusline.inactive`; no pane border | `editor.popup-border = popup \| menu \| all \| none` (default `none`) ([editor.md](https://github.com/helix-editor/helix/blob/master/book/src/editor.md), [themes.md](https://github.com/helix-editor/helix/blob/master/book/src/themes.md)) |
| **superfile** | Outlined with configurable glyphs; borderless by setting `' '` | `*_border_active` colour per region (`file_panel`, `sidebar`, `footer`, `modal`); sidebar ships with `sidebar_border` = bg, i.e. invisible until active | 11 glyph keys `border_top…border_middle_right` in config.toml; `transparent_background`; `enable_file_preview_border` ([config.toml](https://github.com/yorukot/superfile/blob/main/src/superfile_config/config.toml), [catppuccin-mocha.toml](https://github.com/yorukot/superfile/blob/main/src/superfile_config/theme/catppuccin-mocha.toml)) |

Observations:

- **Outlined dominates** dense, many-pane tools (btop, lazygit, k9s,
  Harlequin, Posting). Fills appear where there are few surfaces or they
  float: Helix popups, Textual inputs/buttons.
- **Zellij's `titles` style is a fifth surface** tuilib's four don't name:
  just a title row, no edge, no fill — a "rule" surface.
- **"Invisible until focused"** (superfile sidebar, Huh blurred
  `HiddenBorder`) relies on reserving the edge cell and colouring it, so the
  layout never shifts on focus.

## Rendering pitfalls

**Block-glyph gaps.** Half- and eighth-blocks only tile if the terminal draws
them to the cell, not the font's glyph box. Kitty, WezTerm
(`custom_block_glyphs = true` by default, covering U+2500 box drawing and
U+2580 block elements — "present to work around a hinting issue in
freetype", [docs](https://github.com/wezterm/wezterm/blob/main/docs/config/lua/config/custom_block_glyphs.md)),
Alacritty (`font.builtin_box_drawing`, default true, U+2500–U+259F,
[alacritty(5)](https://github.com/alacritty/alacritty/blob/master/extra/man/alacritty.5.scd))
and Ghostty (exposes `adjust-box-thickness` for its own box drawing,
[Config.zig](https://github.com/ghostty-org/ghostty/blob/main/src/config/Config.zig))
synthesise them. macOS Terminal.app uses the font, and Textual's FAQ shows
misaligned blocks there, recommending Menlo, character spacing 1, **line
spacing 0.805** ([FAQ](https://github.com/Textualize/textual/blob/main/docs/FAQ.md)).
Any user line-height setting > 1 re-opens gaps between `▀`/`▄` rows. tmux's
own FAQ notes some fonts render even `─` dashed.
Implication: Outlined is the only surface safe everywhere; anything with
block edges needs a no-glyph fallback (plain fill + padding row).

**256-colour collapse.** Surface tones are small luminance steps; the xterm
256 grey ramp moves 10/255 per step and the colour cube 40. Nearest-match
(plain Euclidean, computed for this note): Textual's `#121212`→233,
`#1e1e1e`→234 (still distinct), but Nord `#2e3440` and a 6% lift `#343a46`
both → **237**, and every blue-grey loses its hue to neutral grey
(Catppuccin `#1e1e2e`→235). Filled/Banded/Elevated can therefore render as
flat in 256 mode. Terminal.app is 256-only per the Textual FAQ. btop exposes
`truecolor=false` to force 6×6×6 conversion and `lowcolor`/`force_tty` for 16.

**tmux.** tmux passes truecolor only when told the outer terminal has it:
`set -as terminal-features ",xterm*:RGB"` (≥3.2) or `terminal-overrides …:Tc`
([tmux FAQ](https://github.com/tmux/tmux/wiki/FAQ)). On the app side, termenv
(which lipgloss uses) returns TrueColor only for `COLORTERM=truecolor|24bit`
or a known `TERM` (`xterm-kitty`, `wezterm`, `xterm-ghostty`, …); under
`TERM=tmux-256color` without `COLORTERM` it returns **ANSI256**, and with
`TERM=screen*` it demands `TERM_PROGRAM=tmux`
([termenv_unix.go v0.16.0](https://github.com/muesli/termenv/blob/v0.16.0/termenv_unix.go)).
termenv also refuses OSC background queries under `screen`/`tmux`, so
`HasDarkBackground` (and any light/dark-derived surface tone) falls back to
its default.

**Transparency.** A filled surface paints a background; terminal
transparency needs default-bg cells. Textual says outright translucency "is
unlikely to work" and its ANSI mode makes `surface`/`panel` transparent;
btop and superfile expose a "don't paint the background" switch. A Filled
surface on a transparent palette needs to degrade to Outlined.

## For tuilib

### Candidate surfaces beyond Outlined / Filled / Banded / Elevated

1. **Ruled** (Zellij `titles`, Textual `hkey`): a title row or top rule, no
   sides, no fill. The cheapest frame that still names the pane; good for
   dense dashboards and safe in every terminal.
2. **Accented** (Huh focus, Posting/Harlequin `border-left: wide`, Textual
   `vkey`/`tall` left column): unfilled body, a one-cell left bar that is
   hidden when blurred and coloured when focused. Zero shift on focus.
3. **Lipped** (Textual `tall`/`wide`, Ratatui `ONE_EIGHTH_*`): a fill whose
   top/bottom edges are eighth-block lips — thinner than Filled's half rows,
   same cell cost. The most fragile glyph-wise.
4. **Bare** (Textual `blank`/`hidden`, Ratatui `EMPTY`, lazygit `hidden`):
   reserves the edge cells, draws nothing — the fallback that keeps geometry
   identical when a surface can't render (256 collapse, transparency).
   Arguably the degrade target rather than a user choice.

Elevated maps directly onto Ratatui's `Shadow` (offset + style + optional
symbol or custom effect); `overlay()` — dim what's underneath — is worth
copying for modals.

### What a custom-surface interface must express

Drawn from what the frameworks needed and where the weaker ones fall short:

- **Inset per side, independently** (0 or 1+ cells each; Ratatui
  `Borders` + `Padding`, Textual per-side `border-*`). Banded is top-only;
  Accented is left-only; Elevated adds outside cells right and bottom.
- **Cells outside the rect** (shadow offset), declared so layout and
  hit-testing (rule 28) know the drawn extent differs from the content rect.
- **Per-glyph background location**: on the surface fill, on the screen
  background behind it, or reversed — Textual's `BORDER_LOCATIONS`. Without
  this, half/eighth-block edges are unexpressible (lipgloss and Ratatui push
  it onto the caller).
- **Title placement**: which edge (top/bottom/band row), alignment, whether
  it sits *in* the edge line (btop notch, k9s, Textual) or on a row of its
  own (Banded, Zellij `titles`), and whether it draws reversed
  (`BORDER_TITLE_FLIP`). tuilib also needs the existing right/bottom slots
  (loading suffix, counters, error messages).
- **Focus as a state, not a colour**: the surface decides what changes —
  edge colour, fill tint, band colour, accent bar visibility — from
  {blurred, focused, highlight/mode}. Zellij's third state
  (`frame_highlight`) is a useful precedent for "focused while capturing
  keys". Focus must never change insets.
- **Tones derived from the palette**, not hand-picked per palette (Textual
  derives `surface`/`panel`/`border-blurred` from seeds), plus the screen
  background the surface is raised against.
- **A degrade path** chosen by capability: colour profile (truecolor /
  256 / 16), transparency, and a "no block glyphs" switch — so a surface can
  say "in 256 I become Outlined/Bare" instead of rendering flat.
- **Inner joins** (pinned header rule, filter rule; Ratatui
  `MergeStrategy`): the `├─┤` row a filterable component draws under its
  filter must match the surface — a fill-only surface needs a tone change or
  half-block row instead of a line.
