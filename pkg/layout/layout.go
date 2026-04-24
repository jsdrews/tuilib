// Package layout is a tiny declarative engine for composing Bubble Tea view
// strings. Every layout is a tree of Node; each Node knows how to render
// itself at a given (width, height). VStack and HStack split their allotment
// among Fixed- and Flex-sized children; ZStack overlays children.
//
// The point is to remove hand-written "m.h-2" math from callers. You
// describe what goes where and let the stack engine divide the pixels.
//
// Typical use:
//
//	root := layout.VStack(
//	    layout.Fixed(1, layout.RenderFunc(func(w, h int) string { ... })),
//	    layout.Flex(1, body),
//	    layout.Fixed(1, layout.RenderFunc(func(w, h int) string { ... })),
//	)
//	return root.Render(termW, termH)
package layout

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Node is the unit of layout — something that can render itself at a given
// outer size. Components participate by being wrapped in RenderFunc (or any
// adapter that yields a Node).
type Node interface {
	Render(w, h int) string
}

// RenderFunc adapts a render-at-size function into a Node. It is the
// primary bridge from existing components: close over the component, set
// its dimensions inside the func, and return its View().
type RenderFunc func(w, h int) string

// Render satisfies Node.
func (f RenderFunc) Render(w, h int) string { return f(w, h) }

// Widther is satisfied by components sized by width alone (single-row bars
// like breadcrumb and statusbar). Bar wraps one into a Node.
type Widther interface {
	SetWidth(int)
	View() string
}

// Sizer is satisfied by components sized by both width and height. Sized
// wraps one into a Node.
type Sizer interface {
	SetDimensions(w, h int)
	View() string
}

// Bar wraps a single-row component (breadcrumb.Model, statusbar.Model, …)
// as a Node. The height argument to Render is ignored — these components
// always render one row. Pass a pointer: &m.bc, &m.sb.
func Bar(w Widther) Node {
	return RenderFunc(func(width, _ int) string {
		w.SetWidth(width)
		return w.View()
	})
}

// Sized wraps a two-dimensional component (pane.Pane, list.Model, …) as a
// Node. Pass a pointer: &m.list, &m.body.
func Sized(s Sizer) Node {
	return RenderFunc(func(w, h int) string {
		s.SetDimensions(w, h)
		return s.View()
	})
}

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

func (v vstack) Render(w, h int) string {
	sizes := distribute(v, h)
	parts := make([]string, len(v))
	for i, it := range v {
		parts[i] = it.node.Render(w, sizes[i])
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// HStack stacks items left to right within its allotted width.
func HStack(items ...Item) Node { return hstack(items) }

type hstack []Item

func (h hstack) Render(w, height int) string {
	sizes := distribute(h, w)
	parts := make([]string, len(h))
	for i, it := range h {
		parts[i] = it.node.Render(sizes[i], height)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// Center renders child at its natural size (given by naturalW/naturalH)
// padded to the parent's (w, h) with surrounding whitespace. Useful as the
// overlay layer inside a ZStack.
func Center(naturalW, naturalH int, child Node) Node {
	return RenderFunc(func(w, h int) string {
		return lipgloss.Place(w, h,
			lipgloss.Center, lipgloss.Center,
			child.Render(naturalW, naturalH))
	})
}

// ZStack layers overlay on top of base. Both are rendered at the full
// (w, h). Lines in overlay that consist entirely of whitespace pass the
// base through; any other line replaces the base line at that row.
//
// This is a deliberately simple compositor — it covers the common case
// of a centered modal drawn with Center(...). For anything richer,
// write a RenderFunc and do the compositing manually.
func ZStack(base, overlay Node) Node { return zstack{base: base, overlay: overlay} }

type zstack struct {
	base, overlay Node
}

func (z zstack) Render(w, h int) string {
	baseView := z.base.Render(w, h)
	if z.overlay == nil {
		return baseView
	}
	overlayView := z.overlay.Render(w, h)
	baseLines := strings.Split(baseView, "\n")
	overlayLines := strings.Split(overlayView, "\n")
	for i, ol := range overlayLines {
		if i >= len(baseLines) {
			break
		}
		if strings.TrimSpace(stripANSI(ol)) == "" {
			continue
		}
		baseLines[i] = ol
	}
	return strings.Join(baseLines, "\n")
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
			for i < len(s) && !((s[i] >= '@' && s[i] <= '~')) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
