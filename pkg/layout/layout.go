// Package layout is a tiny declarative engine for composing Bubble Tea view
// strings. Every layout is a tree of Node; each Node knows how to render
// itself into a given geom.Rect. VStack and HStack split their allotment
// among Fixed- and Flex-sized children; ZStack overlays children.
//
// The point is to remove hand-written "m.h-2" math from callers. You
// describe what goes where and let the stack engine divide the pixels.
//
// Typical use:
//
//	root := layout.VStack(
//	    layout.Fixed(1, layout.RenderFunc(func(r geom.Rect) string { ... })),
//	    layout.Flex(1, body),
//	    layout.Fixed(1, layout.RenderFunc(func(r geom.Rect) string { ... })),
//	)
//	return root.Render(geom.New(0, 0, termW, termH))
//
// # Rects, not sizes
//
// A Node receives a geom.Rect rather than a bare (width, height): it learns
// where it sits in absolute terminal coordinates, not merely how big it is.
// Components store the rect they were handed and use it to answer mouse
// events without any marker injection into the rendered string — see
// pkg/geom.
//
// The root of a render calls geom.NextGen once per frame and seeds its rect
// with geom.New; every child inherits that generation as the rect propagates
// down. A nested root (pkg/tab rendering its active body's layout) must not
// call NextGen — it renders inside an existing frame and passes the
// generation from the rect it was given.
package layout

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
)

// Node is the unit of layout — something that can render itself into a given
// rect. Components participate by being wrapped in Sized (or any adapter that
// yields a Node).
type Node interface {
	Render(r geom.Rect) string
}

// RenderFunc adapts a render-into-rect function into a Node. It is the
// escape hatch for content the component adapters don't cover: close over
// whatever you need, render at r.W by r.H, and return the string.
type RenderFunc func(r geom.Rect) string

// Render satisfies Node.
func (f RenderFunc) Render(r geom.Rect) string { return f(r) }

// Sizer is satisfied by any component the layout engine can place: it accepts
// the rect it should occupy and renders into it. Every interactive component
// in tuilib satisfies this, as do the single-row bars.
type Sizer interface {
	SetRect(r geom.Rect)
	View() string
}

// Sized wraps a component as a Node, handing it its rect at render time.
// Pass a pointer: &m.list, &m.body.
func Sized(s Sizer) Node {
	return RenderFunc(func(r geom.Rect) string {
		s.SetRect(r)
		return s.View()
	})
}

// Bar wraps a single-row component (breadcrumb.Model, statusbar.Model, …) as
// a Node. It behaves exactly as Sized — the name documents intent, since a
// bar is expected to sit in a Fixed(1, …) slot and render one row.
func Bar(s Sizer) Node { return Sized(s) }

// Item is a child of VStack or HStack. Construct with Fixed or Flex.
type Item struct {
	node   Node
	size   int // fixed cells when flex==false; flex weight otherwise
	isFlex bool
}

// Fixed reserves an exact number of cells (rows in VStack, columns in
// HStack) for node.
func Fixed(size int, node Node) Item {
	return Item{node: node, size: size, isFlex: false}
}

// Flex asks for a proportional share of the space remaining after all
// Fixed items are accounted for. Weight picks the share ratio:
// Flex(2, ...) + Flex(1, ...) gives the first twice as much as the second.
// A non-positive weight is clamped to 1.
func Flex(weight int, node Node) Item {
	if weight <= 0 {
		weight = 1
	}
	return Item{node: node, size: weight, isFlex: true}
}

// VStack stacks items top to bottom within its allotted height.
func VStack(items ...Item) Node { return vstack(items) }

type vstack []Item

func (v vstack) Render(r geom.Rect) string {
	sizes := distribute(v, r.H)
	parts := make([]string, len(v))
	y := r.Y
	for i, it := range v {
		parts[i] = it.node.Render(geom.Rect{X: r.X, Y: y, W: r.W, H: sizes[i], Gen: r.Gen})
		y += sizes[i]
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// HStack stacks items left to right within its allotted width.
func HStack(items ...Item) Node { return hstack(items) }

type hstack []Item

func (h hstack) Render(r geom.Rect) string {
	sizes := distribute(h, r.W)
	parts := make([]string, len(h))
	x := r.X
	for i, it := range h {
		parts[i] = it.node.Render(geom.Rect{X: x, Y: r.Y, W: sizes[i], H: r.H, Gen: r.Gen})
		x += sizes[i]
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// Center renders child at its natural size (given by naturalW/naturalH)
// padded to the parent's rect with surrounding whitespace. Useful as the
// overlay layer inside a ZStack.
//
// The child's rect is offset to where lipgloss.Place will actually draw it,
// so a centered modal hit-tests against its visible position rather than the
// parent's origin.
func Center(naturalW, naturalH int, child Node) Node {
	return RenderFunc(func(r geom.Rect) string {
		return lipgloss.Place(r.W, r.H,
			lipgloss.Center, lipgloss.Center,
			child.Render(geom.CenterIn(r, naturalW, naturalH)))
	})
}

// ZStack layers overlay on top of base. Both are rendered into the full
// rect. Compositing happens cell-by-cell: cells that are spaces in the
// overlay pass the base through; non-space cells replace base at that
// column. Empty overlay rows pass through entirely.
//
// In practice this means a centered modal drawn with Center(...) only
// blots out the modal's bounding box; pane borders and content to the
// left and right of the modal stay visible. Wide characters and ANSI
// styles are handled via x/ansi cell-aware cutting.
//
// A block element (U+2580–U+259F: half blocks, quadrants, eighths, shades)
// drawn with no background of its own leaves part of its cell undrawn, and
// that part shows the base: the cell takes the background of the base cell
// under it. This is what lets a shadow or a lip drawn on a modal's parent
// sit on whatever the modal covers, rather than on the terminal's own
// background.
//
// Both layers receive the same rect, so a component in the base still
// believes it owns cells the overlay covers. Occlusion is the host screen's
// job: while a modal is up, forward messages to the modal alone (see the
// confirm and alert examples) so the covered components never see the click.
func ZStack(base, overlay Node) Node { return zstack{base: base, overlay: overlay} }

type zstack struct {
	base, overlay Node
}

func (z zstack) Render(r geom.Rect) string {
	baseView := z.base.Render(r)
	if z.overlay == nil {
		return baseView
	}
	overlayView := z.overlay.Render(r)
	baseLines := strings.Split(baseView, "\n")
	overlayLines := strings.Split(overlayView, "\n")

	// The base is padded to the rows and columns its rect promises before
	// anything is spliced into it. A node is free to render short — a bare
	// RenderFunc returning one line is legal — but the splice below cuts
	// the base at the overlay's left edge and concatenates, so a base line
	// that stops before that edge would drag the overlay left, and a base
	// with too few lines would cut the overlay off at the bottom. Padding
	// here rather than asking every host to render flush keeps the overlay
	// where the layout put it.
	for len(baseLines) < len(overlayLines) && len(baseLines) < r.H {
		baseLines = append(baseLines, "")
	}
	for i, ol := range overlayLines {
		if i >= len(baseLines) {
			break
		}
		// Cell-aware compositing: find the first and last painted cell in
		// the overlay row, then splice those columns from overlay into the
		// base, leaving the surrounding base cells intact.
		left, right := paintedBounds(ol)
		if left < 0 {
			continue
		}
		baseLines[i] = padCells(ansi.Cut(baseLines[i], 0, left), left) +
			"\x1b[0m" +
			ansi.Cut(underlay(ol, backgrounds(baseLines[i])), left, right+1) +
			"\x1b[0m" +
			ansi.Cut(baseLines[i], right+1, r.W)
	}
	return strings.Join(baseLines, "\n")
}

// padCells right-pads s with spaces until it is width cells wide.
func padCells(s string, width int) string {
	if gap := width - ansi.StringWidth(stripANSI(s)); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// paintedBounds returns the cell index of the first and last painted cell in
// s, or (-1, -1) when none is. A cell is painted when it draws a glyph or
// carries a background: a filled modal's blank rows and padding cover the
// base, while the plain padding Center adds around it, and a margin drawn on
// its parent, stay transparent.
func paintedBounds(s string) (int, int) {
	left, right := -1, -1
	walkCells(s, nil, func(cell int, r rune, w int, bg string) {
		if w == 0 || (r == ' ' && bg == "") {
			return
		}
		if left < 0 {
			left = cell
		}
		right = cell + w - 1
	})
	return left, right
}

// backgrounds returns the background SGR parameters in force at each cell
// of s ("" for none). A wide glyph's trailing cells repeat its background.
func backgrounds(s string) []string {
	var out []string
	walkCells(s, nil, func(_ int, _ rune, w int, bg string) {
		for range w {
			out = append(out, bg)
		}
	})
	return out
}

// underlay gives every block element in s that has no background of its
// own the background under it, from under (indexed by cell): the part of
// the cell the glyph leaves undrawn then shows the base, not the terminal.
func underlay(s string, under []string) string {
	var b strings.Builder
	b.Grow(len(s))
	walkCells(s, func(seq string) { b.WriteString(seq) }, func(cell int, r rune, _ int, bg string) {
		if bg == "" && blockElement(r) && cell < len(under) && under[cell] != "" {
			b.WriteString("\x1b[" + under[cell] + "m" + string(r) + "\x1b[49m")
			return
		}
		b.WriteRune(r)
	})
	return b.String()
}

// blockElement reports whether r is in the Block Elements range, whose
// glyphs cover only part of their cell (█ aside, which covers all of it and
// so looks the same either way).
func blockElement(r rune) bool { return r >= 0x2580 && r <= 0x259f }

// walkCells walks s, handing each escape sequence to esc (when non-nil) and
// each rune to cell with the cell index it starts at, its width (0 for a
// combining mark) and the background SGR parameters in force.
func walkCells(s string, esc func(seq string), cell func(at int, r rune, w int, bg string)) {
	at, bg := 0, ""
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) {
			j := -1
			switch s[i+1] {
			case '[':
				j = i + 2
				for j < len(s) && !(s[j] >= '@' && s[j] <= '~') {
					j++
				}
				if j < len(s) && s[j] == 'm' {
					bg = sgrBackground(bg, s[i+2:j])
				}
				j = min(j+1, len(s))
			case ']':
				if k := strings.Index(s[i:], "\x1b\\"); k >= 0 {
					j = i + k + 2
				} else if k := strings.IndexByte(s[i:], 0x07); k >= 0 {
					j = i + k + 1
				} else {
					j = len(s)
				}
			}
			if j >= 0 {
				if esc != nil {
					esc(s[i:j])
				}
				i = j
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		w := ansi.StringWidth(string(r))
		cell(at, r, w, bg)
		at += w
	}
}

// sgrBackground returns the background parameters in force after the SGR
// parameters params, given those in force before ("" for none). Extended-
// colour operands are skipped, so the 0 in 38;2;255;0;0 isn't read as a
// reset.
func sgrBackground(bg, params string) string {
	toks := strings.Split(params, ";")
	for i := 0; i < len(toks); i++ {
		t, _, sub := strings.Cut(toks[i], ":")
		switch {
		case (t == "" || t == "0") && !sub, t == "49":
			bg = ""
		case t == "38" || t == "48" || t == "58":
			n := 1
			if !sub && i+1 < len(toks) {
				switch toks[i+1] {
				case "5":
					n = 3
				case "2":
					n = 5
				}
			}
			n = min(n, len(toks)-i)
			if t == "48" {
				bg = strings.Join(toks[i:i+n], ";")
			}
			i += n - 1
		case len(t) == 2 && t[0] == '4' && t[1] <= '7',
			len(t) == 3 && t[:2] == "10" && t[2] <= '7':
			bg = t
		}
	}
	return bg
}

// distribute divides total among items. Fixed items take their declared
// size; flex items share whatever remains in proportion to their weights.
// Any rounding remainder goes to the last flex child so the allotment
// sums exactly to total.
func distribute(items []Item, total int) []int {
	sizes := make([]int, len(items))
	fixedSum := 0
	weightSum := 0
	for _, it := range items {
		if it.isFlex {
			weightSum += it.size
		} else {
			fixedSum += it.size
		}
	}
	remaining := total - fixedSum
	if remaining < 0 {
		remaining = 0
	}
	allocated := 0
	lastFlex := -1
	for i, it := range items {
		if it.isFlex {
			if weightSum > 0 {
				sizes[i] = remaining * it.size / weightSum
			}
			allocated += sizes[i]
			lastFlex = i
		} else {
			sizes[i] = it.size
		}
	}
	if lastFlex >= 0 {
		sizes[lastFlex] += remaining - allocated
	}
	return sizes
}

// stripANSI returns s with ANSI escape sequences removed. Used by ZStack
// to detect whether an overlay line is "visually empty" regardless of
// styling codes.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= '@' && s[i] <= '~') {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
