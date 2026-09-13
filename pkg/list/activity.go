package list

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
)

// Row activity: a spinner and status label on the items something is currently
// working on. See pkg/activity for the state itself.
//
// It draws as a right-aligned badge at the end of the row —
//
//	worker-pool                              ⠹ syncing
//
// rather than as a prefix, because the leading cells are already spoken for by
// the cursor glyph and the mark column, and pushing every row sideways the
// moment a spinner appears is the reflow pkg/form avoids by drawing validation
// errors on the border. When the row text and the badge cannot both fit, the
// row gives way: the badge is the news.
//
// Unlike pkg/table there is nothing to opt into. A table needs to be told
// which column to draw in; a badge occupies nothing at all until something is
// running, so it costs an app that never uses it exactly nothing.
//
// Entries are held by key, so this works on SetKeyedItems and is naturally
// inert on anonymous items — an entry whose key matches no row is unreachable
// rather than approximate, which is marking's rule (rule 32) arrived at from
// the other direction.

// observe recomputes the derived layer from the items the list now holds, and
// reports any revision that changed while its row was not busy.
//
// Called from SetKeyedItems. The commands it produces cannot be returned (a
// setter has no return value), so they queue for the next Update, exactly as a
// pending SelectedChangedMsg does.
func (m *Model) observe() {
	if m.actWhen == nil && m.actRev == nil {
		return
	}
	var cmds []tea.Cmd

	if m.actWhen != nil {
		busy := map[string]string{}
		for i, key := range m.itemKeys {
			if key == "" || i >= len(m.items) {
				continue
			}
			if label, ok := m.actWhen(m.items[i]); ok {
				busy[key] = label
			}
		}
		// Unconditionally, including when nothing matches: an empty
		// observation is a real one, and the only thing that can end a
		// handoff (decision 19).
		cmds = append(cmds, m.act.Derive(busy))
	}

	if m.actRev != nil {
		changes := make(map[string]activity.Change, len(m.itemKeys))
		for i, key := range m.itemKeys {
			if key == "" || i >= len(m.items) {
				continue
			}
			// No label: a list row shows its item, not a status, so the glyph
			// alone is the whole message — "this one changed".
			changes[key] = activity.Change{Rev: m.actRev(m.items[i])}
		}
		cmds = append(cmds, m.act.Revise(changes))
	}

	m.actCmd = tea.Batch(append(cmds, m.actCmd)...)
}

// flushActivity hands over any command observe queued.
func (m *Model) flushActivity() tea.Cmd {
	cmd := m.actCmd
	m.actCmd = nil
	return cmd
}

// holdsKey reports whether this list currently shows an item with that key.
//
// It filters the shell's broadcasts: a component declines keys it does not
// hold, the same way it declines a mouse event outside its rect. Direct calls
// to SetActivity are not filtered — there the screen is asserting it knows
// what it is doing, and the row may still be in flight from a fetch.
func (m Model) holdsKey(k string) bool {
	if k == "" {
		return false
	}
	for _, ik := range m.itemKeys {
		if ik == k {
			return true
		}
	}
	return false
}

// withBadge appends the indicator for visible row i to an already-rendered row.
//
// Composed outside the row's own styling rather than inside it: the cursor row
// is one styled run, and a lipgloss-rendered badge nested in it would close
// that run at its first reset (rule 19).
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

// SetActivity starts the spinner on one item, replacing anything already there.
//
// Batch the returned command into your screen's command stream — it is the
// animation's first tick, exactly as SetLoading's is (rule 17).
func (m *Model) SetActivity(key, label string) tea.Cmd {
	cmd := m.act.Start(key, label)
	m.refresh()
	return cmd
}

// EndActivity finishes one item, showing ✓ or ✗ for the hold before it clears.
// A nil err is a success.
func (m *Model) EndActivity(key string, err error) tea.Cmd {
	cmd := m.act.Finish(key, err)
	m.refresh()
	return cmd
}

// ClearActivity retires one item's indicator at once, outcome or not.
func (m *Model) ClearActivity(key string) {
	m.act.Clear(key)
	m.refresh()
}

// Relabel changes what a busy item says without restarting it.
func (m *Model) Relabel(key, label string) {
	m.act.Relabel(key, label)
	m.refresh()
}

// ActivityState is the in-flight set, for carrying across a SetTheme rebuild.
// See SetActivityState.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this list's own palette — the rule-4 pair for row activity.
//
// The returned command re-arms the spinner and any hold that was mid-flight;
// dropping it strands a frozen glyph on the row.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many items are still working, held outcomes excluded.
func (m Model) ActivityCount() int { return m.act.Count() }
