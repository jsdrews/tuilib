// Demo: filterable table (bubbles/table) inside a pane, with a breadcrumb
// header, a filter.Model below the header, and an inline-help statusbar
// footer. Themed via the theme package — starts on Nord, press "t" to cycle.
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
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var themes = []theme.Theme{
	theme.Nord(), theme.Dark(), theme.Dracula(), theme.Gruvbox(),
	theme.TokyoNight(), theme.CatppuccinMocha(), theme.Light(),
}

var allRows = []table.Row{
	{"New York", "North America", "8.3M"},
	{"San Francisco", "North America", "0.87M"},
	{"Toronto", "North America", "2.8M"},
	{"Vancouver", "North America", "0.67M"},
	{"Chicago", "North America", "2.7M"},
	{"Mexico City", "North America", "9.2M"},
	{"London", "Europe", "8.9M"},
	{"Berlin", "Europe", "3.7M"},
	{"Paris", "Europe", "2.1M"},
	{"Madrid", "Europe", "3.3M"},
	{"Amsterdam", "Europe", "0.9M"},
	{"Lisbon", "Europe", "0.55M"},
	{"Prague", "Europe", "1.3M"},
	{"Tokyo", "Asia", "13.9M"},
	{"Singapore", "Asia", "5.7M"},
	{"Seoul", "Asia", "9.7M"},
	{"Mumbai", "Asia", "20.4M"},
	{"Bangkok", "Asia", "10.7M"},
	{"Hong Kong", "Asia", "7.5M"},
	{"Taipei", "Asia", "2.6M"},
	{"Sydney", "Oceania", "5.3M"},
	{"Melbourne", "Oceania", "5.1M"},
	{"Auckland", "Oceania", "1.7M"},
	{"São Paulo", "South America", "12.3M"},
	{"Buenos Aires", "South America", "3.1M"},
	{"Lima", "South America", "10.7M"},
	{"Bogotá", "South America", "7.9M"},
	{"Nairobi", "Africa", "4.4M"},
	{"Cape Town", "Africa", "4.6M"},
	{"Lagos", "Africa", "15.4M"},
	{"Cairo", "Africa", "9.5M"},
}

var keys = struct {
	Filter, Theme, Quit key.Binding
}{
	Filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Theme:  key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// For the footer only — bubbles/table owns the actual cursor keys (up/down/
// j/k/pgup/pgdn/home/end) internally, so we just advertise them here.
var navHelp = []key.Binding{
	key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
}

type model struct {
	themeIdx int
	table    table.Model
	w, h     int

	header breadcrumb.Model
	filter filter.Model
	body   pane.Pane
	status statusbar.Model
}

func initialModel() model {
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "City", Width: 20},
			{Title: "Region", Width: 16},
			{Title: "Population", Width: 12},
		}),
		table.WithRows(allRows),
		table.WithFocused(true),
	)

	m := model{table: t}
	m.apply()
	return m
}

func (m model) theme() theme.Theme { return themes[m.themeIdx] }

func (m *model) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		m.table.SetRows(allRows)
		return
	}
	out := make([]table.Row, 0, len(allRows))
	for _, r := range allRows {
		if rowMatches(r, q) {
			out = append(out, r)
		}
	}
	m.table.SetRows(out)
	if m.table.Cursor() >= len(out) {
		m.table.SetCursor(max(0, len(out)-1))
	}
}

func rowMatches(r table.Row, q string) bool {
	for _, cell := range r {
		if strings.Contains(strings.ToLower(cell), q) {
			return true
		}
	}
	return false
}

// apply rebuilds every themed component. Called on init, resize, and theme swap.
func (m *model) apply() {
	th := m.theme()

	bcOpts := th.Breadcrumb()
	bcOpts.Width = m.w
	bcOpts.Crumbs = []string{"Cities"}
	m.header = breadcrumb.New(bcOpts)

	// Carry any existing value across theme swaps / resizes.
	prev := m.filter.Value()
	fOpts := th.Filter()
	fOpts.Width = m.w
	fOpts.Placeholder = "filter rows…"
	m.filter = filter.New(fOpts)
	if prev != "" {
		m.filter.SetValue(prev)
	}

	// Non-selected cells fall through to terminal default so the Selected
	// row's Accent fg wins (inner Cell fg would otherwise override Selected).
	// Subtle bg gives the row a faint highlight band.
	m.table.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(th.Current).Padding(0, 1),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
		Selected: lipgloss.NewStyle().Bold(true).Foreground(th.Accent).Background(th.Subtle),
	})

	paneOpts := th.Pane()
	paneOpts.Width = m.w
	paneOpts.Height = max(0, m.h-5) // breadcrumb(1) + filter(3) + statusbar(1)
	paneOpts.Title = "Cities"
	paneOpts.SlotBrackets = pane.SlotBracketsNone
	paneOpts.Focused = true
	paneOpts.ActiveBorder = lipgloss.NormalBorder()
	m.body = pane.New(paneOpts)

	// Inner table height = pane inner height minus header-row + separator.
	m.table.SetWidth(max(0, m.w-2-pane.ScrollbarWidth))
	m.table.SetHeight(max(0, paneOpts.Height-2-2))

	m.refreshBody()

	h := help.New(th.Help())
	h.SetBindings(append(append([]key.Binding{}, navHelp...), keys.Filter, keys.Theme, keys.Quit))
	sbOpts := th.Statusbar(h.ShortView(), "theme: "+th.Name)
	sbOpts.Width = m.w
	m.status = statusbar.New(sbOpts)
}

func (m *model) refreshBody() {
	m.body.SetContent(m.table.View())
	m.body.SetBottomLeft(fmt.Sprintf("%d / %d", len(m.table.Rows()), len(allRows)))
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.apply()
		return m, nil

	case tea.KeyMsg:
		if m.filter.Focused() {
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.applyFilter()
			m.refreshBody()
			return m, cmd
		}

		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Theme):
			m.themeIdx = (m.themeIdx + 1) % len(themes)
			m.apply()
			return m, nil
		case key.Matches(msg, keys.Filter):
			return m, m.filter.Focus()
		}

		// Table handles cursor keys.
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		m.refreshBody()
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	return m.header.View() + "\n" +
		m.filter.View() + "\n" +
		m.body.View() + "\n" +
		m.status.View()
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
