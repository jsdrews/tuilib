package pane

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	tuiansi "github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/surface"
)

// lineInsets is the edge of a line border: one cell on every side.
var lineInsets = surface.Insets{Top: 1, Bottom: 1, Left: 1, Right: 1}

// generic reports whether the pane draws its edge from surface data. A pane
// with no surface, or a lined one (Outlined and its variants), draws the line
// border it always has, which is what keeps Outlined byte-identical.
func (p Pane) generic() bool {
	if p.surf.IsZero() {
		return false
	}
	_, lined := p.surf.Surface().Lined()
	return !lined
}

// insets is how many cells the edge takes on each side.
func (p Pane) insets() surface.Insets {
	if p.generic() {
		return p.surf.Insets()
	}
	return lineInsets
}

// colour resolves a role in the pane's current focus state.
func (p Pane) colour(r surface.Role) lipgloss.TerminalColor {
	return p.surf.Colour(r, p.focused)
}

// pick is a cell's role for the current focus state.
func (p Pane) pick(blurred, focused surface.Role) surface.Role {
	if p.focused && focused != surface.None {
		return focused
	}
	return blurred
}

func (p Pane) cellStyle(c surface.Cell) lipgloss.Style {
	s := lipgloss.NewStyle()
	if fg := p.colour(p.pick(c.FG, c.FGFocused)); fg != nil {
		s = s.Foreground(fg)
	}
	if bg := p.colour(p.pick(c.BG, c.BGFocused)); bg != nil {
		s = s.Background(bg)
	}
	if c.Bold || (p.focused && c.BoldFocused) {
		s = s.Bold(true)
	}
	return s
}

// bodyStyle is what the fill pass paints the body with: the surface's fill
// and, on a filled surface, its text colour.
func (p Pane) bodyStyle() (lipgloss.Style, bool) {
	s := p.surf.Surface()
	bg := p.colour(surface.Fill)
	if bg == nil {
		return lipgloss.Style{}, false
	}
	st := lipgloss.NewStyle().Background(bg)
	if fg := p.colour(s.Text); fg != nil {
		st = st.Foreground(fg)
	}
	return st, true
}

// renderSurface draws body inside the surface's rings. body is the pane's
// inner region — header, divider, viewport, scrollbars — at the inner size.
func (p Pane) renderSurface(body string, slots map[BorderPosition]string) string {
	in := p.insets()
	w := max(0, p.width-in.Left-in.Right)
	h := max(0, p.height-in.Top-in.Bottom)

	lines := strings.Split(body, "\n")
	inner := make([]string, h)
	fill, filled := p.bodyStyle()
	for i := range inner {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		l = fit(l, w)
		if filled {
			l = tuiansi.RenderFill(fill.Render, l)
		}
		inner[i] = l
	}
	return strings.Join(p.ring(0, p.width, p.height, inner, slots), "\n")
}

// ring wraps inner (the rendered region inside ring i and every ring inside
// it) in ring i, returning lines w wide and h tall.
func (p Pane) ring(i, w, h int, inner []string, slots map[BorderPosition]string) []string {
	rings := p.surf.Surface().Rings
	if i == len(rings) {
		return inner
	}
	r := rings[i]
	l, rt, t, b := btoi(r.Left.Drawn()), btoi(r.Right.Drawn()), btoi(r.Top.Drawn()), btoi(r.Bottom.Drawn())
	mid := p.ring(i+1, w-l-rt, h-t-b, inner, slots)
	span := w - l - rt

	var out []string
	if t == 1 {
		out = append(out, p.edgeRow(r.Left.Drawn(), r.TopLeft, r.Top, span, r.Right.Drawn(), r.TopRight, true, slots))
	}
	for j, m := range mid {
		row := m
		if l == 1 {
			row = p.sideCell(r.Left, j == 0 && t == 0) + row
		}
		if rt == 1 {
			row += p.sideCell(r.Right, j == 0 && t == 0)
		}
		out = append(out, row)
	}
	if b == 1 {
		out = append(out, p.edgeRow(r.Left.Drawn(), r.BottomLeft, r.Bottom, span, r.Right.Drawn(), r.BottomRight, false, slots))
	}
	return out
}

// sideCell is one cell of a vertical side; first marks the side's top cell.
func (p Pane) sideCell(s surface.Side, first bool) string {
	c := s.Cell
	if first && s.First.Drawn() {
		c = s.First
	}
	return p.cellStyle(c).Render(c.Glyph)
}

// edgeRow is a horizontal side with its corners. A text row carries the
// slots: the title and top-right along the top, the three bottom slots below.
func (p Pane) edgeRow(left bool, lc surface.Cell, s surface.Side, span int, right bool, rc surface.Cell, top bool, slots map[BorderPosition]string) string {
	var row string
	if left {
		row += p.cornerCell(lc, s)
	}
	switch {
	case s.Text && top:
		row += p.textRow(s, span, slots, TopLeftBorder, TopMiddleBorder, TopRightBorder)
	case s.Text:
		row += p.textRow(s, span, slots, BottomLeftBorder, BottomMiddleBorder, BottomRightBorder)
	default:
		row += p.run(s, span)
	}
	if right {
		row += p.cornerCell(rc, s)
	}
	return row
}

// cornerCell is c, or the side's own cell when the corner has no glyph.
func (p Pane) cornerCell(c surface.Cell, s surface.Side) string {
	if !c.Drawn() {
		c = s.Cell
	}
	return p.cellStyle(c).Render(c.Glyph)
}

// run repeats a side's glyph across n cells, with First at the start.
func (p Pane) run(s surface.Side, n int) string {
	if n <= 0 {
		return ""
	}
	if s.First.Drawn() {
		return p.cellStyle(s.First).Render(s.First.Glyph) + p.cellStyle(s.Cell).Render(strings.Repeat(s.Glyph, n-1))
	}
	return p.cellStyle(s.Cell).Render(strings.Repeat(s.Glyph, n))
}

// textRow lays the slot text over a side n cells wide: left-aligned, centred,
// right-aligned, like the slots on a line border. The slot holding the title
// takes the surface's title colour.
func (p Pane) textRow(s surface.Side, n int, slots map[BorderPosition]string, lp, mp, rp BorderPosition) string {
	if n <= 0 {
		return ""
	}
	sf := p.surf.Surface()
	bg := p.colour(p.pick(s.BG, s.BGFocused))
	text := func(pos BorderPosition) string {
		str := slots[pos]
		if str == "" {
			return ""
		}
		role, bold := sf.Slot, false
		if p.title != "" && pos == p.titlePos {
			role, bold = p.pick(sf.Title, sf.TitleFocused), sf.TitleBold
		}
		st := lipgloss.NewStyle().Bold(bold)
		if fg := p.colour(role); fg != nil {
			st = st.Foreground(fg)
		}
		if bg != nil {
			st = st.Background(bg)
		}
		return tuiansi.RenderFill(st.Render, str)
	}
	left, mid, right := text(lp), text(mp), text(rp)

	lw, mw, rw := ansi.StringWidth(left), ansi.StringWidth(mid), ansi.StringWidth(right)
	if lw+mw+rw > n {
		return fit(left+mid+right, n)
	}
	gap := func(k int) string {
		if k <= 0 {
			return ""
		}
		return p.cellStyle(s.Cell).Render(strings.Repeat(s.Glyph, k))
	}
	leftGap := max(0, n/2-lw-mw/2)
	if mw == 0 {
		leftGap = n - lw - rw
	}
	rightGap := max(0, n-lw-mw-rw-leftGap)
	return left + gap(leftGap) + mid + gap(rightGap) + right
}

// fit cuts or pads s to exactly n cells.
func fit(s string, n int) string {
	w := ansi.StringWidth(s)
	if w > n {
		return ansi.Truncate(s, n, "")
	}
	return s + strings.Repeat(" ", n-w)
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
