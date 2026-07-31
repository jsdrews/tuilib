package list

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// A filterable list has two focusable regions inside one Focusable: the
// filter and the body. These tests pin how the two stay reconciled, which is
// what stops focus drifting between components on a multi-pane screen.

func newFilterable(t *testing.T, r geom.Rect) Model {
	t.Helper()
	m := New(Options{Items: []string{"alpha", "bravo", "charlie"}, Filterable: true})
	m.SetRect(geom.New(r.X, r.Y, r.W, r.H))
	return m
}

var fRect = geom.Rect{X: 0, Y: 0, W: 30, H: 12}

// The filter occupies the top 3 rows; its input row is the middle one.
const filterInputY = 1

func TestClickingFilterUnhighlightsBody(t *testing.T) {
	m := newFilterable(t, fRect)
	m.Focus()
	if !m.body.Focused() {
		t.Fatalf("setup: body should be highlighted after Focus")
	}

	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))

	if !m.Filtering() {
		t.Errorf("clicking the filter did not give it input")
	}
	if m.body.Focused() {
		t.Errorf("body stayed highlighted while the filter owns input — " +
			"nothing shows which region is active")
	}
}

// A focus grant arriving after the click must not steal the highlight back.
func TestFocusDoesNotClobberAFocusedFilter(t *testing.T) {
	m := newFilterable(t, fRect)
	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))

	m.Focus() // the group granting the request the click emitted

	if !m.Filtering() {
		t.Errorf("the focus grant blurred the filter")
	}
	if m.body.Focused() {
		t.Errorf("the focus grant re-highlighted the body over the filter")
	}
}

func TestClickingBodyBlursFilter(t *testing.T) {
	m := newFilterable(t, fRect)
	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))
	if !m.Filtering() {
		t.Fatalf("setup: filter should own input")
	}

	// Body content starts below the filter's 3 rows, plus the pane border.
	m, _ = m.Update(press(fRect.X+3, fRect.Y+4, 1))

	if m.Filtering() {
		t.Errorf("clicking the body left the filter focused — it keeps eating keys")
	}
	if !m.body.Focused() {
		t.Errorf("clicking the body did not highlight it")
	}
}

// Blur has to clear the whole component. A filter left focused on a blurred
// component is the state that breaks a second filterable pane.
func TestBlurClearsFilterToo(t *testing.T) {
	m := newFilterable(t, fRect)
	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))
	if !m.Filtering() {
		t.Fatalf("setup: filter should own input")
	}

	m.Blur()

	if m.Filtering() {
		t.Errorf("Blur left the filter focused")
	}
	if m.body.Focused() {
		t.Errorf("Blur left the body highlighted")
	}
	if m.Focused() {
		t.Errorf("Focused() reports true after Blur")
	}
}

// Focused must account for either region, or a group can't tell that a
// component with only its filter active still holds focus.
func TestFocusedCoversFilterRegion(t *testing.T) {
	m := newFilterable(t, fRect)
	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))

	if !m.Focused() {
		t.Errorf("Focused() = false while this component's filter owns input")
	}
}

func TestSlashFocusesFilterAndUnhighlightsBody(t *testing.T) {
	m := newFilterable(t, fRect)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	if !m.Filtering() {
		t.Fatalf("'/' did not focus the filter")
	}
	if m.body.Focused() {
		t.Errorf("'/' left the body highlighted alongside the filter")
	}
}

// Committing or cancelling the filter hands the highlight back to the body.
func TestLeavingFilterRestoresBodyHighlight(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyEsc},
	} {
		m := newFilterable(t, fRect)
		m.Focus()
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		if !m.Filtering() {
			t.Fatalf("setup: filter should own input")
		}

		m, _ = m.Update(k)

		if m.Filtering() {
			t.Errorf("%v left the filter focused", k.Type)
		}
		if !m.body.Focused() {
			t.Errorf("%v did not restore the body highlight", k.Type)
		}
	}
}
