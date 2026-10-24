// Package grouping lays items out under group headers for the two menus that
// draw them, pkg/menu and pkg/action's Menu, so both follow one set of rules:
//
//   - headers appear only when the items name two or more groups;
//   - groups appear in the order first seen, header text as written;
//   - items without a group go last, under Other, merged with any group the
//     caller named Other;
//   - within a group, items keep the order they were given in;
//   - the cursor, clicks and jumps land on items, never on headers.
//
// A Layout is display order only. It holds indices into the caller's items,
// so each menu keeps its own item type and its cursor stays an item index.
package grouping

import "github.com/charmbracelet/lipgloss"

// Other is the header over items that name no group.
const Other = "other"

// Name is the group an item is drawn under: its own, or Other.
func Name(group string) string {
	if group == "" {
		return Other
	}
	return group
}

// Grouped reports whether groups name two or more distinct groups — the
// point at which headers are worth a row each.
func Grouped(groups []string) bool {
	if len(groups) == 0 {
		return false
	}
	first := Name(groups[0])
	for _, g := range groups[1:] {
		if Name(g) != first {
			return true
		}
	}
	return false
}

// Row is one line of a layout: a header, or an item.
type Row struct {
	Header bool
	// Group is the header's text, or the group an item is drawn under.
	Group string
	// Item indexes the groups slice given to New. -1 on a header.
	Item int
}

// Layout is a set of items in display order, with headers between groups.
type Layout struct {
	rows    []Row
	grouped bool
	// order is the items in display order; pos maps an item to its place in
	// order (-1 when it was not kept); rowOf maps an item to its row.
	order []int
	pos   []int
	rowOf []int
}

// New lays out the items whose groups are groups[i], keeping those keep
// accepts (all of them when keep is nil).
//
// Whether to draw headers is decided over every item, kept or not. A filter
// that narrows a grouped menu to one group therefore keeps that group's
// header: the labels under it were written to be read beside it, and "instance"
// alone does not say which database. A group with nothing kept loses its
// header.
func New(groups []string, keep func(int) bool) Layout {
	l := Layout{
		grouped: Grouped(groups),
		pos:     make([]int, len(groups)),
		rowOf:   make([]int, len(groups)),
	}
	for i := range groups {
		l.pos[i], l.rowOf[i] = -1, -1
	}
	add := func(group string, items []int) {
		if len(items) == 0 {
			return
		}
		if l.grouped {
			l.rows = append(l.rows, Row{Header: true, Group: group, Item: -1})
		}
		for _, i := range items {
			l.pos[i] = len(l.order)
			l.rowOf[i] = len(l.rows)
			l.order = append(l.order, i)
			l.rows = append(l.rows, Row{Group: group, Item: i})
		}
	}

	var names []string
	members := map[string][]int{}
	for i, g := range groups {
		if keep != nil && !keep(i) {
			continue
		}
		n := Name(g)
		if _, seen := members[n]; !seen && n != Other {
			names = append(names, n)
		}
		members[n] = append(members[n], i)
	}
	for _, n := range names {
		add(n, members[n])
	}
	add(Other, members[Other])
	return l
}

// Grouped reports whether the layout draws headers.
func (l Layout) Grouped() bool { return l.grouped }

// Rows is every line in display order.
func (l Layout) Rows() []Row { return l.rows }

// Len is the number of lines, headers included.
func (l Layout) Len() int { return len(l.rows) }

// Items is the number of items kept.
func (l Layout) Items() int { return len(l.order) }

// At is the line at row.
func (l Layout) At(row int) (Row, bool) {
	if row < 0 || row >= len(l.rows) {
		return Row{}, false
	}
	return l.rows[row], true
}

// RowOf is the line item is drawn on, or -1 when it was not kept.
func (l Layout) RowOf(item int) int {
	if item < 0 || item >= len(l.rowOf) {
		return -1
	}
	return l.rowOf[item]
}

// Kept reports whether item is in the layout.
func (l Layout) Kept(item int) bool { return l.RowOf(item) >= 0 }

// HeaderAbove reports whether item is the first under its header, so a menu
// scrolling to it can bring the header into view as well.
func (l Layout) HeaderAbove(item int) bool {
	r := l.RowOf(item)
	return r > 0 && l.rows[r-1].Header
}

// ItemNear is the item on row, or the first one below it when row is a
// header — where a scrollbar drag aimed at a header lands. -1 past the end.
func (l Layout) ItemNear(row int) int {
	for r := max(row, 0); r < len(l.rows); r++ {
		if !l.rows[r].Header {
			return l.rows[r].Item
		}
	}
	return -1
}

// First is the first item ok accepts, or -1. A nil ok accepts every item.
func (l Layout) First(ok func(int) bool) int { return l.scan(0, 1, ok) }

// Last is the last item ok accepts, or -1.
func (l Layout) Last(ok func(int) bool) int { return l.scan(len(l.order)-1, -1, ok) }

// scan walks display order from position p by step and returns the first
// item ok accepts.
func (l Layout) scan(p, step int, ok func(int) bool) int {
	for ; p >= 0 && p < len(l.order); p += step {
		if ok == nil || ok(l.order[p]) {
			return l.order[p]
		}
	}
	return -1
}

// Step moves |delta| accepted items from item in display order, in delta's
// direction, stopping at the ends rather than wrapping. From an item that is
// not in the layout it starts at the near end, so the first step lands on the
// first (or last) accepted item. Returns item when nothing can move.
func (l Layout) Step(item, delta int, ok func(int) bool) int {
	if delta == 0 || len(l.order) == 0 {
		return item
	}
	step, n := 1, delta
	if delta < 0 {
		step, n = -1, -delta
	}
	p := -1
	if l.Kept(item) {
		p = l.pos[item]
	} else if step < 0 {
		p = len(l.order)
	}
	cur := item
	for ; n > 0; n-- {
		next := l.scan(p+step, step, ok)
		if next < 0 {
			break
		}
		cur, p = next, l.pos[next]
	}
	return cur
}

// JumpGroup moves to the first accepted item of the next group (dir > 0) or
// the previous one (dir < 0), skipping groups with nothing accepted. Returns
// item when there is no such group, or when the layout has no headers.
func (l Layout) JumpGroup(item, dir int, ok func(int) bool) int {
	if !l.grouped || dir == 0 {
		return item
	}
	if !l.Kept(item) {
		if dir > 0 {
			return l.First(ok)
		}
		return l.Last(ok)
	}
	step := 1
	if dir < 0 {
		step = -1
	}
	here := l.rows[l.rowOf[item]].Group
	for r := l.rowOf[item] + step; r >= 0 && r < len(l.rows); r += step {
		h := l.rows[r]
		if !h.Header || h.Group == here {
			continue
		}
		for j := r + 1; j < len(l.rows) && !l.rows[j].Header; j++ {
			if it := l.rows[j].Item; ok == nil || ok(it) {
				return it
			}
		}
	}
	return item
}

// Styles is how a menu draws its headers.
type Styles struct {
	// Header styles every header.
	Header lipgloss.Style
	// Group, when set, styles each header by its group — a write group in a
	// warning colour, say — in place of Header.
	Group func(group string) lipgloss.Style
}

// For is the style of group's header.
func (s Styles) For(group string) lipgloss.Style {
	if s.Group != nil {
		return s.Group(group)
	}
	return s.Header
}
