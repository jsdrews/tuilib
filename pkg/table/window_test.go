package table

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// windowRows builds n rows labelled by their logical index, so a rendered
// row proves which logical index it was drawn for.
func windowRows(start, n int) []Row {
	out := make([]Row, n)
	for i := range out {
		out[i] = Row{"row-" + itoa(start+i), "resident"}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func newWindowed(t *testing.T, opts ...func(*Options)) Model {
	t.Helper()
	o := Options{
		Columns: []Column{
			{Title: "Name", Width: 12, Sortable: true},
			{Title: "State", Width: 10},
		},
	}
	for _, f := range opts {
		f(&o)
	}
	m := New(o)
	m.SetRect(geom.New(0, 0, 40, 14))
	return m
}

func bodyText(m Model) string { return ansi.Strip(m.View()) }

func TestWindowRowCountIsLogicalTotal(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	if got := m.rowCount(); got != 1000 {
		t.Errorf("rowCount = %d, want the logical total 1000, not the 20 resident", got)
	}
}

func TestWindowCursorReachesBeyondResident(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.Cursor() != 999 {
		t.Errorf("cursor after G = %d, want 999 — the window must not cap the cursor", m.Cursor())
	}
}

func TestWindowUnresidentRowsRenderPlaceholder(t *testing.T) {
	m := newWindowed(t)
	// Window covers 0..19; scroll far past it.
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m.SetCursor(500)
	out := bodyText(m)
	if strings.Contains(out, "row-500") {
		t.Fatal("row 500 is outside the window; it must not render as data")
	}
	if !strings.Contains(out, "·") {
		t.Errorf("unresident rows should render the placeholder glyph:\n%s", out)
	}
}

func TestWindowResidentRowsRenderData(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(490, 20), 490, 1000)
	m.SetCursor(500)
	out := bodyText(m)
	if !strings.Contains(out, "row-500") {
		t.Errorf("resident row 500 should render its data:\n%s", out)
	}
	if strings.Contains(out, "·") {
		t.Errorf("every visible row is resident; no placeholder expected:\n%s", out)
	}
}

func TestWindowSelectedRejectsUnresident(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m.SetCursor(500)
	if row, ok := m.Selected(); ok {
		t.Errorf("Selected() = %v, ok — a screen must not act on a row it hasn't received", row)
	}
	m.SetCursor(5)
	row, ok := m.Selected()
	if !ok || row[0] != "row-5" {
		t.Errorf("Selected() = %v, %v, want the resident row", row, ok)
	}
}

func TestWindowCursorSurvivesWindowArrival(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m.SetCursor(500)
	if _, ok := m.Selected(); ok {
		t.Fatal("precondition: row 500 should not be resident yet")
	}
	m.SetWindow(windowRows(490, 20), 490, 1000)
	if m.Cursor() != 500 {
		t.Fatalf("cursor moved to %d when the window landed, want 500", m.Cursor())
	}
	row, ok := m.Selected()
	if !ok || row[0] != "row-500" {
		t.Errorf("Selected() = %v, %v after the window landed", row, ok)
	}
}

func TestWindowUnknownTotalGrowsWithLoad(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, -1)
	if got := m.rowCount(); got != 20 {
		t.Errorf("rowCount = %d, want 20 — the end of what has loaded", got)
	}
	m.SetWindow(windowRows(20, 20), 20, -1)
	if got := m.rowCount(); got != 40 {
		t.Errorf("rowCount = %d, want 40 after a second page", got)
	}
}

func TestWindowUnknownTotalCounterMarkedApproximate(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, -1)
	if got := m.totalLabel(); got != "20+" {
		t.Errorf("totalLabel = %q, want %q — an unknown total is a floor", got, "20+")
	}
	m.SetWindow(windowRows(0, 20), 0, 1000)
	if got := m.totalLabel(); got != "1000" {
		t.Errorf("totalLabel = %q, want %q", got, "1000")
	}
}

func TestWindowCounterShowsLogicalTotal(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(490, 20), 490, 1000)
	m.SetCursor(500)
	if out := bodyText(m); !strings.Contains(out, "501 / 1000") {
		t.Errorf("counter should read against the logical total:\n%s", out)
	}
}

func TestWindowAccessor(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(100, 25), 100, 1000)
	off, count, total := m.Window()
	if off != 100 || count != 25 || total != 1000 {
		t.Errorf("Window() = (%d, %d, %d), want (100, 25, 1000)", off, count, total)
	}
}

func TestWindowAccessorOnPlainTable(t *testing.T) {
	m := newWindowed(t)
	m.SetRows(windowRows(0, 7))
	off, count, total := m.Window()
	if off != 0 || count != 7 || total != 7 {
		t.Errorf("Window() = (%d, %d, %d), want (0, 7, 7) for a non-windowed table", off, count, total)
	}
}

func TestSetRowsClearsWindow(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(490, 20), 490, 1000)
	m.SetRows(windowRows(0, 3))
	if m.rowCount() != 3 {
		t.Errorf("rowCount = %d after SetRows, want 3 — the window offset must not survive", m.rowCount())
	}
	if _, ok := m.Selected(); !ok {
		t.Error("cursor should land on a real row after leaving windowed mode")
	}
}

func TestSetKeyedRowsClearsWindow(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(490, 20), 490, 1000)
	m.SetKeyedRows([]KeyedRow{{Key: "a", Cells: []string{"A", "x"}}})
	if m.rowCount() != 1 {
		t.Errorf("rowCount = %d, want 1", m.rowCount())
	}
}

func TestWindowViewportReportsLogicalTotal(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	_, _, total, valid := m.viewport()
	if !valid {
		t.Fatal("viewport should be valid")
	}
	if total != 1000 {
		t.Errorf("ViewportChangedMsg.TotalRows would be %d, want the logical 1000", total)
	}
}

func TestWindowNeverFiltersLocally(t *testing.T) {
	m := newWindowed(t, func(o *Options) { o.Filterable = true })
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m.SetValue("row-3")
	if m.rowCount() != 1000 {
		t.Errorf("rowCount = %d — filtering one page of a larger set is not filtering", m.rowCount())
	}
	if got := len(m.Visible()); got != 20 {
		t.Errorf("Visible() = %d rows, want all 20 resident — a window is the source's answer, not a set to narrow", got)
	}
}

func TestWindowNeverSortsLocally(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow([]Row{{"charlie", "x"}, {"alpha", "x"}, {"bravo", "x"}}, 0, 3)
	m.SetSort(0, false)
	row, _ := m.Selected()
	if row[0] != "charlie" {
		t.Errorf("first row = %q, want the source's order preserved", row[0])
	}
	if got := m.Visible()[0][0]; got != "charlie" {
		t.Errorf("Visible()[0] = %q, want the source's order preserved there too", got)
	}
}

func TestWindowSelectedIndexIsAbsolute(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(490, 20), 490, 1000)
	m.SetCursor(500)
	idx, ok := m.SelectedIndex()
	if !ok || idx != 500 {
		t.Errorf("SelectedIndex() = %d, %v, want the logical index 500", idx, ok)
	}
}

func TestWindowDoubleClickOnUnresidentDoesNotActivate(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 2), 0, 1000)
	// Row 3 is inside the logical range but outside the window.
	m, cmd := m.Update(press(2, 1+m.headerRows()+3, 2))
	if m.Cursor() != 3 {
		t.Fatalf("cursor = %d, want the clicked row 3", m.Cursor())
	}
	if a := drainActivatedMsg(cmd); a != nil {
		t.Errorf("double-clicking an unreceived row emitted %+v", *a)
	}
}

func TestWindowDoubleClickOnResidentActivates(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m, cmd := m.Update(press(2, 1+m.headerRows()+3, 2))
	a := drainActivatedMsg(cmd)
	if a == nil {
		t.Fatal("double-clicking a resident row should activate it")
	}
	if a.Cells[0] != "row-3" {
		t.Errorf("activated cells = %v", a.Cells)
	}
}

func TestWindowFocusMsgEmptyOnUnresident(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m.SetCursor(5)
	// Emit the first focus, so the Empty transition below is a real
	// transition rather than the initial-state suppression.
	if m.flushMsgs() == nil {
		t.Fatal("landing on a resident row should emit a focus msg")
	}
	m.SetCursor(500)
	if !m.focusPending {
		t.Fatal("moving onto an unreceived row should mark a focus change")
	}
	if m.focusIdx != -1 {
		t.Errorf("focusIdx = %d, want -1 (Empty) — a placeholder is not a focused row", m.focusIdx)
	}
}

func TestWindowPlaceholderGlyphConfigurable(t *testing.T) {
	m := newWindowed(t, func(o *Options) { o.Placeholder = "~" })
	m.SetWindow(windowRows(0, 2), 0, 1000)
	m.SetCursor(500)
	if out := bodyText(m); !strings.Contains(out, "~") {
		t.Errorf("custom placeholder not rendered:\n%s", out)
	}
}

func TestWindowCursorClampsWhenTotalShrinks(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 20), 0, 1000)
	m.SetCursor(900)
	m.SetWindow(windowRows(0, 5), 0, 5)
	if m.Cursor() != 4 {
		t.Errorf("cursor = %d after the set shrank to 5, want 4", m.Cursor())
	}
}

func TestSetColumnsRebuildsPlaceholder(t *testing.T) {
	m := newWindowed(t)
	m.SetWindow(windowRows(0, 2), 0, 100)
	m.SetColumns([]Column{
		{Title: "A", Width: 6}, {Title: "B", Width: 6}, {Title: "C", Width: 6},
	})
	if len(m.placeholder) != 3 {
		t.Errorf("placeholder has %d cells, want one per column", len(m.placeholder))
	}
}

// drainActivatedMsg mirrors drainViewportMsg for ActivatedMsg.
func drainActivatedMsg(cmd tea.Cmd) *ActivatedMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if a, ok := msg.(ActivatedMsg); ok {
		return &a
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if a := drainActivatedMsg(sub); a != nil {
				return a
			}
		}
	}
	return nil
}
