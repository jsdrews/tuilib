// Package confirm provides a modal yes/no dialog component. It renders a
// bordered titled pane with a message body and two side-by-side buttons,
// and resolves to a ConfirmedMsg or CancelledMsg via tea.Cmd when the user
// commits a choice.
//
// The component is meant to be hosted in the standard modal pattern:
//
//	if s.confirmUp {
//	    return layout.ZStack(
//	        baseLayout,
//	        layout.Center(48, 7, layout.Sized(&s.confirm)),
//	    )
//	}
//	return baseLayout
//
// While the modal is up, the parent screen returns true from
// IsCapturingKeys() so the app shell suppresses its global keys (q, t,
// esc-pop) and the modal owns input. The parent also forwards every
// tea.Msg to the confirm and matches confirm.ConfirmedMsg /
// confirm.CancelledMsg in its own Update to dismiss the modal and act
// on the result.
//
// Keys (always active when the modal is rendered):
//
//	left / right / h / l   move selection
//	tab / shift+tab        move selection
//	y / Y                  commit Yes
//	n / N                  commit No
//	enter                  commit current selection
//	esc                    commit No
package confirm

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// ConfirmedMsg is emitted when the user picks the confirm button.
type ConfirmedMsg struct{}

// CancelledMsg is emitted when the user picks the cancel button (or hits esc).
type CancelledMsg struct{}

// Options configures a new confirm dialog. Zero-value fields fall back to
// defaults; start from theme.Confirm() to fill in the color tokens.
type Options struct {
	// Width and Height set the pane's outer dimensions. SetRect
	// overrides these later if the host re-sizes the modal.
	Width, Height int

	// Title sits on the top border (e.g. "Cancel Run", "Delete file"). Defaults
	// to "confirm".
	Title string

	// Message is the body text shown above the buttons. Multi-line strings are
	// supported; the component does not wrap.
	Message string

	// Confirm and Cancel are the rendered button labels. Default to "Yes"
	// and "No" — wrap your own text ("Yes, cancel", "Keep running").
	Confirm string
	Cancel  string

	// Initial picks which side starts highlighted. false (the zero value)
	// puts the cursor on Cancel, which is the safer default for destructive
	// actions. Set true when the affirmative is the expected path.
	Initial bool

	// SelectedStyle styles the active button. Brackets are part of the render.
	SelectedStyle lipgloss.Style
	// UnselectedStyle styles the inactive button.
	UnselectedStyle lipgloss.Style

	// MessageStyle styles the body text. Defaults to no styling.
	MessageStyle lipgloss.Style

	// Pane pass-throughs. Unset fields fall back to ThickBorder when active,
	// NormalBorder when inactive, and SlotBracketsNone.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
}

// Model is the confirm dialog's exported state.
type Model struct {
	value           bool
	confirmLabel    string
	cancelLabel     string
	message         string
	selectedStyle   lipgloss.Style
	unselectedStyle lipgloss.Style
	messageStyle    lipgloss.Style

	pane pane.Pane
}

// New constructs a confirm dialog. Always renders in the active border
// state — the modal owns input whenever it's visible, so there's no
// "blurred" state to model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "confirm"
	}
	if opts.Confirm == "" {
		opts.Confirm = "Yes"
	}
	if opts.Cancel == "" {
		opts.Cancel = "No"
	}

	p := pane.New(pane.Options{
		Width:          opts.Width,
		Height:         opts.Height,
		Title:          opts.Title,
		Focused:        true,
		SlotBrackets:   opts.SlotBrackets,
		ActiveColor:    opts.ActiveColor,
		InactiveColor:  opts.InactiveColor,
		ActiveBorder:   opts.ActiveBorder,
		InactiveBorder: opts.InactiveBorder,
	})

	m := Model{
		value:           opts.Initial,
		confirmLabel:    opts.Confirm,
		cancelLabel:     opts.Cancel,
		message:         opts.Message,
		selectedStyle:   opts.SelectedStyle,
		unselectedStyle: opts.UnselectedStyle,
		messageStyle:    opts.MessageStyle,
		pane:            p,
	}
	m.pane.SetContent(m.renderInner())
	return m
}

// Init is a no-op.
func (m Model) Init() tea.Cmd { return nil }

// Update handles selection movement and commit. Returns a tea.Cmd carrying
// ConfirmedMsg or CancelledMsg when the user commits; the parent screen
// matches the result in its own Update to dismiss the modal and act.
//
// All keys are active — the modal is the only thing focused while it's up.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if mm, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(mm)
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "left", "right", "h", "l", "tab", "shift+tab":
		m.value = !m.value
		m.pane.SetContent(m.renderInner())
		return m, nil
	case "y", "Y":
		return m, confirmed()
	case "n", "N", "esc":
		return m, cancelled()
	case "enter":
		if m.value {
			return m, confirmed()
		}
		return m, cancelled()
	}
	return m, nil
}

// View renders the dialog as a bordered pane.
func (m Model) View() string { return m.pane.View() }

// SetRect places the surrounding pane at r.
func (m *Model) SetRect(r geom.Rect) {
	m.pane.SetRect(r)
	m.pane.SetContent(m.renderInner())
}

// Rect returns the rect the modal was last placed at.
func (m Model) Rect() geom.Rect { return m.pane.Rect() }

// SetTitle sets the title shown on the pane's top border.
func (m *Model) SetTitle(s string) { m.pane.SetTitle(s) }

// SetMessage updates the body text. Useful when the modal is reused across
// different actions without rebuilding the model.
func (m *Model) SetMessage(s string) {
	m.message = s
	m.pane.SetContent(m.renderInner())
}

// Value reports the current selection. true = confirm, false = cancel.
func (m Model) Value() bool { return m.value }

// SetValue overwrites the current selection.
func (m *Model) SetValue(v bool) {
	m.value = v
	m.pane.SetContent(m.renderInner())
}

// SetSelectedStyle updates the highlighted-button style. Useful when reacting
// to a theme swap without rebuilding the model.
func (m *Model) SetSelectedStyle(s lipgloss.Style) {
	m.selectedStyle = s
	m.pane.SetContent(m.renderInner())
}

// SetUnselectedStyle updates the dimmed-button style.
func (m *Model) SetUnselectedStyle(s lipgloss.Style) {
	m.unselectedStyle = s
	m.pane.SetContent(m.renderInner())
}

// SetMessageStyle updates the body-text style.
func (m *Model) SetMessageStyle(s lipgloss.Style) {
	m.messageStyle = s
	m.pane.SetContent(m.renderInner())
}

// SetActiveColor updates the active border color.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.pane.SetActiveColor(c) }

// SetInactiveColor updates the inactive border color.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.pane.SetInactiveColor(c) }

// Help returns the keys this dialog responds to. Compose into the parent's
// Help() while the modal is up so the hint strip reflects the modal context.
func (m Model) Help() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←→", "select")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "commit")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// renderInner produces the content rendered inside the pane: message body
// on top, an empty line, then the two buttons centered on the last row.
func (m Model) renderInner() string {
	yes := "[ " + m.confirmLabel + " ]"
	no := "[ " + m.cancelLabel + " ]"
	if m.value {
		yes = m.selectedStyle.Render(yes)
		no = m.unselectedStyle.Render(no)
	} else {
		yes = m.unselectedStyle.Render(yes)
		no = m.selectedStyle.Render(no)
	}
	buttons := no + "  " + yes

	body := m.messageStyle.Render(m.message)

	parts := []string{body}
	if body != "" {
		parts = append(parts, "")
	}
	parts = append(parts, buttons)
	return strings.Join(parts, "\n")
}

// handleMouse resolves a click on one of the two buttons.
//
// Clicking a button commits it outright rather than merely highlighting it:
// a mouse user who presses "Delete" has already made the choice, and asking
// them to click once to select and again to confirm would be a worse dialog,
// not a safer one. Moving the highlight first would also make a stray click
// on the destructive side arm it for a subsequent enter, which is the
// failure mode Initial=false exists to avoid.
//
// A click anywhere else inside the modal is swallowed — the modal owns input
// while it is up, and a click on its own chrome should not leak through to
// whatever it covers. Clicks outside it do nothing; dismissing is esc's job,
// since click-outside-to-cancel is too easy to trigger by accident on a
// destructive action.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if !e.IsPress() || !m.pane.Rect().Hit(e.X, e.Y) {
		return m, nil
	}
	switch btn, ok := m.buttonAt(e.X, e.Y); {
	case !ok:
		return m, nil
	case btn:
		m.value = true
		m.pane.SetContent(m.renderInner())
		return m, confirmed()
	default:
		m.value = false
		m.pane.SetContent(m.renderInner())
		return m, cancelled()
	}
}

// buttonAt maps a position to a button: true for confirm, false for cancel.
// The buttons occupy the last inner line, laid out as cancel, two spaces,
// confirm — the same order renderInner writes them in.
func (m Model) buttonAt(x, y int) (confirm bool, ok bool) {
	line, inBody := m.pane.RowAt(x, y)
	if !inBody {
		return false, false
	}
	content := m.renderInner()
	if line != strings.Count(content, "\n") {
		return false, false
	}

	c := m.pane.ContentRect()
	pos := (x - c.X) + m.pane.XOffset()

	cancelW := ansi.StringWidth("[ " + m.cancelLabel + " ]")
	confirmW := ansi.StringWidth("[ " + m.confirmLabel + " ]")
	confirmStart := cancelW + buttonGap

	switch {
	case pos >= 0 && pos < cancelW:
		return false, true
	case pos >= confirmStart && pos < confirmStart+confirmW:
		return true, true
	}
	return false, false
}

// buttonGap is the spacing renderInner puts between the two buttons.
const buttonGap = 2

// confirmed and cancelled return commands carrying the result messages.
func confirmed() tea.Cmd { return func() tea.Msg { return ConfirmedMsg{} } }
func cancelled() tea.Cmd { return func() tea.Msg { return CancelledMsg{} } }
