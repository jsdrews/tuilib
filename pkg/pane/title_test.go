package pane

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/surface"
)

// A title wider than its border row is cut with an ellipsis, on a line
// border and on a filled edge alike, and the row keeps its width.
func TestLongTitleEndsInEllipsis(t *testing.T) {
	caps := surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true}
	p := palette.Nord()
	for _, s := range []surface.Surface{surface.Outlined, surface.Filled, surface.Banded} {
		t.Run(s.Name, func(t *testing.T) {
			pn := New(Options{
				Title:   "List every deployment in the production cluster, newest first",
				Surface: surface.Resolve(s, p.Colours(caps.Profile), caps),
				Focused: true,
			})
			pn.SetTopRight("3/9")
			pn.SetContent("x")
			pn.SetRect(geom.New(0, 0, 30, 5))
			lines := strings.Split(pn.View(), "\n")
			var row string
			for _, l := range lines {
				if strings.Contains(ansi.Strip(l), "List") {
					row = ansi.Strip(l)
				}
			}
			if !strings.Contains(row, "…") {
				t.Fatalf("title not cut with an ellipsis: %q", row)
			}
			if !strings.Contains(row, "3/9") {
				t.Errorf("the other slot on the row was pushed off: %q", row)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != 30 {
					t.Errorf("line %d is %d wide, want 30", i, w)
				}
			}
		})
	}
}
