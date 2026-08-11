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

// The filter is a row inside the pane: border, filter, rule, then items.
const (
	filterInputY = 1
	firstItemY   = 3
)

// The filter is drawn inside the component's pane, so the pane border says
// "this component has focus" and the filter row says "input goes here". The
// border must stay lit while typing — dimming it would claim the component
// lost focus when it did not.
func TestClickingFilterKeepsPaneLit(t *testing.T) {
	m := newFilterable(t, fRect)
	m.Focus()

	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))

	if !m.Filtering() {
		t.Errorf("clicking the filter did not give it input")
	}
	if !m.body.Focused() {
		t.Errorf("the pane went dim while its own filter was taking input")
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
	if !m.body.Focused() {
		t.Errorf("the focus grant left the pane dim")
	}
}

func TestClickingBodyBlursFilter(t *testing.T) {
	m := newFilterable(t, fRect)
	m, _ = m.Update(press(fRect.X+3, filterInputY, 1))
	if !m.Filtering() {
		t.Fatalf("setup: filter should own input")
	}

	// Items start below the border, the filter row, and the rule.
	m, _ = m.Update(press(fRect.X+3, firstItemY, 1))

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

func TestSlashFocusesFilterAndKeepsPaneLit(t *testing.T) {
	m := newFilterable(t, fRect)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	if !m.Filtering() {
		t.Fatalf("'/' did not focus the filter")
	}
	if !m.body.Focused() {
		t.Errorf("'/' dimmed the pane its own filter lives in")
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
