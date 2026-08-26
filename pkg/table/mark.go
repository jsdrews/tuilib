package table

import "strconv"

// Marking: a multi-selection the user builds with space, held by Key.
//
// Same contract as pkg/list — see that package's mark.go for why marks are
// held by key rather than by index, and why they survive filtering. Two
// differences are specific to the table:
//
//   - The gutter is two cells, not three. A table shows its cursor with a
//     background rather than a ▸ glyph, so only the ✓ needs a column.
//   - A windowed table (SetWindow) cannot be marked. SetWindow carries rows
//     without keys, and a mark held against a row index in a sparse, paged
//     set is precisely the drift this design refuses to ship. Marking there
//     is inert rather than approximate.

const markGlyph = "✓"

// gutterW is the width the mark column takes from the columns. Zero when
// marking is off, so a table that does not opt in is laid out exactly as
// before.
func (m Model) gutterW() int {
	if !m.markable {
		return 0
	}
	return 2
}

// gutterFor is the leading cell pair for logical row i.
func (m Model) gutterFor(i int) string {
	if !m.markable {
		return ""
	}
	if m.isMarkedAt(i) {
		return markGlyph + " "
	}
	return "  "
}

// keyAt maps a logical row index to its row key.
func (m Model) keyAt(i int) (string, bool) {
	if m.windowed {
		return "", false
	}
	if i < 0 || i >= len(m.visibleIdx) {
		return "", false
	}
	src := m.visibleIdx[i]
	if src < 0 || src >= len(m.rowKeys) {
		return "", false
	}
	return m.rowKeys[src], true
}

func (m Model) isMarkedAt(i int) bool {
	k, ok := m.keyAt(i)
	return ok && m.marks[k]
}

// onMarkColumn reports whether x falls in the mark gutter of a row.
func (m Model) onMarkColumn(x int) bool {
	if !m.markable {
		return false
	}
	c := m.body.ContentRect()
	pos := (x - c.X) + m.body.XOffset()
	return pos >= 0 && pos < m.gutterW()
}

// Markable reports whether this table accepts marks.
func (m Model) Markable() bool { return m.markable }

// ToggleMark flips the mark on the cursor row. No-op when marking is off, the
// rows are anonymous, or the table is windowed.
func (m *Model) ToggleMark() { m.toggleMarkAt(m.cursor) }

func (m *Model) toggleMarkAt(i int) {
	if !m.markable {
		return
	}
	k, ok := m.keyAt(i)
	if !ok {
		return
	}
	if m.marks == nil {
		m.marks = map[string]bool{}
	}
	if m.marks[k] {
		delete(m.marks, k)
	} else {
		m.marks[k] = true
	}
	m.refresh()
}

// ToggleMarkAll marks every currently visible row, or clears them when they
// are all already marked. Visible means post-filter.
func (m *Model) ToggleMarkAll() {
	if !m.markable || len(m.visible) == 0 {
		return
	}
	if m.marks == nil {
		m.marks = map[string]bool{}
	}

	all := true
	for i := range m.visible {
		if k, ok := m.keyAt(i); ok && !m.marks[k] {
			all = false
			break
		}
	}
	for i := range m.visible {
		if k, ok := m.keyAt(i); ok {
			if all {
				delete(m.marks, k)
			} else {
				m.marks[k] = true
			}
		}
	}
	m.refresh()
}

// Marks returns the marked keys in row order — not in the order they were
// marked, so the result is stable across equivalent selections.
func (m Model) Marks() []string {
	if len(m.marks) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.marks))
	for _, k := range m.rowKeys {
		if m.marks[k] {
			out = append(out, k)
		}
	}
	return out
}

// MarkCount is how many keys are marked, including any the filter hides.
func (m Model) MarkCount() int { return len(m.marks) }

// SetMarks replaces the marked set. Carries marks across a SetTheme rebuild
// (rule 4).
func (m *Model) SetMarks(keys []string) {
	m.marks = make(map[string]bool, len(keys))
	for _, k := range keys {
		m.marks[k] = true
	}
	m.refresh()
}

// ClearMarks drops every mark.
func (m *Model) ClearMarks() {
	if len(m.marks) == 0 {
		return
	}
	m.marks = map[string]bool{}
	m.refresh()
}

// Selection is the marked keys, or the cursor row's key when nothing is
// marked. Empty when the rows are anonymous or the table is windowed.
//
// Reach for this rather than Marks: it removes the branch whose failure mode
// is a verb quietly acting on one row when the user marked six.
func (m Model) Selection() []string {
	if ks := m.Marks(); len(ks) > 0 {
		return ks
	}
	if k, ok := m.SelectedKey(); ok {
		return []string{k}
	}
	return nil
}

// SelectionLabel names the selection for a confirm string or a menu title:
// the single key, or "N items".
func (m Model) SelectionLabel() string {
	sel := m.Selection()
	switch len(sel) {
	case 0:
		return ""
	case 1:
		return sel[0]
	default:
		return strconv.Itoa(len(sel)) + " items"
	}
}
