package componenttest

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/menu"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
	"github.com/jsdrews/tuilib/pkg/tree"
)

func resolvedSurface(s surface.Surface) surface.Resolved {
	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	return surface.Resolve(s, palette.Dark().Colours(caps.Profile), caps)
}

// TestComponentsDrawEverySurfaceAtTheirRectSize renders every pane-owning
// component under every built-in surface, filter engaged, and checks the frame
// fills its rect exactly. A component that still assumed a one-cell border —
// a hard-coded width-2, an inline filter at Y+1 — draws a frame of the wrong
// size, or puts its filter on the wrong row, under any surface but Outlined.
func TestComponentsDrawEverySurfaceAtTheirRectSize(t *testing.T) {
	th := theme.New(palette.Dark())
	const w, h = 40, 14
	r := func() geom.Rect { return geom.New(0, 0, w, h) }

	for _, s := range surface.Builtins() {
		res := resolvedSurface(s)
		for _, tc := range []struct {
			name string
			view func() string
		}{
			{"list", func() string {
				o := th.List()
				o.Surface, o.Filterable, o.Items = res, true, []string{"alpha", "beta"}
				m := list.New(o)
				m.SetRect(r())
				return m.View()
			}},
			{"table", func() string {
				o := th.Table()
				o.Surface, o.Filterable = res, true
				o.Columns = []table.Column{{Title: "A", Width: 8}, {Title: "B", Flex: 1}}
				o.Rows = []table.Row{{"a", "b"}}
				m := table.New(o)
				m.SetRect(r())
				return m.View()
			}},
			{"tree", func() string {
				o := th.Tree()
				o.Surface, o.Searchable = res, true
				o.Root = node{label: "root", kids: []tree.Node{node{label: "child"}}}
				m := tree.New(o)
				m.SetRect(r())
				return m.View()
			}},
			{"inspector", func() string {
				o := th.Inspector()
				o.Surface, o.Filterable = res, true
				o.Fields = []inspector.Field{{Label: "k", Value: "v"}}
				m := inspector.New(o)
				m.SetRect(r())
				return m.View()
			}},
			{"logview", func() string {
				o := th.Logview()
				o.Surface, o.Searchable = res, true
				m := logview.New(o)
				m.SetRect(r())
				m.AppendLines([]string{"one", "two"})
				return m.View()
			}},
			{"textview", func() string {
				o := th.TextView()
				o.Surface, o.Searchable, o.Content = res, true, "one\ntwo"
				m := textview.New(o)
				m.SetRect(r())
				return m.View()
			}},
			{"eventlog", func() string {
				o := th.Eventlog()
				o.Surface, o.Searchable = res, true
				m := eventlog.New(o)
				m.SetRect(r())
				m.SetPage([]eventlog.Item{{Key: "1", Lines: []string{"one"}}}, 0, 1, eventlog.Answer{})
				return m.View()
			}},
			{"titled form", func() string {
				o := th.Form()
				o.Surface, o.Title = res, "New user"
				o.Fields = []form.Field{form.Text(form.TextOptions{Key: "n", Label: "Name"})}
				m := form.New(o)
				m.SetRect(r())
				return m.View()
			}},
			{"menu", func() string {
				o := th.Menu()
				o.Surface, o.Filterable = res, true
				o.Items = []menu.Item{{Label: "alpha", Group: "a", Description: "first"}, {Label: "beta", Group: "b"}}
				m := menu.New(o)
				m.SetRect(r())
				return m.View()
			}},
		} {
			t.Run(s.Name+"/"+tc.name, func(t *testing.T) {
				lines := strings.Split(tc.view(), "\n")
				if len(lines) != h {
					t.Fatalf("%d lines, want %d", len(lines), h)
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got != w {
						t.Errorf("line %d is %d wide, want %d: %q", i, got, w, ansi.Strip(l))
					}
				}
			})
		}
	}
}

// TestFilledModalCoversTheBase composites a confirm drawn in Banded's
// overlay form over a screen of ░. Every row of the card must cover the
// base; before ZStack decided opacity by painted cells, a filled modal's
// blank rows showed the screen through it.
func TestFilledModalCoversTheBase(t *testing.T) {
	th := theme.New(palette.Dark())
	o := th.Confirm()
	o.Surface = resolvedSurface(surface.Banded).Overlay()
	o.Title, o.Message = "Delete?", "Gone for good."
	m := confirm.New(o)

	const w, h, mw, mh = 50, 15, 30, 9
	base := strings.TrimSuffix(strings.Repeat(strings.Repeat("░", w)+"\n", h), "\n")
	frame := layout.ZStack(
		layout.RenderFunc(func(geom.Rect) string { return base }),
		layout.Center(mw, mh, layout.Sized(&m)),
	).Render(geom.New(0, 0, w, h))

	lines := strings.Split(ansi.Strip(frame), "\n")
	top, left := (h-mh)/2, (w-mw)/2
	in := o.Surface.Insets()
	for y := top; y < top+mh-in.Bottom+1; y++ { // the last row is the margin
		row := []rune(lines[y])
		for x := left; x < left+mw-in.Right; x++ {
			if row[x] == '░' {
				t.Fatalf("base shows through the modal at (%d,%d):\n%s", x, y, strings.Join(lines, "\n"))
			}
		}
	}
}

// TestSelectionDimsWhenBlurred: on a filled surface the selected row is the
// Selection tone while its pane has focus and SelectionDim once it loses it,
// in every row component — so a screen with two panes shows which one the
// keys go to.
func TestSelectionDimsWhenBlurred(t *testing.T) {
	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	th := theme.Compose(palette.Nord(), surface.Elevated, caps)
	bgOf := func(r surface.Role) string {
		return backgrounds(lipgloss.NewStyle().Background(th.Surface.Colour(r, true)).Render("x"))[0]
	}
	focusedBG, blurredBG := bgOf(surface.Selection), bgOf(surface.SelectionDim)
	if focusedBG == blurredBG {
		t.Fatalf("Selection and SelectionDim are the same colour, %q", focusedBG)
	}

	type comp interface {
		View() string
		Blur()
	}
	for _, tc := range []struct {
		name  string
		build func() comp
	}{
		{"list", func() comp {
			o := th.List()
			o.Items = []string{"SEL row", "other"}
			m := list.New(o)
			m.SetRect(selRect())
			return &m
		}},
		{"table", func() comp {
			o := th.Table()
			o.Columns = []table.Column{{Title: "A", Width: 8}}
			o.Rows = []table.Row{{"SEL"}, {"x"}}
			m := table.New(o)
			m.SetRect(selRect())
			return &m
		}},
		{"tree", func() comp {
			o := th.Tree()
			o.Root = node{label: "SEL root", kids: []tree.Node{node{label: "child"}}}
			m := tree.New(o)
			m.SetRect(selRect())
			return &m
		}},
		{"inspector", func() comp {
			o := th.Inspector()
			o.Fields = []inspector.Field{{Label: "SEL", Value: "v"}, {Label: "k", Value: "v"}}
			m := inspector.New(o)
			m.SetRect(selRect())
			return &m
		}},
		{"logview", func() comp {
			o := th.Logview()
			o.Searchable = true
			m := logview.New(o)
			m.SetRect(selRect())
			m.AppendLines([]string{"SEL line", "other"})
			m.SetQuery("SEL")
			return &m
		}},
		{"textview", func() comp {
			o := th.TextView()
			o.Searchable, o.Content = true, "SEL line\nother"
			m := textview.New(o)
			m.SetRect(selRect())
			m.SetQuery("SEL")
			return &m
		}},
		{"eventlog", func() comp {
			o := th.Eventlog()
			m := eventlog.New(o)
			m.SetRect(selRect())
			m.SetPage([]eventlog.Item{{Key: "1", Lines: []string{"SEL line"}}, {Key: "2", Lines: []string{"other"}}}, 0, 2, eventlog.Answer{})
			m.SetCursor(0)
			return &m
		}},
		{"menu", func() comp {
			o := th.Menu()
			o.Items = []menu.Item{{Label: "SEL row", Description: "gloss"}, {Label: "other"}}
			m := menu.New(o)
			m.SetRect(selRect())
			return &m
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.build()
			row, start := selectedLine(t, c.View())
			if got := backgrounds(row)[start]; got != focusedBG {
				t.Errorf("focused: selected row background %q, want Selection %q", got, focusedBG)
			}
			c.Blur()
			row, start = selectedLine(t, c.View())
			if got := backgrounds(row)[start]; got != blurredBG {
				t.Errorf("blurred: selected row background %q, want SelectionDim %q", got, blurredBG)
			}
		})
	}
}

// TestFieldsFillOnCardSurfaces: on a filled surface the single-line
// components draw as small cards in the surface's own tone rather than as
// outlined fields. The card — the title and content rows, short of the right
// margin — carries a background in every cell, focused and blurred; the
// margin carries none, so the parent paints it and stacked fields keep a row
// of screen between them.
func TestFieldsFillOnCardSurfaces(t *testing.T) {
	type field interface {
		View() string
		Focus() tea.Cmd
		Blur()
	}
	const w = 30
	r := geom.New(0, 0, w, 3)
	for _, s := range []surface.Surface{surface.Filled, surface.Banded, surface.Elevated} {
		th := theme.Compose(palette.Nord(), s, surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true})
		for _, tc := range []struct {
			name  string
			build func() field
		}{
			{"input", func() field {
				o := th.Input()
				o.Title, o.Placeholder = "name", "type here"
				m := input.New(o)
				m.SetRect(r)
				return &m
			}},
			{"toggle", func() field {
				o := th.Toggle()
				o.Title = "sure?"
				m := toggle.New(o)
				m.SetRect(r)
				return &m
			}},
			{"filter", func() field {
				o := th.Filter()
				o.Title = "filter"
				m := filter.New(o)
				m.SetRect(r)
				return &m
			}},
		} {
			t.Run(s.Name+"/"+tc.name, func(t *testing.T) {
				f := tc.build()
				for _, focused := range []bool{true, false} {
					if focused {
						f.Focus()
					} else {
						f.Blur()
					}
					lines := strings.Split(f.View(), "\n")
					if len(lines) != 3 {
						t.Fatalf("focused=%v: %d rows, want 3", focused, len(lines))
					}
					for y, l := range lines {
						bgs := backgrounds(l)
						if len(bgs) != w {
							t.Fatalf("focused=%v: row %d is %d wide, want %d", focused, y, len(bgs), w)
						}
						for x, bg := range bgs {
							margin := y == 2 || x == w-1
							if margin && bg != "" {
								t.Fatalf("focused=%v: margin cell (%d,%d) is painted %q: %q", focused, x, y, bg, ansi.Strip(l))
							}
							if !margin && bg == "" {
								t.Fatalf("focused=%v: card cell (%d,%d) is unpainted: %q", focused, x, y, ansi.Strip(l))
							}
						}
					}
				}
			})
		}
	}
}
