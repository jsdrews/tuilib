// Package table provides a cursor-driven, optionally filterable tabular
// view inside a bordered pane. Each Row is a []string of cell text aligned
// against a fixed-width Column slice. The header pins to the top of the
// pane (it does not scroll out of view as the cursor moves), but it
// scrolls horizontally with the body so columns stay aligned with their
// titles when the user scrolls a wide table left/right.
//
// Cells are rendered ANSI-aware via x/ansi.Cut, so colored content
// (foreground-only escapes such as pkg/ansi.CellColor) survives column
// truncation without leaking color into adjacent cells. There is no
// "+8 budget for the ANSI escape" caveat — column Width is the visible
// width of the cell and that's what truncation respects. Lipgloss styles
// are also fine inside cells (header/selected styling applies on top of
// any inner styling), but inner full-reset SGR sequences will still
// clobber the selected-row background; prefer foreground-only color
// escapes for status-style cells when you need the row highlight to
// pass through unbroken.
//
// Filter syntax (when Filterable=true): the input is split on whitespace
// into AND-ed terms. A bare term matches any cell as a case-insensitive
// substring. A term shaped "key:value" scopes the match to the column
// whose Title case-insensitively starts with key (e.g. "region:europe");
// an ambiguous or unknown key falls through as a literal bare term, which
// is also how to search for a literal colon. A term whose value starts
// with "~" is compiled as a case-insensitive Go regex (e.g. "~^new",
// "region:~^euro"); compile errors fall back to a literal substring
// including the tilde, so the parser never refuses input. While the user
// is mid-typing a "key:val" term the filter pane's bottom-left slot lists
// the column's distinct values matching val, and tab completes val to the
// longest common prefix of the remaining candidates — regex terms skip
// the hint since enumerating regex matches isn't useful.
//
// Sort: set Column.Sortable to opt a column in. Keys are "[" / "]" to step
// the active sort column among Sortable columns and "s" to toggle
// direction; the active column gets a ▲ / ▼ marker after its title.
// Default comparator is case-insensitive on the ANSI-stripped cell text;
// override per-column with Column.Less for numeric, date, or unit-aware
// sort. SortColumn() / SortDescending() / SetSort(col, desc) carry sort
// state across SetTheme rebuilds the same way Cursor / Value do.
package table

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Column declares one column's title, width (in visible cells), and cell
// alignment. Width must be > 0; if you pass 0 the column auto-sizes to
// max(visible(title), 4).
type Column struct {
	Title string
	Width int
	// Align controls cell padding within the column. Use lipgloss.Left
	// (default), lipgloss.Right (good for numeric columns), or
	// lipgloss.Center.
	Align lipgloss.Position
	// Sortable allows the user to sort by this column with [/] (step
	// active sort column) and s (toggle direction). Non-sortable columns
	// are skipped during step. The active column gets a ▲/▼ marker
	// rendered after its title.
	Sortable bool
	// Less compares two cell strings for this column. If nil, sortable
	// columns use a case-insensitive comparison on the ANSI-stripped
	// text — fine for plain string columns. Set Less for numeric, date,
	// or unit-aware columns ("8.3M") that need custom parsing.
	Less func(a, b string) bool
}

// Row is one row of cell strings, positionally aligned to Options.Columns.
// Cells beyond len(Columns) are ignored; missing cells render as empty.
type Row []string

// Options configures a new table. Theme.Table() returns this pre-styled —
// set Title/Columns/Rows/Filterable/Filter on the returned value.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "Table".
	Title string
	// Columns declares the column layout. Required.
	Columns []Column
	// Rows is the full row set. The table copies this slice so the caller
	// can mutate their source independently.
	Rows []Row
	// Filterable embeds a filter.Model above the body pane (three rows).
	// See the package doc for the full filter syntax — bare substring,
	// "key:value" column scope, "~regex" prefix, and the distinct-value
	// hint + tab completion that fires while typing a "key:" term.
	Filterable bool

	// Pane pass-throughs.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle

	// HScrollbar reserves a row at the bottom of the body pane and lets
	// ←/h and →/l scroll wide tables horizontally. theme.Table() enables
	// this by default — disable when columns are guaranteed to fit.
	HScrollbar bool

	// HeaderStyle is applied to the header row (typically bold). Header
	// cells are still padded/aligned by column before the style runs.
	HeaderStyle lipgloss.Style
	// SelectedStyle is applied to the highlighted row. theme.Table()
	// uses bold + Accent fg + Subtle bg.
	SelectedStyle lipgloss.Style
	// CellStyle is applied to non-selected rows. Defaults to no style.
	CellStyle lipgloss.Style

	// SpinnerStyle styles the loading-state spinner glyph. Pass via
	// theme.Table() for a sensible default.
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading.
	LoadingLabel string

	// Filter configures the embedded filter.Model. Ignored when
	// Filterable=false. Theme.Table() pre-fills this from Theme.Filter().
	Filter filter.Options
}

// Model is the table widget. Embed as a value; mutate via the setters.
type Model struct {
	cols       []Column
	rows       []Row
	visible    []Row
	visibleIdx []int
	cursor     int
	viewStart  int

	filter     filter.Model
	filterable bool

	// sortCol is the active sort column index (-1 = no sort).
	// sortDesc reverses the comparator when true.
	sortCol  int
	sortDesc bool

	// distinct[i] is the sorted unique lowercased ANSI-stripped values of
	// column i across all rows — used to drive the filter hint and tab
	// completion when the user is mid-typing a `key:` term. Rebuilt on
	// SetRows / SetColumns so per-keystroke lookups stay cheap.
	distinct [][]string

	body          pane.Pane
	headerStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	cellStyle     lipgloss.Style
	hScrollbar    bool
}

var keys = struct {
	Up, Down, Top, Bottom, HalfUp, HalfDown, Filter, SortPrev, SortNext, SortDir key.Binding
}{
	Up:       key.NewBinding(key.WithKeys("up", "k")),
	Down:     key.NewBinding(key.WithKeys("down", "j")),
	Top:      key.NewBinding(key.WithKeys("g")),
	Bottom:   key.NewBinding(key.WithKeys("G")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d")),
	Filter:   key.NewBinding(key.WithKeys("/")),
	SortPrev: key.NewBinding(key.WithKeys("[")),
	SortNext: key.NewBinding(key.WithKeys("]")),
	SortDir:  key.NewBinding(key.WithKeys("s")),
}

// New constructs a table.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "Table"
	}
	cols := append([]Column(nil), opts.Columns...)
	for i, c := range cols {
		if c.Width <= 0 {
			w := ansi.StringWidth(c.Title)
			if w < 4 {
				w = 4
			}
			cols[i].Width = w
		}
	}
	m := Model{
		cols:          cols,
		rows:          append([]Row(nil), opts.Rows...),
		filterable:    opts.Filterable,
		sortCol:       -1,
		headerStyle:   opts.HeaderStyle,
		selectedStyle: opts.SelectedStyle,
		cellStyle:     opts.CellStyle,
		hScrollbar:    opts.HScrollbar,
	}
	m.visible = m.rows
	m.visibleIdx = identityIndex(len(m.rows))
	m.rebuildDistinct()

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
	})
	m.refresh()
	return m
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// Update consumes cursor + filter keys; non-key messages flow to the body
// pane so spinner ticks reach the loading-state animation.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	if m.filterable && m.filter.Focused() {
		// Intercept tab before forwarding so the textinput doesn't insert a
		// literal tab — instead, complete the in-progress key:val term to
		// the longest common prefix of its remaining hint candidates.
		if km.String() == "tab" {
			if m.completeFilterTerm() {
				m.applyFilter()
				m.refresh()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		m.refresh()
		return m, cmd
	}
	switch {
	case m.filterable && key.Matches(km, keys.Filter):
		return m, m.filter.Focus()
	case key.Matches(km, keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.refresh()
		}
	case key.Matches(km, keys.Down):
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refresh()
		}
	case key.Matches(km, keys.Top):
		if m.cursor != 0 && len(m.visible) > 0 {
			m.cursor = 0
			m.refresh()
		}
	case key.Matches(km, keys.Bottom):
		if last := len(m.visible) - 1; last >= 0 && m.cursor != last {
			m.cursor = last
			m.refresh()
		}
	case key.Matches(km, keys.HalfUp):
		if m.cursor > 0 {
			m.cursor = max(0, m.cursor-m.halfPage())
			m.refresh()
		}
	case key.Matches(km, keys.HalfDown):
		if last := len(m.visible) - 1; last >= 0 && m.cursor < last {
			m.cursor = min(last, m.cursor+m.halfPage())
			m.refresh()
		}
	case key.Matches(km, keys.SortPrev):
		if m.stepSortColumn(-1) {
			m.applyFilter()
			m.refresh()
		}
	case key.Matches(km, keys.SortNext):
		if m.stepSortColumn(+1) {
			m.applyFilter()
			m.refresh()
		}
	case key.Matches(km, keys.SortDir):
		if m.toggleSortDir() {
			m.applyFilter()
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

// Help returns the keys this table responds to.
func (m Model) Help() []key.Binding {
	if m.filterable && m.filter.Focused() {
		out := m.filter.Help()
		if _, _, _, ok := m.activeKeyTerm(); ok {
			out = append(out, key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "complete")))
		}
		return out
	}
	out := []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("ctrl+u", "ctrl+d"), key.WithHelp("^u/^d", "½ page")),
		key.NewBinding(key.WithKeys("g", "G"), key.WithHelp("g/G", "top/bot")),
	}
	if m.hScrollbar {
		out = append(out, key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←→", "h-scroll")))
	}
	if m.hasSortable() {
		out = append(out,
			key.NewBinding(key.WithKeys("[", "]"), key.WithHelp("[]", "sort col")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort dir")),
		)
	}
	if m.filterable {
		out = append(out, key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")))
	}
	return out
}

// SetDimensions resizes the table in place. When filterable, the internal
// filter pane consumes 3 rows at the top and the body pane gets the rest.
func (m *Model) SetDimensions(w, h int) {
	bodyH := h
	if m.filterable {
		m.filter.SetWidth(w)
		bodyH = max(0, h-3)
	}
	m.body.SetDimensions(w, bodyH)
	m.refresh()
}

// SetRows replaces the row set, re-applies the current filter, redraws.
func (m *Model) SetRows(rows []Row) {
	m.rows = append([]Row(nil), rows...)
	m.rebuildDistinct()
	m.applyFilter()
	m.refresh()
}

// SetColumns replaces the column layout. Cell text is preserved; widths
// re-apply on the next refresh.
func (m *Model) SetColumns(cols []Column) {
	m.cols = append([]Column(nil), cols...)
	for i, c := range m.cols {
		if c.Width <= 0 {
			w := ansi.StringWidth(c.Title)
			if w < 4 {
				w = 4
			}
			m.cols[i].Width = w
		}
	}
	m.rebuildDistinct()
	m.refresh()
}

// SetCursor moves the cursor (clamped) and scrolls to keep it on screen.
func (m *Model) SetCursor(n int) {
	m.cursor = max(0, min(n, len(m.visible)-1))
	m.refresh()
}

// SetValue overwrites the filter text (no-op when not filterable).
func (m *Model) SetValue(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.applyFilter()
	m.refresh()
}

// SetTitle updates the title rendered on the body pane's top border.
func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

// SetFocused sets the body pane's focus state (controls border color).
func (m *Model) SetFocused(b bool) { m.body.SetFocused(b) }

// SetActiveColor / SetInactiveColor update the body pane's border colors.
// Useful for theme swaps that don't rebuild the model.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor)   { m.body.SetActiveColor(c) }
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

// SetHeaderStyle / SetSelectedStyle / SetCellStyle update row styling.
func (m *Model) SetHeaderStyle(s lipgloss.Style)   { m.headerStyle = s; m.refresh() }
func (m *Model) SetSelectedStyle(s lipgloss.Style) { m.selectedStyle = s; m.refresh() }
func (m *Model) SetCellStyle(s lipgloss.Style)     { m.cellStyle = s; m.refresh() }

// Selected returns the currently highlighted row. ok is false when the
// visible set (post-filter) is empty.
func (m Model) Selected() (Row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil, false
	}
	return m.visible[m.cursor], true
}

// SelectedIndex returns the highlighted row's index into the original
// (pre-filter) Rows() slice. Use this when callers maintain a parallel
// source slice and need to identify which source row is selected.
func (m Model) SelectedIndex() (int, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visibleIdx) {
		return 0, false
	}
	return m.visibleIdx[m.cursor], true
}

// Cursor returns the current cursor index into the visible (post-filter) set.
func (m Model) Cursor() int { return m.cursor }

// Visible returns the post-filter rows, in display order.
func (m Model) Visible() []Row { return m.visible }

// Rows returns the full unfiltered row set.
func (m Model) Rows() []Row { return m.rows }

// Columns returns the current column layout.
func (m Model) Columns() []Column { return m.cols }

// Filtering reports whether the embedded filter currently has focus.
func (m Model) Filtering() bool { return m.filterable && m.filter.Focused() }

// SortColumn returns the active sort column index (-1 when no sort).
func (m Model) SortColumn() int { return m.sortCol }

// SortDescending reports the active sort direction.
func (m Model) SortDescending() bool { return m.sortDesc }

// SetSort sets the sort column and direction. col == -1 disables sort;
// otherwise col must reference a Sortable column. Use this on rebuild
// (theme swap) to carry SortColumn/SortDescending across the new model.
func (m *Model) SetSort(col int, desc bool) {
	if col < 0 || col >= len(m.cols) || !m.cols[col].Sortable {
		m.sortCol = -1
		m.sortDesc = false
	} else {
		m.sortCol = col
		m.sortDesc = desc
	}
	m.applyFilter()
	m.refresh()
}

// Value returns the current filter text.
func (m Model) Value() string {
	if m.filterable {
		return m.filter.Value()
	}
	return ""
}

// Loading reports whether the table is in its loading state.
func (m Model) Loading() bool { return m.body.Loading() }

// SetLoading toggles the loading state. Returns the spinner's first Tick
// when entering — propagate it back so the spinner animates.
func (m *Model) SetLoading(b bool) tea.Cmd { return m.body.SetLoading(b) }

// SetLoadingLabel updates the text rendered next to the spinner.
func (m *Model) SetLoadingLabel(s string) { m.body.SetLoadingLabel(s) }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
func (m *Model) SetSpinnerStyle(s lipgloss.Style) { m.body.SetSpinnerStyle(s) }

// ---- internals -------------------------------------------------------------

func (m *Model) applyFilter() {
	if !m.filterable {
		m.visible = append([]Row(nil), m.rows...)
		m.visibleIdx = identityIndex(len(m.rows))
	} else {
		q := strings.TrimSpace(m.filter.Value())
		if q == "" {
			m.visible = append([]Row(nil), m.rows...)
			m.visibleIdx = identityIndex(len(m.rows))
		} else {
			terms := m.parseFilter(q)
			out := make([]Row, 0, len(m.rows))
			idx := make([]int, 0, len(m.rows))
			for i, r := range m.rows {
				if rowMatchesAll(r, terms) {
					out = append(out, r)
					idx = append(idx, i)
				}
			}
			m.visible = out
			m.visibleIdx = idx
		}
	}
	m.applySort()
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

// applySort sorts visible (and visibleIdx in lockstep) by the active
// sort column. No-op when sortCol is unset, out of range, or not Sortable.
func (m *Model) applySort() {
	if m.sortCol < 0 || m.sortCol >= len(m.cols) {
		return
	}
	col := m.cols[m.sortCol]
	if !col.Sortable {
		return
	}
	less := col.Less
	if less == nil {
		less = defaultLess
	}
	sortIdx := m.sortCol
	desc := m.sortDesc
	indices := make([]int, len(m.visible))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a := cellAt(m.visible[indices[i]], sortIdx)
		b := cellAt(m.visible[indices[j]], sortIdx)
		if desc {
			return less(b, a)
		}
		return less(a, b)
	})
	newVisible := make([]Row, len(m.visible))
	newIdx := make([]int, len(m.visibleIdx))
	for i, k := range indices {
		newVisible[i] = m.visible[k]
		newIdx[i] = m.visibleIdx[k]
	}
	m.visible = newVisible
	m.visibleIdx = newIdx
}

func defaultLess(a, b string) bool {
	return strings.ToLower(ansi.Strip(a)) < strings.ToLower(ansi.Strip(b))
}

func cellAt(r Row, i int) string {
	if i < 0 || i >= len(r) {
		return ""
	}
	return r[i]
}

// hasSortable reports whether any column opts into sort. Used by Help()
// to surface [/] / s only when they have an effect.
func (m Model) hasSortable() bool {
	for _, c := range m.cols {
		if c.Sortable {
			return true
		}
	}
	return false
}

// sortableColumns returns the indices of columns where Sortable is true,
// in column order. Used by stepSortColumn to navigate between them.
func (m Model) sortableColumns() []int {
	var out []int
	for i, c := range m.cols {
		if c.Sortable {
			out = append(out, i)
		}
	}
	return out
}

// stepSortColumn moves the active sort column by direction (-1 or +1)
// among sortable columns, wrapping around. Activates the first sortable
// column when none is active. Returns true when state changed.
func (m *Model) stepSortColumn(direction int) bool {
	cols := m.sortableColumns()
	if len(cols) == 0 {
		return false
	}
	if m.sortCol < 0 {
		m.sortCol = cols[0]
		return true
	}
	cur := -1
	for i, c := range cols {
		if c == m.sortCol {
			cur = i
			break
		}
	}
	if cur < 0 {
		m.sortCol = cols[0]
		return true
	}
	next := (cur + direction + len(cols)) % len(cols)
	if cols[next] == m.sortCol {
		return false
	}
	m.sortCol = cols[next]
	return true
}

// toggleSortDir flips the sort direction. When no column is active, picks
// the first sortable column ascending. Returns true when state changed.
func (m *Model) toggleSortDir() bool {
	if m.sortCol < 0 {
		cols := m.sortableColumns()
		if len(cols) == 0 {
			return false
		}
		m.sortCol = cols[0]
		m.sortDesc = false
		return true
	}
	m.sortDesc = !m.sortDesc
	return true
}

// filterTerm is one space-separated clause from the filter input. col == -1
// matches any cell ("bare" term); col >= 0 matches only that column's cell.
// When re is non-nil the term matches via regex; otherwise value (already
// lowercased) is matched as a substring against the lowercased cell text.
type filterTerm struct {
	col   int
	value string
	re    *regexp.Regexp
}

// parseFilter splits the filter string into AND-ed terms. A term shaped
// "key:value" is column-scoped when key (case-insensitively) is a prefix
// of exactly one column title; otherwise the whole term (key:value) is
// treated as a bare any-cell substring so the user can search for literal
// colons. A term whose value starts with "~" is compiled as a
// case-insensitive regex (e.g. "~^new", "region:~^euro"); on compile error
// or a lone "~" the term falls back to a literal substring including the
// tilde, so the parser never refuses input.
func (m Model) parseFilter(q string) []filterTerm {
	parts := strings.Fields(q)
	out := make([]filterTerm, 0, len(parts))
	for _, p := range parts {
		if i := strings.Index(p, ":"); i > 0 && i < len(p)-1 {
			key, val := p[:i], p[i+1:]
			if col := m.columnByPrefix(key); col >= 0 {
				out = append(out, compileTerm(col, val))
				continue
			}
		}
		out = append(out, compileTerm(-1, p))
	}
	return out
}

// compileTerm builds a filterTerm from a raw clause. A leading "~" requests
// regex matching; on compile failure the raw text (including the tilde) is
// kept as a literal substring so the user always sees results for what
// they typed.
func compileTerm(col int, raw string) filterTerm {
	if strings.HasPrefix(raw, "~") && len(raw) > 1 {
		if re, err := regexp.Compile("(?i)" + raw[1:]); err == nil {
			return filterTerm{col: col, re: re}
		}
	}
	return filterTerm{col: col, value: strings.ToLower(raw)}
}

// columnByPrefix returns the index of the unique column whose Title starts
// with key (case-insensitive). Returns -1 when there is no match or when
// the prefix is ambiguous across multiple columns.
func (m Model) columnByPrefix(key string) int {
	key = strings.ToLower(key)
	match := -1
	for i, c := range m.cols {
		if strings.HasPrefix(strings.ToLower(c.Title), key) {
			if match >= 0 {
				return -1
			}
			match = i
		}
	}
	return match
}

// rowMatchesAll returns true when every term matches r. Bare terms (col<0)
// match any cell; column-scoped terms match only that column. Cells are
// ANSI-stripped before matching; substring terms also lowercase, regex
// terms rely on the (?i) flag set during compile.
func rowMatchesAll(r Row, terms []filterTerm) bool {
	for _, t := range terms {
		if t.col >= 0 {
			cell := ""
			if t.col < len(r) {
				cell = r[t.col]
			}
			if !t.matches(cell) {
				return false
			}
			continue
		}
		hit := false
		for _, cell := range r {
			if t.matches(cell) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// matches reports whether the (ANSI-stripped) cell satisfies the term.
func (t filterTerm) matches(cell string) bool {
	stripped := ansi.Strip(cell)
	if t.re != nil {
		return t.re.MatchString(stripped)
	}
	return strings.Contains(strings.ToLower(stripped), t.value)
}

// halfPage is the cursor step for ctrl+u/ctrl+d — half the viewport
// (excluding the header row), floor 1.
func (m Model) halfPage() int {
	if n := (m.body.VisibleRows() - 1) / 2; n > 0 {
		return n
	}
	return 1
}

// dataRows returns the body height available for data rows (the pane's
// inner viewport minus the one row reserved for the header).
func (m Model) dataRows() int { return max(0, m.body.VisibleRows()-1) }

// adjustViewStart slides viewStart to keep the cursor on screen within
// the dataRows() window.
func (m *Model) adjustViewStart() {
	dr := m.dataRows()
	if dr <= 0 {
		m.viewStart = 0
		return
	}
	if m.cursor < m.viewStart {
		m.viewStart = m.cursor
	}
	if m.cursor >= m.viewStart+dr {
		m.viewStart = m.cursor - dr + 1
	}
	maxStart := max(0, len(m.visible)-dr)
	if m.viewStart > maxStart {
		m.viewStart = maxStart
	}
	if m.viewStart < 0 {
		m.viewStart = 0
	}
}

func (m *Model) refresh() {
	m.adjustViewStart()
	header := m.headerStyle.Render(renderRow(m.headerCells(), m.cols))

	dr := m.dataRows()
	end := m.viewStart + dr
	if end > len(m.visible) {
		end = len(m.visible)
	}

	var b strings.Builder
	b.WriteString(header)
	for i := m.viewStart; i < end; i++ {
		b.WriteByte('\n')
		row := renderRow([]string(m.visible[i]), m.cols)
		if i == m.cursor {
			b.WriteString(m.selectedStyle.Render(row))
		} else {
			b.WriteString(m.cellStyle.Render(row))
		}
	}
	m.body.SetContent(b.String())

	// Drive the pane's right-edge scrollbar from our logical row counts so
	// the thumb reflects position within the dataset, not within the
	// pane's in-viewport slice (which is always one window's worth).
	m.body.SetVirtualScroll(len(m.visible), dr, m.viewStart)

	if m.filterable {
		m.body.SetBottomLeft(fmt.Sprintf("%d / %d", len(m.visible), len(m.rows)))
		m.filter.SetBottomLeft(m.filterHint())
	}
	if len(m.visible) > 0 {
		m.body.SetBottomRight(fmt.Sprintf("%d / %d", m.cursor+1, len(m.visible)))
	} else {
		m.body.SetBottomRight("")
	}
}

// rebuildDistinct populates m.distinct[i] with the sorted unique
// (lowercased, ANSI-stripped) values appearing in column i. Cheap to
// recompute; called only on row/column mutations, not per keystroke.
func (m *Model) rebuildDistinct() {
	m.distinct = make([][]string, len(m.cols))
	for i := range m.cols {
		seen := make(map[string]struct{}, len(m.rows))
		vals := make([]string, 0, len(m.rows))
		for _, r := range m.rows {
			if i >= len(r) {
				continue
			}
			v := strings.ToLower(ansi.Strip(r[i]))
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			vals = append(vals, v)
		}
		sort.Strings(vals)
		m.distinct[i] = vals
	}
}

// activeKeyTerm inspects the filter value and returns the column index +
// in-progress value of the trailing `key:val` term, when one is being
// typed. ok is false when the filter is empty, the trailing token isn't a
// resolvable key:val term, or the value is a regex (`~…` — enumerating
// regex matches isn't a useful hint).
func (m Model) activeKeyTerm() (col int, key, val string, ok bool) {
	if !m.filterable {
		return 0, "", "", false
	}
	v := m.filter.Value()
	if v == "" || strings.HasSuffix(v, " ") || strings.HasSuffix(v, "\t") {
		return 0, "", "", false
	}
	tok := v
	if i := strings.LastIndexAny(v, " \t"); i >= 0 {
		tok = v[i+1:]
	}
	i := strings.Index(tok, ":")
	if i <= 0 {
		return 0, "", "", false
	}
	key = tok[:i]
	val = tok[i+1:]
	if strings.HasPrefix(val, "~") {
		return 0, "", "", false
	}
	col = m.columnByPrefix(key)
	if col < 0 {
		return 0, "", "", false
	}
	return col, key, val, true
}

// hintCandidates returns the distinct values for col whose lowercased form
// starts with the lowercased prefix.
func (m Model) hintCandidates(col int, prefix string) []string {
	if col < 0 || col >= len(m.distinct) {
		return nil
	}
	prefix = strings.ToLower(prefix)
	var out []string
	for _, v := range m.distinct[col] {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	return out
}

// filterHint formats the hint string written into the filter pane's
// bottom-left slot. Empty when there's no in-progress key:val term, when
// no distinct values match the prefix, or when the filter is blurred.
func (m Model) filterHint() string {
	if !m.filter.Focused() {
		return ""
	}
	col, _, val, ok := m.activeKeyTerm()
	if !ok {
		return ""
	}
	cands := m.hintCandidates(col, val)
	if len(cands) == 0 {
		return ""
	}
	const max = 6
	shown := cands
	suffix := ""
	if len(shown) > max {
		shown = shown[:max]
		suffix = fmt.Sprintf(" +%d more", len(cands)-max)
	}
	return strings.Join(shown, ", ") + suffix
}

// completeFilterTerm extends the in-progress key:val term to the longest
// common prefix of its remaining hint candidates and writes it back into
// the filter. Returns true when the value actually changed.
func (m *Model) completeFilterTerm() bool {
	col, key, val, ok := m.activeKeyTerm()
	if !ok {
		return false
	}
	cands := m.hintCandidates(col, val)
	if len(cands) == 0 {
		return false
	}
	common := longestCommonPrefix(cands)
	if len(common) <= len(val) {
		return false
	}
	v := m.filter.Value()
	tokStart := 0
	if i := strings.LastIndexAny(v, " \t"); i >= 0 {
		tokStart = i + 1
	}
	newVal := v[:tokStart] + key + ":" + common
	if newVal == v {
		return false
	}
	m.filter.SetValue(newVal)
	return true
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		n := 0
		for n < len(prefix) && n < len(s) && prefix[n] == s[n] {
			n++
		}
		prefix = prefix[:n]
		if prefix == "" {
			break
		}
	}
	return prefix
}

func (m Model) headerCells() []string {
	out := make([]string, len(m.cols))
	for i, c := range m.cols {
		title := c.Title
		if i == m.sortCol {
			if m.sortDesc {
				title += " ▼"
			} else {
				title += " ▲"
			}
		}
		out[i] = title
	}
	return out
}

// renderRow lays cells out across cols with one space between columns.
// Each cell is ANSI-aware truncated/padded to its column width and aligned
// per column.Align.
func renderRow(cells []string, cols []Column) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = formatCell(cell, col.Width, col.Align)
	}
	return strings.Join(parts, " ")
}

// formatCell pads or truncates s to width visible cells, applying align.
// ANSI escapes inside s are preserved; cuts use ansi.Cut so SGR state is
// closed out cleanly at the truncation point.
func formatCell(s string, width int, align lipgloss.Position) string {
	visible := ansi.StringWidth(s)
	if visible == width {
		return s
	}
	if visible > width {
		return ansi.Cut(s, 0, width)
	}
	pad := width - visible
	switch align {
	case lipgloss.Right:
		return strings.Repeat(" ", pad) + s
	case lipgloss.Center:
		l := pad / 2
		r := pad - l
		return strings.Repeat(" ", l) + s + strings.Repeat(" ", r)
	default:
		return s + strings.Repeat(" ", pad)
	}
}

func identityIndex(n int) []int {
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = i
	}
	return out
}
