package surface

import (
	"math"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func darkSeeds() Seeds {
	return Seeds{
		Background: lipgloss.Color("#1e1e2e"), Foreground: lipgloss.Color("#cdd6f4"),
		Bar: lipgloss.Color("#313244"), BarText: lipgloss.Color("#cdd6f4"),
		Muted: lipgloss.Color("#a6adc8"), Subtle: lipgloss.Color("#6c7086"),
		Edge: lipgloss.Color("#585b70"), EdgeActive: lipgloss.Color("#cba6f7"),
		Accent: lipgloss.Color("#f5c2e7"),
	}
}

func lightSeeds() Seeds {
	s := darkSeeds()
	s.Background, s.Foreground = lipgloss.Color("#faf4ed"), lipgloss.Color("#575279")
	s.Bar, s.BarText = lipgloss.Color("#fffaf3"), lipgloss.Color("#575279")
	return s
}

func L(t *testing.T, c lipgloss.TerminalColor) float64 {
	t.Helper()
	col, ok := rgbOf(c)
	if !ok {
		t.Fatalf("colour %v has no RGB", c)
	}
	return lightness(col)
}

func TestDeriveDarkLiftsCardsOffTheScreen(t *testing.T) {
	c := Derive(darkSeeds(), nil, termenv.TrueColor)
	if !c.Tonal() {
		t.Fatal("hex seeds should derive tones")
	}
	screen, card, raised := L(t, c.Of(Screen)), L(t, c.Of(Card)), L(t, c.Of(CardRaised))
	if !(screen < card && card < raised) {
		t.Errorf("want screen < card < raised, got %.3f %.3f %.3f", screen, card, raised)
	}
	if shadow := L(t, c.Of(Shadow)); shadow >= screen {
		t.Errorf("shadow %.3f should be darker than the screen %.3f", shadow, screen)
	}
	if c.Of(Screen) != lipgloss.Color("#1e1e2e") {
		t.Errorf("Screen = %v, want the Background seed", c.Of(Screen))
	}
}

func TestDeriveLightCardsSitBrighterThanTheScreen(t *testing.T) {
	c := Derive(lightSeeds(), nil, termenv.TrueColor)
	if L(t, c.Of(Card)) <= L(t, c.Of(Screen)) {
		t.Errorf("light card %v should be brighter than screen %v", c.Of(Card), c.Of(Screen))
	}
}

func TestDeriveKeepsTheContrastFloor(t *testing.T) {
	for name, seeds := range map[string]Seeds{"dark": darkSeeds(), "light": lightSeeds()} {
		c := Derive(seeds, nil, termenv.TrueColor)
		floor := func(p floorPair) float64 { return p.floor(name == "dark") }
		for _, p := range floorPairs {
			if d := math.Abs(L(t, c.Of(p.derived)) - L(t, c.Of(p.base))); d < floor(p)-1e-3 {
				t.Errorf("%s: %s and %s are %.3f apart, floor is %.3f", name, p.derived, p.base, d, floor(p))
			}
		}
	}
}

// A near-white palette was the prototype's failure: cards lifted toward white
// from a background already almost white came out invisible.
func TestDeriveFloorRescuesANearWhiteBackground(t *testing.T) {
	s := lightSeeds()
	s.Background = lipgloss.Color("#fffefd")
	c := Derive(s, nil, termenv.TrueColor)
	if d := math.Abs(L(t, c.Of(Card)) - L(t, c.Of(Screen))); d < floorL-1e-3 {
		t.Errorf("card and screen %.3f apart, want at least %.3f", d, floorL)
	}
}

func TestDeriveFallsBackToBarWithoutBackground(t *testing.T) {
	s := darkSeeds()
	s.Background, s.Foreground = nil, nil
	c := Derive(s, nil, termenv.TrueColor)
	if c.Of(Screen) != lipgloss.Color("#313244") {
		t.Errorf("Screen = %v, want Bar", c.Of(Screen))
	}
	// Text comes from BarText, brightened as far as the contrast floor needs.
	with := darkSeeds()
	with.Background, with.Foreground = with.Bar, with.BarText
	if want := Derive(with, nil, termenv.TrueColor).Of(Text); c.Of(Text) != want {
		t.Errorf("Text = %v, want %v (derived from BarText)", c.Of(Text), want)
	}
}

func TestDeriveWithoutRGBBackgroundHasNoTones(t *testing.T) {
	s := darkSeeds()
	s.Background = lipgloss.Color("0")
	c := Derive(s, nil, termenv.TrueColor)
	if c.Tonal() {
		t.Error("an ANSI-index background can't derive tones")
	}
	if c.Of(Card) != nil {
		t.Errorf("Card = %v, want unset", c.Of(Card))
	}
	if c.Of(EdgeActive) == nil || c.Of(Accent) == nil {
		t.Error("pass-through roles should still be set")
	}
}

func TestDeriveSelectionUsesSeedsWhenEdgeActiveIsAnIndex(t *testing.T) {
	s := darkSeeds()
	s.EdgeActive = lipgloss.Color("12")
	c := Derive(s, nil, termenv.TrueColor)
	if c.Of(Selection) == nil {
		t.Fatal("Selection should come from the seeds-only recipe")
	}
	if c.Of(EdgeActive) != lipgloss.Color("12") {
		t.Errorf("an ANSI index used directly should pass through, got %v", c.Of(EdgeActive))
	}
	if L(t, c.Of(Selection)) <= L(t, c.Of(Card)) {
		t.Error("the seeds-only selection should lift off the card")
	}
}

func TestDeriveOverrideWinsAndIsNotPushed(t *testing.T) {
	same := lipgloss.Color("#1e1e2e") // identical to the screen: no contrast at all
	c := Derive(darkSeeds(), map[Role]lipgloss.TerminalColor{Card: same}, termenv.TrueColor)
	if c.Of(Card) != same {
		t.Errorf("Card = %v, want the override %v untouched", c.Of(Card), same)
	}
}

// In xterm-256 two tones a few percent apart often round to the same index.
func TestDeriveQuantisedTonesStayDistinct(t *testing.T) {
	nord := darkSeeds()
	nord.Background, nord.Foreground = lipgloss.Color("#2e3440"), lipgloss.Color("#d8dee9")
	for name, seeds := range map[string]Seeds{"catppuccin": darkSeeds(), "nord": nord, "light": lightSeeds()} {
		c := Derive(seeds, nil, termenv.ANSI256)
		for _, p := range floorPairs {
			if c.Of(p.derived) == c.Of(p.base) {
				t.Errorf("%s: %s and %s both round to %v", name, p.derived, p.base, c.Of(p.derived))
			}
		}
	}
}

func TestDeriveRoundsToTheProfile(t *testing.T) {
	c := Derive(darkSeeds(), nil, termenv.ANSI256)
	for _, r := range []Role{Screen, Card, Shadow, Accent, EdgeActive} {
		s := string(c.Of(r).(lipgloss.Color))
		if s == "" || s[0] == '#' {
			t.Errorf("%s = %q, want an xterm-256 index", r, s)
		}
	}
}

func resolved(s Surface, caps Caps) Resolved {
	return Resolve(s, Derive(darkSeeds(), nil, caps.Profile), caps)
}

var rich = Caps{Profile: termenv.TrueColor, BlockGlyphs: true}

func TestResolveFollowsTheFallbackChain(t *testing.T) {
	for _, tc := range []struct {
		asked    Surface
		caps     Caps
		want     string
		fellBack bool
	}{
		{Filled, rich, "filled", false},
		{Elevated, rich, "elevated", false},
		{Accented, rich, "accented", false},
		{Filled, Caps{Profile: termenv.TrueColor}, "banded", true},
		{Elevated, Caps{Profile: termenv.ANSI256}, "banded", true},
		{Filled, Caps{Profile: termenv.ANSI, BlockGlyphs: true}, "outlined", true},
		{Banded, Caps{Profile: termenv.ANSI}, "outlined", true},
		{Accented, Caps{Profile: termenv.ANSI}, "accented", true}, // the ┃ variant
		{Accented, Caps{Profile: termenv.Ascii}, "outlined", true},
		{Outlined, Caps{Profile: termenv.Ascii}, "outlined", false},
	} {
		r := resolved(tc.asked, tc.caps)
		if r.Name() != tc.want {
			t.Errorf("%s under %+v resolved to %s, want %s", tc.asked.Name, tc.caps, r.Name(), tc.want)
		}
		if _, _, fb := r.FellBack(); fb != tc.fellBack && tc.asked.Name != tc.want {
			t.Errorf("%s under %+v: FellBack = %v", tc.asked.Name, tc.caps, fb)
		}
	}
}

func TestResolveSaysWhy(t *testing.T) {
	asked, why, ok := resolved(Elevated, Caps{Profile: termenv.TrueColor}).FellBack()
	if !ok || asked != "elevated" || why != "block glyphs off" {
		t.Errorf("FellBack = %q, %q, %v", asked, why, ok)
	}
}

func TestResolveAccentedWithoutBlocksDrawsALine(t *testing.T) {
	r := resolved(Accented, Caps{Profile: termenv.TrueColor})
	if g := r.Surface().Rings[1].Left.Glyph; g != "┃" {
		t.Errorf("bar glyph = %q, want ┃", g)
	}
}

func TestResolveFilledWithoutTonesFallsBackToOutlined(t *testing.T) {
	s := darkSeeds()
	s.Background = lipgloss.Color("0")
	r := Resolve(Elevated, Derive(s, nil, termenv.TrueColor), rich)
	if r.Name() != "outlined" {
		t.Errorf("resolved to %s, want outlined", r.Name())
	}
	if _, why, _ := r.FellBack(); why != "no tones for this palette" {
		t.Errorf("why = %q", why)
	}
}

func TestZeroResolvedIsOutlined(t *testing.T) {
	var r Resolved
	if r.Name() != "outlined" {
		t.Errorf("zero Resolved draws %s, want outlined", r.Name())
	}
	if r.Colour(Card, false) != nil {
		t.Error("zero Resolved has no colours")
	}
}

func TestInsets(t *testing.T) {
	for _, tc := range []struct {
		s    Surface
		want Insets
	}{
		{Outlined, Insets{1, 1, 1, 1}},
		{Filled, Insets{Top: 2, Bottom: 2, Left: 1, Right: 2}},
		{Banded, Insets{Top: 1, Bottom: 2, Left: 1, Right: 2}},
		{Elevated, Insets{Top: 1, Bottom: 2, Left: 1, Right: 2}},
		{Accented, Insets{Top: 1, Bottom: 2, Left: 1, Right: 2}},
	} {
		if got := tc.s.Insets(); got != tc.want {
			t.Errorf("%s insets = %+v, want %+v", tc.s.Name, got, tc.want)
		}
	}
}

// Every surface has a text row on top and on the bottom, so all five slots
// exist whatever surface is drawn.
func TestEverySurfaceHasBothTextRows(t *testing.T) {
	for _, s := range append(Builtins(), accentedLine) {
		var top, bottom bool
		for _, r := range s.Rings {
			top = top || r.Top.Text
			bottom = bottom || r.Bottom.Text
		}
		if !top || !bottom {
			t.Errorf("%s: top text row %v, bottom %v", s.Name, top, bottom)
		}
	}
}

func TestFillResolvesToTheFocusState(t *testing.T) {
	r := resolved(Filled, rich)
	c := r.Colours()
	if r.Colour(Fill, false) != c.Of(Card) || r.Colour(Fill, true) != c.Of(CardRaised) {
		t.Errorf("Fill = %v / %v, want Card / CardRaised", r.Colour(Fill, false), r.Colour(Fill, true))
	}
	if r.Colour(Parent, true) != nil {
		t.Error("Parent should emit no colour")
	}
}

func TestOverlayForms(t *testing.T) {
	if g := resolved(Outlined, rich).Overlay().Surface().Rings[0].TopLeft.Glyph; g != "┏" {
		t.Errorf("Outlined's overlay corner = %q, want the thick ┏", g)
	}
	o := resolved(Elevated, rich).Overlay().Surface()
	if o.Fill != Overlay || o.FillFocused != Overlay {
		t.Errorf("Elevated's overlay fill = %s/%s, want overlay", o.Fill, o.FillFocused)
	}
	if o.Insets() != Elevated.Insets() {
		t.Error("an overlay form keeps its surface's shape")
	}
}

// A field fits a three-row slot: at most one cell of edge on each side.
func TestFieldFormsFitThreeRows(t *testing.T) {
	for _, s := range append(Builtins(), accentedLine) {
		f := resolved(s, rich).Field()
		in := f.Insets()
		if in.Top > 1 || in.Bottom > 1 || in.Left > 1 || in.Right > 1 {
			t.Errorf("%s field insets %+v exceed one cell", s.Name, in)
		}
	}
	if resolved(Outlined, rich).Field().Name() != "outlined" {
		t.Error("Outlined should be its own field")
	}
}

// A modal covers what is under it, so an unfilled surface's derived overlay
// form fills its body; otherwise every blank cell shows the panes behind.
func TestUnfilledSurfaceOverlayIsFilled(t *testing.T) {
	o := resolved(Accented, rich).Overlay().Surface()
	if !o.Filled() || o.Fill != Overlay || o.Text != Text {
		t.Errorf("Accented overlay: fill %s/%s text %s, want overlay with text", o.Fill, o.FillFocused, o.Text)
	}
	if bg := o.Rings[1].Top.BG; bg != Fill {
		t.Errorf("Accented overlay's title row sits on %s, want the fill", bg)
	}
	if bg := o.Rings[0].Right.BG; bg != Parent {
		t.Errorf("Accented overlay's margin moved onto %s; it should stay on the parent", bg)
	}
	if Accented.Rings[1].Top.BG != Parent {
		t.Error("deriving the overlay changed Accented itself")
	}
}

func TestLegible(t *testing.T) {
	if Legible(lipgloss.Color("#3b4252"), lipgloss.Color("#2e3440")) {
		t.Error("two near-identical darks shouldn't count as legible")
	}
	if !Legible(lipgloss.Color("#d8dee9"), lipgloss.Color("#2e3440")) {
		t.Error("light text on a dark background should count as legible")
	}
	if !Legible(lipgloss.Color("4"), lipgloss.Color("#2e3440")) {
		t.Error("an ANSI index is the terminal's to render and counts as legible")
	}
}

func TestTopGap(t *testing.T) {
	for _, tc := range []struct {
		s    Surface
		want bool
	}{
		{Outlined, false}, // its border line is the gap
		{Filled, false},   // its ▄ lip is the gap
		{Banded, true},
		{Elevated, true},
		{Accented, true},
	} {
		if got := tc.s.TopGap(); got != tc.want {
			t.Errorf("%s TopGap = %v, want %v", tc.s.Name, got, tc.want)
		}
	}
}

// A modal opens over a focused card, so on a dark palette its tone keeps a
// wider gap from CardRaised than one tone keeps from another; one step
// lighter than a focused card reads as the same surface.
func TestOverlayStandsOutFromAFocusedCard(t *testing.T) {
	c := Derive(darkSeeds(), nil, termenv.TrueColor)
	if d := L(t, c.Of(Overlay)) - L(t, c.Of(CardRaised)); d < 0.05-1e-3 {
		t.Errorf("overlay is %.3f lighter than a focused card, want at least 0.05", d)
	}
	l := Derive(lightSeeds(), nil, termenv.TrueColor)
	if L(t, l.Of(Overlay)) <= L(t, l.Of(CardRaised)) {
		t.Error("a light palette's overlay should stay lighter than a focused card, not be pushed below it")
	}
}

// Faint chrome is drawn on cards, so it stays visible against the brightest
// of them; on a dark palette a divider chosen for the screen would otherwise
// sit a hair from a focused card.
func TestChromeStaysVisibleOnAFocusedCard(t *testing.T) {
	s := darkSeeds()
	s.Subtle, s.Edge = s.Background, s.Background // as faint as chrome gets
	c := Derive(s, nil, termenv.TrueColor)
	for _, r := range []Role{Divider, Edge} {
		if d := math.Abs(L(t, c.Of(r)) - L(t, c.Of(CardRaised))); d < floorChrome-1e-3 {
			t.Errorf("%s sits %.3f from a focused card, want at least %.2f", r, d, floorChrome)
		}
	}
}

// Text on a card keeps its contrast floor in dark and light palettes: on a
// dark one every lift takes contrast away, and on a light one a pale accent
// sinks into near-white cards.
func TestTextKeepsItsContrastOnCards(t *testing.T) {
	for name, seeds := range map[string]Seeds{"dark": darkSeeds(), "light": lightSeeds()} {
		pale := seeds
		pale.Accent, pale.Muted = lipgloss.Color("#f0c0e0"), lipgloss.Color("#c8c8d0") // faint on white
		if name == "dark" {
			pale.Accent, pale.Muted = lipgloss.Color("#505070"), lipgloss.Color("#45475a") // faint on a dark card
		}
		c := Derive(pale, nil, termenv.TrueColor)
		for _, tc := range []struct {
			r     Role
			floor float64
		}{{Text, contrastText}, {TextMuted, contrastMuted}, {Accent, contrastAccent}} {
			for _, card := range []Role{Card, CardRaised} {
				// Short of the floor is allowed only at the very end of the
				// range: pure white or pure black.
				got := Contrast(c.Of(tc.r), c.Of(card))
				if l := L(t, c.Of(tc.r)); got < tc.floor-0.05 && l < 0.999 && l > 0.001 {
					t.Errorf("%s: %s on %s is %.1f:1, want at least %.1f:1", name, tc.r, card, got, tc.floor)
				}
			}
		}
	}
}
