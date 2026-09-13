package table

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/query"
)

// Row activity: a spinner and status label on the rows something is currently
// working on. See pkg/activity for the state itself.
//
// The whole feature hangs off Options.ActivityColumn. Unset, the table carries
// no gutter, binds nothing, and every setter here is a no-op — a table that
// did not ask for activity pays nothing for it.
//
// Where it draws:
//
//   - The named column's cell is replaced while its row is busy. This is the
//     shape the feature exists for: Synced → ⠹ syncing → ✓ synced → Synced
//     reads as one cell changing its mind rather than decoration appearing
//     beside it.
//   - A name that resolves to no column falls back to a two-cell gutter after
//     the mark gutter. A typo should degrade to something visible rather than
//     to silence.
//
// Column widths are computed from the rows the table holds, never from the
// indicator, so activity cannot reflow the table under the user. The cost of
// that is the opposite hazard: an auto-sized column fitted to "Synced" has
// room for the glyph and not the word. Give a column that will carry activity
// a Width wide enough for the longest label it will show.
//
// Marking's rules apply unchanged. Entries are held by key, so a polled
// refresh that reorders rows keeps each spinner on its own row; a windowed
// table (SetWindow) carries rows without keys and is inert.

// actGutterW is the width the fallback gutter takes when activity is on and
// its column name resolves to nothing.
func (m Model) actGutterW() int {
	if m.actEnabled && m.activityCol() < 0 {
		return 2
	}
	return 0
}

// actGutterFor is the fallback gutter's cell pair for logical row i.
func (m Model) actGutterFor(i int) string {
	if m.actGutterW() == 0 {
		return ""
	}
	k, ok := m.keyAt(i)
	if !ok {
		return "  "
	}
	text, ok := m.act.Render(k, 1)
	if !ok {
		return "  "
	}
	return text + " "
}

// activityCol resolves Options.ActivityColumn to a column index, matching on
// Title the way a filter's key:value scope does. Reports -1 when activity is
// off, the name is ambiguous or unknown, or the column it names is hidden.
func (m Model) activityCol() int {
	if m.actColName == "" {
		return -1
	}
	titles := make([]string, len(m.cols))
	for i, c := range m.cols {
		titles[i] = c.Title
	}
	idx := query.ColumnByPrefix(m.actColName, titles)
	if idx < 0 || m.cols[idx].Hidden {
		return -1
	}
	return idx
}

// withActivity substitutes the indicator into logical row i's cells. Returns
// the row untouched when nothing is running against it, so the common case
// allocates nothing.
func (m Model) withActivity(i int, cells Row) Row {
	col := m.activityCol()
	if !m.actEnabled || col < 0 || !m.act.Active() {
		return cells
	}
	k, ok := m.keyAt(i)
	if !ok {
		return cells
	}
	w := 0
	if col < len(m.widths) {
		w = m.widths[col]
	}
	text, ok := m.act.Render(k, w)
	if !ok {
		return cells
	}
	out := append(Row(nil), cells...)
	for len(out) <= col {
		out = append(out, "")
	}
	out[col] = text
	return out
}

// observe recomputes the derived layer from the rows the table now holds, and
// reports any revision that changed while its row was not busy.
//
// Called from SetKeyedRows — the data is what derived state is a function of,
// so a filter or a sort changes nothing here. The commands it produces cannot
// be returned (a setter has no return value), so they queue for the next
// Update, exactly as a pending ViewportChangedMsg does.
func (m *Model) observe() {
	if m.actWhen == nil && m.actRev == nil {
		return
	}
	var cmds []tea.Cmd

	if m.actWhen != nil {
		busy := map[string]string{}
		for i, key := range m.rowKeys {
			if key == "" || i >= len(m.rows) {
				continue
			}
			if label, ok := m.actWhen(m.rows[i]); ok {
				busy[key] = label
			}
		}
		// Unconditionally, including when nothing matches: an empty
		// observation is a real one, and the only thing that can end a
		// handoff (decision 19).
		cmds = append(cmds, m.act.Derive(busy))
	}

	if m.actRev != nil {
		col := m.activityCol()
		changes := make(map[string]activity.Change, len(m.rowKeys))
		for i, key := range m.rowKeys {
			if key == "" || i >= len(m.rows) {
				continue
			}
			label := ""
			if col >= 0 && col < len(m.rows[i]) {
				// The indicator replaces the cell, so a flash that showed only
				// a glyph would hide the change it is pointing at.
				label = m.rows[i][col]
			}
			changes[key] = activity.Change{Rev: m.actRev(m.rows[i]), Label: label}
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

// holdsKey reports whether this table currently shows a row with that key.
//
// It is the predicate the shell's broadcasts are filtered through: a component
// declines keys it does not hold, the same way it declines a mouse event
// outside its rect. Direct calls to SetActivity are not filtered this way —
// there, the screen is asserting it knows what it is doing, and the row may
// still be in flight from a fetch.
func (m Model) holdsKey(k string) bool {
	if m.windowed || k == "" {
		return false
	}
	for _, rk := range m.rowKeys {
		if rk == k {
			return true
		}
	}
	return false
}

// SetActivity starts the spinner on one row, replacing anything already there.
//
// Batch the returned command into your screen's command stream — it is the
// animation's first tick, exactly as SetLoading's is (rule 17). Inert when
// Options.ActivityColumn is unset or the table is windowed; on anonymous rows
// nothing ever draws, because an entry with no key to match is unreachable.
func (m *Model) SetActivity(key, label string) tea.Cmd {
	if !m.actEnabled || m.windowed {
		return nil
	}
	cmd := m.act.Start(key, label)
	m.refresh()
	return cmd
}

// EndActivity finishes one row, showing ✓ or ✗ for the hold before it clears.
// A nil err is a success.
func (m *Model) EndActivity(key string, err error) tea.Cmd {
	if !m.actEnabled {
		return nil
	}
	cmd := m.act.Finish(key, err)
	m.refresh()
	return cmd
}

// ClearActivity retires one row's indicator at once, outcome or not.
func (m *Model) ClearActivity(key string) {
	if !m.actEnabled {
		return
	}
	m.act.Clear(key)
	m.refresh()
}

// Relabel changes what a busy row says without restarting it.
func (m *Model) Relabel(key, label string) {
	if !m.actEnabled {
		return
	}
	m.act.Relabel(key, label)
	m.refresh()
}

// ActivityState is the in-flight set, for carrying across a SetTheme rebuild.
// See SetActivityState.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this table's own palette — the rule-4 pair for row activity.
//
// The returned command re-arms the spinner and any hold that was mid-flight;
// dropping it strands a frozen glyph on the row.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	if !m.actEnabled {
		return nil
	}
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many rows are still working, held outcomes excluded.
func (m Model) ActivityCount() int { return m.act.Count() }
