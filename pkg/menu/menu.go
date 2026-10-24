// Package menu is a grouped, filterable list of things to open: the sections
// of an app, the screens a home page leads to. Each item is a label, a dimmed
// description beside it, and an optional group it is filed under; choosing
// one emits a ChosenMsg and the screen decides what opening it means —
// usually screen.Push.
//
// It is a pane component like pkg/list, and sits beside other components the
// same way: it takes focus, filters with /, and answers the mouse from its own
// rect. What it adds over a list is the groups, drawn as headers the cursor
// never lands on, by the same rules pkg/action's menu draws them:
//
//   - headers only once the items name two or more groups;
//   - groups in the order first seen, header text as written;
//   - items without a group last, under "other";
//   - [ and ] jump to the previous and next group.
//
// tab is deliberately left alone, because on a screen with more than one
// component it moves focus between them.
//
// A menu is not pkg/action's Menu. That one is a modal over a screen's verbs;
// this one is a screen's content.
package menu

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/internal/grouping"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/surface"
)

const (
	cursorW = 2 // "▸ " / "  "
	colGap  = 2
)

// Item is one entry in the menu.
type Item struct {
	// ID identifies the item to the caller. Optional; when set, it is also
	// what keeps the cursor on the same item across SetItems.
	ID string
	// Label is the text the user picks by. Required.
	Label string
	// Group files the item under a header. See the package doc for the rules.
	Group string
	// Description is drawn dimmed beside the label and cut off when the pane
	// runs out of room. It is not searched by the filter.
	Description string
	// Data is the caller's payload, handed back in ChosenMsg. The menu never
	// reads it.
	Data any
}

// ChosenMsg is emitted when the user opens an item — enter, or a double
// click. Token says which menu sent it; Chosen matches both for you.
type ChosenMsg struct {
	Item  Item
	Token focus.Token
}

// Keys is the menu's keymap. Vertical movement follows rule 25.
type Keys struct {
	Up, Down         key.Binding
	Top, Bottom      key.Binding
	HalfUp, HalfDown key.Binding
	// PrevGroup and NextGroup jump to the first item of the previous or next
	// group. They do nothing when the menu has no headers.
	PrevGroup, NextGroup key.Binding
	Filter               key.Binding
	Choose               key.Binding
}

// DefaultKeys returns the menu's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ up")),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ down")),
		PrevGroup: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev group")),
		NextGroup: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next group")),
		Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Choose:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
	}
}

// fillDefaults fills any zero-valued binding with its DefaultKeys
// counterpart, so partial overrides work without restating every field.
func (k *Keys) fillDefaults() {
	d := DefaultKeys()
	for _, p := range []struct {
		dst *key.Binding
		src key.Binding
	}{
		{&k.Up, d.Up}, {&k.Down, d.Down},
		{&k.Top, d.Top}, {&k.Bottom, d.Bottom},
		{&k.HalfUp, d.HalfUp}, {&k.HalfDown, d.HalfDown},
		{&k.PrevGroup, d.PrevGroup}, {&k.NextGroup, d.NextGroup},
		{&k.Filter, d.Filter}, {&k.Choose, d.Choose},
	} {
		if len(p.dst.Keys()) == 0 {
			*p.dst = p.src
		}
	}
}

// Options configures a menu. Build it from theme.Menu() and set Title, Items
// and Filterable before passing to New.
type Options struct {
	// Title sits on the pane's top border. Defaults to "Menu".
	Title string
	// Items is the menu's content, copied.
	Items []Item
	// Filterable embeds a filter as a header row inside the pane. / focuses
	// it; it matches the label and the group, case-insensitively.
	Filterable bool

	LabelStyle lipgloss.Style
	// DescStyle styles the description; theme.Menu() dims it.
	DescStyle lipgloss.Style
	// HeaderStyle styles the group headers. GroupStyle, when set, styles each
	// header by its group instead.
	HeaderStyle lipgloss.Style
	GroupStyle  func(group string) lipgloss.Style
	// SelectedStyle is applied to the whole cursor row.
	SelectedStyle lipgloss.Style
	// SelectedBlurredStyle replaces SelectedStyle while the menu doesn't
	// have focus. Used only when it sets a background.
	SelectedBlurredStyle lipgloss.Style

	// Pane pass-throughs. See pkg/pane.Options.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	// Glyphs are the marks this component draws, plus the scrollbar thumb
	// and track it hands to its pane. Empty fields fall back to
	// glyph.Default.
	Glyphs       glyph.Set
	SlotBrackets pane.SlotBracketStyle
	// Surface is how the component's edge and body are drawn. A theme sets
	// it from the app's chosen surface. See docs/surfaces.md.
	Surface surface.Resolved

	// Filter configures the embedded filter. Ignored when Filterable=false.
	Filter filter.Options

	Keys Keys
}

// Model is the menu. Embed as a value; mutate via the setters.
type Model struct {
	glyphs glyph.Set

	items []Item
	// layout is the items the filter keeps, in display order under their
	// headers. cursor is an index into items, or -1 when nothing is kept.
	layout grouping.Layout
	cursor int

	filter     filter.Model
	filterable bool

	body                      pane.Pane
	labelStyle, descStyle     lipgloss.Style
	selectedStyle, selBlurred lipgloss.Style
	headers                   grouping.Styles
	keys                      Keys

	// token is the menu's identity in focus requests and ChosenMsg. Update
	// takes a value receiver, so the model cannot name its own address.
	token focus.Token

	filterRuleActive, filterRuleInactive lipgloss.Style
}

// New constructs a menu.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "Menu"
	}
	opts.Keys.fillDefaults()
	m := Model{
		glyphs:             opts.Glyphs.Resolve(),
		items:              append([]Item(nil), opts.Items...),
		filterable:         opts.Filterable,
		labelStyle:         opts.LabelStyle,
		descStyle:          opts.DescStyle,
		selectedStyle:      opts.SelectedStyle,
		selBlurred:         opts.SelectedBlurredStyle,
		headers:            grouping.Styles{Header: opts.HeaderStyle, Group: opts.GroupStyle},
		keys:               opts.Keys,
		token:              focus.NewToken(),
		filterRuleActive:   lipgloss.NewStyle().Foreground(opts.ActiveColor),
		filterRuleInactive: lipgloss.NewStyle().Foreground(opts.InactiveColor),
	}
	if m.filterable {
		m.filter = filter.New(opts.Filter)
	}
	m.body = pane.New(pane.Options{
		DividerStyle:       m.filterRuleInactive,
		DividerActiveStyle: m.filterRuleActive,
		Title:              opts.Title,
		Focused:            true,
		ActiveColor:        opts.ActiveColor,
		InactiveColor:      opts.InactiveColor,
		ActiveBorder:       opts.ActiveBorder,
		InactiveBorder:     opts.InactiveBorder,
		Glyphs:             opts.Glyphs,
		SlotBrackets:       opts.SlotBrackets,
		Surface:            opts.Surface,
	})
	m.relayout()
	m.cursor = m.layout.First(nil)
	m.refresh()
	return m
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// FocusToken returns the menu's stable focus identity. See focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// Chosen reports whether msg is this menu's ChosenMsg, and the item in it.
//
//	if it, ok := s.menu.Chosen(msg); ok {
//	    return s, screen.Push(it.Data.(func(theme.Theme) screen.Screen)(s.t))
//	}
func (m Model) Chosen(msg tea.Msg) (Item, bool) {
	c, ok := msg.(ChosenMsg)
	if !ok || c.Token != m.token {
		return Item{}, false
	}
	return c.Item, true
}

// relayout re-runs the filter and lays the kept items out under their
// headers, keeping the cursor on its item when the filter kept it.
func (m *Model) relayout() {
	groups := make([]string, len(m.items))
	for i, it := range m.items {
		groups[i] = it.Group
	}
	grouped := grouping.Grouped(groups)
	q := strings.ToLower(strings.TrimSpace(m.Value()))
	keep := func(i int) bool {
		if q == "" {
			return true
		}
		it := m.items[i]
		// Match the group as drawn, so "other" finds the items under that
		// header and nothing when there is no header to read it from.
		g := it.Group
		if grouped {
			g = grouping.Name(g)
		}
		return strings.Contains(strings.ToLower(it.Label), q) ||
			strings.Contains(strings.ToLower(g), q)
	}
	m.layout = grouping.New(groups, keep)
	if !m.layout.Kept(m.cursor) {
		m.cursor = m.layout.First(nil)
	}
}

// refresh redraws the rows and keeps the cursor in view.
func (m *Model) refresh() {
	if m.filterable {
		m.body.SetDividedHeader(m.filter.InlineView(), m.filter.Focused())
		m.body.SetBottomLeft(fmt.Sprintf("%d / %d", m.layout.Items(), len(m.items)))
	}
	w := m.body.ContentRect().W
	labelW := m.labelWidth(w)
	rows := make([]string, 0, m.layout.Len())
	for _, r := range m.layout.Rows() {
		if r.Header {
			rows = append(rows, m.renderHeader(r.Group, w))
			continue
		}
		rows = append(rows, m.renderItem(r.Item, w, labelW))
	}
	m.body.SetContent(strings.Join(rows, "\n"))
	if m.cursor >= 0 {
		if m.layout.HeaderAbove(m.cursor) {
			m.body.EnsureVisible(m.layout.RowOf(m.cursor) - 1)
		}
		m.body.EnsureVisible(m.layout.RowOf(m.cursor))
	}
}

// labelWidth is the label column: the widest label among all items, so
// descriptions line up and do not shift as the filter narrows, capped so a
// long label cannot push every description off the pane.
func (m Model) labelWidth(w int) int {
	lw := 0
	for _, it := range m.items {
		lw = max(lw, xansi.StringWidth(it.Label))
	}
	return max(0, min(lw, w-cursorW))
}

func (m Model) renderHeader(group string, w int) string {
	text := fit(group, w)
	name := strings.TrimRight(text, " ")
	return m.headers.For(group).Render(name) + text[len(name):]
}

func (m Model) renderItem(i, w, labelW int) string {
	it := m.items[i]
	prefix := "  "
	if i == m.cursor {
		prefix = m.glyphs.Cursor + " "
	}
	label := fit(it.Label, labelW)
	descW := w - cursorW - labelW - colGap
	desc := ""
	if it.Description != "" && descW > 0 {
		desc = fit(it.Description, descW)
	}

	if i == m.cursor {
		row := prefix + label
		if desc != "" {
			row += strings.Repeat(" ", colGap) + desc
		}
		if pad := w - xansi.StringWidth(row); pad > 0 {
			row += strings.Repeat(" ", pad)
		}
		return ansi.RenderFill(m.selection().Render, row)
	}
	row := prefix + m.labelStyle.Render(label)
	if desc != "" {
		trimmed := strings.TrimRight(desc, " ")
		row += strings.Repeat(" ", colGap) + m.descStyle.Render(trimmed)
	}
	return row
}

// fit pads s to w, or truncates it with an ellipsis when it is longer.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if n := xansi.StringWidth(s); n > w {
		return xansi.Truncate(s, w, "…")
	} else if n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// selection is the cursor row's style: selectedStyle, or the blurred one
// while the menu is blurred and that style sets a background.
func (m Model) selection() lipgloss.Style {
	if _, unset := m.selBlurred.GetBackground().(lipgloss.NoColor); !m.body.Focused() && !unset {
		return m.selBlurred
	}
	return m.selectedStyle
}

// Update handles keys and mouse. While the filter has input every key goes to
// it; otherwise the menu's own keys move the cursor and enter chooses.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if e, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(e)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	if m.Filtering() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if !m.filter.Focused() {
			m.body.SetFocused(true)
		}
		m.relayout()
		m.refresh()
		return m, cmd
	}
	switch {
	case m.filterable && key.Matches(km, m.keys.Filter):
		return m, m.FocusFilter()
	case key.Matches(km, m.keys.Choose):
		return m, m.choose()
	case key.Matches(km, m.keys.Up):
		m.moveTo(m.layout.Step(m.cursor, -1, nil))
	case key.Matches(km, m.keys.Down):
		m.moveTo(m.layout.Step(m.cursor, 1, nil))
	case key.Matches(km, m.keys.Top):
		m.moveTo(m.layout.First(nil))
	case key.Matches(km, m.keys.Bottom):
		m.moveTo(m.layout.Last(nil))
	case key.Matches(km, m.keys.HalfUp):
		m.moveTo(m.layout.Step(m.cursor, -m.halfPage(), nil))
	case key.Matches(km, m.keys.HalfDown):
		m.moveTo(m.layout.Step(m.cursor, m.halfPage(), nil))
	case key.Matches(km, m.keys.PrevGroup):
		m.moveTo(m.layout.JumpGroup(m.cursor, -1, nil))
	case key.Matches(km, m.keys.NextGroup):
		m.moveTo(m.layout.JumpGroup(m.cursor, 1, nil))
	}
	return m, nil
}

// handleMouse follows rule 28: a click focuses and moves the cursor, a double
// click chooses, the wheel moves the cursor of the menu under the pointer
// whether or not it has focus. A header is not a row the cursor can take, so
// a click on one only focuses.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if row, ok := m.body.HandleScrollbar(e); ok {
		m.scrollTo(row)
		return m, nil
	}
	if m.filterable && m.filter.Rect().Hit(e.X, e.Y) {
		if e.IsPointPress() {
			return m, tea.Batch(m.FocusFilter(), focus.RequestSelf(m.token))
		}
		return m, nil
	}
	inside := m.body.Rect().Hit(e.X, e.Y)
	// Any other press in the pane hands input back from the filter (rule 27).
	if e.IsPointPress() && inside {
		m.BlurFilter()
	}

	switch {
	case e.IsWheelUp():
		if inside {
			m.moveTo(m.layout.Step(m.cursor, -1, nil))
		}
		return m, nil
	case e.IsWheelDown():
		if inside {
			m.moveTo(m.layout.Step(m.cursor, 1, nil))
		}
		return m, nil
	case e.IsPointPress():
		row, ok := m.body.RowAt(e.X, e.Y)
		if !ok {
			return m, nil
		}
		m.body.SetFocused(true)
		cmds := []tea.Cmd{focus.RequestSelf(m.token)}
		if r, ok := m.layout.At(row); ok && !r.Header {
			m.cursor = r.Item
			m.refresh()
			if e.IsDoubleClick() {
				cmds = append(cmds, m.choose())
			}
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// moveTo puts the cursor on item i, when the filter kept it.
func (m *Model) moveTo(i int) {
	if i == m.cursor || !m.layout.Kept(i) {
		return
	}
	m.cursor = i
	m.refresh()
}

// scrollTo is the scrollbar's entry point: it moves the cursor with the view,
// because refresh re-asserts "the cursor is visible" on every frame.
func (m *Model) scrollTo(row int) {
	if m.layout.Len() == 0 {
		return
	}
	row = min(max(row, 0), m.layout.Len()-1)
	m.moveTo(m.layout.ItemNear(row))
	m.body.SetYOffset(row)
}

func (m Model) halfPage() int { return max(1, m.body.VisibleRows()/2) }

func (m Model) choose() tea.Cmd {
	it, ok := m.Selected()
	if !ok {
		return nil
	}
	tok := m.token
	return func() tea.Msg { return ChosenMsg{Item: it, Token: tok} }
}

// View renders the pane.
func (m Model) View() string { return m.body.View() }

// Rect is the area the menu occupies.
func (m Model) Rect() geom.Rect { return m.body.Rect() }

// SetRect places the menu. The filter, when there is one, is a row inside
// the pane, placed once the pane has measured its own edge.
func (m *Model) SetRect(r geom.Rect) {
	m.body.SetRect(r)
	if m.filterable {
		place := func() {
			inner := m.body.ContentRect()
			m.filter.SetInlineRect(geom.Rect{X: inner.X, Y: m.body.InnerRect().Y, W: inner.W, H: 1, Gen: r.Gen})
		}
		place()
		m.body.SetDividedHeader(m.filter.InlineView(), m.filter.Focused())
		place()
	}
	m.refresh()
}

// SetItems replaces the items and re-applies the filter. The cursor stays on
// the item with the same ID when there is one, and otherwise on the same
// position.
func (m *Model) SetItems(items []Item) {
	prev, had := m.Selected()
	cursor := m.cursor
	m.items = append([]Item(nil), items...)
	m.cursor = -1
	if had && prev.ID != "" {
		for i, it := range m.items {
			if it.ID == prev.ID {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor < 0 && cursor >= 0 && cursor < len(m.items) {
		m.cursor = cursor
	}
	m.relayout()
	m.refresh()
}

// Items returns the full, unfiltered item set.
func (m Model) Items() []Item { return m.items }

// Selected returns the item under the cursor. ok is false when the filter
// keeps nothing.
func (m Model) Selected() (Item, bool) {
	if !m.layout.Kept(m.cursor) {
		return Item{}, false
	}
	return m.items[m.cursor], true
}

// Select moves the cursor to the item with this ID, reporting whether one is
// showing.
func (m *Model) Select(id string) bool {
	for i, it := range m.items {
		if it.ID == id && m.layout.Kept(i) {
			m.cursor = i
			m.refresh()
			return true
		}
	}
	return false
}

// Value is the filter text ("" when not filterable).
func (m Model) Value() string {
	if !m.filterable {
		return ""
	}
	return m.filter.Value()
}

// SetValue sets the filter text without it reading as a user action.
func (m *Model) SetValue(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.relayout()
	m.refresh()
}

// Filtering reports whether the filter has input.
func (m Model) Filtering() bool { return m.filterable && m.filter.Focused() }

// IsCapturingKeys satisfies focus.Capturer: true while the filter has input.
func (m Model) IsCapturingKeys() bool { return m.Filtering() }

// Focus gives the menu the keyboard. It never takes input from the filter: a
// click on the filter also asks the group for focus, and that grant lands
// after the click (rule 27).
func (m *Model) Focus() tea.Cmd {
	m.body.SetFocused(true)
	m.refocused()
	return nil
}

// Blur releases the keyboard from both the body and the filter.
func (m *Model) Blur() {
	m.body.SetFocused(false)
	if m.filterable {
		m.filter.Blur()
	}
	m.refocused()
}

// Focused reports whether the body or the filter has input.
func (m Model) Focused() bool { return m.body.Focused() || m.Filtering() }

// FocusFilter moves input to the filter. The pane stays lit: the menu has
// focus, and the filter row shows where the keys go.
func (m *Model) FocusFilter() tea.Cmd {
	if !m.filterable {
		return nil
	}
	m.body.SetFocused(true)
	cmd := m.filter.Focus()
	m.body.SetDividedHeader(m.filter.InlineView(), true)
	return cmd
}

// BlurFilter returns input from the filter to the body.
func (m *Model) BlurFilter() {
	if !m.filterable {
		return
	}
	m.filter.Blur()
	m.body.SetFocused(true)
	m.body.SetDividedHeader(m.filter.InlineView(), false)
}

func (m *Model) refocused() {
	if _, unset := m.selBlurred.GetBackground().(lipgloss.NoColor); !unset {
		m.refresh()
	} else if m.filterable {
		m.body.SetDividedHeader(m.filter.InlineView(), m.filter.Focused())
	}
}

// SetTitle updates the label on the pane's top border.
func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

// Title returns the label on the pane's border.
func (m Model) Title() string { return m.body.Title() }

// Help returns the bindings the menu currently responds to.
func (m Model) Help() []key.Binding { return help.Flatten(m.HelpSections()) }

// HelpSections groups the bindings by what they do. The group keys are
// listed only when there are headers to jump between.
func (m Model) HelpSections() []help.Section {
	if m.Filtering() {
		return help.Sections(help.Group(help.SectionFilter, m.filter.Help()...))
	}
	nav := []key.Binding{m.keys.Up, m.keys.Down, m.keys.HalfUp, m.keys.HalfDown, m.keys.Top, m.keys.Bottom}
	if m.layout.Grouped() {
		nav = append(nav, m.keys.PrevGroup, m.keys.NextGroup)
	}
	var filt []key.Binding
	if m.filterable {
		filt = []key.Binding{m.keys.Filter}
	}
	return help.Sections(
		help.Group(help.SectionNavigate, nav...),
		help.Group(help.SectionFilter, filt...),
		help.Group(help.SectionSubmit, m.keys.Choose,
			key.NewBinding(key.WithKeys("mouse:dblclick"), key.WithHelp("2×click", "open"))),
	)
}

// State is what the user and the data did to a menu — its items, filter,
// cursor and scroll — held across a rebuild (rule 4):
//
//	st := s.menu.State()
//	s.menu = menu.New(opts)
//	s.menu.Restore(st)
type State struct {
	built  bool
	items  []Item
	value  string
	cursor int
	yoff   int
}

// State captures the menu's state.
func (m Model) State() State {
	return State{
		built:  m.token != nil,
		items:  append([]Item(nil), m.items...),
		value:  m.Value(),
		cursor: m.cursor,
		yoff:   m.body.YOffset(),
	}
}

// Restore puts s back: items, then the filter over them, then the cursor.
// A State from a menu never built restores nothing.
func (m *Model) Restore(s State) {
	if !s.built {
		return
	}
	m.items = append([]Item(nil), s.items...)
	if m.filterable {
		m.filter.SetValue(s.value)
	}
	m.relayout()
	if m.layout.Kept(s.cursor) {
		m.cursor = s.cursor
	}
	m.refresh()
	m.body.SetYOffset(s.yoff)
}
