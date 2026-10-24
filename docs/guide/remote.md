# Remote data

When the data lives behind an API and may be too large for one request,
tuilib lets **scrolling be the pagination**. The user scrolls; windows of
rows arrive under them. There are no page keys and no "next page" button.

## Choose the component by the data's shape

Classify two things first.

**What an item is:**

- a **Record**: fields you compare across items (a deployment, a book);
- an **Event**: fields in a timeline read in order (an AWX job event);
- a **Line**: raw text read in order (a log line).

**How an item is reached:**

- **Seekable**: item N can be fetched directly and there is a total
  (`?offset=200&limit=100`);
- **Anchored**: items are walked forwards or backwards from an anchor, with no
  reliable total (Elasticsearch `search_after`, logs filtered by time);
- **Streamed**: only the tail, then whatever arrives.

**Growing** data gains items while the user watches (a running job).

| | Seekable | Anchored | Streamed |
|---|---|---|---|
| **Record** | `table` via `remote.NewTable` + `Seekable` | `table` via `remote.NewTable` + `Anchored` | `table` + `SetKeyedRows` from a watch |
| **Event** | `eventlog` via `remote.NewEventlog` + `Seekable`, `inspector` on enter | `eventlog` via `remote.NewEventlog` + `Anchored`, `inspector` on enter | `logview` + `inspector` |
| **Line** | `eventlog` via `remote.NewEventlog` + `Seekable` | `eventlog` via `remote.NewEventlog` + `Anchored` | `logview` + `pkg/resume` |

Rules of thumb:

- **If it fits in one request, don't page it.** Use `SetRows` or a `textview`.
- **Events never go in a table.** A timeline has one order; a table exists to
  sort and compare. Put an event's fields one `enter` away, in an inspector.
- **Search jumps, filter narrows.** A filter is part of the query and returns
  fewer items; search moves the view to the next match and keeps the context
  around it. The eventlog has both, as separate keys.
- **Push channels are hints.** Websockets that drop messages under load can't
  be the source of truth; poll, and let a push make the next poll sooner.

The [remote data design note](../remote-data.md) classifies specific sources
(Prefect, AWX, Elasticsearch, pod logs, generic REST).

## Binding a view: `pkg/remote`

Start here. `pkg/remote` binds a `table` or `eventlog` to a source, so the
screen writes only the fetch, as ordinary synchronous Go:

```go
s.tab = remote.NewTable(opts, remote.Seekable[table.KeyedRow]{
    Page:     s.page,
    PageSize: 100,
})

func (s *Screen) Init() tea.Cmd                       { return s.tab.Init() }
func (s *Screen) Layout() layout.Node                 { return layout.Sized(s.tab) }
func (s *Screen) IsCapturingKeys() bool               { return s.tab.IsCapturingKeys() }
func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) { return s, s.tab.Update(msg) }
func (s *Screen) SetTheme(t theme.Theme)              { s.tab.Restyle(s.options(t)) }

// page runs off the UI goroutine. ctx is cancelled when a newer request
// supersedes this one.
func (s *Screen) page(ctx context.Context, w remote.Window) ([]table.KeyedRow, int, error) {
    q := url.Values{}
    q.Set("offset", strconv.Itoa(w.Offset))
    q.Set("limit", strconv.Itoa(w.Limit))
    if w.Sort != "" {
        q.Set("sort", w.Sort)
    }
    for _, t := range w.Terms {
        if t.Title != "" && t.Regex == nil {
            q.Set(strings.ToLower(t.Title), t.Value) // region:eu → ?region=eu
        }
    }
    req, err := http.NewRequestWithContext(ctx, "GET", base+"/apps?"+q.Encode(), nil)
    // … do it, decode, return rows and the total
}
```

The view embeds its component, so `Selected`, `IsActivate`, `SetDistinct`,
focus and help are all there. The binding adds:

| Method | Does |
|---|---|
| `Init()` | requests the first page |
| `Update(msg) tea.Cmd` | routes between view and source, runs fetches; forward every message |
| `Restyle(opts)` | rebuilds on a theme swap, keeping rows, filter, sort, cursor and failure |
| `Refresh()` | refetches what's on screen under the same query (a retry) |
| `SetGrowing(b)` | the data is still gaining items: follow the newest and poll |
| `SetAnchor(a)` | (Anchored) move to `source.Newest()`, `source.Oldest()` or `source.At(cursor)` |

### Shapes and request types

| Shape | Fetch functions | Request |
|---|---|---|
| `remote.Seekable[T]` | `Page(ctx, Window) (items, total, err)`, optional `Find(ctx, Find) (at, found, err)` | `Window{Filter, Offset, Limit}` |
| `remote.Anchored[T]` | `Edge(ctx, Edge) (items, more, err)`, optional `Find(ctx, Find) (cursor, found, err)` | `Edge{Filter, Newer, Cursor, Inclusive, Limit}` |

`T` is `table.KeyedRow` for a table and `eventlog.Item` for an eventlog. Every
request embeds the committed `Filter{Raw, Terms, Sort, Desc}`, where scoped
filter terms carry their resolved column title. With `Find` nil, search covers
only what's loaded.

Shared tuning fields on both shapes: `Name` (labels the query history),
`PageSize` (default 100), `Follow` (poll interval for growing data, default
2s), `ViewportDelay` (default 150ms), `Context` (parent of every request
context). `Seekable` also has `Prefetch` (extra pages beyond the screen);
`Anchored` has `Anchor` (where the view starts, default newest).

Worked examples: `examples/patterns/remote` (a seekable table),
`examples/patterns/eventlog` (a seekable, growing eventlog over an AWX-shaped
API), `examples/patterns/anchored` (an Elasticsearch-shaped anchored
eventlog).

## What the library does for a slow source

With replies taking seconds, the screen still does nothing special:

- **Stale, not blank.** Committing a new filter or sort leaves the old rows on
  screen, dimmed, with the border naming the query they answer and the one
  loading. They can still be read and scrolled. The cursor goes to the top
  when the new answer lands. The body spinner shows only before the very first
  answer. `Stale()` lets a verb refuse to act on stale rows.
- **Debounced sort.** A remote sort is staged until sort input has been quiet
  for 400ms (`table.Options.SortDebounce`), so `]` `]` `s` is one request.
- **Debounced scroll.** Scrolling asks only once the viewport has been still
  for 150ms, and not at all while the committed query is unanswered.
- **Placeholders.** Rows not yet received render as `·`, `Selected` reports
  `ok=false` for them, and the border reads `⠙ loading rows 201–300`.
- **Cancellation.** Every request's context is cancelled when a newer request
  supersedes it, so at most one request per source is live on the server.
- **Failures.** A failed query stays stale with `✗ failed` on the border and
  the user's filter and sort kept. Committing it again, or `Refresh`, retries.
  Nothing retries automatically.
- **Refused sorts.** When the fetch marks an error with `remote.Refused(err)`
  (an API answering an unsupported sort with a 400, say) and the query
  changed only its sort, the table goes back to the answered sort with no
  request, takes the column out of the sort cycle, and returns
  `table.SortRefusedMsg`. Your fetch decides what counts as refused: tuilib
  never guesses from status codes. See
  [Table](../components/table.md#remote-data).
- **History.** Each committed query's outcome (answered with timing, failed,
  cancelled) is written to the output console, and failures to the statusbar.

## Completion for remote filters

A table under a remote filter stops scraping completion values from its rows,
because one page of a larger set completes to wrong answers. Feed it real
values from a facet endpoint:

```go
s.tab.SetDistinct(1, regions) // column 1's candidates for tab completion
```

## The manual loop: `pkg/source`

When `pkg/remote` doesn't fit, drive the table and a `source.Model` yourself.
The table reports, the source decides what to ask for, and the screen fetches:

```go
s.src = source.New(source.Options{PageSize: 100})

func (s *Screen) Init() tea.Cmd { return s.src.Init() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
    var cmds []tea.Cmd
    switch m := msg.(type) {
    case table.ViewportChangedMsg:
        cmds = append(cmds, s.src.Viewport(m.FirstVisible, m.LastVisible))
    case table.QueryChangedMsg:
        cmds = append(cmds, s.src.SetQuery(m.Raw, m.Terms, m.Sort, m.Desc))
    case source.RequestMsg:
        return s, s.fetch(m.Query) // your HTTP call, under m.Query.Ctx
    case fetchedMsg:
        ok, cmd := s.src.Deliver(m.page)
        switch {
        case !ok: // superseded; drop it
        case m.page.Err != nil:
            s.table.SetFailed(answered(m.query))
        default:
            s.table.SetWindow(m.rows, m.page.Offset, m.page.Total, answered(m.query))
        }
        return s, cmd
    }
    var cmd tea.Cmd
    s.table, cmd = s.table.Update(msg)
    cmds = append(cmds, cmd, s.src.Update(msg))
    return s, tea.Batch(cmds...)
}

func answered(q source.Query) table.Answer {
    return table.Answer{Raw: q.Raw, Sort: q.Sort, Desc: q.Desc}
}
```

The table must use `FilterMode: table.FilterRemote` and
`SortMode: table.SortRemote`. **Check what `Deliver` returns and return its
command**: the bool is what stops a slow reply to an old query painting over
the current one, and the command carries the query history and any follow-up
request. Fetch under `Query.Ctx` so superseded requests are cancelled on the
wire.

`source.Options`: `Name`, `Mode` (`ByOffset`, or `ByCursor` for
continuation-token APIs that only walk forward), `PageSize`, `Prefetch`,
`ViewportDelay`, `Context`, `MaxHeld` (hold a range rather than one window,
for an eventlog), `Follow`. `source.NewAnchored(source.AnchoredOptions{…})` is
the anchored counterpart. See [Packages: source](../reference/packages.md#source).

## Don'ts

- **Don't paginate by swapping `SetRows` per page.** Use `SetWindow`.
- **Don't add `n`/`p` page keys.** They collide with reserved keys and add a
  second navigation model.
- **Don't give a windowed table content-auto column widths.** Columns would
  reflow on every window. Use fixed `Width` or `Flex`.
- **Don't emit a query per keystroke.** The table reports filters on commit.
- **Don't call `SetLoading` or `SetCursor(0)` in a remote loop.** The table
  decides both.
- **Don't read a placeholder as data.** Check `Selected`'s bool.
- **Don't let state-restoration setters fetch.** `SetSort` / `SetValue`
  adopt state silently; call `Query()` yourself if you set a filter
  programmatically and want it fetched.
