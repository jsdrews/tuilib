package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
)

var caps = surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}

// Bar text chosen to read on the palette's bar colour can sink into the
// screen colour once a surface moves the bars there; it then takes the
// surface's Text colour.
func TestComposeKeepsBarTextLegibleOnTheScreen(t *testing.T) {
	p := palette.Nord()
	p.BarFG = lipgloss.Color("#3b4252") // fine on a pale bar, nearly the screen colour
	th := Compose(p, surface.Elevated, caps)
	if th.BarBG != th.Surface.Colour(surface.Screen, false) {
		t.Fatalf("Elevated's bars should sit on the screen colour, got %v", th.BarBG)
	}
	if want := th.Surface.Colour(surface.Text, false); th.BarFG != want {
		t.Errorf("BarFG = %v, want the surface's Text %v", th.BarFG, want)
	}
}

func TestComposeLeavesLegibleBarTextAlone(t *testing.T) {
	p := palette.Nord()
	if th := Compose(p, surface.Elevated, caps); th.BarFG != p.BarFG {
		t.Errorf("BarFG = %v, want the palette's own %v", th.BarFG, p.BarFG)
	}
	p.BarFG = lipgloss.Color("#3b4252")
	if th := Compose(p, surface.Outlined, caps); th.BarFG != p.BarFG {
		t.Errorf("Outlined keeps its bar, so BarFG should stay %v, got %v", p.BarFG, th.BarFG)
	}
}

func TestComposeKeepsAPalettesWarning(t *testing.T) {
	p := palette.Nord()
	if th := Compose(p, surface.Outlined, caps); th.Warning != p.Warning {
		t.Errorf("Warning = %v, want the palette's own %v", th.Warning, p.Warning)
	}
}

// A palette written before Warning existed still gets one: its error colour.
func TestComposeDerivesAnUnsetWarningFromErrorBG(t *testing.T) {
	p := palette.Nord()
	p.Warning = nil
	if th := Compose(p, surface.Outlined, caps); th.Warning != p.ErrorBG {
		t.Errorf("Warning = %v, want ErrorBG %v", th.Warning, p.ErrorBG)
	}
}

func TestComposeKeepsAPalettesSuccess(t *testing.T) {
	p := palette.Nord()
	if th := Compose(p, surface.Outlined, caps); th.Success != p.Success {
		t.Errorf("Success = %v, want the palette's own %v", th.Success, p.Success)
	}
}

// A palette written before Success existed still gets one: its info colour,
// the band a successful operation is reported in.
func TestComposeDerivesAnUnsetSuccessFromInfoBG(t *testing.T) {
	p := palette.Nord()
	p.Success = nil
	if th := Compose(p, surface.Outlined, caps); th.Success != p.InfoBG {
		t.Errorf("Success = %v, want InfoBG %v", th.Success, p.InfoBG)
	}
}
