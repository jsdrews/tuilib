# Tree

`pkg/tree` is an expand/collapse view over any hierarchy, with search that
highlights matches and a filter mode that hides non-matching subtrees while
keeping their ancestors visible.

```
┌ cluster ─────────────────────────────────┐
│▾ prod                                   █│
│  ▾ default                              █│
│      api-7d8b6c5b4-x2k7p                █│
│      web-56c45fd5ff-7p9mw               █│
│  ▾ kube-system                          ░│
│                                          │
└────────────────────────────────────── 0% ┘
```

## Usage

The tree reads your data through a two-method interface, so you don't convert
your model into tuilib types:

```go
type Node interface {
    Label() string
    Children() []Node
}
```

```go
type dir struct {
    name string
    kids []tree.Node
}

func (d dir) Label() string          { return d.name }
func (d dir) Children() []tree.Node { return d.kids }

opts := t.Tree()
opts.Title = "files"
opts.Searchable = true
opts.InitialDepth = 1
opts.Root = buildTree()
s.tree = tree.New(opts)
```

The root's own label is the first row. Pass a synthetic root if your data has
several top-level items. Labels may contain ANSI color (status icons); the
cursor row's highlight survives it.

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"tree"` | Border title. |
| `Surface` | `surface.Resolved` | the app's surface (`t.Surface`) | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Root` | `tree.Node` | | The top-level node. |
| `Searchable` | `bool` | `false` | Search row inside the pane; `/` focuses it. |
| `InitialDepth` | `int` | `0` | Pre-expand nodes shallower than this. 0 shows only the root, 1 expands the root, 2 its children too. |
| `MatchStyle` | `lipgloss.Style` | bold reverse accent | Highlight on matched text. |
| `CurrentLineStyle` | `lipgloss.Style` | subtle background | The cursor row. |
| `CurrentLineBlurredStyle` | `lipgloss.Style` | unset | `CurrentLineStyle` while the tree doesn't have focus. Used only when it sets a background; filled surfaces set it to `SelectionDim`. |
| `Markable`, `MarkStyle` | | `false` | Multi-select, keyed by node path. |
| `BusyWhen` | `func(Node) (string, bool)` | `nil` | Which nodes are working (row activity). |
| `Revision` | `func(Node) string` | `nil` | A value that moves when work on the node finishes. |
| `Activity` | `activity.Options` | theme | Activity badge style. |
| `Filter` | `filter.Options` | theme | Styles the search field. |
| `HScrollbar`, `SpinnerStyle`, `LoadingLabel` | | theme | |
| `Keys` | `tree.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | move one row |
| `g`, `G` | `Top`, `Bottom` | first / last row |
| `space` | `Toggle` | expand or collapse the cursor row |
| `enter` | `Enter` | select |
| `E`, `C` | `ExpandAll`, `CollapseAll` | expand everything / collapse to the root |
| `}`, `{` | `NextSibling`, `PrevSibling` | jump between siblings |
| `J`, `K` | `NextLeaf`, `PrevLeaf` | jump between leaves |
| `/` | `Search` | focus the search field |
| `n`, `N` | `NextMatch`, `PrevMatch` | next / previous match |
| `\` | `Filter` | toggle filter mode (only matches and their ancestors) |
| `x`, `X`, `A`, `D` | `Mark`, `MarkRange`, `MarkAll`, `ClearMarks` | multi-select |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll |

Arrows and `hjkl` never expand or collapse; they're reserved for scrolling.
Drilling into a child is `space` then `↓`. Mouse: click selects, a single
click on `▸`/`▾` toggles, the wheel moves the cursor.

## Methods

| Method | Does |
|---|---|
| `SetRoot(n)`, `Root()` | Replace the tree. Expansion and cursor are kept by path; this is the auto-refresh primitive. |
| `Selected() (Node, bool)` | The node under the cursor. |
| `Cursor()`, `SetCursor(n)` | Cursor row. |
| `Query()`, `SetQuery(s)`, `Searching()` | Search text and whether the field has input. |
| `FilterMode()`, `SetFilterMode(b)` | Filter mode. |
| `SetTitle`, `Title()` | Border title. |
| `SetMatchStyle`, `SetCurrentLineStyle` | Restyle in place. |

Paths are label-based ("prod/default/api-…"); duplicate sibling labels get a
suffix (`Pod`, `Pod:2`).

### Multi-select

A node's path is its key, so no keyed setter is needed. Marking a branch marks
that node alone. `Marks()` returns paths in document order, including marks
inside collapsed branches; prefix-test them to get a subtree. `A` marks only
visible rows (expanded and surviving the filter). `Selection()` and
`SelectionLabel()` work as on [List](list.md#multi-select). See
`examples/workbench`.

### Row activity

`BusyWhen` is evaluated over every node, collapsed ones included. Reads go
through `ApplyRead(rd, root, err)`; keys are paths. See
[Row activity](../guide/refresh.md#row-activity).

## Messages

| Message | When |
|---|---|
| `tree.SelectedChangedMsg{Path, Label, Depth, Empty}` | the cursor lands on a different node |

## State

`State()` / `Restore(st)` carry the root, which branches are open, cursor,
search and filter mode, marks, activity, loading and scroll.

See `examples/workbench`.
