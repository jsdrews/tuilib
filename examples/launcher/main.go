// Launcher: the single entry point for tuilib examples. Shows a menu of
// available demos; enter pushes the selected one onto the app stack, esc
// pops back.
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"

	appfilters "github.com/jsdrews/tuilib/examples/app/filters"
	appfocus "github.com/jsdrews/tuilib/examples/app/focus"
	applayouts "github.com/jsdrews/tuilib/examples/app/layouts"
	appmouse "github.com/jsdrews/tuilib/examples/app/mouse"
	appprescreen "github.com/jsdrews/tuilib/examples/app/prescreen"
	appreplace "github.com/jsdrews/tuilib/examples/app/replace"
	appstack "github.com/jsdrews/tuilib/examples/app/stack"
	appstatus "github.com/jsdrews/tuilib/examples/app/status"
	apptabs "github.com/jsdrews/tuilib/examples/app/tabs"
	dataalert "github.com/jsdrews/tuilib/examples/data/alert"
	dataconfirm "github.com/jsdrews/tuilib/examples/data/confirm"
	datadrilldown "github.com/jsdrews/tuilib/examples/data/drilldown"
	dataform "github.com/jsdrews/tuilib/examples/data/form"
	datainspector "github.com/jsdrews/tuilib/examples/data/inspector"
	datalist "github.com/jsdrews/tuilib/examples/data/list"
	dataloading "github.com/jsdrews/tuilib/examples/data/loading"
	datalogview "github.com/jsdrews/tuilib/examples/data/logview"
	datametrics "github.com/jsdrews/tuilib/examples/data/metrics"
	datapoll "github.com/jsdrews/tuilib/examples/data/poll"
	datapolltable "github.com/jsdrews/tuilib/examples/data/polltable"
	datarunlog "github.com/jsdrews/tuilib/examples/data/runlog"
	datarunner "github.com/jsdrews/tuilib/examples/data/runner"
	datatable "github.com/jsdrews/tuilib/examples/data/table"
	datatextview "github.com/jsdrews/tuilib/examples/data/textview"
	datatree "github.com/jsdrews/tuilib/examples/data/tree"
	paneshowcase "github.com/jsdrews/tuilib/examples/pane/showcase"
	"github.com/jsdrews/tuilib/examples/themecheck"
)

type entry struct {
	name  string
	blurb string
	build func(theme.Theme) screen.Screen
}

var entries = []entry{
	{"Panes — border + title showcase", "Four panes demonstrating border styles, title positions, and slot-bracket variants.", paneshowcase.New},
	{"List — filterable cities (long, for scroll testing)", "A filterable list.Model with 372 numbered rows, so the scrollbar thumb is small and worth dragging. Every row shows its ordinal, so after a drag or a track click you can read whether the jump landed where the thumb said. Wheel scrolls whether or not the pane has focus; double-click opens a row.", datalist.New},
	{"Logview — streaming with search", "A synthetic log stream with /-search, n/N to jump matches, g/G top/bottom, and pause/follow.", datalogview.New},
	{"TextView — static text with search + wrap", "Two documents (README + git diff) that cycle via d. /-search + n/N to jump matches, w to toggle wrap, g/G and ctrl+u/d to scroll. No follow, no MaxLines, no filter mode — the counterpart to logview for read-static-text.", datatextview.New},
	{"Table — filterable + sortable cities", "table.Model with sticky header, three column sizing modes side-by-side (City uses Flex:1 + MaxWidth:28 to absorb leftover space up to a cap — resize the terminal to watch it stretch then stop; Region/Population fixed; Status uses Width:0 for content-auto). [/] to step sort column + s to toggle direction (City + Region sort lexically; Population sorts numerically via a custom Less that parses \"8.3M\"). Status column uses ansi.CellColor so the selected-row bg passes through colored cells. Wiki column wraps each city's Wikipedia URL in ansi.Hyperlink — shift-click in alacritty/tmux/kitty/iTerm2 launches the full URL even when the column is narrow.", datatable.New},
	{"Form — text + select + confirm", "A form.Model with Text, Select, and Confirm fields; each field is its own bordered component, submit replaces with a result pane.", dataform.New},
	{"Loading — spinners while fetching", "List, logview, and tree all start in SetLoading(true); staggered tea.Tick delays simulate fetches that resolve at different times. Tab cycles focus so / and h/l only affect one pane. Press r to refetch.", dataloading.New},
	{"Drilldown — master-detail + push, with async fetches", "Cities list (loads on Init via tea.Tick) + detail list with focus cycling. Enter on either pane \"opens the focused selection\" — left-enter loads the detail (reqID tags drop stale results so hammering enter never races) and shifts focus right; right-enter pushes a child screen describing the attribute. Esc on the child pops back with parent state intact.", datadrilldown.New},
	{"Runner — interactive subprocess", "Pick a command, hand the terminal to it, return on exit. Demonstrates pkg/runner with $EDITOR, less, man, htop. Last entry uses RunWithNotice to print \"connecting…\" during the handoff.", datarunner.New},
	{"Runlog — stream stdout into logview", "Pick a command on the left; its stdout/stderr stream into a logview on the right. Tab cycles focus, x kills the running process.", datarunlog.New},
	{"Tree — searchable expand/collapse", "A synthetic project tree with cursor, expand/collapse (←→/space), search (/), and filter mode (\\) that hides non-matching subtrees. Leaves carry colored status icons (lipgloss-rendered) so the row highlight stays intact across ANSI segments.", datatree.New},
	{"Inspector — structured record viewer", "A two-column label/value viewer for a synthetic k8s-pod payload, fed through inspector.FromAny so the example shows the typical json.Unmarshal → FromMap path. Sibling labels auto-align per group, ▸/▾ expand nested objects/arrays, / searches labels and values, \\ hides non-matching subtrees while keeping ancestors visible.", datainspector.New},
	{"Poll — auto-refresh with keyed cursor", "pkg/poll drives a 2s tick that mutates a synthetic job list (status flips, new jobs appear, finished ones drop, ordering changes); SetKeyedItems keeps the cursor pinned to the same job ID across every refresh. p pauses, r refreshes now, +/- adjust cadence. Title shows \"refreshed Xs ago\".", datapoll.New},
	{"PollTable — deployments table auto-refresh", "pkg/poll + pkg/table SetKeyedRows: a synthetic deployments table whose Sync/Health/Replicas drift each tick and dirty rows float to the top. The cursor stays on the same deployment ID even as ordering changes. p pauses, r refreshes now, +/- adjust cadence; standard filter (env:prod, health:~degraded) and sort ([/]/s) still work.", datapolltable.New},
	{"Metrics — inline Badge/Ratio/Bar/Spark", "pkg/metrics inline primitives composed into a polled deployments table: Ratio for replica counts (\"6/6\" green / \"3/4\" yellow), Badge for pod-state breakdown (\"6✓ 1⚠ 2✗\"), Bar+percent for CPU, Spark for 24s CPU history. Same poll cadence + keyed cursor as PollTable. Demonstrates that metrics primitives compose cleanly into existing components — no new screens, no new layout.", datametrics.New},
	{"Themes — live palette picker", "Cursor re-skins the whole app; enter shows a theme's field palette.", themecheck.New},
	{"Layouts — five layout.Node trees", "One screen per layout primitive: HStack+Fixed/Flex, nested stacks, ZStack modal, …", applayouts.New},
	{"Stack — data flow between screens", "Parent→child via constructor, child→parent via Pop(result) + OnEnter.", appstack.New},
	{"Focus — tab/shift-tab between components", "A screen with input + list + toggle; tab cycles focus, only the active component takes keys.", appfocus.New},
	{"Filters — two filterable panes", "A list and a table, each with its own filter, on one screen. Exercises the focus states a single filterable pane can't reach: exactly one region highlighted at a time, clicking a body taking input back from its filter, switching panes clearing the filter you left, and tab completing a key:value term instead of cycling panes.", appfilters.New},
	{"Mouse — click, double-click, wheel, drag", "Three panes wired for mouse: click to focus, click a row to select, double-click to open, click a table header to sort or a tree ▸ to expand, wheel over any pane, drag a scrollbar. Requires app.Options.Mouse — the launcher sets it.", appmouse.New},
	{"Prescreen — push a screen in front, take a result back", "The \"log in before you can use this\" shape: a root screen pushes a child from OnEnter, receives its result on Pop, and can re-push it later (L logs out) — all without the child living permanently on the stack. The login form is set dressing; the flow is the point. Every field is clickable.", appprescreen.New},
	{"Tabs — three sub-screens behind one strip", "Cities (filterable list) + Logs (streaming logview) + Counter, switched via shift+left/right or 1/2/3. tab/shift+tab is left alone. Each body keeps its own state across switches; logs keep streaming while you're on another tab.", apptabs.New},
	{"Status — info/error messages from a screen", "Pick an action; the screen returns app.Info / app.Error / app.ClearStatus and the shell paints the statusbar's center slot. Auto-clears on any keypress.", appstatus.New},
	{"Confirm — modal yes/no dialog", "Press d on a file to bring up a confirm modal via pkg/confirm; ConfirmedMsg/CancelledMsg flow back as tea.Msg, the parent dismisses and reports the outcome via app.Info.", dataconfirm.New},
	{"Alert — modal acknowledgement dialog", "A list of mock operations; some succeed (statusbar info), some fail with an error-tinted modal alert via pkg/alert. DismissedMsg flows back as tea.Msg. Contrasts the lightweight statusbar pattern with the modal \"stop and acknowledge\" pattern.", dataalert.New},
	{"Replace — atomic top-of-stack swap", "Press r on the city list (filter set, cursor moved) to swap in a fresh instance — depth stays at 1, no flicker. Push a city detail, bump the visit counter, then r to reset the counter without re-firing the parent's OnEnter.", appreplace.New},
}

type rootScreen struct {
	t    theme.Theme
	menu list.Model
	info pane.Pane
}

func newRoot() *rootScreen {
	s := &rootScreen{}
	s.SetTheme(themecheck.Themes()[1]) // start on Dark (index 0 is Terminal())
	return s
}

func (s *rootScreen) Title() string         { return "Examples" }
func (s *rootScreen) Init() tea.Cmd         { return textinput.Blink }
func (s *rootScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *rootScreen) IsCapturingKeys() bool { return s.menu.Filtering() }

func (s *rootScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	prevIdx, prevOK := s.menu.SelectedIndex()
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)

	if curIdx, curOK := s.menu.SelectedIndex(); curIdx != prevIdx || curOK != prevOK {
		s.rebuildInfo()
	}

	// Enter and double-click are the same verb (rule 14), so they resolve to
	// the same branch. A screen has to opt into the mouse half explicitly:
	// the list reports the activation, and what "open" means is the screen's
	// call — here, pushing the selected example.
	// Enter and double-click are the same verb (rule 14); IsActivate folds
	// both into one branch so what "open" means is written once.
	if s.menu.IsActivate(msg) {
		if idx, ok := s.menu.SelectedIndex(); ok && idx >= 0 && idx < len(entries) {
			return s, tea.Batch(cmd, screen.Push(entries[idx].build(s.t)))
		}
	}
	return s, cmd
}

func (s *rootScreen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.menu)),
		layout.Flex(3, layout.Sized(&s.info)),
	)
}

func (s *rootScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func (s *rootScreen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.menu.Cursor(), s.menu.Value()
	opts := t.List()
	opts.Title = "examples"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter…"
	opts.Items = entryNames()
	s.menu = list.New(opts)
	if value != "" {
		s.menu.SetValue(value)
	}
	s.menu.SetCursor(cursor)

	s.rebuildInfo()
}

func (s *rootScreen) rebuildInfo() {
	s.info = pane.New(s.t.Pane())
	s.info.SetTitle("about")
	idx, ok := s.menu.SelectedIndex()
	if !ok || idx < 0 || idx >= len(entries) {
		s.info.SetContent("Pick an example on the left and press enter.")
		return
	}
	e := entries[idx]
	s.info.SetContent(e.name + "\n\n" + e.blurb)
}

func entryNames() []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.name
	}
	return out
}

func main() {
	m := app.New(app.Options{
		Root:        newRoot(),
		Themes:      themecheck.Themes(),
		ThemeEnvVar: "TUILIB_THEME",
		Version:     "examples",
		ThemeKey:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		// Mouse is opt-in per app, and app.Options is the only place it is
		// configured — the shell enables reporting from Init, so
		// tea.NewProgram below needs no mouse option of its own. Turning it
		// on here makes every example in the suite mouse-capable, which is
		// the point: anything that behaves badly with a pointer shows up
		// immediately rather than only in the mouse demo.
		Mouse: app.MouseClick,
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
