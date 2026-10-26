package look

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/theme"
)

var caps = surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}

func render(s *Screen) string {
	return ansi.Strip(s.Layout().Render(geom.New(0, 0, 110, 30)))
}

// The toggle opens the overlay preview; while it is up it holds the keys,
// and its own answer closes it and puts the toggle back.
func TestToggleOpensAndClosesTheOverlay(t *testing.T) {
	s := New(theme.New(palette.Nord())).(*Screen)
	if strings.Contains(render(s), "an overlay") {
		t.Fatal("the overlay should start closed")
	}
	// Focus the toggle directly: tab can't cycle off the input on the way,
	// since a focused text field holds the keys.
	s.focus.SetIndex(6)
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if !s.modalUp || !strings.Contains(render(s), "an overlay") {
		t.Fatal("y on the toggle should open the overlay")
	}
	if !s.IsCapturingKeys() {
		t.Error("the screen should hold the keys while the overlay is up")
	}

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc on the overlay produced no answer")
	}
	s.Update(cmd())
	if s.modalUp || s.choice.Value() {
		t.Errorf("after esc: overlay up %v, toggle %v; want both off", s.modalUp, s.choice.Value())
	}
}

// Moving through the surfaces rebuilds the screen on every step; the picker
// must stay scrolled to the top, its cursor where the user put it.
func TestPickerSurvivesRebuilds(t *testing.T) {
	s := New(theme.New(palette.Nord())).(*Screen)
	s.focus.SetIndex(1) // the surface picker; the palette picker comes first
	render(s)
	for i := 0; i < 3; i++ {
		s.Update(tea.KeyMsg{Type: tea.KeyDown})
		s.SetTheme(theme.Compose(palette.Nord(), Surfaces()[i+1], caps))
		render(s)
	}
	view := render(s)
	// The rows above the cursor are what a scrolled picker loses.
	if !strings.Contains(view, "  filled") || !strings.Contains(view, "  banded") || !strings.Contains(view, "▸ elevated") {
		t.Errorf("picker scrolled or lost its cursor:\n%s", view)
	}
}

// The palette picker is the other axis: a cursor move asks the shell for that
// palette, and enter opens its fields.
func TestPalettePickerSwitchesAndOpens(t *testing.T) {
	s := New(theme.New(palette.Nord())).(*Screen)
	render(s)
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd == nil {
		t.Fatal("moving the palette cursor asked the shell for nothing")
	}
	_, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on a palette opened nothing")
	}
	if _, ok := cmd().(screen.PushMsg); !ok {
		t.Error("enter on a palette did not push its fields")
	}
}
