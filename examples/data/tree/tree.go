// Package tree demonstrates pkg/tree as a single-screen app: a synthetic
// project layout you can expand/collapse, search ("/"), and filter ("\")
// down to matching subtrees.
package tree

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
	tw "github.com/jsdrews/tuilib/pkg/tree"
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
// exercise expand/collapse and search.
func sample() *node {
	return dir("tuilib",
		dir("pkg",
			dir("app",
				leaf("app.go"),
				leaf("app_test.go"),
			),
			dir("layout",
				leaf("layout.go"),
				leaf("layout_test.go"),
			),
			dir("list",
				leaf("list.go"),
			),
			dir("logview",
				leaf("logview.go"),
			),
			dir("pane",
				leaf("pane.go"),
				leaf("border.go"),
				leaf("scrollbar.go"),
			),
			dir("tree",
				leaf("tree.go"),
			),
			dir("theme",
				leaf("theme.go"),
				leaf("named.go"),
				leaf("base16.go"),
			),
		),
		dir("examples",
			dir("app",
				dir("focus", leaf("focus.go")),
				dir("layouts", leaf("layouts.go")),
				dir("stack", leaf("stack.go")),
			),
			dir("data",
				dir("form", leaf("form.go")),
				dir("list", leaf("list.go")),
				dir("logview", leaf("logview.go")),
				dir("runlog", leaf("runlog.go")),
				dir("runner", leaf("runner.go")),
				dir("table", leaf("table.go")),
				dir("tree", leaf("tree.go")),
			),
			dir("launcher", leaf("main.go")),
		),
		leaf("README.md"),
		leaf("CLAUDE.md"),
		leaf("Taskfile.yml"),
		leaf("go.mod"),
		leaf("go.sum"),
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
