// Package form demonstrates pkg/form: a vertical form with Text, Select,
// and Confirm fields. Submit replaces the form with a result pane; esc pops
// back to the launcher.
package form

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/form"
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

func (s *Screen) renderForm(w, h int) string {
	inner := max(0, w-2-pane.ScrollbarWidth)
	s.form.SetDimensions(inner, max(0, h-2))
	s.body.SetDimensions(w, h)
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
		}),
		form.Text(form.TextOptions{
			Key:         "email",
			Label:       "Email",
			Placeholder: "you@example.com",
		}),
		form.Select(form.SelectOptions{
			Key:     "role",
			Label:   "Role",
			Options: []string{"admin", "editor", "viewer"},
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
	for _, k := range []string{"name", "email", "role", "notify"} {
		fmt.Fprintf(&b, "  %-8s %v\n", k+":", values[k])
	}
	b.WriteString("\nEsc pops back to the launcher.")
	s.result.SetContent(b.String())
}
