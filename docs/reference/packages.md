# Packages

The non-visual packages, and the shell-level ones whose surface isn't covered
on a component page. For the full API of any package, `go doc -all
github.com/jsdrews/tuilib/pkg/<name>` (or
[pkg.go.dev](https://pkg.go.dev/github.com/jsdrews/tuilib)) is authoritative.

## app

The shell. See [The app shell](../guide/app-shell.md) for `app.Options`.

| Symbol | Kind | Does |
|---|---|---|
| `New(Options) Model` | func | Build the shell; resolves the initial theme and calls the root's `SetTheme`. |
| `Info(s)`, `Error(s)` | `tea.Cmd` | Statusbar message (and a console record). |
| `InfoDetail(summary, body)`, `ErrorDetail(…)` | `tea.Cmd` | Summary to the statusbar, body to the console. |
| `ErrorOf(err)` | `tea.Cmd` | `err.Error()` to the statusbar, the unwrapped `%w` chain to the console. |
| `ClearStatus()` | `tea.Cmd` | Clear the statusbar message. |
| `SetPalette(name)` | `tea.Cmd` | Switch to a named palette in `Options.Palettes` (live preview). |
| `SetSurface(name)` | `tea.Cmd` | Switch to a named surface in `Options.Surfaces` (live preview). |
| `OutputClosed` | type | The value the console pops with; ignore it in `OnEnter`. |
| `MouseOff`, `MouseClick` | `MouseMode` | `Options.Mouse` values. |
| `Model.Theme()` | method | The current theme. |

## screen

| Symbol | Does |
|---|---|
| `Screen` | The interface every screen implements. See [Screens](../guide/screens.md). |
| `Push(s)`, `Pop(result)`, `PopTo(depth, result)`, `Replace(s)` | Stack commands. |
| `NewStack(root)`, `Stack.Current`, `Depth`, `Crumbs`, `SetTheme`, `Update`, `Init` | The stack itself, for programs not using the shell. |

## layout

See [Layout](../guide/layout.md). `VStack`, `HStack`, `ZStack`, `Fixed`,
`Flex`, `Sized`, `Bar`, `Center`, `RenderFunc`; `Node` (`Render(geom.Rect)
string`) and `Sizer` (`SetRect(geom.Rect)`, `View() string`).

## geom

Absolute rectangles and the render generation. A leaf package.

| Symbol | Does |
|---|---|
| `Rect{X, Y, W, H, Gen}` | Position and size in terminal cells, stamped with the frame it was drawn in. |
| `New(x, y, w, h)` | A rect stamped with the current generation (for a render root). |
| `NextGen()`, `Gen()` | Advance / read the render generation. Call `NextGen` once per frame at the root. |
| `Rect.Hit(x, y)` | Inside **and** drawn this frame. Use it to claim mouse events. |
| `Rect.Contains(x, y)` | Pure geometry, ignoring generation. |
| `Rect.Fresh()`, `Empty()`, `Inset(n)` | Generation check, emptiness, shrink by `n` on every side. |
| `CenterIn(outer, w, h)` | The rect a centered child occupies (matches `lipgloss.Place`). |
| `AnchorIn(outer, x, y, w, h)` | The rect a child at `(x, y)` occupies, pushed back inside `outer`. |

## mouse

| Symbol | Does |
|---|---|
| `Msg` | `tea.MouseMsg` plus `Clicks` (1 single, 2 double, 0 not a press). What components handle. |
| `Msg.IsPress()` | Left press, the event components act on. |
| `Msg.IsDoubleClick()` | Second press in the same cell within the interval. |
| `Msg.IsRightPress()`, `IsPointPress()` | Right press; either button. |
| `Msg.IsWheelUp()`, `IsWheelDown()` | Wheel. |
| `Tracker`, `NewTracker(interval)`, `Track(e, now)` | Turns `tea.MouseMsg` into `Msg`. The shell owns one. |
| `DefaultDoubleClickInterval` | 500ms. |

Right-click is an accelerator only: some terminals (macOS Terminal.app) never
send it.

## focus

See [Focus groups](../guide/input.md#focus-groups).

| Symbol | Does |
|---|---|
| `NewGroup(items...)` | A group over focusables in tab order; first focused. Pass pointers. |
| `Group.Init`, `Update`, `Is(f)`, `Focused()`, `Index()`, `SetIndex(i)`, `Len()` | Use and state. |
| `Group.IsCapturingKeys()`, `Help()`, `HelpSections()` | Forward these from the screen. |
| `Group.WithKeys(Keys{Next, Prev})`, `WithoutWrap()` | Customize cycling. |
| `Focusable` | `Focus() tea.Cmd`, `Blur()`, `Focused() bool`. |
| `Capturer` | `IsCapturingKeys() bool`. |
| `Identified`, `Token`, `NewToken()` | A component's stable identity in focus requests. |
| `Request(target)`, `RequestSelf(token)` | Commands asking the owning group for focus. |
| `DefaultKeys()` | `tab` / `shift+tab`. |

## help

See [Help and key hints](../guide/help.md).

| Symbol | Does |
|---|---|
| `Section{Title, Bindings}`, `Sectioned` | A named group; the interface for providing groups. |
| `Group(title, bindings...)` | One section (dropped if empty). |
| `Sections(secs...)` | Assemble, dropping empties. |
| `SectionsOf(parts...)` | Collect from components, groups, sections and binding lists. |
| `Flatten(secs)` | Sections → flat bindings, for `Help()`. |
| `Qualify(owner, secs)` | Prefix titles with an owner ("files · Navigate"). |
| `Suppress(claimed, secs)` | Remove bindings whose keys are claimed elsewhere. |
| `Compile`, `CompileSections` | Dedupe. |
| `SectionNavigate` … `SectionTabs` | The shared vocabulary. |
| `New(Options)` → `Model` | The footer: `ShortView`, `ShortViewBudget`, `SetBindings`, `SetMinimal`. |
| `NewOverlay(OverlayOptions)` → `Overlay` | The modal: `SetSections`, `SetRect`, `Update`, `View`; emits `ClosedMsg`. |

## theme, palette, surface, glyph

See [Theming and state](../guide/theming.md), the
[theme token reference](theme.md) and [Surfaces](../surfaces.md).

| Symbol | Does |
|---|---|
| `palette.Palette`, `palette.All()`, `palette.ByName` | The colours, and the 19 built-ins. |
| `surface.Surface`, `surface.Builtins()` | How edges and bodies are drawn, as data: `Outlined`, `Filled`, `Banded`, `Elevated`, `Accented`. |
| `surface.Derive`, `surface.Resolve` | Role colours from a palette's seeds; a surface paired with colours and the terminal's capabilities, following its fallback chain. |
| `theme.Compose(p, s, caps)`, `theme.New(p)` | A theme from a palette and a surface; `New` is Outlined. |
| `t.Resolve(s)` | Another surface against the same palette, for a per-component override. |

`glyph.Set` is the full mark vocabulary; `glyph.Default()` returns it and
`Set.Resolve()` fills empty fields from it.

## ansi

Helpers for color inside cells whose row draws its own background. A leaf
package.

| Function | Does |
|---|---|
| `CellColor(n, text)` | Foreground color by 256-palette index, closed with `\x1b[39m` so an outer background survives. |
| `CellColorRGB(r, g, b, text)` | The same with a 24-bit color. |
| `Hyperlink(url, text)` | An OSC 8 link: shift/cmd-click opens `url` even when `text` is truncated. |
| `ExtractHyperlink(s) (url, text, ok)` | Pull the URL back out, for "open in browser". |

## config

The user's `$XDG_CONFIG_HOME/tuilib/config.yaml` (default
`~/.config/tuilib/config.yaml`). Opt-in: the library never writes it.

| Symbol | Does |
|---|---|
| `Config{Palette, Surface, BlockGlyphs, Theme, DoubleClickMs}` | `palette:`, `surface:`, `block_glyphs:`, `theme:` (old name for `palette:`) and `double_click_ms:`. Unknown keys are ignored. |
| `Load()`, `LoadFrom(path)` | Read; a missing file is `(Config{}, nil)`. Errors only on bad YAML or I/O. |
| `Path()` | The path that would be read. |

## query

The filter grammar shared by local and remote filters. A leaf package.

| Symbol | Does |
|---|---|
| `Parse(input, columns) []Term` | Split into AND-ed terms; never fails. |
| `Term{Column, Title, Op, Value, Text, Regex, Raw}` | One term; `Title` is the resolved column title for scoped terms (`-1`/`""` when bare). `Value` is lowercased, `Text` as typed. |
| `Op`: `OpContains` (zero value), `OpRegex`, `OpEq`, `OpNe`, `OpGt`, `OpGe`, `OpLt`, `OpLe` | The comparison a term makes: `key:v`, `key:~re`, `key=v`, `key!=v`, `key>v`, `key>=v`, `key<v`, `key<=v`. `Op.String()` is the operator as typed. |
| `Term.Match(cell)`, `MatchAll(cells, terms)` | Matching (ANSI-stripped, case-insensitive). |
| `ColumnByPrefix(key, columns)` | Unique column whose title starts with `key`, or `-1`. |
| `ActiveTerm(input, columns)` | The scoped term being typed (`key:value`, or with any comparison operator). |
| `Distinct(rows, ncols)`, `NormalizeValues(values)` | Build completion candidates. |
| `Candidates(distinct, col, prefix)`, `Complete(input, columns, distinct)` | Hints and tab completion. |

## source

Coordinates paged fetches without doing I/O. See
[Remote data](../guide/remote.md#the-manual-loop-pkgsource).

| Symbol | Does |
|---|---|
| `New(Options) Model` | Offset or cursor paging. |
| `Model.Init`, `Viewport(first, last)`, `SetQuery(raw, terms, sort, desc)`, `Deliver(Page)`, `Update`, `Refresh`, `Find`, `SetGrowing`, `SetHeld`, `Cancel` | The loop. |
| `Model.Revert()`, `Anchored.Revert()` | Go back from a query that failed unanswered to the last answered one, with no request (how a refused sort recovers). |
| `NewAnchored(AnchoredOptions) Anchored` | Edge paging from an anchor. |
| `Newest()`, `Oldest()`, `At(cursor)` | Anchors. |
| `Query` | One request: `Offset`, `Limit`, `Cursor`, `Raw`, `Terms`, `Sort`, `Desc`, `Gen`, `Ctx`, `Poll`, `Probe`, `FromAnchor`, `Find`, `Term`, `Dir`, … |
| `Page` | One reply: `Gen`, `Offset`, `Count`, `Total`, `Next`, `Err`, `Found`, `Cursor`, `More`. |
| `RequestMsg{Query, Source}` | "Fetch this." |
| `QueryAnsweredMsg`, `QueryFailedMsg`, `QueryCancelledMsg`, `QueryRecoveredMsg` | Query history, written to the console by the shell. |
| `ByOffset`, `ByCursor` | `Options.Mode`. |
| `DefaultPageSize` (100), `DefaultViewportDelay` (150ms), `DefaultFollow` (2s) | Defaults. |

`Options`: `Name`, `Mode`, `PageSize`, `Prefetch`, `ViewportDelay`,
`Context`, `MaxHeld`, `Follow`. `AnchoredOptions`: `PageSize`, `Name`,
`ViewportDelay`, `Context`, `Follow`, `Anchor`.

## remote

Binds a table or eventlog to a source. See
[Remote data](../guide/remote.md#binding-a-view-pkgremote).

| Symbol | Does |
|---|---|
| `NewTable(table.Options, Shape[table.KeyedRow]) *Table` | A bound table. |
| `NewEventlog(eventlog.Options, Shape[eventlog.Item]) *Eventlog` | A bound eventlog. |
| `Seekable[T]{Page, Find, Name, PageSize, Prefetch, Follow, ViewportDelay, Context}` | Offset-addressable data. |
| `Anchored[T]{Edge, Find, Anchor, Name, PageSize, Follow, ViewportDelay, Context}` | Anchor-walked data. |
| `Window`, `Edge`, `Find`, `Filter` | Typed requests your fetch functions receive. |
| `Init`, `Update`, `Restyle`, `Refresh`, `SetGrowing`, `SetAnchor` | The binding's methods; everything else is the embedded component's. |
| `Refused(err)`, `ErrRefused` | Mark a fetch error as a refusal; `errors.Is(err, remote.ErrRefused)` checks it. A refused sort sends a `Table` back to its answered sort and returns `table.SortRefusedMsg`. |

## poll

See [Auto-refresh](../guide/refresh.md#polling-with-pkgpoll). `New(Options{Interval,
Paused})`, `Init`, `Update` (returns `RefreshMsg` on its tick), `Refresh`,
`Pause`, `Resume`, `SetInterval`, `MarkRefreshed`, `MarkRefreshedAt`,
`LastRefresh`, `Paused`, `Interval`. Panics on a non-positive interval.

## activity

Per-row in-flight state, wrapped by `list`, `table` and `tree`. A leaf package.
See [Row activity](../guide/refresh.md#row-activity).

| Symbol | Does |
|---|---|
| `Observed`, `Held` | `Mode`: who knows when the work ends. |
| `Acknowledged`, `Ended`, `Withdrawn` | `Outcome` of `Done`. |
| `Busy(values...)`, `Settled(values...)` | Predicate builders (ANSI and space stripped). |
| `Options{Spinner, Style, Settle}` | Spinner frames, a styling function, and how many no-news reads an unconfirmed claim survives. |
| `Set` | The keyed collection: `Dispatch`, `Done`, `BeginRead`, `Accept`, `ObserveRows`, `Render`, `Badge`, `State`, `Count`, `Active`, `ReadsFailing`, `Adopt`, `Handle`. |
| `State{Label, Since, Unknown}` | One key's state. |
| `UnobservedMsg{Op, Keys, Label, Changed}` | Work that ended without any read seeing it busy. |

## runner

See [Running subprocesses](../guide/subprocesses.md). A leaf package.

| Symbol | Does |
|---|---|
| `Run(cmd)`, `RunWithNotice(cmd, notice)`, `RunWith(Options{Cmd, Notice, NoClear})` | Hand the terminal to a subprocess; posts `Result{Cmd, Err}`. |
| `Capture(cmd)`, `CaptureWith(CaptureOptions{Cmd, Label, Tag})` | Run in the background, streaming output. |
| `Go(label, fn)`, `GoWith(GoOptions{Label, Detail, Run, Tag, Context})` | Run a Go function the same way. |
| `CaptureStarted`, `CapturedLine`, `CaptureStatus`, `Captured` | The message sequence. |
| `Next(msg)` | Read the next message (the shell does this). |
| `Kill(started)` | Stop the process and everything it spawned. |

## output

The app-wide console. See
[Feedback and the console](../guide/feedback.md#the-output-console).

| Symbol | Does |
|---|---|
| `OptionsFrom(theme) Options` | Console options from a theme. |
| `Options` | `MaxRecords`, `ExportDir`, `SourceWidth`, colors, badge styles, `Logview`, `Confirm`, `Picker`, `Keys`. |
| `Record{Time, Level, Source, Text, Head, Stderr, RunID}` | One line. |
| `Buffer`, `NewBuffer(max)` | The ring, unread accounting and run registry. |
| `Screen`, `NewScreen(buf, opts)` | The console screen. |
| `Closed` | The pop sentinel (`app.OutputClosed`). |
| `Notice{Text, Level}` | Asks the shell to show text on the statusbar. |
| `LevelInfo`, `LevelError` | Severity. |
| `DefaultMaxRecords` (10,000), `DefaultSourceWidth` (10) | Defaults. |

## resume

Exact resume for a stream whose "since" parameter is coarser than its lines
(Kubernetes pod logs truncate `sinceTime` to the second). No I/O.

| Method | Does |
|---|---|
| `New(Options{Granularity})` | Default granularity is one second. |
| `Observe(ts)` | Record each kept line's timestamp. |
| `Lost(now)` | The stream ended (dropped connection). |
| `Resume(now) (since, skip, ok)` | Start a reconnect: ask for lines since `since` and skip the first `skip`. |
| `Drop(ts)` | After a resume, whether a line repeats one already held. |
| `Since()`, `Seen()` | Where to resume; whether anything was observed. |
| `Reconnected()` | How long the stream was down, to mark with `logview.AppendMarker`. |
| `OlderCut(stamps)`, `ObserveOlder(stamps)` | "Load older" from a longer tail, merged with `logview.Prepend`. |

See `examples/patterns/podlogs`.
