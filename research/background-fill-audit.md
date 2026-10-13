# Background-fill audit: where tuilib relies on the terminal background

Ticket: #114 (map #111, "Surfaces"). Vocabulary: `CONTEXT.md` § Surfaces.

**Question.** A Filled, Banded or Elevated surface needs every cell a component draws to carry a background. Where does today's output fall back to the terminal default instead, how big is each fix, and where should the fill happen?

**Answer.** Almost everywhere, and for one reason. Nothing in the library sets a background except bars, selection and current-line highlights. Wrapping a component's output in a background from outside doesn't work, because the first `\x1b[0m` (the first border glyph) cancels it. The fill belongs in **one place: the pane**. The pane already draws the frame each surface replaces, and it sees every pane-owning component's final lines. It should open the surface background at the start of each line and **re-open it after every SGR that clears the background**. Three smaller pieces go with that:

1. **ZStack** opacity has to change. This one is structural but contained to one file.
2. **Screen-background fill** at the app root. This is the only place a whole-frame post-process is correct.
3. **A shared "re-open after reset" helper** that row highlights also use. This fixes the existing rule-19 holes in inspector, eventlog, logview, textview, list and table.

Making components fill themselves would mean changing about 40 sites. It still couldn't fix user-supplied strings or bubbles' textinput, which emit their own resets and unstyled padding.

---

## Method

Sources: this repo at `7603169` (working tree), lipgloss v1.1.0, bubbles v1.0.0 and x/ansi v0.11.6 from the module cache.

I wrote a throwaway harness in the session scratchpad. It forces `lipgloss.SetColorProfile(termenv.TrueColor)` and renders:

- 20 example screens at 120×36 through `Layout().Render`;
- the full `app.New` shell, with the help overlay both closed and open;
- targeted states: a table selected row holding `CellColor`, lipgloss and hyperlink cells; a focused empty filter and input (placeholder plus cursor); a toggle; a confirm modal inside `ZStack`/`Center`; an alert; logview and textview with a search highlight; a loading pane.

For each render, an SGR state machine (built on `x/ansi.DecodeSequence`) classifies every cell as one of: background set; foreground styled with no background; plain (no SGR active); or reverse video. It then runs three simulations:

- **Outer wrap.** Open a sentinel background once at the start of each line.
- **Post-process.** Re-open the sentinel after every SGR that clears the background.
- **Filled overlay.** A filled overlay composited through `layout.ZStack`.

### Results

| Simulation | Result |
|---|---|
| Outer wrap, every render | 0 rows keep the background past the first cell (1 surf cell per row: the left border glyph's `Render` ends with `\x1b[0m`). In examples, 4046–4284 of 4320 cells lose it; the rest are bars and highlights that already set a background. |
| Post-process (re-open after a clearing SGR), every render | **0 cells lost**, 0 unemitted cells, in all 32 renders. |
| Post-process cost | ≈100 µs and 58 KB per full 120×36 frame with the help overlay open (M1 Pro, naive, no fast path). |
| Bars (breadcrumb, statusbar, tab strip, help footer) | Already 100% background (`BarBG`) in every render. |

**Hazard found while building the simulation.** My first post-processor treated any `0` parameter as a reset. That broke the table's selected row: a truecolor `38;2;R;0;B` read as a reset, and the injected background overrode the selection. A real implementation has to walk SGR parameters, skipping the operands of `38`/`48`/`58`. A string replace on `\x1b[0m` is not enough. `tree.renderPreserving` (`pkg/tree/tree.go:1356`) matches only the exact `\x1b[0m` and `\x1b[m` forms. That is safe for lipgloss output, but it would miss `\x1b[49m` and compound resets.

---

## How the libraries behave (verified)

- **lipgloss `Style.Render` applies its SGR once per line, around the whole line** (`lipgloss@v1.1.0/style.go`, the per-line `te.Styled(l[i])` loop). An inner `\x1b[0m` therefore ends the outer background until the end of the line. Harness: `bg.Render(fg.Render("red") + " after")` puts a background on `red` and on the width padding, but not on ` after`.
- **lipgloss padding (`Width`/`Height`/alignment) uses the whitespace style**, which carries the style's background (`colorWhitespace` defaults to true). Padding a styled block fills correctly even when its content has resets.
- **`lipgloss.JoinHorizontal`/`JoinVertical` pad with plain spaces.** In the harness, the short line in a join had no background on its pad cell.
- **`lipgloss.Place` pads with plain spaces** unless you pass `WithWhitespaceBackground`, which fixed the gaps in the harness.
- **`x/ansi.Cut` and `Truncate` preserve every escape from the part they skip.** `Cut(bgRow, 12, 20)` came back as `\x1b[38;2;…;48;2;…m yyyyyyyy \x1b[0m`. Horizontal scroll (`pane.go:707`) and table cell truncation (`table.go:2431`) are therefore background-safe. **Truncation is not a problem.**
- **`ansi.CellColor` / `CellColorRGB` close with `\x1b[39m`**, which resets the foreground only (`pkg/ansi/ansi.go:32-36,103`), so they are background-safe. `output/render.go` `fg()` does the same. `ansi.Hyperlink` emits no SGR and is safe.
- **bubbles textinput.** The cursor is `Style.Reverse(true).Render` (`bubbles@v1.0.0/cursor/cursor.go:220`). Text and padding go through `TextStyle` (`textinput.go:658,696`). **The placeholder's trailing pad is a bare `strings.Repeat(" ", availWidth)`** (`textinput.go:725`). That is library code no style option reaches.
- **bubbles viewport** pads to its width and height through `lipgloss.NewStyle().Width().Height()` with no background (`viewport.go:518-523`). Every blank row and every short line in a pane comes out plain.

---

## Findings

Fix sizes:

- **style**: add a background to a style that already exists.
- **local**: a few lines in one function.
- **structural**: changes how the thing composes.

The last column says whether the [pane-level fill](#recommendation) covers the site automatically.

### Pane, the base of 10+ components (`pkg/pane`)

| Site | What breaks | Fix | Covered by pane fill |
|---|---|---|---|
| `border.go:55` `Borderize` style is `Foreground` only. Corners, edges and slot brackets (`:75-77,89-94`) each end in `\x1b[0m`. | Every border glyph has no background and resets the line. Harness: the left border is the only "surf" cell in the outer-wrap run. | style (`Background`/`BorderBackground`). Moot for Filled/Banded/Elevated, which replace the frame. | yes |
| `border.go:102-105` side borders via `lipgloss.Border(...).Render(content)` with `BorderForeground` only | The `│` cells have no background. | style (`BorderBackground`) | yes |
| `border.go:93` `Inline(true).MaxWidth(width)` | Truncates only, never pads. Fine. | none | n/a |
| `pane.go:826-831` `pad()` spaces around slot text; slot text "passes through verbatim" (`border.go:43`) | Title, counter and percent slots are plain cells. | local | yes |
| `scrollbar.go:31` blank scrollbar column (`" \n"`); `:37-39` track and thumb glyphs, unstyled | The scrollbar column is plain on every row (`.` in every harness render). | local (style the glyphs) | yes |
| `scrollbar.go:59,64` horizontal scrollbar, unstyled; `pane.go:317` extra `" "` under the vertical bar | Plain row. | local | yes |
| `pane.go:313` `JoinHorizontal(viewport, bar)`; `:317,338` `JoinVertical` | Plain join padding. | structural-ish (would need its own padding) | yes |
| bubbles `viewport.View` width and height padding (above) | All blank rows and the tail of every short line are plain, the largest share of lost cells. | style (`viewport.Style.Background`) handles padding only | yes |
| `pane.go:305` loading `lipgloss.Place` without a whitespace background; `:357-362` spinner `Style.Render` | The spinner area is plain; the spinner resets. | style (`WithWhitespaceBackground`, spinner background) | yes |
| `pane.go:338` header (the filter row and rule) joined above the body | Inherits whatever the component emits (below). | n/a | yes |

### Filter rule and inline filter (list, table, tree, inspector, logview, textview, eventlog, help overlay)

| Site | What breaks | Fix | Covered |
|---|---|---|---|
| `rule.Render(strings.Repeat(glyph, inner))` at `list.go:872`, `table.go:1019`, `logview.go:531`, `textview.go:355`, `inspector.go:477`, `eventlog.go:1050`, `help/overlay.go:522` | Foreground-only rule ending in a reset. The rule is one cell short of the scrollbar column, which stays plain (harness row 3 of every filterable pane). | style plus local | yes |
| `filter.go:233` `InlineView()` → textinput `View` | The prompt, text, cursor (reverse) and placeholder each reset. **The placeholder's trailing pad is bare spaces inside bubbles (`textinput.go:725`).** | style for the prompt, text, placeholder and cursor; **the trailing pad can't be fixed in a style** | yes |
| `filter.go:124`, `input.go:187` standalone filter and input: textinput in its own pane | Same as above. | same | yes |

### Row renderers

Unselected rows are almost all plain text plus nested `Render`s, so each nested style resets. Selected rows show where a background exists today and where it is already broken (the rule-19 class).

| Component | Site | What breaks | Fix | Covered |
|---|---|---|---|---|
| list | `list.go:414` `selectedStyle.Render(prefix+item)`, one run; `:416` mark: plain `" "` + `markStyle.Render` + plain item; `:418` plain row | Unselected rows are plain. The selected row isn't padded to the inner width. An item carrying its own lipgloss colour ends the selection background mid-row. | style plus local; nested resets need re-open | surface yes, selection no |
| list/tree activity | `activity/activity.go:552` plain `Repeat(" ")` gap before the badge; `:508` spinner `View()` resets | The gap is plain, and outside the selection run on a selected row. | local | surface yes |
| table | `table.go:2223-2224` header `headerStyle.Render(gutterPad+row)`; `:2257,2262` one `Render` per row (good shape); `:2259` mark glyph in its own style, then the row | Rows end at the sum of column widths (`formatCell`, `:2431-2442`), and past that is viewport padding (plain). **A lipgloss-rendered cell ends the selected row's background** (harness: `BBBB…` stops after the `Lip` column). Rule 19 tells users to avoid this; nothing enforces it. Vertical separators use `CellColor` (safe). | style; nested resets need re-open | surface yes, selection no |
| table | `table.go:2460` `buildRule` | Styled glyph run; stops one column short of the scrollbar. | local | yes |
| tree | `tree.go:1306-1309` unselected rows: plain indent and glyph plus `markStyle.Render`; `:1313-1323` current row via `renderPreserving` and padded to the inner width | **Already the right shape for selection**: re-open after a reset, pad to width. Unselected rows are plain. | style | yes |
| inspector | `inspector.go:1055-1057` `labelStyle.Render(label)` + plain `": "` + `valueStyle.Render(value)`; `:1025` `currentLineStyle.Render(padToWidth(line))` over those | **Broken today**: the current-line background ends at the first reset (after the label). Harness: `fBBBBBBBBBBBB..ff…`. `:1121` search-highlight `Render` resets too. | local (`renderPreserving`) | surface yes, selection no |
| logview / textview | `logview.go:845-867` / `textview.go:640-663` current line rendered per segment with `base.Render`, padded with `base.Render` (good shape) | An ANSI-coloured log line inside a segment (`base.Render(line[cursor:…])`) ends the current-line background mid-segment. Unselected lines are plain plus `matchStyle.Render` resets (`:881`, `:677`). Gutter (`logview.go:835`) is its own run. | local; nested needs re-open | surface yes, selection no |
| eventlog | `eventlog.go:1302` `style.Render(m.highlight(l))`; `:1371` match `Render` inside it; `:1350-1351` gutter pad inside `style.Render` | The selected row's background ends at the first search match (same class as the inspector). Rows aren't padded to width. | local | surface yes, selection no |
| action menu | `menu.go:490` `plain()` segments (used at `:517,527,532`); blank columns deliberately unstyled (`styled`, `:496`); `:446,449` indent and empty rows padding into the outer bounds | Inside the box: plain gaps. Outside: intentionally transparent for ZStack. | local inside; **must stay unpainted outside** | inside yes |
| help overlay | `overlay.go:622-641` plain indent and gap, `Render` per key/desc, `""` blank lines between sections; `:410` `Place` into the outer rect | Same as the menu. | local inside; outside must stay transparent | inside yes |
| toggle / confirm | `toggle.go:278-283` and `confirm.go:271-276` buttons joined with a plain `"  "`; `confirm.go:280` `messageStyle.Render` | Plain gap; resets. | style plus local | yes |
| alert | `alert.go:415,418,442` button and message `Render`; `:273` `Place` into the outer bounds | As above. The outside should stay transparent. | style | inside yes |
| form | `form.go:241,249,265` separators are empty lines (zero width); `:274-276` submit button, unpadded | Rows between fields are unemitted. The form has no pane (rule 9), so these cells belong to whatever contains the form. | n/a: screen background or the parent pane | via the parent pane, or the root fill |
| output console | `output/render.go:28-45` `fg()` foreground-only (safe); `:172` plain pad | Plain pad. | n/a | yes (it's a logview) |
| metrics | `CellColor` throughout | Safe. | none | n/a |

### Bars

These already fill completely. Whether they sit on `BarBG` or the screen background is a separate design question.

| Site | Status |
|---|---|
| breadcrumb `breadcrumb.go:129,141` `barStyle.Width(w).Render(...)` over styled crumbs | 100% background in the harness. Crumb styles set `BarBG`, and the width padding takes the bar style. |
| statusbar `statusbar.go:121-157` every slot `Background(barBG)`; the blank slot is explicit (`:155`) | 100%. |
| tab strip `tab.go:197-200,359` every style `.Background(bg)` | 100%. |
| help footer `help.go:204` `fillToWidth` with `descStyle` (`BarBG`, `theme.go:379-380`); `:215` `padRight` is plain but inside a styled cell | 100%. |

### Layout and compositing (`pkg/layout`)

| Site | What breaks | Fix |
|---|---|---|
| `layout.go:116,132` `JoinVertical`/`JoinHorizontal` | Plain padding when a child renders short. These are screen-background cells between surfaces. | Root fill (below). |
| `layout.go:144` `Center` → `lipgloss.Place`, no whitespace background | Plain cells around a modal. Correct inside a ZStack (they must stay transparent); wrong if `Center` is used outside one. | Leave as is. |
| `layout.go:190-209` **`ZStack` treats overlay spaces as transparent**: `strings.TrimSpace(plain) == ""` skips the row, and `nonSpaceBounds` trims to the first and last non-space | **A filled overlay loses its blank rows and its space-padded edges.** Harness: a 10×3 filled card over a filled base came out as just `hello` spliced in; the top and bottom rows and the padding all showed the base. Today's modals survive only because their border glyphs are non-space. A Filled modal (no side lines) or a Banded or Elevated one would be see-through at the edges. | **structural, one file**: opacity should come from "painted" cells (non-space *or* carrying a background) via a cell decode, not from `ansi.Strip`. |
| `layout.go:205-209` the inserted `\x1b[0m` around the splice | Harmless: `ansi.Cut` re-emits the base's earlier escapes, so the right-hand base fragment restores its own background (verified). | none |

---

## Recommendation: where the fill should happen

### 1. Once in the pane, as a fill pass over its own rendered lines (recommended)

`pane.View` already draws everything a surface changes: the frame, the slots, the scrollbars, the header band. It also already holds every pane-owning component's final content: list, table, tree, inspector, logview, textview, eventlog, filter, input, toggle, confirm, alert, the action menu, the help overlay, and every form field. The surface seam has to live here anyway, since `Borderize` is what Filled, Banded and Elevated replace.

The pass:

- opens the surface background at the start of each line;
- re-opens it after every SGR that clears the background (`0`, empty, `49`), found with a real parameter walk;
- pads every line to the pane width.

The scrollbar and slot styles also take the background, so the frame cells carry it explicitly.

**What it fixes in one place:**

- user-supplied cells and labels with lipgloss resets;
- bubbles textinput's bare placeholder pad;
- viewport padding;
- the `Place` and join padding inside the pane;
- every plain gap listed above.

**Nesting composes.** An inner pane (a form field inside a card) emits `…\x1b[0m<inner bg>`, and the outer pass turns that into `…\x1b[0m<outer bg><inner bg>`, so the inner background wins inside its own rect.

**For Outlined** (`NoColor` surface), the pass is a no-op, so today's output stays byte-identical.

**Tradeoffs:**

- **Cost.** It scans every visible line every frame. Measured at about 100 µs per full frame, naive. A no-escape fast path and per-line work keep that well under the frame budget, but it isn't free.
- **It restores *a* background, not the *right* one.** After a reset inside a selected row, the pass re-opens the *surface* tone, not the selection's. Holes in selection (inspector, eventlog, logview over coloured lines, table and list over lipgloss cells) become card-coloured instead of terminal-coloured. That is no worse than today and arguably less visible, but it hides the rule-19 class rather than fixing it. See point 3.
- **SGR correctness.** A naive matcher corrupts truecolor operands containing `0`; the harness showed this. Use x/ansi's parser.
- **It doesn't reach outside panes**: bars, form gaps, layout gaps. Those are screen background by definition; see point 4.

### 2. Per component (rejected as the primary mechanism)

Giving every theme style a background and styling every gap means changing about 40 sites across 15 packages, the rows above marked style or local. It still can't reach:

- strings the app passes in (cells, labels, log lines, node labels with their own lipgloss resets);
- bubbles internals (`textinput.go:725`);
- viewport padding, unless `viewport.Style` is set too.

It also regresses one component at a time: rule 9's filter rule was hand-copied into seven places. The cases that genuinely belong to components are **selection and focus colours**, which are already the open "selection on a filled surface" question in #111. Those are style-local changes to the theme builders (`theme.go:575,603,625,649,671,698`), not fill.

### 3. A shared "re-open after reset" primitive, used by both

`tree.renderPreserving` (`tree.go:1356-1367`) is already this primitive for selection rows. `output.fg` and `ansi.CellColor` are the other workaround (foreground-only closes). Move a parameter-aware version of `renderPreserving` into `pkg/ansi` (for example `ansi.Fill(s, open)`) and use it:

- in the pane, with the surface background (point 1);
- in every row highlight: inspector `:1025`, eventlog `:1302`, logview/textview `:855-861`, table `:2257`, list `:414`.

This fixes today's visible selection holes with the same code, and it gives the enforcement test (below) one invariant to check.

### 4. Whole-frame post-process at the app root: only for the screen background

A pass over `app.View()`'s frame can't tell a card cell from a gap between cards. If it ran as the *card* fill, every hole would turn screen-coloured, which is exactly the defect. After point 1, though, the only cells left without a background *are* screen background: layout joins, `Center` margins, form gaps, short renders. So a root pass that fills "no background" with the screen tone is correct, and simpler than teaching `pkg/layout` to pad with styled whitespace.

**Unverified alternative:** set the terminal's default background with OSC 11 for the session and restore it with OSC 111. Every reset would then fall back to the screen tone for free, but support across tmux and terminals is uncertain.

### 5. ZStack must decide opacity by painted cells, not non-space cells

This is structural but local to `layout.go`'s `zstack.Render`. Without it, Filled, Banded and Elevated modals are see-through at their padding and blank rows. The modals' outer-bounds padding (`menu.go:446`, `alert.go:273`, `overlay.go:410`, `Center`) has to stay *unpainted* for this to work, which the pane-level fill guarantees: the fill runs inside `pane.View`, before those components pad out to their outer rect.

### Enforcement

The harness's classifier (an SGR walk producing a background state per cell) is the shape of the `internal/componenttest` check #111 lists: for each component, render it on a non-Outlined surface and assert that every cell inside its rect carries a background. To catch the rule-19 class, also assert that no row of the cursor highlight changes background mid-row.
