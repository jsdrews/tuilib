# Inspector

`pkg/inspector` shows one structured record as aligned label/value rows, with
nested objects and arrays expandable underneath. Use it for "show me the
fields of this thing": a pod spec, a REST response, a job's details, an event
opened from an [eventlog](eventlog.md).

```
┌ pod ───────────────────────────────────────┐
│▾ metadata                                 █│
│    name     : api-7d8b6c5b4-x2k7p         █│
│    namespace: default                     █│
│▾ spec                                     █│
│  ▸ containers                             █│
│    nodeName  : node-2                     ░│
│▾ status                                   ░│
│                                            │
└───────────────────────────────────── 1 / 9 ┘
```

## Usage

Build fields by hand, or convert decoded JSON:

```go
var payload map[string]any
_ = json.Unmarshal(body, &payload)

opts := t.Inspector()
opts.Title = "pod"
opts.Filterable = true
opts.InitialDepth = 1
opts.Fields = inspector.FromMap(payload)
s.ins = inspector.New(opts)
```

```go
type Field struct {
    Label    string
    Value    string
    Children []Field
}
```

`FromAny(v)` accepts any JSON-shaped value (maps, slices, scalars, nil);
`FromMap(m)` a `map[string]any`. Map keys are sorted; slice items are labeled
`[0]`, `[1]`, …. Labels of siblings align into a column.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"details"` | Border title. |
| `Surface` | `surface.Resolved` | the app's surface (`t.Surface`) | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Fields` | `[]inspector.Field` | | The record. |
| `Filterable` | `bool` | `false` | Search row inside the pane; `/` focuses it. |
| `InitialDepth` | `int` | `0` | Pre-expand fields shallower than this. Re-applied by `SetFields`. |
| `LabelStyle`, `ValueStyle` | `lipgloss.Style` | theme | Label and value text. |
| `MatchStyle` | `lipgloss.Style` | theme | Search highlight. |
| `CurrentLineStyle` | `lipgloss.Style` | theme | The cursor row. |
| `CurrentLineBlurredStyle` | `lipgloss.Style` | unset | `CurrentLineStyle` while the inspector doesn't have focus. Used only when it sets a background; filled surfaces set it to `SelectionDim`. |
| `Filter` | `filter.Options` | theme | Styles the search field. |
| `HScrollbar`, `SpinnerStyle`, `LoadingLabel` | | theme | |
| `Keys` | `inspector.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | move one row |
| `g`, `G` | `Top`, `Bottom` | first / last row |
| `ctrl+u`, `ctrl+d` | `HalfUp`, `HalfDown` | half a page |
| `space` | `Toggle` | expand or collapse the cursor row |
| `E`, `C` | `ExpandAll`, `CollapseAll` | expand / collapse everything |
| `}`, `{` | `NextSibling`, `PrevSibling` | jump between siblings |
| `J`, `K` | `NextLeaf`, `PrevLeaf` | jump between leaves |
| `/` | `Search` | search labels and values |
| `n`, `N` | `NextMatch`, `PrevMatch` | next / previous match |
| `\` | `Filter` | filter mode: hide non-matching subtrees, keep ancestors |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll |

Mouse: click selects, a click on `▸`/`▾` toggles, the wheel moves the cursor.

## Methods

| Method | Does |
|---|---|
| `SetFields(fs)` | Replace the record, keeping expansion and cursor by row path. The auto-refresh primitive. |
| `Selected() (Field, bool)` | The field under the cursor. |
| `Cursor()`, `SetCursor(n)` | Cursor row. |
| `ExpandAll()`, `CollapseAll()` | Programmatic expansion. |
| `Query()`, `SetQuery(s)`, `Searching()` | Search state. |
| `FilterMode()`, `SetFilterMode(b)` | Filter mode. |
| `SetTitle`, `Title()` | Border title. |
| `SetLoading(b) tea.Cmd` | Loading spinner. |

No marking: an inspector is a record viewer, and no verb acts on a set of its
fields.

## State

`State()` / `Restore(st)` carry fields, open objects, cursor, search, filter
mode, loading and scroll.

See `examples/components/inspector`.
