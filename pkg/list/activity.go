package list

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
)

// Row activity: a spinner and status label on the items the data says are
// working. See pkg/activity for the state itself.
//
// It draws as a right-aligned badge at the end of the row —
//
//	worker-pool                              ⣾ running
//
// rather than as a prefix, because the leading cells are already spoken for by
// the cursor glyph and the mark column, and pushing every row sideways the
// moment a spinner appears is the reflow pkg/form avoids by drawing validation
// errors on the border. When the row text and the badge cannot both fit, the
// row gives way: the badge is the news.
//
// Unlike pkg/table there is nothing to opt into beyond the predicate. A table
// needs to be told which column to draw in; a badge occupies nothing at all
// until something is running, so it costs an app that never uses it exactly
// nothing.
//
// Entries are held by key, so this works on SetKeyedItems and is naturally
// inert on anonymous items — an entry whose key matches no row is unreachable
// rather than approximate, which is marking's rule (rule 32) arrived at from
// the other direction.

// observe recomputes the indicator set from the items the list now holds.
//
// Called from SetKeyedItems. The command it produces cannot be returned (a
// setter has no return value), so it queues for the next Update, exactly as a
// pending SelectedChangedMsg does.
func (m *Model) observe() {
	if m.actWhen == nil {
		return
	}
	busy := map[string]string{}
	for i, key := range m.itemKeys {
		if key == "" || i >= len(m.items) {
			continue
		}
		if label, ok := m.actWhen(m.items[i]); ok {
			busy[key] = label
		}
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

// ActivityState is the observed set, for carrying across a SetTheme rebuild.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this list's own palette — the rule-4 pair for row activity.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many rows the last observation reported working.
func (m Model) ActivityCount() int { return m.act.Count() }

// withBadge appends the indicator to a rendered row, if its key is busy.
func (m Model) withBadge(i int, row string) string {
	if !m.act.Active() {
		return row
	}
	k, ok := m.keyAt(i)
	if !ok {
		return row
	}
	w := m.body.ContentRect().W
	if w <= 0 {
		return row
	}
	return m.act.Badge(k, row, w)
}
