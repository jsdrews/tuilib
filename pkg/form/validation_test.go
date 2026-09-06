package form

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

func errNotEmail(v any) error {
	if s, _ := v.(string); !strings.Contains(s, "@") {
		return errors.New("not an email")
	}
	return nil
}

func validated(t *testing.T) Model {
	t.Helper()
	m := New(Options{
		Styles: Styles{
			PaneActiveColor:   lipgloss.Color("12"),
			PaneInactiveColor: lipgloss.Color("240"),
			ErrorColor:        lipgloss.Color("160"),
		},
		Fields: []Field{
			Text(TextOptions{Key: "name", Label: "Name", Required: true}),
			Text(TextOptions{Key: "email", Label: "Email", Required: true, Validate: errNotEmail}),
			Select(SelectOptions{Key: "role", Label: "Role",
				Options: []string{"admin", "user"}, RequirePick: true}),
			Confirm(ConfirmOptions{Key: "tos", Label: "Accept", Validate: func(v any) error {
				if b, _ := v.(bool); !b {
					return errors.New("must accept")
				}
				return nil
			}}),
		},
	})
	m.SetRect(geom.New(0, 0, 44, 30))
	m.View()
	return m
}

func submit(m Model) (Model, tea.Msg) {
	m.focus = len(m.fields)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		return next, nil
	}
	return next, cmd()
}

// flatten runs a command and returns every message it produces.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, flatten(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func TestSubmitRefusedWhileRequiredFieldsAreEmpty(t *testing.T) {
	m := validated(t)

	m.focus = len(m.fields)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, msg := range flatten(cmd) {
		if _, ok := msg.(SubmittedMsg); ok {
			t.Fatalf("the form submitted with required fields empty")
		}
	}
	var got InvalidMsg
	for _, msg := range flatten(cmd) {
		if v, ok := msg.(InvalidMsg); ok {
			got = v
		}
	}
	if len(got.Keys) == 0 {
		t.Fatalf("no InvalidMsg naming the offending fields")
	}
	// name, email and role all fail; tos fails its Validate too.
	for _, want := range []string{"name", "email", "role", "tos"} {
		if !contains(got.Keys, want) {
			t.Errorf("InvalidMsg.Keys = %v, missing %q", got.Keys, want)
		}
	}
}

// Focus has to land on the first offender, or the user has to hunt for it.
func TestInvalidSubmitFocusesTheFirstBadField(t *testing.T) {
	m := validated(t)
	m.fields[0].(*textField).input.SetValue("Ada") // fix the first field

	m.focus = len(m.fields)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.focus != 1 {
		t.Errorf("focus = %d, want 1 (the first still-invalid field)", m.focus)
	}
}

func TestSubmitSucceedsOnceEverythingIsValid(t *testing.T) {
	m := validated(t)
	m.fields[0].(*textField).input.SetValue("Ada")
	m.fields[1].(*textField).input.SetValue("ada@example.com")
	m.fields[2].(*selectField).list.SetCursor(0)
	m.fields[3].(*confirmField).toggle.SetValue(true)

	m, msg := submit(m)

	got, ok := msg.(SubmittedMsg)
	if !ok {
		t.Fatalf("submit gave %T, want SubmittedMsg", msg)
	}
	if got.Values["email"] != "ada@example.com" {
		t.Errorf("Values[email] = %v", got.Values["email"])
	}
}

// Required is checked before Validate, so an empty required field reports
// "required" rather than whatever a format rule makes of "".
func TestRequiredIsReportedBeforeFormatErrors(t *testing.T) {
	m := validated(t)

	if err := m.fields[1].Validate(); !errors.Is(err, ErrRequired) {
		t.Errorf("empty required+validated field reported %v, want ErrRequired", err)
	}

	m.fields[1].(*textField).input.SetValue("nope")
	if err := m.fields[1].Validate(); err == nil || errors.Is(err, ErrRequired) {
		t.Errorf("non-empty invalid field reported %v, want the format error", err)
	}
}

// Nothing complains until the first submit — that is the whole point of
// validating on submit rather than on every keystroke.
func TestNoErrorsBeforeTheFirstSubmit(t *testing.T) {
	m := validated(t)

	if strings.Contains(ansi.Strip(m.View()), "required") {
		t.Errorf("an untouched form is already showing errors")
	}
}

// Once flagged, a field re-checks on every keystroke so the fix is
// acknowledged immediately.
func TestFlaggedFieldClearsAsSoonAsItIsFixed(t *testing.T) {
	m := validated(t)
	m.focus = len(m.fields)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // flags everything

	if !m.flagged[0] {
		t.Fatalf("setup: field 0 should be flagged after a failed submit")
	}

	m.focus = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})

	if m.flagged[0] {
		t.Errorf("field 0 is still flagged after being filled in")
	}
	if err := m.fields[0].Validate(); err != nil {
		t.Errorf("field 0 still reports %v", err)
	}
}

// A field the user has not reached must stay quiet while they fix another.
func TestFixingOneFieldDoesNotClearAnother(t *testing.T) {
	m := validated(t)
	m.focus = len(m.fields)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m.focus = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})

	if !m.flagged[1] {
		t.Errorf("fixing field 0 cleared field 1's error too")
	}
}

// The required marker is a standing property, visible before any submit.
func TestRequiredFieldsAreMarkedWithAnAsterisk(t *testing.T) {
	m := validated(t)
	view := ansi.Strip(m.View())

	for _, want := range []string{"Name *", "Email *", "Role *"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing the required marker %q", want)
		}
	}
	if strings.Contains(view, "Accept *") {
		t.Errorf("Confirm has no Required option but was marked with an asterisk")
	}
}

// A RequirePick select starts with nothing chosen, so the first pick is
// deliberate rather than inherited from the cursor's resting place.
func TestRequirePickSelectStartsUnset(t *testing.T) {
	m := validated(t)

	if v := m.Values()["role"]; v != "" {
		t.Errorf("Values[role] = %q before any pick, want empty", v)
	}
	if err := m.fields[2].Validate(); !errors.Is(err, ErrRequired) {
		t.Errorf("an unpicked RequirePick select reported %v, want ErrRequired", err)
	}

	m.fields[2].(*selectField).list.SetCursor(1)
	if v := m.Values()["role"]; v != "user" {
		t.Errorf("Values[role] = %q after picking, want \"user\"", v)
	}
	if err := m.fields[2].Validate(); err != nil {
		t.Errorf("a picked select still reports %v", err)
	}
}

// The error has to be visible, not just tracked.
func TestErrorIsRenderedOnTheField(t *testing.T) {
	m := validated(t)
	before := m.View()

	m.focus = len(m.fields)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	after := m.View()
	if after == before {
		t.Fatalf("the view is unchanged after a failed submit")
	}
	if !strings.Contains(ansi.Strip(after), "required") {
		t.Errorf("no error message rendered on the failing fields")
	}
}

// Showing the error must not move anything — a form that reflows while you
// are correcting it fights the user, and every field's rect would shift.
func TestErrorsDoNotChangeTheLayout(t *testing.T) {
	m := validated(t)
	rectsBefore := make([]geom.Rect, len(m.fields))
	for i := range m.fields {
		rectsBefore[i] = m.fields[i].Rect()
	}
	linesBefore := strings.Count(m.View(), "\n")

	m.focus = len(m.fields)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.View()

	if got := strings.Count(m.View(), "\n"); got != linesBefore {
		t.Errorf("the form is %d rows after errors, was %d", got+1, linesBefore+1)
	}
	for i := range m.fields {
		if m.fields[i].Rect() != rectsBefore[i] {
			t.Errorf("field %d moved when errors appeared: %+v → %+v",
				i, rectsBefore[i], m.fields[i].Rect())
		}
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
