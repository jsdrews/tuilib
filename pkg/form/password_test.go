package form

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/input"
)

func withPassword(t *testing.T, opts PasswordOptions) Model {
	t.Helper()
	m := New(Options{Fields: []Field{
		Text(TextOptions{Key: "user", Label: "User"}),
		Password(opts),
	}})
	m.SetRect(geom.New(0, 0, 44, 20))
	return m
}

func TestPasswordMasksTheRenderButSubmitsTheValue(t *testing.T) {
	m := withPassword(t, PasswordOptions{Key: "pass", Label: "Password"})
	m.fields[1].(*textField).input.SetValue("s3cret")

	view := ansi.Strip(m.View())
	if strings.Contains(view, "s3cret") {
		t.Errorf("form render leaked the password:\n%s", view)
	}
	if want := strings.Repeat(string(input.DefaultMaskChar), len("s3cret")); !strings.Contains(view, want) {
		t.Errorf("form render missing the mask %q:\n%s", want, view)
	}

	var got SubmittedMsg
	m.focus = len(m.fields)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range flatten(cmd) {
		if v, ok := msg.(SubmittedMsg); ok {
			got = v
		}
	}
	if got.Values["pass"] != "s3cret" {
		t.Errorf("SubmittedMsg.Values[pass] = %v, want the real text", got.Values["pass"])
	}
}

func TestPasswordMaskCharOverride(t *testing.T) {
	m := withPassword(t, PasswordOptions{Key: "pass", Label: "Password", MaskChar: '*'})
	m.fields[1].(*textField).input.SetValue("abcd")

	if view := ansi.Strip(m.View()); !strings.Contains(view, "****") {
		t.Errorf("form render missing the custom mask:\n%s", view)
	}
}

// Validation reads through the mask, so Required and Validate behave exactly
// as they do on a Text field.
func TestPasswordValidatesTheRealText(t *testing.T) {
	tooShort := func(v any) error {
		if s, _ := v.(string); len(s) < 8 {
			return errors.New("too short")
		}
		return nil
	}
	m := withPassword(t, PasswordOptions{
		Key: "pass", Label: "Password", Required: true, Validate: tooShort,
	})

	m.focus = len(m.fields)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range flatten(cmd) {
		if _, ok := msg.(SubmittedMsg); ok {
			t.Fatal("the form submitted with an empty required password")
		}
	}

	m.fields[1].(*textField).input.SetValue("short")
	if err := m.fields[1].Validate(); err == nil {
		t.Error("Validate() = nil for a 5-char password, want the length error")
	}
	m.fields[1].(*textField).input.SetValue("longenough")
	if err := m.fields[1].Validate(); err != nil {
		t.Errorf("Validate() = %v for a valid password, want nil", err)
	}
}
