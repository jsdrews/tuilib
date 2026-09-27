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
//
// Every node, not only the visible ones: a node inside a collapsed branch is
// still working, and expanding it should reveal a spinner already turning
// rather than start one.
func (m *Model) observe() {
	var paths []string
	var nodes []Node
	if m.root != nil {
		m.walkNodes(m.root, rootPath(m.root), func(path string, n Node) {
			paths = append(paths, path)
			nodes = append(nodes, n)
		})
	}
	var eval func(int) (string, bool, string)
	if m.busyWhen != nil {
		eval = func(i int) (string, bool, string) {
			label, busy := m.busyWhen(nodes[i])
			rev := ""
			if m.revision != nil {
				rev = m.revision(nodes[i])
			}
			return label, busy, rev
		}
	}
	m.actCmd = tea.Batch(m.act.ObserveRows(paths, eval), m.actCmd)
}

// Dispatch opens a claim on paths for work the screen is about to request, and
// returns the operation to carry to Done. mode says who knows when the work
// ends: activity.Observed (the server's status) or activity.Held (the request,
// or a job it returned).
func (m *Model) Dispatch(paths []string, label string, mode activity.Mode) (activity.Op, tea.Cmd) {
	op, cmd := m.act.Dispatch(paths, label, mode)
	m.refresh()
	return op, cmd
}

// Done reports that op's request answered. An error withdraws the claim; nil
// ends a Held operation and acknowledges an Observed one.
func (m *Model) Done(op activity.Op, err error) {
	m.act.Done(op, err)
	m.refresh()
}

// BeginRead stamps a fetch at the moment it is issued; hand the token to
// ApplyRead with the reply. A stream needs none: push events with SetRoot.
func (m *Model) BeginRead() activity.Read { return m.act.BeginRead() }

// ApplyRead is SetRoot for a fetched reply, and reports whether it applied it.
// False means dropped: overtaken by a newer reply, or failed — in which case
// operations waiting on a read are withdrawn and nodes busy on the last good
// read turn Unknown until one succeeds.
func (m *Model) ApplyRead(rd activity.Read, root Node, err error) bool {
	if !m.act.Accept(rd, err) {
		m.refresh()
		return false
	}
	m.SetRoot(root)
	return true
}

// ReadsFailing reports whether the newest fetch handed to ApplyRead failed and
// none has succeeded since.
func (m Model) ReadsFailing() bool { return m.act.ReadsFailing() }

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
//
// The returned command is the spinner's first tick. SetTheme has nowhere to
// return it, and dropping it is safe: the tree re-arms a stalled spinner on
// the next message it receives.
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
// last observation, or dispatched and not yet spoken to.
func (m Model) ActivityCount() int { return m.act.Count() }
