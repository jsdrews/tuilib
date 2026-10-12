# Eventlog

`pkg/eventlog` shows events and lines read in order from a source too large to
hold: an AWX job's events, an Elasticsearch index, a run's logs. It's
[LogView](logview.md)'s counterpart for data fetched in pages rather than
streamed whole.

The **item** is the unit. An item has a key, the lines it draws as, and
optional data for an inspector. A multi-line event draws as a block; an item
with no lines draws as a dim `· no output`. The cursor moves item by item.
There's no sort: the order is the timeline.

## Usage

Bind it to a source with [`pkg/remote`](../guide/remote.md#binding-a-view-pkgremote):

```go
s.log = remote.NewEventlog(t.Eventlog(), remote.Seekable[eventlog.Item]{
    Page: s.page, // func(ctx, remote.Window) ([]eventlog.Item, total int, error)
    Find: s.find, // optional: search past what's loaded
})

func (s *Screen) Init() tea.Cmd { return tea.Batch(s.log.Init(), s.log.SetGrowing(s.jobRunning)) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    if s.log.IsActivate(msg) {
        if it, ok := s.log.Selected(); ok {
            return s, screen.Push(newEventDetail(it.Data, s.t))
        }
    }
    return s, s.log.Update(msg)
}

func (s *Screen) SetTheme(t theme.Theme) { s.log.Restyle(t.Eventlog()) }
```

Set `Filterable` and `Searchable` on the options to enable the two fields.

## Seekable and Anchored

| | Seekable | Anchored |
|---|---|---|
| Data | item N fetchable, with a total | walked from an anchor, no total |
| Options | default | `Anchored: true` |
| Feed | `SetPage(items, offset, total, answered)`: adjacent pages merge, a far page replaces | `Append` / `Prepend(items, answered)`: grown at either edge |
| Gutter | position (or `Item.Mark`) | `Item.Mark` only |
| Source | `source.Model` with `MaxHeld` | `source.Anchored` |

Either way the eventlog holds at most `MaxItems`, trimming the end furthest
from the viewport.

## Item

| Field | Meaning |
|---|---|
| `Key` | Identity across fetches; for anchored data, its cursor. |
| `Lines` | What it draws as, oldest first. None draws `· no output`. |
| `Data` | Carried to `ActivatedMsg` for an inspector. |
| `Hole` | A position the source has no item for yet (an event saved out of order). |
| `Mark` | Gutter label: a timestamp, id or counter, always the source's own. Leave empty if the line text already has the time. |

## Options

| Field | Type | Default | Meaning |
|---|---|---|---|
| `Title` | `string` | `"events"` | Border title. |
| `Anchored` | `bool` | `false` | Hold a span grown with `Append`/`Prepend` instead of offset pages. |
| `Filterable` | `bool` | `false` | A filter (`f`) that narrows the query on the server. |
| `Searchable` | `bool` | `false` | Search (`/`) that jumps to matches. |
| `MaxItems` | `int` | 5000 | Resident cap. |
| `Placeholder` | `string` | `·` | Drawn for items not yet fetched. |
| `NoGutter` | `bool` | `false` | Hide the position/mark gutter. |
| `NewStyle`, `NewFor` | | theme, 2s | Briefly highlight items that just loaded beside ones already on screen. Negative `NewFor` disables. |
| `MatchStyle`, `SelectedStyle`, `CellStyle`, `GutterStyle` | `lipgloss.Style` | theme | |
| `Filter` | `filter.Options` | theme | Styles both the filter and search fields. |
| `Keys` | `eventlog.Keys` | `DefaultKeys()` | Key bindings. |
| pane pass-throughs | | theme | See [Components](index.md#pane-pass-through-options). |

## Keys

| Key | Binding field | Action |
|---|---|---|
| `↑`/`k`, `↓`/`j` | `Up`, `Down` | previous / next item |
| `g` | `Top` | first item |
| `G` | `Bottom` | newest item, and follow |
| `ctrl+u`, `ctrl+d` | `HalfUp`, `HalfDown` | half a page |
| `enter` | `Open` | activate the item |
| `f` | `Filter` | filter (narrows the query) |
| `/` | `Search` | search (jumps; highlights resident matches as you type) |
| `n`, `N` | `NextMatch`, `PrevMatch` | next / previous match, past what's loaded via `Find` |
| `←`/`h`, `→`/`l`, `0`, `$` | `Pane.*` | horizontal scroll |

## Search jumps, filter narrows

They're separate controls on purpose. The **filter** is part of the query: it
returns fewer items, and committing it emits `QueryChangedMsg`. **Search**
moves the view to the next match and keeps the context around it. `n`/`N`
step through resident matches, then emit `FindMsg` for the source to answer
(`Found(ok, at)` for seekable data, `FoundAt(ok, key)` for anchored). A hit
lands once its item is loaded.

## Following growing data

`SetGrowing(true)` says items are still arriving at the newest end. While the
view is on the newest item it follows; scroll away and new items are counted
on the border (`↓ 42 new`) instead; `G` returns. Before the first answer,
`SetFollow(false)` makes the view open at the first item, which is where to
start reading a finished job.

## Slow sources

Shared with [Table](table.md#remote-data): items answering an older query stay
on screen dimmed, the border names what they answer and what's loading,
Loading shows only before the first answer, and a failure reads `✗ failed`.

## Methods

| Method | Does |
|---|---|
| `SetPage`, `Append`, `Prepend`, `Reanchor`, `SetMore`, `Edges` | Feed data (see above). |
| `Selected() (Item, bool)`, `Items()`, `Cursor()`, `SetCursor(i)` | Read and move. |
| `SetGrowing`, `SetFollow`, `Following()`, `SetNew(n, more)` | Follow. |
| `Value()`, `SetValue(s)`, `Query()` | Filter. |
| `Term()`, `SetTerm(s)`, `SetMatchTotal(n)`, `Found`, `FoundAt` | Search. |
| `SetFailed(answer)`, `Stale()`, `Failed()`, `Answered()`, `Committed()` | Remote state. |
| `Focus`, `Blur`, `FocusFilter`, `FocusSearch`, `BlurFilter`, `IsCapturingKeys`, `IsActivate` | Focus and input. |

## Messages

| Message | When |
|---|---|
| `eventlog.ActivatedMsg{Key, Data, Token}` | `enter` or double-click on an item |
| `eventlog.QueryChangedMsg{Raw, Terms}` | a filter was committed |
| `eventlog.FindMsg{Term, Newer, From, FromKey, Token}` | `n`/`N` ran past resident items |
| `eventlog.ViewportChangedMsg{…}` | what's on screen and what's held; feed it to the source |

`pkg/remote` handles all of these for you.

## State

`State()` / `Restore(st)` carry items, query, failure, cursor, scroll, follow,
growth, filter and search. With `pkg/remote`, use `Restyle`.

See `examples/patterns/eventlog` (seekable, growing),
`examples/patterns/anchored` (anchored) and the
[remote data design note](../remote-data.md).
