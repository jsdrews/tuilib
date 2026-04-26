// Package gate demonstrates a one-shot pre-screen flow: a "gated" root
// screen that pushes a login form on first activation, receives the
// credentials when the form pops, and re-pushes the form on logout.
//
// The form lives as a regular screen.Screen — it's only on the stack
// while the user is interacting with it, and is gone afterward. The
// trigger lives in the gate's OnEnter: it fires on initial push (with
// nil), on every return-from-child, and is the natural hook for "if I'm
// not authenticated yet, push the login screen now."
//
// State machine, driven from the gate's OnEnter:
//   - First entry, no creds: push the login form.
//   - Login pops with creds: record them and rebuild the body.
//   - Login pops with nil (cancelled before authenticating): bubble the
//     pop up so the user returns to whatever screen pushed the gate.
//
// The same Push call can be re-issued at any point — e.g. on a "logout"
// keystroke or when a request returns 401 — to bring the form back
// without permanent stack residency.
package gate

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

// New returns the gate demo's entry screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

// creds is what the login form pops back with.
type creds struct {
	user     string
	role     string
	remember bool
}

type Screen struct {
	t          theme.Theme
	body       pane.Pane
	auth       *creds
	formShown  bool // whether we've pushed the login form during this session
}

func (s *Screen) Title() string {
	if s.auth != nil {
		return "Gate (signed in)"
	}
	return "Gate"
}

func (s *Screen) Init() tea.Cmd { return nil }

// OnEnter runs each time the gate becomes the active top of the stack.
// On the initial push from the launcher, result is nil — we kick the
// login flow off here. After the login screen pops, this fires again
// with either credentials (success) or nil (cancelled).
func (s *Screen) OnEnter(result any) tea.Cmd {
	if c, ok := result.(creds); ok {
		s.auth = &c
		s.rebuild()
		return nil
	}
	if s.auth != nil {
		return nil
	}
	if !s.formShown {
		s.formShown = true
		return screen.Push(newLogin(s.t))
	}
	return screen.Pop(nil)
}

func (s *Screen) IsCapturingKeys() bool { return false }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && s.auth != nil && k.String() == "L" {
		s.auth = nil
		s.formShown = true
		s.rebuild()
		return s, screen.Push(newLogin(s.t))
	}
	var cmd tea.Cmd
	s.body, cmd = s.body.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.body) }

func (s *Screen) Help() []key.Binding {
	out := []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	if s.auth != nil {
		out = append(out, key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "logout")))
	}
	return out
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	pOpts := t.Pane()
	pOpts.Title = "workspace"
	s.body = pane.New(pOpts)
	s.rebuild()
}

func (s *Screen) rebuild() {
	var b strings.Builder
	if s.auth == nil {
		b.WriteString("Not authenticated.\n\n")
		b.WriteString("The login form should appear on top of this screen on first entry.\n")
		b.WriteString("If you can read this, the form has been popped and you're between\n")
		b.WriteString("states (e.g. it cancelled and we're about to bubble back up).\n")
	} else {
		fmt.Fprintf(&b, "Signed in as %s (%s).\n\n", s.auth.user, s.auth.role)
		if s.auth.remember {
			b.WriteString("Session is remembered.\n\n")
		} else {
			b.WriteString("Session is not remembered.\n\n")
		}
		b.WriteString("This is the real app body. Press L to log out — the form will be\n")
		b.WriteString("re-pushed on top until you sign in again. The form is never\n")
		b.WriteString("permanently part of the stack: pushed on demand, popped on submit.")
	}
	s.body.SetContent(b.String())
}

// ---- login screen --------------------------------------------------------

// loginScreen is the pushable form. SubmittedMsg pops with creds;
// CancelledMsg pops with nil so the gate can decide what cancellation
// means (here: bubble back to the launcher).
type loginScreen struct {
	t    theme.Theme
	form form.Model
	body pane.Pane
}

func newLogin(t theme.Theme) screen.Screen {
	s := &loginScreen{}
	s.SetTheme(t)
	return s
}

func (s *loginScreen) Title() string         { return "Sign in" }
func (s *loginScreen) Init() tea.Cmd         { return tea.Batch(textinput.Blink, s.form.Init()) }
func (s *loginScreen) OnEnter(any) tea.Cmd   { return nil }
func (s *loginScreen) IsCapturingKeys() bool { return s.form.IsCapturingKeys() }

func (s *loginScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch m := msg.(type) {
	case form.SubmittedMsg:
		c := creds{
			user: fmt.Sprint(m.Values["user"]),
			role: fmt.Sprint(m.Values["role"]),
		}
		if v, ok := m.Values["remember"].(bool); ok {
			c.remember = v
		}
		return s, screen.Pop(c)
	case form.CancelledMsg:
		return s, screen.Pop(nil)
	}
	var cmd tea.Cmd
	s.form, cmd = s.form.Update(msg)
	return s, cmd
}

func (s *loginScreen) Layout() layout.Node { return layout.RenderFunc(s.render) }

func (s *loginScreen) render(w, h int) string {
	inner := max(0, w-2-pane.ScrollbarWidth)
	s.form.SetDimensions(inner, max(0, h-2))
	s.body.SetDimensions(w, h)
	s.body.SetContent(s.form.View())
	return s.body.View()
}

func (s *loginScreen) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "submit")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (s *loginScreen) SetTheme(t theme.Theme) {
	s.t = t
	opts := t.Form()
	opts.Fields = []form.Field{
		form.Text(form.TextOptions{
			Key:         "user",
			Label:       "Username",
			Placeholder: "alice",
		}),
		form.Select(form.SelectOptions{
			Key:     "role",
			Label:   "Role",
			Options: []string{"admin", "editor", "viewer"},
		}),
		form.Confirm(form.ConfirmOptions{
			Key:     "remember",
			Label:   "Remember me?",
			Initial: true,
		}),
	}
	opts.SubmitText = "Sign in"
	s.form = form.New(opts)

	pOpts := t.Pane()
	pOpts.Title = "sign in"
	pOpts.Focused = true
	pOpts.ActiveBorder = lipgloss.NormalBorder()
	s.body = pane.New(pOpts)
}
