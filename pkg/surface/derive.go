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
	liftCard         = 0.035
	liftCardRaised   = 0.065
	liftBand         = 0.10
	liftOverlay      = 0.17
	liftSelectionDim = 0.12
	minSpan          = 0.55 // the least background-to-text lightness span lifts are measured on
	liftSelectionAlt = 0.25 // Selection, when EdgeActive has no RGB
	mixSelection     = 0.45 // light palettes: a pale tint of the accent
	mixSelectionDark = 0.22 // dark palettes: a state layer, not a block
	lightCard        = 0.70
	lightCardRaised  = 0.90
	darkenBandLight  = 0.06
	lightScreenDrop  = 0.05 // how far a light palette's screen sits below its background
	shadowDark       = 0.60
	shadowLight      = 0.22

	// floorL is the least OKLab lightness distance a derived tone keeps from
	// the role beneath it.
	floorL = 0.022
	// floorQuantised is the same floor after rounding to the profile, where
	// only the palette's discrete steps are available.
	floorQuantised = 0.015
	// floorChrome is the least lightness distance faint chrome — dividers,
	// rules, separators, an unfocused edge — keeps from a focused card. The
	// palette chose those colours to sit on its own background; a card lifts
	// away from it, and on a dark palette the chrome would vanish into it.
	floorChrome = 0.12
	// floorText is the least lightness distance text keeps from what it is
	// drawn on. Far above the tone floor: two tones a few percent apart read
	// as two surfaces, but text needs real contrast to be read at all.
	floorText = 0.25
)

// Text contrast floors, as WCAG contrast ratios against the card tone that
// is worst for the colour — the lightest card for dark text, the focused
// (lightest) card for light text. On a dark palette every lift moves a card
// toward its text, so each elevation loses contrast; on a light palette the
// cards are near-white, which a pale accent or a soft grey sinks into. These
// floors keep text legible on any card in either.
const (
	contrastText   = 14.0 // body text
	contrastMuted  = 8.0  // secondary text
	contrastAccent = 7.0  // the accent: further would wash out its colour

	// Vibrance: how far text colours' chroma is boosted, in their own hue,
	// before the contrast floor. A card's tone sits between the palette's
	// background and text, which mutes everything drawn on it; the boost
	// gives the colour back. Accents the most, body text — near-white or
	// near-black — barely, since it carries little chroma to boost.
	vividText   = 1.15
	vividMuted  = 1.2
	vividAccent = 1.3
)

// Contrast is the WCAG contrast ratio between two colours, from 1 to 21. It
// is 0 when either has no known RGB.
func Contrast(a, b lipgloss.TerminalColor) float64 {
	ca, okA := rgbOf(a)
	cb, okB := rgbOf(b)
	if !okA || !okB {
		return 0
	}
	return contrastRGB(ca, cb)
}

func contrastRGB(a, b colorful.Color) float64 {
	lum := func(c colorful.Color) float64 {
		r, g, bl := c.LinearRgb()
		return 0.2126*r + 0.7152*g + 0.0722*bl
	}
	la, lb := lum(a), lum(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// EnsureContrast returns fg moved away from bg in lightness, keeping its hue,
// until their contrast ratio reaches at least ratio. When the hue runs out —
// a tinted colour can't get as bright as white — it gives up tint, blending
// toward white or black only as far as the ratio needs; past pure white or
// black there is nowhere left to go. Colours with no known RGB are returned
// unchanged.
func EnsureContrast(fg, bg lipgloss.TerminalColor, ratio float64) lipgloss.TerminalColor {
	f, okF := rgbOf(fg)
	b, okB := rgbOf(bg)
	if !okF || !okB {
		return fg
	}
	return lipgloss.Color(ensureContrast(f, b, ratio).Hex())
}

func ensureContrast(fg, bg colorful.Color, ratio float64) colorful.Color {
	if contrastRGB(fg, bg) >= ratio {
		return fg
	}
	dir := 1.0
	if lightness(fg) < lightness(bg) {
		dir = -1
	}
	for l := lightness(fg); l >= 0 && l <= 1; l += dir * 0.005 {
		if c := withLightness(fg, l); contrastRGB(c, bg) >= ratio {
			return c
		}
	}
	// The colour's own hue runs out before the floor: a tinted colour at full
	// lightness is outside the gamut, and clamping it back costs brightness.
	// Give up tint instead, blending toward white (or black) only as far as
	// the floor needs.
	end := colorful.Color{}
	if dir > 0 {
		end = colorful.Color{R: 1, G: 1, B: 1}
	}
	edge := withLightness(fg, math.Max(0, math.Min(1, (dir+1)/2)))
	for f := 0.05; f < 1; f += 0.05 {
		if c := mix(edge, end, f); contrastRGB(c, bg) >= ratio {
			return c
		}
	}
	return end
}

// worstCard is the card tone text in fg is hardest to read on: the one
// closest to it in contrast, among the body and its focused form.
func worstCard(fg colorful.Color, tones map[Role]colorful.Color) colorful.Color {
	worst := tones[Card]
	if contrastRGB(fg, tones[CardRaised]) < contrastRGB(fg, worst) {
		worst = tones[CardRaised]
	}
	return worst
}

// Legible reports whether text in fg reads on bg: their OKLab lightness
// differs by at least the text floor. Colours with no known RGB (ANSI
// indices) are the terminal's to render and count as legible.
func Legible(fg, bg lipgloss.TerminalColor) bool {
	f, okF := rgbOf(fg)
	b, okB := rgbOf(bg)
	if !okF || !okB {
		return true
	}
	return math.Abs(lightness(f)-lightness(b)) >= floorText
}

// floorPair is a derived tone that must stay distinct from the tone it sits
// on, by at least floorL — or by darkFloor on a dark palette, when set.
type floorPair struct {
	derived, base Role
	darkFloor     float64
	// elevation marks a tone that sits above its base on a dark palette —
	// a card above the screen, a focused card above a card. The floor only
	// ever pushes it up: pushed back down, a focused card can land on the
	// very screen it is meant to rise from.
	elevation bool
}

// floorPairs are the tones kept apart. A modal sits over a focused card, so
// the overlay keeps a wider gap than the tone floor: one step lighter than a
// focused card reads as the same surface. A light palette's overlay is
// already white, which the regular floor keeps clear of the card.
var floorPairs = []floorPair{
	{derived: Card, base: Screen, elevation: true},
	{derived: CardRaised, base: Card, elevation: true},
	{derived: Band, base: Card, elevation: true},
	{derived: Selection, base: Card},
	{derived: Overlay, base: CardRaised, darkFloor: 0.05, elevation: true},
}

// floor is the distance p keeps on a palette that is dark or not.
func (p floorPair) floor(dark bool) float64 {
	if dark && p.darkFloor > 0 {
		return p.darkFloor
	}
	return floorL
}

// Derive resolves every role for a palette's seeds. Surface tones are the
// Background raised in OKLab lightness toward the Foreground's, in the
// background's own hue (on a light palette, cards lighten toward white
// instead), kept a minimum lightness apart from the tone beneath
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
		if fixed[p.derived] {
			continue
		}
		if dark && p.elevation {
			tones[p.derived] = raiseAbove(tones[p.derived], tones[p.base], p.floor(dark))
			continue
		}
		tones[p.derived] = keepApart(tones[p.derived], tones[p.base], dark, p.floor(dark))
	}
	// Faint chrome is drawn on cards, so it is kept clear of the brightest
	// of them — a focused card — rather than of the screen it was chosen for.
	for r, from := range map[Role]lipgloss.TerminalColor{Divider: s.Subtle, Edge: s.Edge} {
		if fixed[r] {
			continue
		}
		if col, ok := rgbOf(from); ok {
			tones[r] = keepApart(col, tones[CardRaised], dark, floorChrome)
		}
	}
	for r, f := range map[Role]struct {
		from  lipgloss.TerminalColor
		ratio float64
		vivid float64
	}{
		Text:      {fg, contrastText, vividText},
		TextMuted: {s.Muted, contrastMuted, vividMuted},
		Accent:    {s.Accent, contrastAccent, vividAccent},
	} {
		if fixed[r] {
			continue
		}
		if col, ok := rgbOf(f.from); ok {
			col = vivid(col, f.vivid)
			tones[r] = ensureContrast(col, worstCard(col, tones), f.ratio)
		}
	}
	for r, t := range tones {
		if !fixed[r] {
			c.of[r] = quantiseRGB(t, profile)
		}
	}
	if profile == termenv.ANSI256 {
		for _, p := range floorPairs {
			if fixed[p.derived] {
				continue
			}
			c.of[p.derived] = apartQuantised(tones[p.derived], c.of[p.base], dark, dark && p.elevation)
		}
	}
	return c
}

// deriveTones mixes the surface tones from the two seeds.
func deriveTones(bg, fg colorful.Color, edgeActive lipgloss.TerminalColor) map[Role]colorful.Color {
	white, black := colorful.Color{R: 1, G: 1, B: 1}, colorful.Color{}
	t := map[Role]colorful.Color{Screen: bg}
	if lightness(bg) < lightness(fg) {
		// Tonal elevation: each surface is the background raised in
		// lightness, in the background's own hue. Mixing toward the text
		// colour instead drags its tint in — Gruvbox's neutral screen grew
		// beige cards, and Everforest's tones each landed on a different hue.
		// Lifts are fractions of the span from background to text, at least
		// minSpan: a deliberately low-contrast palette (Solarized's text is a
		// mid-grey) would otherwise get cards that barely rise.
		span := math.Max(lightness(fg)-lightness(bg), minSpan)
		up := func(f float64) colorful.Color { return withLightness(bg, lightness(bg)+span*f) }
		t[Card] = up(liftCard)
		t[CardRaised] = up(liftCardRaised)
		t[Band] = up(liftBand)
		t[Overlay] = up(liftOverlay)
		t[Shadow] = mix(bg, black, shadowDark)
	} else {
		// The screen sits a step below the palette's background, in its hue,
		// so near-white cards stand off it: a light palette's background is
		// itself nearly white, and cards lifted toward white from it barely
		// separate.
		t[Screen] = withLightness(bg, lightness(bg)-lightScreenDrop)
		t[Card] = mix(bg, white, lightCard)
		t[CardRaised] = mix(bg, white, lightCardRaised)
		t[Band] = mix(t[Card], black, darkenBandLight)
		t[Overlay] = white
		t[Shadow] = mix(t[Screen], black, shadowLight)
	}
	if ea, ok := rgbOf(edgeActive); ok {
		f := mixSelection
		if lightness(bg) < lightness(fg) {
			f = mixSelectionDark
		}
		t[Selection] = mix(t[Card], ea, f)
	} else {
		t[Selection] = lift(t[Card], fg, liftSelectionAlt)
	}
	t[SelectionDim] = lift(t[Card], fg, liftSelectionDim)
	return t
}

// vivid scales c's chroma by k in its own hue, as far as the RGB gamut
// allows: past it, a clamp would shift the hue, so the boost backs off
// instead.
func vivid(c colorful.Color, k float64) colorful.Color {
	l, a, b := c.OkLab()
	for f := k; f > 1; f -= 0.02 {
		if v := colorful.OkLab(l, a*f, b*f); v.IsValid() {
			return v
		}
	}
	return c
}

// lift moves c's lightness the fraction f of the way to toward's, keeping
// c's own hue and chroma.
func lift(c, toward colorful.Color, f float64) colorful.Color {
	return withLightness(c, lightness(c)+(lightness(toward)-lightness(c))*f)
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

// raiseAbove lifts d until it is at least floor lighter than base, keeping
// its hue. Unlike keepApart it never moves d down, so an elevated tone stays
// above what it rises from even when the floor below it pushed that up.
func raiseAbove(d, base colorful.Color, floor float64) colorful.Color {
	want := lightness(base) + floor
	if lightness(d) >= want {
		return d
	}
	return withLightness(d, math.Min(1, want+0.004))
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
	// Aim a little past the floor: rounding to an 8-bit colour afterwards
	// can lose a few thousandths, which would land a tone just inside it.
	reach := floor + 0.004
	target := lb + dir*reach
	switch {
	case target >= 0 && target <= 1:
	case math.Abs(math.Round(target)-lb) >= floor:
		// Past white or black, but the end of the range still clears the
		// floor: stop there rather than flip to the other side.
		target = math.Round(target)
	default:
		target = lb - dir*reach
	}
	return withLightness(d, target)
}

// apartQuantised re-checks a floor pair after rounding to xterm-256, where
// two tones a few percent apart often land on the same index. It walks d's
// lightness outward until it rounds to a distinct, visibly different step.
//
// An elevated tone (up) searches upward only, and must land above base, so
// rounding can't sink a focused card below the card it rises from.
func apartQuantised(d colorful.Color, base lipgloss.TerminalColor, dark, up bool) lipgloss.TerminalColor {
	baseRGB, ok := rgbOf(base)
	if !ok {
		return quantiseRGB(d, termenv.ANSI256)
	}
	dir := 1.0
	if lightness(d) < lightness(baseRGB) || (lightness(d) == lightness(baseRGB) && !dark) {
		dir = -1
	}
	signs := []float64{dir, -dir}
	if up {
		signs = []float64{1}
	}
	for _, sign := range signs {
		for step := 0; step < 60; step++ {
			cand := withLightness(d, lightness(d)+sign*float64(step)*0.01)
			q := quantiseRGB(cand, termenv.ANSI256)
			qRGB, _ := rgbOf(q)
			gap := lightness(qRGB) - lightness(baseRGB)
			if !up {
				gap = math.Abs(gap)
			}
			if q != base && gap >= floorQuantised {
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
