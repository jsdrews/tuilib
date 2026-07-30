// Package focus owns "which component currently has the keyboard" for a
// screen with more than one interactive component.
//
// Before this package, every multi-component screen hand-rolled the same
// thing: an int index, a switch in Update for tab/shift-tab, and an
// applyFocus helper that blurred everything and focused one. Group replaces
// that, and adds the piece a hand-rolled index can't do — granting focus in
// response to a mouse click, which arrives at the clicked component rather
// than at the screen that owns the ordering.
//
// A screen holds a Group alongside its components, forwards messages to it,
// and reads Focused to decide where its own keys go:
//
//	func newScreen(t theme.Theme) *screen {
//	    s := &screen{…}
//	    s.focus = focus.NewGroup(&s.query, &s.results, &s.caseTgl)
//	    return s
//	}
//
//	func (s *screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
//	    var cmd tea.Cmd
//	    s.focus, cmd = s.focus.Update(msg)
//	    …
//	}
//
// # Click-to-focus
//
// A component that decides a click landed on it returns Request(itself) as a
// tea.Cmd. The resulting RequestMsg travels up to whatever Group holds it;
// the Group blurs everything else and focuses the target. The component never
// has to know its siblings exist, and the Group never has to know how any
// component decides it was clicked.
//
// A RequestMsg naming something the Group doesn't hold is ignored, so nesting
// Groups is safe: each takes only the requests it recognises.
package focus

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Focusable is anything a Group can move focus between. Every interactive
// component in tuilib satisfies it.
//
// Focus returns a tea.Cmd because some components need one — a text input
// returns its cursor-blink command. Components with nothing to start return
// nil.
type Focusable interface {
	Focus() tea.Cmd
	Blur()
	Focused() bool
}

// Capturer is an optional interface for components that swallow printable
// keys while focused — a text field, or a list with its filter engaged. A
// Group reports IsCapturingKeys from its focused member when that member
// implements this; the screen forwards the answer to the app shell so global
// keys (q, theme-cycle, esc-pop) stay out of the way.
type Capturer interface {
	IsCapturingKeys() bool
}

// Token is a component's stable identity, handed out by NewToken and held as
// a field. It exists because bubbletea components take a value receiver on
// Update and return a new copy, so a component cannot refer to its own
// address — &m inside Update names a temporary. The token is copied along
// with every copy of the model, so it stays the same value no matter how
// many times the model is passed around.
type Token *struct{ name string }

// NewToken mints a fresh identity. Components call it once, in their
// constructor, and keep the result for the model's lifetime.
func NewToken() Token { return &struct{ name string }{} }

// Identified is implemented by components carrying a Token. A Group matches
// incoming requests against it, which is how a click that started inside a
// component finds its way back to the Group that owns focus ordering.
type Identified interface {
	FocusToken() Token
}

// RequestMsg asks whichever Group owns the named component to give it focus.
// Target is set when the requester has the component's address; Token is set
// when a component is asking on its own behalf. A Group matches on either.
type RequestMsg struct {
	Target Focusable
	Token  Token
}

// Request returns a command carrying a focus request for target. Use it when
// you hold the component — a screen focusing one of its own panes.
func Request(target Focusable) tea.Cmd {
	return func() tea.Msg { return RequestMsg{Target: target} }
}

// RequestSelf returns a command by which a component asks for focus on its
// own behalf, naming itself by token. This is what a component returns from
// Update when a click lands inside its rect.
func RequestSelf(tk Token) tea.Cmd {
	return func() tea.Msg { return RequestMsg{Token: tk} }
}

// matches reports whether it is the component req names.
func matches(it Focusable, req RequestMsg) bool {
	if req.Target != nil && it == req.Target {
		return true
	}
	if req.Token != nil {
		if id, ok := it.(Identified); ok && id.FocusToken() == req.Token {
			return true
		}
	}
	return false
}

// Keys are the bindings a Group dispatches against. Zero-valued fields fall
// back to the defaults from DefaultKeys.
type Keys struct {
	Next key.Binding
	Prev key.Binding
}

// DefaultKeys returns tab / shift+tab, the library-wide focus cycling pair.
// These are deliberately not rebound elsewhere: pkg/tab uses shift+left and
// shift+right for tab switching precisely so tab stays free for focus.
func DefaultKeys() Keys {
	return Keys{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "next field")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev field")),
	}
}

// Group holds an ordered set of focusables and grants focus to exactly one.
type Group struct {
	items []Focusable
	idx   int
	keys  Keys
	// wrap controls whether cycling past either end wraps around.
	wrap bool
}

// NewGroup returns a Group over items in tab order, with the first item
// focused. Pass pointers — the Group stores the interface values and calls
// through them, so a component rebuilt in place (as SetTheme does) stays
// addressed correctly as long as the field address is stable.
//
// Call the returned Group's Init to focus the first item and collect its
// command.
func NewGroup(items ...Focusable) Group {
	return Group{items: items, keys: DefaultKeys(), wrap: true}
}

// WithKeys returns a copy of g using custom cycling bindings. Zero-valued
// fields keep their defaults.
func (g Group) WithKeys(k Keys) Group {
	if k.Next.Keys() != nil {
		g.keys.Next = k.Next
	}
	if k.Prev.Keys() != nil {
		g.keys.Prev = k.Prev
	}
	return g
}

// WithoutWrap returns a copy of g where cycling stops at the ends rather than
// wrapping around.
func (g Group) WithoutWrap() Group {
	g.wrap = false
	return g
}

// Init focuses the first item and returns its command. Batch it into the
// screen's Init.
func (g Group) Init() tea.Cmd { return g.apply() }

// Update handles the cycling keys and focus requests. Everything else passes
// through untouched — the screen still forwards each message to its
// components itself, since a Group tracks focus, not content.
func (g Group) Update(msg tea.Msg) (Group, tea.Cmd) {
	switch msg := msg.(type) {
	case RequestMsg:
		for i, it := range g.items {
			if matches(it, msg) {
				if i == g.idx {
					return g, nil
				}
				g.idx = i
				return g, g.apply()
			}
		}
		// Not ours — a nested Group elsewhere owns this target.
		return g, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, g.keys.Next):
			return g.step(1)
		case key.Matches(msg, g.keys.Prev):
			return g.step(-1)
		}
	}
	return g, nil
}

// step moves focus by delta, wrapping unless WithoutWrap was applied.
func (g Group) step(delta int) (Group, tea.Cmd) {
	if len(g.items) == 0 {
		return g, nil
	}
	next := g.idx + delta
	if g.wrap {
		n := len(g.items)
		next = ((next % n) + n) % n
	} else if next < 0 || next >= len(g.items) {
		return g, nil
	}
	g.idx = next
	return g, g.apply()
}

// apply blurs every item but the focused one and returns the focused item's
// command.
func (g Group) apply() tea.Cmd {
	var cmd tea.Cmd
	for i, it := range g.items {
		if i == g.idx {
			cmd = it.Focus()
			continue
		}
		it.Blur()
	}
	return cmd
}

// Index returns the focused item's position in tab order.
func (g Group) Index() int { return g.idx }

// SetIndex focuses the item at i, clamped to the group's bounds. Useful for
// carrying focus across a SetTheme rebuild.
func (g *Group) SetIndex(i int) tea.Cmd {
	if len(g.items) == 0 {
		return nil
	}
	if i < 0 {
		i = 0
	}
	if i >= len(g.items) {
		i = len(g.items) - 1
	}
	g.idx = i
	return g.apply()
}

// Focused returns the item that currently owns focus, or nil for an empty
// group.
func (g Group) Focused() Focusable {
	if len(g.items) == 0 {
		return nil
	}
	return g.items[g.idx]
}

// Is reports whether f is the currently focused item. Screens use it to
// route their own shortcuts to the right pane.
func (g Group) Is(f Focusable) bool { return g.Focused() == f }

// Len returns how many items the group holds.
func (g Group) Len() int { return len(g.items) }

// IsCapturingKeys reports whether the focused item is currently swallowing
// printable keys. Screens forward this from their own IsCapturingKeys so the
// app shell suppresses its global keys while a text field or an engaged
// filter owns input. Items that don't implement Capturer never capture.
func (g Group) IsCapturingKeys() bool {
	if c, ok := g.Focused().(Capturer); ok {
		return c.IsCapturingKeys()
	}
	return false
}

// Help returns the cycling bindings, plus the focused item's own when it
// exposes them. Compose into the screen's Help so the hint strip tracks
// whichever pane is active.
func (g Group) Help() []key.Binding {
	out := []key.Binding{g.keys.Next, g.keys.Prev}
	if h, ok := g.Focused().(interface{ Help() []key.Binding }); ok {
		out = append(out, h.Help()...)
	}
	return out
}
