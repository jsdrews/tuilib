# Components

Every component is a package under `pkg/` with an `Options` struct, a
`New(Options)` constructor and the Bubble Tea `Init` / `Update` / `View`
methods. Every interactive one owns a bordered `pane.Pane`, so `View()`
returns a finished box and `Options.Title` draws on its border.

## Catalog

| Component | Package | Use it for |
|---|---|---|
| [Pane](pane.md) | `pkg/pane` | A bordered, scrollable region for arbitrary text; the primitive under every other component. |
| [List](list.md) | `pkg/list` | A cursor over strings, optionally filterable and markable. Menus, pickers, master lists. |
| [Menu](menu.md) | `pkg/menu` | A grouped, filterable list of things to open: an app's sections, a home screen. |
| [Table](table.md) | `pkg/table` | Records with columns: sort, scoped filters, flex widths, remote paging. |
| [Tree](tree.md) | `pkg/tree` | Hierarchies: expand/collapse, search, filter that keeps ancestors. |
| [Inspector](inspector.md) | `pkg/inspector` | One structured record as aligned label/value rows (a k8s manifest, a JSON response). |
| [TextView](textview.md) | `pkg/textview` | A static document: scroll, search, wrap. |
| [LogView](logview.md) | `pkg/logview` | A stream of lines held whole: follow, search, filter mode, capped buffer. |
| [Eventlog](eventlog.md) | `pkg/eventlog` | A timeline paged from a source too large to hold. |
| [Filter](filter.md) | `pkg/filter` | A "/-to-focus, enter commits, esc clears" text field. |
| [Input](input.md) | `pkg/input` | A plain bordered text field, including passwords. |
| [Toggle](toggle.md) | `pkg/toggle` | A yes/no selector. |
| [Form](form.md) | `pkg/form` | Text, password, select and confirm fields with validation and a submit button. |
| [Confirm](confirm.md) | `pkg/confirm` | A yes/no modal. |
| [Alert](alert.md) | `pkg/alert` | An acknowledge-this modal. |
| [Action menu](action-menu.md) | `pkg/action` | A screen's verbs in a self-sizing menu. |
| [Tabs](tabs.md) | `pkg/tab` | Several screens behind a one-row tab strip. |
| [Breadcrumb and statusbar](bars.md) | `pkg/breadcrumb`, `pkg/statusbar` | The shell's header and footer rows. |
| [Metrics](metrics.md) | `pkg/metrics` | Inline badges, ratios, bars and sparklines for cells. |

## The common shape

```go
opts := t.List()             // pre-styled Options from the theme
opts.Title = "Cities"        // what's specific to this instance
opts.Filterable = true
s.list = list.New(opts)

// in Update — forward every message
s.list, cmd = s.list.Update(msg)

// in Layout
layout.Sized(&s.list)
```

- **Start from the theme builder** (`t.List()`, `t.Table()`, …). It fills in
  colors, border shapes, glyphs, key bindings and nested filter options.
- **Don't set `Width`/`Height`** on a component placed with `layout.Sized`.
- **Forward every message.** Each component decides what to act on. Spinner
  ticks, focus requests and activity messages all arrive through `Update`.
- **Label with `Title`**, never with a line above the component.

## Capabilities shared across components

Most components implement the same small interfaces, so the surrounding
machinery works with any of them.

| Capability | Methods | Components |
|---|---|---|
| Placement | `SetRect(geom.Rect)`, `View()` | all |
| State across rebuilds | `State()`, `Restore(State)` | list, menu, table, tree, inspector, textview, logview, eventlog, filter, input, toggle |
| Key hints | `Help()`, `HelpSections()` | all interactive (filter has `Help` only) |
| Focus | `Focus() tea.Cmd`, `Blur()`, `Focused()` (satisfies `focus.Focusable`) | list, menu, table, tree, inspector, textview, logview, eventlog, filter, input, toggle |
| Key capture | `IsCapturingKeys()` | list, menu, table, tree, inspector, textview, logview, eventlog, filter, input, form |
| Filter regions | `FocusFilter()`, `BlurFilter()` | list, menu, table, tree, inspector, textview, logview, eventlog |
| Open the selection | `IsActivate(msg)`, `ActivatedMsg` | list, table, eventlog (menu: `Chosen(msg)`, `ChosenMsg`) |
| Loading spinner | `SetLoading(bool) tea.Cmd`, `SetLoadingLabel`, `Loading()` | pane and every pane-owning viewer |
| Keyed data | `SetKeyedItems` / `SetKeyedRows`, `SelectedKey()` | list, table |
| Multi-select | `Options.Markable`, `Selection()`, `SelectionLabel()`, `Marks()`, `SetMarks`, `ClearMarks` | list, table, tree |
| Row activity | `BusyWhen`, `Dispatch`, `DispatchEach`, `Done`, `BeginRead`, `ApplyRead`, `ReadsFailing` | list, table, tree |
| Restyle in place | `SetActiveColor`, `SetInactiveColor`, `SetActiveBorder`, `SetInactiveBorder`, `SetTitle` | most |
| Custom keys | `Options.Keys` (from `DefaultKeys()`) | list, menu, table, tree, inspector, textview, logview, eventlog, action menu |

## Pane pass-through options

Every pane-owning component's Options repeat the pane's chrome fields. The
theme builder fills them; override one only deliberately.

| Field | Meaning |
|---|---|
| `ActiveColor`, `InactiveColor` | border color when focused / not |
| `ActiveBorder`, `InactiveBorder` | border shape when focused / not (`lipgloss.Border`) |
| `Glyphs` | the `glyph.Set` the component and its scrollbars draw with |
| `SlotBrackets` | how the title meets the border: `pane.SlotBracketsNone`, `SlotBracketsCorners`, `SlotBracketsTees` |
| `HScrollbar` | reserve a row for a horizontal scrollbar and enable `←`/`→` scrolling |
| `SpinnerStyle`, `LoadingLabel` | the loading spinner's style and caption |

## Loading state

Any pane-owning component can replace its body with a centered spinner while
data is in flight:

```go
s.list.SetItems(nil)            // drop stale data first
cmd := s.list.SetLoading(true)  // returns the spinner's first tick: return it
// … later, on the fetched message:
s.list.SetItems(items)
s.list.SetLoading(false)
```

Remote-bound tables and eventlogs manage this themselves; don't call it there.
See `examples/patterns/loading`.

## Surfaces

Every component that owns a pane takes an `Options.Surface`: how its edge and
body are drawn, from a line border to a filled card with a drop shadow. The
theme builders set it, handing each component the form it needs:

| Form | Components |
|---|---|
| the surface itself | pane, list, menu, table, tree, inspector, textview, logview, eventlog |
| its **field form** (at most one cell of edge per side, so it fits a three-row slot) | filter, input, toggle |
| its **overlay form** (always filled, so it covers what is under it) | confirm, alert, action menu, help overlay |

You don't set it; the user picks the surface and the shell rebuilds every
screen in it. To pin one component to a different surface, resolve it
against the same theme:

```go
opts := t.Logview()
opts.Surface = t.Resolve(surface.Outlined)
```

See [Surfaces](../surfaces.md).
