# Examples

The repository ships a launcher hosting every demo as a child screen:

```sh
git clone https://github.com/jsdrews/tuilib.git
cd tuilib
task examples            # or: go run ./examples/launcher
```

Select a demo and press `enter` to open it; `esc` returns to the menu. Each
demo's about pane names the directory its code lives in. Filtering the menu
matches the group as well as the name, so `/patterns` narrows to one section.
The launcher turns on mouse support, the output console (`o`) and the action
menu (`a`) for the whole suite.

`task demo` does the same against a local demo API server (an HTTP service in
`demoapi/` shaped like the APIs the demos model), so the remote, eventlog and
activity demos run over real HTTP. See the [demo API design note](demoapi.md).

`examples/quickstart` is the standalone program from the
[Quick start](quickstart.md): `go run ./examples/quickstart`.

Every demo is a package at `examples/<area>/<name>/` exporting
`New(theme.Theme) screen.Screen`. To start a new screen, copy the closest one
and delete what you don't need.

## `examples/components/` — one per component, in isolation

| Entry | Demonstrates |
|---|---|
| Pane | Border styles, title positions, and slot-bracket variants in one 2×2 grid. Every interactive component owns one of these internally, which is why none of them need a wrapper |
| List | A filterable `list.Model` with 372 numbered rows, so every scroll affordance is reachable: drag the thumb, click the track, wheel over an unfocused pane, `g`/`G`, `ctrl+u/d`, double-click to open |
| Table | `table.Model` with a sticky header and all three column sizing modes side by side (City `Flex:1` + `MaxWidth:28`, Region/Population fixed, Status content-auto). `[`/`]` step the sort column, `s` toggles direction, Population sorts numerically via a custom `Less` that parses "8.3M". Status uses `ansi.CellColor` so the selected-row background passes through colored cells; Wiki wraps each URL in `ansi.Hyperlink` so shift-click launches the full link even when the column truncates it |
| Tree | Synthetic project tree with cursor, expand/collapse (`space`), `/`-search, and `\` filter mode that hides non-matching subtrees while keeping ancestors. Leaves carry colored status icons so the row highlight survives ANSI segments |
| Inspector | Two-column label/value viewer over a synthetic k8s-pod payload, fed through `inspector.FromAny` — the typical `json.Unmarshal` → `FromMap` path. Sibling labels auto-align per group; `/` searches labels and values |
| TextView | Two documents (README + git diff) cycled with `d`. `/`-search with `n`/`N`, `w` toggles wrap, `g`/`G` and `ctrl+u/d` scroll. No follow, no `MaxLines` — the read-static-text counterpart to logview |
| LogView | Streaming log tail with `/`-search, `n`/`N` jump, `\` filter-mode toggle, current-line highlight, and a `MaxLines` cap so an open-ended producer can't grow memory without limit |
| Form | `form.Model` with Text / Select / Confirm fields + submit. Validation lives in the form, not the screen: errors render on the field's own border so nothing reflows while you correct it |
| Metrics | `pkg/metrics` inline primitives in a live deployments table — `Ratio` for replica counts, `Badge` for pod-state breakdown, `Bar`+percent for CPU, `Spark` for CPU history. The point is that they drop into existing components as ordinary cells: no new screens, no custom layout. Polls every 2s with a keyed cursor |

## `examples/shell/` — what `pkg/app` owns

| Entry | Demonstrates |
|---|---|
| Layouts | One sub-screen per layout primitive: `HStack`+`Fixed`/`Flex`, nested stacks, `ZStack` modal, and more — the answer to every "how do I leave room for a bar" question that would otherwise be written as `m.h - 2` |
| Stack | Navigation plus all three directions data moves: parent→child through the constructor, child→parent through `Pop(result)` landing in `OnEnter`, and self→self through `Replace`. `r` on either screen swaps it for a fresh instance — filter and visit counter reset, depth stays put, and the screen underneath is never reactivated |
| Tabs | Three sub-screens (filterable list / streaming logview / counter) behind one strip; switch via shift+arrows, `1`/`2`/`3`, or a click on a label. Each body keeps its own state, and the logs keep streaming while you are on another tab |
| Status | Screens emit `app.Info` / `app.Error` / `app.ClearStatus` as `tea.Cmd`s and the shell paints the statusbar's center slot. Auto-clears on the next keypress, which is why anything worth reading twice belongs in the console |
| Output | The app-wide console: `app.Info` / `app.ErrorDetail` / `app.ErrorOf`, plus `runner.Capture` streaming a live subprocess. `o` opens it, `c` clears, `x` kills a running capture, `w` exports. Note the `OnEnter` guard on `app.OutputClosed` — closing the console must not look like a fresh activation |
| Themes | Live palette picker — moving the cursor re-skins the whole app, enter shows a theme's field palette. Also the package every other example's theme list comes from |
| Chrome | The two border-shape slots a `Theme` carries, one for ordinary components and one for overlays. Pick a shape and every component on screen is rebuilt from the modified theme. Set both pickers the same to see why the overlay slot is separate: the modal flattens into the content behind it |
| Prescreen | The "log in before you can use this" shape — a root screen pushes a child from `OnEnter`, takes its result on `Pop`, and can re-push it later (`L` logs out) without the child living permanently on the stack |

## `examples/patterns/` — composition idioms

| Entry | Demonstrates |
|---|---|
| Focus | `input` + `list` + `toggle` behind one `focus.Group`: tab cycles, only the focused component takes keys, and a clicked component gets focus because the Group sees the request it sends |
| Filters | A list and a table, each with its own filter, on one screen — the focus states a single filterable pane can't reach: one region highlighted at a time, clicking a body taking input back from its filter, switching panes clearing the filter you left, `tab` completing a `key:value` term instead of cycling panes |
| Mouse | Click to focus, click a row to select, double-click to open, click a table header to sort or a tree `▸` to expand, wheel over any pane focused or not, drag a scrollbar. Every rect comes from `pkg/layout` — nothing is injected into the rendered string |
| Loading | `list`, `logview`, and `tree` all start in `SetLoading(true)`; staggered `tea.Tick` delays simulate fetches that resolve at different times. `r` refetches, and the spinner replaces the previous result rather than overlaying it |
| Drilldown | Master-detail with async fetches at every level. Enter on either pane "opens the focused selection": left-enter loads the detail (reqID-tagged so stale results drop) and shifts focus right, right-enter pushes a child screen |
| Poll | `pkg/poll` drives a 2s tick that mutates a synthetic job list; `SetKeyedItems` keeps the cursor on the same job ID across every refresh even as statuses flip and the list reorders. `p` pauses, `r` refreshes now, `+`/`-` adjust cadence |
| Remote | The whole windowed-source loop: `pkg/source` coordinating a table in `FilterRemote`/`SortRemote` over a simulated 5,000-row API that answers one 100-row page at a time, with latency `L` cycles from 250ms to 8s. Scroll faster than it answers and you see the `·` placeholders; the cursor stays put and data arrives under it. Commit a filter or sort and the rows stay on screen, dimmed, with the border naming the query they answer and the one loading; `[`/`]`/`s` wait for a quiet moment before asking, superseded requests are cancelled, and every query's outcome lands in the output console |
| Eventlog | `pkg/eventlog` bound with `pkg/remote` to demoapi's AWX-shaped job-events endpoints over HTTP — a running job: counter ranges, holes from late-saved events that fill in, follow with a new-events count, `f` to filter on the server (`failed`, `host:web-1`), `/` + `n`/`N` to find past what is loaded, enter to open an event, `L` to cycle latency |
| Eventlog (finished job) | The same over a finished AWX job of 8,000 events — the troubleshooting case: opens at the first event, nothing polls, and `/` + `n`/`N` find matches across the whole job, asking AWX for the next one past what is loaded |
| Anchored | `pkg/eventlog` bound with `pkg/remote` to demoapi's Elasticsearch-shaped `_search` over HTTP — `search_after` with the `_shard_doc` tiebreaker, a span grown at whichever edge you scroll toward, a tail that rewinds to catch late-ingested documents, and a search hit past the span that re-anchors around it |
| Pod logs | A fake pod log in `pkg/logview` with `pkg/resume`: exact reconnects at one-second resolution, marked reconnects and restarts, `O` to load older lines from a longer tail, `P` for the previous run |
| Actions | The verb menu. `a` or right-click opens `action.Menu`, sized to its widest row and anchored where you asked. Single-target on purpose. Start a Restart and reopen the menu to see the `Exclusive` gate; Delete confirms first |
| Multi-select | `table.Model` with `Options.Markable` + `pkg/action`: `x` marks, `X` (or shift+click) extends a range in either direction, `A` marks everything the filter shows, `D` drops. Marks are held by Key, so they survive filtering and a theme swap. The menu is titled with what it will act on ("3 items"), and the action that did not declare `Multi` dims itself with a reason |
| Tree actions | `pkg/tree` marking + `pkg/action` on a cluster hierarchy. Marking a branch marks that branch alone; the screen resolves it to the pods a verb should touch with a prefix test on the path, which is why Restart cascades into a marked namespace and Describe does not |
| Modals | The three weights of feedback on one list of operations: `app.Info` for a success, `pkg/confirm` for a destructive op, `pkg/alert` for a failure the user must acknowledge. "Force push main" goes through both modals in sequence. Note the two hosting shapes — fixed-size inside `layout.Center` vs. an autosizing alert that centers itself |
| Runner | Handing the terminal to an interactive subprocess: the TUI suspends, `$EDITOR` / `less` / `man` / `htop` gets the real TTY, and it resumes on exit. The last entry uses `RunWithNotice` to print "connecting…" during the handoff |
| Capture | The counterpart: `runner.Capture` streams a subprocess into an on-screen logview while the TUI stays live. The shell chains the reads and forwards every message, so the screen just matches `CaptureStarted` / `CapturedLine` / `Captured` — no `io.Pipe`, no goroutine, no scanner |
