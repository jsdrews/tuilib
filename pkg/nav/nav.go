// Package nav is a navigation stack for Bubble Tea TUIs.
//
// Screens are pushed on top of the stack when the user drills deeper and
// popped when they go back. The top of the stack is the active screen:
// messages flow to it, and its View() is what gets rendered. Each screen
// carries a Title() which feeds the breadcrumb trail (see pkg/breadcrumb).
//
// Navigation is initiated by screens themselves: on Enter (or whatever the
// screen chooses), a screen returns nav.Push(newScreen) as a tea.Cmd. nav
// intercepts the resulting PushMsg and handles the state transition. Esc is
// handled automatically when AutoEscPop is true (the default): if depth > 1,
// it pops; otherwise it passes through to the screen.
package nav

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Screen is the unit of navigation. Each screen is a self-contained Bubble
// Tea-like model that also knows its Title (used for breadcrumbs).
type Screen interface {
	Title() string
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
}

// PushMsg tells nav.Model to push a new screen.
type PushMsg struct{ Screen Screen }

// PopMsg tells nav.Model to pop the current screen.
type PopMsg struct{}

// Push returns a tea.Cmd that pushes screen onto the nav stack.
func Push(screen Screen) tea.Cmd {
	return func() tea.Msg { return PushMsg{Screen: screen} }
}

// Pop returns a tea.Cmd that pops the current screen from the nav stack.
func Pop() tea.Cmd {
	return func() tea.Msg { return PopMsg{} }
}

// Options configures a nav.Model.
type Options struct {
	// Root is the screen at the base of the stack. Required.
	Root Screen
	// DisableAutoEscPop turns off automatic esc-to-pop handling. When false
	// (default), esc pops the stack when depth > 1.
	DisableAutoEscPop bool
}

// Model is a navigation stack wrapped as a Bubble Tea model. Embed it in a
// parent model, forward Update calls to it, and read Crumbs() for breadcrumbs.
type Model struct {
	stack      []Screen
	autoEscPop bool
}

// New constructs a nav.Model with the given root screen.
func New(opts Options) Model {
	return Model{
		stack:      []Screen{opts.Root},
		autoEscPop: !opts.DisableAutoEscPop,
	}
}

func (m Model) Init() tea.Cmd {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[0].Init()
}

// Update handles push/pop and forwards all other messages to the current
// screen. PushMsg appends and runs the new screen's Init. PopMsg and esc
// (when AutoEscPop) remove the top screen if depth > 1.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PushMsg:
		m.stack = append(m.stack, msg.Screen)
		return m, msg.Screen.Init()
	case PopMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil
	case tea.KeyMsg:
		if m.autoEscPop && msg.String() == "esc" && len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
			return m, nil
		}
	}

	if len(m.stack) == 0 {
		return m, nil
	}
	top := m.stack[len(m.stack)-1]
	top, cmd := top.Update(msg)
	m.stack[len(m.stack)-1] = top
	return m, cmd
}

// View renders the current screen.
func (m Model) View() string {
	if len(m.stack) == 0 {
		return ""
	}
	return m.stack[len(m.stack)-1].View()
}

// Current returns the active screen (top of stack).
func (m Model) Current() Screen {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

// Depth is the current stack depth (1 = root only).
func (m Model) Depth() int { return len(m.stack) }

// Crumbs returns the Title of every screen in the stack, root first. Feed
// this directly to breadcrumb.Model.SetCrumbs.
func (m Model) Crumbs() []string {
	out := make([]string, len(m.stack))
	for i, s := range m.stack {
		out[i] = s.Title()
	}
	return out
}
