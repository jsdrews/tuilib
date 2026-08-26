// Package geom carries the geometry primitives shared by pkg/layout and the
// components it places. It is a leaf: nothing in tuilib is imported here, so
// both the layout engine and the components it sizes can depend on it without
// pointing at each other.
//
// A Rect is what the layout engine hands a component during render — where it
// sits in absolute terminal coordinates, not just how big it is. Components
// store the rect they were last given and use it to answer "was this click
// mine?" without any marker injection into the rendered string.
//
// Typical use inside a component:
//
//	func (m *Model) SetRect(r geom.Rect) { m.rect = r; … }
//
//	func (m *Model) Update(msg tea.Msg) (Model, tea.Cmd) {
//	    if e, ok := msg.(tea.MouseMsg); ok && m.rect.Hit(e.X, e.Y) {
//	        …
//	    }
//	}
//
// # Generations
//
// A component that wasn't drawn in the last frame still holds the rect it had
// when it was last visible — a hidden tab body, a modal that's been dismissed.
// Hit-testing against that stale rect would let an invisible component claim a
// click.
//
// Every rect therefore carries the render generation it was stamped with. The
// root of a render calls NextGen once per frame and seeds its rect with it;
// children inherit the value as it propagates down the tree. Hit reports false
// unless the rect's generation is the current one, so anything not drawn last
// frame silently declines every click.
//
// The counter is package-level because bubbletea's View has a value receiver
// and so cannot thread per-program state. Two concurrently-running programs in
// one process would share it and invalidate each other's rects; that is not a
// shape tuilib supports. Tests that build rects by hand get generation 0, which
// matches the counter's initial value, so hit-testing works without rendering.
package geom

import "sync/atomic"

// Rect is a component's absolute position and size in terminal cells, plus
// the render generation it was stamped with. X/Y are the top-left corner,
// measured from the top-left of the terminal.
type Rect struct {
	X, Y, W, H int
	// Gen is the render generation this rect was produced in. See the
	// package doc — Hit uses it to reject rects from earlier frames.
	Gen uint64
}

// New returns a rect stamped with the current generation. Layout uses it when
// seeding a render root; callers building a rect for a child should copy the
// parent's Gen instead so a single frame stays internally consistent.
func New(x, y, w, h int) Rect {
	return Rect{X: x, Y: y, W: w, H: h, Gen: Gen()}
}

// Contains reports whether the cell at (x, y) falls inside r, ignoring
// generation. Use Hit for click routing; this is the pure geometry test.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Fresh reports whether r was stamped in the current render generation.
func (r Rect) Fresh() bool { return r.Gen == Gen() }

// Hit reports whether (x, y) falls inside r *and* r was drawn in the current
// frame. This is the test components should use to claim a mouse event: a
// component that wasn't rendered last frame declines everything.
func (r Rect) Hit(x, y int) bool { return r.Fresh() && r.Contains(x, y) }

// Inset shrinks r by n cells on every side, clamping width and height at zero.
// Components use it to skip their own border when mapping a click to content.
func (r Rect) Inset(n int) Rect {
	out := Rect{X: r.X + n, Y: r.Y + n, W: r.W - 2*n, H: r.H - 2*n, Gen: r.Gen}
	if out.W < 0 {
		out.W = 0
	}
	if out.H < 0 {
		out.H = 0
	}
	return out
}

// Empty reports whether r covers no cells.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// CenterIn returns the rect a w×h child occupies when centered inside outer.
// It mirrors lipgloss.Place's centering exactly — the gap is split with
// integer division, biasing the remainder to the right and bottom — so a
// component that draws itself with Place can hit-test against the result.
// Offsets clamp at zero when the child is larger than outer.
func CenterIn(outer Rect, w, h int) Rect {
	dx, dy := (outer.W-w)/2, (outer.H-h)/2
	if dx < 0 {
		dx = 0
	}
	if dy < 0 {
		dy = 0
	}
	return Rect{X: outer.X + dx, Y: outer.Y + dy, W: w, H: h, Gen: outer.Gen}
}

// AnchorIn returns the rect a w×h child occupies when its top-left is placed
// at (x, y) inside outer, pushed back inside outer when it would overflow.
//
// This is the positioned counterpart to CenterIn, and it is what a context
// menu needs: opened by a right-click near the bottom-right corner, the box
// flips up and to the left instead of hanging off the edge. A child larger
// than outer clamps to outer's origin, so it is clipped from the far edge
// rather than sliding out of view at the near one.
func AnchorIn(outer Rect, x, y, w, h int) Rect {
	if x+w > outer.X+outer.W {
		x = outer.X + outer.W - w
	}
	if y+h > outer.Y+outer.H {
		y = outer.Y + outer.H - h
	}
	if x < outer.X {
		x = outer.X
	}
	if y < outer.Y {
		y = outer.Y
	}
	return Rect{X: x, Y: y, W: w, H: h, Gen: outer.Gen}
}

var generation atomic.Uint64

// NextGen advances the render generation and returns the new value. The root
// of a render tree calls it exactly once per frame, before rendering; nested
// roots (a tab body's layout, say) must not — they inherit the generation from
// the rect they were handed so the whole frame shares one value.
func NextGen() uint64 { return generation.Add(1) }

// Gen returns the current render generation.
func Gen() uint64 { return generation.Load() }
