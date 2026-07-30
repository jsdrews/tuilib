package logview

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

var lvRect = geom.Rect{X: 1, Y: 1, W: 40, H: 8}

func newLogview(t *testing.T, lines int) Model {
	t.Helper()
	m := New(Options{})
	m.SetRect(geom.New(lvRect.X, lvRect.Y, lvRect.W, lvRect.H))
	for i := range lines {
		m.Append(fmt.Sprintf("line %d", i))
	}
	return m
}

func wheel(x, y int, up bool) mouse.Msg {
	btn := tea.MouseButtonWheelDown
	if up {
		btn = tea.MouseButtonWheelUp
	}
	return mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: btn}}
}

func press(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

func TestWheelScrollsViewport(t *testing.T) {
	m := newLogview(t, 100)
	before := m.body.YOffset()

	m, _ = m.Update(wheel(lvRect.X+2, lvRect.Y+2, true))

	if m.body.YOffset() >= before {
		t.Errorf("wheel up did not scroll: offset %d → %d", before, m.body.YOffset())
	}
}

// Scrolling up leaves the bottom, so auto-follow must disengage — exactly as
// it does when the user scrolls with the keyboard.
func TestWheelUpDisengagesFollow(t *testing.T) {
	m := newLogview(t, 100)
	if !m.Following() {
		t.Fatalf("setup: a fresh logview should be following")
	}

	m, _ = m.Update(wheel(lvRect.X+2, lvRect.Y+2, true))

	if m.Following() {
		t.Errorf("scrolling up with the wheel left auto-follow engaged")
	}
}

func TestWheelBackToBottomReengagesFollow(t *testing.T) {
	m := newLogview(t, 100)
	m, _ = m.Update(wheel(lvRect.X+2, lvRect.Y+2, true))
	if m.Following() {
		t.Fatalf("setup: follow should be off after scrolling up")
	}

	for range 20 {
		m, _ = m.Update(wheel(lvRect.X+2, lvRect.Y+2, false))
	}

	if !m.Following() {
		t.Errorf("scrolling back to the bottom did not re-engage follow")
	}
}

func TestWheelOutsideRectIsDeclined(t *testing.T) {
	m := newLogview(t, 100)
	before := m.body.YOffset()

	m, _ = m.Update(wheel(lvRect.X+200, lvRect.Y+2, true))

	if m.body.YOffset() != before {
		t.Errorf("wheel outside the rect scrolled the viewport")
	}
}

func TestPressRequestsFocus(t *testing.T) {
	m := newLogview(t, 10)

	_, cmd := m.Update(press(lvRect.X+2, lvRect.Y+2))

	if cmd == nil {
		t.Errorf("press inside the logview did not request focus")
	}
}

func TestStaleRectDeclinesWheel(t *testing.T) {
	m := newLogview(t, 100)
	geom.NextGen()
	before := m.body.YOffset()

	m, _ = m.Update(wheel(lvRect.X+2, lvRect.Y+2, true))

	if m.body.YOffset() != before {
		t.Errorf("a logview with a stale rect scrolled on a wheel event")
	}
}
