// Package screen defines a richer navigation model for pkg/app. A Screen owns
// its own layout tree, lifecycle, help bindings, and (optionally) a focus
// signal so the surrounding app shell can route global keys correctly.
//
// The Stack manages push/pop with result passing: a child can Pop with a
// value, and the parent's OnEnter receives it. This makes drilldown flows
// ("pick something, come back with the pick") trivially composable.
package screen

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Screen is the unit the Stack manages. Implementations are typically
// pointer types so Update/OnEnter/SetTheme can mutate local state.
type Screen interface {
	// Title labels the screen in the breadcrumb trail. Free to change over
	// the screen's lifetime (e.g., after an item is selected).
	Title() string

	// Init runs once when the screen is first constructed. Rarely non-nil.
	Init() tea.Cmd

	// Update handles a single tea.Msg. Return the screen (same pointer for
	// pointer-receiver implementations) and any follow-up command.
	Update(msg tea.Msg) (Screen, tea.Cmd)

	// Layout returns a layout tree. The app shell renders this inside the
	// body rect — the screen never sees the breadcrumb/statusbar chrome.
	Layout() layout.Node

	// Help returns the bindings shown inline in the statusbar footer.
	Help() []key.Binding

	// OnEnter runs each time the screen becomes the active top of the stack.
	//   - On the initial push:                 result == nil
	//   - When a child popped with a value:    result == that value
	// Return a tea.Cmd for any activation work (e.g. kick off a fetch).
	OnEnter(result any) tea.Cmd

	// SetTheme is called when the app swaps themes. Rebuild themed widgets
	// here, preserving state through accessors/setters (Cursor, Value, …).
	SetTheme(t theme.Theme)

	// IsCapturingKeys reports whether an embedded input (filter, textinput,
	// …) currently owns the keyboard. The app shell suppresses its global
	// keys (quit, theme-swap, …) while this is true.
	IsCapturingKeys() bool
}

// PushMsg is emitted by Push; the Stack handles it.
type PushMsg struct{ Screen Screen }

// PopMsg is emitted by Pop; the Stack handles it. Result is delivered to
// the newly-active screen's OnEnter.
type PopMsg struct{ Result any }

// Push returns a command that pushes s onto the stack.
func Push(s Screen) tea.Cmd {
	return func() tea.Msg { return PushMsg{Screen: s} }
}

// Pop returns a command that pops the current screen, passing result to
// whatever screen is uncovered. Pass nil to indicate "no result / cancel".
func Pop(result any) tea.Cmd {
	return func() tea.Msg { return PopMsg{Result: result} }
}

// Stack is the push/pop navigation stack. The last element is active.
type Stack struct {
	items []Screen
}

// NewStack constructs a stack with root at the bottom.
func NewStack(root Screen) Stack {
	return Stack{items: []Screen{root}}
}

// Init returns the root screen's Init + OnEnter(nil), to be run by the
// surrounding program.
func (s Stack) Init() tea.Cmd {
	if len(s.items) == 0 {
		return nil
	}
	root := s.items[0]
	return tea.Batch(root.Init(), root.OnEnter(nil))
}

// Current returns the active screen (top of stack).
func (s Stack) Current() Screen {
	if len(s.items) == 0 {
		return nil
	}
	return s.items[len(s.items)-1]
}

// Depth is the number of screens on the stack (1 = root only).
func (s Stack) Depth() int { return len(s.items) }

// Crumbs returns each screen's Title, root first. Feed to breadcrumb.
func (s Stack) Crumbs() []string {
	out := make([]string, len(s.items))
	for i, it := range s.items {
		out[i] = it.Title()
	}
	return out
}

// Update dispatches Push/Pop messages and forwards everything else to the
// active screen.
func (s Stack) Update(msg tea.Msg) (Stack, tea.Cmd) {
	switch m := msg.(type) {
	case PushMsg:
		s.items = append(s.items, m.Screen)
		return s, tea.Batch(m.Screen.Init(), m.Screen.OnEnter(nil))

	case PopMsg:
		if len(s.items) > 1 {
			s.items = s.items[:len(s.items)-1]
			top := s.items[len(s.items)-1]
			return s, top.OnEnter(m.Result)
		}
		return s, nil
	}

	if len(s.items) == 0 {
		return s, nil
	}
	top := s.items[len(s.items)-1]
	top, cmd := top.Update(msg)
	s.items[len(s.items)-1] = top
	return s, cmd
}

// SetTheme fans the new theme out to every screen on the stack. A screen
// that's not currently visible still gets the update so when the user pops
// back it renders in the new palette.
func (s Stack) SetTheme(t theme.Theme) {
	for _, sc := range s.items {
		sc.SetTheme(t)
	}
}
