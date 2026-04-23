// Demo: interactive theme picker with drilldown.
//
// Root view lists every built-in theme — moving the cursor re-skins the TUI
// so you can compare palettes live. Pressing enter drills into the current
// theme and dumps every Theme struct field with a color swatch and the value
// rendered in that color.
//
// Keys (list):
//   ↑/↓ or j/k   highlight a theme (and apply it)
//   enter        show that theme's fields
//   q            quit
// Keys (detail):
//   esc          back to the list
//   q            quit
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
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/theme"
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

type viewMode int

const (
	viewList viewMode = iota
	viewDetail
)

type model struct {
	themes []theme.Theme
	cursor int
	mode   viewMode
	w, h   int

	// Re-created on every theme or mode change so each component's baked-in
	// styles reflect the active palette.
	header breadcrumb.Model
	body   pane.Pane
	status statusbar.Model
}

func initialModel() model {
	// Terminal() queries the live terminal palette — prepend so it's the first
	// entry, not buried in All(). Must run before tea.NewProgram takes stdin.
	themes := append([]theme.Theme{theme.Terminal()}, theme.All()...)
	return model{themes: themes}
}

// apply rebuilds every themed component from the current theme + mode.
func (m *model) apply() {
	th := m.themes[m.cursor]

	// Breadcrumb reflects drilldown depth.
	bcOpts := th.Breadcrumb()
	bcOpts.Width = m.w
	if m.mode == viewDetail {
		bcOpts.Crumbs = []string{"Themes", th.Name}
	} else {
		bcOpts.Crumbs = []string{"Themes"}
	}
	m.header = breadcrumb.New(bcOpts)

	// Pane.
	paneOpts := th.Pane()
	paneOpts.Width = m.w
	paneOpts.Height = m.h - 2
	paneOpts.TitlePosition = pane.TopMiddleBorder
	paneOpts.SlotBrackets = pane.SlotBracketsNone
	paneOpts.Focused = true
	paneOpts.ActiveBorder = lipgloss.NormalBorder()
	if m.mode == viewDetail {
		paneOpts.Title = th.Name
	} else {
		paneOpts.Title = "Themes"
	}
	m.body = pane.New(paneOpts)
	if m.mode == viewDetail {
		m.body.SetContent(m.detail(th))
		m.body.SetBottomLeft(fmt.Sprintf("%d fields", 14))
	} else {
		m.body.SetContent(m.list(th))
		m.body.SetBottomLeft(fmt.Sprintf("%d themes", len(m.themes)))
	}

	// Help hints match the current mode.
	var bindings []key.Binding
	if m.mode == viewDetail {
		bindings = []key.Binding{keys.Back, keys.Quit}
	} else {
		bindings = []key.Binding{keys.Up, keys.Down, keys.Enter, keys.Quit}
	}
	h := help.New(th.Help())
	h.SetBindings(bindings)

	sbOpts := th.Statusbar(h.ShortView(), "theme: "+th.Name)
	sbOpts.Width = m.w
	m.status = statusbar.New(sbOpts)
}

// list renders the theme menu. The highlighted row uses the active theme's
// accent color — a preview of the palette's body-content highlight.
func (m model) list(th theme.Theme) string {
	selected := lipgloss.NewStyle().Bold(true).Foreground(th.Accent)
	var b strings.Builder
	for i, t := range m.themes {
		if i == m.cursor {
			b.WriteString(selected.Render("▸ " + t.Name))
		} else {
			b.WriteString("  " + t.Name)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// detail renders every Theme field. Each color value is drawn in that color,
// preceded by a solid swatch so very-dark colors (e.g. 0) are still visible.
func (m model) detail(th theme.Theme) string {
	type field struct {
		label string
		color lipgloss.TerminalColor
	}
	fields := []field{
		{"BarBG", th.BarBG},
		{"BarFG", th.BarFG},
		{"Current", th.Current},
		{"Muted", th.Muted},
		{"Subtle", th.Subtle},
		{"KeyFG", th.KeyFG},
		{"BorderActive", th.BorderActive},
		{"BorderInactive", th.BorderInactive},
		{"InfoBG", th.InfoBG},
		{"InfoFG", th.InfoFG},
		{"ErrorBG", th.ErrorBG},
		{"ErrorFG", th.ErrorFG},
		{"Accent", th.Accent},
	}

	label := lipgloss.NewStyle().Foreground(th.Muted).Width(18)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(th.Current)

	var b strings.Builder
	b.WriteString(label.Render("Name") + nameStyle.Render(th.Name) + "\n\n")
	for _, f := range fields {
		swatch := lipgloss.NewStyle().Background(f.color).Render("   ")
		value := lipgloss.NewStyle().Foreground(f.color).Render(colorString(f.color))
		b.WriteString(label.Render(f.label) + swatch + "  " + value + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// colorString extracts the ANSI index / hex string from a TerminalColor.
// lipgloss.Color is the only concrete type the built-in themes use, so the
// switch is narrow on purpose — the default branch is a safety net.
func colorString(c lipgloss.TerminalColor) string {
	switch c := c.(type) {
	case lipgloss.Color:
		return string(c)
	case lipgloss.AdaptiveColor:
		return fmt.Sprintf("adaptive(light=%s dark=%s)", c.Light, c.Dark)
	case lipgloss.CompleteColor:
		return fmt.Sprintf("complete(%s)", c.TrueColor)
	default:
		return fmt.Sprintf("%v", c)
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.apply()
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case m.mode == viewList && key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.apply()
			}
		case m.mode == viewList && key.Matches(msg, keys.Down):
			if m.cursor < len(m.themes)-1 {
				m.cursor++
				m.apply()
			}
		case m.mode == viewList && key.Matches(msg, keys.Enter):
			m.mode = viewDetail
			m.apply()
		case m.mode == viewDetail && key.Matches(msg, keys.Back):
			m.mode = viewList
			m.apply()
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	return m.header.View() + "\n" + m.body.View() + "\n" + m.status.View()
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
