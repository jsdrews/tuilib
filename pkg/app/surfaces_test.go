package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// listScreen is a screen holding one themed list, rebuilt on SetTheme the way
// rule 4 says.
type listScreen struct {
	stubScreen
	list list.Model
}

func (s *listScreen) SetTheme(t theme.Theme) {
	st := s.list.State()
	o := t.List()
	o.Title, o.Items = "Cities", []string{"London", "Paris", "Tokyo"}
	s.list = list.New(o)
	s.list.Restore(st)
}

func (s *listScreen) Layout() layout.Node { return layout.Sized(&s.list) }

func trueColour(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

var richCaps = surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}

func surfaceApp(t *testing.T, opts Options) Model {
	t.Helper()
	opts.Root = &listScreen{stubScreen: stubScreen{name: "root"}}
	opts.SkipConfig = true
	if opts.Capabilities == nil {
		opts.Capabilities = &richCaps
	}
	m := New(opts)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	return tm.(Model)
}

// Built from Palettes with the default surface, an app draws exactly what the
// same palette drew as a Theme.
func TestPalettesWithOutlinedMatchThemes(t *testing.T) {
	trueColour(t)
	viaPalettes := surfaceApp(t, Options{Palettes: []palette.Palette{palette.Nord()}})
	viaThemes := New(Options{
		Root:       &listScreen{stubScreen: stubScreen{name: "root"}},
		Palettes:   []palette.Palette{palette.Nord()},
		SkipConfig: true,
	})
	tm, _ := viaThemes.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	if got, want := viaPalettes.View(), tm.(Model).View(); got != want {
		t.Errorf("Outlined via Palettes differs from Themes:\n%s\n---\n%s", got, want)
	}
}

// Under a filled surface every cell of the frame is painted: the cards fill
// themselves and the shell paints everything else in the screen tone.
func TestFilledSurfaceFrameIsFullyPainted(t *testing.T) {
	trueColour(t)
	for _, s := range []surface.Surface{surface.Filled, surface.Banded, surface.Elevated, surface.Accented} {
		m := surfaceApp(t, Options{Palettes: []palette.Palette{palette.Nord()}, Surfaces: []surface.Surface{s}})
		lines := strings.Split(m.View(), "\n")
		if len(lines) != termH {
			t.Fatalf("%s: %d lines, want %d", s.Name, len(lines), termH)
		}
		for y, l := range lines {
			if w := ansi.StringWidth(l); w != termW {
				t.Errorf("%s: line %d is %d wide", s.Name, y, w)
			}
			for x, bg := range backgroundsOf(l) {
				if bg == "" {
					t.Fatalf("%s: cell (%d,%d) is unpainted:\n%q", s.Name, x, y, l)
				}
			}
		}
	}
}

// A surface with margins gets a matching gutter on the screen's left edge,
// and one whose top edge is a solid row gets a row of screen under the
// breadcrumb. Filled's top edge is a lip, which is its own gap.
func TestSurfacePadsTheBody(t *testing.T) {
	trueColour(t)
	m := surfaceApp(t, Options{Palettes: []palette.Palette{palette.Nord()}, Surfaces: []surface.Surface{surface.Elevated}})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if strings.TrimSpace(lines[1]) != "" {
		t.Errorf("Elevated: row under the breadcrumb should be screen, got %q", lines[1])
	}
	if r := []rune(lines[2]); r[0] != ' ' || r[1] != '▎' {
		t.Errorf("Elevated: card should start at column 1, row 2: %q", lines[2])
	}

	m = surfaceApp(t, Options{Palettes: []palette.Palette{palette.Nord()}, Surfaces: []surface.Surface{surface.Filled}})
	lines = strings.Split(ansi.Strip(m.View()), "\n")
	if r := []rune(lines[1]); r[0] != ' ' || r[1] != '▄' {
		t.Errorf("Filled: its lip should sit right under the breadcrumb: %q", lines[1])
	}
}

func TestSurfaceKeyCyclesAndSkipsDuplicates(t *testing.T) {
	noBlocks := surface.Caps{Profile: termenv.TrueColor}
	m := surfaceApp(t, Options{
		Palettes:     []palette.Palette{palette.Nord()},
		Surfaces:     []surface.Surface{surface.Outlined, surface.Filled, surface.Elevated, surface.Banded},
		SurfaceKey:   key.NewBinding(key.WithKeys("S")),
		Capabilities: &noBlocks,
	})
	var seen []string
	for i := 0; i < 3; i++ {
		seen = append(seen, m.theme().Surface.Name())
		m = send(t, m, typeKey("S"))
	}
	// Filled and Elevated both fall back to Banded without block glyphs, and
	// Banded is offered as itself: one step reaches it, the next returns.
	want := []string{"outlined", "banded", "outlined"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("surfaces cycled %v, want %v", seen, want)
		}
	}
}

func TestThemeKeyCyclesPalettes(t *testing.T) {
	m := surfaceApp(t, Options{
		Palettes: []palette.Palette{palette.Nord(), palette.Dracula()},
		ThemeKey: key.NewBinding(key.WithKeys("t")),
	})
	m = send(t, m, typeKey("t"))
	if got := m.theme().Name; got != "dracula" {
		t.Errorf("palette after t = %q, want dracula", got)
	}
}

func TestFallbackIsLoggedToTheConsole(t *testing.T) {
	noBlocks := surface.Caps{Profile: termenv.TrueColor}
	m := surfaceApp(t, Options{
		Palettes:     []palette.Palette{palette.Nord()},
		Surfaces:     []surface.Surface{surface.Elevated},
		Capabilities: &noBlocks,
		OutputKey:    key.NewBinding(key.WithKeys("o")),
	})
	var found bool
	for _, r := range m.outBuf.Records() {
		if strings.Contains(r.Text, "surface elevated → banded (block glyphs off)") {
			found = true
		}
	}
	if !found {
		t.Errorf("no fallback note in the console: %+v", m.outBuf.Records())
	}
}

func TestPaletteEnvVarPicksTheStart(t *testing.T) {
	t.Setenv("TEST_PALETTE", "dracula")
	m := New(Options{
		Root:          &listScreen{stubScreen: stubScreen{name: "root"}},
		Palettes:      []palette.Palette{palette.Nord(), palette.Dracula()},
		PaletteEnvVar: "TEST_PALETTE",
		Capabilities:  &richCaps,
	})
	if got := m.theme().Name; got != "dracula" {
		t.Errorf("start palette = %q, want dracula", got)
	}
}

var _ screen.Screen = (*listScreen)(nil)

// backgroundsOf reports, per cell, whether a background is in force — the
// SGR parameters setting it, or "" for none.
func backgroundsOf(s string) []string {
	var out []string
	bg := ""
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				toks := strings.Split(s[i+2:j], ";")
				for k := 0; k < len(toks); k++ {
					switch tk := toks[k]; {
					case tk == "" || tk == "0" || tk == "49":
						bg = ""
					case tk == "38" || tk == "48" || tk == "58":
						n := 1
						if k+1 < len(toks) && toks[k+1] == "5" {
							n = 3
						} else if k+1 < len(toks) && toks[k+1] == "2" {
							n = 5
						}
						n = min(n, len(toks)-k)
						if tk == "48" {
							bg = strings.Join(toks[k:k+n], ";")
						}
						k += n - 1
					}
				}
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

func TestSetSurfaceSwitchesByName(t *testing.T) {
	m := surfaceApp(t, Options{
		Palettes: []palette.Palette{palette.Nord()},
		Surfaces: []surface.Surface{surface.Outlined, surface.Banded},
	})
	m = send(t, m, SetSurfaceMsg{Name: "banded"})
	if got := m.theme().Surface.Name(); got != "banded" {
		t.Errorf("surface = %q, want banded", got)
	}
	m = send(t, m, SetSurfaceMsg{Name: "nope"})
	if got := m.theme().Surface.Name(); got != "banded" {
		t.Errorf("an unknown name changed the surface to %q", got)
	}
}
