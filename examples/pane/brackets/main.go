// Demo: thin border variants, plus the three SlotBrackets styles that
// control how the title meets the border edge.
//
// From left to right:
//   - thin border, corners (default, pug-style tab)
//   - thin border, none    (title sits flush on the border line)
//   - thin border, tees    (junction tees point into the title)
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
	paneW = 36
	paneH = 9
)

func content(label string) string {
	lines := []string{
		label,
		"",
		"the quick brown fox",
		"jumps over the lazy dog",
		"—",
		"thin = lipgloss.NormalBorder()",
	}
	return strings.Join(lines, "\n")
}

func makePane(title string, brackets pane.SlotBracketStyle) pane.Pane {
	p := pane.New(pane.Options{
		Width:  paneW,
		Height: paneH,
		// Active border is thin (NormalBorder) and we force focus=true so it
		// renders. To go "thinner still" you can use RoundedBorder() or
		// HiddenBorder(); see examples/pane/styles for those.
		ActiveBorder:  lipgloss.NormalBorder(),
		ActiveColor:   lipgloss.Color("12"),
		Focused:       true,
		Title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Render(title),
		TitlePosition: pane.TopMiddleBorder,
		SlotBrackets:  brackets,
	})
	return p
}

type model struct {
	panes [3]pane.Pane
}

func initialModel() model {
	p1 := makePane("Corners", pane.SlotBracketsCorners)
	p1.SetContent(content("SlotBracketsCorners (default)"))

	p2 := makePane("No Brackets", pane.SlotBracketsNone)
	p2.SetContent(content("SlotBracketsNone"))

	p3 := makePane("Tees", pane.SlotBracketsTees)
	p3.SetContent(content("SlotBracketsTees"))

	return model{panes: [3]pane.Pane{p1, p2, p3}}
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
	row := lipgloss.JoinHorizontal(lipgloss.Top,
		m.panes[0].View(), m.panes[1].View(), m.panes[2].View())
	return row + "\n\n  press q to quit"
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
