// Package form is the pkg/form showcase: a two-step sign-up wizard holding
// every field kind on one card.
//
//	Account  Text with a format rule, Password with a length rule, both
//	         required and each with a note saying what it is for
//	Profile  a Select that must be picked (it chooses on arrival), a
//	         MultiSelect that needs at least one, a TextArea, and a Confirm
//
// Enter moves down and, on a step's last field, on to the next step; esc goes
// back a step. Submitting replaces the form with what it submitted. A theme
// swap keeps every answer, the step and the focus (rule 4).
package form

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type Screen struct {
	t      theme.Theme
	form   form.Model
	result pane.Pane
	values form.Values
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string       { return "Form" }
func (s *Screen) Init() tea.Cmd       { return s.form.Init() }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

func (s *Screen) IsCapturingKeys() bool { return s.values == nil && s.form.IsCapturingKeys() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case form.SubmittedMsg:
		s.values = m.Values
		s.rebuildResult()
		return s, nil
	case form.CancelledMsg:
		return s, screen.Pop(nil)
	case form.InvalidMsg:
		// The form already flagged the fields and moved focus to the first;
		// this is only a summary at a glance, and reacting is optional.
		return s, app.Error(fmt.Sprintf("%d field(s) need attention: %s",
			len(m.Keys), strings.Join(m.Keys, ", ")))
	}
	var cmd tea.Cmd
	if s.values != nil {
		s.result, cmd = s.result.Update(msg)
		return s, cmd
	}
	s.form, cmd = s.form.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node {
	if s.values != nil {
		return layout.Sized(&s.result)
	}
	return layout.Sized(&s.form)
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

func (s *Screen) HelpSections() []help.Section {
	if s.values != nil {
		return help.Sections(help.Group("Signup",
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))))
	}
	return help.SectionsOf(&s.form)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	st := s.form.State()
	opts := t.Form()
	opts.Title = "new account"
	opts.SubmitText = "Create account"
	opts.Steps = []form.Step{
		{Title: "Account", Fields: []form.Field{
			form.Text(form.TextOptions{
				Key: "name", Label: "Name", Placeholder: "your name…", Required: true,
				Note: "Shown to everyone in your workspace.",
			}),
			form.Text(form.TextOptions{
				Key: "email", Label: "Email", Placeholder: "you@example.com", Required: true,
				Note: "Where sign-in links and alerts go.",
				// Required runs first, so an empty field says "required" and
				// only a non-empty one gets as far as this.
				Validate: func(v any) error {
					if !strings.Contains(v.(string), "@") {
						return errors.New("needs an @")
					}
					return nil
				},
			}),
			form.Password(form.PasswordOptions{
				Key: "password", Label: "Password", Placeholder: "at least 8 characters", Required: true,
				// Validation reads the real text, not the mask.
				Validate: func(v any) error {
					if len(v.(string)) < 8 {
						return errors.New("too short")
					}
					return nil
				},
			}),
		}},
		{Title: "Profile", Fields: []form.Field{
			form.Select(form.SelectOptions{
				Key: "role", Label: "Role", RequirePick: true,
				Options: []string{"admin", "editor", "viewer", "auditor", "billing", "guest"},
				Note:    "Decides what you can change. Nothing is chosen until you reach it.",
			}),
			form.MultiSelect(form.MultiSelectOptions{
				Key: "scopes", Label: "Scopes", Min: 1,
				Options: []string{"read", "write", "deploy", "admin-api"},
				Note:    "space toggles. Pick at least one.",
			}),
			form.TextArea(form.TextAreaOptions{
				Key: "bio", Label: "Bio", Placeholder: "a line or two about you",
				Note: "Optional. Enter starts a new line; tab moves on.",
			}),
			form.Confirm(form.ConfirmOptions{Key: "notify", Label: "Send notifications?", Initial: true}),
		}},
	}
	s.form = form.New(opts)
	s.form.Restore(st)
	if s.values != nil {
		s.rebuildResult()
	}
}

func (s *Screen) rebuildResult() {
	s.result = pane.New(s.t.Pane())
	s.result.SetTitle("submitted")

	var b strings.Builder
	b.WriteString("Form submitted with:\n\n")
	for _, k := range []string{"name", "email", "password", "role", "scopes", "bio", "notify"} {
		v := s.values[k]
		if k == "password" {
			// The real string is right there in Values — this prints a mask
			// because echoing a submitted secret back undoes the point of
			// masking the field it came from.
			v = strings.Repeat("•", len(s.values.String(k)))
		}
		fmt.Fprintf(&b, "  %-10s %v\n", k+":", v)
	}
	b.WriteString("\nEsc pops back to the launcher.")
	s.result.SetContent(b.String())
}
