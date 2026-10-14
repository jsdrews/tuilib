// Command quickstart is the program the documentation's quick start builds:
// a filterable list of cities that opens a detail screen for the one you pick,
// running under the app shell.
//
//	go run ./examples/quickstart
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type city struct {
	Name, Country, Region, Population string
}

var cities = []city{
	{"London", "United Kingdom", "Europe", "8.9M"},
	{"Tokyo", "Japan", "Asia", "13.9M"},
	{"Madrid", "Spain", "Europe", "3.3M"},
	{"Lima", "Peru", "South America", "10.0M"},
	{"Nairobi", "Kenya", "Africa", "4.4M"},
	{"Toronto", "Canada", "North America", "2.8M"},
}

// cityList is the root screen: one filterable list filling the body.
type cityList struct {
	t    theme.Theme
	list list.Model
}

func (s *cityList) Title() string                { return "Cities" }
func (s *cityList) Init() tea.Cmd                { return nil }
func (s *cityList) OnEnter(any) tea.Cmd          { return nil }
func (s *cityList) IsCapturingKeys() bool        { return s.list.IsCapturingKeys() }
func (s *cityList) Layout() layout.Node          { return layout.Sized(&s.list) }
func (s *cityList) Help() []key.Binding          { return s.list.Help() }
func (s *cityList) HelpSections() []help.Section { return s.list.HelpSections() }

func (s *cityList) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	// Enter and double-click are one verb: open the selection.
	if s.list.IsActivate(msg) {
		if i, ok := s.list.SelectedIndex(); ok {
			return s, screen.Push(newCityDetail(cities[i], s.t))
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *cityList) SetTheme(t theme.Theme) {
	s.t = t
	st := s.list.State()
	opts := t.List()
	opts.Title = "Cities"
	opts.Filterable = true
	for _, c := range cities {
		opts.Items = append(opts.Items, c.Name)
	}
	s.list = list.New(opts)
	s.list.Restore(st)
}

// cityDetail is pushed on top of the list. Esc pops it; the shell does that.
type cityDetail struct {
	city city
	ins  inspector.Model
}

func newCityDetail(c city, t theme.Theme) *cityDetail {
	s := &cityDetail{city: c}
	s.SetTheme(t)
	return s
}

func (s *cityDetail) Title() string                { return s.city.Name }
func (s *cityDetail) Init() tea.Cmd                { return nil }
func (s *cityDetail) OnEnter(any) tea.Cmd          { return nil }
func (s *cityDetail) IsCapturingKeys() bool        { return s.ins.IsCapturingKeys() }
func (s *cityDetail) Layout() layout.Node          { return layout.Sized(&s.ins) }
func (s *cityDetail) Help() []key.Binding          { return s.ins.Help() }
func (s *cityDetail) HelpSections() []help.Section { return s.ins.HelpSections() }

func (s *cityDetail) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.ins, cmd = s.ins.Update(msg)
	return s, cmd
}

func (s *cityDetail) SetTheme(t theme.Theme) {
	st := s.ins.State()
	opts := t.Inspector()
	opts.Title = s.city.Name
	opts.Fields = []inspector.Field{
		{Label: "Country", Value: s.city.Country},
		{Label: "Region", Value: s.city.Region},
		{Label: "Population", Value: s.city.Population},
	}
	s.ins = inspector.New(opts)
	s.ins.Restore(st)
}

func main() {
	m := app.New(app.Options{
		Root:     &cityList{},
		Palettes: palette.All(),
		ThemeKey: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		Mouse:    app.MouseClick,
		Version:  "quickstart",
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
