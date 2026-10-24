// Package menu demonstrates menu.Model as the home screen of an app with many
// sections — the shape tabs run out of room for.
//
//   - Items are filed under groups, drawn as headers the cursor skips. The
//     two without a group land last, under "other".
//   - [ and ] jump to the previous and next group.
//   - "/" filters on the label and the group (try "network" or "instance");
//     a group with nothing left loses its header. Descriptions aren't
//     searched.
//   - Enter or a double click opens the section: the menu emits ChosenMsg
//     and this screen decides that opening means screen.Push.
package menu

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

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

func items() []menu.Item {
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

func (s *Screen) Title() string         { return "Sections" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.menu.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if it, ok := s.menu.Chosen(msg); ok {
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
	opts.Title = fmt.Sprintf("sections · %d", len(items()))
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
