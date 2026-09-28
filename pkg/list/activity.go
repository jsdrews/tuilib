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
// The surface is the table's: BusyWhen (and Revision) for what the server
// reports, Dispatch / Done for work the user starts, BeginRead / ApplyRead for
// fetches. Entries are held by key, so it works on SetKeyedItems and is inert
// on anonymous items.

// observe recomputes the indicator set from the items the list now holds.
//
// Called from SetKeyedItems. The command it produces cannot be returned (a
// setter has no return value), so it queues for the next Update, exactly as a
// pending SelectedChangedMsg does.
func (m *Model) observe() {
	var eval func(int) (string, bool, string)
	if m.busyWhen != nil {
		eval = func(i int) (string, bool, string) {
			if i >= len(m.items) {
				return "", false, ""
			}
			var data any
			if i < len(m.itemData) {
				data = m.itemData[i]
			}
			it := KeyedItem{Key: m.itemKeys[i], Display: m.items[i], Data: data}
			label, busy := m.busyWhen(it)
			rev := ""
			if m.revision != nil {
				rev = m.revision(it)
			}
			return label, busy, rev
		}
	}
	m.actCmd = tea.Batch(m.act.ObserveRows(m.itemKeys, eval), m.actCmd)
}

// Dispatch opens a claim on keys for work the screen is about to request, and
// returns the operation to carry to Done. mode says who knows when the work
// ends: activity.Observed (the server's status) or activity.Held (the request,
// or a job it returned).
func (m *Model) Dispatch(keys []string, label string, mode activity.Mode) (activity.Op, tea.Cmd) {
	op, cmd := m.act.Dispatch(keys, label, mode)
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
func (m *Model) DispatchEach(keys []string, label string, mode activity.Mode,
	request func(op activity.Op, key string) tea.Cmd) tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2*len(keys))
	for _, k := range keys {
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
// ApplyRead with the reply. A stream needs none: push events with
// SetKeyedItems.
func (m *Model) BeginRead() activity.Read { return m.act.BeginRead() }

// ApplyRead is SetKeyedItems for a fetched reply, and reports whether it
// applied it. False means dropped: overtaken by a newer reply, or failed —
// in which case operations waiting on a read are withdrawn and items busy on
// the last good read turn Unknown until one succeeds.
func (m *Model) ApplyRead(rd activity.Read, items []KeyedItem, err error) bool {
	if !m.act.Accept(rd, err) {
		m.refresh()
		return false
	}
	m.SetKeyedItems(items)
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

// ActivityState is the observed set, for carrying across a SetTheme rebuild.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this list's own palette — the rule-4 pair for row activity.
//
// The returned command is the spinner's first tick. SetTheme has nowhere to
// return it, and dropping it is safe: the list re-arms a stalled spinner on
// the next message it receives.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many of this list's items are working — reported by the
// last observation, or dispatched and not yet spoken to.
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
