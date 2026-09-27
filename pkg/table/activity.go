package table

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/query"
)

// Row activity: a spinner and status label on the rows the data says are
// working, plus the operations the screen starts on them. See pkg/activity for
// the state itself.
//
// Three things, and a screen uses as many as its API needs:
//
//   - BusyWhen (an option) reads each row and says which are working. A
//     read-only dashboard needs nothing else.
//   - Dispatch / Done cover work the user starts: Dispatch puts the row in
//     motion at once, Done reports that the request answered. The mode passed
//     to Dispatch — activity.Observed or activity.Held — says who knows when
//     the work ends.
//   - BeginRead / ApplyRead put every fetch on one clock with those answers,
//     so a reply that predates the user's write cannot end their spinner.
//
// Where it draws: the ActivityColumn cell is replaced while its row is busy;
// a name that resolves to no column, or none with a predicate set, falls back
// to a two-cell gutter so a typo degrades to something visible. Widths come
// from the rows and never from the indicator, so give that column a Width
// wide enough for the longest label it will carry.
//
// Entries are held by key, so a refresh that reorders rows keeps each spinner
// on its own row; a windowed table (SetWindow) carries no keys and is inert.

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
	var eval func(int) (string, bool, string)
	if m.busyWhen != nil {
		eval = func(i int) (string, bool, string) {
			if i >= len(m.rows) {
				return "", false, ""
			}
			var data any
			if i < len(m.rowData) {
				data = m.rowData[i]
			}
			row := KeyedRow{Key: m.rowKeys[i], Cells: m.rows[i], Data: data}
			label, busy := m.busyWhen(row)
			rev := ""
			if m.revision != nil {
				rev = m.revision(row)
			}
			return label, busy, rev
		}
	}
	m.actCmd = tea.Batch(m.act.ObserveRows(m.rowKeys, eval), m.actCmd)
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
// Call it after SetKeyedRows, so the adopted claims land on rows that exist.
// The returned command is the spinner's first tick. SetTheme has nowhere to
// return it, and dropping it is safe: the table re-arms a stalled spinner on
// the next message it receives.
func (m *Model) SetActivityState(s activity.Set) tea.Cmd {
	if !m.actEnabled {
		return nil
	}
	cmd := m.act.Adopt(s)
	m.refresh()
	return cmd
}

// ActivityCount is how many of this table's rows are working — reported by the
// last observation, or dispatched and not yet spoken to.
func (m Model) ActivityCount() int { return m.act.Count() }

// ReadsFailing reports whether the newest fetch handed to ApplyRead failed and
// none has succeeded since. Rows busy only because an earlier read said so are
// drawn Unknown — static, with "?" — meanwhile; say so on the title too.
func (m Model) ReadsFailing() bool { return m.act.ReadsFailing() }

// --- BusyWhen, Dispatch / Done, BeginRead / ApplyRead -----------------------

// Dispatch opens a claim on keys for work the screen is about to request, and
// returns the operation to carry to Done.
//
// mode is the one decision: activity.Observed when the server's status says
// when the work ends (a sync POST that answers at once), activity.Held when
// the request or a job handle does (a refresh GET that holds the connection).
// See pkg/activity.
func (m *Model) Dispatch(keys []string, label string, mode activity.Mode) (activity.Op, tea.Cmd) {
	m.actEnabled = true
	op, cmd := m.act.Dispatch(keys, label, mode)
	m.refresh()
	return op, cmd
}

// Done reports that op's request answered. An error withdraws the claim; nil
// ends a Held operation and acknowledges an Observed one.
func (m *Model) Done(op activity.Op, err error) {
	m.act.Done(op, err)
	m.refresh()
}

// BeginRead stamps a fetch at the moment it is issued. Carry the token on the
// fetch's reply and hand it to ApplyRead. A stream needs none: push its events
// with SetKeyedRows as they arrive.
func (m *Model) BeginRead() activity.Read { return m.act.BeginRead() }

// ApplyRead is SetKeyedRows for a fetched reply: it applies rows if the read
// may be applied, and reports whether it did.
//
// False means the reply was dropped — overtaken by a newer one already
// applied, or failed (err != nil), in which case the operations that were
// waiting on a read to end them are withdrawn, since none is coming. A read
// issued before one of the user's requests answered is applied, but cannot end
// that request's spinner.
func (m *Model) ApplyRead(rd activity.Read, rows []KeyedRow, err error) bool {
	if !m.act.Accept(rd, err) {
		m.refresh()
		return false
	}
	m.SetKeyedRows(rows)
	return true
}
