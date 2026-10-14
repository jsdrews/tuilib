# Pane

`pkg/pane` is a bordered, titled, scrollable region. It holds any string
content, draws a vertical scrollbar, truncates long lines to its width with
horizontal scroll past the cut, and can replace its body with a loading
spinner. Every other interactive component embeds one.

```
┌ notes ───────────────────────────────────────────┐
│A pane is a bordered, titled, scrollable region. █│
│Every component owns one.                        █│
│                                                 █│
│line 4                                           █│
│line 5                                           █│
│line 6                                           ░│
└ bottom-left slot ──────────────────────────── 0% ┘
```

Use a bare pane for read-only text you render yourself: a help panel, a
summary, a preview. For a document with search, use [TextView](textview.md);
for a stream, [LogView](logview.md). Never wrap another component in a pane;
set the component's `Title` instead.

## Usage

```go
opts := t.Pane()
opts.Title = "notes"
s.notes = pane.New(opts)
s.notes.SetContent(renderNotes())

// Update: forward messages so scroll keys and the wheel work
s.notes, cmd = s.notes.Update(msg)

// Layout
layout.Sized(&s.notes)
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `""` | Text in the title slot. |
| `Surface` | `surface.Resolved` | the app's surface (`t.Surface`) | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `TitlePosition` | `pane.BorderPosition` | `TopLeftBorder` | Which of the six border slots holds the title. |
| `Focused` | `bool` | `false` | Start in the focused (active) style. |
| `ActiveColor`, `InactiveColor` | `lipgloss.TerminalColor` | theme | Border color when focused / not. |
| `ActiveBorder`, `InactiveBorder` | `lipgloss.Border` | normal border | Border shape when focused / not. |
| `Glyphs` | `glyph.Set` | `glyph.Default()` | Scrollbar thumb and track. |
| `SlotBrackets` | `pane.SlotBracketStyle` | `SlotBracketsNone` | How slot text meets the border line. An alias for `surface.Brackets`: on a themed pane it comes from the surface. |
| `DividerStyle`, `DividerActiveStyle` | `lipgloss.Style` | unset | The divider under a header set with `SetDividedHeader`, on a line border. A surface's own divider takes their place. |
| `HScrollbar` | `bool` | `false` | Reserve one row for a horizontal scrollbar. |
| `Spinner` | `*spinner.Spinner` | `spinner.Dot` | Frames used while loading. |
| `SpinnerStyle` | `lipgloss.Style` | theme accent | Spinner style. |
| `LoadingLabel` | `string` | `""` | Text beside the spinner. |
| `Keys` | `pane.Keys` | `DefaultKeys()` | Horizontal scroll bindings. |
| `Width`, `Height` | `int` | | Set by the layout engine; leave zero under `layout.Sized`. |

### Border slots

A pane has six text slots on its border: `TopLeftBorder`, `TopMiddleBorder`,
`TopRightBorder`, `BottomLeftBorder`, `BottomMiddleBorder`,
`BottomRightBorder`. The title takes one; components use the others for
counts, status ("FOLLOWING"), filter hints and validation errors. The
bottom-right slot shows the scroll percentage unless something overrides it.

| `SlotBrackets` | Renders as |
|---|---|
| `SlotBracketsNone` | `── title ──` |
| `SlotBracketsCorners` | `┐ title ┌` |
| `SlotBracketsTees` | `─┤ title ├─` |

## Keys

| Key | Action |
|---|---|
| `↑`/`k`, `↓`/`j`, `pgup`, `pgdown` | scroll vertically (viewport) |
| `←`/`h`, `→`/`l` | scroll horizontally by 4 cells |
| `0`/`home`, `$`/`end` | jump to the left / right edge |
| wheel | scroll 3 lines, under the pointer |
| click/drag scrollbar | jump / drag |

## Methods

| Method | Does |
|---|---|
| `SetContent(s)` | Replace the content. Lines are cut to the inner width (ANSI-aware). |
| `SetTitle`, `SetTitlePosition` | Change the title and its slot. |
| `SetTopLeft`, `SetTopRight`, `SetBottomLeft`, `SetBottomMiddle`, `SetBottomRight` | Write a border slot. `SetBottomRight("")` restores the scroll percentage. |
| `SetHeader(s)` | Pin lines under the top border, above the scrolling content (how filters sit inside panes). |
| `SetFocused(b)`, `Focused()` | Active style. |
| `SetLoading(b) tea.Cmd`, `SetLoadingLabel`, `Loading()` | Spinner state. Return the command from `SetLoading(true)`. |
| `GotoTop`, `GotoBottom`, `SetYOffset`, `SetXOffset`, `EnsureVisible(n)` | Scroll programmatically. |
| `YOffset`, `XOffset`, `MaxXOffset`, `AtBottom` | Read scroll position. |
| `VisibleRows`, `VisibleWidth`, `Width`, `Height` | Inner and outer size. |
| `Rect`, `ContentRect`, `VScrollbarRect`, `HScrollbarRect` | Geometry for hit-testing. |
| `RowAt(x, y)` | Map a terminal position to a content row (accounts for border and scroll). |
| `ScrollWheel(x, y, up)` | Apply a wheel event; reports whether it was consumed. |
| `HandleScrollbar(mouse.Msg)` | Process a click or drag on the scrollbars. |
| `SetVirtualScroll(total, visible, offset)` | Drive the scrollbar from logical counts when you window content yourself. |
| `SetActiveColor`, `SetInactiveColor`, `SetActiveBorder`, `SetInactiveBorder`, `SetSlotBrackets`, `SetSpinnerStyle` | Restyle in place. |
| `SetSurface(r)`, `SetDividerStyles(inactive, active)` | Change the surface, or the line-border divider colours. |
| `SetDividedHeader(row, input)` | Pin a header row with a divider the pane draws beneath it; `input` lights the divider while the header holds the keys. |
| `InnerRect()`, `Insets()` | The rect inside the edge (header, divider and content), and how many cells the edge takes on each side. Use these rather than assuming a one-cell border: a filled surface's edge is deeper. |

## Helpers

- `pane.Borderize(content, border, color, slots, brackets)` draws a border
  with slot text around any string.
- `pane.Scrollbar(height, total, visible, offset)` and
  `pane.HScrollbar(width, total, visible, offset)` render standalone
  scrollbars (`…With` variants take a `glyph.Set`).

## Notes

- Don't pre-wrap content to avoid terminal wrap glitches. The pane truncates
  to its width and offers horizontal scroll. Wrap only genuine prose.
- See `examples/components/pane` for border styles, title positions and slot
  brackets side by side.
