// Package navigation is the navigation showcase: how an app moves between
// screens, from one home, so a change that breaks an older feature shows up
// where you can see it.
//
// The home is a grouped, filterable pkg/menu — the shape tabs run out of room
// for (rule 36). Its "navigation" group opens the screens beside this package,
// each a different way of moving:
//
//	screen stack   push with data, pop with a result into the parent's
//	               OnEnter, Replace a screen with a fresh one of itself
//	               (rule 7)
//	sign in first  a screen pushed in front of another on first activation,
//	               whose result decides what happens next
//	drilldown      master and detail with async loads; enter moves focus to
//	               the detail, enter there pushes a third level (rule 16)
//	tabbed host    pkg/tab: bodies behind a strip inside one screen, never
//	               touching the stack (rule 21)
//
// The other groups are the menu's own subject: items filed under headers the
// cursor skips, those without a group last under "other", tab / shift+tab or
// ] / [ jumping between groups and wrapping, "/" filtering on the label and
// the group (try "network" or "instance"). Enter or a double click emits
// ChosenMsg, and this screen decides that opening means screen.Push. The
// breadcrumb names every level, and clicking a crumb unwinds to it.
package navigation

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/examples/navigation/drilldown"
	"github.com/jsdrews/tuilib/examples/navigation/prescreen"
	"github.com/jsdrews/tuilib/examples/navigation/stack"
	"github.com/jsdrews/tuilib/examples/navigation/tabs"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// section is what each item carries in Data and what opening it shows.
type section struct {
	path string
	verb string
}

// screenFor is what a navigation item opens: one of the screens beside this
// package, built against the current theme.
type screenFor func(theme.Theme) screen.Screen

func items() []menu.Item {
	nav := func(label, desc string, open screenFor) menu.Item {
		return menu.Item{ID: label, Label: label, Group: "navigation", Description: desc, Data: open}
	}
	return append([]menu.Item{
		nav("screen stack", "push, pop with a result, replace", stack.New),
		nav("sign in first", "a screen pushed in front", prescreen.New),
		nav("drilldown", "master and detail, then a third level", drilldown.New),
		nav("tabbed host", "tabs inside one screen", tabs.New),
	}, sections()...)
}

func sections() []menu.Item {
	it := func(group, label, verb, path string) menu.Item {
		return menu.Item{
			ID: path, Label: label, Group: group,
			Description: verb + " " + path,
			Data:        section{path: path, verb: verb},
		}
	}
	return []menu.Item{
		it("database", "postgres › instance", "GET", "/v1/db/postgres/instance"),
		it("database", "postgres › backups", "GET", "/v1/db/postgres/backups"),
		it("database", "mysql › instance", "GET", "/v1/db/mysql/instance"),
		it("database", "redis › cache", "GET", "/v1/db/redis/cache"),
		it("network", "vpc", "GET", "/v1/net/vpc"),
		it("network", "subnets", "GET", "/v1/net/subnets"),
		it("network", "firewall rules", "GET", "/v1/net/firewall"),
		it("network", "load balancers", "GET", "/v1/net/lb"),
		{ID: "about", Label: "about", Description: "what this demo shows"},
		it("compute", "instances", "GET", "/v1/compute/instances"),
		it("compute", "instance groups", "GET", "/v1/compute/groups"),
		it("compute", "images", "GET", "/v1/compute/images"),
		it("identity", "users", "GET", "/v1/iam/users"),
		it("identity", "service accounts", "GET", "/v1/iam/service-accounts"),
		it("identity", "roles", "GET", "/v1/iam/roles"),
		{ID: "settings", Label: "settings", Description: "nothing to configure"},
		it("observability", "dashboards", "GET", "/v1/obs/dashboards"),
		it("observability", "alert policies", "GET", "/v1/obs/alerts"),
		it("observability", "log sinks", "GET", "/v1/obs/sinks"),
	}
}

type Screen struct {
	t    theme.Theme
	menu menu.Model
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Navigation" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.menu.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if it, ok := s.menu.Chosen(msg); ok {
		if open, isNav := it.Data.(screenFor); isNav {
			return s, screen.Push(open(s.t))
		}
		return s, screen.Push(newSection(it, s.t))
	}
	var cmd tea.Cmd
	s.menu, cmd = s.menu.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.menu) }

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section { return help.SectionsOf(&s.menu) }

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	st := s.menu.State()
	opts := t.Menu()
	opts.Title = fmt.Sprintf("navigation · %d", len(items()))
	opts.Filterable = true
	opts.Filter.Placeholder = "filter by label or group…"
	opts.Items = items()
	s.menu = menu.New(opts)
	s.menu.Restore(st)
}

// sectionScreen is what opening an item pushes: a stand-in for the screen a
// real app would build for that section.
type sectionScreen struct {
	item menu.Item
	view textview.Model
}

func newSection(it menu.Item, t theme.Theme) *sectionScreen {
	s := &sectionScreen{item: it}
	s.SetTheme(t)
	return s
}

func (s *sectionScreen) Title() string         { return s.item.Label }
func (s *sectionScreen) Init() tea.Cmd         { return nil }
func (s *sectionScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *sectionScreen) IsCapturingKeys() bool { return s.view.IsCapturingKeys() }
func (s *sectionScreen) Layout() layout.Node   { return layout.Sized(&s.view) }
func (s *sectionScreen) Help() []key.Binding   { return s.view.Help() }

func (s *sectionScreen) HelpSections() []help.Section { return help.SectionsOf(&s.view) }

func (s *sectionScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.view, cmd = s.view.Update(msg)
	return s, cmd
}

func (s *sectionScreen) SetTheme(t theme.Theme) {
	st := s.view.State()
	opts := t.TextView()
	opts.Title = s.item.Label
	opts.Content = s.content()
	s.view = textview.New(opts)
	s.view.Restore(st)
}

func (s *sectionScreen) content() string {
	sec, ok := s.item.Data.(section)
	if !ok {
		return s.item.Description + "\n\nEsc goes back to the menu, with the cursor and filter where you left them."
	}
	group := s.item.Group
	return fmt.Sprintf("group  %s\nverb   %s\npath   %s\n\n"+
		"A real app would fetch %s here. The menu handed this screen the item's Data;\n"+
		"what opening means was this screen's call, not the menu's.\n\n"+
		"Esc goes back.", group, sec.verb, sec.path, sec.path)
}
