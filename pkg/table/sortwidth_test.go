package table

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func sortTable(cols []Column, rows []Row) Model {
	m := New(Options{
		Columns: cols, Rows: rows,
		HeaderStyle: lipgloss.NewStyle(), SelectedStyle: lipgloss.NewStyle(),
	})
	m.SetRect(geom.New(0, 0, 80, 10))
	return m
}

// A column sized exactly to its title, in either sizing mode, must still show
// the caret once it is the sort column.
func TestSortCaretIsNeverClipped(t *testing.T) {
	for _, tc := range []struct {
		name string
		col  Column
	}{
		{"explicit width", Column{Title: "Region", Width: 6, Sortable: true}},
		{"content-auto", Column{Title: "Region", Sortable: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sortTable([]Column{tc.col, {Title: "Other", Width: 8}}, []Row{{"eu", "x"}})
			if got, want := m.widths[0], 6+sortCaretW; got != want {
				t.Errorf("width %d, want %d", got, want)
			}
			m.SetSort(0, false)
			header := strings.Split(ansi.Strip(m.View()), "\n")[1]
			if !strings.Contains(header, "Region "+m.glyphs.SortAsc) {
				t.Errorf("caret clipped: %q", header)
			}
		})
	}
}

func TestUnsortableColumnKeepsItsWidth(t *testing.T) {
	m := sortTable([]Column{{Title: "Region", Width: 6}}, nil)
	if m.widths[0] != 6 {
		t.Errorf("width %d, want 6", m.widths[0])
	}
}

func TestSortStepSkipsHiddenColumns(t *testing.T) {
	m := sortTable([]Column{
		{Title: "A", Sortable: true},
		{Title: "B", Sortable: true, Hidden: true},
		{Title: "C", Sortable: true},
	}, []Row{{"1", "2", "3"}})
	next := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")}
	var got []int
	for range 4 {
		m, _ = m.Update(next)
		got = append(got, m.SortColumn())
	}
	for _, c := range got {
		if c == 1 {
			t.Fatalf("] stepped onto the hidden column: %v", got)
		}
	}
	if !m.hasSortable() {
		t.Error("hasSortable false with visible sortable columns")
	}
	only := sortTable([]Column{{Title: "A"}, {Title: "B", Sortable: true, Hidden: true}}, nil)
	if only.hasSortable() {
		t.Error("a table whose only sortable column is hidden advertises sort keys")
	}
}
