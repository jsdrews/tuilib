// Package toggle provides a yes/no (boolean) selector wrapped in a pane —
// two labelled buttons rendered side-by-side with the active side highlighted,
// inside a bordered titled box.
//
// Like every other tuilib component (input, filter, list, …), toggle owns
// its own pane: View() returns the bordered render. Don't wrap a toggle in
// another pane — set Options.Title to put the label on the border instead.
//
// While focused, left/right/h/l/space toggle, and y/n set explicitly:
//
//	tg := toggle.New(theme.Dark().Toggle())
//	tg.SetTitle("Send notifications?")
//	// in your screen's Update: tg, _ = tg.Update(msg)
//	// later: send := tg.Value()
package toggle

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/pane"
)

// Options configures a new toggle. Zero-value fields fall back to defaults.
type Options struct {
	// Width sets the pane's outer width. Height is fixed at 3.
	Width int

	// Title sits on the top border of the pane. Defaults to "toggle" — set
	// it to the field's question (e.g. "Send notifications?").
	Title string

	// Initial is the starting value. Defaults to false.
	Initial bool
	// YesLabel and NoLabel are the rendered button labels. Default to "yes"
	// and "no" — wrap your own text (e.g. "on"/"off", "save"/"discard").
	YesLabel string
	NoLabel  string

	// SelectedStyle styles the active side, brackets included.
	SelectedStyle lipgloss.Style
	// UnselectedStyle styles the inactive side.
	UnselectedStyle lipgloss.Style

	// Pane pass-throughs. Unset fields fall back to NormalBorder both states
	// and SlotBracketsNone.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
}

// Model is the toggle's exported state. Focus state lives on both the
// internal flag and the pane (so the border color reflects focus) — toggle
// them together via Focus/Blur.
type Model struct {
	value           bool
	focused         bool
	yesLabel        string
	noLabel         string
	selectedStyle   lipgloss.Style
	unselectedStyle lipgloss.Style

	pane pane.Pane
}

// New constructs a toggle.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "toggle"
	}
	if opts.YesLabel == "" {
		opts.YesLabel = "yes"
	}
	if opts.NoLabel == "" {
		opts.NoLabel = "no"
	}
	if (opts.ActiveBorder == lipgloss.Border{}) {
		opts.ActiveBorder = lipgloss.NormalBorder()
	}
	if (opts.InactiveBorder == lipgloss.Border{}) {
		opts.InactiveBorder = lipgloss.NormalBorder()
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

	m := Model{
		value:           opts.Initial,
		yesLabel:        opts.YesLabel,
		noLabel:         opts.NoLabel,
		selectedStyle:   opts.SelectedStyle,
		unselectedStyle: opts.UnselectedStyle,
		pane:            p,
	}
	m.pane.SetContent(m.renderInner())
	return m
}

// Init is a no-op.
func (m Model) Init() tea.Cmd { return nil }

// Update handles toggle keys when focused; no-op when blurred. The caller
// should still forward every message — the toggle decides whether to act.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "left", "right", "h", "l", " ":
			m.value = !m.value
		case "y", "Y":
			m.value = true
		case "n", "N":
			m.value = false
		}
	}
	m.pane.SetContent(m.renderInner())
	return m, nil
}

// View renders the toggle as a bordered three-line pane.
func (m Model) View() string { return m.pane.View() }

// SetWidth resizes the surrounding pane. Height is fixed at 3.
func (m *Model) SetWidth(w int) {
	m.pane.SetDimensions(w, 3)
	m.pane.SetContent(m.renderInner())
}

// SetTitle sets the title shown on the pane's top border.
func (m *Model) SetTitle(s string) { m.pane.SetTitle(s) }

// Value returns the current bool.
func (m Model) Value() bool { return m.value }

// SetValue overwrites the value.
func (m *Model) SetValue(v bool) {
	m.value = v
	m.pane.SetContent(m.renderInner())
}

// Focus grabs focus. Returns nil — there's no cursor to blink.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	m.pane.SetFocused(true)
	return nil
}

// Blur releases focus.
func (m *Model) Blur() {
	m.focused = false
	m.pane.SetFocused(false)
}

// Focused reports whether the toggle is accepting keys.
func (m Model) Focused() bool { return m.focused }

// SetSelectedStyle updates the rendered style of the active side. Useful when
// reacting to a theme swap without rebuilding the model.
func (m *Model) SetSelectedStyle(s lipgloss.Style) {
	m.selectedStyle = s
	m.pane.SetContent(m.renderInner())
}

// SetUnselectedStyle updates the rendered style of the inactive side.
func (m *Model) SetUnselectedStyle(s lipgloss.Style) {
	m.unselectedStyle = s
	m.pane.SetContent(m.renderInner())
}

// Help returns the keys this toggle responds to. Compose into a screen's
// Help() to drive a per-component help line.
func (m Model) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←→", "flip")),
		key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
	}
}

// SetActiveColor updates the border color used when focused.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.pane.SetActiveColor(c) }

// SetInactiveColor updates the border color used when unfocused.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.pane.SetInactiveColor(c) }

// renderInner produces the content rendered inside the pane.
func (m Model) renderInner() string {
	yes := "[ " + m.yesLabel + " ]"
	no := "[ " + m.noLabel + " ]"
	if m.value {
		yes = m.selectedStyle.Render(yes)
		no = m.unselectedStyle.Render(no)
	} else {
		yes = m.unselectedStyle.Render(yes)
		no = m.selectedStyle.Render(no)
	}
	return yes + "  " + no
}
