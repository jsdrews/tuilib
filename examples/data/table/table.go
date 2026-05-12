// Package table demonstrates pkg/table — a filterable cursor-driven
// tabular view with a sticky header. Cells use ansi.CellColor for the
// status column so the selected-row background passes through unbroken
// across colored cells.
//
// Column sizing modes shown side-by-side:
//   - City — Flex: 1, MaxWidth: 28. Expands to absorb leftover space
//     up to a cap so it doesn't grow unboundedly on wide terminals.
//     Resize and watch the column stretch up to 28 then stop.
//   - Region — Width: 16, classic fixed width.
//   - Population — Width: 12, fixed width (right-aligned).
//   - Status — Width: 0, content-auto. Sizes to the widest cell value
//     (here "● degraded" with ANSI stripped) plus the title floor.
//
// The Wiki column wraps each city's Wikipedia URL in an OSC 8
// hyperlink via ansi.Hyperlink. The visible label ("wiki ↗") is short
// so it never truncates, but even on a column squeezed to "wi" the
// underlying URL is intact — shift-click in alacritty/tmux/kitty
// launches the full Wikipedia page in the browser.
package table

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Status values use ansi.CellColor (foreground-only \x1b[39m reset) so
// the selected-row Background stays intact across colored cells. pkg/table
// truncates ANSI-aware via x/ansi.Cut, so column Width is the visible cell
// width — no escape-byte budget to add.
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
	t   theme.Theme
	tab table.Model
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Table" }
func (s *Screen) Init() tea.Cmd         { return nil }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.tab.Filtering() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tab, cmd = s.tab.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node {
	return layout.Sized(&s.tab)
}

func (s *Screen) Help() []key.Binding {
	out := s.tab.Help()
	out = append(out,
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	)
	return out
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, value := s.tab.Cursor(), s.tab.Value()
	sortCol, sortDesc := s.tab.SortColumn(), s.tab.SortDescending()
	opts := t.Table()
	opts.Title = "Cities"
	opts.Filterable = true
	opts.Filter.Placeholder = "filter, region:europe, ~^new…"
	opts.Columns = []table.Column{
		// Flex: 1 — City absorbs leftover space. Width: 20 acts as
		// the minimum (never shrinks below "San Francisco");
		// MaxWidth: 28 caps growth so the column doesn't get
		// unwieldy on wide terminals.
		{Title: "City", Width: 20, Sortable: true, Flex: 1, MaxWidth: 28},
		// Fixed.
		{Title: "Region", Width: 16, Sortable: true},
		// Fixed. Population cells are "8.3M" / "20.4M" — sort
		// numerically by parsing the leading float, not lexically
		// (else "8.3M" sorts after "20.4M").
		{Title: "Population", Width: 12, Sortable: true, Less: popLess},
		// Width: 0 → content-auto. Sizes to max(title, longest cell
		// with ANSI stripped). Status sorts by severity (healthy <
		// degraded < down), not by the leading icon glyph. Press s
		// with Status active to flip to "errors first".
		{Title: "Status", Sortable: true, Less: statusLess},
		// OSC 8 hyperlinks — shift-click to launch each city's
		// Wikipedia page. The visible label is short so the cell
		// renders cleanly, but the underlying URL is the full
		// Wikipedia URL; ansi.Cut preserves the OSC envelope across
		// truncation, so even at narrow widths the launched URL is
		// the original.
		{Title: "Wiki", Width: 7},
	}
	opts.Rows = make([]table.Row, len(allRows))
	for i, r := range allRows {
		opts.Rows[i] = append(table.Row{}, r...)
		opts.Rows[i] = append(opts.Rows[i], wikiCell(r[0]))
	}
	s.tab = table.New(opts)
	if value != "" {
		s.tab.SetValue(value)
	}
	s.tab.SetCursor(cursor)
	s.tab.SetSort(sortCol, sortDesc)
}

// wikiCell builds an OSC 8 hyperlinked cell pointing at the city's
// Wikipedia page. Display text is "wiki ↗"; the underlying URL is the
// full Wikipedia URL. ansi.Hyperlink emits an OSC 8 envelope that
// x/ansi.Cut preserves across truncation, so the launched URL survives
// even when the column is squeezed to "wi".
func wikiCell(city string) string {
	slug := strings.ReplaceAll(city, " ", "_")
	url := "https://en.wikipedia.org/wiki/" + slug
	return ansi.Hyperlink(url, "wiki ↗")
}

func popLess(a, b string) bool {
	return parsePop(a) < parsePop(b)
}

func parsePop(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "M")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func statusLess(a, b string) bool {
	return statusRank(a) < statusRank(b)
}

func statusRank(s string) int {
	switch {
	case strings.Contains(s, "down"):
		return 2
	case strings.Contains(s, "degraded"):
		return 1
	case strings.Contains(s, "healthy"):
		return 0
	}
	return -1
}
