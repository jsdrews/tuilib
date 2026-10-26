# LogView

`pkg/logview` is a streaming text viewer: append lines as they arrive, follow
the tail, scroll back, search, and filter down to matching lines. The buffer
is capped so an open-ended stream can't grow memory without limit.

```
┌ api logs ────────────────────────────────────┐
│/                                             │
│───────────────────────────────────────────── │
│12:00:01 INFO  starting api v1.42.0          █│
│12:00:01 INFO  listening on :8080            █│
│12:00:04 WARN  slow query: 1.2s              █│
│12:00:07 INFO  GET /healthz 200              █│
│12:00:09 ERROR upstream timeout              █│
│reconnected after 42s                        ░│
│                                              │
└ FOLLOWING ─────────────────────────────── 0% ┘
```

Use it for a stream held whole: tailing a pod, a subprocess's output, a
websocket. For static text use [TextView](textview.md); for a timeline paged
from a source too large to hold, use [Eventlog](eventlog.md).

## Usage

```go
opts := t.Logview()
opts.Title = "api logs"
opts.Searchable = true
opts.MaxLines = 50_000
s.log = logview.New(opts)

case lineMsg:
    s.log.Append(string(msg))
    return s, s.readNext() // chain the next read
```

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"logs"` | Border title. |
| `Surface` | `surface.Resolved` | the app's surface (`t.Surface`) | How the edge and body are drawn. Set by the theme; set it yourself only to pin this component to another surface, with `t.Resolve(s)`. See [Surfaces](../surfaces.md). |
| `Searchable` | `bool` | `false` | Search row inside the pane; `/` focuses it. |
| `MaxLines` | `int` | `0` → 10,000 | Buffer cap. A positive value sets it; `-1` disables it (only for bounded producers). |
| `FilterMode` | `bool` | `false` | Start in filter mode (only matching lines, once a query is set). |
| `LineNumbers` | `bool` | `false` | Number lines in a gutter (markers aren't numbered). |
| `GutterStyle` | `lipgloss.Style` | theme | The line-number gutter. |
| `MatchStyle` | `lipgloss.Style` | bold reverse accent | Highlight on matches. |
| `CurrentLineStyle` | `lipgloss.Style` | subtle background | The line holding the current match. |
| `CurrentLineBlurredStyle` | `lipgloss.Style` | unset | `CurrentLineStyle` while the logview doesn't have focus. Used only when it sets a background; filled surfaces set it to `SelectionDim`. |
| `Filter` | `filter.Options` | theme | Styles the search field. |
| `HScrollbar` | `bool` | `true` (theme) | Horizontal scroll for long lines. |
| `SpinnerStyle`, `LoadingLabel` | | theme | |
| `Keys` | `logview.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j`, `pgup`, `pgdown` | | scroll (scrolling up pauses follow) |
| `g` | `Top` | top |
| `G` | `Bottom` | bottom, and resume following |
| `/` | `Search` | search; typing highlights; `enter` keeps the query, `esc` clears it |
| `n`, `N` | `NextMatch`, `PrevMatch` | next / previous match |
| `\` | `Filter` | toggle filter mode |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll |

The bottom-left slot reads `FOLLOWING` or `PAUSED`, plus the match position
and `filter` when those are active.

## Methods

| Method | Does |
|---|---|
| `Append(line)`, `AppendLines(lines)` | Add at the bottom; scrolls when following. |
| `AppendMarker(text)` | Add a dimmed annotation line ("reconnected after 42s", "container restarted"). |
| `Prepend(lines)` | Add above the oldest line ("load older"); the view stays on what the user was reading. |
| `Clear()` | Empty the buffer, drop the query, resume following. |
| `Lines()` | The buffer (aliased; copy to keep). |
| `Following()`, `SetFollow(b)` | Follow state. |
| `Query()`, `SetQuery(s)`, `Searching()` | Search state. |
| `FilterMode()`, `SetFilterMode(b)` | Filter mode. |
| `SetTitle`, `Title()` | Border title. |
| `SetMatchStyle`, `SetCurrentLineStyle` | Restyle in place. |
| `SetLoading(b) tea.Cmd` | Loading spinner. |

Pair with [`pkg/resume`](../reference/packages.md#resume) for pod-log
reconnects at one-second resolution. See `examples/remotedata/podlogs`.

## State

`State()` / `Restore(st)` carry the lines and which are markers, follow,
search, filter mode, current match, loading and scroll.

See the output pane in `examples/processes`.
