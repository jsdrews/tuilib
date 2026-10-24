package table

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRefusedColumnLeavesTheSortCycle(t *testing.T) {
	m := sortTable([]Column{
		{Title: "A", Width: 6, Sortable: true},
		{Title: "B", Width: 6, Sortable: true},
	}, []Row{{"1", "2"}})
	m.RefuseSort(0)
	if !m.SortUnavailable(0) || m.SortUnavailable(1) {
		t.Fatal("SortUnavailable")
	}
	press := func(s string) { m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	press("]")
	press("]")
	if m.SortColumn() != 1 {
		t.Errorf("] landed on %d, want 1 every time", m.SortColumn())
	}
	if m.clickHeader(m.body.ContentRect().X + 1); m.SortColumn() != 1 {
		t.Errorf("header click on the refused column sorted by it")
	}

	st := m.State()
	n := sortTable(m.cols, nil)
	n.Restore(st)
	if !n.SortUnavailable(0) {
		t.Error("Restore dropped the refused column")
	}
}

// A direction-only refusal leaves the rows sorted by the column; s must not
// then flip it to the direction just refused.
func TestSortDirIgnoresARefusedColumn(t *testing.T) {
	m := sortTable([]Column{{Title: "A", Width: 6, Sortable: true}}, []Row{{"1"}})
	m.SetSort(0, false)
	m.rq.unavailable = map[int]bool{0: true}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.SortDescending() {
		t.Error("s flipped a refused column")
	}
}
