// Package statusbar renders a one-line status strip with left/center/right
// slots, modeled on pug's footer.
//
// The left and right slots are static (e.g. a "? help" hint, a version
// string). The center is a message region with three states — neutral (empty),
// info, and error — each with its own style preset. Call SetInfo / SetError
// to show a message; call ClearMessage (or forward tea messages through
// Update, which auto-clears on any KeyMsg) to wipe it.
package statusbar

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// MessageKind is the state of the center slot.
type MessageKind int

const (
	MessageNone MessageKind = iota
	MessageInfo
	MessageError
)

// Options configures a status bar. Zero-value fields fall back to defaults.
// Style fields are pointers so nil can mean "use default."
//
// Use BarBackground / BarForeground to set a single color pair for the whole
// strip; the left, right, and neutral-middle styles all derive from them so
// the bar reads as one continuous band. Info and Error styles intentionally
// pop with different colors.
type Options struct {
	Width       int
	Left, Right string

	// BarBackground is the background color applied to left, right, and the
	// neutral middle slot. Defaults to "236" (dark grey).
	BarBackground lipgloss.TerminalColor
	// BarForeground is the foreground color for left, right, and neutral
	// middle. Defaults to "252".
	BarForeground lipgloss.TerminalColor

	LeftStyle, RightStyle               *lipgloss.Style
	NeutralStyle, InfoStyle, ErrorStyle *lipgloss.Style
}

// Model is a one-line status bar.
type Model struct {
	rect geom.Rect

	width       int
	left, right string

	middle     string
	middleKind MessageKind

	barBG, barFG                        lipgloss.TerminalColor
	leftStyle, rightStyle               lipgloss.Style
	neutralStyle, infoStyle, errorStyle lipgloss.Style
}

// New constructs a status bar with the given options, applying defaults for
// any unset field so you get a usable bar out of the box.
func New(opts Options) Model {
	if opts.BarBackground == nil {
		opts.BarBackground = lipgloss.Color("236")
	}
	if opts.BarForeground == nil {
		opts.BarForeground = lipgloss.Color("252")
	}
	defPad := lipgloss.NewStyle().Padding(0, 1)
	barBase := defPad.Background(opts.BarBackground).Foreground(opts.BarForeground)
	pick := func(s *lipgloss.Style, def lipgloss.Style) lipgloss.Style {
		if s != nil {
			return *s
		}
		return def
	}
	return Model{
		width:        opts.Width,
		left:         opts.Left,
		right:        opts.Right,
		barBG:        opts.BarBackground,
		barFG:        opts.BarForeground,
		leftStyle:    pick(opts.LeftStyle, barBase),
		rightStyle:   pick(opts.RightStyle, barBase),
		neutralStyle: pick(opts.NeutralStyle, barBase),
		infoStyle:    pick(opts.InfoStyle, defPad.Background(lipgloss.Color("35")).Foreground(lipgloss.Color("0"))),
		errorStyle:   pick(opts.ErrorStyle, defPad.Background(lipgloss.Color("160")).Foreground(lipgloss.Color("15"))),
	}
}

// BarBackground returns the status bar's base background color. Useful when
// styling content you intend to embed in the bar (e.g. help key/desc styles)
// so the backgrounds line up.
func (m Model) BarBackground() lipgloss.TerminalColor { return m.barBG }

// BarForeground returns the status bar's base foreground color.
func (m Model) BarForeground() lipgloss.TerminalColor { return m.barFG }

func (m Model) Init() tea.Cmd { return nil }

// Update auto-clears any info/error message on the next KeyMsg, matching pug's
// behavior. If you want persistent messages, skip Update and use ClearMessage
// explicitly.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.middleKind = MessageNone
		m.middle = ""
	}
	return m, nil
}

// View renders the status bar as a single line, clipped to Width.
func (m Model) View() string {
	left := m.leftStyle.Render(m.left)
	right := m.rightStyle.Render(m.right)
	middleW := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))

	row := left + m.renderMiddle(middleW) + right
	return lipgloss.NewStyle().Inline(true).MaxWidth(m.width).Width(m.width).Render(row)
}

// renderMiddle fills the center slot in exactly w cells.
//
// The "exactly" is the point, and lipgloss will not do it on its own. Width
// is a minimum, so a message longer than the slot overflows and shoves the
// right slot off the end of the bar; and a style with Padding(0,1) renders
// two cells even for empty content, so a slot sized to zero still costs two.
// Either way the outer MaxWidth then clips the right-hand end — which used
// to cost a couple of characters of the version string and now costs the
// output badge, the one thing on the bar saying the full text is still
// readable.
func (m Model) renderMiddle(w int) string {
	if w <= 0 {
		return ""
	}

	style, text := m.neutralStyle, ""
	switch m.middleKind {
	case MessageInfo:
		style, text = m.infoStyle, m.middle
	case MessageError:
		style, text = m.errorStyle, m.middle
	}

	// Too narrow for the style's own padding: fill with bar background
	// rather than let the padding alone overrun the slot.
	if w <= style.GetHorizontalPadding() {
		return lipgloss.NewStyle().Background(m.barBG).Render(strings.Repeat(" ", w))
	}
	return style.Width(w).Render(fit(text, w, style))
}

// fit cuts s to the space the middle slot actually has.
//
// lipgloss's Width is a minimum, not a maximum, so a message longer than the
// slot pushes the right slot off the end of the bar and the outer MaxWidth
// then clips it away entirely. That used to cost only the version string.
// It now costs the output-log badge as well — which is the one piece of the
// bar you most need when a long error message is on screen.
func fit(s string, slotW int, style lipgloss.Style) string {
	avail := slotW - style.GetHorizontalPadding()
	if avail <= 0 {
		return ""
	}
	if xansi.StringWidth(s) <= avail {
		return s
	}
	return xansi.Cut(s, 0, avail)
}

func (m *Model) SetRect(r geom.Rect) { m.rect = r; m.width = r.W }
func (m Model) Rect() geom.Rect      { return m.rect }

// LeftContentRect returns the rect covering the left slot's text, excluding
// the style's own padding. The app shell uses it to locate the trailing help
// affordance inside the rendered hint line — the statusbar is handed that
// line pre-rendered, so it knows where the slot sits but not what is in it.
func (m Model) LeftContentRect() geom.Rect {
	return geom.Rect{
		X:   m.rect.X + m.leftStyle.GetPaddingLeft(),
		Y:   m.rect.Y,
		W:   lipgloss.Width(m.left),
		H:   1,
		Gen: m.rect.Gen,
	}
}

// RightContentRect returns the rect covering the right slot's text, excluding
// the style's own padding — the mirror of LeftContentRect.
//
// The right slot is flush to the bar's right edge (View sizes the middle to
// fill whatever the two ends leave), so the slot's position is derived from
// its own rendered width rather than tracked. The app shell uses this to
// hit-test the output-log affordance it composes into the slot ahead of the
// version string.
func (m Model) RightContentRect() geom.Rect {
	w := lipgloss.Width(m.right)
	slot := m.rightStyle.GetPaddingLeft() + w + m.rightStyle.GetPaddingRight()
	return geom.Rect{
		X:   m.rect.X + m.rect.W - slot + m.rightStyle.GetPaddingLeft(),
		Y:   m.rect.Y,
		W:   w,
		H:   1,
		Gen: m.rect.Gen,
	}
}

func (m *Model) SetLeft(s string)  { m.left = s }
func (m *Model) SetRight(s string) { m.right = s }

// SetInfo shows s in the center slot with the info style.
func (m *Model) SetInfo(s string) {
	m.middle = s
	m.middleKind = MessageInfo
}

// SetError shows s in the center slot with the error style.
func (m *Model) SetError(s string) {
	m.middle = s
	m.middleKind = MessageError
}

// ClearMessage resets the center slot to its neutral state.
func (m *Model) ClearMessage() {
	m.middle = ""
	m.middleKind = MessageNone
}

// MessageKind reports the current state of the center slot.
func (m Model) MessageKind() MessageKind { return m.middleKind }

// Message returns the current center-slot text and its kind. Useful when
// rebuilding the bar (e.g. on theme swap) and you want to carry an
// in-flight info/error message across the rebuild.
func (m Model) Message() (string, MessageKind) { return m.middle, m.middleKind }
