package inspector

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

var insRect = geom.Rect{X: 2, Y: 1, W: 50, H: 10}

const insFirstRowY = 2 // insRect.Y + 1 (border)

func newInspector(t *testing.T) Model {
	t.Helper()
	m := New(Options{
		Fields: []Field{
			{Label: "name", Value: "web-1"},
			{Label: "spec", Children: []Field{
				{Label: "image", Value: "nginx"},
				{Label: "port", Value: "80"},
			}},
			{Label: "phase", Value: "Running"},
		},
	})
	m.SetRect(geom.New(insRect.X, insRect.Y, insRect.W, insRect.H))
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

func TestClickMovesCursorToRow(t *testing.T) {
	m := newInspector(t)

	m, _ = m.Update(press(insRect.X+10, insFirstRowY+2, 1))

	if m.Cursor() != 2 {
		t.Errorf("Cursor() = %d, want 2", m.Cursor())
	}
}

// The ▸/▾ glyph of a top-level field sits at content column 0.
func TestClickOnGlyphExpandsField(t *testing.T) {
	m := newInspector(t)
	before := len(m.rows)

	m, _ = m.Update(press(insRect.X+1, insFirstRowY+1, 1))

	if len(m.rows) <= before {
		t.Errorf("clicking the glyph did not expand (%d rows before, %d after)", before, len(m.rows))
	}
}

func TestClickOnValueSelectsWithoutExpanding(t *testing.T) {
	m := newInspector(t)
	before := len(m.rows)

	m, _ = m.Update(press(insRect.X+20, insFirstRowY+1, 1))

	if len(m.rows) != before {
		t.Errorf("clicking the value expanded the field")
	}
	if m.Cursor() != 1 {
		t.Errorf("Cursor() = %d, want 1", m.Cursor())
	}
}

func TestDoubleClickExpandsField(t *testing.T) {
	m := newInspector(t)
	before := len(m.rows)

	m, _ = m.Update(press(insRect.X+20, insFirstRowY+1, 2))

	if len(m.rows) <= before {
		t.Errorf("double click did not expand (%d rows before, %d after)", before, len(m.rows))
	}
}

func TestWheelMovesCursor(t *testing.T) {
	m := newInspector(t)

	m, _ = m.Update(wheel(insRect.X+10, insFirstRowY, false))
	if m.Cursor() != 1 {
		t.Errorf("after wheel down, Cursor() = %d, want 1", m.Cursor())
	}
	m, _ = m.Update(wheel(insRect.X+10, insFirstRowY, true))
	if m.Cursor() != 0 {
		t.Errorf("after wheel up, Cursor() = %d, want 0", m.Cursor())
	}
}

func TestClickOutsideIsDeclined(t *testing.T) {
	m := newInspector(t)

	_, cmd := m.Update(press(insRect.X+400, insFirstRowY, 1))

	if cmd != nil {
		t.Errorf("click outside the inspector produced a command")
	}
}

func TestStaleRectDeclinesClicks(t *testing.T) {
	m := newInspector(t)
	geom.NextGen()

	before := m.Cursor()
	m, cmd := m.Update(press(insRect.X+10, insFirstRowY+2, 1))

	if m.Cursor() != before || cmd != nil {
		t.Errorf("an inspector with a stale rect responded to a click")
	}
}

// The inspector windows its own rows, so a click after scrolling must map
// through viewStart rather than resolving to the raw content line.
func TestClickAccountsForWindowStart(t *testing.T) {
	fields := make([]Field, 40)
	for i := range fields {
		fields[i] = Field{Label: "f", Value: "v"}
	}
	m := New(Options{Fields: fields})
	m.SetRect(geom.New(insRect.X, insRect.Y, insRect.W, insRect.H))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	bottom := m.Cursor()
	if bottom != len(fields)-1 {
		t.Fatalf("setup: G left cursor at %d, want %d", bottom, len(fields)-1)
	}

	m, _ = m.Update(press(insRect.X+10, insFirstRowY, 1))

	if m.Cursor() == 0 {
		t.Errorf("click resolved to row 0; the window start was ignored")
	}
	if m.Cursor() > bottom {
		t.Errorf("click resolved to %d, past the last visible row", m.Cursor())
	}
}
