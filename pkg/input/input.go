// Package input provides a single-line text input wrapped in a pane — a
// theme-styled, bordered text field with a title slot on the border.
//
// Like every other tuilib component (filter, list, …), input owns its own
// pane: View() returns the bordered render. Don't wrap an input in another
// pane — set Options.Title to put the label on the border instead.
//
// Use input as the building block for any text-entry need:
//
//	in := input.New(theme.Dark().Input())
//	in.SetTitle("Name")
//	// in your screen's Update: in, cmd = in.Update(msg)
//	// in your screen's View:   string := in.View()
//
// Set Options.Echo to EchoMask for a password field. Masking is a display
// property only — Value returns the real text — so a "reveal" affordance is
// just SetEcho(input.EchoNormal) on a keypress. Reach for form.Password when
// the field belongs to a pkg/form rather than standing on its own.
//
// Reach for pkg/filter when you want the same input wired with the
// "/-to-focus, enter-commits, esc-clears" key behavior.
package input

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// EchoMode controls how typed characters are rendered. It affects the
// display only: Value always returns the real text.
type EchoMode int

const (
	// EchoNormal renders text as typed. The zero value, so an input is
	// never accidentally masked.
	EchoNormal EchoMode = iota
	// EchoMask renders MaskChar once per character — the password field.
	EchoMask
	// EchoNone renders nothing at all, the way a terminal passphrase prompt
	// takes input: the field looks empty however much is typed.
	EchoNone
)

// DefaultMaskChar is substituted per character under EchoMask when
// Options.MaskChar is unset.
const DefaultMaskChar = '•'

// echo maps to the underlying textinput's vocabulary.
func (e EchoMode) echo() textinput.EchoMode {
	switch e {
	case EchoMask:
		return textinput.EchoPassword
	case EchoNone:
		return textinput.EchoNone
	}
	return textinput.EchoNormal
}

// Options configures a new input. Zero-value fields fall back to sensible
// defaults so a caller can `input.New(input.Options{})` and get a working
// (un-themed) input. Mutate the typed-text styles via the *Style fields and
// the surrounding border via the pane pass-throughs.
type Options struct {
	// Width sets the pane's outer width. Height is fixed at 3.
	Width int

	// Title sits on the top border of the pane. Defaults to "input" — set it
	// to the field's label (e.g. "Name", "Email").
	Title string
	// Placeholder shows when the input is empty.
	Placeholder string
	// Initial pre-fills the value.
	Initial string
	// Prompt is rendered inline before the cursor inside the input. Defaults
	// to "" — input is the bare-text variant; use pkg/filter when you want
	// a prompt glyph.
	Prompt string
	// CharLimit caps input length. Defaults to 0 (unlimited).
	CharLimit int

	// Echo controls how typed characters are rendered. Defaults to
	// EchoNormal; set EchoMask for a password field.
	Echo EchoMode
	// MaskChar is the glyph EchoMask substitutes per character. Defaults to
	// DefaultMaskChar. Ignored under any other EchoMode.
	MaskChar rune

	// Text-input styling.
	PromptStyle      lipgloss.Style
	TextStyle        lipgloss.Style
	PlaceholderStyle lipgloss.Style
	CursorColor      lipgloss.TerminalColor

	// Pane pass-throughs. Unset fields fall back to NormalBorder both states
	// and SlotBracketsNone so the input reads as a plain bordered field.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
}

// Model is the input's exported state. Focus state lives on both the
// embedded textinput (so keys route correctly) and the pane (so the border
// color reflects focus) — toggle them together via Focus/Blur.
type Model struct {
	input textinput.Model
	pane  pane.Pane

	// echo is kept alongside the textinput's own EchoMode so Echo can report
	// in this package's vocabulary rather than mapping bubbles' back.
	echo EchoMode

	// token is this input's stable identity for focus requests. Update takes
	// a value receiver, so the model cannot name its own address.
	token focus.Token
}

// FocusToken returns the input's stable focus identity. See focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// handleMouse claims a press inside the input, focusing it and asking the
// group for the keyboard. Clicking a text field is the most direct way there
// is to say "type here", so it works whether or not the field already has
// focus.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if !e.IsPress() || !m.pane.Rect().Hit(e.X, e.Y) {
		return m, nil
	}
	return m, tea.Batch(m.Focus(), focus.RequestSelf(m.token))
}

// New constructs an input. The cursor does not blink until Focus is called
// and its returned tea.Cmd is propagated.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "input"
	}

	ti := textinput.New()
	ti.Prompt = opts.Prompt
	ti.Placeholder = opts.Placeholder
	ti.CharLimit = opts.CharLimit
	ti.EchoMode = opts.Echo.echo()
	if opts.MaskChar != 0 {
		ti.EchoCharacter = opts.MaskChar
	} else {
		ti.EchoCharacter = DefaultMaskChar
	}
	if opts.Initial != "" {
		ti.SetValue(opts.Initial)
	}
	ti.PromptStyle = opts.PromptStyle
	ti.TextStyle = opts.TextStyle
	ti.PlaceholderStyle = opts.PlaceholderStyle
	if opts.CursorColor != nil {
		ti.Cursor.Style = lipgloss.NewStyle().Foreground(opts.CursorColor)
	}
	if opts.Width > 0 {
		ti.Width = max(0, opts.Width-4) // pane border + padding
	}

	p := pane.New(pane.Options{
		Width:          opts.Width,
		Height:         3,
		Title:          opts.Title,
		SlotBrackets:   opts.SlotBrackets,
		ActiveColor:    opts.ActiveColor,
		InactiveColor:  opts.InactiveColor,
		ActiveBorder:   opts.ActiveBorder,
		InactiveBorder: opts.InactiveBorder,
	})
	p.SetContent(ti.View())

	return Model{
		token: focus.NewToken(), input: ti, pane: p, echo: opts.Echo}
}

// Init returns nil. Use Focus to start the cursor blink.
func (m Model) Init() tea.Cmd { return nil }

// Update forwards messages to the textinput. Esc is the one key intercepted:
// it blurs the field, leaving the value alone. Everything else — committing,
// clearing, what enter means — belongs to the caller (see pkg/filter for the
// "enter commits, esc clears" pattern).
//
// Esc is here because input reports IsCapturingKeys while focused, and
// focus.Group declines to cycle away from a capturing member. Without a key
// that releases it, an input in a Group is a one-way trap: tab is swallowed
// and nothing hands the keyboard back. Rule 27 tells the reader to "leave the
// field with enter or esc, then cycle" — for a bare input this is what makes
// that true.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if e, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(e)
	}
	if k, ok := msg.(tea.KeyMsg); ok && m.Focused() && k.Type == tea.KeyEsc {
		m.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.pane.SetContent(m.input.View())
	return m, cmd
}

// View renders the input as a bordered three-line pane.
func (m Model) View() string { return m.pane.View() }

// SetRect places the input at r. Height is fixed at 3 regardless of what the
// rect offers, since the pane is border + one content row + border.
func (m *Model) SetRect(r geom.Rect) {
	m.pane.SetRect(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 3, Gen: r.Gen})
	m.input.Width = max(0, r.W-4)
	m.pane.SetContent(m.input.View())
}

// Rect returns the rect the input was last placed at.
func (m Model) Rect() geom.Rect { return m.pane.Rect() }

// SetTitle sets the title shown on the pane's top border.
// SetTopRight writes into the pane's top-right border slot. Hosts use it for
// a short annotation that belongs to the component as a whole — pkg/form
// paints validation errors there. Pass "" to clear.
func (m *Model) SetTopRight(s string) { m.pane.SetTopRight(s) }

func (m *Model) SetTitle(s string) { m.pane.SetTitle(s) }

// Value returns the current text.
func (m Model) Value() string { return m.input.Value() }

// SetValue replaces the text.
func (m *Model) SetValue(s string) {
	m.input.SetValue(s)
	m.pane.SetContent(m.input.View())
}

// Echo reports how typed characters are currently rendered.
func (m Model) Echo() EchoMode { return m.echo }

// SetEcho changes how typed characters are rendered, leaving the value
// alone. This is the "reveal password" affordance: flip a masked field to
// EchoNormal and back without the user retyping anything.
func (m *Model) SetEcho(e EchoMode) {
	m.echo = e
	m.input.EchoMode = e.echo()
	m.pane.SetContent(m.input.View())
}

// Focus grabs focus and returns the cursor-blink command. Always propagate
// the cmd — without it the cursor won't blink.
func (m *Model) Focus() tea.Cmd {
	cmd := m.input.Focus()
	m.pane.SetFocused(true)
	m.pane.SetContent(m.input.View())
	return cmd
}

// Blur releases focus without touching the value.
func (m *Model) Blur() {
	m.input.Blur()
	m.pane.SetFocused(false)
	m.pane.SetContent(m.input.View())
}

// Focused reports whether the input is accepting keystrokes.
func (m Model) Focused() bool { return m.input.Focused() }

// IsCapturingKeys reports whether the input owns the keyboard — true
// whenever it is focused, since every printable belongs to the text field.
// Satisfies focus.Capturer.
func (m Model) IsCapturingKeys() bool { return m.Focused() }

// Help returns the keys this input "owns". Typing is implied and needs no
// hint; esc is advertised only while focused, since that is the only time it
// does anything.
func (m Model) Help() []key.Binding {
	if !m.Focused() {
		return nil
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "leave field")),
	}
}

// Reset clears the value and blurs.
func (m *Model) Reset() {
	m.input.Reset()
	m.input.Blur()
	m.pane.SetFocused(false)
	m.pane.SetContent(m.input.View())
}

// SetTextStyle updates the rendered style of typed text. Useful when reacting
// to a theme swap without rebuilding the model.
func (m *Model) SetTextStyle(s lipgloss.Style) {
	m.input.TextStyle = s
	m.pane.SetContent(m.input.View())
}

// SetPlaceholderStyle updates the rendered style of placeholder text.
func (m *Model) SetPlaceholderStyle(s lipgloss.Style) {
	m.input.PlaceholderStyle = s
	m.pane.SetContent(m.input.View())
}

// SetCursorColor updates the foreground color of the blinking cursor.
func (m *Model) SetCursorColor(c lipgloss.TerminalColor) {
	if c == nil {
		return
	}
	m.input.Cursor.Style = lipgloss.NewStyle().Foreground(c)
	m.pane.SetContent(m.input.View())
}

// SetActiveColor updates the border color used when focused.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.pane.SetActiveColor(c) }

// SetInactiveColor updates the border color used when unfocused.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.pane.SetInactiveColor(c) }

// SetActiveBorder updates the border shape drawn while focused.
func (m *Model) SetActiveBorder(b lipgloss.Border) { m.pane.SetActiveBorder(b) }

// SetInactiveBorder updates the border shape drawn while unfocused.
func (m *Model) SetInactiveBorder(b lipgloss.Border) { m.pane.SetInactiveBorder(b) }
