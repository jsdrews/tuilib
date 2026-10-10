package table

import (
	"github.com/jsdrews/tuilib/internal/remoteview"
	"github.com/jsdrews/tuilib/pkg/activity"
)

// State is what the user and the data did to a table — its rows (a window,
// a span or the whole set), cursor and scroll, filter, sort (committed and
// staged), marks, row activity, the query its rows answer and whether it
// failed, completion values, and loading — held across a rebuild. It
// carries nothing Options sets, so a table built from new Options (a theme
// swap, rule 4) takes it back with Restore:
//
//	st := s.table.State()
//	s.table = table.New(opts)
//	s.table.Restore(st)
type State struct {
	// built is false for a State taken from a zero Model — a screen's
	// first SetTheme — so restoring it leaves the new Options alone.
	built bool

	rows     []Row
	keys     []string
	data     []any
	windowed bool
	winStart int
	winTotal int
	span     *remoteview.Range[KeyedRow]
	reanchor bool

	value    string
	sortCol  int
	sortDesc bool
	rq       remoteState
	distinct [][]string

	marks  []string
	anchor string
	act    activity.Set

	cursor    int
	viewStart int
	loading   bool
	xoff      int
}

// State captures the table's state. See State.
func (m Model) State() State {
	s := State{
		built:     m.token != nil,
		rows:      append([]Row(nil), m.rows...),
		keys:      append([]string(nil), m.rowKeys...),
		data:      append([]any(nil), m.rowData...),
		windowed:  m.windowed,
		winStart:  m.winStart,
		winTotal:  m.winTotal,
		value:     m.Value(),
		sortCol:   m.sortCol,
		sortDesc:  m.sortDesc,
		rq:        m.rq,
		distinct:  m.distinct,
		marks:     m.Marks(),
		anchor:    m.markAnchor,
		act:       m.act,
		cursor:    m.cursor,
		viewStart: m.viewStart,
		loading:   m.body.Loading(),
		xoff:      m.body.XOffset(),
	}
	if m.rowKeys == nil {
		s.keys, s.data = nil, nil
	}
	if m.span != nil {
		sp := *m.span
		s.span, s.reanchor = &sp, m.reanchor
	}
	return s
}

// Restore puts s back: rows first, then the filter, sort and remote state
// that describe them, then marks and activity on rows that exist, then the
// cursor. The new Options keep the say over everything they set — columns,
// styles, the sort debounce, the title. A staged sort starts its quiet
// period again, and a spinner re-arms on the next message.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	switch {
	case s.span != nil && m.span != nil:
		sp := *s.span
		m.span, m.reanchor = &sp, s.reanchor
		_, held := m.spanRows()
		m.setKeyedRows(held)
	case s.windowed:
		m.SetWindow(s.rows, s.winStart, s.winTotal, s.rq.st.Answer)
	case s.keys != nil:
		rows := make([]KeyedRow, len(s.rows))
		for i := range s.rows {
			rows[i] = KeyedRow{Key: s.keys[i], Cells: s.rows[i], Data: s.data[i]}
		}
		m.setKeyedRows(rows)
	default:
		m.SetRows(s.rows)
	}

	m.SetValue(s.value)
	m.sortCol, m.sortDesc = s.sortCol, s.sortDesc
	rq := s.rq
	rq.debounce, rq.baseTitle, rq.loadCmd = m.rq.debounce, m.rq.baseTitle, nil
	rq.stageArmd = false
	rq.st.Rearm()
	m.rq = rq
	m.syncQuery()
	if s.distinct != nil {
		m.distinct = s.distinct
		m.ensureDistinct()
	}

	m.SetMarks(s.marks)
	m.markAnchor = s.anchor
	m.SetActivityState(s.act)

	m.applyFilter()
	m.cursor = max(0, min(s.cursor, m.rowCount()-1))
	m.viewStart = s.viewStart
	m.refresh()
	m.SetLoading(s.loading)
	m.body.SetXOffset(s.xoff)
}
