package tree

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
)

// Node activity: a spinner and status label on the nodes the data says are
// working. See pkg/activity for the state itself.
//
// It draws as a right-aligned badge at the end of the node's row, the way
// pkg/list does and for the same reason: the leading cells are spoken for by
// the indent, the expand glyph and the mark column.
//
// A tree needs no keyed setter, because a node's path already is its identity
// — the same path used for expansion state and cursor restore.

// observe recomputes the indicator set from the tree the model now holds.
//
// Called from New and SetRoot. New matters as much as SetRoot: a tree takes
// its data through Options.Root, so a tree that never calls SetRoot would
// otherwise never have observed anything at all.
func (m *Model) observe() {
	if m.actWhen == nil {
		return
	}
	busy := map[string]string{}
	if m.root != nil {
		// Every node, not only the visible ones: a node inside a collapsed
		// branch is still working, and expanding it should reveal a spinner
		// already turning rather than start one.
		m.walkNodes(m.root, rootPath(m.root), func(path string, n Node) {
			if label, ok := m.actWhen(n); ok {
				busy[path] = label
			}
		})
	}
	// Unconditionally, including when nothing matches: an empty observation is
	// a real one, and the only thing that can stop the last spinner.
	m.actCmd = tea.Batch(m.act.Derive(busy), m.actCmd)
}

// flushActivity hands over any command observe queued.
func (m *Model) flushActivity() tea.Cmd {
	cmd := m.actCmd
	m.actCmd = nil
	return cmd
}

// withBadge appends the indicator to a rendered row, if its path is busy.
func (m Model) withBadge(path, row string) string {
	if !m.act.Active() {
		return row
	}
	w := m.body.ContentRect().W
	if w <= 0 {
		return row
	}
	return m.act.Badge(path, row, w)
}

// ActivityState is the observed set, for carrying across a SetTheme rebuild.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this tree's own palette — the rule-4 pair for row activity.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// walkNodes visits every node with the path the rest of the tree addresses it
// by, expanded or not — the same numbering collectAllPaths and allPaths use,
// so an activity key and a mark key for one node are the same string.
func (m *Model) walkNodes(n Node, path string, fn func(string, Node)) {
	fn(path, n)
	seen := map[string]int{}
	for _, c := range n.Children() {
		label := c.Label()
		seen[label]++
		m.walkNodes(c, childPath(path, label, seen[label]), fn)
	}
}

// ActivityCount is how many nodes the last observation reported working.
func (m Model) ActivityCount() int { return m.act.Count() }
