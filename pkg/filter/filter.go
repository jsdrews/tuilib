// Package filter provides a single-line textinput wrapped in a pane — the
// "press / to search, enter to commit, esc to clear" pattern every TUI
// eventually needs. Model owns its focus state and the commit/cancel key
// handling; the caller reads Value() after each Update to drive whatever
// list or table is being filtered.
package filter

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/pane"
)

// Options configures a new filter. Zero-value fields fall back to sensible
// defaults so a caller can `filter.New(filter.Options{Width: w})` and get a
// working, un-themed filter bar.
type Options struct {
	Width int

	// Title sits on the top border of the pane. Defaults to "filter".
	Title string
	// Prompt appears before the cursor inside the input. Defaults to "/ ".
	Prompt string
	// Placeholder shows when the input is empty.
	Placeholder string
	// CharLimit caps input length. Defaults to 64.
	CharLimit int

	// Text-input styling.
	PromptStyle      lipgloss.Style
	TextStyle        lipgloss.Style
	PlaceholderStyle lipgloss.Style
	CursorColor      lipgloss.TerminalColor

	// Pane pass-throughs. Unset fields fall back to filter's defaults, which
	// differ from the base pane's (NormalBorder both states, no slot brackets)
	// because a filter bar reads cleaner without thick borders or corner tabs.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
}

// Model is the filter's exported state. Focus state lives on both the
// embedded textinput (so keys route correctly) and the pane (so the border
// color reflects focus) — toggle them together via Focus/Blur.
type Model struct {
	input textinput.Model
	pane  pane.Pane
}

// New constructs a filter. Call Update/View from the parent model; use
// Focus/Blur/Value/Reset to drive it.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "filter"
	}
	if opts.Prompt == "" {
		opts.Prompt = "/ "
	}
	if opts.CharLimit == 0 {
		opts.CharLimit = 64
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
	ti.PromptStyle = opts.PromptStyle
	ti.TextStyle = opts.TextStyle
	ti.PlaceholderStyle = opts.PlaceholderStyle
	if opts.CursorColor != nil {
		ti.Cursor.Style = lipgloss.NewStyle().Foreground(opts.CursorColor)
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

// Focus grabs focus and returns the cursor-blink command. Always propagate
// the cmd — without it the cursor won't blink.
func (m *Model) Focus() tea.Cmd {
	cmd := m.input.Focus()
	m.pane.SetFocused(true)
	m.pane.SetContent(m.input.View())
	return cmd
}

// Blur releases focus without touching the value. See Reset for clearing.
func (m *Model) Blur() {
	m.input.Blur()
	m.pane.SetFocused(false)
	m.pane.SetContent(m.input.View())
}

// Focused reports whether the filter is currently accepting keystrokes.
func (m Model) Focused() bool { return m.input.Focused() }

// Value is the current filter text. Read after every Update to drive the
// downstream list/table filter.
func (m Model) Value() string { return m.input.Value() }

// SetValue overwrites the current filter text. Useful when rebuilding the
// filter on theme swap / resize — carry the old Value() across.
func (m *Model) SetValue(s string) {
	m.input.SetValue(s)
	m.pane.SetContent(m.input.View())
}

// Reset clears the value and blurs.
func (m *Model) Reset() {
	m.input.Reset()
	m.input.Blur()
	m.pane.SetFocused(false)
	m.pane.SetContent(m.input.View())
}

// SetWidth resizes the surrounding pane. Height is fixed at 3.
func (m *Model) SetWidth(w int) { m.pane.SetDimensions(w, 3) }

// Update is a no-op when blurred. When focused, "enter" commits (blur, keep
// value) and "esc" cancels (reset + blur); anything else is forwarded to the
// textinput. The caller should still forward every message — the filter
// decides whether to act on it.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.input.Focused() {
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "enter":
			m.Blur()
			return m, nil
		case "esc":
			m.Reset()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.pane.SetContent(m.input.View())
	return m, cmd
}

// View renders the filter as a bordered three-line pane.
func (m Model) View() string { return m.pane.View() }
