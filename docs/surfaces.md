# Surfaces — spec

Status: **built.** Every decision here was made on the map
[Map: surfaces — card-style theming](https://github.com/jsdrews/tuilib/issues/111),
and each section links the ticket that holds its reasoning. The vocabulary is in
`CONTEXT.md` § Surfaces.

## The model

> A **surface** is how a component's edge and body are drawn. It is data that the
> pane interprets, written in colour **roles**. Any palette can be drawn with any
> surface.

The user picks two things independently: a **palette** (colours) and a
**surface** (shape). The shell combines them, with the glyphs, into the
**Theme** that components are built from.

Five surfaces ship:

| Surface | Look | Needs |
|---|---|---|
| `Outlined` | a line border, no fill. Today's look, **byte-identical** | nothing |
| `Filled` | a solid card whose half-block rows (`▄`/`▀`) are its own edge | 256 colours, block glyphs |
| `Banded` | a card under a header band carrying the title; focus colours the band | 256 colours |
| `Elevated` | a card with a half-block drop shadow (`▌`/`▀`) right and below; accent stripe on focus | 256 colours, block glyphs |
| `Accented` | unfilled, with a `▌` bar down the left edge, accent-coloured when focused | block glyphs (else `┃`) |

Ruled, Lipped (eighth-block lips) and Bare were considered and don't ship; they
are what custom surfaces look like
([Which built-in surfaces ship](https://github.com/jsdrews/tuilib/issues/119)).

## Build order

Each step ships on its own and leaves the library working.

1. **`ansi.Fill` and the selection fix.** Today's selection holes are fixed under
   Outlined before any surface exists. See [Selection](#selection).
2. **Outlined goldens.** Capture every launcher example's rendered frame from
   today's output. From here on, Outlined must match them byte for byte.
   See [Enforcement](#enforcement).
3. **`pkg/surface` and `pkg/palette`** as leaf packages, with derivation,
   quantisation and resolution unit-tested with no UI.
4. **The pane** takes a resolved surface: edges, text rows, dividers, the fill
   pass, inset-aware hit-testing. The ~12 hard-coded `width-2`/`Y+1` sites move
   to `ContentRect`. The pane draws the header divider, so the 8 copies of
   `glyphs.Rule` go.
5. **`layout.ZStack`** composites by painted cells.
6. **The app shell:** two axes, the body padding, the screen pass, bars on the
   surface's role, fallback logging, config keys.
7. **Migration** of the library's examples and the six downstream modules,
   with a "Migrating to surfaces" section in this doc's successor guide.
8. **The surface demo**: `examples/shell/surfaces` (was `shell/chrome`), a
   live surface picker that defines two custom surfaces, Ruled and Lipped,
   as struct literals offered beside the built-ins.

## Packages

Imports point one way: `pane → surface`, `palette → surface`, and
`theme → palette, surface, pane, …`. `surface` imports nothing from tuilib, and `palette` only `surface`.

- **`pkg/surface`** (leaf): `Surface`, `Role`, the five built-ins, `Derive`,
  `Resolve`, and the resolved type. All colour maths lives here.
- **`pkg/palette`** (leaf, imports `surface`): `Palette`, the 19 named palettes
  (`palette.Dark()`, `palette.FromBase16`, `palette.Terminal()`, `palette.All()`),
  `ByName`. Picking the starting palette and surface from env and config lives
  in the app shell, which already reads `pkg/config`, so `palette` depends on
  nothing but `surface`.
- **`pkg/theme`** keeps `Theme` = palette + surface + glyphs, and the
  `th.Component()` builders, which need all three. It is not renamed.

([Where the surface choice lives](https://github.com/jsdrews/tuilib/issues/116),
[The surface seam](https://github.com/jsdrews/tuilib/issues/117))

## A surface is data

A `Surface` is a struct literal, the way `lipgloss.Border` is. There is no
`Draw` interface, so a custom surface can't get the geometry wrong. It declares:

- **Name**, for config matching and pickers.
- **Edges per side:** glyphs, the cells each side takes, and where each glyph's
  background comes from: the **card** (the `Fill` role), or the **parent**
  (whatever contains the surface; the `Parent` role). Textual's "reversed"
  placement needs no third value: a half-block lip is a glyph in `Fill` on
  `Parent`, which is what makes half-block edges expressible.
- **Text rows:** every surface has a top and a bottom text row, so all five
  slots (title, top-right, bottom-left, bottom-middle, bottom-right) exist on
  every surface. The surface decides how each row looks: a line with text
  inlaid, a band, or a card-tone row inside a lip or shadow. Elevated's bottom
  edge is two rows: footer, then shadow. Slot brackets are text-row data.
- **The divider** under a pinned header: glyph, role, the role while the header
  has input, and which background it sits on.
- **Focus effects** for two states, focused and blurred: edge role, fill role,
  band role, whether the accent bar shows. **Edge space never differs between
  states**, so Accented reserves its bar column when blurred.
- **Selection treatment** (see [Selection](#selection)).
- **Bar role:** which role the breadcrumb and statusbar sit on.
- **Overlay form:** another surface value for modals (see [Overlays](#overlays)).
- **Field form:** another surface value for single-line components — input,
  filter, toggle — which screens place in three-row slots, so it takes at
  most one cell of edge per side. Nil derives it: a lined surface is its own
  field, and anything else draws an outlined field (Material's outlined text
  field). Theme builders hand `Input()`, `Filter()` and `Toggle()` this form.
- **Needs and fallback:** minimum colour depth, whether it uses block glyphs, and
  the `Fallback` surface (see [Degraded terminals](#degraded-terminals)).

Colours appear only as roles, never as literals.

### Geometry

- **A surface draws only inside its rect.** The rect `layout` hands out is where
  the surface draws, and content is the rect minus the edges. Elevated's shadow
  is edge, drawn on the parent.
- **Margins:** a filled surface's edge includes one cell on its right and bottom,
  drawn with no background, so whatever contains it paints it: an outer pane's
  fill pass, or at the top level the shell's screen pass. Adjacent cards end up
  exactly one cell apart. Elevated's shadow *is* its margin. Filled keeps a
  right margin only: its lips are its vertical margin, since the lower half of
  one card's `▀` and the upper half of the next card's `▄` leave exactly one
  row of screen between them. Outlined has none.
- **The shell pads the body** one column on the left when the current surface
  has a right margin (`Surface.Margined`), so the screen's left edge matches
  the gaps between cards; and one row under the breadcrumb when the surface's
  top edge starts with a solid row (`Surface.TopGap`: Banded, Elevated,
  Accented), which would otherwise sit jammed against it. A top edge that is
  already a lip or a line (Filled, Outlined) brings its own gap. `layout`
  knows nothing about any of this.
- **Hit-testing:** `pane.RowAt`, scrollbar hits and slot clicks derive from the
  surface's per-side edge space. A click on a margin or shadow cell is in the
  rect but outside the card, so the component declines it (rule 28).

([The surface seam](https://github.com/jsdrews/tuilib/issues/117),
[Gutters between surfaces](https://github.com/jsdrews/tuilib/issues/118))

## Colour roles

A palette gains two **seeds**: `Background` (the palette's canonical background:
base16 `base00`, Catppuccin `base`, `nord0`) and `Foreground`. Built-in palettes
set them from upstream. Unset, they fall back to `BarBG` and `BarFG`.

Every role is **derived** from the palette, and any role can be **overridden**
per palette with `Palette.Surface map[surface.Role]lipgloss.TerminalColor`, where
a missing key means derive.

"Lift" mixes toward `Foreground` in OKLab. On light palettes, cards lift toward
white.

| Role | Used for | Derived (dark) | Derived (light) |
|---|---|---|---|
| `Screen` | behind and between surfaces | `Background` | `Background` |
| `Card` | surface fill | lift 8% | toward white 70% |
| `CardRaised` | focused fill | lift 13% | toward white 90% |
| `Band` | Banded's header, Filled's divider step | lift 18% | darken `Card` 6% |
| `Overlay` | overlay-form fill | lift 16% | white |
| `Shadow` | Elevated's shadow | darken `Screen` 60% | darken `Screen` 22% |
| `Selection` | focused selected row | `Card` mixed 45% toward `EdgeActive` | same |
| `SelectionDim` | blurred selected row | lift `Card` 12% | same |
| `Bar` | bars under Outlined | `BarBG` | same |
| `Text` / `TextMuted` | body text on filled surfaces | `Foreground` / `Muted` | same |
| `Edge` / `EdgeActive` | lines, the active band | `BorderInactive` / `BorderActive` | same |
| `Divider` / `Accent` | | `Subtle` / `Accent` | same |

The percentages are starting points to tune, not spec.

- **Contrast floor:** a derived tone too close in lightness to the role beneath
  it is pushed further: `Card` on `Screen`, `CardRaised` on `Card`, `Band` on
  `Card`, `Selection` on `Card`.
- **Text:** filled surfaces paint body text in `Text`, and the fill pass
  re-opens the foreground as well as the background. Outlined keeps the
  terminal's own colours.
- **Pipeline:** `surface.Derive(seeds, overrides, profile)` returns the full
  role-to-colour map; `surface.Resolve(s, colours)` returns a resolved surface
  with concrete colours. Theme builders do both. The pane only ever sees
  resolved surfaces.
- Expected built-in override: Dark's `Selection` (its `EdgeActive` is ANSI
  bright blue, so the mix comes out saturated).

([Surface colour tokens](https://github.com/jsdrews/tuilib/issues/115),
[Bars on filled surfaces](https://github.com/jsdrews/tuilib/issues/123))

## The fill pass

The pane fills once, in `pane.View`, over its own rendered lines:

- open the surface background (and `Text` foreground) at the start of each line;
- re-open it after every SGR that clears it (`0`, empty, `49`, and the foreground
  equivalents), found with a real parameter walk that skips the operands of
  `38`/`48`/`58` (a string replace corrupts `38;2;R;0;B`);
- pad every line to the pane width.

It is `ansi.Fill(s, open)`, the same primitive the row renderers use. It is a
no-op for Outlined. Nesting composes: an inner pane's background wins inside its
rect. The audit measured 0 unpainted cells across 32 renders at about 100 µs per
120×36 frame, before a no-escape fast path.

Bars, layout gaps and margins are outside any pane. The shell paints them with a
whole-frame pass in `Screen`.

([Audit where components rely on the terminal background](https://github.com/jsdrews/tuilib/issues/114);
findings in `research/background-fill-audit.md`)

## Selection

- **`ansi.Fill` replaces `tree.renderPreserving`.** All six row renderers use it
  with the selection background and pad the row to the inner width: inspector,
  eventlog, logview, textview, table, list. This is step 1 of the build and
  fixes today's holes under Outlined.
- **The surface declares the treatment.** Outlined declares today's exactly: the
  list's bold `Accent` foreground with no background, a `Subtle` background in
  the others, and no difference on blur. Filled, Banded, Elevated and Accented
  declare a `Selection` background with `Text` foreground in every component,
  and `SelectionDim` when the pane is blurred. Each row component carries the
  blurred style as its own option (`SelectedBlurredStyle` on list, table and
  eventlog; `CurrentLineBlurredStyle` on tree, inspector, logview and
  textview), set by the theme and used only when it sets a background, and
  redraws its rows on a focus change when it has one.

([Selection on filled surfaces](https://github.com/jsdrews/tuilib/issues/124))

## Overlays

- **Each surface carries an overlay form**, `Surface.Overlay`. Unset, it is the
  same shape with its fill role switched from `Card` to `Overlay`. Outlined sets
  Thick lines explicitly, so today's modals are byte-identical. It replaces
  `Theme.BorderShapeOverlay`. Confirm, alert, the action menu and the help
  overlay are built with it.
- **`layout.ZStack` composites by painted cells:** a cell that isn't a space, or
  that carries a background, covers the base. On each row it splices from the
  first painted cell to the last. Padding from `Center`/`lipgloss.Place` and
  margin cells stay transparent.
- **No scrim:** the base isn't dimmed (out of scope, revisit only if modals don't
  stand out).

([Overlays on filled surfaces](https://github.com/jsdrews/tuilib/issues/121))

## Bars

The surface names the role the breadcrumb and statusbar sit on. Outlined uses
`Bar` (byte-identical). Filled, Banded, Elevated and Accented use `Screen`, so
the cards float in one field. Bar text keeps `BarFG`, falling back to `Text`
when it isn't legible there (`surface.Legible`: an OKLab lightness distance of
at least 0.25, well above the tone floor). Outlined keeps the palette's own
pair untouched. Info and Error keep their own bands. The `? help` affordance
and the inline hints take the bar's role as their background.

([Bars on filled surfaces](https://github.com/jsdrews/tuilib/issues/123))

## Choosing a surface

- **`app.Options.Palettes`** (was `Themes`), cycled by `ThemeKey`.
- **`app.Options.Surfaces`**, default `[surface.Outlined]`, cycled by
  **`SurfaceKey`**, unset by default (opt-in, like `ThemeKey`).
- **Config:** `config.yaml` gains `palette:` (with `theme:` read as an alias),
  `surface:` (matched by name from what the app offers, ignored when it offers
  one) and `block_glyphs:`.
- **Env:** `PaletteEnvVar` (was `ThemeEnvVar`) and `SurfaceEnvVar`.
- **Live preview:** `app.SetPalette(name)` (was `SetTheme`).
- **Screens need nothing new.** A swap is a new `Theme` through the existing
  `SetTheme` path, and rule 4's `State`/`Restore` covers it.
- **Chrome moves:** `BorderShapeActive`/`Inactive` become Outlined's edge glyphs
  (`surface.OutlinedWith(lipgloss.RoundedBorder())` for a variant).
  `SlotBrackets` become `Surface.Brackets`. `Glyphs` stay on `Theme`, except
  `Glyphs.Rule`, which is `Surface.Divider.Glyph` now.

### Per-component overrides

```go
opts := t.Logview()
opts.Surface = t.Resolve(surface.Outlined)   // this pane stays Outlined
```

`Options.Surface` holds only a **resolved** surface, so skipping `Theme.Resolve`
is a compile error. The override survives a surface cycle because `SetTheme`
re-applies it. A pinned surface brings its own margins or none.

([Where the surface choice lives](https://github.com/jsdrews/tuilib/issues/116),
[Per-component surface overrides](https://github.com/jsdrews/tuilib/issues/125))

## Degraded terminals

- **Quantise, then enforce the floor.** `Derive` rounds each role to the
  detected colour profile, then re-checks the contrast-floor pairs and pushes any
  that collapsed to the next distinct step. 256 colours, and so default tmux,
  keep filled surfaces. Palette overrides are rounded but never pushed.
- **Fallback chain.** Resolution follows `Fallback` until the terminal meets the
  surface's needs:
  - Filled → Banded
  - Elevated → Banded
  - Banded → Outlined, below 256 colours
  - Accented → Accented with a `┃` bar, without block glyphs
  - Outlined ends every chain
- **Block glyphs:** `block_glyphs:` in config. Unset, it is off for known
  font-drawn terminals (`TERM_PROGRAM=Apple_Terminal` first) and on elsewhere.
- **ANSI-index inputs:** every derivation that mixes in a non-seed colour has a
  seeds-only recipe, used when an input has no RGB (`Selection` becomes "lift
  `Card` 25%"). ANSI colours used directly pass through, so `Terminal()` works.
- **Visibility:** when a chosen surface resolves to its fallback, the shell writes
  one line to the output console ("surface elevated → banded (block glyphs
  off)"). `SurfaceKey` skips entries that resolve to a surface already offered.

([Surface fallback on degraded terminals](https://github.com/jsdrews/tuilib/issues/120))

## Enforcement

The check works from geometry, never heuristics. Four assertions:

1. Every cell inside the card (rect minus margins) carries a background.
2. Margin cells carry none.
3. The selected row has one background across the full inner width.
4. Every cell of the frame carries a background, after any `ZStack` composite.

The classifier is an SGR state machine on `x/ansi.DecodeSequence` in
**`internal/cells`**, shared with `ansi.Fill`, so the check and the fill agree on
what clears a background. It runs over three layers:

1. **Components** in `internal/componenttest`: every pane-owning component × the
   5 surfaces × focused/blurred. States: empty, loading, filter focused, a
   selected row holding lipgloss, `CellColor` and hyperlink cells, a search
   highlight, row activity. Plus the four overlays through `ZStack`, and pinned
   surfaces against their own surface.
2. **Examples:** every launcher example through the full app shell, per surface.
3. **Outlined goldens**, captured before the work starts (build step 2).

Downstream apps get the same assertions as a `pkg/screentest` rule,
`SurfacesFill`, once `pkg/screentest` exists.

([Enforcing the background-fill invariant](https://github.com/jsdrews/tuilib/issues/122))

## Migration

This is a clean break in one minor version. There are no Go shims. The one
thing kept is the config key: users' config files are outside any app's
control, so `theme:` is still read when `palette:` is unset.

| Before | After |
|---|---|
| `theme.Dark()` (and every named palette), as a `Theme` | `theme.New(palette.Dark())`; as a palette, `palette.Dark()` |
| `theme.All()` | `palette.All()` |
| `theme.ByName(themes, name)` | `palette.ByName(palettes, name)` |
| `theme.Resolve(themes, envVar)` | gone: `app.New` picks the start from `PaletteEnvVar` and the config file |
| `theme.FromBase16`, `theme.Base16` | `palette.FromBase16`, `palette.Base16` |
| `theme.Terminal()` | `palette.Terminal()` |
| a `Theme` literal | a `palette.Palette` literal; `Theme` embeds it, so `t.Accent` still reads |
| `app.Options.Themes: []theme.Theme{…}` | `app.Options.Palettes: []palette.Palette{…}` |
| `app.Options.ThemeEnvVar` | `app.Options.PaletteEnvVar` |
| `app.SetTheme(name)`, `app.SetThemeMsg` | `app.SetPalette(name)`, `app.SetPaletteMsg` |
| `Theme.BorderShapeActive/Inactive` | a surface: `t.Surface = t.Resolve(surface.OutlinedWith(shape))` |
| `Theme.BorderShapeOverlay` | the surface's `Overlay` form: `s.Overlay = &overlay` |
| `Theme.SlotBrackets` | `Surface.Brackets` (`pane.SlotBracketStyle` is now `surface.Brackets`) |
| `Glyphs.Rule` | `Surface.Divider.Glyph` |
| `config.yaml` `theme:` | `palette:` (`theme:` still read) |

`Options.Surfaces` defaults to `[surface.Outlined]`, so an app that migrates
these calls and nothing else draws exactly what it drew before; the Outlined
goldens hold that.

The local modules that use tuilib — tui-builder (v0.35.0), tuiquery (v0.21.0)
and tuiquery_old — pin released versions, so they migrate when a release
carries this change. tuilib-skills and tuilib-detailed-progress are other
checkouts of tuilib itself, not consumers.

## Out of scope

- **A scrim** under modals.
- **Ruled, Lipped and Bare** as built-ins; they're custom surfaces.
