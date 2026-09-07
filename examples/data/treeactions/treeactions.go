// Package treeactions demonstrates marking and pkg/action on a tree: a
// cluster-shaped resource hierarchy where you mark nodes at any depth and run
// verbs over the selection.
//
// Press x to mark a node, X to mark a range, A to mark everything visible, D
// to drop the selection. Then "a" or right-click for the menu.
//
// # The thing this screen exists to show
//
// A tree's mark key is its node **path** ("prod/api-server/pod-7f3a"), and
// marking a branch marks that branch alone — not its descendants. That is a
// deliberate choice (CLAUDE.md rule 32): it keeps Marks() equal to "what the
// user pressed x on", and it leaves the expansion question to the caller,
// because only the caller knows whether its verb is one that cascades.
//
// Resolving a marked branch to the things a verb should actually touch is
// therefore the screen's job, and it is four lines — see expand() below. The
// paths are hierarchical strings, so "everything under this node" is a prefix
// test and nothing more. Restart cascades that way; Describe does not, because
// describing a namespace is not describing its pods.
//
// # Why the menu title matters more here than in a flat list
//
// Marks survive collapsing. Mark a pod, collapse its namespace, and the pod is
// still selected while nothing on screen says so — the same property that lets
// marks survive a filter, one level more surprising. Set.Target is the
// disclosure: the menu names what it is about to touch, counted after expand(),
// so "Restart 9 pods" is honest even when three of them are hidden inside a
// folded branch.
package treeactions

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/help"
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

// node satisfies tw.Node. kind is the example's own field — the tree neither
// knows nor needs it, which is the point of Node being an interface over your
// data rather than a struct to convert into.
type node struct {
	label string
	kind  string // "namespace", "workload", "pod"
	kids  []*node
}

func (n *node) Label() string { return n.label }
func (n *node) Children() []tw.Node {
	out := make([]tw.Node, len(n.kids))
	for i, k := range n.kids {
		out[i] = k
	}
	return out
}

func pod(label string) *node { return &node{label: label, kind: "pod"} }
func workload(label string, kids ...*node) *node {
	return &node{label: label, kind: "workload", kids: kids}
}
func namespace(label string, kids ...*node) *node {
	return &node{label: label, kind: "namespace", kids: kids}
}

func cluster() *node {
	return &node{label: "cluster", kind: "root", kids: []*node{
		namespace("prod",
			workload("api-server",
				pod(iconOK+" api-server-7f3a"),
				pod(iconOK+" api-server-9b21"),
				pod(iconErr+" api-server-2c88"),
			),
			workload("web-frontend",
				pod(iconOK+" web-frontend-4de1"),
				pod(iconOK+" web-frontend-6a07"),
			),
			workload("cache-redis",
				pod(iconWarn+" cache-redis-0f5b"),
			),
		),
		namespace("staging",
			workload("api-server",
				pod(iconOK+" api-server-1a44"),
			),
			workload("batch-runner",
				pod(iconOK+" batch-runner-8e12"),
				pod(iconWarn+" batch-runner-3d90"),
			),
		),
	}}
}

// Screen is a tree plus an Actions method. Nothing else about it is special.
type Screen struct {
	t    theme.Theme
	tree tw.Model
	root *node
}

// New returns the tree-actions demo screen.
func New(t theme.Theme) screen.Screen {
	s := &Screen{root: cluster()}
	s.SetTheme(t)
	return s
}

func (s *Screen) Title() string         { return "Tree actions" }
func (s *Screen) Init() tea.Cmd         { return nil }
func (s *Screen) OnEnter(any) tea.Cmd   { return nil }
func (s *Screen) IsCapturingKeys() bool { return s.tree.IsCapturingKeys() }
func (s *Screen) Layout() layout.Node   { return layout.Sized(&s.tree) }

func (s *Screen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	s.tree, cmd = s.tree.Update(msg)
	s.refreshTitle()
	return s, cmd
}

func (s *Screen) Help() []key.Binding { return help.Flatten(s.HelpSections()) }

// HelpSections passes the tree's own groups through — Navigate, Expand,
// Select, Search — and adds this screen's verbs under its own heading.
func (s *Screen) HelpSections() []help.Section {
	return help.SectionsOf(&s.tree, help.Group("Tree actions",
		key.NewBinding(key.WithKeys("mouse:right"), key.WithHelp("right-click", "actions")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "theme")),
	))
}

func (s *Screen) SetTheme(t theme.Theme) {
	s.t = t

	cursor, query := s.tree.Cursor(), s.tree.Query()
	filterMode := s.tree.FilterMode()
	marks := s.tree.Marks()

	opts := t.Tree()
	opts.Title = "cluster"
	opts.Root = s.root
	opts.Searchable = true
	opts.Markable = true
	opts.InitialDepth = 3
	opts.Filter.Placeholder = "search pods…"
	s.tree = tw.New(opts)

	if query != "" {
		s.tree.SetQuery(query)
		s.tree.SetFilterMode(filterMode)
	}
	s.tree.SetCursor(cursor)
	s.tree.SetMarks(marks)
	s.refreshTitle()
}

// refreshTitle keeps the count on the border, which is what makes a mark
// hidden inside a collapsed branch visible at all.
func (s *Screen) refreshTitle() {
	if n := s.tree.MarkCount(); n > 0 {
		s.tree.SetTitle("cluster · " + strconv.Itoa(n) + " marked")
		return
	}
	s.tree.SetTitle("cluster")
}

// expand resolves a selection of paths to the pods a cascading verb should
// touch. A marked branch contributes its whole subtree; a marked pod
// contributes itself.
//
// This is the whole "does marking a branch mark its children" question, and it
// lives here rather than in pkg/tree on purpose: the answer differs per verb.
// Restart cascades. Describe does not.
func (s *Screen) expand(sel []string) []string {
	seen := map[string]bool{}
	for _, p := range sel {
		for _, leaf := range s.podsUnder(p) {
			seen[leaf] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// podsUnder returns every pod path at or below p. Paths are hierarchical
// strings, so "below" is a prefix test.
func (s *Screen) podsUnder(p string) []string {
	var out []string
	for _, cand := range s.allPodPaths() {
		if cand == p || strings.HasPrefix(cand, p+"/") {
			out = append(out, cand)
		}
	}
	return out
}

func (s *Screen) allPodPaths() []string {
	var out []string
	var walk func(n *node, path string)
	walk = func(n *node, path string) {
		if n.kind == "pod" {
			out = append(out, path)
			return
		}
		for _, k := range n.kids {
			walk(k, path+"/"+k.label)
		}
	}
	walk(s.root, s.root.label)
	return out
}

// Actions satisfies action.Provider.
func (s *Screen) Actions() action.Set {
	sel := s.tree.Selection()
	if len(sel) == 0 {
		return action.Set{}
	}

	// Count what the cascading verbs will actually touch, not how many rows
	// were marked — "Restart 9 pods" after marking one namespace.
	pods := s.expand(sel)
	label := cascadeLabel(pods)

	return action.Set{
		Target: label,
		Count:  len(pods),
		Actions: []action.Action{
			{
				Label:     "Restart",
				Desc:      "roll every pod in the selection",
				Key:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
				Multi:     true,
				Exclusive: true,
				Run:       rollout(pods),
			},
			{
				Label: "Cordon",
				Desc:  "stop scheduling here",
				Multi: true,
				Run:   cordon(pods),
			},
			{
				// Describe deliberately does not cascade: it reports on the
				// node the user marked, whatever kind it is. So it reads
				// sel, not pods — and without Multi the menu disables it as
				// soon as there is more than one.
				Label: "Describe",
				Desc:  "the marked node itself",
				Run:   describe(sel),
			},
			{
				Label:    "Delete",
				Multi:    true,
				Confirm:  "Delete " + label + "? This cannot be undone.",
				Disabled: s.deleteGuard(sel),
				Run:      remove(pods),
			},
			{
				Label: "Drop marks",
				Desc:  "clear the selection",
				Multi: true,
				Do: func() tea.Cmd {
					s.tree.ClearMarks()
					s.refreshTitle()
					return nil
				},
			},
		},
	}
}

// deleteGuard refuses a selection that reaches outside staging — the kind of
// author-supplied rule that belongs on the action rather than inside a Run
// that has already been confirmed.
func (s *Screen) deleteGuard(sel []string) string {
	for _, p := range sel {
		if !strings.HasPrefix(p, "cluster/staging") {
			return "only staging resources can be deleted in this demo"
		}
	}
	return ""
}

func cascadeLabel(pods []string) string {
	switch len(pods) {
	case 0:
		return ""
	case 1:
		return short(pods[0])
	default:
		return strconv.Itoa(len(pods)) + " pods"
	}
}

// short trims a path to its last segment for display. The path is the
// identity; the leaf name is what reads well in a sentence.
func short(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func rollout(pods []string) action.Func {
	return func(ctx context.Context, out io.Writer) error {
		for _, p := range pods {
			fmt.Fprintf(out, "restarting %s\n", short(p))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(400 * time.Millisecond):
			}
		}
		fmt.Fprintf(out, "rolled out %d pod(s)\n", len(pods))
		return nil
	}
}

func cordon(pods []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, p := range pods {
			fmt.Fprintf(out, "cordoned %s\n", short(p))
		}
		return nil
	}
}

func describe(sel []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		p := sel[0]
		fmt.Fprintf(out, "Path:   %s\nName:   %s\nStatus: Running\n", p, short(p))
		return nil
	}
}

func remove(pods []string) action.Func {
	return func(_ context.Context, out io.Writer) error {
		for _, p := range pods {
			fmt.Fprintf(out, "deleted pod/%s\n", short(p))
		}
		return fmt.Errorf("refusing to delete in a demo")
	}
}
