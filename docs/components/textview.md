# TextView

`pkg/textview` is a read-only viewer for a static document: a rendered README,
a diff, `kubectl describe` output, an API response body. It scrolls, searches
and wraps. For text that keeps arriving, use [LogView](logview.md).

```
┌ README.md ───────────────────────────────────┐
│/                                             │
│───────────────────────────────────────────── │
│tuilib is a component library for building   █│
│Bubble Tea TUIs quickly and reliably. Each   █│
│component is a small package with an Options █│
│struct, a New constructor, and the standard  █│
│Init/Update/View methods.                    █│
│                                             ░│
└────────────────────────────────────────── 0% ┘
```

## Usage

```go
opts := t.TextView()
opts.Title = "describe pod/api"
opts.Searchable = true
opts.Wrap = true
s.doc = textview.New(opts)

// later, when the content arrives
s.doc.SetContent(out)
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"text"` | Border title. |
| `Surface` | `surface.Resolved` | the app's surface (`t.Surface`) | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Content` | `string` | `""` | Initial text. |
| `Wrap` | `bool` | `true` (theme) | Word-wrap to the pane width (ANSI-aware). When off, long lines truncate and scroll horizontally. |
| `Searchable` | `bool` | `false` | Search row inside the pane; `/` focuses it. |
| `MatchStyle` | `lipgloss.Style` | bold reverse accent | Highlight on matches. |
| `CurrentLineStyle` | `lipgloss.Style` | subtle background | The line holding the current match. |
| `CurrentLineBlurredStyle` | `lipgloss.Style` | unset | `CurrentLineStyle` while the textview doesn't have focus. Used only when it sets a background; filled surfaces set it to `SelectionDim`. |
| `Filter` | `filter.Options` | theme | Styles the search field. |
| `SpinnerStyle`, `LoadingLabel` | | theme | |
| `Keys` | `textview.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j`, `pgup`, `pgdown` | | scroll |
| `ctrl+u`, `ctrl+d` | | half a page |
| `g`, `G` | `Top`, `Bottom` | top / bottom |
| `/` | `Search` | search; `enter` keeps the query, `esc` clears it |
| `n`, `N` | `NextMatch`, `PrevMatch` | next / previous match |
| `w` | `Wrap` | toggle wrap |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll (wrap off) |

## Methods

| Method | Does |
|---|---|
| `SetContent(s)` | Replace the text and scroll to the top. An active query is kept and re-matched. |
| `Content()` | The raw text. |
| `Wrap()`, `SetWrap(b)` | Wrap state. |
| `Query()`, `SetQuery(s)`, `Searching()` | Search state. |
| `SetTitle`, `Title()` | Border title. |
| `SetMatchStyle`, `SetCurrentLineStyle` | Restyle in place. |
| `SetLoading(b) tea.Cmd` | Loading spinner. |

## State

`State()` / `Restore(st)` carry content, wrap, search and current match,
loading and scroll.

See `examples/components/textview`.
