package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
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
	th := theme.Dark()
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
	th := theme.Dark()
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
