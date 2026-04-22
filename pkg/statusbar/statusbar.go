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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	var middle string
	switch m.middleKind {
	case MessageInfo:
		middle = m.infoStyle.Width(middleW).Render(m.middle)
	case MessageError:
		middle = m.errorStyle.Width(middleW).Render(m.middle)
	default:
		middle = m.neutralStyle.Width(middleW).Render("")
	}

	row := left + middle + right
	return lipgloss.NewStyle().Inline(true).MaxWidth(m.width).Width(m.width).Render(row)
}

func (m *Model) SetWidth(w int)    { m.width = w }
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
