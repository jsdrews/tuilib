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
	// Scoping happens on every swap, predicate or not: it is what keeps a
	// SetBusy map — which comes from somewhere other than these items — from
	// animating a key this list does not hold.
	m.act.Scope(m.itemKeys)
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
	m.actCmd = tea.Batch(m.act.Observe(busy, m.actValues(m.act.Expecting())), m.actCmd)
}

// actValues is the current text of every key in keys, which is how an
// observation notices the server acted (activity.Options.Settle).
//
// Only meaningful on the predicate path — the predicate reads this text, so a
// change in it is evidence about the work — and nil off it, where busy-ness
// comes from elsewhere and the row's text may have nothing to do with it.
func (m Model) actValues(keys []string) map[string]string {
	if m.actWhen == nil || len(keys) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(keys))
	for _, k := range keys {
		wanted[k] = true
	}
	values := make(map[string]string, len(keys))
	for i, key := range m.itemKeys {
		if wanted[key] && i < len(m.items) {
			values[key] = m.items[i]
		}
	}
	return values
}

// SetBusy is the second entrance: the screen says which items are working
// instead of a predicate reading it off their text.
//
// For busy-ness that is not a property of the row — an operations API, a job
// status resource. The map replaces the previous one outright, exactly as a
// predicate's observation does, so there is still one map and one writer.
// Keys the list does not hold are kept but not drawn.
//
// Panics if the list was built with Options.ActivityWhen: a component uses one
// entrance or the other, and a screen needing both merges them itself.
func (m *Model) SetBusy(busy map[string]string) tea.Cmd {
	if m.actWhen != nil {
		panic("list.SetBusy: built with Options.ActivityWhen; use one entrance or the other")
	}
	cmd := m.act.Observe(busy, nil)
	m.refresh()
	return cmd
}

// Expect marks keys as working because the screen has just asked the server to
// work on them, before any observation can say so. The claim is retired by the
// observations that follow — see activity.Options.Settle.
func (m *Model) Expect(keys []string, label string) tea.Cmd {
	cmd := m.act.Expect(keys, label, m.actValues(keys))
	m.refresh()
	return cmd
}

// Retract drops the claims on keys, for a write the server refused.
func (m *Model) Retract(keys ...string) {
	m.act.Retract(keys...)
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

// ActivityState is the observed set, for carrying across a SetTheme rebuild.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this list's own palette — the rule-4 pair for row activity.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many of this list's items are working — reported by the
// last observation, or claimed by Expect and not yet spoken to.
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
