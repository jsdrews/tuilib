package app

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tuiansi "github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/config"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/output"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// axes is the app's palette × surface state: the two lists, where the user is in each, the terminal's
// capabilities, and the theme composed from the current pair.
type axes struct {
	palettes []palette.Palette
	surfaces []surface.Surface
	pi, si   int
	caps     surface.Caps
	theme    theme.Theme
}

func newAxes(opts Options) *axes {
	a := &axes{palettes: opts.Palettes, surfaces: opts.Surfaces}
	if len(a.surfaces) == 0 {
		a.surfaces = []surface.Surface{surface.Outlined}
	}
	var cfg config.Config
	if !opts.SkipConfig {
		cfg, _ = config.Load()
	}
	palNames := make([]string, len(a.palettes))
	for i, p := range a.palettes {
		palNames[i] = p.Name
	}
	surfNames := make([]string, len(a.surfaces))
	for i, s := range a.surfaces {
		surfNames[i] = s.Name
	}
	if !opts.SkipConfig {
		a.pi = pick(palNames, env(opts.PaletteEnvVar), cfg.Palette, cfg.Theme)
		a.si = pick(surfNames, env(opts.SurfaceEnvVar), cfg.Surface)
	}
	a.caps = detectCaps(opts, cfg)
	a.compose()
	return a
}

// pick is the index of the first non-empty name in picks found in names, or 0.
func pick(names []string, picks ...string) int {
	for _, want := range picks {
		if want == "" {
			continue
		}
		for i, n := range names {
			if n == want {
				return i
			}
		}
	}
	return 0
}

func env(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}

// detectCaps is what the terminal offers: its colour profile, and whether it
// draws block glyphs without seams — from config when the user said, else
// off for terminals known to draw them from the font.
func detectCaps(opts Options, cfg config.Config) surface.Caps {
	if opts.Capabilities != nil {
		return *opts.Capabilities
	}
	caps := surface.Caps{Profile: lipgloss.ColorProfile(), BlockGlyphs: true}
	switch {
	case cfg.BlockGlyphs != nil:
		caps.BlockGlyphs = *cfg.BlockGlyphs
	case fontDrawnBlocks[os.Getenv("TERM_PROGRAM")]:
		caps.BlockGlyphs = false
	}
	return caps
}

// fontDrawnBlocks are terminals that draw block elements from the font, so
// half-block edges show hairline seams.
var fontDrawnBlocks = map[string]bool{"Apple_Terminal": true}

func (a *axes) compose() {
	a.theme = theme.Compose(a.palettes[a.pi], a.surfaces[a.si], a.caps)
}

func (a *axes) nextPalette() {
	a.pi = (a.pi + 1) % len(a.palettes)
	a.compose()
}

// nextSurface steps to the next surface that draws differently from the
// current one. On a terminal without block glyphs Filled and Elevated both
// resolve to Banded, and stepping through two identical Bandeds would read
// as a key that did nothing.
func (a *axes) nextSurface() {
	cur := a.theme.Surface.Name()
	for range a.surfaces {
		a.si = (a.si + 1) % len(a.surfaces)
		a.compose()
		if a.theme.Surface.Name() != cur {
			return
		}
	}
}

// selectPalette switches to the palette called name, if there is one.
func (a *axes) selectPalette(name string) bool {
	for i, p := range a.palettes {
		if p.Name == name {
			a.pi = i
			a.compose()
			return true
		}
	}
	return false
}

// selectSurface switches to the surface called name, if there is one.
func (a *axes) selectSurface(name string) bool {
	for i, s := range a.surfaces {
		if s.Name == name {
			a.si = i
			a.compose()
			return true
		}
	}
	return false
}

// fallbackNote says why the chosen surface isn't the one drawn, or "".
func (a *axes) fallbackNote() string {
	asked, why, ok := a.theme.Surface.FellBack()
	if !ok {
		return ""
	}
	return "surface " + asked + " → " + a.theme.Surface.Name() + " (" + why + ")"
}

// logFallback writes the current fallback to the output console, once per
// distinct fallback, so a user whose chosen surface didn't take can see why
// and which setting changes it.
func (m *Model) logFallback() {
	note := m.ax.fallbackNote()
	if note == "" || note == m.lastFallback {
		m.lastFallback = note
		return
	}
	m.lastFallback = note
	m.logEntry("surface", note, "", output.LevelInfo)
}

// surfaceBody pads the body to match a surface's margins: cards keep one
// cell of screen to their right and below, so the screen's left and top
// edges get one too.
func (m Model) surfaceBody(body layout.Node) layout.Node {
	if !m.ax.theme.Surface.Surface().Margined() {
		return body
	}
	blank := layout.RenderFunc(func(r geom.Rect) string {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", r.W)+"\n", r.H), "\n")
	})
	return layout.HStack(
		layout.Fixed(1, blank),
		layout.Flex(1, layout.VStack(
			layout.Fixed(1, blank),
			layout.Flex(1, body),
		)),
	)
}

// screenPass paints every cell nothing else painted in the Screen tone:
// gutters, margins, padding, anything a component left bare. It runs once
// over the whole frame, after compositing, so a margin a card left for its
// container gets the container's colour — and at the top level that is the
// screen.
func (m Model) screenPass(frame string) string {
	r := m.ax.theme.Surface
	if _, lined := r.Surface().Lined(); lined {
		return frame
	}
	bg := r.Colour(surface.Screen, false)
	if bg == nil {
		return frame
	}
	st := lipgloss.NewStyle().Background(bg)
	if fg := r.Colour(surface.Text, false); fg != nil {
		st = st.Foreground(fg)
	}
	lines := strings.Split(frame, "\n")
	for i, l := range lines {
		lines[i] = tuiansi.RenderFill(st.Render, l)
	}
	return strings.Join(lines, "\n")
}
