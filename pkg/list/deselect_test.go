package list

import (
	"testing"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func TestDeselectLeavesNothingSelected(t *testing.T) {
	m := New(Options{Items: []string{"a", "b", "c"}})
	m.SetRect(geom.New(0, 0, 20, 8))

	m.Deselect()

	if _, ok := m.Selected(); ok {
		t.Errorf("Selected() reported a selection after Deselect")
	}
	if m.Cursor() >= 0 {
		t.Errorf("Cursor() = %d, want negative", m.Cursor())
	}
}

// refresh calls EnsureVisible(cursor) on every pass; -1 must not drag the
// viewport somewhere invalid.
func TestDeselectDoesNotBreakTheViewport(t *testing.T) {
	items := make([]string, 100)
	for i := range items {
		items[i] = "row"
	}
	m := New(Options{Items: items})
	m.SetRect(geom.New(0, 0, 20, 8))
	m.SetCursor(50)

	m.Deselect()

	if got := m.body.YOffset(); got < 0 {
		t.Errorf("YOffset() = %d after Deselect; negative offset", got)
	}
	if m.View() == "" {
		t.Errorf("View() is empty after Deselect")
	}
}

// The highlight must actually be gone from the render, not just the index.
func TestDeselectRemovesTheHighlight(t *testing.T) {
	m := New(Options{Items: []string{"alpha", "bravo"}})
	m.SetRect(geom.New(0, 0, 20, 8))
	withCursor := m.View()

	m.Deselect()

	if m.View() == withCursor {
		t.Errorf("the render is unchanged after Deselect; the cursor row is still drawn")
	}
}

// SetCursor still clamps, so a negative index can't blank a list by accident.
func TestSetCursorStillClamps(t *testing.T) {
	m := New(Options{Items: []string{"a", "b"}})
	m.SetRect(geom.New(0, 0, 20, 8))

	m.SetCursor(-5)

	if m.Cursor() != 0 {
		t.Errorf("SetCursor(-5) = %d, want 0 (clamped)", m.Cursor())
	}
}
