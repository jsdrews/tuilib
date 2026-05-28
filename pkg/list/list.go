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
	Pane             pane.Keys
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
		Pane:     pane.DefaultKeys(),
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
	k.Pane.FillDefaults()
}

// KeyedItem is a list entry with a stable identity. Pass to SetKeyedItems
// to preserve the cursor across data swaps even when items are reordered
// or partially replaced — e.g. polled refreshes of a live data set.
type KeyedItem struct {
	Key     string
	Display string
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

	keys Keys
}

// New constructs a list. Call Update/View from the parent model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "List"
	}
	opts.Keys.fillDefaults()
	m := Model{
		items:         append([]string(nil), opts.Items...),
		filterable:    opts.Filterable,
		selectedStyle: lipgloss.NewStyle().Bold(true).Foreground(opts.SelectedColor),
		hScrollbar:    opts.HScrollbar,
		keys:          opts.Keys,
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
		if i == m.cursor {
			b.WriteString(m.selectedStyle.Render("▸ " + it))
		} else {
			b.WriteString("  " + it)
		}
		b.WriteString("\n")
	}
	m.body.SetContent(strings.TrimRight(b.String(), "\n"))
	m.body.EnsureVisible(m.cursor)
	if m.filterable {
		m.body.SetBottomLeft(fmt.Sprintf("%d / %d", len(m.visible), len(m.items)))
	}
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// Update consumes up/down/j/k and "/" (when filterable); while the filter is
// focused, every key is forwarded to it. Non-key messages are forwarded
// to the body pane so spinner ticks reach the loading-state animation.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	if m.filterable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		m.refresh()
		return m, cmd
	}
	switch {
	case m.filterable && key.Matches(km, m.keys.Filter):
		return m, m.filter.Focus()
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
	default:
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View stacks filter (if filterable) and the body pane.
func (m Model) View() string {
	if m.filterable {
		return m.filter.View() + "\n" + m.body.View()
	}
	return m.body.View()
}

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
	return out
}

// Value returns the current filter text ("" when not filterable or empty).
func (m Model) Value() string {
	if m.filterable {
		return m.filter.Value()
	}
	return ""
}

// SetDimensions resizes the list in place. When filterable, the internal
// filter pane consumes 3 rows at the top and the body pane gets the rest;
// otherwise the body pane takes the full height.
func (m *Model) SetDimensions(w, h int) {
	bodyH := h
	if m.filterable {
		m.filter.SetWidth(w)
		bodyH = max(0, h-3)
	}
	m.body.SetDimensions(w, bodyH)
	m.refresh()
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

// SetFocused sets the body pane's focus state so its border reads as
// active or inactive. Useful when embedding a list inside a parent that
// owns focus (e.g. a form field that gates input).
func (m *Model) SetFocused(b bool) { m.body.SetFocused(b) }

// SetTitle updates the title rendered on the body pane's top border.
// Useful when the list represents a slice that can change identity at
// runtime (e.g. "detail · <selection>").
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
