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
	for i, ol := range overlayLines {
		if i >= len(baseLines) {
			break
		}
		plain := stripANSI(ol)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		// Cell-aware compositing: find the first and last non-space cell
		// in the overlay, then splice those columns from overlay into
		// the base, leaving the surrounding base cells intact.
		left, right := nonSpaceBounds(plain)
		if left < 0 {
			continue
		}
		baseLines[i] = ansi.Cut(baseLines[i], 0, left) +
			"\x1b[0m" +
			ansi.Cut(ol, left, right+1) +
			"\x1b[0m" +
			ansi.Cut(baseLines[i], right+1, r.W)
	}
	return strings.Join(baseLines, "\n")
}

// nonSpaceBounds returns the cell-index of the first and last non-space
// rune in s (which must be ANSI-stripped). Returns (-1, -1) when s is
// entirely whitespace.
func nonSpaceBounds(s string) (int, int) {
	left, right, cell := -1, -1, 0
	for _, r := range s {
		w := ansi.StringWidth(string(r))
		if w == 0 {
			continue
		}
		if r != ' ' {
			if left < 0 {
				left = cell
			}
			right = cell + w - 1
		}
		cell += w
	}
	return left, right
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
