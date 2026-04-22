// Demo: header breadcrumb + 3-level drilldown + inline help footer.
//
// Header and footer share the same background color by default so the frame
// reads as one unified shell around the body.
//
// Keys:
//   ↑/↓ or j/k   change selection
//   enter        drill in
//   esc          go back (auto-handled by nav)
//   q            quit (from root only)
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/nav"
	"github.com/jsdrews/tuilib/pkg/statusbar"
)

// Shared theme — header, footer, and embedded help all use barBG so the
// frame is visually continuous.
var (
	barBG  = lipgloss.Color("236")
	barFG  = lipgloss.Color("252")
	keyFG  = lipgloss.Color("75")
	accent = lipgloss.Color("213")
)

var keys = struct {
	Up, Down, Enter, Back, Quit key.Binding
}{
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Back:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ---- Screens ----------------------------------------------------------------

var data = map[string][]string{
	"North America": {"New York", "San Francisco", "Toronto"},
	"Europe":        {"London", "Berlin", "Paris"},
	"Asia":          {"Tokyo", "Singapore", "Seoul"},
}

// listScreen is a generic list screen used for the top two levels.
type listScreen struct {
	title   string
	items   []string
	cursor  int
	onEnter func(selected string) nav.Screen
}

func (s listScreen) Title() string { return s.title }
func (s listScreen) Init() tea.Cmd { return nil }
func (s listScreen) Update(msg tea.Msg) (nav.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, keys.Up):
			if s.cursor > 0 {
				s.cursor--
			}
		case key.Matches(k, keys.Down):
			if s.cursor < len(s.items)-1 {
				s.cursor++
			}
		case key.Matches(k, keys.Enter):
			if s.onEnter != nil {
				return s, nav.Push(s.onEnter(s.items[s.cursor]))
			}
		}
	}
	return s, nil
}
func (s listScreen) View() string {
	var b strings.Builder
	for i, item := range s.items {
		cursor := "  "
		if i == s.cursor {
			cursor = lipgloss.NewStyle().Bold(true).Foreground(accent).Render("▸ ")
			b.WriteString(cursor)
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(accent).Render(item))
		} else {
			b.WriteString(cursor + item)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// cityDetail is a leaf screen — no further navigation.
type cityDetail struct {
	region, city string
}

func (c cityDetail) Title() string { return c.city }
func (c cityDetail) Init() tea.Cmd { return nil }
func (c cityDetail) Update(msg tea.Msg) (nav.Screen, tea.Cmd) {
	return c, nil
}
func (c cityDetail) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(c.city)
	return fmt.Sprintf("%s\n\nregion: %s\npopulation: (unknown)\nweather:    sunny", header, c.region)
}

func newRegionList() listScreen {
	regions := []string{"North America", "Europe", "Asia"}
	return listScreen{
		title:   "Regions",
		items:   regions,
		onEnter: func(region string) nav.Screen { return newCityList(region) },
	}
}

func newCityList(region string) listScreen {
	return listScreen{
		title: region,
		items: data[region],
		onEnter: func(city string) nav.Screen {
			return cityDetail{region: region, city: city}
		},
	}
}

// ---- Root model -------------------------------------------------------------

type model struct {
	nav    nav.Model
	header breadcrumb.Model
	status statusbar.Model
	help   help.Model
	w, h   int
}

func initialModel() model {
	n := nav.New(nav.Options{Root: newRegionList()})

	bc := breadcrumb.New(breadcrumb.Options{
		BarBackground: barBG,
		BarForeground: barFG,
		Crumbs:        n.Crumbs(),
	})

	h := help.New(help.Options{
		KeyStyle:  lipgloss.NewStyle().Bold(true).Foreground(keyFG).Background(barBG),
		DescStyle: lipgloss.NewStyle().Foreground(barFG).Background(barBG),
	})
	h.SetBindings([]key.Binding{
		keys.Up, keys.Down, keys.Enter, keys.Back, keys.Quit,
	})

	sb := statusbar.New(statusbar.Options{
		BarBackground: barBG,
		BarForeground: barFG,
		Left:          h.ShortView(),
		Right:         "v0.1.0",
	})

	return model{nav: n, header: bc, status: sb, help: h}
}

func (m model) Init() tea.Cmd { return m.nav.Init() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.status, _ = m.status.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.header.SetWidth(m.w)
		m.status.SetWidth(m.w)
	case tea.KeyMsg:
		// q quits only when at the root; otherwise esc/pop takes it.
		if key.Matches(msg, keys.Quit) && m.nav.Depth() == 1 {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.nav, cmd = m.nav.Update(msg)
	m.header.SetCrumbs(m.nav.Crumbs())
	return m, cmd
}

func (m model) View() string {
	// Body fills the space between the 1-line header and 1-line footer.
	bodyH := m.h - 2
	if bodyH < 1 {
		bodyH = 1
	}
	body := lipgloss.NewStyle().
		Width(m.w).
		Height(bodyH).
		Padding(1, 2).
		Render(m.nav.View())

	return m.header.View() + "\n" + body + "\n" + m.status.View()
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
