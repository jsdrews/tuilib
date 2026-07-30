package alert

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

var alertRect = geom.Rect{X: 8, Y: 3, W: 40, H: 7}

// Inner content is message, blank, OK — so the button is on the third line.
const okY = 3 + 1 + 2 // rect.Y + border + two lines above

func newAlert(t *testing.T) Model {
	t.Helper()
	m := New(Options{Message: "Something failed", OK: "OK"})
	m.SetRect(geom.New(alertRect.X, alertRect.Y, alertRect.W, alertRect.H))
	return m
}

func pressAt(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

func wheelAt(x, y int, up bool) mouse.Msg {
	btn := tea.MouseButtonWheelDown
	if up {
		btn = tea.MouseButtonWheelUp
	}
	return mouse.Msg{MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: btn}}
}

func msgOf(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestClickOKDismisses(t *testing.T) {
	m := newAlert(t)

	_, cmd := m.Update(pressAt(alertRect.X+2, okY))

	if _, ok := msgOf(cmd).(DismissedMsg); !ok {
		t.Errorf("clicking OK gave %T, want DismissedMsg", msgOf(cmd))
	}
}

func TestClickPastOKDoesNothing(t *testing.T) {
	m := newAlert(t)

	// "[ OK ]" is 6 cells; anything beyond is empty modal body.
	_, cmd := m.Update(pressAt(alertRect.X+1+10, okY))

	if cmd != nil {
		t.Errorf("clicking past the OK button gave %T", msgOf(cmd))
	}
}

func TestClickMessageDoesNotDismiss(t *testing.T) {
	m := newAlert(t)

	_, cmd := m.Update(pressAt(alertRect.X+3, alertRect.Y+1))

	if cmd != nil {
		t.Errorf("clicking the message gave %T", msgOf(cmd))
	}
}

func TestClickOutsideIsIgnored(t *testing.T) {
	m := newAlert(t)

	_, cmd := m.Update(pressAt(alertRect.X+300, okY))

	if cmd != nil {
		t.Errorf("clicking outside the modal gave %T", msgOf(cmd))
	}
}

// In autosize mode the message region scrolls and the OK button stays pinned
// to the last inner row, so both must remain reachable.
func TestAutosizeWheelScrollsMessage(t *testing.T) {
	long := strings.Repeat("a wrapped line of error text\n", 60)
	m := New(Options{Message: long, Autosize: true})
	m.SetRect(geom.New(0, 0, 80, 24))

	if !m.canScroll() {
		t.Fatalf("setup: a 60-line message at 24 rows should overflow")
	}
	before := m.scrollOffset

	inner := m.pane.ContentRect()
	m, _ = m.Update(wheelAt(inner.X+2, inner.Y+1, false))

	if m.scrollOffset <= before {
		t.Errorf("wheel down did not scroll the message: %d → %d", before, m.scrollOffset)
	}
}

func TestAutosizeOKStaysClickableAfterScrolling(t *testing.T) {
	long := strings.Repeat("a wrapped line of error text\n", 60)
	m := New(Options{Message: long, Autosize: true})
	m.SetRect(geom.New(0, 0, 80, 24))

	inner := m.pane.ContentRect()
	for range 5 {
		m, _ = m.Update(wheelAt(inner.X+2, inner.Y+1, false))
	}

	// The button is pinned to the last inner row regardless of scroll.
	_, cmd := m.Update(pressAt(inner.X, inner.Y+inner.H-1))

	if _, ok := msgOf(cmd).(DismissedMsg); !ok {
		t.Errorf("OK was not clickable after scrolling; got %T", msgOf(cmd))
	}
}

func TestStaleRectDeclinesClicks(t *testing.T) {
	m := newAlert(t)
	geom.NextGen()

	_, cmd := m.Update(pressAt(alertRect.X+2, okY))

	if cmd != nil {
		t.Errorf("an alert with a stale rect responded to a click")
	}
}
