package tree

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
)

// Node activity: a spinner and status label on the nodes something is
// currently working on. See pkg/activity for the state itself.
//
// It draws as a right-aligned badge at the end of the row, for the reason
// pkg/list gives: the leading cells are the mark gutter, the indent and the
// disclosure arrow, and a prefix that appeared mid-run would shove every one
// of them sideways.
//
// Keys here are node paths — the same identity the tree already uses for
// expansion state, marking and cursor restore, so nothing new has to be
// supplied. Activity is on a node, not a subtree: a busy branch says so about
// itself and nothing about its children, exactly as marking does (rule 32).

// observe recomputes the derived layer from the tree the model now holds, and
// reports any revision that changed while its node was not busy.
//
// Called from SetRoot, and unconditionally rather than only for visible rows:
// derived state is a fact about the data, and a node inside a collapsed branch
// is still working. Expanding it should reveal a spinner already turning, not
// start one.
//
// The commands it produces cannot be returned (a setter has no return value),
// so they queue for the next Update.
func (m *Model) observe() {
	if m.actWhen == nil && m.actRev == nil {
		return
	}
	busy := map[string]string{}
	changes := map[string]activity.Change{}
	if m.root != nil {
		m.walkNodes(m.root, rootPath(m.root), func(path string, n Node) {
			if m.actWhen != nil {
				if label, ok := m.actWhen(n); ok {
					busy[path] = label
				}
			}
			if m.actRev != nil {
				// No label: a tree row shows its node, not a status, so the
				// glyph alone is the whole message.
				changes[path] = activity.Change{Rev: m.actRev(n)}
			}
		})
	}

	var cmds []tea.Cmd
	if m.actWhen != nil {
		// Unconditionally, including when nothing matches: an empty
		// observation is a real one, and the only thing that can end a
		// handoff (decision 19).
		cmds = append(cmds, m.act.Derive(busy))
	}
	if m.actRev != nil {
		cmds = append(cmds, m.act.Revise(changes))
	}
	m.actCmd = tea.Batch(append(cmds, m.actCmd)...)
}

// walkNodes visits every node with the path the rest of the tree addresses it
// by, expanded or not — the same numbering collectAllPaths and allPaths use,
// so a derived key and a mark key for one node are the same string.
func (m *Model) walkNodes(n Node, path string, fn func(string, Node)) {
	fn(path, n)
	seen := map[string]int{}
	for _, c := range n.Children() {
		label := c.Label()
		seen[label]++
		m.walkNodes(c, childPath(path, label, seen[label]), fn)
	}
}

// flushActivity hands over any command observe queued.
func (m *Model) flushActivity() tea.Cmd {
	cmd := m.actCmd
	m.actCmd = nil
	return cmd
}

// holdsKey reports whether this tree currently shows a node at that path.
//
// It filters the shell's broadcasts. Only flattened rows count, so a node
// inside a collapsed branch declines — it is not on screen to spin.
func (m Model) holdsKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range m.rows {
		if r.path == k {
			return true
		}
	}
	return false
}

// withBadge appends the indicator for row r to an already-rendered row.
//
// Composed outside the row's own styling: the current row is one styled run
// padded to the pane's inner width, and a lipgloss-rendered badge nested in it
// would close that run at its first reset (rule 19). Badge cuts the padding
// back down to make room, so the row keeps its width either way.
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

// SetActivity starts the spinner on one node path, replacing anything already
// there.
//
// Batch the returned command into your screen's command stream — it is the
// animation's first tick, exactly as SetLoading's is (rule 17).
func (m *Model) SetActivity(path, label string) tea.Cmd {
	cmd := m.act.Start(path, label)
	m.refresh()
	return cmd
}

// EndActivity finishes one node, showing ✓ or ✗ for the hold before it clears.
// A nil err is a success.
func (m *Model) EndActivity(path string, err error) tea.Cmd {
	cmd := m.act.Finish(path, err)
	m.refresh()
	return cmd
}

// ClearActivity retires one node's indicator at once, outcome or not.
func (m *Model) ClearActivity(path string) {
	m.act.Clear(path)
	m.refresh()
}

// Relabel changes what a busy node says without restarting it.
func (m *Model) Relabel(path, label string) {
	m.act.Relabel(path, label)
	m.refresh()
}

// ActivityState is the in-flight set, for carrying across a SetTheme rebuild.
// See SetActivityState.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this tree's own palette — the rule-4 pair for node activity.
//
// The returned command re-arms the spinner and any hold that was mid-flight;
// dropping it strands a frozen glyph on the row.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many nodes are still working, held outcomes excluded.
func (m Model) ActivityCount() int { return m.act.Count() }
