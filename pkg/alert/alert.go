// Package alert provides a modal acknowledgement dialog component. It
// renders a bordered titled pane with a message body and a single "OK"
// button, and resolves to a DismissedMsg via tea.Cmd when the user
// commits.
//
// Use it for "stop and read this" notifications — surfaced errors,
// destructive-action results, anything the user must acknowledge before
// continuing. For passive feedback (success notices, transient warnings)
// prefer the lighter app.Info / app.Error statusbar messages.
//
// The component is meant to be hosted in the standard modal pattern —
// identical to pkg/confirm:
//
//	if s.alertUp {
//	    return layout.ZStack(
//	        baseLayout,
//	        layout.Center(48, 7, layout.Sized(&s.alert)),
//	    )
//	}
//	return baseLayout
//
// While the modal is up, the parent screen returns true from
// IsCapturingKeys() so the app shell suppresses its global keys (q, t,
// esc-pop) and the modal owns input. The parent forwards every tea.Msg to
// the alert and matches alert.DismissedMsg in its own Update to dismiss
// the modal and act.
//
// Keys (always active when the modal is rendered):
//
//	enter / space / esc / o / O   dismiss
package alert

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/pane"
)

// DismissedMsg is emitted when the user dismisses the alert.
type DismissedMsg struct{}

// Options configures a new alert dialog. Zero-value fields fall back to
// defaults; start from theme.Alert() to fill in the color tokens. For an
// error-styled alert, override ActiveColor with the theme's ErrorBG after
// calling theme.Alert().
type Options struct {
	// Width and Height set the pane's outer dimensions. SetDimensions
	// overrides these later if the host re-sizes the modal.
	Width, Height int

	// Title sits on the top border (e.g. "Error", "Notice"). Defaults to
	// "alert".
	Title string

	// Message is the body text shown above the button. Multi-line strings are
	// supported; the component does not wrap.
	Message string

	// OK is the rendered button label. Defaults to "OK".
	OK string

	// OKStyle styles the dismissal button. Brackets are part of the render.
	OKStyle lipgloss.Style

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

// Model is the alert dialog's exported state.
type Model struct {
	okLabel      string
	message      string
	okStyle      lipgloss.Style
	messageStyle lipgloss.Style

	pane pane.Pane
}

// New constructs an alert dialog. Always renders in the active border
// state — the modal owns input whenever it's visible.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "alert"
	}
	if opts.OK == "" {
		opts.OK = "OK"
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
		okLabel:      opts.OK,
		message:      opts.Message,
		okStyle:      opts.OKStyle,
		messageStyle: opts.MessageStyle,
		pane:         p,
	}
	m.pane.SetContent(m.renderInner())
	return m
}

// Init is a no-op.
func (m Model) Init() tea.Cmd { return nil }

// Update dismisses the alert on enter/space/esc/o/O. Returns a tea.Cmd
// carrying DismissedMsg; the parent screen matches the result in its own
// Update to dismiss the modal and act.
//
// All keys are active — the modal is the only thing focused while it's up.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "enter", " ", "esc", "o", "O":
		return m, dismissed()
	}
	return m, nil
}

// View renders the dialog as a bordered pane.
func (m Model) View() string { return m.pane.View() }

// SetDimensions resizes the surrounding pane.
func (m *Model) SetDimensions(w, h int) {
	m.pane.SetDimensions(w, h)
	m.pane.SetContent(m.renderInner())
}

// SetTitle sets the title shown on the pane's top border.
func (m *Model) SetTitle(s string) { m.pane.SetTitle(s) }

// SetMessage updates the body text. Useful when the modal is reused across
// different errors without rebuilding the model.
func (m *Model) SetMessage(s string) {
	m.message = s
	m.pane.SetContent(m.renderInner())
}

// SetOKStyle updates the button style. Useful when reacting to a theme
// swap without rebuilding the model.
func (m *Model) SetOKStyle(s lipgloss.Style) {
	m.okStyle = s
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
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "dismiss")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss")),
	}
}

// renderInner produces the content rendered inside the pane: message body
// on top, an empty line, then the OK button centered on the last row.
func (m Model) renderInner() string {
	ok := m.okStyle.Render("[ " + m.okLabel + " ]")
	body := m.messageStyle.Render(m.message)

	parts := []string{body}
	if body != "" {
		parts = append(parts, "")
	}
	parts = append(parts, ok)
	return strings.Join(parts, "\n")
}

func dismissed() tea.Cmd { return func() tea.Msg { return DismissedMsg{} } }
