package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// shapeTheme uses borders no component defaults to, so a component that
// ignores the theme and falls back to its own literal fails here rather than
// passing by coincidence. Asserting on NormalBorder would prove nothing: it
// is what every component would draw anyway.
func shapeTheme() theme.Theme {
	t := theme.Dark()
	t.BorderShapeActive = lipgloss.RoundedBorder()
	t.BorderShapeInactive = lipgloss.RoundedBorder()
	t.BorderShapeOverlay = lipgloss.DoubleBorder()
	return t
}

const (
	componentCorner = "╭" // RoundedBorder top-left
	overlayCorner   = "╔" // DoubleBorder top-left
)

// corner returns a component's top-left border glyph, which identifies the
// shape it drew. It skips leading blank rows and indent because a component
// that centers itself inside its rect (pkg/action) pads before its frame.
func corner(t *testing.T, view string) string {
	t.Helper()
	for _, line := range strings.Split(ansi.Strip(view), "\n") {
		if trimmed := strings.TrimLeft(line, " "); trimmed != "" {
			return string([]rune(trimmed)[0])
		}
	}
	t.Fatalf("component rendered nothing")
	return ""
}

func shapeRect() geom.Rect { return geom.New(0, 0, 40, 9) }

// TestThemeOwnsComponentBorderShape is the invariant that keeps border shape
// consolidated. Before it existed, seven theme builders pinned a literal
// shape, three components inherited a different default through pane, and
// pkg/form could not be reached at all — so the same theme drew thick chrome
// on a confirm and normal chrome on the list behind it.
func TestThemeOwnsComponentBorderShape(t *testing.T) {
	th := shapeTheme()

	for _, tc := range []struct {
		name string
		view func() string
	}{
		{"pane", func() string {
			o := th.Pane()
			o.Title = "pane"
			p := pane.New(o)
			p.SetRect(shapeRect())
			return p.View()
		}},
		{"list", func() string {
			o := th.List()
			o.Items = []string{"a", "b"}
			m := list.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"table", func() string {
			o := th.Table()
			o.Columns = []table.Column{{Title: "C", Width: 6}}
			o.Rows = []table.Row{{"x"}}
			m := table.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"tree", func() string {
			o := th.Tree()
			o.Root = node{label: "root"}
			m := tree.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"inspector", func() string {
			o := th.Inspector()
			o.Fields = []inspector.Field{{Label: "k", Value: "v"}}
			m := inspector.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"logview", func() string {
			m := logview.New(th.Logview())
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"textview", func() string {
			o := th.TextView()
			o.Content = "hello"
			m := textview.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"input", func() string {
			m := input.New(th.Input())
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"filter", func() string {
			m := filter.New(th.Filter())
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"toggle", func() string {
			m := toggle.New(th.Toggle())
			m.SetRect(shapeRect())
			return m.View()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := corner(t, tc.view()); got != componentCorner {
				t.Errorf("%s drew %q, want %q — it is not taking its shape from the theme",
					tc.name, got, componentCorner)
			}
		})
	}
}

// TestThemeOwnsOverlayBorderShape covers the three components that float over
// content in a ZStack. They take a heavier border on purpose: it is what
// separates an overlay from the pane it is covering.
func TestThemeOwnsOverlayBorderShape(t *testing.T) {
	th := shapeTheme()

	for _, tc := range []struct {
		name string
		view func() string
	}{
		{"confirm", func() string {
			o := th.Confirm()
			o.Title, o.Message = "confirm", "sure?"
			m := confirm.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"alert", func() string {
			o := th.Alert()
			o.Title, o.Message = "alert", "notice"
			m := alert.New(o)
			m.SetRect(shapeRect())
			return m.View()
		}},
		{"action", func() string {
			o := th.Actions()
			o.Set = action.Set{Actions: []action.Action{{Label: "Run"}}}
			m := action.New(o)
			m.SetRect(shapeRect())
			m.Center() // a menu measures and places itself before it draws
			return m.View()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := corner(t, tc.view()); got != overlayCorner {
				t.Errorf("%s drew %q, want %q — an overlay must read as raised above content",
					tc.name, got, overlayCorner)
			}
		})
	}
}

// TestFormFieldsTakeThemeBorderShape covers the gap that motivated this pass:
// a form's field borders were hardcoded and unreachable, so a theme could not
// restyle them by any means at all.
func TestFormFieldsTakeThemeBorderShape(t *testing.T) {
	th := shapeTheme()
	o := th.Form()
	o.Fields = []form.Field{
		form.Text(form.TextOptions{Key: "name", Label: "Name"}),
		form.Password(form.PasswordOptions{Key: "pass", Label: "Password"}),
		form.Select(form.SelectOptions{Key: "role", Label: "Role", Options: []string{"a"}}),
		form.Confirm(form.ConfirmOptions{Key: "ok", Label: "OK?"}),
	}
	m := form.New(o)
	m.SetRect(geom.New(0, 0, 40, 22))

	view := ansi.Strip(m.View())
	if n := strings.Count(view, componentCorner); n != len(o.Fields) {
		t.Errorf("form drew %d themed field corners, want %d\n%s", n, len(o.Fields), view)
	}
	if strings.Contains(view, "┌") || strings.Contains(view, "┏") {
		t.Errorf("a form field ignored the theme and drew its own shape:\n%s", view)
	}
}
