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
// Reach for pkg/filter when you want the same input wired with the
// "/-to-focus, enter-commits, esc-clears" key behavior.
package input

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/pane"
)

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
}

// New constructs an input. The cursor does not blink until Focus is called
// and its returned tea.Cmd is propagated.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "input"
	}
	if (opts.ActiveBorder == lipgloss.Border{}) {
		opts.ActiveBorder = lipgloss.NormalBorder()
	}
	if (opts.InactiveBorder == lipgloss.Border{}) {
		opts.InactiveBorder = lipgloss.NormalBorder()
	}

	ti := textinput.New()
	ti.Prompt = opts.Prompt
	ti.Placeholder = opts.Placeholder
	ti.CharLimit = opts.CharLimit
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

	return Model{input: ti, pane: p}
}

// Init returns nil. Use Focus to start the cursor blink.
func (m Model) Init() tea.Cmd { return nil }

// Update forwards messages to the textinput. No keys are intercepted —
// commit/cancel semantics belong to the caller (see pkg/filter for the
// "enter commits, esc clears" pattern).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.pane.SetContent(m.input.View())
	return m, cmd
}

// View renders the input as a bordered three-line pane.
func (m Model) View() string { return m.pane.View() }

// SetWidth resizes the surrounding pane. Height is fixed at 3.
func (m *Model) SetWidth(w int) {
	m.pane.SetDimensions(w, 3)
	m.input.Width = max(0, w-4)
	m.pane.SetContent(m.input.View())
}

// SetTitle sets the title shown on the pane's top border.
func (m *Model) SetTitle(s string) { m.pane.SetTitle(s) }

// Value returns the current text.
func (m Model) Value() string { return m.input.Value() }

// SetValue replaces the text.
func (m *Model) SetValue(s string) {
	m.input.SetValue(s)
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

// Help returns the keys this input "owns" — there are no special
// shortcuts (typing is implied), so the slice is empty. Kept for
// interface symmetry with other components.
func (m Model) Help() []key.Binding { return nil }

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
