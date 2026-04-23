// Demo: filterable list.Model inside a breadcrumb header + statusbar footer,
// themed via the theme package — starts on Nord, press "t" to cycle.
//
// Keys:
//   ↑/↓ or j/k   move cursor
//   /            focus the filter (then enter to commit, esc to clear)
//   t            cycle theme
//   q            quit
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var themes = []theme.Theme{
	theme.Nord(), theme.Dark(), theme.Dracula(), theme.Gruvbox(),
	theme.TokyoNight(), theme.CatppuccinMocha(), theme.Light(),
}

var cities = []string{
	"New York", "San Francisco", "Toronto", "Vancouver", "Chicago",
	"London", "Berlin", "Paris", "Madrid", "Amsterdam", "Lisbon", "Prague",
	"Tokyo", "Singapore", "Seoul", "Mumbai", "Bangkok", "Hong Kong", "Taipei",
	"Sydney", "Melbourne", "Auckland",
	"São Paulo", "Buenos Aires", "Mexico City", "Lima", "Bogotá",
	"Nairobi", "Cape Town", "Lagos", "Cairo",
}

// Advertised in the statusbar help. The list widget owns the actual
// ↑/↓/j/k/"/" key routing internally.
var keys = struct {
	Up, Down, Filter, Theme, Quit key.Binding
}{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Theme:  key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

type model struct {
	themeIdx int
	w, h     int

	header breadcrumb.Model
	list   list.Model
	status statusbar.Model
}

func initialModel() model {
	m := model{}
	m.apply()
	return m
}

func (m model) theme() theme.Theme { return themes[m.themeIdx] }

// apply rebuilds every themed component. Called on init, resize, and theme swap.
func (m *model) apply() {
	th := m.theme()

	bcOpts := th.Breadcrumb()
	bcOpts.Width = m.w
	bcOpts.Crumbs = []string{"Cities"}
	m.header = breadcrumb.New(bcOpts)

	// Preserve cursor + filter across theme swaps / resizes.
	prevCursor, prevValue := m.list.Cursor(), m.list.Value()

	lOpts := th.List()
	lOpts.Width = m.w
	lOpts.Height = max(0, m.h-2) // breadcrumb(1) + statusbar(1)
	lOpts.Title = "Cities"
	lOpts.Items = cities
	lOpts.Filterable = true
	lOpts.Filter.Placeholder = "filter cities…"
	m.list = list.New(lOpts)
	if prevValue != "" {
		m.list.SetValue(prevValue)
	}
	m.list.SetCursor(prevCursor)

	h := help.New(th.Help())
	h.SetBindings([]key.Binding{keys.Up, keys.Down, keys.Filter, keys.Theme, keys.Quit})
	sbOpts := th.Statusbar(h.ShortView(), "theme: "+th.Name)
	sbOpts.Width = m.w
	m.status = statusbar.New(sbOpts)
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.apply()
		return m, nil

	case tea.KeyMsg:
		// Only intercept global keys when the list isn't taking typed input.
		if !m.list.Filtering() {
			switch {
			case key.Matches(msg, keys.Quit):
				return m, tea.Quit
			case key.Matches(msg, keys.Theme):
				m.themeIdx = (m.themeIdx + 1) % len(themes)
				m.apply()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	return m.header.View() + "\n" +
		m.list.View() + "\n" +
		m.status.View()
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
