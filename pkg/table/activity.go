package table

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/query"
)

// Row activity: a spinner and status label on the rows the data says are
// working. See pkg/activity for the state itself.
//
// The whole feature is two options. ActivityWhen says which rows are busy;
// ActivityColumn says where to draw. Neither set, the table carries no gutter
// and pays nothing.
//
// Where it draws:
//
//   - The named column's cell is replaced while its row is busy. This is the
//     shape the feature exists for: Synced → ⣾ Syncing → Synced reads as one
//     cell changing its mind rather than decoration appearing beside it.
//   - A name that resolves to no column — or no name at all, with a predicate
//     set — falls back to a two-cell gutter after the mark gutter. A typo, or
//     an omission, should degrade to something visible rather than to a
//     spinner nobody can see that animates anyway.
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
// off, the name is absent, ambiguous or unknown, or the column it names is
// hidden.
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
//
// It copies before writing. Substituting in place would put the indicator into
// the rows the table holds, and the next observation would then run the
// predicate against the spinner instead of the status — the one way this
// feature could feed back into its own input.
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

// observe recomputes the indicator set from the rows the table now holds.
//
// Called from SetKeyedRows — the data is what this is a function of, so a
// filter or a sort changes nothing here. The command it produces cannot be
// returned (a setter has no return value), so it queues for the next Update,
// exactly as a pending ViewportChangedMsg does.
func (m *Model) observe() {
	if m.actWhen == nil {
		return
	}
	busy := map[string]string{}
	for i, key := range m.rowKeys {
		if key == "" || i >= len(m.rows) {
			continue
		}
		if label, ok := m.actWhen(m.rows[i]); ok {
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
// See SetActivityState.
func (m Model) ActivityState() activity.Set { return m.act }

// SetActivityState adopts the entries of a previous instance's ActivityState,
// keeping this table's own palette — the rule-4 pair for row activity.
//
// The returned command re-arms the spinner; dropping it strands a frozen glyph
// until the next observation.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	if !m.actEnabled {
		return nil
	}
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many rows the last observation reported working.
func (m Model) ActivityCount() int { return m.act.Count() }
