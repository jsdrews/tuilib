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

	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Status values use ansi.CellColor so the foreground-only \x1b[39m reset
// keeps the selected-row background intact across colored cells.
var (
	statusOK    = ansi.CellColor(2, "✓ healthy")  // green
	statusWarn  = ansi.CellColor(3, "● degraded") // yellow
	statusError = ansi.CellColor(1, "✗ down")     // red
)

var allRows = []table.Row{
	{"New York", "North America", "8.3M", statusOK},
	{"San Francisco", "North America", "0.87M", statusOK},
	{"Toronto", "North America", "2.8M", statusWarn},
	{"Vancouver", "North America", "0.67M", statusOK},
	{"Chicago", "North America", "2.7M", statusOK},
	{"Mexico City", "North America", "9.2M", statusError},
	{"London", "Europe", "8.9M", statusOK},
	{"Berlin", "Europe", "3.7M", statusOK},
	{"Paris", "Europe", "2.1M", statusWarn},
	{"Madrid", "Europe", "3.3M", statusOK},
	{"Amsterdam", "Europe", "0.9M", statusOK},
	{"Lisbon", "Europe", "0.55M", statusOK},
	{"Prague", "Europe", "1.3M", statusWarn},
	{"Tokyo", "Asia", "13.9M", statusOK},
	{"Singapore", "Asia", "5.7M", statusOK},
	{"Seoul", "Asia", "9.7M", statusOK},
	{"Mumbai", "Asia", "20.4M", statusError},
	{"Bangkok", "Asia", "10.7M", statusWarn},
	{"Hong Kong", "Asia", "7.5M", statusOK},
	{"Taipei", "Asia", "2.6M", statusOK},
	{"Sydney", "Oceania", "5.3M", statusOK},
	{"Melbourne", "Oceania", "5.1M", statusOK},
	{"Auckland", "Oceania", "1.7M", statusOK},
	{"São Paulo", "South America", "12.3M", statusOK},
	{"Buenos Aires", "South America", "3.1M", statusWarn},
	{"Lima", "South America", "10.7M", statusOK},
	{"Bogotá", "South America", "7.9M", statusOK},
	{"Nairobi", "Africa", "4.4M", statusError},
	{"Cape Town", "Africa", "4.6M", statusOK},
	{"Lagos", "Africa", "15.4M", statusWarn},
	{"Cairo", "Africa", "9.5M", statusOK},
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
			// Status column is sized generously: bubbles/table's truncation
			// is not ANSI-aware, so colored cells need to fit comfortably.
			// Status column needs ~visible_chars + 8 width budget when using
			// 8/16-color CellColor (4 bytes open + 4 bytes close that
			// bubbles/table's runewidth.Truncate counts as visible). Longest
			// status here is "● degraded" (10 visible) → 18 minimum. We use
			// 22 for breathing room.
			{Title: "Status", Width: 22},
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
