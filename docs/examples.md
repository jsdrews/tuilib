# Examples

The repository ships a launcher hosting every demo as a child screen:

```sh
git clone https://github.com/jsdrews/tuilib.git
cd tuilib
task examples            # or: go run ./examples/launcher
```

Select a showcase and press `enter` to open it; `esc` returns to the menu.
Each one's about pane names the directory its code lives in. The launcher
turns on mouse support, the output console (`o`) and the action menu (`a`)
for the whole suite.

`task demo` does the same against a local demo API server (an HTTP service in
`demoapi/` shaped like the APIs the showcases model), so Live ops and Remote
data run over real HTTP. See the [demo API design note](demoapi.md).

`examples/quickstart` is the standalone program from the
[Quick start](quickstart.md): `go run ./examples/quickstart`.

Each showcase puts many features on one screen, cooperating, so a change that
breaks an older feature shows up where you can see it. Each is a package at
`examples/<name>/` exporting `New(theme.Theme) screen.Screen`; the tabbed ones
keep each tab in a sub-package, which is the place to copy from when starting
a new screen.

| Showcase | What it puts together |
|---|---|
| Workbench | Every row component over one cluster, in one `focus.Group`: a filterable, sortable, markable `table`; a searchable, markable `tree`; a markable `list` following the table; an `inspector` following whichever pane last had focus; a grouped `menu`. The action menu (`a`, right-click) offers verbs for the focused pane's selection; the operations menu runs statusbar notices, `pkg/confirm` and `pkg/alert`; the mouse does everything |
| Live ops | Data that changes while you watch: row activity against demoapi's Argo-shaped API with one verb per operation shape, `pkg/metrics` cells (`Ratio`, `Badge`, `Bar`, `Spark`), an events log, one `pkg/poll` driving all three panes, and every pane loading until its first data |
| Remote data | Data too large for one request, one `pkg/tab` tab per shape: a paged table where scrolling is the pagination, an AWX job's events (running and finished), an Elasticsearch-style index read with `search_after`, and a pod log with exact reconnects |
| Processes | Commands three ways: `runner.Capture` streaming into a searchable logview while the TUI stays live, `runner.Run` handing the terminal to an editor or pager, and statusbar notices the output console (`o`) keeps — with the `OnEnter` guard on `app.OutputClosed` |
| Navigation | A grouped, filterable `menu` home that opens a screen stack (push, pop with a result, replace), a screen pushed in front of another, a master-detail drilldown, and a tab host; the breadcrumb names every level and a click on a crumb unwinds to it |
| Appearance | Palette, surface and glyph pickers that redraw the whole app live, beside what each kind of component is handed; then tabs for a titled form, a textview, pane variants, and one screen per layout primitive |
