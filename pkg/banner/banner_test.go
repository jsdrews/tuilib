package banner

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/internal/cmdtest"
	"github.com/jsdrews/tuilib/internal/tick"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

var colours = []lipgloss.TerminalColor{lipgloss.Color("#ff0000"), lipgloss.Color("#00ff00"), lipgloss.Color("214")}

// A glyph whose rows differ in width shears every letter after it.
func TestEveryGlyphIsRectangular(t *testing.T) {
	for r, g := range font {
		w := len([]rune(g[0]))
		for y, row := range g {
			if n := len([]rune(row)); n != w {
				t.Errorf("%q row %d is %d wide, want %d", r, y, n, w)
			}
		}
	}
}

func TestDrawsSixRowsAtItsOwnWidth(t *testing.T) {
	b := New(Options{Text: "tui", Colours: colours})
	lines := strings.Split(b.View(), "\n")
	if len(lines) != b.Height() || b.Height() != 6 {
		t.Fatalf("%d lines, Height %d, want 6", len(lines), b.Height())
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != b.Width() {
			t.Errorf("line %d is %d wide, want %d", i, got, b.Width())
		}
	}
	if !strings.Contains(ansi.Strip(lines[0]), "████████╗") {
		t.Errorf("lower case should draw as upper case:\n%s", ansi.Strip(b.View()))
	}
}

func TestUnknownRunesDrawAsSpace(t *testing.T) {
	if a, b := New(Options{Text: "a€b"}), New(Options{Text: "a b"}); a.View() != b.View() {
		t.Error("an unknown rune should draw as a space")
	}
}

func TestEmptyTextDrawsNothing(t *testing.T) {
	b := New(Options{Colours: colours})
	if b.View() != "" || b.Width() != 0 || b.Height() != 0 || b.Fits(80) || b.Init() != nil {
		t.Error("an empty banner should draw nothing and ask for no ticks")
	}
}

func TestFits(t *testing.T) {
	b := New(Options{Text: "tui"})
	if !b.Fits(b.Width()) || b.Fits(b.Width()-1) {
		t.Errorf("Fits is wrong at width %d", b.Width())
	}
}

// The shadow strokes keep their colour while the faces sweep.
func TestShadowIsStaticAndFacesMove(t *testing.T) {
	tick.Instant(t)
	b := New(Options{Text: "t", Colours: colours, Shadow: lipgloss.Color("#123456")})
	if !strings.Contains(b.View(), "38;2;18;52;86m╗") {
		t.Errorf("shadow not drawn in its colour: %q", b.View())
	}
	before := b.View()
	for range 10 {
		msgs := cmdtest.Run(b.Init())
		if len(msgs) != 1 {
			t.Fatalf("Init ran to %d messages, want one tick", len(msgs))
		}
		b, _ = b.Update(msgs[0])
	}
	if b.View() == before {
		t.Error("ten ticks left the gradient where it was")
	}
}

func TestStaticAndFlatAskForNoTicks(t *testing.T) {
	for name, o := range map[string]Options{
		"static": {Text: "t", Colours: colours, Static: true},
		"flat":   {Text: "t", Colours: colours[:1]},
		"bare":   {Text: "t"},
	} {
		if New(o).Init() != nil {
			t.Errorf("%s banner asked for a tick", name)
		}
	}
}

// A tick delivered twice advances once, and one banner's ticks don't move
// another — the poll chain's doubling, guarded here the same way.
func TestTicksAreOwnedAndSerial(t *testing.T) {
	tick.Instant(t)
	a := New(Options{Text: "t", Colours: colours})
	other := New(Options{Text: "t", Colours: colours})
	msg := cmdtest.Run(a.Init())[0]

	b, cmd := a.Update(msg)
	if cmd == nil {
		t.Fatal("an accepted tick should arm the next")
	}
	if _, again := b.Update(msg); again != nil {
		t.Error("a duplicate tick armed a second chain")
	}
	if _, cross := other.Update(msg); cross != nil {
		t.Error("a tick moved a banner that did not ask for it")
	}
}
