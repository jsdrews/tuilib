// Package banner draws a title in large block letters with a drop shadow, its
// faces swept by a colour gradient — the heading of an intro or sign-in
// screen.
//
//	opts := t.Banner()
//	opts.Text = "deploy"
//	b := banner.New(opts)
//
// The letters are six rows tall and drawn from A–Z, 0–9, space and - . ! ? :.
// Lower case is drawn as upper case, and anything else as a space, so a
// banner never refuses its text.
//
// The gradient moves on a timer, so a banner is a component like a spinner:
// batch Init into the screen's Init and forward every message to Update. With
// Options.Static, or fewer than two colours, it draws once and asks for no
// ticks. Every tick carries the banner's identity and a serial number, and a
// banner honours only the one it most recently issued, so two banners on a
// screen don't advance each other and a tick delivered twice doesn't double
// the animation.
//
// A banner has no pane: it is a heading drawn straight onto the screen. It is
// as wide as its text needs; check Fits before placing one in a narrow rect,
// and fall back to a plain title when it doesn't.
package banner

import (
	"math"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/internal/tick"
)

// DefaultPeriod is how long one sweep of the gradient across the letters
// takes when Options.Period is zero.
const DefaultPeriod = 4 * time.Second

// frame is how often the gradient moves.
const frame = 60 * time.Millisecond

// Options configures a banner. Start from theme.Banner(), which fills the
// colours from the palette.
type Options struct {
	// Text is what the banner spells. Empty draws nothing.
	Text string
	// Colours are the gradient's stops. The sweep runs from the first to the
	// last and wraps back to the first, so the loop has no seam. One colour
	// draws the letters flat; none leaves them unstyled.
	Colours []lipgloss.TerminalColor
	// Shadow colours the box-drawing strokes beside and under each letter.
	Shadow lipgloss.TerminalColor
	// Static draws the gradient where it starts and never moves it.
	Static bool
	// Period is how long one sweep takes. Zero means DefaultPeriod.
	Period time.Duration
}

// tickMsg advances one banner's gradient.
type tickMsg struct{ id, serial int }

var ids atomic.Int64

// Model is a banner.
type Model struct {
	id     int
	serial int
	lines  [rows][]rune
	width  int
	stops  []stop
	shadow lipgloss.Style
	moves  bool
	step   float64
	phase  float64
}

// stop is one gradient colour: its RGB when it has one, so neighbouring stops
// can be blended, and the colour as given otherwise.
type stop struct {
	given lipgloss.TerminalColor
	rgb   colorful.Color
	ok    bool
}

// New builds a banner.
func New(opts Options) Model {
	m := Model{id: int(ids.Add(1))}
	for _, r := range strings.ToUpper(opts.Text) {
		g, ok := font[r]
		if !ok {
			g = font[' ']
		}
		for y := range rows {
			m.lines[y] = append(m.lines[y], []rune(g[y])...)
		}
	}
	m.width = len(m.lines[0])
	for _, c := range opts.Colours {
		s := stop{given: c}
		s.rgb, s.ok = rgbOf(c)
		m.stops = append(m.stops, s)
	}
	if len(m.stops) > 1 {
		m.stops = append(m.stops, m.stops[0])
	}
	if opts.Shadow != nil {
		m.shadow = lipgloss.NewStyle().Foreground(opts.Shadow)
	}
	period := opts.Period
	if period <= 0 {
		period = DefaultPeriod
	}
	m.step = float64(frame) / float64(period)
	m.moves = !opts.Static && len(m.stops) > 2 && m.width > 0
	return m
}

// Init starts the sweep, or returns nil for a banner that doesn't move.
func (m Model) Init() tea.Cmd {
	if !m.moves {
		return nil
	}
	return m.next()
}

func (m Model) next() tea.Cmd {
	msg := tickMsg{id: m.id, serial: m.serial}
	return tick.After(frame, func(time.Time) tea.Msg { return msg })
}

// Update advances the gradient on the banner's own ticks and ignores every
// other message.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	t, ok := msg.(tickMsg)
	if !ok || !m.moves || t.id != m.id || t.serial != m.serial {
		return m, nil
	}
	m.serial++
	m.phase = math.Mod(m.phase+m.step, 1)
	return m, m.next()
}

// Width is how many columns the banner takes; zero when it has no text.
func (m Model) Width() int { return m.width }

// Height is how many rows the banner takes; zero when it has no text.
func (m Model) Height() int {
	if m.width == 0 {
		return 0
	}
	return rows
}

// Fits reports whether the banner fits in w columns.
func (m Model) Fits(w int) bool { return m.width > 0 && m.width <= w }

// View draws the banner at its own width, Height rows tall.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	out := make([]string, rows)
	for y, line := range m.lines {
		var b strings.Builder
		for x, r := range line {
			switch {
			case r == ' ':
				b.WriteByte(' ')
			case face(r):
				b.WriteString(m.faceStyle(x, y).Render(string(r)))
			default:
				b.WriteString(m.shadow.Render(string(r)))
			}
		}
		out[y] = b.String()
	}
	return strings.Join(out, "\n")
}

// faceStyle is the gradient's colour at column x of row y. Rows are offset a
// little so the sweep runs on a diagonal rather than as vertical bars.
func (m Model) faceStyle(x, y int) lipgloss.Style {
	s := lipgloss.NewStyle()
	switch len(m.stops) {
	case 0:
		return s
	case 1:
		return s.Foreground(m.stops[0].given)
	}
	pos := float64(x)/float64(m.width) + float64(y)*0.04 - m.phase
	pos -= math.Floor(pos)
	seg := pos * float64(len(m.stops)-1)
	i := min(int(seg), len(m.stops)-2)
	a, b := m.stops[i], m.stops[i+1]
	if !a.ok || !b.ok {
		if seg-float64(i) < 0.5 {
			return s.Foreground(a.given)
		}
		return s.Foreground(b.given)
	}
	return s.Foreground(lipgloss.Color(a.rgb.BlendLuv(b.rgb, seg-float64(i)).Clamped().Hex()))
}

// rgbOf converts a hex or ANSI-indexed colour to RGB.
func rgbOf(c lipgloss.TerminalColor) (colorful.Color, bool) {
	s, ok := c.(lipgloss.Color)
	if !ok || s == "" {
		return colorful.Color{}, false
	}
	tc := termenv.TrueColor.Color(string(s))
	if tc == nil {
		return colorful.Color{}, false
	}
	return termenv.ConvertToRGB(tc), true
}
