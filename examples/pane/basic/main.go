package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jsdrews/tuilib/pkg/pane"
)

type model struct {
	pane pane.Pane
}

func initialModel() model {
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d — the quick brown fox jumps over the lazy dog", i+1)
	}
	p := pane.New(pane.Options{
		Width:   80,
		Height:  20,
		Title:   "Demo",
		Focused: true,
	})
	p.SetContent(strings.Join(lines, "\n"))
	p.SetBottomLeft("q: quit  ↑/↓: scroll")
	return model{pane: p}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.pane.SetDimensions(msg.Width, msg.Height)
	}
	var cmd tea.Cmd
	m.pane, cmd = m.pane.Update(msg)
	return m, cmd
}

func (m model) View() string {
	return m.pane.View()
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
