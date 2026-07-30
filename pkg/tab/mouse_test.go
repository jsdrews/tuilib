package tab

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var hostRect = geom.Rect{X: 0, Y: 0, W: 60, H: 12}

func newMouseTabs(t *testing.T, pos StripPos) Model {
	t.Helper()
	m := New(Options{
		Theme:    theme.Dark(),
		StripPos: pos,
		Tabs: []Tab{
			{Label: "Cities", Body: &stubBody{label: "cities"}},
			{Label: "Counter", Body: &stubBody{label: "counter"}},
			{Label: "Logs", Body: &stubBody{label: "logs"}},
		},
	})
	m.SetRect(geom.New(hostRect.X, hostRect.Y, hostRect.W, hostRect.H))
	return m
}

func pressAt(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

// Labels render as " Label " separated by " │ ", inside the bar's one cell of
// left padding. " Cities " is 8 wide, the separator 3.
const (
	stripY   = 0
	citiesX  = 1 + 2  // padding + into " Cities "
	counterX = 1 + 12 // past " Cities " (8) + separator (3)
	logsX    = 1 + 23 // past " Counter " (9) + separator (3)
	gapX     = 1 + 9  // on the separator between Cities and Counter
	pastEndX = 50     // empty bar background past the last label
)

func TestClickSwitchesToTab(t *testing.T) {
	m := newMouseTabs(t, StripTop)

	m, _ = m.Update(pressAt(counterX, stripY))
	if m.ActiveTab() != 1 {
		t.Errorf("ActiveTab() = %d, want 1", m.ActiveTab())
	}

	m, _ = m.Update(pressAt(logsX, stripY))
	if m.ActiveTab() != 2 {
		t.Errorf("ActiveTab() = %d, want 2", m.ActiveTab())
	}

	m, _ = m.Update(pressAt(citiesX, stripY))
	if m.ActiveTab() != 0 {
		t.Errorf("ActiveTab() = %d, want 0", m.ActiveTab())
	}
}

// The separator and the empty stretch past the last label are not tabs —
// clicking them should not snap to the nearest one.
func TestClickOnStripGapDoesNotSwitch(t *testing.T) {
	m := newMouseTabs(t, StripTop)

	for _, x := range []int{gapX, pastEndX} {
		m, _ = m.Update(pressAt(x, stripY))
		if m.ActiveTab() != 0 {
			t.Errorf("click at x=%d switched to tab %d; dead strip space should do nothing", x, m.ActiveTab())
		}
	}
}

func TestStripBottomPositionsHitTest(t *testing.T) {
	m := newMouseTabs(t, StripBottom)
	bottomY := hostRect.Y + hostRect.H - 1

	m, _ = m.Update(pressAt(counterX, bottomY))
	if m.ActiveTab() != 1 {
		t.Errorf("ActiveTab() = %d, want 1 — strip should hit-test on the last row", m.ActiveTab())
	}

	// The old top row is now body, not strip.
	m, _ = m.Update(pressAt(citiesX, stripY))
	if m.ActiveTab() != 1 {
		t.Errorf("a click on the body row switched tabs")
	}
}

// Clicking a tab that is already active is a no-op rather than a re-entry —
// switchTo bails when the index is unchanged, so OnEnter does not re-fire.
func TestClickActiveTabDoesNotReenter(t *testing.T) {
	m := newMouseTabs(t, StripTop)
	body := m.tabs[0].Body.(*stubBody)
	before := body.entered

	m, _ = m.Update(pressAt(citiesX, stripY))

	if got := m.tabs[0].Body.(*stubBody).entered; got != before {
		t.Errorf("OnEnter fired %d extra times when re-clicking the active tab", got-before)
	}
}

func TestStaleRectDeclinesStripClicks(t *testing.T) {
	m := newMouseTabs(t, StripTop)
	geom.NextGen()

	m, _ = m.Update(pressAt(counterX, stripY))

	if m.ActiveTab() != 0 {
		t.Errorf("a tab host with a stale rect switched tabs on a click")
	}
}
