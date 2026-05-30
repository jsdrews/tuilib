// Package inspector provides a two-column label/value viewer for
// structured records — pod specs, REST responses, Prefect run details,
// any "show me the fields of this thing" surface. Each Field carries a
// Label and a Value; nested Children render below their parent with
// indent + ▸/▾ expand glyphs. Labels at the same parent auto-align so
// sibling key:value pairs read in a clean column.
//
// The body is a pkg/pane.Pane (so pgup/pgdn/arrows/mouse-wheel and
// horizontal scroll work out of the box) plus an optional
// pkg/filter.Model for "/-to-search". A field whose Label or Value
// matches the query highlights inline with MatchStyle; backslash toggles
// filter mode, which hides non-matching subtrees while keeping ancestors
// visible so the path stays readable. Same nav verbs as pkg/tree:
// up/down/j/k per row, g/G top/bottom, ctrl+u/ctrl+d half-page,
// space to toggle the cursor row, E/C to expand/collapse all. Arrow
// keys and hjkl follow the library-wide scroll convention (rule 23)
// and are reserved for vertical/horizontal scroll — they do not
// expand/collapse.
//
// Use FromAny / FromMap to convert json-unmarshal output (or any
// map[string]any / []any soup) into Fields without writing per-shape
// converters by hand.
package inspector

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Field is one record entry. Value renders to the right of Label; pass an
// empty Value when the field is just a header for its Children. Children
// nest under the field with indent + an expand glyph; pass nil/empty for
// scalar fields. Field is value-typed so callers can build a tree by
// composition without worrying about pointer aliasing.
type Field struct {
	Label    string
	Value    string
	Children []Field
}

// Options configures a new inspector. Theme.Inspector() returns this
// pre-styled — set Title/Fields/Filterable/Filter on the returned value.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "details".
	Title string
	// Fields is the top-level record. Each field's Children render as a
	// nested expandable block.
	Fields []Field
	// Filterable embeds a filter.Model above the body pane (three rows).
	Filterable bool
	// InitialDepth pre-expands every node whose depth is < InitialDepth.
	// 0 (default) shows only top-level rows collapsed; pass a large number
	// to expand everything.
	InitialDepth int

	// LabelStyle is applied to label text (typically a subtle color or
	// bold). Padding to the per-group label-column width is applied
	// before the style runs.
	LabelStyle lipgloss.Style
	// ValueStyle is applied to value text after label rendering.
	ValueStyle lipgloss.Style
	// MatchStyle highlights matched substrings while a query is active.
	MatchStyle lipgloss.Style
	// CurrentLineStyle is applied to the entire row holding the cursor,
	// padded out to the pane's inner width.
	CurrentLineStyle lipgloss.Style

	// Pane pass-throughs.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
	HScrollbar     bool

	// SpinnerStyle is applied to the spinner glyph rendered while the
	// inspector is in its loading state (see SetLoading).
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading.
	LoadingLabel string

	// Filter configures the embedded filter. Ignored when Filterable=false.
	Filter filter.Options

	// Keys overrides the default keymap. Any field left as the zero
	// key.Binding falls back to the corresponding DefaultKeys() value, so
	// callers can override a single action without restating the rest.
	// theme.Inspector() pre-populates this with DefaultKeys().
	Keys Keys
}

// Keys is the inspector's keymap. Each binding carries both its dispatch
// keys (WithKeys) and its help label (WithHelp) — Update and Help() read
// from the same struct, so a custom binding propagates everywhere. The
// embedded pane.Keys covers horizontal scroll; mutate fields on Pane to
// override h-scroll without touching the rest.
type Keys struct {
	Up, Down                       key.Binding
	Top, Bottom                    key.Binding
	HalfUp, HalfDown               key.Binding
	Toggle, ExpandAll, CollapseAll key.Binding
	NextSibling, PrevSibling       key.Binding
	NextLeaf, PrevLeaf             key.Binding
	Search, NextMatch, PrevMatch   key.Binding
	Filter                         key.Binding
	Pane                           pane.Keys
}

// DefaultKeys returns the inspector's stock keymap. Mutate the returned
// value to override individual actions.
func DefaultKeys() Keys {
	return Keys{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:         key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:      key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfUp:      key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ up")),
		HalfDown:    key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ down")),
		Toggle:      key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		ExpandAll:   key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "expand all")),
		CollapseAll: key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "collapse all")),
		NextSibling: key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next sibling")),
		PrevSibling: key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev sibling")),
		NextLeaf:    key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next leaf")),
		PrevLeaf:    key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "prev leaf")),
		Search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		NextMatch:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch:   key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
		Filter:      key.NewBinding(key.WithKeys("\\"), key.WithHelp(`\`, "filter mode")),
		Pane:        pane.DefaultKeys(),
	}
}

// fillDefaults fills any zero-valued binding in k with its DefaultKeys()
// counterpart, so partial overrides on Options.Keys work without restating
// every field.
func (k *Keys) fillDefaults() {
	d := DefaultKeys()
	if len(k.Up.Keys()) == 0          { k.Up = d.Up }
	if len(k.Down.Keys()) == 0        { k.Down = d.Down }
	if len(k.Top.Keys()) == 0         { k.Top = d.Top }
	if len(k.Bottom.Keys()) == 0      { k.Bottom = d.Bottom }
	if len(k.HalfUp.Keys()) == 0      { k.HalfUp = d.HalfUp }
	if len(k.HalfDown.Keys()) == 0    { k.HalfDown = d.HalfDown }
	if len(k.Toggle.Keys()) == 0      { k.Toggle = d.Toggle }
	if len(k.ExpandAll.Keys()) == 0   { k.ExpandAll = d.ExpandAll }
	if len(k.CollapseAll.Keys()) == 0 { k.CollapseAll = d.CollapseAll }
	if len(k.NextSibling.Keys()) == 0 { k.NextSibling = d.NextSibling }
	if len(k.PrevSibling.Keys()) == 0 { k.PrevSibling = d.PrevSibling }
	if len(k.NextLeaf.Keys()) == 0    { k.NextLeaf = d.NextLeaf }
	if len(k.PrevLeaf.Keys()) == 0    { k.PrevLeaf = d.PrevLeaf }
	if len(k.Search.Keys()) == 0      { k.Search = d.Search }
	if len(k.NextMatch.Keys()) == 0   { k.NextMatch = d.NextMatch }
	if len(k.PrevMatch.Keys()) == 0   { k.PrevMatch = d.PrevMatch }
	if len(k.Filter.Keys()) == 0      { k.Filter = d.Filter }
	k.Pane.FillDefaults()
}

// Model is the inspector widget. Embed by value; mutate via the setters.
type Model struct {
	fields       []Field
	expanded     map[string]bool
	rows         []row
	cursor       int
	initialDepth int

	body       pane.Pane
	filter     filter.Model
	filterable bool

	labelStyle       lipgloss.Style
	valueStyle       lipgloss.Style
	matchStyle       lipgloss.Style
	currentLineStyle lipgloss.Style

	keys Keys

	query      string
	matchRows  []int
	matchIdx   int
	filterMode bool
}

// row is one rendered line in the flattened view.
type row struct {
	field    Field
	path     string
	depth    int
	isLeaf   bool
	expanded bool
	// labelW is the column width to pad this row's Label to (max label
	// width across this row's sibling group at this depth).
	labelW int
}

// New constructs an inspector. Call Update/View from the parent model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "details"
	}
	opts.Keys.fillDefaults()
	m := Model{
		fields:           append([]Field(nil), opts.Fields...),
		expanded:         map[string]bool{},
		filterable:       opts.Filterable,
		labelStyle:       opts.LabelStyle,
		valueStyle:       opts.ValueStyle,
		matchStyle:       opts.MatchStyle,
		currentLineStyle: opts.CurrentLineStyle,
		keys:             opts.Keys,
		matchIdx:         -1,
		initialDepth:     opts.InitialDepth,
	}

	bodyH := opts.Height
	if m.filterable {
		bodyH = max(0, opts.Height-3)
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

	m.preExpand(m.fields, "", 0, m.initialDepth)
	m.refresh()
	return m
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles cursor movement, expand/collapse, search, and forwards
// non-key messages to the body pane (so spinner ticks reach the
// loading-state animation).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.filterable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyQuery()
		return m, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.refresh()
			}
			return m, nil
		case key.Matches(k, m.keys.Down):
			if m.cursor < len(m.rows)-1 {
				m.cursor++
				m.refresh()
			}
			return m, nil
		case key.Matches(k, m.keys.Top):
			m.cursor = 0
			m.refresh()
			return m, nil
		case key.Matches(k, m.keys.Bottom):
			m.cursor = max(0, len(m.rows)-1)
			m.refresh()
			return m, nil
		case key.Matches(k, m.keys.HalfUp):
			if m.cursor > 0 {
				m.cursor = max(0, m.cursor-m.halfPage())
				m.refresh()
			}
			return m, nil
		case key.Matches(k, m.keys.HalfDown):
			if last := len(m.rows) - 1; last >= 0 && m.cursor < last {
				m.cursor = min(last, m.cursor+m.halfPage())
				m.refresh()
			}
			return m, nil
		case key.Matches(k, m.keys.Toggle):
			m.toggleCursor()
			return m, nil
		case key.Matches(k, m.keys.ExpandAll):
			m.ExpandAll()
			return m, nil
		case key.Matches(k, m.keys.CollapseAll):
			m.CollapseAll()
			return m, nil
		case key.Matches(k, m.keys.NextSibling):
			m.jumpSibling(+1)
			return m, nil
		case key.Matches(k, m.keys.PrevSibling):
			m.jumpSibling(-1)
			return m, nil
		case key.Matches(k, m.keys.NextLeaf):
			m.jumpLeaf(+1)
			return m, nil
		case key.Matches(k, m.keys.PrevLeaf):
			m.jumpLeaf(-1)
			return m, nil
		case m.filterable && key.Matches(k, m.keys.Search):
			return m, m.filter.Focus()
		case m.filterable && key.Matches(k, m.keys.NextMatch):
			m.jumpMatch(+1)
			return m, nil
		case m.filterable && key.Matches(k, m.keys.PrevMatch):
			m.jumpMatch(-1)
			return m, nil
		case m.filterable && key.Matches(k, m.keys.Filter):
			m.filterMode = !m.filterMode
			m.refresh()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

// View stacks the filter (when filterable) above the body pane.
func (m Model) View() string {
	if m.filterable {
		return m.filter.View() + "\n" + m.body.View()
	}
	return m.body.View()
}

// Help returns the keys this inspector responds to. Each entry comes
// straight from m.keys (the same bindings Update dispatches against), so
// custom keymaps propagate to the hint strip automatically.
func (m Model) Help() []key.Binding {
	if m.filterable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		m.keys.Up, m.keys.Down,
		m.keys.HalfUp, m.keys.HalfDown,
		m.keys.Top, m.keys.Bottom,
		m.keys.Toggle,
		m.keys.ExpandAll, m.keys.CollapseAll,
		m.keys.NextSibling, m.keys.PrevSibling,
		m.keys.NextLeaf, m.keys.PrevLeaf,
	}
	out = append(out, m.body.HelpBindings()...)
	if m.filterable {
		out = append(out, m.keys.Search, m.keys.Filter)
		if m.query != "" {
			out = append(out, m.keys.NextMatch, m.keys.PrevMatch)
		}
	}
	return out
}

// SetDimensions resizes the inspector in place.
func (m *Model) SetDimensions(w, h int) {
	bodyH := h
	if m.filterable {
		m.filter.SetWidth(w)
		bodyH = max(0, h-3)
	}
	m.body.SetDimensions(w, bodyH)
	m.refresh()
}

// SetFields replaces the record. Expansion state is preserved by row
// path so refreshing a polled record doesn't collapse what the user had
// open; cursor falls back to the nearest valid row when its path
// disappears from the new fields. Options.InitialDepth is re-applied
// against the new field set so an inspector populated asynchronously
// (empty at construction, SetFields after a fetch) lands at the same
// pre-expansion the static-Fields path gets in New.
func (m *Model) SetFields(fs []Field) {
	prevPath := m.cursorPath()
	m.fields = append([]Field(nil), fs...)
	m.preExpand(m.fields, "", 0, m.initialDepth)
	m.refresh()
	if prevPath != "" {
		for i, r := range m.rows {
			if r.path == prevPath {
				m.cursor = i
				m.refresh()
				return
			}
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
		m.refresh()
	}
}

// SetTitle updates the title rendered on the body pane's top border.
func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

// SetFocused sets the body pane's focus state.
func (m *Model) SetFocused(b bool) { m.body.SetFocused(b) }

// SetActiveColor / SetInactiveColor update the body pane's border colors.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor)   { m.body.SetActiveColor(c) }
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

// Selected returns the field under the cursor.
func (m Model) Selected() (Field, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return Field{}, false
	}
	return m.rows[m.cursor].field, true
}

// Cursor returns the current cursor index into the visible row set.
func (m Model) Cursor() int { return m.cursor }

// SetCursor moves the cursor (clamped) and scrolls to keep it on screen.
func (m *Model) SetCursor(n int) {
	m.cursor = max(0, min(n, len(m.rows)-1))
	m.refresh()
}

// Searching reports whether the embedded filter currently has focus.
func (m Model) Searching() bool { return m.filterable && m.filter.Focused() }

// FilterMode reports whether non-matching subtrees are currently hidden.
func (m Model) FilterMode() bool { return m.filterMode }

// SetFilterMode turns filter-only rendering on or off.
func (m *Model) SetFilterMode(b bool) { m.filterMode = b; m.refresh() }

// Query returns the current search query (empty when no search active).
func (m Model) Query() string { return m.query }

// SetQuery seeds the search programmatically. Used to carry state across
// SetTheme rebuilds (read Query(), rebuild, SetQuery back).
func (m *Model) SetQuery(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.applyQuery()
}

// Loading reports whether the body pane is in its loading state.
func (m Model) Loading() bool { return m.body.Loading() }

// SetLoading toggles the loading state and returns the spinner's first tick.
func (m *Model) SetLoading(b bool) tea.Cmd { return m.body.SetLoading(b) }

// SetLoadingLabel updates the spinner's accompanying text.
func (m *Model) SetLoadingLabel(s string) { m.body.SetLoadingLabel(s) }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
func (m *Model) SetSpinnerStyle(s lipgloss.Style) { m.body.SetSpinnerStyle(s) }

// ExpandAll expands every nested field. Useful right after a refresh when
// the caller wants the whole record visible.
func (m *Model) ExpandAll() {
	m.walkAll(m.fields, "", func(p string, f Field) {
		if len(f.Children) > 0 {
			m.expanded[p] = true
		}
	})
	m.refresh()
}

// CollapseAll collapses every nested field.
func (m *Model) CollapseAll() {
	m.expanded = map[string]bool{}
	m.refresh()
}

// ---- internals ------------------------------------------------------------

func (m *Model) preExpand(fs []Field, parentPath string, depth, max int) {
	if depth >= max {
		return
	}
	for i, f := range fs {
		p := childPath(parentPath, i, f.Label)
		if len(f.Children) > 0 {
			m.expanded[p] = true
			m.preExpand(f.Children, p, depth+1, max)
		}
	}
}

// walkAll calls fn for every field (visible or not) in DFS order.
func (m *Model) walkAll(fs []Field, parentPath string, fn func(path string, f Field)) {
	for i, f := range fs {
		p := childPath(parentPath, i, f.Label)
		fn(p, f)
		if len(f.Children) > 0 {
			m.walkAll(f.Children, p, fn)
		}
	}
}

// refresh rebuilds m.rows from m.fields + m.expanded, recomputes match
// state, clamps the cursor, and pushes content into the body pane.
func (m *Model) refresh() {
	m.rows = m.flatten(m.fields, "", 0)
	if len(m.rows) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	} else if m.cursor < 0 {
		m.cursor = 0
	}
	m.recomputeMatches()
	m.draw()
}

// flatten produces the visible row list. Sibling label widths are computed
// per-group so labels at the same parent line up.
func (m *Model) flatten(fs []Field, parentPath string, depth int) []row {
	if len(fs) == 0 {
		return nil
	}
	groupW := 0
	for _, f := range fs {
		if w := ansi.StringWidth(f.Label); w > groupW {
			groupW = w
		}
	}
	var out []row
	for i, f := range fs {
		p := childPath(parentPath, i, f.Label)
		isLeaf := len(f.Children) == 0
		expanded := !isLeaf && m.expanded[p]
		r := row{
			field:    f,
			path:     p,
			depth:    depth,
			isLeaf:   isLeaf,
			expanded: expanded,
			labelW:   groupW,
		}
		if m.filterMode && m.query != "" {
			if !m.subtreeMatches(f) {
				continue
			}
		}
		out = append(out, r)
		if expanded {
			out = append(out, m.flatten(f.Children, p, depth+1)...)
		}
	}
	return out
}

// subtreeMatches reports whether f or any descendant matches the query.
// Used by filter mode to keep ancestors visible above a deeper match.
func (m Model) subtreeMatches(f Field) bool {
	if rowMatches(f, m.query) {
		return true
	}
	for _, c := range f.Children {
		if m.subtreeMatches(c) {
			return true
		}
	}
	return false
}

func rowMatches(f Field, q string) bool {
	if q == "" {
		return false
	}
	if strings.Contains(strings.ToLower(ansi.Strip(f.Label)), q) {
		return true
	}
	if strings.Contains(strings.ToLower(ansi.Strip(f.Value)), q) {
		return true
	}
	return false
}

// recomputeMatches walks the visible rows and records which contain the
// current query, so n/N can step through them.
func (m *Model) recomputeMatches() {
	m.matchRows = m.matchRows[:0]
	if m.query == "" {
		m.matchIdx = -1
		return
	}
	for i, r := range m.rows {
		if rowMatches(r.field, m.query) {
			m.matchRows = append(m.matchRows, i)
		}
	}
	m.matchIdx = -1
	for i, idx := range m.matchRows {
		if idx >= m.cursor {
			m.matchIdx = i
			break
		}
	}
	if m.matchIdx < 0 && len(m.matchRows) > 0 {
		m.matchIdx = 0
	}
}

func (m *Model) applyQuery() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == m.query {
		return
	}
	m.query = q
	m.refresh()
}

func (m *Model) jumpMatch(direction int) {
	if len(m.matchRows) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx + direction + len(m.matchRows)) % len(m.matchRows)
	m.cursor = m.matchRows[m.matchIdx]
	m.refresh()
}

// jumpSibling moves the cursor to the next (step=+1) or previous
// (step=-1) visible row at the same depth as the current cursor row.
// Cousins under sibling parents count as same-depth neighbors — the
// useful skip-past-expanded-children semantics rather than strict
// same-parent. No-op when no such row exists on the requested side.
func (m *Model) jumpSibling(step int) {
	if len(m.rows) == 0 {
		return
	}
	d := m.rows[m.cursor].depth
	for i := m.cursor + step; i >= 0 && i < len(m.rows); i += step {
		if m.rows[i].depth == d {
			m.cursor = i
			m.refresh()
			return
		}
	}
}

// jumpLeaf moves the cursor to the next (step=+1) or previous (step=-1)
// visible leaf row (a field with no Children). Collapsed parents are
// not leaves — their hidden subtree still has content, so jumpLeaf
// skips past them. No-op when no such row exists on the requested side.
func (m *Model) jumpLeaf(step int) {
	if len(m.rows) == 0 {
		return
	}
	for i := m.cursor + step; i >= 0 && i < len(m.rows); i += step {
		if m.rows[i].isLeaf {
			m.cursor = i
			m.refresh()
			return
		}
	}
}

func (m *Model) toggleCursor() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.isLeaf {
		return
	}
	m.expanded[r.path] = !m.expanded[r.path]
	m.refresh()
}

func (m Model) cursorPath() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	return m.rows[m.cursor].path
}

// halfPage is half the body's visible row count, floor 1.
func (m Model) halfPage() int {
	if n := m.body.VisibleRows() / 2; n > 0 {
		return n
	}
	return 1
}

// draw composes the rendered content for the body pane and pushes the
// current line + virtual scroll metrics through.
func (m *Model) draw() {
	if len(m.rows) == 0 {
		m.body.SetContent("")
		m.body.SetVirtualScroll(0, 0, 0)
		m.body.SetBottomRight("")
		return
	}
	visible := m.body.VisibleRows()
	start := 0
	if visible > 0 && m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	if maxStart := max(0, len(m.rows)-visible); start > maxStart {
		start = maxStart
	}
	end := start + visible
	if end > len(m.rows) {
		end = len(m.rows)
	}

	// Render each visible row at its natural width — no truncation here. The
	// pane's pushContent re-cuts every line to the visible window via
	// ansi.Cut on every scroll tick, so passing the full content lets the
	// horizontal scrollbar work (otherwise maxLineW = innerW and MaxXOffset
	// stays at 0).
	innerW := m.body.VisibleWidth()
	lines := make([]string, end-start)
	maxW := innerW
	for i := start; i < end; i++ {
		line := m.renderRow(m.rows[i])
		if w := ansi.StringWidth(line); w > maxW {
			maxW = w
		}
		lines[i-start] = line
	}
	// Apply the cursor highlight last, padded to maxW so the background bar
	// spans the whole visible band rather than ending at the cursor row's
	// natural text width.
	if cur := m.cursor - start; cur >= 0 && cur < len(lines) {
		lines[cur] = m.currentLineStyle.Render(padToWidth(lines[cur], maxW))
	}
	m.body.SetContent(strings.Join(lines, "\n"))
	m.body.SetVirtualScroll(len(m.rows), visible, start)
	m.body.SetBottomRight(fmt.Sprintf("%d / %d", m.cursor+1, len(m.rows)))
}

// renderRow lays out one row: indent + glyph + label + (": " + value).
// Label and value are highlighted when a query is active. Rows are returned
// at their natural width; the pane handles horizontal truncation on scroll.
func (m Model) renderRow(r row) string {
	indent := strings.Repeat("  ", r.depth)
	glyph := "  "
	if !r.isLeaf {
		if r.expanded {
			glyph = "▾ "
		} else {
			glyph = "▸ "
		}
	}
	label := r.field.Label
	value := r.field.Value
	if m.query != "" {
		label = highlight(label, m.query, m.matchStyle)
		value = highlight(value, m.query, m.matchStyle)
	}
	labelPadded := padCellTo(label, r.labelW)
	rendered := indent + glyph + m.labelStyle.Render(labelPadded)
	if value != "" {
		rendered += ": " + m.valueStyle.Render(value)
	}
	return rendered
}

// childPath builds a stable identity for a field so expansion state and
// cursor selection survive refreshes. Using both index and label keeps
// it readable in debug logs while still being unique among siblings even
// when labels collide.
func childPath(parent string, idx int, label string) string {
	seg := strconv.Itoa(idx) + ":" + label
	if parent == "" {
		return seg
	}
	return parent + "/" + seg
}

// padCellTo pads s with spaces (right) to width visible cells. Truncates
// when s already exceeds width.
func padCellTo(s string, width int) string {
	visible := ansi.StringWidth(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// padToWidth pads or truncates s to exactly w cells — used when applying
// the current-line background so the highlight extends to the pane edge.
func padToWidth(s string, w int) string {
	visible := ansi.StringWidth(s)
	if visible == w {
		return s
	}
	if visible > w {
		return ansi.Cut(s, 0, w)
	}
	return s + strings.Repeat(" ", w-visible)
}

// highlight wraps every (case-insensitive) occurrence of q in s with style.
// ANSI-aware: walks visible runes, preserves SGR escapes.
func highlight(s, q string, style lipgloss.Style) string {
	if q == "" || s == "" {
		return s
	}
	stripped := strings.ToLower(ansi.Strip(s))
	if !strings.Contains(stripped, q) {
		return s
	}
	// Simple path: rebuild from the stripped text + style each match. We
	// lose any inner SGR styling on the matched characters, which is fine
	// for the inspector's typical scalar values.
	plain := ansi.Strip(s)
	lower := strings.ToLower(plain)
	var b strings.Builder
	i := 0
	for i < len(plain) {
		j := strings.Index(lower[i:], q)
		if j < 0 {
			b.WriteString(plain[i:])
			break
		}
		b.WriteString(plain[i : i+j])
		b.WriteString(style.Render(plain[i+j : i+j+len(q)]))
		i += j + len(q)
	}
	return b.String()
}

// FromAny converts an arbitrary JSON-shaped value (string, number, bool,
// nil, []any, map[string]any) into a slice of Fields. Maps render with
// keys (sorted alphabetically for determinism); slices render with
// "[N]" labels. Use this for REST responses or k8s manifests after a
// json.Unmarshal into any/map[string]any.
func FromAny(v any) []Field {
	switch x := v.(type) {
	case map[string]any:
		return FromMap(x)
	case []any:
		out := make([]Field, len(x))
		for i, item := range x {
			out[i] = fieldFromAny(fmt.Sprintf("[%d]", i), item)
		}
		return out
	default:
		return []Field{{Label: "value", Value: scalarString(v)}}
	}
}

// FromMap converts a map[string]any into Fields, sorting keys for
// deterministic output. Nested maps and slices recurse.
func FromMap(m map[string]any) []Field {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Field, len(keys))
	for i, k := range keys {
		out[i] = fieldFromAny(k, m[k])
	}
	return out
}

func fieldFromAny(label string, v any) Field {
	switch x := v.(type) {
	case map[string]any:
		return Field{Label: label, Children: FromMap(x)}
	case []any:
		children := make([]Field, len(x))
		for i, item := range x {
			children[i] = fieldFromAny(fmt.Sprintf("[%d]", i), item)
		}
		return Field{Label: label, Children: children}
	default:
		return Field{Label: label, Value: scalarString(v)}
	}
}

func scalarString(v any) string {
	if v == nil {
		return "null"
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// json.Unmarshal lands numbers in float64; render integral values
		// without trailing ".0" so "Replicas: 3" reads cleanly.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
