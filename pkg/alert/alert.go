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
// identical to pkg/confirm. Fixed-size shape:
//
//	if s.alertUp {
//	    return layout.ZStack(
//	        baseLayout,
//	        layout.Center(48, 7, layout.Sized(&s.alert)),
//	    )
//	}
//	return baseLayout
//
// Autosize shape (set Options.Autosize = true) — the modal measures its
// message and centers itself inside whatever bounds it gets, so the caller
// drops the outer Center wrapper:
//
//	if s.alertUp {
//	    return layout.ZStack(baseLayout, layout.Sized(&s.alert))
//	}
//	return baseLayout
//
// Reach for autosize whenever the message body is dynamic (subprocess
// stderr, wrapped API errors) — the fixed size clips those. The modal
// caps at 80% terminal width (40-col floor) × 60% terminal height,
// word-wraps against the effective width, and scrolls the message region
// internally when content overflows; the OK button stays pinned to the
// last inner row throughout.
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
//
// Extra keys in autosize mode when content overflows the viewport
// (rule 23 — arrows + hjkl are reserved for scroll library-wide):
//
//	↑ / k                          scroll up one line
//	↓ / j                          scroll down one line
//	g / G                          jump to top / bottom
//	ctrl+u / ctrl+d                half-page up / down
//	pgup / pgdown                  page up / down
package alert

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Autosize sizing caps. Width is capped at 80% of the outer bounds (with
// a 40-column floor so the modal doesn't collapse into a sliver on narrow
// terminals); height is capped at 60%. Message content beyond the height
// cap scrolls internally with the OK button pinned to the last inner row.
const (
	autosizeMinWidth  = 40
	autosizeWidthNum  = 4
	autosizeWidthDen  = 5
	autosizeHeightNum = 3
	autosizeHeightDen = 5
	autosizeMinHeight = 5
)

// DismissedMsg is emitted when the user dismisses the alert.
type DismissedMsg struct{}

// Options configures a new alert dialog. Zero-value fields fall back to
// defaults; start from theme.Alert() to fill in the color tokens. For an
// error-styled alert, override ActiveColor with the theme's ErrorBG after
// calling theme.Alert().
type Options struct {
	// Width and Height set the pane's outer dimensions. SetRect
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
	// Glyphs are the marks this component draws, plus the scrollbar
	// thumb and track it hands to its pane. Empty fields fall back to
	// glyph.Default.
	Glyphs       glyph.Set
	SlotBrackets pane.SlotBracketStyle

	// Autosize enables message-content sizing. When true, SetRect
	// treats the rect as the outer bounds (typically the terminal size).
	// The modal word-wraps its message, sizes itself to fit — capped at
	// 80% width (min 40 cols) and 60% height — and centers within those
	// bounds. Content past the height cap scrolls internally with
	// arrows / j/k / g/G / ctrl+u/d / PgUp/PgDn; the OK button stays
	// pinned to the last inner row regardless of scroll position.
	//
	// When false (the zero value, preserving the fixed-size shape),
	// SetRect sets the pane's outer size directly and the caller
	// handles centering with layout.Center(w, h, layout.Sized(&m)).
	Autosize bool
}

// Model is the alert dialog's exported state.
type Model struct {
	okLabel      string
	message      string
	okStyle      lipgloss.Style
	messageStyle lipgloss.Style

	pane pane.Pane

	// autosize-mode state.
	autosize     bool
	rect         geom.Rect
	outerW       int
	outerH       int
	modalW       int
	modalH       int
	wrapped      []string
	viewportRows int
	scrollOffset int
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
		Glyphs:         opts.Glyphs,
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
		autosize:     opts.Autosize,
	}
	m.pane.SetContent(m.renderInner())
	return m
}

// Init is a no-op.
func (m Model) Init() tea.Cmd { return nil }

// Update dismisses the alert on enter/space/esc/o/O. In autosize mode with
// overflowing content, arrows / j/k / g/G / ctrl+u/d / PgUp/PgDn scroll
// the message region while the OK button stays pinned; when content fits,
// those keys are inert (they never carry a competing verb — rule 23). All
// dismiss keys remain active in either mode.
//
// Returns a tea.Cmd carrying DismissedMsg on dismissal; the parent screen
// matches the result in its own Update to dismiss the modal and act. All
// keys are active — the modal is the only thing focused while it's up.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if mm, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(mm)
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.autosize && m.canScroll() {
		switch k.String() {
		case "up", "k":
			m.scrollOffset--
			m.clampScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "down", "j":
			m.scrollOffset++
			m.clampScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "g":
			m.scrollOffset = 0
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "G":
			m.scrollOffset = m.maxScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "ctrl+u":
			m.scrollOffset -= halfPage(m.viewportRows)
			m.clampScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "ctrl+d":
			m.scrollOffset += halfPage(m.viewportRows)
			m.clampScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "pgup":
			m.scrollOffset -= m.viewportRows
			m.clampScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		case "pgdown":
			m.scrollOffset += m.viewportRows
			m.clampScroll()
			m.pane.SetContent(m.renderInner())
			return m, nil
		}
	}
	switch k.String() {
	case "enter", " ", "esc", "o", "O":
		return m, dismissed()
	}
	return m, nil
}

// View renders the dialog as a bordered pane. In autosize mode the pane is
// centered within the last-set outer bounds via lipgloss.Place so the
// caller only needs layout.Sized(&m) inside its ZStack overlay.
func (m Model) View() string {
	if !m.autosize || m.outerW == 0 || m.outerH == 0 {
		return m.pane.View()
	}
	return lipgloss.Place(m.outerW, m.outerH,
		lipgloss.Center, lipgloss.Center,
		m.pane.View())
}

// SetRect places the surrounding pane. In autosize mode the rect is
// treated as outer bounds — the pane sizes itself to fit the
// wrapped message content, capped at 80% width / 60% height — and the
// wrapped-message viewport recomputes so the modal remains responsive to
// terminal resizes.
// The rect is retained so an autosize modal, which draws itself centered
// inside the bounds it was given, can hit-test against where it actually
// landed rather than against the outer bounds.
func (m *Model) SetRect(r geom.Rect) {
	m.rect = r
	if m.autosize {
		m.outerW, m.outerH = r.W, r.H
		m.recomputeAutosize()
		m.pane.SetRect(geom.CenterIn(r, m.modalW, m.modalH))
		m.pane.SetContent(m.renderInner())
		return
	}
	m.pane.SetRect(r)
	m.pane.SetContent(m.renderInner())
}

// SetTitle sets the title shown on the pane's top border.
func (m *Model) SetTitle(s string) { m.pane.SetTitle(s) }

// SetMessage updates the body text. Useful when the modal is reused across
// different errors without rebuilding the model. In autosize mode the
// modal re-measures and re-centers, and the scroll offset resets to the
// top so a fresh error surfaces at its first line.
func (m *Model) SetMessage(s string) {
	m.message = s
	if m.autosize {
		m.scrollOffset = 0
		m.recomputeAutosize()
		m.pane.SetRect(geom.CenterIn(m.rect, m.modalW, m.modalH))
	}
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

// SetActiveBorder updates the border shape drawn while focused.
func (m *Model) SetActiveBorder(b lipgloss.Border) { m.pane.SetActiveBorder(b) }

// SetInactiveBorder updates the border shape drawn while unfocused.
func (m *Model) SetInactiveBorder(b lipgloss.Border) { m.pane.SetInactiveBorder(b) }

// Help returns the keys this dialog responds to. Compose into the parent's
// Help() while the modal is up so the hint strip reflects the modal context.
// In autosize mode with overflowing content, the returned set includes the
// scroll bindings.
func (m Model) Help() []key.Binding {
	out := []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "dismiss")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss")),
	}
	if m.autosize && m.canScroll() {
		out = append(out,
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "scroll")),
			key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("PgUp/PgDn", "page")),
		)
	}
	return out
}

// renderInner produces the content rendered inside the pane. In the
// fixed-size shape it's message body + blank + button, joined with "\n".
// In autosize mode the message region is a windowed slice of the wrapped
// lines starting at scrollOffset, always padded to viewportRows so the
// blank spacer + OK button pin to the last two inner rows regardless of
// scroll position.
// handleMouse resolves a click on the OK button and, in autosize mode,
// wheel scrolling over the message region.
//
// Clicking OK dismisses outright — the button is an acknowledgement, so
// there is nothing to arm. A click elsewhere inside the modal is swallowed
// rather than passed through: the alert owns input while it is up. Clicks
// outside do nothing, leaving esc and the button as the ways out.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if !m.pane.Rect().Hit(e.X, e.Y) {
		return m, nil
	}
	if m.autosize && m.canScroll() && (e.IsWheelUp() || e.IsWheelDown()) {
		if e.IsWheelUp() {
			m.scrollOffset -= pane.WheelStep
		} else {
			m.scrollOffset += pane.WheelStep
		}
		m.clampScroll()
		m.pane.SetContent(m.renderInner())
		return m, nil
	}
	if e.IsPress() && m.onOK(e.X, e.Y) {
		return m, dismissed()
	}
	return m, nil
}

// onOK reports whether a position falls on the OK button. The button always
// occupies the last inner line, left-aligned — in autosize mode it stays
// pinned there regardless of how far the message has scrolled.
func (m Model) onOK(x, y int) bool {
	line, inBody := m.pane.RowAt(x, y)
	if !inBody {
		return false
	}
	if line != strings.Count(m.renderInner(), "\n") {
		return false
	}
	c := m.pane.ContentRect()
	pos := (x - c.X) + m.pane.XOffset()
	return pos >= 0 && pos < ansi.StringWidth("[ "+m.okLabel+" ]")
}

func (m Model) renderInner() string {
	ok := m.okStyle.Render("[ " + m.okLabel + " ]")

	if !m.autosize {
		body := m.messageStyle.Render(m.message)
		parts := []string{body}
		if body != "" {
			parts = append(parts, "")
		}
		parts = append(parts, ok)
		return strings.Join(parts, "\n")
	}

	if m.message == "" || m.viewportRows <= 0 {
		return ok
	}
	lo := m.scrollOffset
	hi := lo + m.viewportRows
	if hi > len(m.wrapped) {
		hi = len(m.wrapped)
	}
	if lo > hi {
		lo = hi
	}
	window := append([]string{}, m.wrapped[lo:hi]...)
	for len(window) < m.viewportRows {
		window = append(window, "")
	}
	body := m.messageStyle.Render(strings.Join(window, "\n"))
	return body + "\n\n" + ok
}

// recomputeAutosize measures the message against the current outer bounds
// and sets modalW / modalH / wrapped / viewportRows. Called on
// SetRect and SetMessage in autosize mode.
func (m *Model) recomputeAutosize() {
	if m.outerW < 4 || m.outerH < 3 {
		m.modalW, m.modalH = m.outerW, m.outerH
		m.wrapped = splitLines(m.message)
		m.viewportRows = 0
		return
	}
	maxW := m.outerW * autosizeWidthNum / autosizeWidthDen
	if maxW < autosizeMinWidth {
		maxW = autosizeMinWidth
	}
	if maxW > m.outerW {
		maxW = m.outerW
	}
	maxH := m.outerH * autosizeHeightNum / autosizeHeightDen
	if maxH < autosizeMinHeight {
		maxH = autosizeMinHeight
	}
	if maxH > m.outerH {
		maxH = m.outerH
	}

	// pane reserves 2 border cols + 1 scrollbar col; inner content width
	// is outer - 3 (see pkg/pane SetRect).
	const paneChromeW = 3
	innerW := maxW - paneChromeW
	if innerW < 1 {
		innerW = 1
	}
	m.wrapped = wrapLines(m.message, innerW)

	btnW := ansi.StringWidth("[ " + m.okLabel + " ]")
	longest := btnW
	for _, ln := range m.wrapped {
		if w := ansi.StringWidth(ln); w > longest {
			longest = w
		}
	}
	m.modalW = longest + paneChromeW
	if m.modalW > maxW {
		m.modalW = maxW
	}
	if m.modalW < paneChromeW+1 {
		m.modalW = paneChromeW + 1
	}

	n := len(m.wrapped)
	if m.message == "" {
		n = 0
	}
	contentRows := 1
	if n > 0 {
		contentRows = n + 1 + 1
	}
	m.modalH = contentRows + 2
	if m.modalH > maxH {
		m.modalH = maxH
	}
	if m.modalH < 3 {
		m.modalH = 3
	}

	innerH := m.modalH - 2
	if n == 0 {
		m.viewportRows = 0
	} else {
		m.viewportRows = innerH - 2
		if m.viewportRows < 0 {
			m.viewportRows = 0
		}
	}
	m.clampScroll()
}

// canScroll reports whether the wrapped message overflows the viewport.
func (m Model) canScroll() bool {
	return m.autosize && m.viewportRows > 0 && len(m.wrapped) > m.viewportRows
}

// maxScroll is the largest legal scrollOffset given the current viewport.
func (m Model) maxScroll() int {
	if !m.canScroll() {
		return 0
	}
	return len(m.wrapped) - m.viewportRows
}

// clampScroll bounds scrollOffset to [0, maxScroll].
func (m *Model) clampScroll() {
	max := m.maxScroll()
	if m.scrollOffset > max {
		m.scrollOffset = max
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// wrapLines word-wraps s at width cells, preserving explicit newlines in
// the input and yielding a per-visual-line slice. ANSI codes in s pass
// through via ansi.Wrap. Empty s returns nil.
func wrapLines(s string, width int) []string {
	if s == "" {
		return nil
	}
	wrapped := ansi.Wrap(s, width, " -")
	return strings.Split(wrapped, "\n")
}

// splitLines is the un-wrapped fallback path used when outer bounds are
// too small to autosize meaningfully.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// halfPage returns floor(rows/2) with a minimum step of 1 so half-page
// scrolls always move at least one row on a tiny viewport.
func halfPage(rows int) int {
	step := rows / 2
	if step < 1 {
		step = 1
	}
	return step
}

func dismissed() tea.Cmd { return func() tea.Msg { return DismissedMsg{} } }
