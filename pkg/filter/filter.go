// Package filter provides a single-line textinput wrapped in a pane — the
// "press / to search, enter to commit, esc to clear" pattern every TUI
// eventually needs. Model owns its focus state and the commit/cancel key
// handling; the caller reads Value() after each Update to drive whatever
// list or table is being filtered. SetBottomLeft / SetBottomRight expose
// the surrounding pane's bottom-border slots so a host (e.g. pkg/table)
// can paint hints, completion previews, or counts into the filter chrome
// without wrapping it in a second pane.
package filter

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/geom"
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
	// inline is set when a host renders this filter inside its own pane;
	// rect is then the host-assigned row rather than the filter's own pane.
	inline bool
	rect   geom.Rect

	input textinput.Model
	pane  pane.Pane

	// basePrompt is the caller's prompt style; the foreground is swapped
	// per focus state so an inline filter (one with no border of its own)
	// still shows where input is going.
	basePrompt    lipgloss.Style
	activeColor   lipgloss.TerminalColor
	inactiveColor lipgloss.TerminalColor
}

// applyPromptStyle tints the prompt by focus state. Inline filters have no
// border to light up, so the prompt carries that signal instead.
func (m *Model) applyPromptStyle() {
	c := m.inactiveColor
	if m.input.Focused() {
		c = m.activeColor
	}
	if c != nil {
		m.input.PromptStyle = m.basePrompt.Foreground(c)
	}
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

	m := Model{
		input:         ti,
		pane:          p,
		basePrompt:    opts.PromptStyle,
		activeColor:   opts.ActiveColor,
		inactiveColor: opts.InactiveColor,
	}
	m.applyPromptStyle()
	m.pane.SetContent(m.input.View())
	return m
}

// Focus grabs focus and returns the cursor-blink command. Always propagate
// the cmd — without it the cursor won't blink.
func (m *Model) Focus() tea.Cmd {
	cmd := m.input.Focus()
	m.applyPromptStyle()
	m.pane.SetFocused(true)
	m.pane.SetContent(m.input.View())
	return cmd
}

// Blur releases focus without touching the value. See Reset for clearing.
func (m *Model) Blur() {
	m.input.Blur()
	m.applyPromptStyle()
	m.pane.SetFocused(false)
	m.pane.SetContent(m.input.View())
}

// Focused reports whether the filter is currently accepting keystrokes.
func (m Model) Focused() bool { return m.input.Focused() }

// IsCapturingKeys reports whether the filter owns the keyboard — true
// whenever it is focused. Satisfies focus.Capturer.
func (m Model) IsCapturingKeys() bool { return m.Focused() }

// Help returns the keys the filter responds to. Empty when blurred
// (the parent owns the "/" focus binding); commit/clear when focused.
func (m Model) Help() []key.Binding {
	if !m.input.Focused() {
		return nil
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "apply")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
	}
}

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
	m.applyPromptStyle()
	m.pane.SetFocused(false)
	m.pane.SetContent(m.input.View())
}

// SetRect places the filter at r. Height is fixed at 3 regardless of what
// the rect offers, since the pane is border + one content row + border.
func (m *Model) SetRect(r geom.Rect) {
	m.pane.SetRect(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 3, Gen: r.Gen})
}

// Rect returns the rect the filter was last placed at — its own pane's rect
// normally, or the host-assigned row when inline.
func (m Model) Rect() geom.Rect {
	if m.inline {
		return m.rect
	}
	return m.pane.Rect()
}

// SetInlineRect places the filter as a single row inside a host component's
// pane, rather than as a pane of its own. The host renders InlineView into
// its pane header; this call is what makes the filter's own hit-testing
// agree with where the host actually drew it.
func (m *Model) SetInlineRect(r geom.Rect) {
	m.inline = true
	m.rect = r
	m.input.Width = max(0, r.W-lipgloss.Width(m.input.Prompt)-1)
	m.pane.SetContent(m.input.View())
}

// InlineView renders just the input line — no border, no pane. Used by
// components that host the filter inside their own pane so the filter reads
// as part of the thing it filters instead of a sibling box floating above
// it.
func (m Model) InlineView() string { return m.input.View() }

// Inline reports whether the filter is rendering as a row inside a host's
// pane rather than as its own pane.
func (m Model) Inline() bool { return m.inline }

// SetBottomLeft / SetBottomRight write into the surrounding pane's bottom
// border slots. Useful for hints, completion previews, or counts that
// pertain to whatever the filter is driving. Pass "" to clear.
func (m *Model) SetBottomLeft(s string)  { m.pane.SetBottomLeft(s) }
func (m *Model) SetBottomRight(s string) { m.pane.SetBottomRight(s) }

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
