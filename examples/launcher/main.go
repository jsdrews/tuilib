// Launcher: the single entry point for tuilib's examples. It lists the
// showcases — screens that each put many features together, so a change that
// breaks an older feature shows up where you can see it — and enter pushes the
// one highlighted onto the app stack; esc pops back.
//
// The list is a pkg/menu (rule 36): a set of places to go. Its items name no
// groups, so it draws flat.
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
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"

	"github.com/jsdrews/tuilib/examples/appearance"
	"github.com/jsdrews/tuilib/examples/liveops"
	"github.com/jsdrews/tuilib/examples/navigation"
	"github.com/jsdrews/tuilib/examples/processes"
	"github.com/jsdrews/tuilib/examples/remotedata"
	"github.com/jsdrews/tuilib/examples/workbench"
)

type entry struct {
	name string
	// tagline is the menu's description column: what the showcase is about,
	// in a few words.
	tagline string
	// dir is the package directory under examples/, shown in the about pane
	// so the answer to "where is this code" is on screen rather than guessed
	// from the name.
	dir   string
	blurb string
	build func(theme.Theme) screen.Screen
}

var entries = []entry{
	{"Workbench", "row components", "workbench",
		"Every row component on one screen over one cluster: a filterable, sortable, markable table of deployments; a searchable, markable tree of namespaces, workloads and pods; the table's pods in a markable list; a filterable inspector following whichever of those last had focus; and a grouped operations menu. tab cycles the five panes and a click focuses one. Enter on a deployment moves to its pods, enter on a pod to the detail. x / X / A / D mark, and a or right-click opens verbs for the focused pane's selection — Describe dims on a second mark, Delete confirms, a marked tree branch cascades to its pods. The operations menu runs statusbar notices, the confirm modal and the alert modal. Swap the theme or surface with anything marked, filtered or open, and it all comes back.",
		workbench.New},
	{"Live ops", "live data", "liveops",
		"Everything that changes while you watch, on one screen. Applications come from demoapi over HTTP, shaped like Argo CD: rows spin because the server says so (it schedules syncs of its own), and a or right-click offers one verb per operation shape — Sync (the phase says when it ends), Refresh (a blocking GET only the request can end), Sync and follow (a streamed job), Quick sync (over between polls, so it is reported), Fail a sync, and Sync (dropped), a job accepted and never run. Mark rows with x and one verb drives them all. Beside it, workloads draw pkg/metrics' Ratio, Badge, Bar and Spark, and below, events logs what each poll changed. One poll drives all three panes; its verbs (pause, refresh now, faster, slower, reload) are on the workloads and events panes, and every pane shows its spinner until its first data lands.",
		liveops.New},
	{"Remote data", "paged and streamed", "remotedata",
		"Data too large for one request, one tab per shape, each read from demoapi over HTTP the way the real API is read. Records: a table over a paged API, where scrolling is the pagination — filter it (region:eu-west), sort it, and watch windows arrive under you. Events: an AWX job's events still arriving, followed until you scroll away, with an inspector one enter away. Finished job: the same once it has ended. Anchored: an Elasticsearch-style index read with search_after, no offsets and no total; a search past what is loaded re-anchors on the hit. Pod logs: a Kubernetes pod's log, a tail then a follow, with reconnects kept exact. shift+←/→ or 1–5 switch tabs, and a tab you are not looking at keeps following its stream.",
		remotedata.New},
	{"Processes", "commands, console", "processes",
		"Running commands, three ways, from one grouped menu. capture streams a command into the output pane with runner.Capture while the TUI stays live — scroll, search, tab away — and the same stream feeds the console (o) with its exit status; Kill is on the action menu (a) while one runs. The synthetic tail follows a log on a tick. terminal hands the real terminal to an editor, a pager or htop with runner.Run and shows the result when you come back. console says something to the statusbar — Info, Error, a 12-line body, a %w chain — and the console keeps all of it after the bar wipes it. The about pane counts activations, which closing the console must not move.",
		processes.New},
	{"Navigation", "screens and stacks", "navigation",
		"How an app moves between screens, from one grouped, filterable menu — the home screen of an app with too many sections for tabs. The navigation group opens a screen stack (push with data, pop with a result, replace a screen with a fresh one), a screen that asks you to sign in first, a master-detail drilldown with a third level, and a tabbed host. The other groups are the menu's own subject: headers the cursor skips, tab / shift+tab or ] / [ jumping between groups and wrapping, / filtering on the label and the group (try network or instance). The breadcrumb names every level; click a crumb to go back to it.",
		navigation.New},
	{"Appearance", "how it is drawn", "appearance",
		"Palettes, surfaces and glyphs, and the components whose subject is how they are drawn. Look: palette, surface and glyph pickers that redraw the whole app as the cursor moves — any palette in any surface — beside what each kind of component is handed: a list the surface, an input and a toggle its field form, a modal its overlay form, and one pane pinned to Outlined. Enter on a palette shows every field with a swatch. Form: a titled form with validation on submit; on a filled surface its fields are cards a step lighter than the frame and its buttons are chips. Text: a document to read, search (/, n, N) and wrap (w). Panes: border styles, title positions and slot brackets. Layouts: one screen per layout primitive. shift+←/→ or 1–5 switch tabs.",
		appearance.New},
}

type rootScreen struct {
	t    theme.Theme
	menu menu.Model
	// info is a textview rather than a bare pane because the blurbs are
	// prose: textview wraps to the inner width on every SetRect, which is
	// the one case rule 18 says to wrap rather than truncate.
	info textview.Model
}

func newRoot() *rootScreen {
	s := &rootScreen{}
	s.SetTheme(theme.New(appearance.Palettes()[1])) // start on Dark (index 0 is Terminal())
	return s
}

func (s *rootScreen) Title() string         { return "Examples" }
func (s *rootScreen) Init() tea.Cmd         { return textinput.Blink }
func (s *rootScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *rootScreen) IsCapturingKeys() bool { return s.menu.IsCapturingKeys() }

func (s *rootScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// Enter and a double click both arrive as the menu's ChosenMsg (rule 16).
	if it, ok := s.menu.Chosen(msg); ok {
		return s, screen.Push(entries[it.Data.(int)].build(s.t))
	}
	before, _ := s.menu.Selected()
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)
	if now, _ := s.menu.Selected(); now.ID != before.ID {
		s.rebuildInfo()
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

	st := s.menu.State()
	opts := t.Menu()
	opts.Title = fmt.Sprintf("examples · %d", len(entries))
	opts.Items = items()
	s.menu = menu.New(opts)
	s.menu.Restore(st)

	s.rebuildInfo()
}

func (s *rootScreen) rebuildInfo() {
	opts := s.t.TextView()
	opts.Wrap = true
	s.info = textview.New(opts)

	it, ok := s.menu.Selected()
	if !ok {
		s.info.SetTitle("about")
		s.info.SetContent("Pick an example on the left and press enter.")
		return
	}
	e := entries[it.Data.(int)]
	s.info.SetTitle("examples/" + e.dir)
	s.info.SetContent(e.name + "\n\n" + e.blurb)
}

func items() []menu.Item {
	out := make([]menu.Item, len(entries))
	for i, e := range entries {
		out[i] = menu.Item{ID: e.dir, Label: e.name, Description: e.tagline, Data: i}
	}
	return out
}

func main() {
	m := app.New(app.Options{
		Root:          newRoot(),
		Palettes:      appearance.Palettes(),
		Surfaces:      appearance.Surfaces(),
		PaletteEnvVar: "TUILIB_THEME",
		Version:       "examples",
		ThemeKey:      key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "palette")),
		SurfaceKey:    key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "surface")),
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
