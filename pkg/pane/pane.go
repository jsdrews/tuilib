// Package pane provides a bordered, titled, scrollable region for Bubble Tea
// TUIs. A Pane owns a viewport and renders a vertical scrollbar along its
// right edge. Any string content can be placed inside — render a child model
// to a string via its View() method and pass it to SetContent.
package pane

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// HScrollStep is how many cells left/right scroll the pane horizontally.
const HScrollStep = 4

// Pane is a bordered region with a title, six metadata slots around the
// border, and a vertical scrollbar down the right edge.
type Pane struct {
	viewport viewport.Model

	width, height int

	rawLines []string
	maxLineW int
	xOffset  int

	title       string
	topLeft     string
	topRight    string
	bottomLeft  string
	bottomMid   string
	bottomRight string // empty => auto-filled with scroll percent

	focused     bool
	hScrollbar  bool
	titlePos    BorderPosition

	activeColor    lipgloss.TerminalColor
	inactiveColor  lipgloss.TerminalColor
	activeBorder   lipgloss.Border
	inactiveBorder lipgloss.Border
	slotBrackets   SlotBracketStyle
}

// Options configures a new Pane. Zero-value fields fall back to defaults.
type Options struct {
	Width, Height int
	Title         string
	// TitlePosition picks which border slot the title occupies. Defaults to
	// TopLeftBorder (the zero value).
	TitlePosition BorderPosition
	Focused       bool
	ActiveColor   lipgloss.TerminalColor
	InactiveColor lipgloss.TerminalColor
	// ActiveBorder is drawn when the Pane is focused. Defaults to
	// lipgloss.ThickBorder().
	ActiveBorder lipgloss.Border
	// InactiveBorder is drawn when the Pane is not focused. Defaults to
	// lipgloss.NormalBorder().
	InactiveBorder lipgloss.Border
	// SlotBrackets controls how the title and other border slot text are
	// bracketed against the border line. Defaults to SlotBracketsNone
	// (text sits inline on the border with no surrounding glyphs).
	SlotBrackets SlotBracketStyle
	// HScrollbar reserves a single row at the bottom of the inner content
	// area for a horizontal scrollbar. The thumb tracks xOffset against
	// the longest line; when content fits, the track renders blank.
	HScrollbar bool
}

// New constructs a Pane. SetContent must be called separately to populate it.
func New(opts Options) Pane {
	if opts.ActiveColor == nil {
		opts.ActiveColor = lipgloss.Color("12")
	}
	if opts.InactiveColor == nil {
		opts.InactiveColor = lipgloss.Color("240")
	}
	if (opts.ActiveBorder == lipgloss.Border{}) {
		opts.ActiveBorder = lipgloss.ThickBorder()
	}
	if (opts.InactiveBorder == lipgloss.Border{}) {
		opts.InactiveBorder = lipgloss.NormalBorder()
	}
	p := Pane{
		viewport:       viewport.New(0, 0),
		title:          opts.Title,
		titlePos:       opts.TitlePosition,
		focused:        opts.Focused,
		hScrollbar:     opts.HScrollbar,
		activeColor:    opts.ActiveColor,
		inactiveColor:  opts.InactiveColor,
		activeBorder:   opts.ActiveBorder,
		inactiveBorder: opts.InactiveBorder,
		slotBrackets:   opts.SlotBrackets,
	}
	p.SetDimensions(opts.Width, opts.Height)
	return p
}

func (p Pane) Init() tea.Cmd { return nil }

// Update forwards key/mouse events to the embedded viewport so vertical
// scroll keys (pgup/pgdn/up/down/mouse wheel) work by default. Left and
// right arrow keys are intercepted for horizontal scrolling — the
// content is re-cut to the visible window via ansi.Cut so ANSI styles
// stay intact across the slice.
func (p Pane) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "left", "h":
			p.xOffset -= HScrollStep
			p.pushContent()
			return p, nil
		case "right", "l":
			p.xOffset += HScrollStep
			p.pushContent()
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

// View renders the pane: content inside viewport, scrollbar on the right,
// both wrapped in a titled border with metadata slots.
func (p Pane) View() string {
	bar := Scrollbar(
		p.viewport.Height,
		p.viewport.TotalLineCount(),
		p.viewport.VisibleLineCount(),
		p.viewport.YOffset,
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, p.viewport.View(), bar)
	if p.hScrollbar {
		inner := p.viewport.Width
		hbar := HScrollbar(inner, p.maxLineW, inner, p.xOffset)
		body = lipgloss.JoinVertical(lipgloss.Left, body, hbar+strings.Repeat(" ", ScrollbarWidth))
	}

	// Auto-fill bottom-right with scroll percent only when content actually
	// overflows. Panes used as input strips (filter bars, one-liners) would
	// otherwise show a meaningless "100%".
	br := p.bottomRight
	if br == "" && p.viewport.TotalLineCount() > p.viewport.VisibleLineCount() {
		br = fmt.Sprintf("%d%%", int(p.viewport.ScrollPercent()*100))
	}

	slots := map[BorderPosition]string{
		TopLeftBorder:      pad(p.topLeft),
		TopMiddleBorder:    pad(""),
		TopRightBorder:     pad(p.topRight),
		BottomLeftBorder:   pad(p.bottomLeft),
		BottomMiddleBorder: pad(p.bottomMid),
		BottomRightBorder:  pad(br),
	}
	// Title overrides whichever slot it's assigned to.
	if p.title != "" {
		slots[p.titlePos] = pad(p.title)
	}

	border := p.inactiveBorder
	color := p.inactiveColor
	if p.focused {
		border = p.activeBorder
		color = p.activeColor
	}
	return Borderize(body, border, color, slots, p.slotBrackets)
}

// SetSlotBrackets controls how the title and other slot text meet the border.
func (p *Pane) SetSlotBrackets(s SlotBracketStyle) { p.slotBrackets = s }

// SetContent replaces the pane's content. Pass any string — a child model's
// View() output, a pre-rendered table, a log, raw text — and the pane will
// scroll it. Long lines are truncated to the inner width so terminal wrap
// can't break row counting; use left/right (or SetXOffset) to scroll
// horizontally past the cut.
func (p *Pane) SetContent(s string) {
	if s == "" {
		p.rawLines = nil
	} else {
		p.rawLines = strings.Split(s, "\n")
	}
	p.maxLineW = 0
	for _, l := range p.rawLines {
		if w := lipgloss.Width(l); w > p.maxLineW {
			p.maxLineW = w
		}
	}
	p.pushContent()
}

// SetDimensions sets the Pane's outer size (including border). The inner
// content area is sized as (width-2-scrollbar) × (height-2), shrunk by
// one more row when HScrollbar is enabled.
func (p *Pane) SetDimensions(width, height int) {
	p.width, p.height = width, height
	p.viewport.Width = max(0, width-2-ScrollbarWidth)
	innerH := height - 2
	if p.hScrollbar {
		innerH -= ScrollbarHeight
	}
	p.viewport.Height = max(0, innerH)
	p.pushContent()
}

// XOffset returns the current horizontal scroll column.
func (p Pane) XOffset() int { return p.xOffset }

// MaxXOffset returns the largest meaningful horizontal scroll column —
// 0 when content fits in the inner width.
func (p Pane) MaxXOffset() int {
	return max(0, p.maxLineW-p.viewport.Width)
}

// SetXOffset jumps to the given horizontal scroll column, clamped into
// [0, MaxXOffset()].
func (p *Pane) SetXOffset(n int) {
	p.xOffset = n
	p.pushContent()
}

// pushContent re-cuts every raw line to the visible window and pushes
// the result to the viewport. Skips the cut entirely when xOffset is 0
// and every line fits — a fast path for the common case.
func (p *Pane) pushContent() {
	maxOff := p.MaxXOffset()
	if p.xOffset > maxOff {
		p.xOffset = maxOff
	}
	if p.xOffset < 0 {
		p.xOffset = 0
	}
	if len(p.rawLines) == 0 {
		p.viewport.SetContent("")
		return
	}
	inner := p.viewport.Width
	if inner <= 0 || (p.xOffset == 0 && p.maxLineW <= inner) {
		p.viewport.SetContent(strings.Join(p.rawLines, "\n"))
		return
	}
	var b strings.Builder
	for i, line := range p.rawLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ansi.Cut(line, p.xOffset, p.xOffset+inner))
	}
	p.viewport.SetContent(b.String())
}

// Width returns the Pane's outer width.
func (p Pane) Width() int { return p.width }

// Height returns the Pane's outer height.
func (p Pane) Height() int { return p.height }

// Focused reports whether the pane is drawn in its active style.
func (p Pane) Focused() bool { return p.focused }

func (p *Pane) SetFocused(b bool)               { p.focused = b }
func (p *Pane) SetTitle(s string)               { p.title = s }
func (p *Pane) SetTitlePosition(pos BorderPosition) { p.titlePos = pos }

// SetActiveColor updates the border color used when the pane is focused.
// Useful when reacting to a theme swap without rebuilding the model.
func (p *Pane) SetActiveColor(c lipgloss.TerminalColor) { p.activeColor = c }

// SetInactiveColor updates the border color used when the pane is unfocused.
func (p *Pane) SetInactiveColor(c lipgloss.TerminalColor) { p.inactiveColor = c }
func (p *Pane) SetTopLeft(s string)      { p.topLeft = s }
func (p *Pane) SetTopRight(s string)     { p.topRight = s }
func (p *Pane) SetBottomLeft(s string)   { p.bottomLeft = s }
func (p *Pane) SetBottomMiddle(s string) { p.bottomMid = s }

// SetBottomRight overrides the auto-generated scroll percentage. Pass "" to
// restore the default.
func (p *Pane) SetBottomRight(s string) { p.bottomRight = s }

// GotoTop scrolls the viewport to the first line.
func (p *Pane) GotoTop() { p.viewport.GotoTop() }

// GotoBottom scrolls the viewport to the last line.
func (p *Pane) GotoBottom() { p.viewport.GotoBottom() }

// AtBottom reports whether the viewport is scrolled to the last line —
// useful for streaming-content components that auto-follow new output
// only while the user is parked at the bottom.
func (p Pane) AtBottom() bool { return p.viewport.AtBottom() }

// YOffset returns the current vertical scroll offset (top visible line).
func (p Pane) YOffset() int { return p.viewport.YOffset }

// SetYOffset jumps to the given vertical scroll offset.
func (p *Pane) SetYOffset(n int) { p.viewport.SetYOffset(n) }

// EnsureVisible scrolls the viewport the minimum amount needed to put line
// `n` inside the visible window. Useful for cursor-driven list views, where
// moving the cursor past the viewport's bottom should pull the view with it.
func (p *Pane) EnsureVisible(n int) {
	top := p.viewport.YOffset
	bottom := top + p.viewport.Height - 1
	switch {
	case n < top:
		p.viewport.SetYOffset(n)
	case n > bottom:
		p.viewport.SetYOffset(n - p.viewport.Height + 1)
	}
}

func pad(s string) string {
	if s == "" {
		return ""
	}
	return " " + s + " "
}
