// Package list provides a cursor-driven, optionally filterable list inside a
// bordered pane. It bundles item storage, cursor tracking, viewport auto-
// scroll, and a filter.Model together so parents can drop it in with one
// New + Update + View.
//
// Items are plain []string — callers format their data before passing it in.
// For filtering, the match is a case-insensitive substring across the item
// text. Anything richer (fuzzy match, per-field search, struct items) is out
// of scope and should be composed via pane + filter directly.
package list

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Options configures a new list. Zero-value fields fall back to sane defaults
// where that's meaningful; otherwise the pane/filter defaults apply.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "List".
	Title string
	// Items is the full item set. The list copies this slice so the caller
	// can mutate their source independently.
	Items []string
	// Filterable embeds a filter.Model above the body pane (three rows). If
	// false, "/" is ignored and the full height is used for items.
	Filterable bool

	// Pane pass-throughs. See pkg/pane.Options for defaults.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle

	// HScrollbar reserves a row at the bottom of the list pane for a
	// horizontal scrollbar and lets ←/h and →/l scroll long rows
	// horizontally. theme.List() enables this by default — disable when
	// items are guaranteed short and the extra row is unwanted.
	HScrollbar bool

	// SelectedColor foregrounds the highlighted row (bold).
	SelectedColor lipgloss.TerminalColor

	// Markable adds a mark column and binds x / X / A / D, so the
	// user can build a multi-selection the screen reads back with Selection().
	//
	// Off by default, and it costs a row nothing when off: the gutter stays
	// two cells wide. Marking requires keyed items (SetKeyedItems) — see
	// mark.go for why holding marks by index is not an option.
	Markable bool

	// MarkStyle colors the ✓ on a marked row that isn't under the cursor.
	// The cursor row is drawn as one styled run instead, so its highlight
	// cannot be broken mid-row (rule 19).
	MarkStyle lipgloss.Style

	// SpinnerStyle is applied to the spinner glyph rendered while the list
	// is in its loading state (see SetLoading). Pass via theme.List() for
	// a sensible default.
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading — e.g.
	// "loading cities…".
	LoadingLabel string

	// Filter configures the embedded filter. Ignored when Filterable=false.
	Filter filter.Options

	// Keys is the list's keymap. Leave zero to use DefaultKeys; set
	// individual bindings to override (others fall back to defaults via
	// fillDefaults). theme.List() pre-populates this.
	Keys Keys
}

// Keys is the list's keymap. Each binding carries both its dispatch keys
// (WithKeys) and its help label (WithHelp) — Update and Help() read from
// the same struct, so a custom binding propagates everywhere. The embedded
// pane.Keys covers horizontal scroll; mutate fields on Pane to override
// h-scroll without touching the rest.
type Keys struct {
	Up, Down         key.Binding
	Top, Bottom      key.Binding
	HalfUp, HalfDown key.Binding
	Filter           key.Binding
	Mark, MarkAll    key.Binding
	// ClearMarks drops the whole selection unconditionally.
	//
	// Separate from MarkAll because MarkAll is a toggle over the *visible*
	// rows: from a partial selection it marks the rest before a second press
	// clears, so "undo my selection" would otherwise route through a state
	// where everything is marked — an alarming detour on a screen whose next
	// keystroke might be a destructive verb. This key always does the same
	// thing regardless of what is currently marked.
	ClearMarks key.Binding

	// MarkRange extends the selection from the anchor (the most recently
	// marked row) to the cursor. Forward only; see mark.go.
	MarkRange key.Binding
	Pane      pane.Keys
}

// DefaultKeys returns the list's stock keymap. Mutate the returned value
// to override individual actions.
func DefaultKeys() Keys {
	return Keys{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ up")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ down")),
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		// space already means "toggle" in tree, inspector and toggle, so
		// marking extends the existing vocabulary rather than inventing one.
		// Neither list nor table bound it.
		Mark:       key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "mark")),
		MarkAll:    key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "mark all")),
		ClearMarks: key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "drop marks")),
		MarkRange:  key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "mark to here")),
		Pane:       pane.DefaultKeys(),
	}
}

// fillDefaults fills any zero-valued binding in k with its DefaultKeys()
// counterpart, so partial overrides on Options.Keys work without restating
// every field.
func (k *Keys) fillDefaults() {
	d := DefaultKeys()
	if len(k.Up.Keys()) == 0 {
		k.Up = d.Up
	}
	if len(k.Down.Keys()) == 0 {
		k.Down = d.Down
	}
	if len(k.Top.Keys()) == 0 {
		k.Top = d.Top
	}
	if len(k.Bottom.Keys()) == 0 {
		k.Bottom = d.Bottom
	}
	if len(k.HalfUp.Keys()) == 0 {
		k.HalfUp = d.HalfUp
	}
	if len(k.HalfDown.Keys()) == 0 {
		k.HalfDown = d.HalfDown
	}
	if len(k.Filter.Keys()) == 0 {
		k.Filter = d.Filter
	}
	if len(k.Mark.Keys()) == 0 {
		k.Mark = d.Mark
	}
	if len(k.MarkAll.Keys()) == 0 {
		k.MarkAll = d.MarkAll
	}
	if len(k.ClearMarks.Keys()) == 0 {
		k.ClearMarks = d.ClearMarks
	}
	if len(k.MarkRange.Keys()) == 0 {
		k.MarkRange = d.MarkRange
	}
	k.Pane.FillDefaults()
}

// KeyedItem is a list entry with a stable identity. Pass to SetKeyedItems
// to preserve the cursor across data swaps even when items are reordered
// or partially replaced — e.g. polled refreshes of a live data set.
type KeyedItem struct {
	Key     string
	Display string
}

// SelectedChangedMsg is emitted by the list when the cursor lands on a
// different item than the last time we emitted — after cursor movement,
// after a SetItems / filter operation that changes which item is under
// the cursor, or on the initial view. Parents subscribe to it to drive
// "detail on hover" patterns: refetch a parameterized detail source
// keyed on the focused item. Dedup is on (index, item) so a SetItems
// swap that lands the same content under the cursor doesn't re-emit; a
// swap that changes the content does. Empty=true fires only as a
// transition (had focus → no focus) — an initially-empty list never
// emits.
type SelectedChangedMsg struct {
	// Index is the cursor's position in the post-filter visible slice.
	// Zero when Empty is true.
	Index int
	// Item is the currently focused item's string value. Empty when
	// Empty is true.
	Item string
	// Empty is true when no item is focused (empty visible set or
	// cursor out of range). Index / Item are zero-valued.
	Empty bool
}

// Model is the list widget. Embed as a value; mutate via the setters.
type Model struct {
	items   []string
	visible []string
	// visibleIdx maps visible[i] back to items[visibleIdx[i]]. Used by
	// SelectedIndex so callers with parallel source data can recover the
	// original (pre-filter) row position from a selection.
	visibleIdx []int
	// itemKeys parallels items when the list was populated via
	// SetKeyedItems; nil when items are anonymous strings.
	itemKeys []string
	cursor   int

	filter     filter.Model
	filterable bool

	body          pane.Pane
	selectedStyle lipgloss.Style
	hScrollbar    bool

	// marks holds the multi-selection by item key. See mark.go.
	markable bool
	marks    map[string]bool
	// markAnchor is the key of the most recently marked row — the fixed end
	// of a MarkRange. Held as a key, like the marks themselves, so a reorder
	// cannot slide the anchor onto a different row.
	markAnchor string
	markStyle  lipgloss.Style

	keys Keys

	// token is this list's stable identity for focus requests. Update takes
	// a value receiver, so the model cannot name its own address; the token
	// is copied along with the model and stays constant.
	token focus.Token

	// filterRule{Active,Inactive} draw the line separating the inline filter
	// row from the items. The active one is used while the filter has input —
	// an inline filter has no border of its own to light up, so the prompt
	// and this rule carry that signal instead.
	filterRuleActive   lipgloss.Style
	filterRuleInactive lipgloss.Style

	// Focus tracking for SelectedChangedMsg. focusInit flips true after
	// the first emit so an initially-empty list doesn't send an Empty
	// message before any items exist. focusIdx / -Item hold the last
	// emitted (index, value) for dedup; focusIdx == -1 means the last
	// emit was Empty.
	focusInit    bool
	focusIdx     int
	focusItem    string
	focusPending bool
}

// New constructs a list. Call Update/View from the parent model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "List"
	}
	opts.Keys.fillDefaults()
	m := Model{
		token:              focus.NewToken(),
		filterRuleActive:   lipgloss.NewStyle().Foreground(opts.ActiveColor),
		filterRuleInactive: lipgloss.NewStyle().Foreground(opts.InactiveColor),
		items:              append([]string(nil), opts.Items...),
		filterable:         opts.Filterable,
		selectedStyle:      lipgloss.NewStyle().Bold(true).Foreground(opts.SelectedColor),
		hScrollbar:         opts.HScrollbar,
		keys:               opts.Keys,
		markable:           opts.Markable,
		marks:              map[string]bool{},
		markStyle:          opts.MarkStyle,
		focusIdx:           -1,
	}
	m.visible = m.items
	m.visibleIdx = identityIndex(len(m.items))

	bodyH := opts.Height
	if m.filterable {
		bodyH = max(0, opts.Height-3) // filter.Model is 3 rows tall.
		fOpts := opts.Filter
		fOpts.Width = opts.Width
		m.filter = filter.New(fOpts)
	}

	m.body = pane.New(pane.Options{
		Width:          opts.Width,
		Height:         bodyH,
		Title:          opts.Title,
		Focused:        true,
		ActiveColor:    opts.ActiveColor,
		InactiveColor:  opts.InactiveColor,
		ActiveBorder:   opts.ActiveBorder,
		InactiveBorder: opts.InactiveBorder,
		SlotBrackets:   opts.SlotBrackets,
		HScrollbar:     opts.HScrollbar,
		SpinnerStyle:   opts.SpinnerStyle,
		LoadingLabel:   opts.LoadingLabel,
		Keys:           opts.Keys.Pane,
	})
	m.refresh()
	return m
}

func (m *Model) applyFilter() {
	if !m.filterable {
		m.visible = m.items
		m.visibleIdx = identityIndex(len(m.items))
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		m.visible = m.items
		m.visibleIdx = identityIndex(len(m.items))
	} else {
		out := make([]string, 0, len(m.items))
		idx := make([]int, 0, len(m.items))
		for i, it := range m.items {
			if strings.Contains(strings.ToLower(it), q) {
				out = append(out, it)
				idx = append(idx, i)
			}
		}
		m.visible = out
		m.visibleIdx = idx
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func identityIndex(n int) []int {
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = i
	}
	return out
}

// halfPage is the cursor step for ctrl+u/ctrl+d — half the viewport,
// floor 1 so it always moves at least one row even on tiny panes.
func (m Model) halfPage() int {
	if n := m.body.VisibleRows() / 2; n > 0 {
		return n
	}
	return 1
}

func (m *Model) refresh() {
	var b strings.Builder
	for i, it := range m.visible {
		switch {
		case i == m.cursor:
			// One styled run over the whole row: a nested mark style would
			// close the highlight at its first reset (rule 19).
			b.WriteString(m.selectedStyle.Render(m.prefixFor(i) + it))
		case m.markable && m.isMarkedAt(i):
			b.WriteString(" " + m.markStyle.Render(markGlyph) + " " + it)
		default:
			b.WriteString(m.prefixFor(i) + it)
		}
		b.WriteString("\n")
	}
	if m.filterable {
		m.body.SetHeader(m.filterHeader())
	}
	m.body.SetContent(strings.TrimRight(b.String(), "\n"))
	m.body.EnsureVisible(m.cursor)
	if m.filterable {
		m.body.SetBottomLeft(fmt.Sprintf("%d / %d", len(m.visible), len(m.items)))
	}
	m.noteFocus()
}

// noteFocus samples the current focused item and, if it differs from the
// last emitted (index, item) tuple, marks a SelectedChangedMsg for the
// next flush. An empty visible slice is only reported once we've
// previously emitted a non-empty focus — an initially-empty list skips
// the msg so parents don't get an Empty ping before any items exist.
func (m *Model) noteFocus() {
	empty := len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible)
	if empty {
		if !m.focusInit || m.focusIdx == -1 {
			return
		}
		m.focusIdx = -1
		m.focusItem = ""
		m.focusPending = true
		return
	}
	item := m.visible[m.cursor]
	if m.focusInit && m.focusIdx == m.cursor && m.focusItem == item {
		return
	}
	m.focusIdx = m.cursor
	m.focusItem = item
	m.focusPending = true
}

// flushMsgs returns a tea.Cmd carrying the pending SelectedChangedMsg
// (or nil when nothing has changed) and clears the pending flag. Every
// Update return path batches this into its returned cmd.
func (m *Model) flushMsgs() tea.Cmd {
	if !m.focusPending {
		return nil
	}
	m.focusPending = false
	m.focusInit = true
	if m.focusIdx < 0 {
		return func() tea.Msg { return SelectedChangedMsg{Empty: true} }
	}
	idx, item := m.focusIdx, m.focusItem
	return func() tea.Msg {
		return SelectedChangedMsg{Index: idx, Item: item}
	}
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// FocusToken returns the list's stable focus identity. See focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// ActivatedMsg is emitted when the user opens the selected item with a
// double click — the mouse spelling of enter (rule 14). Index and Item match
// the selection at the moment of the second click.
//
// Token identifies which list sent it, so a screen holding several lists can
// tell them apart. Prefer IsActivate over matching this type directly unless
// you need the payload.
type ActivatedMsg struct {
	Index int
	Item  string
	Token focus.Token
}

// IsActivate reports whether msg means "open this list's selection" — enter
// from the keyboard while the filter isn't taking input, or this list's own
// double-click activation.
//
// Rule 14 makes those the same verb, and routing them through one predicate
// is what keeps them that way: a screen writes the open branch once and both
// inputs reach it.
//
//	if s.menu.IsActivate(msg) {
//	    return s, screen.Push(detailFor(s.menu.Cursor()))
//	}
func (m Model) IsActivate(msg tea.Msg) bool {
	switch k := msg.(type) {
	case tea.KeyMsg:
		return !m.Filtering() && k.String() == "enter"
	case ActivatedMsg:
		return k.Token == m.token
	}
	return false
}

// handleMouse routes a mouse event that may or may not belong to this list.
//
// The wheel does exactly what the up and down arrows do (rule 23), and it
// works on whichever list is under the pointer whether or not that list has
// focus — scrolling is navigation, not a claim on the keyboard. A press,
// by contrast, both moves the cursor and asks for focus.
//
// Events outside the list's rect return untouched so a sibling can claim
// them. Rect.Hit also rejects events when this list wasn't drawn in the
// current frame, so a list hidden behind a modal or on an inactive tab
// declines everything without knowing it is hidden.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if target, ok := m.body.HandleScrollbar(e); ok {
		m.scrollTo(target)
		return m, m.flushMsgs()
	}
	// The filter pane owns its own rows; a click there focuses the filter
	// rather than moving the cursor.
	if m.filterable && m.filter.Rect().Hit(e.X, e.Y) {
		if e.IsPointPress() {
			return m, tea.Batch(m.FocusFilter(), focus.RequestSelf(m.token), m.flushMsgs())
		}
		return m, m.flushMsgs()
	}

	// Any press elsewhere in this component's pane hands input back to the
	// body — the rule, the content, blank space below it, the scrollbar row,
	// the borders. Leaving the filter focused after a click into the body is
	// invisible and keeps swallowing keys. Scrollbar presses returned above,
	// so dragging the bar leaves a query alive (rule 23: scrolling never
	// claims the keyboard).
	if e.IsPointPress() && m.body.Rect().Hit(e.X, e.Y) {
		m.BlurFilter()
	}

	switch {
	case e.IsWheelUp():
		if !m.body.Rect().Hit(e.X, e.Y) {
			return m, nil
		}
		m.moveCursor(-1)
		return m, m.flushMsgs()

	case e.IsWheelDown():
		if !m.body.Rect().Hit(e.X, e.Y) {
			return m, nil
		}
		m.moveCursor(1)
		return m, m.flushMsgs()

	case e.IsPointPress():
		row, ok := m.body.RowAt(e.X, e.Y)
		if !ok {
			return m, nil
		}
		m.body.SetFocused(true)
		cmds := []tea.Cmd{focus.RequestSelf(m.token)}
		if row < len(m.visible) {
			m.cursor = row
			// Clicking the ✓ column toggles the mark, the way a tree's ▸
			// toggles on a single click — library-drawn chrome is clickable
			// (rule 28). It returns before the activate branch: a double
			// click on the mark column would otherwise toggle twice and open
			// the row as well, which is nobody's intent.
			if e.IsPress() && m.markable && m.onMarkColumn(e.X) {
				m.toggleMarkAt(row)
				cmds = append(cmds, m.flushMsgs())
				return m, tea.Batch(cmds...)
			}
			m.refresh()
			if e.IsDoubleClick() {
				idx, item, tok := m.cursor, m.visible[m.cursor], m.token
				cmds = append(cmds, func() tea.Msg {
					return ActivatedMsg{Index: idx, Item: item, Token: tok}
				})
			}
		}
		cmds = append(cmds, m.flushMsgs())
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// scrollTo puts row at the top of the view and moves the cursor onto it.
//
// Moving the cursor is what makes the scroll stick. refresh re-asserts "the
// cursor is visible" on every frame, and layout.Sized calls SetRect — hence
// refresh — on every render, so a viewport moved on its own is undone one
// frame later and the view snaps back to wherever the cursor still was.
func (m *Model) scrollTo(row int) {
	n := len(m.visible)
	if n == 0 {
		return
	}
	if row < 0 {
		row = 0
	}
	if row >= n {
		row = n - 1
	}
	m.cursor = row
	m.refresh()
	// refresh pulled the viewport to the cursor; put the dragged row back
	// at the top. The next frame's EnsureVisible is then a no-op, because
	// the cursor is already the first visible row.
	m.body.SetYOffset(row)
}

// moveCursor steps the cursor by delta, clamped to the visible set.
func (m *Model) moveCursor(delta int) {
	next := m.cursor + delta
	if next < 0 || next >= len(m.visible) {
		return
	}
	m.cursor = next
	m.refresh()
}

// Update consumes up/down/j/k and "/" (when filterable); while the filter is
// focused, every key is forwarded to it. Mouse events inside the list's rect
// move the cursor and request focus. Non-key messages are forwarded to the
// body pane so spinner ticks reach the loading-state animation.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if mm, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(mm)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, tea.Batch(cmd, m.flushMsgs())
	}
	if m.filterable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// enter commits and esc cancels, both of which blur the filter from
		// the inside; the body takes the highlight back when they do.
		if !m.filter.Focused() {
			m.body.SetFocused(true)
		}
		m.applyFilter()
		m.refresh()
		return m, tea.Batch(cmd, m.flushMsgs())
	}
	switch {
	case m.filterable && key.Matches(km, m.keys.Filter):
		return m, tea.Batch(m.FocusFilter(), m.flushMsgs())
	case key.Matches(km, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.refresh()
		}
	case key.Matches(km, m.keys.Down):
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refresh()
		}
	case key.Matches(km, m.keys.Top):
		if m.cursor != 0 && len(m.visible) > 0 {
			m.cursor = 0
			m.refresh()
		}
	case key.Matches(km, m.keys.Bottom):
		if last := len(m.visible) - 1; last >= 0 && m.cursor != last {
			m.cursor = last
			m.refresh()
		}
	case key.Matches(km, m.keys.HalfUp):
		if m.cursor > 0 {
			m.cursor = max(0, m.cursor-m.halfPage())
			m.refresh()
		}
	case key.Matches(km, m.keys.HalfDown):
		if last := len(m.visible) - 1; last >= 0 && m.cursor < last {
			m.cursor = min(last, m.cursor+m.halfPage())
			m.refresh()
		}
	case m.markable && key.Matches(km, m.keys.Mark):
		m.ToggleMark()
	case m.markable && key.Matches(km, m.keys.MarkAll):
		m.ToggleMarkAll()
	case m.markable && key.Matches(km, m.keys.ClearMarks):
		m.ClearMarks()
	case m.markable && key.Matches(km, m.keys.MarkRange):
		m.MarkRange()
	default:
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, tea.Batch(cmd, m.flushMsgs())
	}
	return m, m.flushMsgs()
}

// View stacks filter (if filterable) and the body pane.
func (m Model) View() string { return m.body.View() }

// Rect is the area the list occupies, for a caller that needs to test a
// position against it before acting — deciding whether a right-click landed
// on this list, say. Matches the accessor input, toggle, form and confirm
// already expose.
func (m Model) Rect() geom.Rect { return m.body.Rect() }

// Selected returns the currently highlighted item. ok is false when the
// visible set (post-filter) is empty.
func (m Model) Selected() (string, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return "", false
	}
	return m.visible[m.cursor], true
}

// SelectedIndex returns the highlighted row's index into the original
// (pre-filter) Items() slice. ok is false when the visible set is empty.
//
// Use this when list items are formatted display strings rendered from
// a richer source slice — SelectedIndex identifies which source row is
// selected, regardless of whether a filter is currently applied. For
// the row's text, use Selected; for the cursor's position within the
// filtered view, use Cursor.
func (m Model) SelectedIndex() (int, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visibleIdx) {
		return 0, false
	}
	return m.visibleIdx[m.cursor], true
}

// Cursor returns the current cursor index into the visible (post-filter) set.
func (m Model) Cursor() int { return m.cursor }

// Visible returns the post-filter items, in display order.
func (m Model) Visible() []string { return m.visible }

// Items returns the full unfiltered item set.
func (m Model) Items() []string { return m.items }

// Filtering reports whether the embedded filter currently has focus —
// callers use this to decide whether to intercept global keys like "q".
func (m Model) Filtering() bool { return m.filterable && m.filter.Focused() }

// IsCapturingKeys reports whether the list currently swallows printable
// keys — true while its filter is focused. Satisfies focus.Capturer, so a
// focus.Group can answer the app shell's global-key gating (rule 5) without
// the screen restating it.
func (m Model) IsCapturingKeys() bool { return m.Filtering() }

// Help returns the keys this list responds to. While the embedded filter
// is focused it returns the filter's keys; otherwise the configured
// nav/scroll/filter bindings from m.keys.
func (m Model) Help() []key.Binding {
	if m.filterable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		m.keys.Up, m.keys.Down,
		m.keys.HalfUp, m.keys.HalfDown,
		m.keys.Top, m.keys.Bottom,
	}
	if m.hScrollbar {
		out = append(out, m.body.HelpBindings()...)
	}
	if m.filterable {
		out = append(out, m.keys.Filter)
	}
	if m.markable {
		out = append(out, m.keys.Mark, m.keys.MarkRange, m.keys.MarkAll, m.keys.ClearMarks,
			key.NewBinding(key.WithKeys("mouse:mark"), key.WithHelp("click ✓", "mark")))
	}
	return out
}

// Value returns the current filter text ("" when not filterable or empty).
func (m Model) Value() string {
	if m.filterable {
		return m.filter.Value()
	}
	return ""
}

// SetRect places the list in the given rect. When filterable, the internal
// filter pane consumes the top 3 rows and the body pane gets the rest,
// offset below it; otherwise the body pane takes the whole rect. Each child
// receives its own absolute rect so a click resolves to the right one.
func (m *Model) SetRect(r geom.Rect) {
	m.body.SetRect(r)
	if m.filterable {
		// The filter is a row inside the body pane, not a pane beside it, so
		// it reads as belonging to the list it filters. Setting the header
		// first would be circular — the header's width comes from the pane —
		// so place the pane, measure, then fill the header and let the pane
		// re-measure.
		inner := m.body.ContentRect()
		m.filter.SetInlineRect(geom.Rect{X: inner.X, Y: m.body.Rect().Y + 1, W: inner.W, Gen: r.Gen, H: 1})
		m.body.SetHeader(m.filterHeader())
		inner = m.body.ContentRect()
		m.filter.SetInlineRect(geom.Rect{X: inner.X, Y: m.body.Rect().Y + 1, W: inner.W, Gen: r.Gen, H: 1})
	}
	m.refresh()
}

// filterHeader is the filter row plus a rule separating it from the items.
func (m Model) filterHeader() string {
	inner := m.body.ContentRect().W
	if inner <= 0 {
		return ""
	}
	rule := m.filterRuleInactive
	if m.filter.Focused() {
		rule = m.filterRuleActive
	}
	return m.filter.InlineView() + "\n" + rule.Render(strings.Repeat("─", inner))
}

// SetItems replaces the item set, re-applies the current filter, and redraws.
// Clears any per-item keys previously set via SetKeyedItems.
func (m *Model) SetItems(items []string) {
	m.items = append([]string(nil), items...)
	m.itemKeys = nil
	m.applyFilter()
	m.refresh()
}

// SetKeyedItems replaces the item set with keyed entries and snaps the cursor
// to the previously-selected Key after the swap (falling back to the clamped
// previous cursor index when the key is gone). Use this for polled refreshes
// of a live data set so the user's selection survives reordering or partial
// replacement.
func (m *Model) SetKeyedItems(items []KeyedItem) {
	prevKey, hadKey := m.SelectedKey()
	prevCursor := m.cursor
	m.items = make([]string, len(items))
	m.itemKeys = make([]string, len(items))
	for i, it := range items {
		m.items[i] = it.Display
		m.itemKeys[i] = it.Key
	}
	m.applyFilter()
	if hadKey {
		for i, src := range m.visibleIdx {
			if src >= 0 && src < len(m.itemKeys) && m.itemKeys[src] == prevKey {
				m.cursor = i
				m.refresh()
				return
			}
		}
	}
	if last := len(m.visible) - 1; last >= 0 {
		m.cursor = max(0, min(prevCursor, last))
	} else {
		m.cursor = 0
	}
	m.refresh()
}

// SelectedKey returns the Key of the currently highlighted item when the list
// was populated via SetKeyedItems. ok is false when the visible set is empty
// or when items are anonymous (no keys set).
func (m Model) SelectedKey() (string, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visibleIdx) {
		return "", false
	}
	src := m.visibleIdx[m.cursor]
	if src < 0 || src >= len(m.itemKeys) {
		return "", false
	}
	return m.itemKeys[src], true
}

// SetCursor moves the cursor (clamped to the visible range) and scrolls to
// keep it on screen.
func (m *Model) SetCursor(n int) {
	m.cursor = max(0, min(n, len(m.visible)-1))
	m.refresh()
}

// Deselect moves the cursor off every row, so nothing is highlighted and
// Selected reports ok=false. Use it for a list that must start with no
// choice made — a form select that demands a deliberate pick.
//
// SetCursor deliberately does not accept -1: it clamps, so a stray negative
// index cannot silently blank a list. Deselecting is a distinct intent and
// gets a distinct call.
func (m *Model) Deselect() {
	m.cursor = -1
	m.refresh()
}

// SetValue overwrites the filter text (no-op when not filterable). Useful
// when rebuilding the list on theme swap / resize — carry the old Value().
func (m *Model) SetValue(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.applyFilter()
	m.refresh()
}

// Focus gives the component the keyboard, highlighting the body pane.
//
// A filterable list has two focusable regions behind one Focusable, so this
// deliberately does nothing when the filter already owns input: a click on
// the filter also asks the group for focus, and the grant arrives afterwards.
// Without this guard that grant would snatch the highlight back to the body
// while the filter kept the keystrokes.
func (m *Model) Focus() tea.Cmd {
	m.body.SetFocused(true)
	return nil
}

// Blur releases the keyboard, clearing *both* regions. Leaving a filter
// focused on a blurred component is what lets a second filterable pane end
// up invisibly eating keys.
func (m *Model) Blur() {
	m.body.SetFocused(false)
	if m.filterable {
		m.filter.Blur()
		m.body.SetHeader(m.filterHeader())
	}
}

// Focused reports whether either of the component's regions owns input.
func (m Model) Focused() bool {
	return m.body.Focused() || (m.filterable && m.filter.Focused())
}

// FocusFilter moves input to the filter and takes the highlight off the
// body, so exactly one region ever reads as active.
func (m *Model) FocusFilter() tea.Cmd {
	if !m.filterable {
		return nil
	}
	// The filter lives inside the body pane, so the pane stays lit — it is
	// the component that has focus. The filter row shows where input goes.
	m.body.SetFocused(true)
	cmd := m.filter.Focus()
	m.body.SetHeader(m.filterHeader())
	return cmd
}

// BlurFilter returns input from the filter to the body.
func (m *Model) BlurFilter() {
	if !m.filterable {
		return
	}
	m.filter.Blur()
	m.body.SetFocused(true)
	m.body.SetHeader(m.filterHeader())
}

// SetTitle updates the title rendered on the body pane's top border.
// Useful when the list represents a slice that can change identity at
// runtime (e.g. "detail · <selection>").
// SetTopRight writes into the pane's top-right border slot. Hosts use it for
// a short annotation that belongs to the component as a whole — pkg/form
// paints validation errors there. Pass "" to clear.
func (m *Model) SetTopRight(s string) { m.body.SetTopRight(s) }

func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

// SetActiveColor updates the body pane's active border color. Useful when
// reacting to a theme swap without rebuilding the model.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.body.SetActiveColor(c) }

// SetInactiveColor updates the body pane's inactive border color.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

// SetSelectedColor updates the foreground color of the highlighted row.
func (m *Model) SetSelectedColor(c lipgloss.TerminalColor) {
	m.selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(c)
	m.refresh()
}

// Loading reports whether the list is currently in its loading state.
func (m Model) Loading() bool { return m.body.Loading() }

// SetLoading toggles the loading state. When entering, returns the
// spinner's initial Tick command — propagate it back from your screen's
// Update so the spinner animates. The list's Update already forwards
// every msg to the body pane, so subsequent ticks chain automatically.
func (m *Model) SetLoading(b bool) tea.Cmd { return m.body.SetLoading(b) }

// SetLoadingLabel updates the text rendered next to the spinner while
// loading.
func (m *Model) SetLoadingLabel(s string) { m.body.SetLoadingLabel(s) }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
func (m *Model) SetSpinnerStyle(s lipgloss.Style) { m.body.SetSpinnerStyle(s) }
