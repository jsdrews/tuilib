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
	// Scoping happens on every swap, predicate or not: it is what keeps a
	// SetBusy map — which comes from somewhere other than these rows — from
	// animating a key this table does not hold.
	m.act.Scope(m.rowKeys)
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
	m.actCmd = tea.Batch(m.act.Observe(busy, m.actValues()), m.actCmd)
}

// actValues is the current activity-column cell for every key carrying a
// claim, which is how an observation notices the server acted (Options.Settle).
//
// Only on the predicate path, and that restriction is the point rather than a
// limitation. Under ActivityWhen the column is the status by construction — the
// predicate reads it — so a change in it is evidence about the work. Under
// SetBusy the busy-ness comes from elsewhere and this column may have nothing
// to do with it, where a cell moving for unrelated reasons would retire a claim
// that is still perfectly live.
//
// Empty whenever nothing has been dispatched, which is almost always.
func (m Model) actValues() map[string]string {
	want := m.act.Expecting()
	if len(want) == 0 {
		return nil
	}
	col := m.activityCol()
	wanted := make(map[string]bool, len(want))
	for _, k := range want {
		wanted[k] = true
	}
	values := make(map[string]string, len(want))
	for i, key := range m.rowKeys {
		if !wanted[key] || i >= len(m.rows) {
			continue
		}
		if col >= 0 && col < len(m.rows[i]) {
			values[key] = m.rows[i][col]
			continue
		}
		values[key] = ""
	}
	return values
}

// SetBusy is the second entrance: the screen says which rows are working
// instead of a predicate reading it off their cells.
//
// For busy-ness that is not a field on the row — an operations API, a job
// status resource, GET /jobs?status=running. The map is the whole truth as of
// that moment and replaces the previous one outright, exactly as a predicate's
// observation does; there is still one map and one writer.
//
// Keys the table does not hold are kept but not drawn, so a paged table can be
// handed the busy set for rows it has not reached yet.
//
// Panics if the table was built with Options.ActivityWhen. Two writers for one
// map is the property that makes this feature unable to contradict itself, and
// a component uses one entrance or the other. A screen that needs both merges
// them itself and calls this — activity.Settled is available for the half that
// reads off the row.
func (m *Model) SetBusy(busy map[string]string) tea.Cmd {
	if m.actWhen != nil {
		panic("table.SetBusy: built with Options.ActivityWhen; use one entrance or the other")
	}
	m.actEnabled = true
	cmd := m.act.Observe(busy, nil)
	m.refresh()
	return cmd
}

// Expect marks keys as working because the screen has just asked the server to
// work on them, before any observation can say so.
//
// The claim is retired by the observations that follow — see Options.Settle for
// the three ways that happens. It is not a second source of truth and cannot
// outlive the data's ability to speak to it.
//
// The screen must not apply a read taken across its own write, or the claim is
// cleared by a reply that predates it. See the activity example.
func (m *Model) Expect(keys []string, label string) tea.Cmd {
	m.actEnabled = true
	cmd := m.act.Expect(keys, label, m.claimValues(keys))
	m.refresh()
	return cmd
}

// claimValues is each key's activity-column cell at the moment of the claim.
func (m Model) claimValues(keys []string) map[string]string {
	if m.actWhen == nil || len(keys) == 0 {
		return nil
	}
	col := m.activityCol()
	wanted := make(map[string]bool, len(keys))
	for _, k := range keys {
		wanted[k] = true
	}
	at := make(map[string]string, len(keys))
	for i, key := range m.rowKeys {
		if !wanted[key] || i >= len(m.rows) {
			continue
		}
		if col >= 0 && col < len(m.rows[i]) {
			at[key] = m.rows[i][col]
			continue
		}
		at[key] = ""
	}
	return at
}

// Retract drops the claims on keys, for a write the server refused.
func (m *Model) Retract(keys ...string) {
	m.act.Retract(keys...)
	m.refresh()
}

// RetractAll drops every claim, for a read that failed.
//
// Required rather than polite: a claim is a promise that the next observation
// will explain it, and an outage is exactly the case where no observation is
// coming to keep it.
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

// ActivityCount is how many of this table's rows are working — reported by the
// last observation, or claimed by Expect and not yet spoken to.
func (m Model) ActivityCount() int { return m.act.Count() }
