package mouse

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func press(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func TestFirstPressIsSingleClick(t *testing.T) {
	tr := NewTracker(500 * time.Millisecond)
	m := tr.Track(press(3, 4), time.Unix(0, 0))
	if m.Clicks != 1 {
		t.Errorf("Clicks = %d, want 1", m.Clicks)
	}
	if m.IsDoubleClick() {
		t.Errorf("first press reported as a double click")
	}
	if !m.IsPress() {
		t.Errorf("left press not reported as a press")
	}
}

func TestSecondPressSameCellInTimeIsDoubleClick(t *testing.T) {
	tr := NewTracker(500 * time.Millisecond)
	base := time.Unix(100, 0)
	tr.Track(press(3, 4), base)
	m := tr.Track(press(3, 4), base.Add(200*time.Millisecond))

	if m.Clicks != 2 {
		t.Errorf("Clicks = %d, want 2", m.Clicks)
	}
	if !m.IsDoubleClick() {
		t.Errorf("second quick press in the same cell should be a double click")
	}
}

func TestSecondPressTooSlowIsSingleClick(t *testing.T) {
	tr := NewTracker(500 * time.Millisecond)
	base := time.Unix(100, 0)
	tr.Track(press(3, 4), base)
	m := tr.Track(press(3, 4), base.Add(900*time.Millisecond))

	if m.Clicks != 1 {
		t.Errorf("Clicks = %d, want 1 — press was outside the interval", m.Clicks)
	}
}

// Clicking one row then quickly clicking a different row is two selections,
// not a double click.
func TestSecondPressDifferentCellIsSingleClick(t *testing.T) {
	tr := NewTracker(500 * time.Millisecond)
	base := time.Unix(100, 0)
	tr.Track(press(3, 4), base)
	m := tr.Track(press(3, 5), base.Add(50*time.Millisecond))

	if m.Clicks != 1 {
		t.Errorf("Clicks = %d, want 1 — press landed in a different cell", m.Clicks)
	}
}

func TestThirdRapidPressResetsToSingle(t *testing.T) {
	tr := NewTracker(500 * time.Millisecond)
	base := time.Unix(100, 0)
	tr.Track(press(3, 4), base)
	tr.Track(press(3, 4), base.Add(100*time.Millisecond))
	m := tr.Track(press(3, 4), base.Add(200*time.Millisecond))

	if m.Clicks != 1 {
		t.Errorf("Clicks = %d, want 1 — a triple click restarts the count", m.Clicks)
	}
}

func TestNonPressEventsCarryNoClicks(t *testing.T) {
	tr := NewTracker(500 * time.Millisecond)
	for _, e := range []tea.MouseMsg{
		{X: 1, Y: 1, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft},
		{X: 1, Y: 1, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone},
		{X: 1, Y: 1, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp},
	} {
		if m := tr.Track(e, time.Unix(0, 0)); m.Clicks != 0 {
			t.Errorf("%v: Clicks = %d, want 0", e.Action, m.Clicks)
		}
	}
}

func TestWheelHelpers(t *testing.T) {
	tr := NewTracker(0)
	up := tr.Track(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}, time.Unix(0, 0))
	down := tr.Track(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}, time.Unix(0, 0))

	if !up.IsWheelUp() || up.IsWheelDown() {
		t.Errorf("wheel-up misreported")
	}
	if !down.IsWheelDown() || down.IsWheelUp() {
		t.Errorf("wheel-down misreported")
	}
	if up.IsPress() {
		t.Errorf("wheel event reported as a button press")
	}
}

func TestZeroIntervalFallsBackToDefault(t *testing.T) {
	tr := NewTracker(0)
	base := time.Unix(100, 0)
	tr.Track(press(1, 1), base)
	m := tr.Track(press(1, 1), base.Add(DefaultDoubleClickInterval/2))
	if m.Clicks != 2 {
		t.Errorf("Clicks = %d, want 2 — zero interval should fall back to the default", m.Clicks)
	}
}
