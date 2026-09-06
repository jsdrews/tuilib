package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// glyphTheme uses marks no component would ever draw on its own, so a
// component still holding a literal fails here instead of passing because the
// default happens to match.
func glyphTheme() theme.Theme {
	t := theme.Dark()
	t.Glyphs = glyph.Set{
		Cursor:       "»",
		Mark:         "@",
		ExpandOpen:   "v",
		ExpandClosed: ">",
		Rule:         "=",
		ScrollThumb:  "#",
		ScrollTrack:  "+",
		SortAsc:      "^",
		SortDesc:     "!",
	}
	return t
}

func glyphRect() geom.Rect { return geom.New(0, 0, 44, 12) }

// TestComponentsDrawThemeGlyphs is the invariant behind the glyph pass: the
// same marks were hardcoded in seven places (the inline-filter rule alone
// appeared identically in every filterable component), so changing one meant
// finding all of them by hand.
func TestComponentsDrawThemeGlyphs(t *testing.T) {
	th := glyphTheme()

	for _, tc := range []struct {
		name string
		want string
		what string
		view func() string
	}{
		{"list cursor", "»", "the row cursor", func() string {
			o := th.List()
			o.Items = []string{"alpha", "beta"}
			m := list.New(o)
			m.SetRect(glyphRect())
			return m.View()
		}},
		{"list filter rule", "=", "the inline-filter rule", func() string {
			o := th.List()
			o.Items, o.Filterable = []string{"alpha"}, true
			m := list.New(o)
			m.SetRect(glyphRect())
			return m.View()
		}},
		{"tree expand", ">", "the collapsed disclosure arrow", func() string {
			o := th.Tree()
			o.Root = node{label: "root", kids: []tree.Node{node{label: "child"}}}
			m := tree.New(o)
			m.SetRect(glyphRect())
			return m.View()
		}},
		{"inspector expand", "v", "the expanded disclosure arrow", func() string {
			o := th.Inspector()
			o.InitialDepth = 2
			o.Fields = []inspector.Field{{Label: "meta", Children: []inspector.Field{{Label: "k", Value: "x"}}}}
			m := inspector.New(o)
			m.SetRect(glyphRect())
			return m.View()
		}},
		{"table sort", "^", "the ascending sort marker", func() string {
			o := th.Table()
			o.Columns = []table.Column{{Title: "Name", Width: 10, Sortable: true}}
			o.Rows = []table.Row{{"a"}, {"b"}}
			m := table.New(o)
			m.SetSort(0, false)
			m.SetRect(glyphRect())
			return m.View()
		}},
		{"tree mark", "@", "the mark gutter glyph", func() string {
			o := th.Tree()
			o.Markable = true
			o.Root = node{label: "root", kids: []tree.Node{node{label: "child"}}}
			m := tree.New(o)
			m.SetRect(glyphRect())
			m.ToggleMark()
			return m.View()
		}},
		{"action cursor", "»", "the menu cursor", func() string {
			o := th.Actions()
			o.Set = action.Set{Actions: []action.Action{{Label: "Run"}}}
			m := action.New(o)
			m.SetRect(glyphRect())
			m.Center()
			return m.View()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := ansi.Strip(tc.view())
			if !strings.Contains(view, tc.want) {
				t.Errorf("%s: %s is not coming from the theme (want %q)\n%s",
					tc.name, tc.what, tc.want, view)
			}
		})
	}
}

// TestPartialGlyphSetKeepsDefaults guards the resolve step. A theme that sets
// one arrow must not blank the other twelve — the failure mode would be
// invisible marks rather than a crash.
func TestPartialGlyphSetKeepsDefaults(t *testing.T) {
	th := theme.Dark()
	th.Glyphs = glyph.Set{Cursor: "»"}

	o := th.List()
	o.Markable = true
	m := list.New(o)
	// Marks are keyed, so anonymous string items cannot be marked at all.
	m.SetKeyedItems([]list.KeyedItem{{Key: "a", Display: "alpha"}, {Key: "b", Display: "beta"}})
	m.SetRect(glyphRect())
	m.ToggleMark()
	m.SetCursor(1) // the cursor glyph owns its own row's gutter

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "»") {
		t.Error("the overridden cursor did not reach the list")
	}
	if !strings.Contains(view, glyph.Default().Mark) {
		t.Errorf("an unset field came through empty instead of defaulting\n%s", view)
	}
}
