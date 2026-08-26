package list

import "strconv"

// Marking: a multi-selection the user builds with space, held by Key.
//
// # Why keys and not indices
//
// Marks have to survive the thing that makes them worth having: a polled
// refresh (CLAUDE.md rule 24) reorders and partially replaces the item set
// between the moment a user marks rows and the moment they pick a verb. Marks
// held by index would drift onto whatever landed at that position — which, on
// a destructive action, is the worst bug this library could ship.
//
// So marking works only on keyed items (SetKeyedItems). On anonymous items
// every mark operation is a no-op rather than an approximation: an inert
// feature is recoverable, a silently wrong selection is not.
//
// Marks also survive filtering, because a key does not care whether its row is
// currently visible. That means a user can mark a row, filter it out of sight,
// and still act on it — correct, and a genuine surprise, which is why
// action.Set.Target names the target on the menu's own border.

const (
	cursorGlyph = "▸"
	markGlyph   = "✓"
)

// keyAt maps a visible row index to its item key.
func (m Model) keyAt(i int) (string, bool) {
	if i < 0 || i >= len(m.visibleIdx) {
		return "", false
	}
	src := m.visibleIdx[i]
	if src < 0 || src >= len(m.itemKeys) {
		return "", false
	}
	return m.itemKeys[src], true
}

func (m Model) isMarkedAt(i int) bool {
	k, ok := m.keyAt(i)
	return ok && m.marks[k]
}

// prefixFor is the row gutter: the cursor glyph, and — when marking is on —
// the mark glyph beside it. Two cells without marking, three with, so turning
// Markable off costs a row nothing.
func (m Model) prefixFor(i int) string {
	cur := " "
	if i == m.cursor {
		cur = cursorGlyph
	}
	if !m.markable {
		return cur + " "
	}
	mk := " "
	if m.isMarkedAt(i) {
		mk = markGlyph
	}
	return cur + mk + " "
}

// onMarkColumn reports whether x falls on the ✓ cell of a row — the second
// gutter column, immediately right of the cursor glyph.
//
// One cell wide, deliberately: the first gutter column is the cursor glyph,
// and swallowing a click there would mean the left edge of every row toggled
// a mark instead of selecting the row.
func (m Model) onMarkColumn(x int) bool {
	c := m.body.ContentRect()
	return c.W > 0 && x-c.X == 1
}

// Markable reports whether this list accepts marks.
func (m Model) Markable() bool { return m.markable }

// ToggleMark flips the mark on the cursor row. No-op when marking is off or
// the items are anonymous.
func (m *Model) ToggleMark() {
	m.toggleMarkAt(m.cursor)
}

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
// are all already marked.
//
// Visible means post-filter, which is the useful reading: filter to a subset,
// mark it wholesale, act. Under a windowed table the same rule applies to the
// rows actually held — see table.ToggleMarkAll.
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

// Marks returns the marked keys in item order — not in the order they were
// marked, so the result is stable across equivalent selections.
func (m Model) Marks() []string {
	if len(m.marks) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.marks))
	for _, k := range m.itemKeys {
		if m.marks[k] {
			out = append(out, k)
		}
	}
	return out
}

// MarkCount is how many keys are marked, including any whose rows the current
// filter hides.
func (m Model) MarkCount() int { return len(m.marks) }

// SetMarks replaces the marked set. Keys that match no item are kept: an item
// set swapped out and back should not silently lose the user's selection.
// Carries marks across a SetTheme rebuild (rule 4).
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
// marked. Empty when the items are anonymous.
//
// This is the accessor a screen should reach for, and the reason it exists is
// the branch it removes. Without it every caller writes
//
//	if ks := l.Marks(); len(ks) > 0 { … } else { … }
//
// which is easy to write once and easy to forget, and whose failure mode is a
// verb quietly acting on one row when the user had marked six.
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
//
// It returns the key rather than the display text because the key is the
// identity — display text is usually a formatted row with columns in it,
// which reads badly in a sentence. A caller wanting something else formats
// Selection() itself.
func (m Model) SelectionLabel() string { return selectionLabel(m.Selection()) }

func selectionLabel(sel []string) string {
	switch len(sel) {
	case 0:
		return ""
	case 1:
		return sel[0]
	default:
		return plural(len(sel))
	}
}

func plural(n int) string {
	return strconv.Itoa(n) + " items"
}
