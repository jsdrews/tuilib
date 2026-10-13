package surface

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

// Seeds are the palette colours every role is derived from. Background and
// Foreground are the palette's canonical pair; the rest pass straight through
// to the roles of the same meaning.
type Seeds struct {
	// Background is what the screen is. Unset, Bar stands in for it.
	Background lipgloss.TerminalColor
	// Foreground is body text on a filled surface. Unset, BarText stands in.
	Foreground lipgloss.TerminalColor

	Bar, BarText, Muted, Subtle, Edge, EdgeActive, Accent lipgloss.TerminalColor
}

// Colours is every role resolved to a concrete colour for one palette and
// one colour profile.
type Colours struct {
	of      map[Role]lipgloss.TerminalColor
	tonal   bool
	profile termenv.Profile
}

// Of returns the colour for r, or nil when the role is unset (None, Parent,
// or a tone the palette couldn't derive).
func (c Colours) Of(r Role) lipgloss.TerminalColor {
	if c.of == nil {
		return nil
	}
	return c.of[r]
}

// Tonal reports whether the surface tones (Screen, Card, Shadow, …) could be
// derived. They can't when the background has no known RGB — an ANSI index,
// whose colour is whatever the terminal's scheme says — and a surface that
// fills its body then falls back to one that doesn't.
func (c Colours) Tonal() bool { return c.tonal }

// Profile is the colour profile the colours were rounded to.
func (c Colours) Profile() termenv.Profile { return c.profile }

// Derivation constants. They are starting points to tune by eye, not spec.
const (
	liftCard         = 0.08
	liftCardRaised   = 0.13
	liftBand         = 0.18
	liftOverlay      = 0.16
	liftSelectionDim = 0.12
	liftSelectionAlt = 0.25 // Selection, when EdgeActive has no RGB
	mixSelection     = 0.45
	lightCard        = 0.70
	lightCardRaised  = 0.90
	darkenBandLight  = 0.06
	shadowDark       = 0.60
	shadowLight      = 0.22

	// floorL is the least OKLab lightness distance a derived tone keeps from
	// the role beneath it.
	floorL = 0.03
	// floorQuantised is the same floor after rounding to the profile, where
	// only the palette's discrete steps are available.
	floorQuantised = 0.015
)

// floorPairs are the derived tones that must stay distinct from the tone
// they sit on: {derived, base}.
var floorPairs = [][2]Role{
	{Card, Screen},
	{CardRaised, Card},
	{Band, Card},
	{Selection, Card},
}

// Derive resolves every role for a palette's seeds. Surface tones are mixed in
// OKLab from Background toward Foreground (on a light palette, cards lighten
// toward white instead), kept a minimum lightness apart from the tone beneath
// them, and rounded to profile. An override replaces a role outright: it is
// rounded to profile but never pushed, because an author's choice is theirs.
//
// A role mixed from a colour with no known RGB (an ANSI index below 16) uses
// a recipe from the seeds alone, so a palette written in ANSI indices still
// gets every tone it can.
func Derive(s Seeds, overrides map[Role]lipgloss.TerminalColor, profile termenv.Profile) Colours {
	bg, fg := s.Background, s.Foreground
	if bg == nil {
		bg = s.Bar
	}
	if fg == nil {
		fg = s.BarText
	}

	c := Colours{of: map[Role]lipgloss.TerminalColor{}, profile: profile}
	pass := map[Role]lipgloss.TerminalColor{
		Bar: s.Bar, Text: fg, TextMuted: s.Muted, Edge: s.Edge,
		EdgeActive: s.EdgeActive, Divider: s.Subtle, Accent: s.Accent,
	}

	tones := map[Role]colorful.Color{}
	bgRGB, okBG := rgbOf(bg)
	fgRGB, okFG := rgbOf(fg)
	if okBG {
		c.tonal = true
		if !okFG {
			fgRGB = contrasting(bgRGB)
		}
		tones = deriveTones(bgRGB, fgRGB, s.EdgeActive)
	}

	for r, col := range pass {
		if col != nil {
			c.of[r] = quantise(col, profile)
		}
	}

	fixed := map[Role]bool{}
	for r, col := range overrides {
		if col == nil {
			continue
		}
		fixed[r] = true
		if t, ok := rgbOf(col); ok {
			tones[r] = t
		}
		c.of[r] = quantise(col, profile)
	}
	if !c.tonal {
		return c
	}

	dark := lightness(bgRGB) < lightness(fgRGB)
	for _, p := range floorPairs {
		d, base := p[0], p[1]
		if fixed[d] {
			continue
		}
		tones[d] = keepApart(tones[d], tones[base], dark, floorL)
	}
	for r, t := range tones {
		if !fixed[r] {
			c.of[r] = quantiseRGB(t, profile)
		}
	}
	if profile == termenv.ANSI256 {
		for _, p := range floorPairs {
			d, base := p[0], p[1]
			if fixed[d] {
				continue
			}
			c.of[d] = apartQuantised(tones[d], c.of[base], dark)
		}
	}
	return c
}

// deriveTones mixes the surface tones from the two seeds.
func deriveTones(bg, fg colorful.Color, edgeActive lipgloss.TerminalColor) map[Role]colorful.Color {
	white, black := colorful.Color{R: 1, G: 1, B: 1}, colorful.Color{}
	t := map[Role]colorful.Color{Screen: bg}
	if lightness(bg) < lightness(fg) {
		t[Card] = mix(bg, fg, liftCard)
		t[CardRaised] = mix(bg, fg, liftCardRaised)
		t[Band] = mix(bg, fg, liftBand)
		t[Overlay] = mix(bg, fg, liftOverlay)
		t[Shadow] = mix(bg, black, shadowDark)
	} else {
		t[Card] = mix(bg, white, lightCard)
		t[CardRaised] = mix(bg, white, lightCardRaised)
		t[Band] = mix(t[Card], black, darkenBandLight)
		t[Overlay] = white
		t[Shadow] = mix(bg, black, shadowLight)
	}
	if ea, ok := rgbOf(edgeActive); ok {
		t[Selection] = mix(t[Card], ea, mixSelection)
	} else {
		t[Selection] = mix(t[Card], fg, liftSelectionAlt)
	}
	t[SelectionDim] = mix(t[Card], fg, liftSelectionDim)
	return t
}

// rgbOf returns c's RGB when it is knowable: a hex colour, or an xterm-256
// index from 16 up. Indices below 16 are the terminal's own scheme, so their
// RGB is whatever the user configured.
func rgbOf(c lipgloss.TerminalColor) (colorful.Color, bool) {
	lc, ok := c.(lipgloss.Color)
	if !ok {
		return colorful.Color{}, false
	}
	s := string(lc)
	if strings.HasPrefix(s, "#") {
		col, err := colorful.Hex(s)
		return col, err == nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 16 || n > 255 {
		return colorful.Color{}, false
	}
	return termenv.ConvertToRGB(termenv.ANSI256Color(n)), true
}

func mix(a, b colorful.Color, f float64) colorful.Color {
	al, aa, ab := a.OkLab()
	bl, ba, bb := b.OkLab()
	return colorful.OkLab(al+(bl-al)*f, aa+(ba-aa)*f, ab+(bb-ab)*f).Clamped()
}

func lightness(c colorful.Color) float64 {
	l, _, _ := c.OkLab()
	return l
}

func withLightness(c colorful.Color, l float64) colorful.Color {
	_, a, b := c.OkLab()
	return colorful.OkLab(math.Max(0, math.Min(1, l)), a, b).Clamped()
}

func contrasting(bg colorful.Color) colorful.Color {
	if lightness(bg) < 0.6 {
		return colorful.Color{R: 1, G: 1, B: 1}
	}
	return colorful.Color{}
}

// keepApart moves d's lightness away from base until they differ by at least
// floor. It moves the way d already leans, or the way the palette lifts when
// they are equal, and flips if that runs out of range.
func keepApart(d, base colorful.Color, dark bool, floor float64) colorful.Color {
	ld, lb := lightness(d), lightness(base)
	if math.Abs(ld-lb) >= floor {
		return d
	}
	dir := 1.0
	switch {
	case ld < lb:
		dir = -1
	case ld == lb && !dark:
		dir = -1
	}
	target := lb + dir*floor
	if target < 0 || target > 1 {
		target = lb - dir*floor
	}
	return withLightness(d, target)
}

// apartQuantised re-checks a floor pair after rounding to xterm-256, where
// two tones a few percent apart often land on the same index. It walks d's
// lightness outward until it rounds to a distinct, visibly different step.
func apartQuantised(d colorful.Color, base lipgloss.TerminalColor, dark bool) lipgloss.TerminalColor {
	baseRGB, ok := rgbOf(base)
	if !ok {
		return quantiseRGB(d, termenv.ANSI256)
	}
	dir := 1.0
	if lightness(d) < lightness(baseRGB) || (lightness(d) == lightness(baseRGB) && !dark) {
		dir = -1
	}
	for _, sign := range []float64{dir, -dir} {
		for step := 0; step < 60; step++ {
			cand := withLightness(d, lightness(d)+sign*float64(step)*0.01)
			q := quantiseRGB(cand, termenv.ANSI256)
			qRGB, _ := rgbOf(q)
			if q != base && math.Abs(lightness(qRGB)-lightness(baseRGB)) >= floorQuantised {
				return q
			}
		}
	}
	return quantiseRGB(d, termenv.ANSI256)
}

// quantise rounds a palette colour to profile. Colours with no RGB (ANSI
// indices, adaptive colours) are the terminal's to render and pass through.
func quantise(c lipgloss.TerminalColor, p termenv.Profile) lipgloss.TerminalColor {
	if col, ok := rgbOf(c); ok {
		if lc, isColour := c.(lipgloss.Color); isColour && !strings.HasPrefix(string(lc), "#") && p != termenv.ANSI && p != termenv.Ascii {
			return c
		}
		return quantiseRGB(col, p)
	}
	return c
}

func quantiseRGB(c colorful.Color, p termenv.Profile) lipgloss.TerminalColor {
	hex := c.Clamped().Hex()
	switch p {
	case termenv.TrueColor:
		return lipgloss.Color(hex)
	case termenv.Ascii:
		return lipgloss.NoColor{}
	}
	if p == termenv.ANSI256 {
		return lipgloss.Color(strconv.Itoa(nearest256(c)))
	}
	switch v := p.Color(hex).(type) {
	case termenv.ANSIColor:
		return lipgloss.Color(strconv.Itoa(int(v)))
	default:
		return lipgloss.Color(fmt.Sprint(v))
	}
}

// xterm256 holds the OKLab coordinates of indices 16–255, whose RGB is fixed
// by the xterm cube and grey ramp (0–15 are the terminal's to choose).
var xterm256 = func() [240][3]float64 {
	var t [240][3]float64
	for i := range t {
		l, a, b := termenv.ConvertToRGB(termenv.ANSI256Color(i + 16)).OkLab()
		t[i] = [3]float64{l, a, b}
	}
	return t
}()

// nearest256 is the xterm-256 index perceptually closest to c. termenv's own
// rounding measures distance in RGB, which sends a dark grey to a saturated
// blue; OKLab keeps a grey grey.
func nearest256(c colorful.Color) int {
	l, a, b := c.OkLab()
	best, bestD := 16, math.Inf(1)
	for i, x := range xterm256 {
		d := (l-x[0])*(l-x[0]) + (a-x[1])*(a-x[1]) + (b-x[2])*(b-x[2])
		if d < bestD {
			best, bestD = i+16, d
		}
	}
	return best
}
