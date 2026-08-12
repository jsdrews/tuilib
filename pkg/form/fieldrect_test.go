package form

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

func sample() Model {
	return New(Options{Fields: []Field{
		Text(TextOptions{Key: "name", Label: "Name"}),
		Text(TextOptions{Key: "email", Label: "Email"}),
		Select(SelectOptions{Key: "role", Label: "Role", Options: []string{"admin", "user"}}),
		Confirm(ConfirmOptions{Key: "notify", Label: "Notify"}),
	}})
}

func placed(t *testing.T) Model {
	t.Helper()
	m := sample()
	m.SetRect(geom.New(0, 0, 40, 24))
	m.View() // fields learn their rects during render
	return m
}

// A field's rect has to be the rows it actually drew. Handing every field the
// form's full height makes them all overlap, and then a click resolves to
// whichever the loop happens to reach first rather than the one under the
// pointer.
func TestFieldRectsAreDisjoint(t *testing.T) {
	m := placed(t)

	for i := range m.fields {
		a := m.fields[i].Rect()
		if a.Empty() {
			t.Errorf("field %d has an empty rect", i)
		}
		for j := i + 1; j < len(m.fields); j++ {
			b := m.fields[j].Rect()
			if a.Y+a.H > b.Y && b.Y+b.H > a.Y {
				t.Errorf("field %d (y %d..%d) overlaps field %d (y %d..%d)",
					i, a.Y, a.Y+a.H-1, j, b.Y, b.Y+b.H-1)
			}
		}
	}
}

// Each field's rect must start where the previous one ended, plus the
// configured spacing — otherwise there are dead bands a click falls into.
func TestFieldRectsFollowTheRenderedLayout(t *testing.T) {
	m := placed(t)

	prev := m.fields[0].Rect()
	for i := 1; i < len(m.fields); i++ {
		got := m.fields[i].Rect()
		want := prev.Y + prev.H + m.spacing
		if got.Y != want {
			t.Errorf("field %d starts at y=%d, want %d (field %d ended at %d)",
				i, got.Y, want, i-1, prev.Y+prev.H-1)
		}
		prev = got
	}
}

func press(x, y int) mouse.Msg {
	return mouse.Msg{
		MouseMsg: tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft},
		Clicks:   1,
	}
}

// Clicking each field in turn must focus that field — not cycle between two
// of them, which is what overlapping rects produced.
func TestClickingEachFieldFocusesThatField(t *testing.T) {
	m := placed(t)

	for i := range m.fields {
		r := m.fields[i].Rect()
		m, _ = m.Update(press(r.X+2, r.Y+1))
		if m.focus != i {
			t.Errorf("clicking field %d (y %d..%d) focused %d instead",
				i, r.Y, r.Y+r.H-1, m.focus)
		}
	}
}

// Clicking the field that already has focus keeps it there. The bug was a
// loop that skipped the focused field without stopping, so the click fell
// through to whichever field's rect also happened to cover the point.
func TestClickingTheFocusedFieldKeepsFocus(t *testing.T) {
	m := placed(t)
	r := m.fields[0].Rect()

	for range 3 {
		m, _ = m.Update(press(r.X+2, r.Y+1))
		if m.focus != 0 {
			t.Fatalf("repeated clicks on field 0 moved focus to %d", m.focus)
		}
	}
}

func TestClickOutsideAnyFieldLeavesFocusAlone(t *testing.T) {
	m := placed(t)
	last := m.fields[len(m.fields)-1].Rect()

	m, _ = m.Update(press(last.X+2, last.Y+last.H+3))

	if m.focus != 0 {
		t.Errorf("a click below every field moved focus to %d", m.focus)
	}
}

// The submit button is a focus stop like any field, so it has to be
// clickable. It isn't a Field, so nothing recorded a rect for it and a click
// simply fell through.
func TestClickingSubmitSubmits(t *testing.T) {
	m := placed(t)

	r := m.SubmitRect()
	if r.Empty() {
		t.Fatalf("the submit button has no rect")
	}

	m, cmd := m.Update(press(r.X+2, r.Y))

	if cmd == nil {
		t.Fatalf("clicking submit produced no command")
	}
	if _, ok := cmd().(SubmittedMsg); !ok {
		t.Errorf("clicking submit gave %T, want SubmittedMsg", cmd())
	}
	if m.focus != len(m.fields) {
		t.Errorf("clicking submit left focus on field %d", m.focus)
	}
}

// The button sits below the last field, after the configured spacing.
func TestSubmitRectFollowsTheLastField(t *testing.T) {
	m := placed(t)

	last := m.fields[len(m.fields)-1].Rect()
	got := m.SubmitRect()

	if want := last.Y + last.H + m.spacing; got.Y != want {
		t.Errorf("submit at y=%d, want %d (last field ends at %d)",
			got.Y, want, last.Y+last.H-1)
	}
	if got.H != 1 {
		t.Errorf("submit rect is %d rows, want 1", got.H)
	}
}

func TestClickBesideSubmitDoesNotSubmit(t *testing.T) {
	m := placed(t)
	r := m.SubmitRect()

	_, cmd := m.Update(press(r.X+r.W+4, r.Y))

	if cmd != nil {
		if _, ok := cmd().(SubmittedMsg); ok {
			t.Errorf("a click past the end of the button submitted the form")
		}
	}
}
