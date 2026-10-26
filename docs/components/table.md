# Table

`pkg/table` is a cursor-driven table of records inside a pane: a pinned
header, sortable columns, fixed / content-sized / flex widths, a filter with
column-scoped and regex terms, multi-select, row activity, and remote paging
where scrolling is the pagination.

```
┌ Cities ────────────────────────────────────────┐
│/                                               │
│─────────────────────────────────────────────── │
│City ▲               │ Region     │  Population │
│─────────────────────────────────────────────── │
│Lima                 │ Americas   │       10.0M │
│London               │ Europe     │        8.9M │
│Madrid               │ Europe     │        3.3M │
│Nairobi              │ Africa     │        4.4M │
│Tokyo                │ Asia       │       13.9M │
│                                                │
└ 5 / 5 ────────────────────────────────── 1 / 5 ┘
```

## Usage

```go
opts := t.Table()
opts.Title = "Cities"
opts.Filterable = true
opts.Columns = []table.Column{
    {Title: "City", Flex: 1, Sortable: true},
    {Title: "Region", Width: 10, Sortable: true},
    {Title: "Population", Width: 11, Align: lipgloss.Right, Sortable: true, Less: popLess},
}
opts.Rows = []table.Row{{"London", "Europe", "8.9M"}, {"Tokyo", "Asia", "13.9M"}}
s.table = table.New(opts)

if s.table.IsActivate(msg) {
    if row, ok := s.table.Selected(); ok {
        return s, screen.Push(newCityDetail(row[0], s.t))
    }
}
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"Table"` | Border title. |
| `Surface` | `surface.Resolved` | the app's surface (`t.Surface`) | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Columns` | `[]table.Column` | **required** | Column layout (below). |
| `Rows` | `[]table.Row` | | Initial rows; `Row` is `[]string`, one cell per column. |
| `Filterable` | `bool` | `false` | Filter row inside the pane; `/` focuses it. |
| `Filter` | `filter.Options` | theme | Styles the embedded filter. |
| `FilterMode` | `table.FilterMode` | `FilterLocal` | `FilterRemote`: report filters as `QueryChangedMsg` instead of applying them. Requires `Filterable`. |
| `SortMode` | `table.SortMode` | `SortLocal` | `SortRemote`: report sorts instead of applying them. |
| `SortDebounce` | `time.Duration` | 400ms | Quiet period before a remote sort commits. Negative commits immediately. |
| `Anchored` | `bool` | `false` | Hold a span of anchored rows grown with `AppendRows` / `PrependRows`. |
| `MaxItems` | `int` | 5000 | Resident cap for an anchored span. |
| `Placeholder` | `string` | `"·"` | Cell text for rows a window doesn't hold. |
| `HeaderStyle` | `lipgloss.Style` | bold | Header row. |
| `SelectedStyle` | `lipgloss.Style` | bold accent on subtle bg | Cursor row. |
| `SelectedBlurredStyle` | `lipgloss.Style` | unset | Cursor row while the table doesn't have focus. Used only when it sets a background; filled surfaces set it to `SelectionDim`. |
| `CellStyle` | `lipgloss.Style` | none | Non-selected rows. |
| `Borders` | `table.Borders` | `│` and `─` | Interior separators (below). |
| `HScrollbar` | `bool` | `true` (theme) | Horizontal scroll for wide tables. |
| `Markable`, `MarkStyle` | | `false` | Multi-select. Requires keyed rows. |
| `ActivityColumn` | `string` | `""` | Column (by title prefix) whose cell shows the activity spinner. |
| `BusyWhen` | `func(KeyedRow) (string, bool)` | `nil` | Which rows are working. |
| `Revision` | `func(KeyedRow) string` | `nil` | A value that moves when work on the row finishes. |
| `Activity` | `activity.Options` | theme | Spinner styling (cell-safe). |
| `SpinnerStyle`, `LoadingLabel` | | theme | Loading spinner. |
| `Keys` | `table.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

### Columns

| Field | Meaning |
|---|---|
| `Title` | Header text. Also the name `key:value` filter terms and `ActivityColumn` match against. |
| `Width` | Visible cell width. ANSI escapes don't count. |
| `Align` | `lipgloss.Left` (default), `lipgloss.Right`, `lipgloss.Center`. |
| `Sortable` | Opt the column into sorting. A sortable column is 2 cells wider than its base width, so the ▲ / ▼ is never cut off. |
| `Less` | Custom comparator for the column (numbers, dates, "8.3M"). Default is case-insensitive on the ANSI-stripped text. |
| `Flex` | Weight for a share of leftover width; `Width` becomes a minimum. |
| `MaxWidth` | Cap on the effective width; surplus goes to other flex columns. |
| `Hidden` | Keep the column in the data (filterable, in `Selected`, in messages) without drawing it. A hidden column is left out of the `[` / `]` sort cycle. |

| `Width` | `Flex` | Sizing |
|---|---|---|
| > 0 | 0 | fixed |
| 0 | 0 | content-auto: widest of title and cells, floor 4 |
| any | > 0 | grows by weight into leftover space |

When the table is narrower than the base widths, flex columns don't grow and
the overflow goes to horizontal scroll. Widths recompute on row, column and
size changes.

### Borders

`Borders{Vertical, HeaderRule}` are pre-styled glyph strings. `Vertical`
replaces the column gap with `" <glyph> "`; `HeaderRule` repeats its glyph
under the header. Style them with `ansi.CellColor` (foreground-only) so the
selected row's background passes through. Empty strings disable them.

### Colored cells

Color cell text with `ansi.CellColor(n, text)`, not `lipgloss.Render`: a full
`\x1b[0m` reset inside a cell breaks the selected row's background.
`ansi.Hyperlink(url, label)` makes a cell open a URL on shift/cmd-click even
when truncated. See [`pkg/ansi`](../reference/packages.md#ansi).

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | move one row |
| `g`, `G` | `Top`, `Bottom` | first / last row |
| `ctrl+u`, `ctrl+d` | `HalfUp`, `HalfDown` | half a page |
| `/` | `Filter` | focus the filter |
| `tab` (in the filter) | | complete a `key:value` term |
| `[`, `]` | `SortPrev`, `SortNext` | step the sort column among sortable columns |
| `s` | `SortDir` | toggle sort direction |
| `shift+←`, `shift+→` | `ColPrev`, `ColNext` | snap horizontal scroll to the previous / next column |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll |
| `x`, `X`, `A`, `D` | `Mark`, `MarkRange`, `MarkAll`, `ClearMarks` | multi-select |
| `enter` | | activate (via `IsActivate`) |

Mouse: click a row to select, double-click to activate, click a header to sort
by it, wheel to move, click the `✓` gutter to toggle a mark, shift+click for a
range.

## Filter syntax

The filter splits on whitespace into AND-ed terms:

| Term | Matches |
|---|---|
| `europe` | any cell containing "europe" (case-insensitive) |
| `region:eu` | the column whose title starts with "region" |
| `~^new` | any cell matching the regex (case-insensitive) |
| `region:~^euro` | a regex scoped to one column |
| `name=tolkien` | the column's value equals "tolkien" (case-insensitive) |
| `status!=done` | the column's value isn't "done" |
| `year>1950`, `year>=1950` | greater than (or equal to) 1950 |
| `year<2000`, `year<=2000` | less than (or equal to) 2000 |

The ordering operators compare numerically when both the cell and the value
are numbers, and as case-insensitive text otherwise. Operators apply to
scoped terms only: a bare `>=5` is the text ">=5". Each term's `Op` says
which comparison it makes, so a remote source gets `year>=1950` as a
comparison it can translate rather than as text.

An ambiguous or unknown key is treated as literal text, as is a regex that
doesn't compile, so the filter never refuses input. While typing a
`key:value` term, the filter's bottom-left slot lists that column's matching
values and `tab` completes to their longest common prefix, after any
operator. The grammar lives
in [`pkg/query`](../reference/packages.md#query).

## Data and selection

| Method | Does |
|---|---|
| `SetRows([]Row)` | Replace rows; cursor kept by index. |
| `SetKeyedRows([]KeyedRow)` | Replace rows with stable keys; the cursor follows its key. Use for polled data. |
| `Rows()`, `KeyedRows()`, `Visible()` | All rows, keyed rows, post-filter/sort rows. |
| `Selected() (Row, bool)` | The cursor row. `false` when empty, or on a placeholder under `SetWindow`. |
| `SelectedIndex()`, `SelectedKey()` | Index into the unfiltered rows; the row's key. |
| `Cursor()`, `SetCursor(n)` | Cursor within the visible rows. |
| `Columns()`, `SetColumns(cols)` | Column layout. |
| `SortColumn()`, `SortDescending()`, `SetSort(col, desc)` | Sort state (`-1` = none). |
| `Value()`, `SetValue(s)` | Filter text. |
| `SetDistinct(col, values)` | Completion candidates for a column (for remote filters). |
| `SetTitle`, `Title()` | Border title. |
| `SetHeaderStyle`, `SetSelectedStyle`, `SetCellStyle` | Restyle rows in place. |

`KeyedRow` is `{Key string; Cells []string; Data any}`.

## Remote data

| Method | Does |
|---|---|
| `SetWindow(rows, offset, total, answered)` | Hold rows `[offset, offset+len)` of `total` (`-1` if unknown); everything else draws as placeholder. |
| `Window() (offset, count, total)` | The resident window. |
| `Query() QueryChangedMsg` | The query a source should be answering now (for the first fetch). |
| `SetFailed(answer)` | The fetch for this query failed. |
| `Stale()`, `Failed()`, `Answered()`, `Committed()` | Remote state. |
| `CommittedSort()`, `StagedSort()`, `SetStagedSort()` | Debounced remote sort state. |
| `RefuseSort(col)`, `SortUnavailable(col)`, `ColumnByTitle(title)` | A sort the source refused (see below). |
| `AppendRows`, `PrependRows`, `Edges()`, `SetMore`, `Reanchor()` | Anchored spans. |

`table.Answer{Raw, Sort, Desc}` names the query a set of rows answers.

**Refused sorts.** Some APIs list columns as sortable and then reject sorting
by them with a 400. The fetch marks that error with `remote.Refused(err)`,
and the table then:

- goes back to the sort its rows answer, without a request, so the rows are
  current again and the border goes back to normal;
- makes the column **unavailable** for the life of the table: `[`, `]`, `s`
  and header clicks skip it, and its title is drawn with
  `Options.UnavailableTitle` (the theme dims it). This survives a theme swap
  through `State` / `Restore`;
- on the first load, with nothing to go back to, drops the sort and asks once
  more unsorted;
- returns `table.SortRefusedMsg{Column, Title, Err}` and posts nothing
  itself. The screen decides what to tell the user.

It's a sort refusal only when the failed query differs from the answered one
in its sort alone. Any other refused query is an ordinary failure.
Prefer [`pkg/remote`](../guide/remote.md#binding-a-view-pkgremote), which
does all of this for you.

## Messages

| Message | When |
|---|---|
| `table.ActivatedMsg{Row, Cells, Token}` | double-click on a row (prefer `IsActivate`) |
| `table.RowFocusedMsg{Row, Cells, Columns, Empty}` | the cursor lands on a different row |
| `table.ViewportChangedMsg{FirstVisible, LastVisible, TotalRows}` | the visible slice changed: the signal to fetch |
| `table.QueryChangedMsg{Raw, Terms, Sort, SortColumn, Desc}` | (remote modes) a filter was committed or a sort settled |
| `table.SortRefusedMsg{Column, Title, Err}` | (from `pkg/remote`) the source refused a sort; the table has gone back to the answered one |

Filters are reported on commit (`enter`, `esc` or blur), never per keystroke.

## Multi-select and activity

Same surface as [List](list.md#multi-select): `Selection()`,
`SelectionLabel()`, `Marks()`, `SetMarks`, `ToggleMark`, `MarkRange`,
`ToggleMarkAll`, `ClearMarks`, `MarkCount`; and `Dispatch`, `DispatchEach`,
`Done`, `BeginRead`, `ApplyRead`, `ReadsFailing`. Marking needs
`SetKeyedRows` and is inert under `SetWindow`.

## State

`State()` / `Restore(st)` carry rows (a window, a span or the whole set),
cursor, scroll, filter, sort (committed and staged), marks, activity, the
answered query, failure, completion values and loading.

See `examples/workbench`, `examples/remotedata/records` and
`examples/liveops`.
