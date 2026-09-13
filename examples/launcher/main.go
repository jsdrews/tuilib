// Launcher: the single entry point for tuilib examples. Shows a menu of
// available demos; enter pushes the selected one onto the app stack, esc
// pops back.
//
// Entries are grouped the way the directories are, and for the same reason —
// so the name tells you what kind of thing you are about to open:
//
//	Components  one per UI component, in isolation.
//	Shell       what pkg/app owns: navigation, chrome, statusbar, console.
//	Patterns    composition idioms — focus, mouse, polling, paging, verbs.
//
// Filtering searches the group as well as the name, so "/patterns" narrows
// to one section.
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"

	compform "github.com/jsdrews/tuilib/examples/components/form"
	compinspector "github.com/jsdrews/tuilib/examples/components/inspector"
	complist "github.com/jsdrews/tuilib/examples/components/list"
	complogview "github.com/jsdrews/tuilib/examples/components/logview"
	compmetrics "github.com/jsdrews/tuilib/examples/components/metrics"
	comppane "github.com/jsdrews/tuilib/examples/components/pane"
	comptable "github.com/jsdrews/tuilib/examples/components/table"
	comptextview "github.com/jsdrews/tuilib/examples/components/textview"
	comptree "github.com/jsdrews/tuilib/examples/components/tree"
	patactions "github.com/jsdrews/tuilib/examples/patterns/actions"
	patactivity "github.com/jsdrews/tuilib/examples/patterns/activity"
	patcapture "github.com/jsdrews/tuilib/examples/patterns/capture"
	patdrilldown "github.com/jsdrews/tuilib/examples/patterns/drilldown"
	patfilters "github.com/jsdrews/tuilib/examples/patterns/filters"
	patfocus "github.com/jsdrews/tuilib/examples/patterns/focus"
	patloading "github.com/jsdrews/tuilib/examples/patterns/loading"
	patmodals "github.com/jsdrews/tuilib/examples/patterns/modals"
	patmouse "github.com/jsdrews/tuilib/examples/patterns/mouse"
	patmultiselect "github.com/jsdrews/tuilib/examples/patterns/multiselect"
	patpoll "github.com/jsdrews/tuilib/examples/patterns/poll"
	patremote "github.com/jsdrews/tuilib/examples/patterns/remote"
	patrunner "github.com/jsdrews/tuilib/examples/patterns/runner"
	pattreeactions "github.com/jsdrews/tuilib/examples/patterns/treeactions"
	shellchrome "github.com/jsdrews/tuilib/examples/shell/chrome"
	shelllayouts "github.com/jsdrews/tuilib/examples/shell/layouts"
	shelloutput "github.com/jsdrews/tuilib/examples/shell/output"
	shellprescreen "github.com/jsdrews/tuilib/examples/shell/prescreen"
	shellstack "github.com/jsdrews/tuilib/examples/shell/stack"
	shellstatus "github.com/jsdrews/tuilib/examples/shell/status"
	shelltabs "github.com/jsdrews/tuilib/examples/shell/tabs"
	shellthemes "github.com/jsdrews/tuilib/examples/shell/themes"
)

// Group names. Kept as constants because the menu label, the filter text and
// the column padding all derive from them.
const (
	groupComponents = "Components"
	groupShell      = "Shell"
	groupPatterns   = "Patterns"
)

type entry struct {
	group string
	name  string
	// dir is the package directory under examples/, shown in the about pane
	// so the answer to "where is this code" is on screen rather than guessed
	// from the name.
	dir   string
	blurb string
	build func(theme.Theme) screen.Screen
}

var entries = []entry{
	// ---- Components: one per component, in isolation --------------------
	{groupComponents, "Pane", "components/pane",
		"Four panes demonstrating border styles, title positions, and slot-bracket variants. Every interactive component owns one of these internally, which is why none of them need a wrapper.",
		comppane.New},
	{groupComponents, "List", "components/list",
		"A filterable list.Model with 372 numbered rows, so the scrollbar thumb is small and worth dragging. Every row shows its ordinal, so after a drag or a track click you can read whether the jump landed where the thumb said. Wheel scrolls whether or not the pane has focus; double-click opens a row.",
		complist.New},
	{groupComponents, "Table", "components/table",
		"table.Model with sticky header and three column sizing modes side-by-side (City uses Flex:1 + MaxWidth:28 to absorb leftover space up to a cap — resize the terminal to watch it stretch then stop; Region/Population fixed; Status uses Width:0 for content-auto). [/] steps the sort column and s toggles direction (Population sorts numerically via a custom Less that parses \"8.3M\"). The Status column uses ansi.CellColor so the selected-row background passes through colored cells; the Wiki column wraps each URL in ansi.Hyperlink, so shift-click launches the full link even when the column truncates it.",
		comptable.New},
	{groupComponents, "Tree", "components/tree",
		"A synthetic project tree with cursor, expand/collapse (space), search (/), and filter mode (\\) that hides non-matching subtrees while keeping ancestors. Arrows and hjkl stay on scroll, library-wide. Leaves carry colored status icons so the row highlight survives ANSI segments.",
		comptree.New},
	{groupComponents, "Inspector", "components/inspector",
		"A two-column label/value viewer for a synthetic k8s-pod payload, fed through inspector.FromAny so the example shows the typical json.Unmarshal → FromMap path. Sibling labels auto-align per group, ▸/▾ expand nested objects and arrays, / searches labels and values, \\ hides non-matching subtrees.",
		compinspector.New},
	{groupComponents, "TextView", "components/textview",
		"Two documents (README + git diff) that cycle via d. /-search with n/N to step matches, w to toggle wrap, g/G and ctrl+u/d to scroll. No follow, no MaxLines, no filter mode — the read-static-text counterpart to logview.",
		comptextview.New},
	{groupComponents, "LogView", "components/logview",
		"A synthetic log stream with /-search, n/N to jump matches, g/G top/bottom, and pause/follow. The streaming counterpart to textview, with a MaxLines cap so an open-ended producer can't grow memory without limit.",
		complogview.New},
	{groupComponents, "Form", "components/form",
		"A form.Model with Text, Select, and Confirm fields; each field is its own bordered component and validation lives in the form rather than the screen. Submit replaces the form with a result pane.",
		compform.New},
	{groupComponents, "Metrics", "components/metrics",
		"pkg/metrics inline primitives composed into a live deployments table: Ratio for replica counts (\"6/6\" green, \"3/4\" yellow), Badge for pod-state breakdown (\"6✓ 1⚠ 2✗\"), Bar+percent for CPU, Spark for CPU history. Everything renders as a table cell — the point is that the primitives drop into existing components with no new screens and no new layout. Polls every 2s with a keyed cursor; p pauses, r refreshes, +/- adjust cadence.",
		compmetrics.New},

	// ---- Shell: what pkg/app owns ---------------------------------------
	{groupShell, "Layouts", "shell/layouts",
		"One sub-screen per layout primitive: HStack+Fixed/Flex, nested stacks, ZStack modal, and more. The answer to every \"how do I leave room for a bar\" question that would otherwise be written as m.h - 2.",
		shelllayouts.New},
	{groupShell, "Stack", "shell/stack",
		"Navigation and the three directions data moves: parent→child through the constructor, child→parent through Pop(result) landing in OnEnter, and self→self through Replace. r on either screen swaps it for a fresh instance — the filter and the visit counter reset, depth stays put, and the screen underneath is never reactivated. The city detail's visit counter is there to make that last part visible.",
		shellstack.New},
	{groupShell, "Tabs", "shell/tabs",
		"Cities (filterable list) + Logs (streaming logview) + Counter behind one strip, switched via shift+left/right, 1/2/3, or a click on a label. tab/shift+tab is left alone for inner focus cycling. Each body keeps its own state, and the logs keep streaming while you are on another tab — keys go to the active body, everything else fans out to all of them.",
		shelltabs.New},
	{groupShell, "Status", "shell/status",
		"Pick an action; the screen returns app.Info / app.Error / app.ClearStatus and the shell paints the statusbar's center slot. Auto-clears on the next keypress, which is why anything worth reading twice belongs in the console instead.",
		shellstatus.New},
	{groupShell, "Output", "shell/output",
		"The app-wide console. Actions that emit app.Info / app.ErrorDetail / app.ErrorOf, plus runner.Capture streaming a live subprocess. The statusbar shows a sliver and wipes it on the next keypress; o opens the console where all of it is still there, with c to clear, x to kill a running capture, and w to export. Note the OnEnter guard on app.OutputClosed — closing the console must not look like a fresh activation.",
		shelloutput.New},
	{groupShell, "Themes", "shell/themes",
		"Live palette picker: moving the cursor re-skins the whole app, and enter shows a theme's field palette. Also the package every other example's theme list comes from.",
		shellthemes.New},
	{groupShell, "Chrome", "shell/chrome",
		"The two border-shape slots a Theme carries: one for ordinary components, one for overlays. Pick a shape on the left and every component on screen — the pickers included — is rebuilt from the modified theme, the same path any app gets from th.List() / th.Input() / th.Confirm(). Set both pickers to the same shape to see why the overlay slot is separate: the modal flattens into the content behind it.",
		shellchrome.New},
	{groupShell, "Prescreen", "shell/prescreen",
		"The \"log in before you can use this\" shape: a root screen pushes a child from OnEnter, receives its result on Pop, and can re-push it later (L logs out) — all without the child living permanently on the stack. The login form is set dressing; the flow is the point. Every field is clickable.",
		shellprescreen.New},

	// ---- Patterns: composition idioms -----------------------------------
	{groupPatterns, "Focus", "patterns/focus",
		"A screen with input + list + toggle behind one focus.Group: tab cycles, only the focused component takes keys, and a clicked component gets focus because the Group sees the request it sends. Esc leaves the input — without it the field captures every key and tab can never cycle off it.",
		patfocus.New},
	{groupPatterns, "Filters", "patterns/filters",
		"A list and a table, each with its own filter, on one screen. Exercises the focus states a single filterable pane can't reach: exactly one region highlighted at a time, clicking a body taking input back from its filter, switching panes clearing the filter you left, and tab completing a key:value term instead of cycling panes.",
		patfilters.New},
	{groupPatterns, "Mouse", "patterns/mouse",
		"Three panes wired for mouse: click to focus, click a row to select, double-click to open, click a table header to sort or a tree ▸ to expand, wheel over any pane whether or not it has focus, drag a scrollbar. Every rect comes from pkg/layout — nothing is injected into the rendered string. Requires app.Options.Mouse, which the launcher sets for the whole suite.",
		patmouse.New},
	{groupPatterns, "Loading", "patterns/loading",
		"List, logview, and tree all start in SetLoading(true); staggered tea.Tick delays simulate fetches that resolve at different times. Tab cycles focus so / and h/l only affect one pane. Press r to refetch and watch the spinner replace the previous result rather than overlay it.",
		patloading.New},
	{groupPatterns, "Drilldown", "patterns/drilldown",
		"Master-detail with async fetches at every level. Cities list loads on Init; enter on either pane \"opens the focused selection\" — left-enter loads the detail (reqID tags drop stale results, so hammering enter never races) and shifts focus right, right-enter pushes a child screen describing the attribute. Esc on the child pops back with parent state intact.",
		patdrilldown.New},
	{groupPatterns, "Poll", "patterns/poll",
		"pkg/poll drives a 2s tick that mutates a synthetic job list (statuses flip, new jobs appear, finished ones drop, ordering changes); SetKeyedItems keeps the cursor pinned to the same job ID across every refresh. p pauses, r refreshes now, +/- adjust cadence, and the title shows \"refreshed Xs ago\".",
		patpoll.New},
	{groupPatterns, "Remote", "patterns/remote",
		"The whole windowed-source loop: pkg/source coordinating a pkg/table in FilterRemote/SortRemote over a simulated 5,000-row API that answers one 100-row page at a time with 250ms of latency. Scroll faster than it answers and you see the \"·\" placeholders for rows it hasn't received; the cursor stays put and the data arrives under it. / filters at the source (enter commits — one request, not one per keystroke), [/]/s sorts there too, r refetches the current window. Completion candidates come from SetDistinct, since one page can't know every region.",
		patremote.New},
	{groupPatterns, "Actions", "patterns/actions",
		"The verb menu. Press a or right-click a row to open action.Menu: it sizes itself to its widest row and anchors where you asked. This screen is single-target on purpose — Multi-select is the one that marks rows. Start a Restart and reopen the menu to see the Exclusive gate. Single click commits, click-away dismisses, Delete confirms first.",
		patactions.New},
	{groupPatterns, "Activity", "patterns/activity",
		"Per-row in-flight state, from all three directions at once. Press a and pick Sync: the row spins before any request is answered, because the screen put its Selection() into action.Set.Targets and the shell broadcast the verb's Busy label against it — then hands over to the server's own \"Syncing\" as the poll catches up. Meanwhile scheduled syncs fire with nobody pressing anything and spin because ActivityWhen recognises the status in the polled data. Refresh is the case worth watching: it finishes server-side long before the next poll, and the row keeps moving until that poll lands, because the indicator covers the gap between asking and being told rather than the work. Operations that begin and end between two polls flash • instead, off a hidden revision column.",
		patactivity.New},
	{groupPatterns, "Multi-select", "patterns/multiselect",
		"table.Model with Options.Markable: x marks the cursor row, X (or shift+click) extends the selection between the anchor and the cursor in either direction, A marks everything the filter shows, D drops the selection, and clicking the ✓ gutter toggles without opening the row. Marks are held by Key (SetKeyedRows), so they survive filtering — mark a row, filter it away, and the count on the border does not move — and survive a theme swap via SetMarks. Press a or right-click for the menu: it is titled with what it will act on (\"3 items\"), and Describe, which did not declare Multi, dims itself with a reason as soon as a second row is marked.",
		patmultiselect.New},
	{groupPatterns, "Tree actions", "patterns/treeactions",
		"pkg/tree marking + pkg/action on a cluster-shaped hierarchy. x marks a node at any depth, X a range, A everything visible, D drops. Marking a branch marks that branch alone — the screen resolves it to the pods a verb should touch with a prefix test on the path, which is why Restart cascades into a marked namespace and Describe does not. Marks survive collapsing, so the menu title counts what it will actually touch (\"Restart 9 pods\") even when some are folded out of sight. Delete is guarded to staging.",
		pattreeactions.New},
	{groupPatterns, "Modals", "patterns/modals",
		"The three weights of feedback on one list of operations: app.Info for a success (passive, wiped by the next keypress), pkg/confirm for a destructive op (a gate, with the cancel side starting highlighted), and pkg/alert for a failure the user has to acknowledge. \"Force push main\" goes through both modals in sequence. Note the two hosting shapes — the confirm modal is fixed-size inside layout.Center, while the autosizing alert centers itself and needs no wrapper.",
		patmodals.New},
	{groupPatterns, "Runner", "patterns/runner",
		"Handing the terminal to an interactive subprocess: pick a command, the TUI suspends, and it resumes when the command exits. $EDITOR, less, man, htop — anything that wants a real TTY. The last entry uses RunWithNotice to print \"connecting…\" during the handoff. The counterpart to Capture, which never gives the terminal away.",
		patrunner.New},
	{groupPatterns, "Capture", "patterns/capture",
		"Streaming a subprocess into an on-screen logview with runner.Capture, while the TUI stays live. The shell chains the reads and forwards every message, so the screen just matches CaptureStarted / CapturedLine / Captured — no io.Pipe, no goroutine, no scanner. x kills the run, and o shows the same stream in the app-wide console with its exit status.",
		patcapture.New},
}

type rootScreen struct {
	t    theme.Theme
	menu list.Model
	// info is a textview rather than a bare pane because the blurbs are
	// prose: textview wraps to the inner width on every SetRect, which is
	// the one case rule 18 says to wrap rather than truncate.
	info textview.Model
}

func newRoot() *rootScreen {
	s := &rootScreen{}
	s.SetTheme(shellthemes.Themes()[1]) // start on Dark (index 0 is Terminal())
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

	// Enter and double-click are the same verb (rule 14); IsActivate folds
	// both into one branch so what "open" means is written once.
	if s.menu.IsActivate(msg) {
		if idx, ok := s.menu.SelectedIndex(); ok && idx >= 0 && idx < len(entries) {
			return s, tea.Batch(cmd, screen.Push(entries[idx].build(s.t)))
		}
	}

	// Keys belong to the menu; the mouse goes to both, so the wheel scrolls
	// whichever pane it is over (rules 6 and 25).
	if _, isMouse := msg.(mouse.Msg); isMouse {
		var icmd tea.Cmd
		s.info, icmd = s.info.Update(msg)
		return s, tea.Batch(cmd, icmd)
	}
	return s, cmd
}

func (s *rootScreen) Layout() layout.Node {
	return layout.HStack(
		layout.Flex(2, layout.Sized(&s.menu)),
		layout.Flex(3, layout.Sized(&s.info)),
	)
}

func (s *rootScreen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *rootScreen) HelpSections() []help.Section {
	return help.SectionsOf(&s.menu, help.Group("Launcher",
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	))
}

func (s *rootScreen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.menu.Cursor(), s.menu.Value()
	opts := t.List()
	opts.Title = fmt.Sprintf("examples · %d", len(entries))
	opts.Filterable = true
	opts.Filter.Placeholder = "filter by group or name…"
	opts.Items = entryNames()
	s.menu = list.New(opts)
	if value != "" {
		s.menu.SetValue(value)
	}
	s.menu.SetCursor(cursor)

	s.rebuildInfo()
}

func (s *rootScreen) rebuildInfo() {
	opts := s.t.TextView()
	opts.Wrap = true
	s.info = textview.New(opts)

	idx, ok := s.menu.SelectedIndex()
	if !ok || idx < 0 || idx >= len(entries) {
		s.info.SetTitle("about")
		s.info.SetContent("Pick an example on the left and press enter.\n\n" +
			groupComponents + " — one demo per UI component, in isolation.\n\n" +
			groupShell + " — navigation, chrome, statusbar and console: what pkg/app owns.\n\n" +
			groupPatterns + " — composition idioms: focus, mouse, polling, paging, verbs.")
		return
	}
	e := entries[idx]
	s.info.SetTitle("examples/" + e.dir)
	s.info.SetContent(e.group + " · " + e.name + "\n\n" + e.blurb)
}

// entryNames renders the menu labels, padded so the names line up in a
// column and the group reads as a heading down the left edge.
func entryNames() []string {
	width := 0
	for _, e := range entries {
		if n := len(e.group); n > width {
			width = n
		}
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = fmt.Sprintf("%-*s · %s", width, e.group, e.name)
	}
	return out
}

func main() {
	m := app.New(app.Options{
		Root:        newRoot(),
		Themes:      shellthemes.Themes(),
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

		// Actions are opt-in per app, exactly as the output console is: the
		// shell claiming a letter takes it from every component downstream.
		// "a" is the conventional choice and is otherwise unclaimed.
		ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		// The console is opt-in, and this is the line that spends the key:
		// once the shell claims "o" it is claimed for every screen in the
		// app, which is why the library ships no default for it.
		OutputKey: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output")),
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
