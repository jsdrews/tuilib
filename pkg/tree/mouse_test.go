package tree

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

// mouseNode is a minimal Node for exercising mouse geometry.
type mouseNode struct {
	label string
	kids  []Node
}

func (n mouseNode) Label() string    { return n.label }
func (n mouseNode) Children() []Node { return n.kids }

var treeRect = geom.Rect{X: 6, Y: 2, W: 40, H: 12}

const treeFirstRowY = 3 // treeRect.Y + 1 (border)

// newMouseTree builds root → (branch → leaf1, leaf2), leaf3, pre-expanded
// one level so the root's children are visible rows: row 0 is root, row 1 is
// branch (collapsed, depth 1), row 2 is leaf3.
func newMouseTree(t *testing.T) Model {
	t.Helper()
	root := mouseNode{label: "root", kids: []Node{
		mouseNode{label: "branch", kids: []Node{
			mouseNode{label: "leaf1"},
			mouseNode{label: "leaf2"},
		}},
		mouseNode{label: "leaf3"},
	}}
	m := New(Options{Root: root, InitialDepth: 1})
	m.SetRect(geom.New(treeRect.X, treeRect.Y, treeRect.W, treeRect.H))
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

// rowCount is how many rows are currently visible.
func rowCount(m Model) int { return len(m.rows) }

func TestClickMovesCursorToRow(t *testing.T) {
	m := newMouseTree(t)

	m, _ = m.Update(press(treeRect.X+8, treeFirstRowY+1, 1))

	if m.Cursor() != 1 {
		t.Errorf("Cursor() = %d, want 1", m.Cursor())
	}
}

// Clicking the ▸/▾ glyph expands or collapses directly — the glyph is drawn
// to say "this opens", so it should not need a double click.
func TestClickOnGlyphTogglesNode(t *testing.T) {
	m := newMouseTree(t)
	before := rowCount(m)

	// Row 1 is "branch" at depth 1, so its glyph is at content column 2.
	glyphX := treeRect.X + 1 + 2
	m, _ = m.Update(press(glyphX, treeFirstRowY+1, 1))

	if rowCount(m) <= before {
		t.Errorf("clicking the glyph did not expand the node (%d rows before, %d after)", before, rowCount(m))
	}

	m, _ = m.Update(press(glyphX, treeFirstRowY+1, 1))
	if rowCount(m) != before {
		t.Errorf("clicking the glyph again did not collapse back (%d rows, want %d)", rowCount(m), before)
	}
}

// Clicking the label — anywhere off the glyph — selects without toggling.
func TestClickOnLabelSelectsWithoutToggling(t *testing.T) {
	m := newMouseTree(t)
	before := rowCount(m)

	m, _ = m.Update(press(treeRect.X+1+10, treeFirstRowY+1, 1))

	if rowCount(m) != before {
		t.Errorf("clicking the label toggled the node (%d rows, want %d)", rowCount(m), before)
	}
	if m.Cursor() != 1 {
		t.Errorf("Cursor() = %d, want 1", m.Cursor())
	}
}

// Double click is the same verb space carries.
func TestDoubleClickTogglesNode(t *testing.T) {
	m := newMouseTree(t)
	before := rowCount(m)

	m, _ = m.Update(press(treeRect.X+1+10, treeFirstRowY+1, 2))

	if rowCount(m) <= before {
		t.Errorf("double click did not expand the node (%d rows before, %d after)", before, rowCount(m))
	}
}

// A leaf has no glyph, so clicking where one would be just selects it.
func TestClickOnLeafGlyphColumnDoesNotToggle(t *testing.T) {
	m := newMouseTree(t)
	before := rowCount(m)

	// Row 2 is "leaf3" at depth 1 — same column as branch's glyph.
	m, _ = m.Update(press(treeRect.X+1+2, treeFirstRowY+2, 1))

	if rowCount(m) != before {
		t.Errorf("clicking a leaf's glyph column changed the row count")
	}
	if m.Cursor() != 2 {
		t.Errorf("Cursor() = %d, want 2", m.Cursor())
	}
}

func TestWheelMovesCursor(t *testing.T) {
	m := newMouseTree(t)

	m, _ = m.Update(wheel(treeRect.X+8, treeFirstRowY, false))
	if m.Cursor() != 1 {
		t.Errorf("after wheel down, Cursor() = %d, want 1", m.Cursor())
	}
	m, _ = m.Update(wheel(treeRect.X+8, treeFirstRowY, true))
	if m.Cursor() != 0 {
		t.Errorf("after wheel up, Cursor() = %d, want 0", m.Cursor())
	}
}

func TestClickOutsideIsDeclined(t *testing.T) {
	m := newMouseTree(t)

	_, cmd := m.Update(press(treeRect.X+200, treeFirstRowY, 1))

	if cmd != nil {
		t.Errorf("click outside the tree produced a command")
	}
}

func TestStaleRectDeclinesClicks(t *testing.T) {
	m := newMouseTree(t)
	geom.NextGen()

	before := m.Cursor()
	m, cmd := m.Update(press(treeRect.X+8, treeFirstRowY+1, 1))

	if m.Cursor() != before || cmd != nil {
		t.Errorf("a tree with a stale rect responded to a click")
	}
}
