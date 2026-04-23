// Demo: breadcrumb header + nav.Stack + Pane-wrapped body + inline help,
// themed via the theme package. Change th below to swap palettes across the
// whole app in one line.
//
// Keys:
//   ↑/↓ or j/k   change selection
//   enter        drill in
//   esc          back (auto-handled by nav)
//   q            quit (from root)
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
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Single source of truth for the palette. Swap to theme.Dark(), Dracula(),
// Solarized(), … and every bar/border/highlight follows.
var th = theme.Nord()

var keys = struct {
	Up, Down, Enter, Back, Quit key.Binding
}{
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Back:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ---- Screens ---------------------------------------------------------------

var data = map[string][]string{
	"North America": {"New York", "San Francisco", "Toronto", "Vancouver", "Chicago"},
	"Europe":        {"London", "Berlin", "Paris", "Madrid", "Amsterdam"},
	"Asia":          {"Tokyo", "Singapore", "Seoul", "Mumbai", "Bangkok"},
}

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
	selected := lipgloss.NewStyle().Bold(true).Foreground(th.Accent)
	var b strings.Builder
	for i, item := range s.items {
		if i == s.cursor {
			b.WriteString(selected.Render("▸ " + item))
		} else {
			b.WriteString("  " + item)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ItemCount lets us put "N items" in the pane's bottom-left slot.
func (s listScreen) ItemCount() int { return len(s.items) }

type cityDetail struct{ region, city string }

func (c cityDetail) Title() string                          { return c.city }
func (c cityDetail) Init() tea.Cmd                          { return nil }
func (c cityDetail) Update(_ tea.Msg) (nav.Screen, tea.Cmd) { return c, nil }
func (c cityDetail) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Render(c.city)
	return fmt.Sprintf("%s\n\nregion:     %s\npopulation: (unknown)\nweather:    sunny", header, c.region)
}

func newRegionList() listScreen {
	return listScreen{
		title:   "Regions",
		items:   []string{"North America", "Europe", "Asia"},
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

// ---- Root model ------------------------------------------------------------

type model struct {
	nav    nav.Model
	header breadcrumb.Model
	body   pane.Pane
	status statusbar.Model
	w, h   int
}

func initialModel() model {
	n := nav.New(nav.Options{Root: newRegionList()})

	bcOpts := th.Breadcrumb()
	bcOpts.Crumbs = n.Crumbs()
	bc := breadcrumb.New(bcOpts)

	paneOpts := th.Pane()
	paneOpts.Title = n.Current().Title()
	paneOpts.TitlePosition = pane.TopMiddleBorder
	paneOpts.SlotBrackets = pane.SlotBracketsNone
	paneOpts.Focused = true
	paneOpts.ActiveBorder = lipgloss.NormalBorder()
	body := pane.New(paneOpts)
	body.SetContent(n.View())

	h := help.New(th.Help())
	h.SetBindings([]key.Binding{keys.Up, keys.Down, keys.Enter, keys.Back, keys.Quit})

	sb := statusbar.New(th.Statusbar(h.ShortView(), "v0.1.0"))

	return model{nav: n, header: bc, body: body, status: sb}
}

func (m model) Init() tea.Cmd { return m.nav.Init() }

func (m *model) refreshBody() {
	cur := m.nav.Current()
	m.body.SetTitle(cur.Title())
	m.body.SetContent(cur.View())
	if ls, ok := cur.(listScreen); ok {
		m.body.SetBottomLeft(fmt.Sprintf("%d items", ls.ItemCount()))
	} else {
		m.body.SetBottomLeft("")
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.status, _ = m.status.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.header.SetWidth(m.w)
		m.status.SetWidth(m.w)
		m.body.SetDimensions(m.w, m.h-2)
	case tea.KeyMsg:
		if key.Matches(msg, keys.Quit) && m.nav.Depth() == 1 {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.nav, cmd = m.nav.Update(msg)
	m.header.SetCrumbs(m.nav.Crumbs())
	m.refreshBody()
	return m, cmd
}

func (m model) View() string {
	return m.header.View() + "\n" + m.body.View() + "\n" + m.status.View()
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
