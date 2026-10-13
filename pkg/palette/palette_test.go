package palette

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/surface"
)

func TestBuiltinsHaveUniqueNames(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range All() {
		if p.Name == "" || seen[p.Name] {
			t.Errorf("palette name %q is empty or repeated", p.Name)
		}
		seen[p.Name] = true
	}
}

// Every built-in names its upstream background and foreground, so tones lift
// from the palette's real background rather than from its bar colour.
func TestBuiltinsSetBothSeeds(t *testing.T) {
	for _, p := range All() {
		if p.Background == nil || p.Foreground == nil {
			t.Errorf("%s: Background %v, Foreground %v", p.Name, p.Background, p.Foreground)
		}
	}
}

// Every built-in can be drawn on a filled surface at 256 colours and up, with
// each floor pair still distinct after rounding — the two profiles the
// fallback chain lets filled surfaces run in.
func TestBuiltinsDeriveDistinctTones(t *testing.T) {
	pairs := [][2]surface.Role{
		{surface.Card, surface.Screen},
		{surface.CardRaised, surface.Card},
		{surface.Band, surface.Card},
		{surface.Selection, surface.Card},
	}
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.ANSI256} {
		for _, p := range All() {
			c := p.Colours(profile)
			if !c.Tonal() {
				t.Errorf("%s (profile %d): no tones", p.Name, profile)
				continue
			}
			for _, pr := range pairs {
				if c.Of(pr[0]) == c.Of(pr[1]) {
					t.Errorf("%s (profile %d): %s and %s are both %v", p.Name, profile, pr[0], pr[1], c.Of(pr[0]))
				}
			}
		}
	}
}

func TestOverrideReplacesADerivedTone(t *testing.T) {
	p := Dark()
	p.Surface = map[surface.Role]lipgloss.TerminalColor{surface.Card: lipgloss.Color("#123456")}
	if got := p.Colours(termenv.TrueColor).Of(surface.Card); got != lipgloss.Color("#123456") {
		t.Errorf("Card = %v, want the override", got)
	}
}

func TestByName(t *testing.T) {
	if p, ok := ByName(All(), "nord"); !ok || p.Name != "nord" {
		t.Errorf("ByName(nord) = %q, %v", p.Name, ok)
	}
	if _, ok := ByName(All(), "nope"); ok {
		t.Error("ByName found a palette that doesn't exist")
	}
}
