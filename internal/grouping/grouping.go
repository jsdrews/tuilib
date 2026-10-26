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

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/surface"
)

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
// the previous one (dir < 0), skipping groups with nothing accepted and
// wrapping past either end, so the jumps cycle the way tab cycles panes.
// Returns item when no other group has anything accepted, or when the layout
// has no headers.
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
	// spans are the groups in display order, each a run of positions in order.
	type span struct{ from, to int }
	var spans []span
	here := 0
	for p := range l.order {
		if p == 0 || l.groupAt(p) != l.groupAt(p-1) {
			spans = append(spans, span{p, p})
		}
		spans[len(spans)-1].to = p + 1
		if p == l.pos[item] {
			here = len(spans) - 1
		}
	}
	step := 1
	if dir < 0 {
		step = -1
	}
	n := len(spans)
	for k := 1; k < n; k++ {
		sp := spans[((here+k*step)%n+n)%n]
		for p := sp.from; p < sp.to; p++ {
			if ok == nil || ok(l.order[p]) {
				return l.order[p]
			}
		}
	}
	return item
}

// groupAt is the group of the item at position p in display order.
func (l Layout) groupAt(p int) string { return l.rows[l.rowOf[l.order[p]]].Group }

// Styles is how a menu draws its headers: as a thin section divider with the
// group's name set into it, the way a pane sets its title into its border.
//
//	── read ──────────────
//
// On a card there is no border for the rule to meet, and a rule stopping at
// the fill reads as a stray line rather than a title set into an edge, so an
// Inset header puts the name in the label column and trails the rule after it:
//
//	  read ────────────
//	▸ refresh
type Styles struct {
	// Header styles every header's name.
	Header lipgloss.Style
	// Group, when set, styles each header's name by its group — a write group
	// in a warning colour, say — in place of Header.
	Group func(group string) lipgloss.Style
	// Rule styles the divider either side of the name.
	Rule lipgloss.Style
	// Glyph is the divider's glyph, one cell wide. Empty draws "─".
	Glyph string
	// Inset draws the card form. Set it from Joins.
	Inset bool
}

// Joins reports whether a divider drawn edge to edge would meet a line at
// each end: the content's nearest edge draws box-drawing lines on both sides,
// as a line border does. A card's edge is fill, a bar or a shadow.
func Joins(s surface.Surface) bool {
	for i := len(s.Rings) - 1; i >= 0; i-- {
		r := s.Rings[i]
		if r.Left.Drawn() || r.Right.Drawn() {
			return boxLine(r.Left.Glyph) && boxLine(r.Right.Glyph)
		}
	}
	return false
}

func boxLine(g string) bool {
	r, _ := utf8.DecodeRuneInString(g)
	return r >= 0x2500 && r <= 0x257f
}

// For is the style of group's header name.
func (s Styles) For(group string) lipgloss.Style {
	if s.Group != nil {
		return s.Group(group)
	}
	return s.Header
}

// lead is how many glyphs come before the name, which lines the name up just
// past the rows' cursor column.
const lead = 2

// Width is the narrowest a header for group draws in full: the name with a
// space and a lead's worth of rule on each side.
func Width(group string) int { return ansi.StringWidth(group) + 2*(lead+1) }

// Render draws group's header w cells wide: the rule, the name, and the rule
// again to the edge. A name with no room is truncated before the rule is.
func (s Styles) Render(group string, w int) string {
	if w <= 0 {
		return ""
	}
	if s.Inset {
		return s.inset(group, w)
	}
	room := w - lead - 2
	if room <= 0 {
		return s.rule(w)
	}
	name := group
	if ansi.StringWidth(name) > room {
		name = ansi.Truncate(name, room, "…")
	}
	return s.rule(lead) + " " + s.For(group).Render(name) + " " + s.rule(room-ansi.StringWidth(name))
}

// inset is the card form: the name where the rows' labels start, then the
// rule to the edge.
func (s Styles) inset(group string, w int) string {
	room := w - lead - 1
	if room <= 0 {
		return strings.Repeat(" ", w)
	}
	name := group
	if ansi.StringWidth(name) > room {
		name = ansi.Truncate(name, room, "…")
	}
	return strings.Repeat(" ", lead) + s.For(group).Render(name) + " " + s.rule(room-ansi.StringWidth(name))
}

func (s Styles) rule(n int) string {
	if n <= 0 {
		return ""
	}
	g := s.Glyph
	if g == "" {
		g = "─"
	}
	return s.Rule.Render(strings.Repeat(g, n))
}
