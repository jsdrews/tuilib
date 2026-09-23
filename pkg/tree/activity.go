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
	var paths []string
	busy := map[string]string{}
	if m.root != nil {
		// Every node, not only the visible ones: a node inside a collapsed
		// branch is still working, and expanding it should reveal a spinner
		// already turning rather than start one.
		m.walkNodes(m.root, rootPath(m.root), func(path string, n Node) {
			paths = append(paths, path)
			if m.actWhen == nil {
				return
			}
			if label, ok := m.actWhen(n); ok {
				busy[path] = label
			}
		})
	}
	// Scoping happens on every swap, predicate or not: it is what keeps a
	// SetBusy map — which comes from somewhere other than this tree — from
	// animating a path the tree does not hold.
	m.act.Scope(paths)
	if m.actWhen == nil {
		return
	}
	// Unconditionally, including when nothing matches: an empty observation is
	// a real one, and the only thing that can stop the last spinner.
	// Values, deliberately nil: a tree has no value to compare.
	//
	// The other two components hand Observe each claimed key's current cell,
	// so an observation can notice the server acted by seeing that value
	// change. A node's label is the only string a tree can read generically,
	// and a node's label is its identity — the path expansion state, cursor
	// restore, marking and activity all key on is built from it. A label that
	// changed is not the same node reporting something new; it is a different
	// node. So that signal cannot exist here, and a claim on a tree ends by
	// being confirmed or by spending its allowance (activity.Options.Settle).
	m.actCmd = tea.Batch(m.act.Observe(busy, nil), m.actCmd)
}

// SetBusy is the second entrance: the screen says which nodes are working
// instead of a predicate reading it off the node.
//
// For busy-ness that is not a property of the node — an operations API, a job
// status resource. The map replaces the previous one outright, exactly as a
// predicate's observation does. Paths the tree does not hold are kept but not
// drawn.
//
// Panics if the tree was built with Options.ActivityWhen: a component uses one
// entrance or the other, and a screen needing both merges them itself.
func (m *Model) SetBusy(busy map[string]string) tea.Cmd {
	if m.actWhen != nil {
		panic("tree.SetBusy: built with Options.ActivityWhen; use one entrance or the other")
	}
	cmd := m.act.Observe(busy, nil)
	m.refresh()
	return cmd
}

// Expect marks paths as working because the screen has just asked the server to
// work on them, before any observation can say so.
//
// The claim is retired by an observation reporting the node busy, or by
// spending activity.Options.Settle. Unlike list and table there is no third
// ending here: a tree has no value to watch for change, because a node's label
// is its identity rather than a field on it (see observe). A tree backed by a
// reconciler therefore wants a slightly larger allowance than the same data in
// a table would.
func (m *Model) Expect(paths []string, label string) tea.Cmd {
	cmd := m.act.Expect(paths, label, nil)
	m.refresh()
	return cmd
}

// Retract drops the claims on paths, for a write the server refused.
func (m *Model) Retract(paths ...string) {
	m.act.Retract(paths...)
	m.refresh()
}

// RetractAll drops every claim, for a read that failed — an outage is the case
// where no observation is coming to retire them.
func (m *Model) RetractAll() {
	m.act.RetractAll()
	m.refresh()
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

// ActivityCount is how many of this tree's nodes are working — reported by the
// last observation, or claimed by Expect and not yet spoken to.
func (m Model) ActivityCount() int { return m.act.Count() }
