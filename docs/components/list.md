# List

`pkg/list` is a cursor over strings inside a pane, optionally filterable and
markable. Use it for menus, pickers and the master side of a master-detail
screen.

```
┌ Cities ──────────────────────────────┐
│/                                     │
│───────────────────────────────────── │
│▸ London                             █│
│  Tokyo                              █│
│  Madrid                             █│
│  Lima                               █│
│  Nairobi                            ░│
│                                      │
└ 7 / 7 ─────────────────────────── 0% ┘
```

Items are plain strings: format your data before passing it in. The filter is
a case-insensitive substring match. For columns, use a [Table](table.md).

## Usage

```go
opts := t.List()
opts.Title = "Cities"
opts.Filterable = true
opts.Items = names
s.list = list.New(opts)

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    if s.list.IsActivate(msg) { // enter or double-click
        if i, ok := s.list.SelectedIndex(); ok {
            return s, screen.Push(newDetail(cities[i], s.t))
        }
    }
    var cmd tea.Cmd
    s.list, cmd = s.list.Update(msg)
    return s, cmd
}

func (s *Screen) IsCapturingKeys() bool { return s.list.IsCapturingKeys() }
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"List"` | Border title. |
| `Items` | `[]string` | | Initial items (copied). |
| `Filterable` | `bool` | `false` | Adds a filter row inside the pane; `/` focuses it. |
| `Filter` | `filter.Options` | theme | Styles the embedded filter (e.g. `Filter.Placeholder`). |
| `SelectedStyle` | `lipgloss.Style` | bold accent | The cursor row. |
| `HScrollbar` | `bool` | `true` (theme) | Horizontal scroll for long rows. |
| `Markable` | `bool` | `false` | Multi-select with `x`/`X`/`A`/`D`. Requires keyed items. |
| `MarkStyle` | `lipgloss.Style` | theme | The `✓` on marked rows. |
| `BusyWhen` | `func(KeyedItem) (string, bool)` | `nil` | Which items are working (row activity). |
| `Revision` | `func(KeyedItem) string` | `nil` | A value that moves when work on the item finishes. |
| `Activity` | `activity.Options` | theme | Spinner and label style for row activity. |
| `SpinnerStyle`, `LoadingLabel` | | theme | Loading spinner. |
| `Keys` | `list.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | move one row |
| `g`, `G` | `Top`, `Bottom` | first / last row |
| `ctrl+u`, `ctrl+d` | `HalfUp`, `HalfDown` | half a page |
| `/` | `Filter` | focus the filter (`enter` keeps the query, `esc` clears it) |
| `enter` | | activate (via `IsActivate`) |
| `x` | `Mark` | toggle the mark on the cursor row (`Markable`) |
| `X` | `MarkRange` | mark from the anchor to the cursor |
| `A` | `MarkAll` | mark every visible row / clear them |
| `D` | `ClearMarks` | drop all marks |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll |

Mouse: click selects and focuses, double-click activates, the wheel moves the
cursor, a click on the `✓` gutter toggles a mark, shift+click marks a range.

## Data

| Method | Does |
|---|---|
| `SetItems([]string)` | Replace items, re-apply the filter. Clears keys. |
| `SetKeyedItems([]KeyedItem)` | Replace items with stable keys; the cursor follows its key. Use for polled data. |
| `Items()`, `KeyedItems()`, `Visible()` | All items, keyed items, post-filter items. |
| `Selected() (string, bool)` | The cursor row's text. |
| `SelectedIndex() (int, bool)` | The cursor row's index into the **unfiltered** items, for mapping back to your source slice. |
| `SelectedKey() (string, bool)` | The cursor row's key (keyed items only). |
| `Cursor()`, `SetCursor(n)` | Cursor position within the visible rows. |
| `Deselect()` | Move the cursor off every row (a select that demands a deliberate pick). |
| `Value()`, `SetValue(s)` | Filter text. |
| `SetTitle`, `SetTopRight`, `Title()` | Border text. |

`KeyedItem` is `{Key, Display string; Data any}`. `Data` is carried for
`BusyWhen` and otherwise ignored.

## Focus

`Focus()`, `Blur()`, `Focused()`, `FocusFilter()`, `BlurFilter()`,
`Filtering()` (the filter has input), `IsCapturingKeys()`, `FocusToken()`.
See [two regions](../guide/input.md#filterable-components-have-two-regions).

## Multi-select

`ToggleMark`, `MarkRange`, `ToggleMarkAll`, `ClearMarks`, `SetMarks(keys)`,
`Marks()`, `MarkCount()`, `Markable()`, and the two a screen should use:
`Selection()` (marks, or the cursor's key) and `SelectionLabel()` ("api" or
"3 items"). Marks are held by key and inert on anonymous items. See
[Verbs and multi-select](../guide/actions.md#multi-select).

```
┌ Services ────────────────────────────┐
│ ✓ api                                │
│   web                                │
│▸✓ worker                             │
│   cache-redis                        │
└──────────────────────────────────────┘
```

## Row activity

`Dispatch`, `DispatchEach`, `Done`, `BeginRead`, `ApplyRead(rd, items, err)`,
`ReadsFailing()`, `ActivityCount()`, `ActivityState()` /
`SetActivityState()`. The spinner draws as a right-aligned badge. See
[Row activity](../guide/refresh.md#row-activity).

## Messages

| Message | When |
|---|---|
| `list.ActivatedMsg{Index, Item, Token}` | double-click on a row (prefer `IsActivate`) |
| `list.SelectedChangedMsg{Index, Item, Empty}` | the cursor lands on a different item; drive "detail on hover" from it |

## State

`State()` / `Restore(st)` carry items (keyed or not), cursor, filter, marks,
activity, loading and scroll across a rebuild.

See `examples/components/list` and `examples/shell/stack`.
