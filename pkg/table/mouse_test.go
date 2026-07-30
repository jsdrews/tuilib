package table

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

// The table is placed here in every test. Content starts one row down and
// one column right (the pane border); content line 0 is the pinned header,
// so the first data row sits at firstDataY.
var tblRect = geom.Rect{X: 4, Y: 3, W: 40, H: 12}

const (
	headerY    = 4 // tblRect.Y + 1 (border)
	firstDataY = 5 // header occupies one content line
)

func newTable(t *testing.T, rows ...Row) Model {
	t.Helper()
	m := New(Options{
		Columns: []Column{
			{Title: "Name", Width: 10, Sortable: true},
			{Title: "Region", Width: 10},
			{Title: "Pop", Width: 8, Sortable: true},
		},
		Rows: rows,
	})
	m.SetRect(geom.New(tblRect.X, tblRect.Y, tblRect.W, tblRect.H))
	return m
}

func press(x, y, clicks int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   clicks,
	}
}

func wheel(x, y int, up bool) mouse.Msg {
	btn := tea.MouseButtonWheelDown
	if up {
		btn = tea.MouseButtonWheelUp
	}
	return mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: btn}}
}

func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collect(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func sampleRows() []Row {
	return []Row{
		{"alpha", "eu", "10"},
		{"bravo", "us", "20"},
		{"charlie", "ap", "30"},
		{"delta", "eu", "40"},
	}
}

// The header is pinned at content line 0, so a click on the first data row
// must resolve to row 0 — not row 1.
func TestClickSkipsPinnedHeader(t *testing.T) {
	m := newTable(t, sampleRows()...)

	m, _ = m.Update(press(tblRect.X+2, firstDataY, 1))

	if m.Cursor() != 0 {
		t.Errorf("Cursor() = %d, want 0 — the header row was counted as data", m.Cursor())
	}
}

func TestClickResolvesNthDataRow(t *testing.T) {
	m := newTable(t, sampleRows()...)

	m, _ = m.Update(press(tblRect.X+2, firstDataY+2, 1))

	if m.Cursor() != 2 {
		t.Errorf("Cursor() = %d, want 2", m.Cursor())
	}
}

func TestDoubleClickActivatesRow(t *testing.T) {
	m := newTable(t, sampleRows()...)

	_, cmd := m.Update(press(tblRect.X+2, firstDataY+1, 2))

	for _, msg := range collect(cmd) {
		if a, ok := msg.(ActivatedMsg); ok {
			if a.Row != 1 || a.Cells[0] != "bravo" {
				t.Errorf("ActivatedMsg = %+v, want row 1 (bravo)", a)
			}
			return
		}
	}
	t.Errorf("double click emitted no ActivatedMsg")
}

// Clicking a Sortable header sorts by it; clicking again flips direction.
func TestHeaderClickSortsAndTogglesDirection(t *testing.T) {
	m := newTable(t, sampleRows()...)

	m, _ = m.Update(press(tblRect.X+2, headerY, 1)) // "Name" starts at column 0
	if m.SortColumn() != 0 {
		t.Fatalf("SortColumn() = %d, want 0 after clicking the Name header", m.SortColumn())
	}
	if m.SortDescending() {
		t.Errorf("first click on a header should sort ascending")
	}

	m, _ = m.Update(press(tblRect.X+2, headerY, 1))
	if !m.SortDescending() {
		t.Errorf("second click on the active sort column should flip direction")
	}
}

func TestHeaderClickOnNonSortableColumnIsIgnored(t *testing.T) {
	m := newTable(t, sampleRows()...)

	// "Region" is the second column: 10 cells of "Name" plus a separator.
	m, _ = m.Update(press(tblRect.X+1+12, headerY, 1))

	if m.SortColumn() != -1 {
		t.Errorf("SortColumn() = %d, want -1 — Region is not Sortable", m.SortColumn())
	}
}

func TestHeaderClickDoesNotMoveCursor(t *testing.T) {
	m := newTable(t, sampleRows()...)
	m, _ = m.Update(press(tblRect.X+2, firstDataY+2, 1))
	before := m.Cursor()

	m, _ = m.Update(press(tblRect.X+2, headerY, 1))

	if m.Cursor() != before {
		t.Errorf("clicking the header moved the cursor from %d to %d", before, m.Cursor())
	}
}

func TestWheelMovesCursor(t *testing.T) {
	m := newTable(t, sampleRows()...)

	m, _ = m.Update(wheel(tblRect.X+2, firstDataY, false))
	if m.Cursor() != 1 {
		t.Errorf("after wheel down, Cursor() = %d, want 1", m.Cursor())
	}
	m, _ = m.Update(wheel(tblRect.X+2, firstDataY, true))
	if m.Cursor() != 0 {
		t.Errorf("after wheel up, Cursor() = %d, want 0", m.Cursor())
	}
}

func TestClickOutsideIsDeclined(t *testing.T) {
	m := newTable(t, sampleRows()...)

	_, cmd := m.Update(press(tblRect.X+200, firstDataY, 1))

	if cmd != nil {
		t.Errorf("click outside the table produced a command")
	}
}

func TestStaleRectDeclinesClicks(t *testing.T) {
	m := newTable(t, sampleRows()...)
	geom.NextGen()

	before := m.Cursor()
	m, cmd := m.Update(press(tblRect.X+2, firstDataY+2, 1))

	if m.Cursor() != before || cmd != nil {
		t.Errorf("a table with a stale rect responded to a click")
	}
}

// A click below the last row must not select a phantom row.
func TestClickBelowLastRowSelectsNothingNew(t *testing.T) {
	m := newTable(t, sampleRows()...)
	m, _ = m.Update(press(tblRect.X+2, firstDataY+1, 1))
	before := m.Cursor()

	m, _ = m.Update(press(tblRect.X+2, firstDataY+8, 1))

	if m.Cursor() != before {
		t.Errorf("click past the last row moved the cursor to %d", m.Cursor())
	}
}
