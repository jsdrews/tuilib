package tree

import "strconv"

// Marking: a multi-selection the user builds with x, held by node path.
//
// Same contract as pkg/list and pkg/table — see list/mark.go for why marks are
// held by an identity rather than an index. Three things are specific to the
// tree:
//
//   - The key is the row's path, which the tree already maintains for
//     expansion state and cursor restore. So there is no keyed-data
//     precondition here: unlike a list, whose display strings are not
//     identities, every tree node has one already.
//   - The mark key is x, not space. space is spent on expand/collapse, and
//     rebinding it would cost the tree its most-used verb to buy marking. x
//     is also bound in list and table, so one verb marks everywhere.
//   - Marking a branch marks that node and nothing else. Whether a marked
//     branch implies its descendants is the caller's question to answer, and
//     paths are hierarchical strings, so a caller who wants the subtree can
//     prefix-test the ones it got. The alternative — marking a branch marks
//     its children — needs a tri-state glyph and a rule for children that
//     arrive on a later refresh, neither of which pays for itself until
//     something actually needs it.
//
// The gutter is drawn leftmost, before the indent, so the ✓ column lines up
// at every depth. A gutter that indented with its row would be unreadable as
// a column, which is the whole reason to have one.

const markGlyph = "✓"

// gutterW is the width the mark column takes. Zero when marking is off, so a
// tree that does not opt in is laid out exactly as before.
func (m Model) gutterW() int {
	if !m.markable {
		return 0
	}
	return 2
}

// gutterForRow is the leading cell pair for r. Keyed off the row's path
// rather than its index so rendering needs no index to thread through.
func (m Model) gutterForRow(r row) string {
	if !m.markable {
		return ""
	}
	if m.marks[r.path] {
		return markGlyph + " "
	}
	return "  "
}

// keyAt maps a visible row index to its node path.
func (m Model) keyAt(i int) (string, bool) {
	if i < 0 || i >= len(m.rows) {
		return "", false
	}
	return m.rows[i].path, true
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

// Markable reports whether this tree accepts marks.
func (m Model) Markable() bool { return m.markable }

// ToggleMark flips the mark on the cursor row.
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
		if m.markAnchor == k {
			m.markAnchor = ""
		}
	} else {
		m.marks[k] = true
		m.markAnchor = k
	}
	m.refresh()
}

// ToggleMarkAll marks every currently visible row, or clears them when they
// are all already marked.
//
// Visible means what is actually on screen: expanded, and surviving the
// filter. Rows inside a collapsed branch are deliberately not included —
// marking what the user cannot see is how a selection becomes a surprise.
func (m *Model) ToggleMarkAll() {
	if !m.markable || len(m.rows) == 0 {
		return
	}
	if m.marks == nil {
		m.marks = map[string]bool{}
	}

	all := true
	for i := range m.rows {
		if k, ok := m.keyAt(i); ok && !m.marks[k] {
			all = false
			break
		}
	}
	for i := range m.rows {
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

// Marks returns the marked paths in document order — the order they appear
// walking the tree top to bottom, whatever is currently expanded.
//
// It walks the whole tree rather than the visible rows on purpose: a mark on
// a row inside a collapsed branch is still a mark, and ordering only by what
// happens to be on screen would drop it from the result while MarkCount kept
// counting it.
func (m Model) Marks() []string {
	if len(m.marks) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.marks))
	for _, p := range m.allPaths() {
		if m.marks[p] {
			out = append(out, p)
		}
	}
	return out
}

// allPaths walks the tree unconditionally, in document order.
func (m Model) allPaths() []string {
	if m.root == nil {
		return nil
	}
	var out []string
	var walk func(n Node, path string)
	walk = func(n Node, path string) {
		out = append(out, path)
		seen := map[string]int{}
		for _, c := range n.Children() {
			label := c.Label()
			seen[label]++
			walk(c, childPath(path, label, seen[label]))
		}
	}
	walk(m.root, rootPath(m.root))
	return out
}

// MarkCount is how many paths are marked, including any currently hidden by
// a collapsed ancestor or the filter.
func (m Model) MarkCount() int { return len(m.marks) }

// SetMarks replaces the marked set. Carries marks across a SetTheme rebuild
// (rule 4).
func (m *Model) SetMarks(keys []string) {
	m.marks = make(map[string]bool, len(keys))
	for _, k := range keys {
		m.marks[k] = true
	}
	if !m.marks[m.markAnchor] {
		m.markAnchor = ""
	}
	m.refresh()
}

// ClearMarks drops every mark.
func (m *Model) ClearMarks() {
	if len(m.marks) == 0 {
		return
	}
	m.marks = map[string]bool{}
	m.markAnchor = ""
	m.refresh()
}

// anchorIndex resolves the anchor path to its current row. Not found means the
// anchor is collapsed away or filtered out, in which case there is no range to
// draw.
func (m Model) anchorIndex() (int, bool) {
	if m.markAnchor == "" {
		return 0, false
	}
	for i := range m.rows {
		if m.rows[i].path == m.markAnchor {
			return i, true
		}
	}
	return 0, false
}

func (m *Model) markAt(i int) (string, bool) {
	k, ok := m.keyAt(i)
	if !ok {
		return "", false
	}
	if m.marks == nil {
		m.marks = map[string]bool{}
	}
	m.marks[k] = true
	return k, true
}

// MarkRange marks every visible row between the anchor and the cursor,
// inclusive, in whichever direction they sit. With no usable anchor it marks
// the cursor row alone and anchors there.
//
// The span runs over visible rows, so it follows what the user can see rather
// than reaching into collapsed branches between the two ends.
func (m *Model) MarkRange() {
	if !m.markable {
		return
	}
	cur := m.cursor
	if _, ok := m.keyAt(cur); !ok {
		return
	}

	start, ok := m.anchorIndex()
	if !ok {
		if k, ok := m.markAt(cur); ok {
			m.markAnchor = k
		}
		m.refresh()
		return
	}

	lo, hi := start, cur
	if lo > hi {
		lo, hi = hi, lo
	}
	for i := lo; i <= hi; i++ {
		m.markAt(i)
	}
	m.refresh()
}

// Selection is the marked paths, or the cursor row's path when nothing is
// marked.
func (m Model) Selection() []string {
	if ks := m.Marks(); len(ks) > 0 {
		return ks
	}
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return []string{m.rows[m.cursor].path}
	}
	return nil
}

// SelectionLabel names the selection for a confirm string or a menu title:
// the single path, or "N items".
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
