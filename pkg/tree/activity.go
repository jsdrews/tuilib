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

// DispatchEach opens one operation per key and runs request for each, so the
// server refusing one row stops that row alone. It is the per-row loop every
// verb over a selection needs: Dispatch([]string{key}, …) per key, the
// request's command per key, all batched with the spinner's first tick.
//
// request returns the command that performs the work for one key and replies
// with a message carrying op, which the screen hands to Done. Use Dispatch
// directly only when one request acts on several keys at once.
func (m *Model) DispatchEach(paths []string, label string, mode activity.Mode,
	request func(op activity.Op, key string) tea.Cmd) tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2*len(paths))
	for _, k := range paths {
		op, spin := m.Dispatch([]string{k}, label, mode)
		cmds = append(cmds, spin, request(op, k))
	}
	return tea.Batch(cmds...)
}

// Done reports that op's request answered, and returns what that means:
// activity.Acknowledged (an Observed request answered; the work is still
// running, so say "requested"), activity.Ended (a Held request answered; the
// work is over) or activity.Withdrawn (it failed; report the error). Switch on
// it when writing the reply's message, rather than assuming the work is done.
func (m *Model) Done(op activity.Op, err error) activity.Outcome {
	out := m.act.Done(op, err)
	m.refresh()
	return out
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
