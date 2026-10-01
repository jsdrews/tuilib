package componenttest

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/theme"
	"github.com/jsdrews/tuilib/pkg/toggle"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// A theme swap rebuilds a component from new Options (rule 4), and State /
// Restore carry what the user did across it. The contract, for every
// component: snapshot, build a fresh one from the same Options, restore —
// and it must draw exactly what it drew before. Rendering equality is the
// strictest observable check there is: a cursor, a filter, an open branch,
// a mark, a scroll offset or a stale suffix that didn't come back shows.

type restorable interface {
	View() string
}

type roundTrip struct {
	name string
	// build returns a fresh component from fixed Options, placed.
	build func() restorable
	// use changes its state the way a user and a source would.
	use func(restorable) restorable
	// swap snapshots c, builds a fresh one, restores into it.
	swap func(c restorable) restorable
}

func keyOf(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func lines(n int, f string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(f, i)
	}
	return out
}

func TestStateRestoreRedrawsTheSame(t *testing.T) {
	th := theme.Dark()
	cases := []roundTrip{
		{
			name: "list",
			build: func() restorable {
				o := th.List()
				o.Title, o.Filterable, o.Markable = "l", true, true
				m := list.New(o)
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*list.Model)
				var items []list.KeyedItem
				for i, s := range lines(40, "item-%02d") {
					items = append(items, list.KeyedItem{Key: fmt.Sprint(i), Display: s})
				}
				m.SetKeyedItems(items)
				m.SetValue("1")
				m.SetCursor(3)
				m.ToggleMark()
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*list.Model).State()
				n := list.New(func() list.Options { o := th.List(); o.Title, o.Filterable, o.Markable = "l", true, true; return o }())
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "table",
			build: func() restorable {
				m := table.New(tableOpts(th))
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*table.Model)
				var rows []table.KeyedRow
				for i := 0; i < 40; i++ {
					rows = append(rows, table.KeyedRow{Key: fmt.Sprint(i), Cells: []string{fmt.Sprintf("city-%02d", i), fmt.Sprint(i % 3)}})
				}
				m.SetKeyedRows(rows)
				m.SetSort(0, true)
				m.SetValue("1")
				m.SetCursor(2)
				m.ToggleMark()
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*table.Model).State()
				n := table.New(tableOpts(th))
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "windowed table, stale",
			build: func() restorable {
				o := tableOpts(th)
				o.FilterMode, o.SortMode, o.SortDebounce = table.FilterRemote, table.SortRemote, -1
				m := table.New(o)
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*table.Model)
				rows := make([]table.Row, 50)
				for i := range rows {
					rows[i] = table.Row{fmt.Sprintf("r%03d", 100+i), "x"}
				}
				m.SetWindow(rows, 100, 1000, table.Answer{})
				m.SetCursor(110)
				m.SetValue("x") // silent: the committed query is now "x"…
				m.SetFailed(table.Answer{Raw: "x"})
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*table.Model).State()
				o := tableOpts(th)
				o.FilterMode, o.SortMode, o.SortDebounce = table.FilterRemote, table.SortRemote, -1
				n := table.New(o)
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "tree",
			build: func() restorable {
				m := tree.New(treeOpts(th))
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*tree.Model)
				m.SetRoot(sampleTree())
				*m, _ = m.Update(keyOf("j"))
				*m, _ = m.Update(keyOf("space")) // open a branch
				*m, _ = m.Update(keyOf("j"))
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*tree.Model).State()
				n := tree.New(treeOpts(th))
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "inspector",
			build: func() restorable {
				m := inspector.New(inspectorOpts(th))
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*inspector.Model)
				m.SetFields(inspector.FromMap(map[string]any{
					"metadata": map[string]any{"name": "api", "labels": map[string]any{"app": "api", "tier": "web"}},
					"spec":     map[string]any{"replicas": 3},
				}))
				*m, _ = m.Update(keyOf("j"))
				*m, _ = m.Update(keyOf("space"))
				*m, _ = m.Update(keyOf("j"))
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*inspector.Model).State()
				n := inspector.New(inspectorOpts(th))
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "logview",
			build: func() restorable {
				o := th.Logview()
				o.Title, o.Searchable, o.LineNumbers = "log", true, true
				m := logview.New(o)
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*logview.Model)
				m.AppendLines(lines(60, "line %02d"))
				m.AppendMarker("-- reconnected --")
				m.SetFollow(false)
				m.SetQuery("line 1")
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*logview.Model).State()
				o := th.Logview()
				o.Title, o.Searchable, o.LineNumbers = "log", true, true
				n := logview.New(o)
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "textview",
			build: func() restorable {
				o := th.TextView()
				o.Title, o.Searchable = "doc", true
				m := textview.New(o)
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*textview.Model)
				m.SetContent(strings.Join(lines(60, "paragraph %02d of the document"), "\n"))
				m.SetWrap(false)
				m.SetQuery("paragraph 3")
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*textview.Model).State()
				o := th.TextView()
				o.Title, o.Searchable = "doc", true
				n := textview.New(o)
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "eventlog",
			build: func() restorable {
				m := eventlog.New(eventlogOpts(th))
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*eventlog.Model)
				m.SetFollow(false)
				var items []eventlog.Item
				for i := 0; i < 100; i++ {
					items = append(items, eventlog.Item{Key: fmt.Sprint(i), Lines: []string{fmt.Sprintf("event %d", i)}})
				}
				m.SetPage(items, 0, 800, eventlog.Answer{})
				m.SetCursor(20)
				m.SetTerm("event 2")
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*eventlog.Model).State()
				n := eventlog.New(eventlogOpts(th))
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "filter",
			build: func() restorable {
				m := filter.New(th.Filter())
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*filter.Model)
				m.SetValue("region:eu")
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*filter.Model).State()
				n := filter.New(th.Filter())
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "input",
			build: func() restorable {
				o := th.Input()
				o.Title = "name"
				m := input.New(o)
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*input.Model)
				m.SetValue("orders-api")
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*input.Model).State()
				o := th.Input()
				o.Title = "name"
				n := input.New(o)
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
		{
			name: "toggle",
			build: func() restorable {
				o := th.Toggle()
				o.Title = "dry run"
				m := toggle.New(o)
				m.SetRect(placed())
				return &m
			},
			use: func(c restorable) restorable {
				m := c.(*toggle.Model)
				m.SetValue(true)
				return m
			},
			swap: func(c restorable) restorable {
				st := c.(*toggle.Model).State()
				o := th.Toggle()
				o.Title = "dry run"
				n := toggle.New(o)
				n.SetRect(placed())
				n.Restore(st)
				return &n
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.use(tc.build())
			before := c.View()
			if before == tc.build().View() {
				t.Fatal("use() changed nothing visible; the case proves nothing")
			}
			after := tc.swap(c).View()
			if after != before {
				t.Errorf("restored component draws differently.\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

func tableOpts(th theme.Theme) table.Options {
	o := th.Table()
	o.Title, o.Filterable, o.Markable = "t", true, true
	o.Columns = []table.Column{{Title: "Name", Width: 12, Sortable: true}, {Title: "N", Width: 4}}
	return o
}

func treeOpts(th theme.Theme) tree.Options {
	o := th.Tree()
	o.Title, o.Searchable = "tree", true
	return o
}

func inspectorOpts(th theme.Theme) inspector.Options {
	o := th.Inspector()
	o.Title, o.Filterable = "rec", true
	return o
}

func eventlogOpts(th theme.Theme) eventlog.Options {
	o := th.Eventlog()
	o.Title, o.Filterable, o.Searchable = "events", true, true
	return o
}

func sampleTree() tree.Node {
	leaf := func(l string) tree.Node { return node{label: l} }
	return node{label: "root", kids: []tree.Node{
		node{label: "a", kids: []tree.Node{leaf("a1"), leaf("a2")}},
		node{label: "b", kids: []tree.Node{leaf("b1")}},
		leaf("c"),
	}}
}

// A screen's first SetTheme takes State from a zero Model — nothing was
// built yet — and restores it onto one freshly built with Options that
// already carry data. That restore must change nothing.
func TestRestoringAZeroStateLeavesOptionsAlone(t *testing.T) {
	th := theme.Dark()

	lo := th.List()
	lo.Title, lo.Items = "l", lines(5, "item %d")
	var zl list.Model
	l := list.New(lo)
	l.SetRect(placed())
	before := l.View()
	l.Restore(zl.State())
	if l.View() != before {
		t.Error("list: a zero State wiped Options.Items")
	}

	to := tableOpts(th)
	to.Rows = []table.Row{{"a", "1"}, {"b", "2"}}
	var zt table.Model
	tb := table.New(to)
	tb.SetRect(placed())
	before = tb.View()
	tb.Restore(zt.State())
	if tb.View() != before {
		t.Error("table: a zero State wiped Options.Rows")
	}

	gOpts := th.Toggle()
	gOpts.Title, gOpts.Initial = "t", true
	var zg toggle.Model
	g := toggle.New(gOpts)
	g.Restore(zg.State())
	if !g.Value() {
		t.Error("toggle: a zero State overrode Options.Initial")
	}
}
