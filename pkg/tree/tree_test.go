package tree

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// testNode is a minimal in-memory Node used to drive SetRoot tests.
type testNode struct {
	label    string
	children []Node
}

func (t *testNode) Label() string  { return t.label }
func (t *testNode) Children() []Node { return t.children }

func n(label string, kids ...Node) *testNode {
	return &testNode{label: label, children: kids}
}

func newTree(root Node) Model {
	return New(Options{
		Width:            40,
		Height:           20,
		Root:             root,
		InitialDepth:     1,
		MatchStyle:       lipgloss.NewStyle(),
		CurrentLineStyle: lipgloss.NewStyle(),
	})
}

// cursorPath returns the path of the row under the cursor. Empty string
// when there's no visible row.
func cursorPath(m Model) string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rows[m.cursor].path
}

func rowPaths(m Model) []string {
	out := make([]string, len(m.rows))
	for i, r := range m.rows {
		out[i] = r.path
	}
	return out
}

func TestChildPathCollisionSuffix(t *testing.T) {
	if got, want := childPath("root", "foo", 1), "root/foo"; got != want {
		t.Errorf("first occurrence = %q, want %q", got, want)
	}
	if got, want := childPath("root", "foo", 2), "root/foo:2"; got != want {
		t.Errorf("second occurrence = %q, want %q", got, want)
	}
	if got, want := childPath("root", "foo", 3), "root/foo:3"; got != want {
		t.Errorf("third occurrence = %q, want %q", got, want)
	}
}

func TestPathsAreLabelBased(t *testing.T) {
	// root has 2 kids "a" and "b"; "a" has 1 kid "leaf".
	m := newTree(n("root", n("a", n("leaf")), n("b")))
	paths := rowPaths(m)
	// InitialDepth=1 expands root, so root/a and root/b are visible; "leaf"
	// is under a collapsed "a" (a's kids only visible if a is expanded).
	want := []string{"root", "root/a", "root/b"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestSiblingLabelCollisionUsesSuffix(t *testing.T) {
	// Two sibling nodes labeled "dup" — second should get :2.
	m := newTree(n("root", n("dup"), n("dup"), n("dup")))
	paths := rowPaths(m)
	want := []string{"root", "root/dup", "root/dup:2", "root/dup:3"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestSetRootPreservesExpandedState(t *testing.T) {
	// Expand root/a, then swap root — a should remain expanded so its
	// child "leaf" is still visible.
	m := newTree(n("root", n("a", n("leaf")), n("b")))
	m.expanded["root/a"] = true
	m.refresh()
	if got := len(m.rows); got != 4 {
		t.Fatalf("pre-swap row count = %d, want 4", got)
	}
	m.SetRoot(n("root", n("a", n("leaf")), n("b")))
	paths := rowPaths(m)
	if len(paths) != 4 {
		t.Fatalf("post-swap paths = %v, want 4 rows", paths)
	}
	if paths[2] != "root/a/leaf" {
		t.Errorf("leaf missing after SetRoot: paths=%v", paths)
	}
}

func TestSetRootPinsCursorToSameNode(t *testing.T) {
	m := newTree(n("root", n("a"), n("b"), n("c")))
	m.SetCursor(2) // "b"
	if got := cursorPath(m); got != "root/b" {
		t.Fatalf("pre-swap cursorPath = %q, want root/b", got)
	}
	m.SetRoot(n("root", n("a"), n("b"), n("c")))
	if got := cursorPath(m); got != "root/b" {
		t.Errorf("post-swap cursorPath = %q, want root/b", got)
	}
}

func TestSetRootCursorFallsBackToAncestor(t *testing.T) {
	// Cursor is on root/a/leaf; new tree drops "leaf" and "a" but keeps root.
	m := newTree(n("root", n("a", n("leaf"))))
	m.expanded["root/a"] = true
	m.refresh()
	// Move cursor onto the leaf.
	for i, r := range m.rows {
		if r.path == "root/a/leaf" {
			m.SetCursor(i)
			break
		}
	}
	if got := cursorPath(m); got != "root/a/leaf" {
		t.Fatalf("pre-swap cursorPath = %q, want root/a/leaf", got)
	}
	// New tree removes "a" entirely — nearest surviving ancestor is root.
	m.SetRoot(n("root", n("b")))
	if got := cursorPath(m); got != "root" {
		t.Errorf("post-swap cursorPath = %q, want root", got)
	}
}

func TestSetRootPrunesDeadExpandedEntries(t *testing.T) {
	m := newTree(n("root", n("a", n("leaf"))))
	m.expanded["root/a"] = true
	m.expanded["root/a/leaf"] = true
	m.refresh()
	// New tree drops the "a" subtree entirely.
	m.SetRoot(n("root", n("b")))
	if _, ok := m.expanded["root/a"]; ok {
		t.Error("expanded['root/a'] should be pruned")
	}
	if _, ok := m.expanded["root/a/leaf"]; ok {
		t.Error("expanded['root/a/leaf'] should be pruned")
	}
}

func TestSetRootNilClearsState(t *testing.T) {
	m := newTree(n("root", n("a")))
	m.expanded["root/a"] = true
	m.refresh()
	m.SetRoot(nil)
	if len(m.expanded) != 0 {
		t.Errorf("expanded map should be empty after nil root, got %v", m.expanded)
	}
	if len(m.rows) != 0 {
		t.Errorf("rows should be empty after nil root, got %d", len(m.rows))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestSetRootNewNodesStartCollapsed(t *testing.T) {
	// Swap in a tree where "new" is a brand-new path not present before —
	// it should render collapsed even though its parent is expanded.
	m := newTree(n("root", n("a")))
	m.expanded["root/a"] = true
	m.refresh()
	m.SetRoot(n("root", n("a", n("new-child"))))
	// root and root/a should be visible (a is expanded), and root/a/new-child
	// is a leaf → visible.
	paths := rowPaths(m)
	if len(paths) != 3 {
		t.Fatalf("paths = %v, want 3 rows", paths)
	}
	if paths[2] != "root/a/new-child" {
		t.Errorf("expected new-child leaf visible under expanded a; paths=%v", paths)
	}
	// But if the "new" node were an interior with kids, the new node itself
	// should render collapsed by default (verified separately below).
}

func TestSetRootNewInteriorNodeIsCollapsedByDefault(t *testing.T) {
	m := newTree(n("root", n("a")))
	m.expanded["root/a"] = true
	m.refresh()
	// New tree adds a subtree under an existing expanded parent.
	m.SetRoot(n("root", n("a", n("subdir", n("file")))))
	paths := rowPaths(m)
	// root, root/a (expanded), root/a/subdir (collapsed — no expand entry)
	want := []string{"root", "root/a", "root/a/subdir"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestExpandAllUsesLabelPaths(t *testing.T) {
	m := newTree(n("root", n("a", n("leaf"))))
	m.expandAll()
	paths := rowPaths(m)
	want := []string{"root", "root/a", "root/a/leaf"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}
