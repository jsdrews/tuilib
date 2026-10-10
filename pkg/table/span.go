package table

// AppendRows adds rows at the newer edge of an Anchored table's span,
// oldest first, merged by key: a key already held is skipped and a late row
// lands where the page puts it. answered names the query the rows answer,
// as for SetWindow. The cursor stays on its row.
func (m *Model) AppendRows(rows []KeyedRow, answered Answer) { m.addSpan(rows, answered, false) }

// PrependRows adds rows at the older edge of the span, oldest first.
func (m *Model) PrependRows(rows []KeyedRow, answered Answer) { m.addSpan(rows, answered, true) }

func (m *Model) addSpan(rows []KeyedRow, answered Answer, front bool) {
	if m.span == nil {
		return
	}
	fresh := false
	if m.remote() {
		fresh = m.rq.st.SetAnswer(answered, m.committed())
	}
	if m.reanchor {
		fresh, m.reanchor = true, false
	}
	if fresh {
		m.span.Clear()
	}
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.Key
	}
	var before int
	if front {
		before = m.span.Prepend(rows, keys, m.cursor)
	} else {
		before = m.span.Append(rows, keys, m.cursor)
	}
	if front := m.span.Trim(m.maxItems, m.viewStart, m.viewStart+m.dataRows()-1); front > 0 {
		before -= front
	}
	m.viewStart = max(0, m.viewStart+before)
	_, held := m.spanRows()
	m.setKeyedRows(held)
	if fresh {
		m.cursor, m.viewStart = 0, 0
		m.refresh()
	}
}

func (m Model) spanRows() (int, []KeyedRow) {
	start, n := m.span.Held()
	out := make([]KeyedRow, 0, n)
	for i := start; i < start+n; i++ {
		r, _ := m.span.At(i)
		out = append(out, r)
	}
	return start, out
}

// Reanchor says the source is re-anchoring (source.Anchored.SetAnchor):
// the next page replaces the span instead of extending it. The rows on
// screen stay until then.
func (m *Model) Reanchor() { m.reanchor = m.span != nil }

// SetMore sets whether each edge of the span has more beyond it — from the
// source's page.More after each page.
func (m *Model) SetMore(older, newer bool) {
	if m.span == nil {
		return
	}
	m.span.SetMore(older, newer)
	m.refresh()
}

// Edges returns the keys — cursors — of the oldest and newest rows held,
// for the screen to extend the span from.
func (m Model) Edges() (older, newer string) {
	if m.span == nil {
		return "", ""
	}
	return m.span.Edges()
}

// spanEdgeLoading reports the border text while the viewport sits at an
// edge with more beyond it, or "".
func (m Model) spanEdgeLoading() string {
	if m.span == nil || !m.rq.st.HasAnswer || m.Stale() || m.rowCount() == 0 {
		return ""
	}
	older, newer := m.span.More()
	last := min(m.viewStart+m.dataRows()-1, m.rowCount()-1)
	switch {
	case older && m.viewStart == 0:
		return m.rq.st.Spin() + " loading older…"
	case newer && last >= m.rowCount()-1:
		return m.rq.st.Spin() + " loading newer…"
	}
	return ""
}
