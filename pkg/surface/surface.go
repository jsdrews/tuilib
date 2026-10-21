package surface

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Surface is how a component's edge and body are drawn. It is a value, like
// lipgloss.Border: the pane interprets it, so a surface declares geometry and
// colour roles and never draws anything itself. A custom surface is a struct
// literal; start from a built-in and change what differs.
type Surface struct {
	// Name identifies the surface in config files and pickers.
	Name string

	// Rings are the edge, outermost first. Each ring takes one cell on each
	// side it draws, so a surface's inset on a side is the number of rings
	// drawing there. Margins and shadows are rings too: a surface draws only
	// inside the rect it is given.
	Rings []Ring

	// Fill and FillFocused are the body's background, blurred and focused.
	// None leaves the body unfilled, as Outlined and Accented do.
	Fill, FillFocused Role
	// Text is the body's foreground on a filled surface. None keeps the
	// terminal's own.
	Text Role

	// Title and TitleFocused colour the title slot. TitleBold makes it bold.
	Title, TitleFocused Role
	TitleBold           bool
	// Slot colours the other slots: counters, hints, errors.
	Slot Role
	// Brackets controls how slot text set into a line meets it — inline,
	// "┐ title ┌" or "┤ title ├". Only a lined surface has a line to set
	// text into; the rest ignore it.
	Brackets Brackets

	// Divider is the row the pane draws under a pinned header.
	Divider DividerStyle
	// Selection is how a selected row is painted.
	Selection SelectionStyle
	// Bar is the role the breadcrumb and statusbar sit on.
	Bar Role

	// Overlay is the form this surface takes for modals. Nil derives it: the
	// same surface with its body filled in the Overlay tone — filled even
	// when the surface itself isn't, since a modal must cover what is under
	// it.
	Overlay *Surface
	// Field is the form this surface takes for single-line components —
	// input, filter, toggle — which screens place in three-row slots. It
	// must take at most one cell of edge on each side. Nil derives it: a
	// lined surface is its own field, and anything else draws an outlined
	// field, Material's outlined text field.
	Field *Surface

	// Needs is what the terminal must offer to draw this surface, and
	// Fallback is drawn instead when it doesn't. Nil ends the chain.
	Needs    Needs
	Fallback *Surface
}

// Ring is one cell's depth of edge around a surface. A side with an empty
// glyph isn't drawn and takes no space. Where two drawn sides meet, the
// corner cell is used; a side whose neighbours aren't drawn runs the full
// length of the ring.
type Ring struct {
	Top, Bottom, Left, Right Side

	TopLeft, TopRight, BottomLeft, BottomRight Cell
}

// onFill is r with every cell drawn on the parent moved onto the fill.
func (r Ring) onFill() Ring {
	cell := func(c Cell) Cell {
		if c.BG == Parent {
			c.BG = Fill
		}
		if c.BGFocused == Parent {
			c.BGFocused = Fill
		}
		return c
	}
	side := func(sd Side) Side {
		sd.Cell, sd.First = cell(sd.Cell), cell(sd.First)
		return sd
	}
	r.Top, r.Bottom, r.Left, r.Right = side(r.Top), side(r.Bottom), side(r.Left), side(r.Right)
	r.TopLeft, r.TopRight = cell(r.TopLeft), cell(r.TopRight)
	r.BottomLeft, r.BottomRight = cell(r.BottomLeft), cell(r.BottomRight)
	return r
}

// Side is one side of a ring.
type Side struct {
	Cell
	// First, when its glyph is set, replaces the side's first cell (the top
	// of a vertical side, the left of a horizontal one) — where a shadow
	// starts half a cell in, say.
	First Cell
	// Text marks a row that carries the pane's slots: the title on top, and
	// counters, hints and errors along the bottom. Every surface has one
	// text row on each.
	Text bool
}

// Cell is a glyph and its colours. FG and BG are roles: Fill for the body's
// current tone, Parent for whatever contains the surface. The Focused
// variants apply while the pane has focus; None means "same as blurred".
type Cell struct {
	Glyph             string
	FG, FGFocused     Role
	BG, BGFocused     Role
	Bold, BoldFocused bool
}

// Drawn reports whether the cell has a glyph.
func (c Cell) Drawn() bool { return c.Glyph != "" }

// Brackets is how slot text meets the line it is set into.
type Brackets int

const (
	// BracketsNone runs the line straight through, so text sits inline on
	// it: ── text ──.
	BracketsNone Brackets = iota
	// BracketsCorners flanks the text with inverted corners, like a tab
	// dipping into the pane: ┐ text ┌.
	BracketsCorners
	// BracketsTees flanks it with junction tees: ─┤ text ├─.
	BracketsTees
)

// DividerStyle is the row under a pinned header. FGActive applies while the header
// has input, which is how the filter shows it holds the keys.
type DividerStyle struct {
	Glyph        string
	FG, FGActive Role
	BG           Role
}

// SelectionStyle is how a selected row is painted.
type SelectionStyle struct {
	// FromTheme keeps each component's own selected style, as built by the
	// theme. Outlined does this, so it stays byte-identical to the look
	// before surfaces.
	FromTheme bool
	// BG is a focused pane's selected row, BGBlurred a blurred one's, and FG
	// the text on either.
	BG, BGBlurred, FG Role
}

// Needs is what a terminal must offer to draw a surface.
type Needs struct {
	// Profile is the poorest colour profile the surface still reads in.
	// termenv orders profiles richest first, so TrueColor (the zero value)
	// demands the most; ANSI256 is the usual floor for tonal surfaces and
	// Ascii accepts anything.
	Profile termenv.Profile
	// BlockGlyphs is set when the surface draws with block elements
	// (▄ ▀ ▌), which font-drawn terminals render with seams.
	BlockGlyphs bool
}

// Insets is how many cells a surface's edge takes on each side.
type Insets struct{ Top, Bottom, Left, Right int }

// Insets counts the drawn sides of every ring.
func (s Surface) Insets() Insets {
	var in Insets
	for _, r := range s.Rings {
		if r.Top.Drawn() {
			in.Top++
		}
		if r.Bottom.Drawn() {
			in.Bottom++
		}
		if r.Left.Drawn() {
			in.Left++
		}
		if r.Right.Drawn() {
			in.Right++
		}
	}
	return in
}

// Filled reports whether the surface paints its body.
func (s Surface) Filled() bool { return s.Fill != None || s.FillFocused != None }

// overlayForm is s.Overlay, or s with its body in the Overlay tone.
func (s Surface) overlayForm() Surface {
	if s.Overlay != nil {
		return *s.Overlay
	}
	o := s
	o.Name = s.Name + "-overlay"
	// A modal covers what is under it, so its body is filled even when the
	// surface's isn't: an unfilled overlay would let the panes behind show
	// through every blank cell.
	o.Fill, o.FillFocused = Overlay, Overlay
	if o.Text == None {
		o.Text = Text
	}
	// Edge cells inside the margin sit on the overlay's fill too: an
	// unfilled surface draws its title row and padding on the parent, which
	// for a modal is the pane it covers.
	o.Rings = make([]Ring, len(s.Rings))
	for i, r := range s.Rings {
		if i == 0 && s.Margined() {
			o.Rings[i] = r
			continue
		}
		o.Rings[i] = r.onFill()
	}
	return o
}

// fieldForm is s.Field, or s itself when s is lined, or an outlined field.
func (s Surface) fieldForm() Surface {
	if s.Field != nil {
		return *s.Field
	}
	if _, lined := s.Lined(); lined {
		return s
	}
	f := OutlinedWith(lipgloss.NormalBorder())
	f.Name = s.Name + "-field"
	return f
}

// lineRing is a ring of box-drawing lines in Edge, lit in EdgeActive while
// focused, whose top and bottom carry the slots.
func lineRing(b lipgloss.Border) Ring {
	side := func(g string, text bool) Side {
		return Side{Cell: Cell{Glyph: g, FG: Edge, FGFocused: EdgeActive, BG: Parent}, Text: text}
	}
	corner := func(g string) Cell { return Cell{Glyph: g, FG: Edge, FGFocused: EdgeActive, BG: Parent} }
	return Ring{
		Top: side(b.Top, true), Bottom: side(b.Bottom, true),
		Left: side(b.Left, false), Right: side(b.Right, false),
		TopLeft: corner(b.TopLeft), TopRight: corner(b.TopRight),
		BottomLeft: corner(b.BottomLeft), BottomRight: corner(b.BottomRight),
	}
}

// margin is the one-cell gap a filled surface keeps on its right and bottom,
// drawn on the parent so the gutter between cards is whatever contains them.
func margin() Ring {
	blank := Cell{Glyph: " ", BG: Parent}
	return Ring{
		Right: Side{Cell: blank}, Bottom: Side{Cell: blank},
		BottomRight: blank,
	}
}

// textRows is a ring of plain rows in the body's tone carrying the slots,
// padded one cell either side.
func textRows() Ring {
	row := Cell{Glyph: " ", BG: Fill}
	return Ring{
		Top: Side{Cell: row, Text: true}, Bottom: Side{Cell: row, Text: true},
		Left: Side{Cell: row}, Right: Side{Cell: row},
		TopLeft: row, TopRight: row, BottomLeft: row, BottomRight: row,
	}
}

// OutlinedWith is Outlined drawn with border's lines — rounded, thick, double.
func OutlinedWith(border lipgloss.Border) Surface {
	thick := Surface{
		Name:      "outlined",
		Rings:     []Ring{lineRing(lipgloss.ThickBorder())},
		Selection: SelectionStyle{FromTheme: true},
		Bar:       Bar,
		Divider:   DividerStyle{Glyph: "─", FG: Divider, FGActive: EdgeActive, BG: Parent},
		Needs:     Needs{Profile: termenv.Ascii},
	}
	s := thick
	s.Rings = []Ring{lineRing(border)}
	s.Overlay = &thick
	return s
}

// The built-in surfaces. Outlined is the default and draws today's look byte
// for byte; the rest fill their body, except Accented.
var (
	// Outlined is a line border around an unfilled body. Its overlay form
	// has thick lines, which is what makes a modal read as raised.
	Outlined = OutlinedWith(lipgloss.NormalBorder())

	// Banded is a card under a header band carrying the title. Focus colours
	// the band. It needs no block glyphs, which makes it the fallback for
	// the surfaces that do.
	Banded = Surface{
		Name: "banded",
		Rings: []Ring{
			margin(),
			{
				Top:        Side{Cell: Cell{Glyph: " ", BG: Band, BGFocused: EdgeActive}, Text: true},
				TopLeft:    Cell{Glyph: " ", BG: Band, BGFocused: EdgeActive},
				TopRight:   Cell{Glyph: " ", BG: Band, BGFocused: EdgeActive},
				Bottom:     Side{Cell: Cell{Glyph: " ", BG: Fill}, Text: true},
				Left:       Side{Cell: Cell{Glyph: " ", BG: Fill}},
				Right:      Side{Cell: Cell{Glyph: " ", BG: Fill}},
				BottomLeft: Cell{Glyph: " ", BG: Fill}, BottomRight: Cell{Glyph: " ", BG: Fill},
			},
		},
		Fill: Card, FillFocused: Card, Text: Text,
		Title: Text, TitleFocused: Screen, TitleBold: true, Slot: TextMuted,
		Divider:   DividerStyle{Glyph: "─", FG: Divider, FGActive: EdgeActive, BG: Fill},
		Selection: SelectionStyle{BG: Selection, BGBlurred: SelectionDim, FG: Text},
		Bar:       Screen,
		Needs:     Needs{Profile: termenv.ANSI256},
		Fallback:  &Outlined,
	}

	// Filled is a solid card whose half-block rows are its own edge. The
	// lips are its vertical margin too: the lower half of one card's ▀ and
	// the upper half of the next card's ▄ leave exactly one row of screen
	// between them, so it keeps a margin on the right only.
	Filled = Surface{
		Name: "filled",
		Rings: []Ring{
			{Right: Side{Cell: Cell{Glyph: " ", BG: Parent}}},
			{
				Top:    Side{Cell: Cell{Glyph: "▄", FG: Fill, BG: Parent}},
				Bottom: Side{Cell: Cell{Glyph: "▀", FG: Fill, BG: Parent}},
			},
			textRows(),
		},
		Fill: Card, FillFocused: CardRaised, Text: Text,
		Title: Text, TitleFocused: Accent, TitleBold: true, Slot: TextMuted,
		Divider:   DividerStyle{Glyph: "─", FG: Divider, FGActive: EdgeActive, BG: Fill},
		Selection: SelectionStyle{BG: Selection, BGBlurred: SelectionDim, FG: Text},
		Bar:       Screen,
		Needs:     Needs{Profile: termenv.ANSI256, BlockGlyphs: true},
		Fallback:  &Banded,
	}

	// Elevated is a card with a drop shadow below and to the right, and an
	// accent stripe down its left edge while focused. The shadow is its
	// margin.
	Elevated = Surface{
		Name: "elevated",
		Rings: []Ring{
			{
				Right:       Side{Cell: Cell{Glyph: "▌", FG: Shadow, BG: Parent}, First: Cell{Glyph: "▖", FG: Shadow, BG: Parent}},
				Bottom:      Side{Cell: Cell{Glyph: "▀", FG: Shadow, BG: Parent}, First: Cell{Glyph: " ", BG: Parent}},
				BottomRight: Cell{Glyph: "▘", FG: Shadow, BG: Parent},
			},
			{
				Top:         Side{Cell: Cell{Glyph: " ", BG: Fill}, Text: true},
				Bottom:      Side{Cell: Cell{Glyph: " ", BG: Fill}, Text: true},
				Left:        Side{Cell: Cell{Glyph: "▎", FG: Fill, FGFocused: Accent, BG: Fill}},
				Right:       Side{Cell: Cell{Glyph: " ", BG: Fill}},
				TopLeft:     Cell{Glyph: "▎", FG: Fill, FGFocused: Accent, BG: Fill},
				BottomLeft:  Cell{Glyph: "▎", FG: Fill, FGFocused: Accent, BG: Fill},
				TopRight:    Cell{Glyph: " ", BG: Fill},
				BottomRight: Cell{Glyph: " ", BG: Fill},
			},
		},
		Fill: Card, FillFocused: CardRaised, Text: Text,
		Title: Text, TitleFocused: Accent, TitleBold: true, Slot: TextMuted,
		Divider:   DividerStyle{Glyph: "─", FG: Divider, FGActive: EdgeActive, BG: Fill},
		Selection: SelectionStyle{BG: Selection, BGBlurred: SelectionDim, FG: Text},
		Bar:       Screen,
		Needs:     Needs{Profile: termenv.ANSI256, BlockGlyphs: true},
		Fallback:  &Banded,
	}

	// Accented is an unfilled body with a bar down its left edge, coloured
	// while focused.
	Accented = accented("▌", true)
)

// accentedLine is Accented drawn with a box-drawing bar, for terminals that
// draw block glyphs with seams.
var accentedLine = accented("┃", false)

func accented(bar string, blocks bool) Surface {
	barCell := Cell{Glyph: bar, FG: Edge, FGFocused: Accent, BG: Parent}
	row := Cell{Glyph: " ", BG: Parent}
	s := Surface{
		Name: "accented",
		Rings: []Ring{
			margin(),
			{
				Left: Side{Cell: barCell}, TopLeft: barCell, BottomLeft: barCell,
				Top: Side{Cell: row, Text: true}, Bottom: Side{Cell: row, Text: true},
				Right: Side{Cell: row}, TopRight: row, BottomRight: row,
			},
		},
		Title: Text, TitleFocused: Accent, TitleBold: true, Slot: TextMuted,
		Divider:   DividerStyle{Glyph: "─", FG: Divider, FGActive: EdgeActive, BG: Parent},
		Selection: SelectionStyle{BG: Selection, BGBlurred: SelectionDim, FG: Text},
		Bar:       Screen,
		Needs:     Needs{Profile: termenv.ANSI, BlockGlyphs: blocks},
	}
	if blocks {
		line := accented("┃", false)
		s.Fallback = &line
	} else {
		s.Fallback = &Outlined
	}
	return s
}

// Builtins are the surfaces the library ships, in picker order.
func Builtins() []Surface {
	return []Surface{Outlined, Filled, Banded, Elevated, Accented}
}

// Lined reports whether s is a single ring of lines around an unfilled body —
// Outlined and its variants — and returns that ring as a lipgloss.Border. A
// pane draws a lined surface as the line border it always drew, so Outlined
// stays byte-identical to the look before surfaces existed.
func (s Surface) Lined() (lipgloss.Border, bool) {
	if s.Filled() || len(s.Rings) != 1 {
		return lipgloss.Border{}, false
	}
	r := s.Rings[0]
	if !r.Top.Drawn() || !r.Bottom.Drawn() || !r.Left.Drawn() || !r.Right.Drawn() {
		return lipgloss.Border{}, false
	}
	return lipgloss.Border{
		Top: r.Top.Glyph, Bottom: r.Bottom.Glyph, Left: r.Left.Glyph, Right: r.Right.Glyph,
		TopLeft: r.TopLeft.Glyph, TopRight: r.TopRight.Glyph,
		BottomLeft: r.BottomLeft.Glyph, BottomRight: r.BottomRight.Glyph,
	}, true
}

// Margined reports whether s keeps a margin on its right — an outermost ring
// drawn on the parent there. The app shell pads the screen's left edge to
// match when the current surface does.
func (s Surface) Margined() bool {
	if _, lined := s.Lined(); lined || len(s.Rings) == 0 {
		return false
	}
	r := s.Rings[0]
	return r.Right.Drawn() && r.Right.BG == Parent
}

// TopGap reports whether s wants a row of the parent above it. A surface
// whose top edge starts with a solid row — a title, a band — reads as
// jammed against whatever is above it; one whose top edge is already a lip
// or a line (Filled's ▄, Outlined's border) brings its own gap. The app
// shell adds the row under the breadcrumb when the current surface asks.
func (s Surface) TopGap() bool {
	if _, lined := s.Lined(); lined {
		return false
	}
	for _, r := range s.Rings {
		if !r.Top.Drawn() {
			continue
		}
		c := r.Top.Cell
		return c.Glyph == " " || c.BG != Parent
	}
	return false
}
