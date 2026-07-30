package textview

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

var tvRect = geom.Rect{X: 3, Y: 2, W: 40, H: 8}

func newTextview(t *testing.T, lines int) Model {
	t.Helper()
	body := make([]string, lines)
	for i := range body {
		body[i] = fmt.Sprintf("line %d", i)
	}
	m := New(Options{Content: strings.Join(body, "\n")})
	m.SetRect(geom.New(tvRect.X, tvRect.Y, tvRect.W, tvRect.H))
	return m
}

func wheelMsg(x, y int, up bool) mouse.Msg {
	btn := tea.MouseButtonWheelDown
	if up {
		btn = tea.MouseButtonWheelUp
	}
	return mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: btn}}
}

func pressMsg(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

func TestWheelDownScrollsViewport(t *testing.T) {
	m := newTextview(t, 100)
	before := m.body.YOffset()

	m, _ = m.Update(wheelMsg(tvRect.X+2, tvRect.Y+2, false))

	if m.body.YOffset() <= before {
		t.Errorf("wheel down did not scroll: offset %d → %d", before, m.body.YOffset())
	}
}

func TestWheelUpFromTopStaysAtTop(t *testing.T) {
	m := newTextview(t, 100)

	m, _ = m.Update(wheelMsg(tvRect.X+2, tvRect.Y+2, true))

	if m.body.YOffset() != 0 {
		t.Errorf("wheel up at the top scrolled to %d, want 0", m.body.YOffset())
	}
}

func TestWheelOutsideRectIsDeclined(t *testing.T) {
	m := newTextview(t, 100)
	before := m.body.YOffset()

	m, _ = m.Update(wheelMsg(tvRect.X+200, tvRect.Y+2, false))

	if m.body.YOffset() != before {
		t.Errorf("wheel outside the rect scrolled the viewport")
	}
}

func TestPressRequestsFocus(t *testing.T) {
	m := newTextview(t, 10)

	_, cmd := m.Update(pressMsg(tvRect.X+2, tvRect.Y+2))

	if cmd == nil {
		t.Errorf("press inside the textview did not request focus")
	}
}

func TestStaleRectDeclinesWheel(t *testing.T) {
	m := newTextview(t, 100)
	geom.NextGen()
	before := m.body.YOffset()

	m, _ = m.Update(wheelMsg(tvRect.X+2, tvRect.Y+2, false))

	if m.body.YOffset() != before {
		t.Errorf("a textview with a stale rect scrolled on a wheel event")
	}
}
