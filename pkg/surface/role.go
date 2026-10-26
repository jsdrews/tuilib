// Package surface describes how a component's edge and body are drawn — a
// line border, a filled card, a header band, a drop shadow — as data, and
// derives the colours that data needs from a palette.
//
// A Surface never holds a colour. It names roles (Card, Shadow, EdgeActive,
// …), and a palette supplies them through Derive, so any surface can be drawn
// in any palette. Resolve pairs a surface with derived colours and the
// terminal's capabilities, following the surface's fallback chain until it
// reaches one the terminal can draw, and that resolved value is what a pane
// renders.
//
// The package imports nothing from tuilib: palettes, themes and panes all
// depend on it, and it depends on none of them. See docs/surfaces.md.
package surface

// Role is a named colour slot. Surfaces refer to colours only by role.
type Role int

const (
	// None leaves a colour unset: the component's own default foreground, or
	// no background at all.
	None Role = iota

	// Screen is behind and between surfaces: the palette's background.
	Screen
	// Card is a filled surface's body.
	Card
	// CardRaised is a filled surface's body while it has focus.
	CardRaised
	// Band is Banded's header and a filled surface's divider step.
	Band
	// Overlay is the body of a surface's overlay form — modals.
	Overlay
	// OverlayCard is a card drawn on an overlay: a field in a modal form.
	OverlayCard
	// OverlayCardRaised is an OverlayCard while it has focus.
	OverlayCardRaised
	// Button is a button drawn as a chip on a filled surface: a toggle's
	// side, a modal's choice, a form's submit. The chosen one takes Accent.
	Button
	// Shadow is Elevated's drop shadow.
	Shadow
	// Selection is a focused pane's selected row.
	Selection
	// SelectionDim is a blurred pane's selected row.
	SelectionDim
	// Bar is the breadcrumb and statusbar band under Outlined.
	Bar
	// Text is body text on a filled surface.
	Text
	// TextMuted is secondary text: slots, counters, hints.
	TextMuted
	// Edge is a line or bar on a blurred surface.
	Edge
	// EdgeActive is a line or bar on a focused surface, and the active band.
	EdgeActive
	// Divider is the rule under a pinned header.
	Divider
	// Accent is emphasis: a focused title, Accented's bar.
	Accent

	// Fill stands for whichever role the surface's body is filled with in its
	// current focus state — Card, CardRaised, Overlay. A lip or a stripe
	// drawn in Fill always matches the body it borders.
	Fill
	// Parent stands for whatever contains the surface. A cell on Parent is
	// emitted with no background, so the container paints it: an outer
	// pane's fill, or the screen. Margins and shadows sit on Parent.
	Parent

	roleCount
)

var roleNames = [...]string{
	None: "none", Screen: "screen", Card: "card", CardRaised: "card-raised",
	Band: "band", Overlay: "overlay", OverlayCard: "overlay-card",
	OverlayCardRaised: "overlay-card-raised", Button: "button", Shadow: "shadow",
	Selection:    "selection",
	SelectionDim: "selection-dim", Bar: "bar", Text: "text",
	TextMuted: "text-muted", Edge: "edge", EdgeActive: "edge-active",
	Divider: "divider", Accent: "accent", Fill: "fill", Parent: "parent",
}

func (r Role) String() string {
	if r < 0 || r >= roleCount {
		return "role(?)"
	}
	return roleNames[r]
}
