// Demo of pkg/pane + pkg/statusbar + pkg/help working together.
//
// Keys:
//   ?           toggle help overlay
//   i           post an info message
//   e           post an error message
//   ↑/↓/pgup/pgdn   scroll the pane
//   q           quit
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/statusbar"
)

var keys = struct {
	Help, Quit, Info, Error key.Binding
	Up, Down                key.Binding
}{
	Help:  key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle help")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Info:  key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info msg")),
	Error: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "error msg")),
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
}

type model struct {
	pane     pane.Pane
	status   statusbar.Model
	help     help.Model
	showHelp bool
	w, h     int
}

func initialModel() model {
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d — the quick brown fox jumps over the lazy dog", i+1)
	}

	p := pane.New(pane.Options{
		Title:         "Demo",
		Focused:       true,
		SlotBrackets:  pane.SlotBracketsNone,
		TitlePosition: pane.TopMiddleBorder,
	})
	p.SetContent(strings.Join(lines, "\n"))

	sb := statusbar.New(statusbar.Options{
		Left:  "? help",
		Right: "v0.1.0",
	})

	h := help.New(help.Options{
		Height: 6,
		KeyStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginRight(1),
		DescStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
	})
	h.SetBindings([]key.Binding{
		keys.Help, keys.Quit, keys.Info, keys.Error, keys.Up, keys.Down,
	})

	return model{pane: p, status: sb, help: h}
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) resize() {
	footerH := 1
	if m.showHelp {
		footerH += m.help.Height()
	}
	m.pane.SetDimensions(m.w, m.h-footerH)
	m.status.SetWidth(m.w)
	m.help.SetDimensions(m.w, 6)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Let statusbar auto-clear messages on any key event.
	m.status, _ = m.status.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp
			m.resize()
			return m, nil
		case key.Matches(msg, keys.Info):
			m.status.SetInfo("info: clicked i at " + fmt.Sprintf("%dx%d", m.w, m.h))
			return m, nil
		case key.Matches(msg, keys.Error):
			m.status.SetError("error: something went wrong")
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.pane, cmd = m.pane.Update(msg)
	return m, cmd
}

func (m model) View() string {
	parts := []string{m.pane.View()}
	if m.showHelp {
		parts = append(parts, m.help.View())
	}
	parts = append(parts, m.status.View())
	return strings.Join(parts, "\n")
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
