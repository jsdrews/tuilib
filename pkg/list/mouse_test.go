package list

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

// listRect is where every test in this file places the list. Content starts
// one row down and one column right of the outer rect (the pane border), so
// row 0 sits at y = listRect.Y+1.
var listRect = geom.Rect{X: 10, Y: 5, W: 30, H: 10}

const firstRowY = 6 // listRect.Y + 1 (border)

func newList(t *testing.T, items ...string) Model {
	t.Helper()
	m := New(Options{Items: items})
	m.SetRect(geom.New(listRect.X, listRect.Y, listRect.W, listRect.H))
	return m
}

// press builds a mouse.Msg the way the app shell would, with the given click
// count already resolved.
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

// collect flattens a command (including batches) into the messages it emits.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collect(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func hasFocusRequest(msgs []tea.Msg, tok focus.Token) bool {
	for _, m := range msgs {
		if r, ok := m.(focus.RequestMsg); ok && r.Token == tok {
			return true
		}
	}
	return false
}

func findActivated(msgs []tea.Msg) (ActivatedMsg, bool) {
	for _, m := range msgs {
		if a, ok := m.(ActivatedMsg); ok {
			return a, true
		}
	}
	return ActivatedMsg{}, false
}

func TestClickMovesCursorToClickedRow(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie", "delta")

	m, _ = m.Update(press(listRect.X+3, firstRowY+2, 1))

	if m.Cursor() != 2 {
		t.Errorf("Cursor() = %d, want 2 (third visible row)", m.Cursor())
	}
}

func TestClickRequestsFocusForItself(t *testing.T) {
	m := newList(t, "alpha", "bravo")

	_, cmd := m.Update(press(listRect.X+3, firstRowY, 1))

	if !hasFocusRequest(collect(cmd), m.FocusToken()) {
		t.Errorf("click did not emit a focus request naming this list")
	}
}

// Rule 14: double click is the mouse spelling of enter.
func TestDoubleClickActivatesSelection(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie")

	m, cmd := m.Update(press(listRect.X+3, firstRowY+1, 2))

	got, ok := findActivated(collect(cmd))
	if !ok {
		t.Fatalf("double click emitted no ActivatedMsg")
	}
	if got.Index != 1 || got.Item != "bravo" {
		t.Errorf("ActivatedMsg = %+v, want {Index:1 Item:bravo}", got)
	}
}

func TestSingleClickDoesNotActivate(t *testing.T) {
	m := newList(t, "alpha", "bravo")

	_, cmd := m.Update(press(listRect.X+3, firstRowY+1, 1))

	if _, ok := findActivated(collect(cmd)); ok {
		t.Errorf("single click activated the selection; only a double click should")
	}
}

// A click outside the list must pass through untouched so a sibling pane can
// claim it.
func TestClickOutsideRectIsDeclined(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie")
	m, _ = m.Update(press(listRect.X+3, firstRowY+2, 1)) // cursor to row 2

	before := m.Cursor()
	m, cmd := m.Update(press(listRect.X+100, firstRowY, 1))

	if m.Cursor() != before {
		t.Errorf("cursor moved on a click outside the rect")
	}
	if cmd != nil {
		t.Errorf("click outside the rect produced a command")
	}
}

// The border is not content — clicking it must not select the first row.
func TestClickOnBorderIsDeclined(t *testing.T) {
	m := newList(t, "alpha", "bravo")

	_, cmd := m.Update(press(listRect.X, listRect.Y, 1))

	if cmd != nil {
		t.Errorf("click on the pane border produced a command")
	}
}

func TestWheelMovesCursorLikeArrowKeys(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie", "delta")

	m, _ = m.Update(wheel(listRect.X+3, firstRowY, false))
	if m.Cursor() != 1 {
		t.Fatalf("after wheel down, Cursor() = %d, want 1", m.Cursor())
	}
	m, _ = m.Update(wheel(listRect.X+3, firstRowY, false))
	if m.Cursor() != 2 {
		t.Fatalf("after second wheel down, Cursor() = %d, want 2", m.Cursor())
	}
	m, _ = m.Update(wheel(listRect.X+3, firstRowY, true))
	if m.Cursor() != 1 {
		t.Errorf("after wheel up, Cursor() = %d, want 1", m.Cursor())
	}
}

func TestWheelClampsAtBounds(t *testing.T) {
	m := newList(t, "alpha", "bravo")

	m, _ = m.Update(wheel(listRect.X+3, firstRowY, true))
	if m.Cursor() != 0 {
		t.Errorf("wheel up at the top moved the cursor to %d", m.Cursor())
	}
	for range 5 {
		m, _ = m.Update(wheel(listRect.X+3, firstRowY, false))
	}
	if m.Cursor() != 1 {
		t.Errorf("wheel down past the end moved the cursor to %d, want 1", m.Cursor())
	}
}

// Scrolling does not require focus — the wheel acts on whatever is under the
// pointer, and must not emit a focus request.
func TestWheelDoesNotRequestFocus(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie")

	_, cmd := m.Update(wheel(listRect.X+3, firstRowY, false))

	if hasFocusRequest(collect(cmd), m.FocusToken()) {
		t.Errorf("wheel stole focus; scrolling should leave focus alone")
	}
}

// A component not drawn in the current frame holds a stale rect and must
// decline every event — this is what stops a hidden tab body from claiming
// clicks meant for the visible one.
func TestStaleRectDeclinesClicks(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie")

	geom.NextGen() // a frame passes without this list being drawn

	before := m.Cursor()
	m, cmd := m.Update(press(listRect.X+3, firstRowY+2, 1))

	if m.Cursor() != before {
		t.Errorf("a list with a stale rect moved its cursor on a click")
	}
	if cmd != nil {
		t.Errorf("a list with a stale rect produced a command")
	}
}

// Clicking row N when the viewport has scrolled must select the Nth visible
// row, not the Nth item.
func TestClickAccountsForScrollOffset(t *testing.T) {
	items := make([]string, 40)
	for i := range items {
		items[i] = string(rune('a'+i%26)) + "-item"
	}
	m := newList(t, items...)

	// Drive the cursor to the bottom so the viewport scrolls.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	bottom := m.Cursor()
	if bottom != len(items)-1 {
		t.Fatalf("setup: G left cursor at %d, want %d", bottom, len(items)-1)
	}

	// Click the first visible row. It should be well below index 0.
	m, _ = m.Update(press(listRect.X+3, firstRowY, 1))

	if m.Cursor() == 0 {
		t.Errorf("click resolved to item 0; the viewport's scroll offset was ignored")
	}
	if m.Cursor() > bottom {
		t.Errorf("click resolved to %d, past the last visible row", m.Cursor())
	}
}

// The shell resolves double clicks, so the list needs no timing of its own —
// but a real sequence through the tracker must still produce an activation.
func TestEndToEndDoubleClickThroughTracker(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie")
	tr := mouse.NewTracker(500 * time.Millisecond)
	base := time.Unix(100, 0)

	raw := tea.MouseMsg{
		X: listRect.X + 3, Y: firstRowY + 1,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	}
	m, _ = m.Update(tr.Track(raw, base))
	m, cmd := m.Update(tr.Track(raw, base.Add(100*time.Millisecond)))

	if _, ok := findActivated(collect(cmd)); !ok {
		t.Errorf("two quick presses in the same cell did not activate the row")
	}
}

// Enter and double-click must resolve through one predicate, or a screen
// ends up implementing "open" twice and the two drift apart.
func TestIsActivateAcceptsEnterAndDoubleClick(t *testing.T) {
	m := newList(t, "alpha", "bravo")

	if !m.IsActivate(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Errorf("enter did not read as an activation")
	}
	if !m.IsActivate(ActivatedMsg{Token: m.FocusToken()}) {
		t.Errorf("this list's own ActivatedMsg did not read as an activation")
	}
	if m.IsActivate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) {
		t.Errorf("an unrelated key read as an activation")
	}
}

// Two lists on one screen must not claim each other's double clicks.
func TestIsActivateIgnoresAnotherListsActivation(t *testing.T) {
	a := newList(t, "alpha")
	b := newList(t, "bravo")

	if a.IsActivate(ActivatedMsg{Token: b.FocusToken()}) {
		t.Errorf("a list claimed another list's activation")
	}
}

// While the filter is taking input, enter commits the filter rather than
// opening the selection.
func TestIsActivateIgnoresEnterWhileFiltering(t *testing.T) {
	m := New(Options{Items: []string{"alpha"}, Filterable: true})
	m.SetRect(geom.New(listRect.X, listRect.Y, listRect.W, listRect.H))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.Filtering() {
		t.Fatalf("setup: '/' did not focus the filter")
	}

	if m.IsActivate(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Errorf("enter read as an activation while the filter had input")
	}
}

// The double-click path must reach IsActivate end to end.
func TestDoubleClickSatisfiesIsActivate(t *testing.T) {
	m := newList(t, "alpha", "bravo", "charlie")

	_, cmd := m.Update(press(listRect.X+3, firstRowY+1, 2))

	for _, msg := range collect(cmd) {
		if m.IsActivate(msg) {
			return
		}
	}
	t.Errorf("no message from a double click satisfied IsActivate")
}
