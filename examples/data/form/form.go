// Package form demonstrates pkg/form: a vertical form with Text, Password,
// Select, and Confirm fields. Submit replaces the form with a result pane;
// esc pops back to the launcher.
package form

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/app"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

type Screen struct {
	t      theme.Theme
	form   form.Model
	body   pane.Pane
	result pane.Pane
	done   bool
}

func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string       { return "Form" }
func (s *Screen) Init() tea.Cmd       { return tea.Batch(textinput.Blink, s.form.Init()) }
func (s *Screen) OnEnter(any) tea.Cmd { return nil }

func (s *Screen) IsCapturingKeys() bool {
	return !s.done && s.form.IsCapturingKeys()
}

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case form.SubmittedMsg:
		s.done = true
		s.rebuildResult(m.Values)
		return s, nil
	case form.CancelledMsg:
		return s, screen.Pop(nil)
	case form.InvalidMsg:
		// The form already flagged the fields and moved focus to the first
		// offender; this is purely for a summary the user can see at a
		// glance. Reacting at all is optional.
		return s, app.Error(fmt.Sprintf("%d field(s) need attention: %s",
			len(m.Keys), strings.Join(m.Keys, ", ")))
	}

	if s.done {
		var cmd tea.Cmd
		s.result, cmd = s.result.Update(msg)
		return s, cmd
	}

	var cmd tea.Cmd
	s.form, cmd = s.form.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node {
	if s.done {
		return layout.Sized(&s.result)
	}
	return layout.RenderFunc(s.renderForm)
}

func (s *Screen) renderForm(r geom.Rect) string {
	s.body.SetRect(r)
	s.form.SetRect(s.body.ContentRect())
	s.body.SetContent(s.form.View())
	return s.body.View()
}

func (s *Screen) Help() []key.Binding {
	if s.done {
		return []key.Binding{
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "submit")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	opts := t.Form()
	opts.Fields = []form.Field{
		form.Text(form.TextOptions{
			Key:         "name",
			Label:       "Name",
			Placeholder: "your name…",
			Required:    true,
		}),
		form.Text(form.TextOptions{
			Key:         "email",
			Label:       "Email",
			Placeholder: "you@example.com",
			Required:    true,
			// Required runs first, so an empty field says "required" and
			// only a non-empty one gets as far as this.
			Validate: func(v any) error {
				if s, _ := v.(string); !strings.Contains(s, "@") {
					return errors.New("needs an @")
				}
				return nil
			},
		}),
		form.Password(form.PasswordOptions{
			Key:   "password",
			Label: "Password",
			// The placeholder is not masked — it is a hint, not a secret, and
			// it disappears the moment anything is typed.
			Placeholder: "at least 8 characters",
			Required:    true,
			// Validation reads the real text, not the mask.
			Validate: func(v any) error {
				if s, _ := v.(string); len(s) < 8 {
					return errors.New("too short")
				}
				return nil
			},
		}),
		form.Select(form.SelectOptions{
			Key:     "role",
			Label:   "Role",
			Options: []string{"admin", "editor", "viewer"},
			// Starts with nothing highlighted, so picking a role is a
			// deliberate act rather than whatever the cursor defaulted to.
			RequirePick: true,
		}),
		form.Confirm(form.ConfirmOptions{
			Key:     "notify",
			Label:   "Send notifications?",
			Initial: true,
		}),
	}
	opts.SubmitText = "Create account"
	s.form = form.New(opts)

	paneOpts := t.Pane()
	paneOpts.Title = "new account"
	paneOpts.Focused = true
	paneOpts.ActiveBorder = lipgloss.NormalBorder()
	s.body = pane.New(paneOpts)

	if s.done {
		s.rebuildResult(nil)
	}
}

func (s *Screen) rebuildResult(values map[string]any) {
	s.result = pane.New(s.t.Pane())
	s.result.SetTitle("submitted")

	var b strings.Builder
	b.WriteString("Form submitted with:\n\n")
	if values == nil {
		values = s.form.Values()
	}
	for _, k := range []string{"name", "email", "password", "role", "notify"} {
		v := values[k]
		if k == "password" {
			// The real string is right there in Values — this prints a mask
			// because echoing a submitted secret back to the screen undoes
			// the point of masking the field it came from.
			v = strings.Repeat(string(input.DefaultMaskChar), len(fmt.Sprint(v)))
		}
		fmt.Fprintf(&b, "  %-10s %v\n", k+":", v)
	}
	b.WriteString("\nEsc pops back to the launcher.")
	s.result.SetContent(b.String())
}
