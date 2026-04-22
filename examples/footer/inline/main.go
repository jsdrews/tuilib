// Demo: all help hints always visible inline in the status bar.
//
// No toggle, no overlay — the status bar's left slot shows every binding.
// Info/error messages still work and appear in the middle slot when triggered.
//
// Keys:
//   i         post info message
//   e         post error message
//   ↑/↓       scroll
//   q         quit
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
	Quit, Info, Error key.Binding
	Up, Down          key.Binding
}{
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Info:  key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info")),
	Error: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "error")),
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
}

type model struct {
	pane   pane.Pane
	status statusbar.Model
	w, h   int
}

func initialModel() model {
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d — pane content", i+1)
	}
	p := pane.New(pane.Options{
		Title:        "Demo",
		Focused:      true,
		SlotBrackets: pane.SlotBracketsNone,
	})
	p.SetContent(strings.Join(lines, "\n"))

	// Every slot in the bar — including the key/desc inside the inline help
	// — needs the same background for a continuous strip. Grab the bar's bg
	// from the statusbar and feed it into the help styles.
	barBG := lipgloss.Color("236")
	keyFG := lipgloss.Color("75")
	descFG := lipgloss.Color("252")

	h := help.New(help.Options{
		KeyStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(keyFG).
			Background(barBG),
		DescStyle: lipgloss.NewStyle().
			Foreground(descFG).
			Background(barBG),
		ShortSeparator: "  •  ",
	})
	h.SetBindings([]key.Binding{
		keys.Up, keys.Down, keys.Info, keys.Error, keys.Quit,
	})

	sb := statusbar.New(statusbar.Options{
		BarBackground: barBG,
		BarForeground: descFG,
		Left:          h.ShortView(),
		Right:         "v0.1.0",
	})

	return model{pane: p, status: sb}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.status, _ = m.status.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.pane.SetDimensions(m.w, m.h-1)
		m.status.SetWidth(m.w)
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Info):
			m.status.SetInfo("info: clicked i")
			return m, nil
		case key.Matches(msg, keys.Error):
			m.status.SetError("error: clicked e")
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.pane, cmd = m.pane.Update(msg)
	return m, cmd
}

func (m model) View() string {
	return m.pane.View() + "\n" + m.status.View()
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
