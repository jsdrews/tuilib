// Package tree demonstrates pkg/tree as a single-screen app: a synthetic
// project layout you can expand/collapse, search ("/"), and filter ("\")
// down to matching subtrees.
package tree

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
	tw "github.com/jsdrews/tuilib/pkg/tree"
)

var (
	iconOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
	iconWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("●")
	iconErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("●")
)

// node is the example's data shape — a label plus children. It satisfies
// tree.Node so it can be handed straight to tw.New.
type node struct {
	label string
	kids  []*node
}

func (n *node) Label() string    { return n.label }
func (n *node) Children() []tw.Node {
	out := make([]tw.Node, len(n.kids))
	for i, k := range n.kids {
		out[i] = k
	}
	return out
}

func leaf(label string) *node { return &node{label: label} }
func dir(label string, kids ...*node) *node {
	return &node{label: label, kids: kids}
}

// sample returns a synthetic project tree with enough depth and breadth to
// exercise expand/collapse and search. A handful of leaves carry colored
// status icons (lipgloss-rendered ANSI in Label) so the cursor's row-level
// background highlight can be visually verified across colored segments.
func sample() *node {
	return dir("tuilib",
		dir("pkg",
			dir("app",
				leaf(iconOK+" app.go"),
				leaf(iconOK+" app_test.go"),
			),
			dir("layout",
				leaf(iconOK+" layout.go"),
				leaf(iconOK+" layout_test.go"),
			),
			dir("list",
				leaf(iconOK+" list.go"),
			),
			dir("logview",
				leaf(iconWarn+" logview.go"),
			),
			dir("pane",
				leaf(iconOK+" pane.go"),
				leaf(iconOK+" border.go"),
				leaf(iconErr+" scrollbar.go"),
			),
			dir("tree",
				leaf(iconOK+" tree.go"),
			),
			dir("theme",
				leaf(iconOK+" theme.go"),
				leaf(iconOK+" named.go"),
				leaf(iconWarn+" base16.go"),
			),
		),
		dir("examples",
			dir("app",
				dir("focus", leaf(iconOK+" focus.go")),
				dir("layouts", leaf(iconOK+" layouts.go")),
				dir("stack", leaf(iconOK+" stack.go")),
			),
			dir("data",
				dir("form", leaf(iconOK+" form.go")),
				dir("list", leaf(iconOK+" list.go")),
				dir("logview", leaf(iconOK+" logview.go")),
				dir("runlog", leaf(iconOK+" runlog.go")),
				dir("runner", leaf(iconOK+" runner.go")),
				dir("table", leaf(iconWarn+" table.go")),
				dir("tree", leaf(iconOK+" tree.go")),
			),
			dir("launcher", leaf(iconOK+" main.go")),
		),
		leaf(iconOK+" README.md"),
		leaf(iconOK+" CLAUDE.md"),
		leaf(iconOK+" Taskfile.yml"),
		leaf(iconOK+" go.mod"),
		leaf(iconOK+" go.sum"),
	)
}

// New returns the tree demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{}
	s.SetTheme(t)
	return s
}

type Screen struct {
	t    theme.Theme
	tree tw.Model
}

func (s *Screen) Title() string         { return "Tree" }
func (s *Screen) Init() tea.Cmd         { return textinput.Blink }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.tree.Searching() }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tree, cmd = s.tree.Update(msg)
	return s, cmd
}

func (s *Screen) Layout() layout.Node { return layout.Sized(&s.tree) }

func (s *Screen) Help() []key.Binding {
	base := []key.Binding{
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	}
	return append(base, s.tree.Help()...)
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t
	cursor, query := s.tree.Cursor(), s.tree.Query()
	opts := t.Tree()
	opts.Title = "tuilib (synthetic)"
	opts.Root = sample()
	opts.Searchable = true
	opts.InitialDepth = 2
	opts.Filter.Placeholder = "search nodes…"
	s.tree = tw.New(opts)
	if query != "" {
		s.tree.SetQuery(query)
	}
	s.tree.SetCursor(cursor)
}
