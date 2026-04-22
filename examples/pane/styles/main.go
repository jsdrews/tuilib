// Demo of border/title style variations for pkg/pane.
//
// Renders a 2x2 grid of panes, each configured differently:
//   - top-left:     thick blue border, centered bold title
//   - top-right:    rounded magenta border, left-aligned title with bg
//   - bottom-left:  double yellow border, right-aligned title
//   - bottom-right: hidden border, title tucked bottom-left
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jsdrews/tuilib/pkg/pane"
)

const (
	paneW = 44
	paneH = 10
)

func fillerFor(label string) string {
	lines := []string{
		label,
		"",
		"the quick brown fox",
		"jumps over the lazy dog",
		"the quick brown fox",
		"jumps over the lazy dog",
	}
	return strings.Join(lines, "\n")
}

type model struct {
	panes [4]pane.Pane
}

func initialModel() model {
	// 1. Thick blue border, centered bold title.
	p1 := pane.New(pane.Options{
		Width:   paneW,
		Height:  paneH,
		Focused: true, // active => thick border
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			Render("Thick / Centered"),
		TitlePosition: pane.TopMiddleBorder,
		ActiveColor:   lipgloss.Color("12"),
	})
	p1.SetContent(fillerFor("thick border, centered bold title"))

	// 2. Rounded magenta border, top-left title with pink background.
	p2 := pane.New(pane.Options{
		Width:          paneW,
		Height:         paneH,
		Focused:        false, // inactive => inactiveBorder
		InactiveBorder: lipgloss.RoundedBorder(),
		InactiveColor:  lipgloss.Color("205"),
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("205")).
			Bold(true).
			Render(" Rounded / Left "),
		TitlePosition: pane.TopLeftBorder,
	})
	p2.SetContent(fillerFor("rounded border, left title w/ bg"))

	// 3. Double yellow border, right-aligned italic title.
	p3 := pane.New(pane.Options{
		Width:          paneW,
		Height:         paneH,
		Focused:        false,
		InactiveBorder: lipgloss.DoubleBorder(),
		InactiveColor:  lipgloss.Color("226"),
		Title: lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("226")).
			Render("Double / Right"),
		TitlePosition: pane.TopRightBorder,
	})
	p3.SetContent(fillerFor("double border, right italic title"))

	// 4. Hidden border, title tucked bottom-left.
	p4 := pane.New(pane.Options{
		Width:          paneW,
		Height:         paneH,
		Focused:        false,
		InactiveBorder: lipgloss.HiddenBorder(),
		InactiveColor:  lipgloss.Color("244"),
		Title: lipgloss.NewStyle().
			Faint(true).
			Render("(hidden border / bottom-left title)"),
		TitlePosition: pane.BottomLeftBorder,
	})
	p4.SetContent(fillerFor("no visible border"))

	return model{panes: [4]pane.Pane{p1, p2, p3, p4}}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		s := k.String()
		if s == "q" || s == "ctrl+c" || s == "esc" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	top := lipgloss.JoinHorizontal(lipgloss.Top, m.panes[0].View(), m.panes[1].View())
	bot := lipgloss.JoinHorizontal(lipgloss.Top, m.panes[2].View(), m.panes[3].View())
	return lipgloss.JoinVertical(lipgloss.Left, top, bot, "\n  press q to quit")
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
