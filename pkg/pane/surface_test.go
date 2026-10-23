package pane

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
)

// trueColor forces a colour profile for the test, without which lipgloss
// strips every style and a background check passes whatever the code does.
func trueColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func resolve(s surface.Surface) surface.Resolved {
	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	return surface.Resolve(s, palette.CatppuccinMocha().Colours(caps.Profile), caps)
}

const sw, sh = 30, 10

func surfacePane(s surface.Surface, focused bool) Pane {
	p := New(Options{Surface: resolve(s), Focused: focused})
	p.SetRect(geom.New(2, 3, sw, sh))
	p.SetTitle("Title")
	p.SetBottomLeft("3/9")
	p.SetContent("alpha\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("beta") + "\ngamma")
	return p
}

func filledSurfaces() []surface.Surface {
	return []surface.Surface{surface.Filled, surface.Banded, surface.Elevated}
}

func TestSurfaceFrameIsTheRectSize(t *testing.T) {
	trueColor(t)
	for _, s := range append(filledSurfaces(), surface.Accented) {
		for _, focused := range []bool{false, true} {
			lines := strings.Split(surfacePane(s, focused).View(), "\n")
			if len(lines) != sh {
				t.Errorf("%s: %d lines, want %d", s.Name, len(lines), sh)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != sw {
					t.Errorf("%s focused=%v: line %d is %d wide, want %d: %q", s.Name, focused, i, w, sw, l)
				}
			}
		}
	}
}

func TestSurfaceInsetsDriveGeometry(t *testing.T) {
	for _, s := range append(filledSurfaces(), surface.Accented) {
		p := surfacePane(s, false)
		in := s.Insets()
		c := p.ContentRect()
		if c.X != 2+in.Left || c.Y != 3+in.Top {
			t.Errorf("%s: content at (%d,%d), want (%d,%d)", s.Name, c.X, c.Y, 2+in.Left, 3+in.Top)
		}
		if c.W != sw-in.Left-in.Right-ScrollbarWidth || c.H != sh-in.Top-in.Bottom {
			t.Errorf("%s: content %dx%d", s.Name, c.W, c.H)
		}
		if row, ok := p.RowAt(c.X, c.Y+1); !ok || row != 1 {
			t.Errorf("%s: RowAt second content row = %d, %v", s.Name, row, ok)
		}
		if _, ok := p.RowAt(2+sw-1, 3); ok {
			t.Errorf("%s: a click on the margin landed on a row", s.Name)
		}
	}
}

// Inside the card every cell is painted; the margin is left for the
// container to paint.
func TestSurfaceFillsTheCardAndLeavesTheMargin(t *testing.T) {
	trueColor(t)
	for _, s := range filledSurfaces() {
		lines := strings.Split(surfacePane(s, true).View(), "\n")
		margin := 1 // every filled surface keeps one cell right and below
		for y := 0; y < sh; y++ {
			bgs := backgrounds(lines[y])
			for x := 0; x < sw; x++ {
				inMargin := x >= sw-margin || y >= sh-margin
				lip := s.Name == "filled" && (y == 0 || y == sh-margin-1)
				switch {
				case inMargin && bgs[x] != "":
					// Elevated's shadow glyphs carry a foreground only.
					t.Errorf("%s: margin cell (%d,%d) has background %q", s.Name, x, y, bgs[x])
				case !inMargin && !lip && bgs[x] == "":
					t.Errorf("%s: card cell (%d,%d) has no background\n%q", s.Name, x, y, lines[y])
				}
			}
		}
	}
}

func TestSurfaceTextRowsCarryTheSlots(t *testing.T) {
	trueColor(t)
	for _, s := range append(filledSurfaces(), surface.Accented) {
		p := surfacePane(s, false)
		lines := strings.Split(p.View(), "\n")
		in := s.Insets()
		top := ansi.Strip(lines[in.Top-1])
		if !strings.Contains(top, "Title") {
			t.Errorf("%s: title row %q has no title", s.Name, top)
		}
		var bottom string
		for _, l := range lines[sh-in.Bottom:] {
			bottom += ansi.Strip(l)
		}
		if !strings.Contains(bottom, "3/9") {
			t.Errorf("%s: bottom rows %q have no bottom slot", s.Name, bottom)
		}
	}
}

func TestSurfaceDrawsTheDivider(t *testing.T) {
	trueColor(t)
	p := surfacePane(surface.Elevated, true)
	p.SetDividedHeader("/ query", true)
	lines := strings.Split(p.View(), "\n")
	in := surface.Elevated.Insets()
	if got := ansi.Strip(lines[in.Top]); !strings.Contains(got, "/ query") {
		t.Errorf("header row = %q", got)
	}
	if got := ansi.Strip(lines[in.Top+1]); !strings.Contains(got, "───") {
		t.Errorf("divider row = %q", got)
	}
	if c := p.ContentRect(); c.Y != 3+in.Top+2 {
		t.Errorf("content starts at y=%d, want below header and divider", c.Y)
	}
}

func TestElevatedShadow(t *testing.T) {
	trueColor(t)
	lines := strings.Split(surfacePane(surface.Elevated, false).View(), "\n")
	first := []rune(ansi.Strip(lines[0]))
	if first[len(first)-1] != '▖' {
		t.Errorf("top-right shadow cell = %q, want ▖", first[len(first)-1])
	}
	last := []rune(ansi.Strip(lines[sh-1]))
	if last[0] != ' ' || last[1] != '▀' || last[len(last)-1] != '▘' {
		t.Errorf("shadow row = %q", string(last))
	}
}

// A pane given no surface, or Outlined, draws exactly what it drew before.
func TestOutlinedSurfaceIsTheLineBorder(t *testing.T) {
	trueColor(t)
	plain := New(Options{Focused: true})
	plain.SetRect(geom.New(0, 0, sw, sh))
	plain.SetTitle("Title")
	outlined := New(Options{Focused: true, Surface: resolve(surface.Outlined)})
	outlined.SetRect(geom.New(0, 0, sw, sh))
	outlined.SetTitle("Title")
	if plain.View() != outlined.View() {
		t.Errorf("Outlined differs from no surface\n%s\n%s", plain.View(), outlined.View())
	}
}

// backgrounds reports the background SGR parameters in force per cell.
func backgrounds(s string) []string {
	var out []string
	bg := ""
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				bg = applyBG(bg, s[i+2:j])
			}
			i = j + 1
			continue
		}
		r := []rune(s[i:])[0]
		out = append(out, bg)
		i += len(string(r))
	}
	return out
}

func applyBG(bg, params string) string {
	toks := strings.Split(params, ";")
	for i := 0; i < len(toks); i++ {
		switch t := toks[i]; {
		case t == "" || t == "0" || t == "49":
			bg = ""
		case t == "38" || t == "48" || t == "58":
			n := 1
			if i+1 < len(toks) {
				switch toks[i+1] {
				case "5":
					n = 3
				case "2":
					n = 5
				}
			}
			n = min(n, len(toks)-i)
			if t == "48" {
				bg = strings.Join(toks[i:i+n], ";")
			}
			i += n - 1
		}
	}
	return bg
}

// A pane asked to keep a row visible before it has a size must not scroll:
// the offset would stick once the real size arrives, and the component would
// open with its first rows out of view.
func TestEnsureVisibleBeforeSizingDoesNotScroll(t *testing.T) {
	p := New(Options{})
	p.SetContent("a\nb\nc\nd\ne\nf")
	p.EnsureVisible(3)
	p.SetRect(geom.New(0, 0, 20, 10))
	if got := p.viewport.YOffset; got != 0 {
		t.Errorf("offset after sizing = %d, want 0", got)
	}
}
