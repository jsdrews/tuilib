package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// TestModalsDrawInTheOverlayTone composites every modal the library draws,
// built from its theme builder, over a base painted in a colour of its own,
// on every surface. Inside the modal's edge every cell is the overlay form's
// fill; the edge itself carries a background (on a filled surface) and never
// lets the base through; the margin shows the base's background, whether it
// is blank or draws a shadow (Elevated), whose undrawn half takes the
// background of what the modal covers.
// A modal drawn in the card form fails both ways at once: its body is the
// card tone, and its lips leave the terminal's own background — a dark line
// — along its top and bottom rows.
func TestModalsDrawInTheOverlayTone(t *testing.T) {
	const w, h = 60, 24
	baseStyle := lipgloss.NewStyle().Background(lipgloss.Color("#ff00ff"))
	baseRow := baseStyle.Render(strings.Repeat("░", w))
	base := strings.TrimSuffix(strings.Repeat(baseRow+"\n", h), "\n")
	baseBG := backgrounds(baseRow)[0]

	type modal struct {
		name string
		// draw composites the modal over base and returns the frame and the
		// rect the modal landed in.
		draw func(th theme.Theme) (string, geom.Rect)
	}
	// cursorFirst names the modals whose first body row is their cursor, which
	// a filled surface paints in its selection band rather than the fill.
	cursorFirst := map[string]bool{"action menu": true}
	// cards names the modals whose body holds cards of its own: a form's
	// wells, a step lighter than its frame (held by
	// TestFormWellsAreAStepAboveTheCard).
	cards := map[string]bool{"titled form": true}
	centered := func(mw, mh int, c layout.Sizer) (string, geom.Rect) {
		frame := layout.ZStack(
			layout.RenderFunc(func(geom.Rect) string { return base }),
			layout.Center(mw, mh, layout.Sized(c)),
		).Render(geom.New(0, 0, w, h))
		return frame, geom.Rect{X: (w - mw) / 2, Y: (h - mh) / 2, W: mw, H: mh}
	}
	placed := func(c layout.Sizer, at func() geom.Rect) (string, geom.Rect) {
		frame := layout.ZStack(
			layout.RenderFunc(func(geom.Rect) string { return base }),
			layout.Sized(c),
		).Render(geom.New(0, 0, w, h))
		return frame, at()
	}
	modals := []modal{
		{"confirm", func(th theme.Theme) (string, geom.Rect) {
			o := th.Confirm()
			o.Title, o.Message = "Delete?", "Gone for good."
			m := confirm.New(o)
			return centered(32, 10, &m)
		}},
		{"alert", func(th theme.Theme) (string, geom.Rect) {
			o := th.Alert()
			o.Title, o.Message = "Notice", "Something happened."
			m := alert.New(o)
			return centered(32, 10, &m)
		}},
		{"titled form", func(th theme.Theme) (string, geom.Rect) {
			o := th.Form()
			o.Title, o.Modal, o.MaxWidth = "New user", true, 36
			o.Fields = []form.Field{form.Text(form.TextOptions{Key: "n", Label: "Name"})}
			m := form.New(o)
			// The form sizes and centres itself, so its rect is where it
			// landed rather than what it was given.
			return placed(&m, func() geom.Rect { return m.Rect() })
		}},
		{"action menu", func(th theme.Theme) (string, geom.Rect) {
			o := th.Actions()
			o.Set = action.Set{Target: "cache-redis", Actions: []action.Action{{Label: "Sync"}, {Label: "Delete"}}}
			m := action.New(o)
			m.Center()
			return placed(&m, func() geom.Rect { return m.Rect() })
		}},
		{"help overlay", func(th theme.Theme) (string, geom.Rect) {
			o := th.HelpOverlay()
			o.Title = "Keys"
			m := help.NewOverlay(o)
			m.SetSections([]help.Section{help.Group(help.SectionNavigate,
				key.NewBinding(key.WithKeys("j"), key.WithHelp("j", "down")),
				key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "up")),
			)})
			return placed(&m, func() geom.Rect { return m.Bounds() })
		}},
	}

	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	for _, s := range surface.Builtins() {
		th := theme.Compose(palette.Nord(), s, caps)
		over := th.Surface.Overlay()
		os := over.Surface()
		overlayBG := ""
		if c := over.Colour(surface.Fill, true); c != nil {
			overlayBG = backgrounds(lipgloss.NewStyle().Background(c).Render("x"))[0]
		}
		var marginR, marginB int
		var shadowR, shadowB bool
		if os.Margined() {
			m := os.Rings[0]
			if m.Right.Drawn() {
				marginR, shadowR = 1, strings.TrimSpace(m.Right.Glyph) != ""
			}
			if m.Bottom.Drawn() {
				marginB, shadowB = 1, strings.TrimSpace(m.Bottom.Glyph) != ""
			}
		}
		in := os.Insets()
		cardBG := map[string]bool{}
		// A button drawn as a chip, idle or chosen, in any modal.
		chipBG := map[string]bool{}
		if c := th.Surface.Colours().Of(surface.Button); c != nil {
			chipBG[backgrounds(lipgloss.NewStyle().Background(c).Render("x"))[0]] = true
		}
		chipBG[backgrounds(lipgloss.NewStyle().Background(th.Accent).Render("x"))[0]] = true
		// A field card's body, and Banded's header band across its top.
		for _, r := range []surface.Role{surface.OverlayCard, surface.OverlayCardRaised, surface.Band, surface.EdgeActive} {
			if c := th.Surface.Colours().Of(r); c != nil {
				cardBG[backgrounds(lipgloss.NewStyle().Background(c).Render("x"))[0]] = true
			}
		}

		for _, md := range modals {
			t.Run(s.Name+"/"+md.name, func(t *testing.T) {
				frame, r := md.draw(th)
				if r.W == 0 || r.H == 0 {
					t.Fatal("the modal reported no rect")
				}
				lines := strings.Split(frame, "\n")
				show := func() string { return ansi.Strip(strings.Join(lines[r.Y:r.Y+r.H], "\n")) }
				for y := r.Y; y < r.Y+r.H; y++ {
					bgs := backgrounds(lines[y])
					for x := r.X; x < r.X+r.W; x++ {
						bg := bgs[x]
						right, bottom := x >= r.X+r.W-marginR, y >= r.Y+r.H-marginB
						margin := right || bottom
						shadow := (right && shadowR) || (bottom && shadowB)
						body := x >= r.X+in.Left && x < r.X+r.W-in.Right &&
							y >= r.Y+in.Top && y < r.Y+r.H-in.Bottom
						switch {
						case margin:
							if bg != baseBG {
								t.Fatalf("margin cell (%d,%d) is %q, want the base's background:\n%s", x, y, bg, show())
							}
							if shadow && right && []rune(ansi.Strip(lines[y]))[x] == ' ' {
								t.Fatalf("shadow cell (%d,%d) is blank:\n%s", x, y, show())
							}
						case body && cursorFirst[md.name] && y == r.Y+in.Top:
							if bg != overlayBG && (bg == baseBG || bg == "") {
								t.Fatalf("cursor cell (%d,%d) is %q, want the modal's own background:\n%s", x, y, bg, show())
							}
						case body && chipBG[bg]:
						case body && cards[md.name] && cardBG[bg]:
						case body:
							if bg != overlayBG {
								t.Fatalf("body cell (%d,%d) is %q, want the overlay fill %q:\n%s", x, y, bg, overlayBG, show())
							}
						default:
							if bg == baseBG || (os.Filled() && bg == "") {
								t.Fatalf("edge cell (%d,%d) is %q, want a background of the modal's own:\n%s", x, y, bg, show())
							}
						}
					}
				}
			})
		}
	}
}

// A form's typed values sit in wells a step lighter than the card: the
// overlay's card tone, raised while focused. Drawn in the card's own tone
// they would vanish into it, and the form would read as labels with nothing
// to type into.
func TestFormWellsAreAStepAboveTheCard(t *testing.T) {
	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	bgOf := func(c lipgloss.TerminalColor) string {
		return backgrounds(lipgloss.NewStyle().Background(c).Render("x"))[0]
	}
	for _, s := range surface.Builtins() {
		th := theme.Compose(palette.Nord(), s, caps)
		if !th.Surface.Overlay().Surface().Filled() {
			continue
		}
		t.Run(s.Name, func(t *testing.T) {
			o := th.Form()
			// The first field takes focus; the second is blurred.
			o.Fields = []form.Field{
				form.Text(form.TextOptions{Key: "a", Label: "A", Placeholder: "qqa"}),
				form.Text(form.TextOptions{Key: "b", Label: "B", Placeholder: "zqx"}),
			}
			m := form.New(o)
			geom.NextGen()
			m.SetRect(geom.New(0, 0, 40, 16))
			want := map[string]surface.Role{"qqa": surface.OverlayCardRaised, "zqx": surface.OverlayCard}
			found := 0
			for _, line := range strings.Split(m.View(), "\n") {
				plain := ansi.Strip(line)
				for marker, role := range want {
					i := strings.Index(plain, marker)
					if i < 0 {
						continue
					}
					found++
					col := len([]rune(plain[:i]))
					if got, w := backgrounds(line)[col], bgOf(th.Surface.Colours().Of(role)); got != w {
						t.Errorf("%s well is %q, want the %s tone %q", marker, got, role, w)
					}
				}
			}
			if found != 2 {
				t.Fatalf("found %d of the two placeholders", found)
			}
		})
	}
}
