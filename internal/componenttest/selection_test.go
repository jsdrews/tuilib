package componenttest

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// selBG is the selection background every component is given here, written
// as the SGR parameters TrueColor renders it as.
const selBG = "48;2;1;2;3"

var selStyle = lipgloss.NewStyle().Background(lipgloss.Color("#010203"))

// styled is content carrying its own lipgloss styling, which closes with a
// full reset — the thing that used to end a selected row's background
// mid-row. Its colour has a zero operand, which a naive reset match misreads.
var styled = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render("red")

func selRect() geom.Rect { return geom.New(0, 0, 44, 12) }

// TestSelectedRowKeepsItsBackground is the invariant behind ansi.Fill: a
// selected row's background spans from its first cell to the pane's inner
// edge, whatever styling the row's own content carries. Each component had
// its own way of painting the row, and five of them stopped at the first
// nested reset.
func TestSelectedRowKeepsItsBackground(t *testing.T) {
	th := theme.New(palette.Dark())
	content := "SEL " + styled + " tail"

	for _, tc := range []struct {
		name string
		view func() string
	}{
		{"list", func() string {
			o := th.List()
			o.SelectedStyle = selStyle
			o.Items = []string{content, "other"}
			m := list.New(o)
			m.SetRect(selRect())
			return m.View()
		}},
		{"table", func() string {
			o := th.Table()
			o.SelectedStyle = selStyle
			o.Columns = []table.Column{{Title: "A", Width: 6}, {Title: "B", Width: 6}}
			o.Rows = []table.Row{{"SEL", styled}, {"x", "y"}}
			m := table.New(o)
			m.SetRect(selRect())
			return m.View()
		}},
		{"tree", func() string {
			o := th.Tree()
			o.CurrentLineStyle = selStyle
			o.Root = node{label: content, kids: []tree.Node{node{label: "child"}}}
			m := tree.New(o)
			m.SetRect(selRect())
			return m.View()
		}},
		{"inspector", func() string {
			o := th.Inspector()
			o.CurrentLineStyle = selStyle
			o.Fields = []inspector.Field{{Label: "SEL", Value: styled}, {Label: "k", Value: "v"}}
			m := inspector.New(o)
			m.SetRect(selRect())
			return m.View()
		}},
		{"logview", func() string {
			o := th.Logview()
			o.CurrentLineStyle = selStyle
			o.Searchable = true
			m := logview.New(o)
			m.SetRect(selRect())
			m.AppendLines([]string{content, "other"})
			m.SetQuery("SEL")
			return m.View()
		}},
		{"textview", func() string {
			o := th.TextView()
			o.CurrentLineStyle = selStyle
			o.Searchable = true
			o.Content = content + "\nother"
			m := textview.New(o)
			m.SetRect(selRect())
			m.SetQuery("SEL")
			return m.View()
		}},
		{"eventlog", func() string {
			o := th.Eventlog()
			o.SelectedStyle = selStyle
			m := eventlog.New(o)
			m.SetRect(selRect())
			m.SetPage([]eventlog.Item{{Key: "1", Lines: []string{content}}, {Key: "2", Lines: []string{"other"}}}, 0, 2, eventlog.Answer{})
			m.SetCursor(0)
			return m.View()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, start := selectedLine(t, tc.view())
			end := selRect().W - 1 - pane.ScrollbarWidth
			bgs := backgrounds(row)
			for x := start; x < end; x++ {
				if bgs[x] != selBG {
					t.Fatalf("cell %d of the selected row has background %q, want %q\nrow: %q",
						x, bgs[x], selBG, row)
				}
			}
		})
	}
}

// selectedLine finds the row holding the selection marker and the column
// its text starts at. It takes the last such row, since a search query
// naming the marker sits in the filter row above the content.
func selectedLine(t *testing.T, view string) (string, int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	for n := len(lines) - 1; n >= 0; n-- {
		plain := stripSGR(lines[n])
		if i := strings.Index(plain, "SEL"); i >= 0 {
			return lines[n], len([]rune(plain[:i]))
		}
	}
	t.Fatalf("no selected row in view:\n%s", view)
	return "", 0
}

// backgrounds reports, per visible cell, the background SGR parameters in
// force ("" for none). Every glyph drawn here is one cell wide.
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
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == ']' {
			j := strings.Index(s[i:], "\x1b\\")
			if j < 0 {
				break
			}
			i += j + 2
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
		case t == "38" || t == "58" || t == "48":
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
		case len(t) == 2 && t[0] == '4' && t[1] <= '7',
			len(t) == 3 && t[:2] == "10" && t[2] <= '7':
			bg = t
		}
	}
	return bg
}

func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == ']' {
			j := strings.Index(s[i:], "\x1b\\")
			if j < 0 {
				break
			}
			i += j + 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
