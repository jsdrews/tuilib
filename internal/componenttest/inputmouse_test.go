package componenttest

import (
	"testing"

	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// The input-style components were the last gap in mouse support: list, table,
// tree, inspector, logview and textview all handled clicks while input,
// toggle and form handled none. That is what made the gate and focus
// examples behave inconsistently — a screen holding a list and an input had
// one clickable pane and one dead one.

var inputRect = geom.Rect{X: 2, Y: 1, W: 30, H: 3}

func placedInput() geom.Rect {
	return geom.New(inputRect.X, inputRect.Y, inputRect.W, inputRect.H)
}

func TestClickingAnInputFocusesIt(t *testing.T) {
	m := input.New(input.Options{})
	m.SetRect(placedInput())
	if m.Focused() {
		t.Fatalf("setup: input should start blurred")
	}

	m, cmd := m.Update(press(inputRect.X+4, inputRect.Y+1))

	if !m.Focused() {
		t.Errorf("clicking an input did not focus it")
	}
	if !hasFocusRequest(cmd) {
		t.Errorf("clicking an input did not ask the group for focus")
	}
}

func TestClickingOutsideAnInputIsDeclined(t *testing.T) {
	m := input.New(input.Options{})
	m.SetRect(placedInput())

	m, cmd := m.Update(press(inputRect.X+200, inputRect.Y+1))

	if m.Focused() || cmd != nil {
		t.Errorf("a click outside the input was claimed by it")
	}
}

// A toggle is two buttons; clicking one picks it outright, the same call
// pkg/confirm makes. Requiring a click to focus and another to choose would
// be a worse control, not a safer one.
func TestClickingAToggleButtonPicksThatSide(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial bool
		dx      int
		want    bool
	}{
		{"clicking yes selects yes", false, 2, true},
		{"clicking no selects no", true, 12, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := toggle.New(toggle.Options{Initial: tc.initial})
			m.SetRect(placedInput())

			m, _ = m.Update(press(inputRect.X+tc.dx, inputRect.Y+1))

			if m.Value() != tc.want {
				t.Errorf("Value() = %v, want %v", m.Value(), tc.want)
			}
			if !m.Focused() {
				t.Errorf("clicking a toggle did not focus it")
			}
		})
	}
}

// An unfocused toggle must still respond to a click — that is how it gets
// focused. It used to bail before looking at the message.
func TestClickingAnUnfocusedToggleWorks(t *testing.T) {
	m := toggle.New(toggle.Options{})
	m.SetRect(placedInput())
	m.Blur()

	m, cmd := m.Update(press(inputRect.X+2, inputRect.Y+1))

	if !m.Focused() {
		t.Errorf("an unfocused toggle ignored a click")
	}
	if !hasFocusRequest(cmd) {
		t.Errorf("the toggle did not ask the group for focus")
	}
}

// Clicking a form field focuses that field, including one that isn't already
// focused — the failure mode is a form routing mouse the way it routes keys,
// to the focused field only.
func TestClickingAFormFieldFocusesIt(t *testing.T) {
	f := form.New(form.Options{Fields: []form.Field{
		form.Text(form.TextOptions{Key: "a", Label: "A"}),
		form.Text(form.TextOptions{Key: "b", Label: "B"}),
		form.Confirm(form.ConfirmOptions{Key: "c", Label: "C"}),
	}})
	r := geom.New(0, 0, 40, 20)
	f.SetRect(r)
	f.View() // fields learn their rects during render

	if f.FocusedIndex() != 0 {
		t.Fatalf("setup: expected the first field focused, got %d", f.FocusedIndex())
	}

	// The second field sits below the first; each text field is 3 rows.
	second := f.FieldRect(1)
	if second.Empty() {
		t.Fatalf("setup: second field has no rect")
	}
	f, _ = f.Update(press(second.X+2, second.Y+1))

	if f.FocusedIndex() != 1 {
		t.Errorf("FocusedIndex() = %d, want 1 — the click did not move focus",
			f.FocusedIndex())
	}
}
