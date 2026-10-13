package surface

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Caps is what the terminal offers.
type Caps struct {
	// Profile is the terminal's colour profile.
	Profile termenv.Profile
	// BlockGlyphs reports whether block elements draw without seams.
	BlockGlyphs bool
}

// Resolved is a surface paired with concrete colours, ready to draw. It is
// built only by Resolve, so a pane can never be handed roles it would have to
// guess at. The zero value is Outlined with no colours, which draws exactly
// what a pane drew before surfaces existed.
type Resolved struct {
	s        Surface
	c        Colours
	caps     Caps
	resolved bool
	// from is the surface that was asked for, when a fallback was drawn.
	from string
	why  string
}

// Resolve picks the surface to draw for s: s itself, or the first surface
// along its Fallback chain whose needs caps meets. A surface that fills its
// body also needs tonal colours, so a palette whose background has no RGB
// falls back too. The chain always ends at Outlined.
func Resolve(s Surface, c Colours, caps Caps) Resolved {
	asked := s.Name
	why := ""
	for {
		reason := unmet(s, c, caps)
		if reason == "" {
			break
		}
		if why == "" {
			why = reason
		}
		if s.Fallback == nil {
			s = Outlined
			break
		}
		s = *s.Fallback
	}
	r := Resolved{s: s, c: c, caps: caps, resolved: true}
	if s.Name != asked {
		r.from, r.why = asked, why
	}
	return r
}

// unmet names what caps or c lacks for s, or "" when s can be drawn.
func unmet(s Surface, c Colours, caps Caps) string {
	switch {
	case caps.Profile > s.Needs.Profile:
		return "too few colours"
	case s.Needs.BlockGlyphs && !caps.BlockGlyphs:
		return "block glyphs off"
	case s.Filled() && !c.Tonal():
		return "no tones for this palette"
	}
	return ""
}

// Surface is the surface that will be drawn.
func (r Resolved) Surface() Surface {
	if !r.resolved {
		return Outlined
	}
	return r.s
}

// Name is the drawn surface's name.
func (r Resolved) Name() string { return r.Surface().Name }

// FellBack reports the surface that was asked for and why it couldn't be
// drawn, when Resolve settled on a fallback.
func (r Resolved) FellBack() (asked, why string, ok bool) {
	return r.from, r.why, r.from != ""
}

// Insets is how many cells the drawn surface's edge takes on each side.
func (r Resolved) Insets() Insets { return r.Surface().Insets() }

// Colour resolves role for the given focus state. Fill becomes the body's
// tone in that state; Parent and None resolve to nil, meaning "emit no
// colour".
func (r Resolved) Colour(role Role, focused bool) lipgloss.TerminalColor {
	if role == Fill {
		s := r.Surface()
		role = s.Fill
		if focused && s.FillFocused != None {
			role = s.FillFocused
		}
	}
	if role == None || role == Parent {
		return nil
	}
	return r.c.Of(role)
}

// Overlay is the drawn surface's overlay form, resolved against the same
// colours and capabilities.
func (r Resolved) Overlay() Resolved {
	return Resolve(r.Surface().overlayForm(), r.c, r.caps)
}

// Colours are the colours the surface was resolved against.
func (r Resolved) Colours() Colours { return r.c }
