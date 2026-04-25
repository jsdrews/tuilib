// Package table demonstrates a filterable bubbles/table inside a pane,
// composed with filter.Model via pkg/layout.
package table

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

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

type Screen struct {
	t      theme.Theme
	table  table.Model
	filter filter.Model
	body   pane.Pane
}

func New(t theme.Theme) screen.Screen {
	tab := table.New(
		table.WithColumns([]table.Column{
			{Title: "City", Width: 20},
			{Title: "Region", Width: 16},
			{Title: "Population", Width: 12},
		}),
		table.WithRows(allRows),
		table.WithFocused(true),
	)
	s := &Screen{table: tab}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Table" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.filter.Focused() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if s.filter.Focused() {
			var cmd tea.Cmd
			s.filter, cmd = s.filter.Update(msg)
			s.applyFilter()
			return s, cmd
		}
		if k.String() == "/" {
			return s, s.filter.Focus()
		}
	}
	var cmd tea.Cmd
	s.table, cmd = s.table.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.VStack(
		layout.Fixed(3, layout.Bar(&s.filter)),
		layout.Flex(1, layout.RenderFunc(s.renderBody)),
	)
}

func (s *Screen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	prev := s.filter.Value()
	fOpts := t.Filter()
	fOpts.Placeholder = "filter rows…"
	s.filter = filter.New(fOpts)
	if prev != "" {
		s.filter.SetValue(prev)
	}

	// Non-selected cells fall through to terminal default so the Selected row's
	// Accent fg wins; Subtle bg gives the row a faint highlight band.
	s.table.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.Current).Padding(0, 1),
		Cell:     lipgloss.NewStyle().Padding(0, 1),
		Selected: lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Background(t.Subtle),
	})

	paneOpts := t.Pane()
	paneOpts.Title = "Cities"
	paneOpts.Focused = true
	paneOpts.ActiveBorder = lipgloss.NormalBorder()
	s.body = pane.New(paneOpts)
}

func (s *Screen) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(s.filter.Value()))
	if q == "" {
		s.table.SetRows(allRows)
		return
	}
	out := make([]table.Row, 0, len(allRows))
	for _, r := range allRows {
		if rowMatches(r, q) {
			out = append(out, r)
		}
	}
	s.table.SetRows(out)
	if s.table.Cursor() >= len(out) {
		s.table.SetCursor(max(0, len(out)-1))
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

func (s *Screen) renderBody(w, h int) string {
	s.table.SetWidth(max(0, w-2-pane.ScrollbarWidth))
	s.table.SetHeight(max(0, h-2-2))
	s.body.SetDimensions(w, h)
	s.body.SetContent(s.table.View())
	s.body.SetBottomLeft(fmt.Sprintf("%d / %d", len(s.table.Rows()), len(allRows)))
	return s.body.View()
}
