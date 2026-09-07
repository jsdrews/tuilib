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
// the hint since enumerating regex matches isn't useful. The grammar
// itself lives in pkg/query, so a caller translating the same filter into
// a remote request gets the identical parse without importing this package.
//
// Horizontal nav: ←/→ (or h/l) scroll by HScrollStep cells; 0/home jump
// to the leftmost edge; $/end jump to the rightmost edge; shift+←/shift+→
// snap the viewport to the previous / next column boundary so a wide
// table can be stepped column-by-column instead of cell-by-cell.
//
// Sort: set Column.Sortable to opt a column in. Keys are "[" / "]" to step
// the active sort column among Sortable columns and "s" to toggle
// direction; the active column gets a ▲ / ▼ marker after its title.
// Default comparator is case-insensitive on the ANSI-stripped cell text;
// override per-column with Column.Less for numeric, date, or unit-aware
// sort. SortColumn() / SortDescending() / SetSort(col, desc) carry sort
// state across SetTheme rebuilds the same way Cursor / Value do.
//
// Remote sources: Options.FilterMode and Options.SortMode decide whether
// the table answers the filter and sort itself or reports them for someone
// else to answer. Under FilterRemote / SortRemote the table displays its
// rows exactly as given and emits QueryChangedMsg when the user commits a
// filter or requests a sort; the screen turns that into a request and
// pushes the result back through SetRows / SetKeyedRows. Filter hints come
// from SetDistinct rather than from the rows on screen, since a single page
// of a larger set completes to values that are wrong rather than merely
// incomplete. The two modes are independent — a table can sort remotely
// while filtering the page it holds, or the reverse.
//
// SetWindow(rows, offset, total) is the other half: the table holds only
// the logical indices [offset, offset+len(rows)) of a set total rows long,
// while the cursor, the scrollbar and the counters all work against total.
// Indices the window doesn't hold render as Options.Placeholder and report
// ok=false from Selected, so scrolling past the loaded range shows filler
// rather than wrong data and a screen can't act on a row it never
// received. Pair it with ViewportChangedMsg, which reports the logical
// range now on screen — that is the signal to fetch the next window.
package table

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/query"
)

// Column declares one column's title, width (in visible cells), and cell
// alignment. Width sizing modes:
//   - Width > 0, Flex == 0: fixed width.
//   - Width == 0, Flex == 0: content-auto — sized to the widest of
//     title and any cell value (ANSI-aware, floor of 4).
//   - Flex > 0: column expands to absorb a share of leftover horizontal
//     space, weighted by Flex. Width (or content-auto when Width==0)
//     acts as a minimum; MaxWidth (when > 0) caps growth.
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
	// Flex, when > 0, makes this column absorb a share of leftover
	// horizontal space after every column's base width is accounted for.
	// Multiple flex columns split the remainder proportionally
	// (Flex=1 + Flex=2 → 1:2 split). The base width acts as a minimum:
	// a flex column never shrinks below it, but it does grow when room
	// is available. When the table is narrower than the sum of base
	// widths, flex columns get no expansion (extra space goes to the
	// pane's horizontal scroll, not column reflow).
	Flex int
	// MaxWidth, when > 0, caps the column's effective width — flex
	// growth never pushes it above this value. When a flex column hits
	// its cap, the surplus redistributes to the remaining uncapped
	// flex columns by their weights (iteratively, so chains of caps
	// are handled). When every flex column is capped, leftover space
	// stays unused on the right edge of the row.
	MaxWidth int
	// Hidden, when true, keeps the column in the data model but omits it
	// from rendering + width computation entirely. Hidden columns still:
	//   - Participate in filter matching (bare terms scan every cell,
	//     key:value scoped terms resolve Title against hidden columns
	//     too — e.g. "namespace:default" filters a table that has a
	//     hidden Namespace column).
	//   - Appear in SelectedRow / RowFocusedMsg.Cells / Columns so
	//     parents can capture identity fields for drilldown (e.g. bind
	//     ${selection.Namespace}) without giving up screen real estate.
	// Rows must still have one cell per declared column — hidden columns
	// don't change the row shape, they just hide their slot from view.
	Hidden bool
}

// Row is one row of cell strings, positionally aligned to Options.Columns.
// Cells beyond len(Columns) are ignored; missing cells render as empty.
type Row []string

// KeyedRow pairs a stable identity Key with the row's cells. Pass through
// SetKeyedRows when the row source is polled so the cursor can re-bind to
// the same Key after a refresh — when the row at the cursor's Key
// reappears in the new set the cursor follows it; otherwise it falls back
// to the clamped previous index. Use SelectedKey to read the current
// row's identity. KeyedRow is a separate path (not a swap of the Row
// type) so existing SetRows callers see no change.
type KeyedRow struct {
	Key   string
	Cells []string
}

// RowFocusedMsg is emitted by the table when the cursor lands on a
// different row than the last time we emitted — after cursor movement,
// after a filter/sort/SetRows swap that changes which row is under the
// cursor, or on the initial view. Parents subscribe to it to drive
// "detail on hover" patterns: refetch a parameterized detail source
// keyed on the focused row's cells. Dedup is on (row index, cells) so a
// SetRows swap that lands the same content under the cursor doesn't
// re-emit; a swap that changes the content does. Empty=true fires only
// as a transition (had focus → no focus) — an empty table never emits
// as its first message.
type RowFocusedMsg struct {
	// Row is the cursor's index in the post-filter, post-sort visible
	// slice. Zero when Empty is true.
	Row int
	// Cells is the focused row's values in column order. Nil when Empty.
	Cells []string
	// Columns is the parallel column titles so subscribers can look up
	// cells by name without hardcoding index. Nil when Empty.
	Columns []string
	// Empty is true when no row is currently focused (empty visible set
	// or cursor out of range). Row / Cells / Columns are zero-valued.
	Empty bool
}

// ViewportChangedMsg is emitted by the table whenever the visible slice of
// rows changes — scroll, resize, filter, sort, or a row-set swap. Parents
// can match this in their own Update to lazy-load off-screen data for the
// currently visible window (fetch details for FirstVisible..LastVisible,
// prefetch adjacent pages, etc). Indices are into the post-filter, post-
// sort visible slice, so they map directly to the rows the user sees. The
// message is only emitted once the viewport is real (dimensions applied,
// visible rows non-empty); a SetRows call that lands before the first
// WindowSizeMsg produces no msg until the next refresh sees a valid
// viewport. Consecutive duplicate viewports are elided.
type ViewportChangedMsg struct {
	// FirstVisible is the index of the topmost row currently on screen.
	FirstVisible int
	// LastVisible is the index of the bottommost row currently on screen
	// (inclusive). Equal to FirstVisible when only one row fits.
	LastVisible int
	// TotalRows is len(visible) at the time of emission — the size of the
	// filtered/sorted set, not the raw row count.
	TotalRows int
}

// FilterMode selects who applies the filter the user types.
type FilterMode int

const (
	// FilterLocal applies the filter to the rows the table holds. This is
	// the zero value, so existing tables keep filtering themselves.
	FilterLocal FilterMode = iota
	// FilterRemote stops the table filtering its own rows and makes it
	// report committed queries as QueryChangedMsg instead. The rows the
	// table holds are already the answer to the last query, so they are
	// displayed as given.
	FilterRemote
)

// SortMode selects who applies the sort.
type SortMode int

const (
	// SortLocal reorders the rows the table holds. Zero value.
	SortLocal SortMode = iota
	// SortRemote leaves row order alone and reports the requested sort as
	// QueryChangedMsg. The ▲/▼ header marker still tracks the active
	// column, so the user sees what they asked for while it is in flight.
	SortRemote
)

// QueryChangedMsg is emitted when the query a remote source should answer
// changes — a filter the user committed, or a sort they requested. It only
// fires when FilterMode is FilterRemote or SortMode is SortRemote;
// a fully local table never emits it.
//
// Filters are reported on commit, not per keystroke: enter, esc, or the
// filter losing focus. Typing is not a query, and a request per keystroke
// is a request storm. Tab completion is likewise silent — it edits the
// in-progress term without committing it.
//
// The message does not move the cursor. A screen answering it typically
// resets cursor and offset to the top, since row 40 of the previous result
// set means nothing in the next one — but doing that here would jump the
// cursor through stale rows a frame before the new ones land, so it is the
// screen's call. Consecutive duplicate queries are elided, and the
// state-restoration setters (SetSort, SetValue) adopt their new state
// silently, so a SetTheme rebuild never reads as a user-driven change.
type QueryChangedMsg struct {
	// Raw is the committed filter text exactly as typed. Empty when the
	// filter is empty or FilterMode is FilterLocal.
	Raw string
	// Terms is Raw parsed against the column titles. Scoped terms carry
	// the resolved Title, so building "?region=europe" needs no lookup.
	// Nil when Raw is empty.
	Terms []query.Term
	// Sort is the active sort column's title, or "" when nothing is
	// sorted or SortMode is SortLocal.
	Sort string
	// SortColumn is that column's index, or -1.
	SortColumn int
	// Desc reverses the sort. False when SortColumn is -1.
	Desc bool
}

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

	// FilterMode selects who applies the filter. Defaults to FilterLocal.
	// FilterRemote requires Filterable — without a filter bar there is no
	// query to report.
	FilterMode FilterMode
	// SortMode selects who applies the sort. Defaults to SortLocal.
	SortMode SortMode
	// Placeholder is the cell text drawn for a row inside the logical
	// range that the current window doesn't hold — see SetWindow.
	// Defaults to "·". Pre-style it foreground-only (pkg/ansi.CellColor)
	// if you want it dimmed, the same way Borders glyphs are styled, so
	// the selected-row background passes through unbroken.
	Placeholder string

	// Pane pass-throughs.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	// Glyphs are the marks this component draws, plus the scrollbar
	// thumb and track it hands to its pane. Empty fields fall back to
	// glyph.Default.
	Glyphs       glyph.Set
	SlotBrackets pane.SlotBracketStyle

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

	// Markable adds a mark gutter and binds x / X / A / D, so the
	// user can build a multi-selection the screen reads back with Selection().
	//
	// Off by default and free when off: the gutter takes no columns. Marking
	// requires keyed rows (SetKeyedRows) and is inert on a windowed table —
	// see mark.go.
	Markable bool

	// MarkStyle colors the ✓ on a marked row that isn't under the cursor.
	MarkStyle lipgloss.Style
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

	// Borders configures table-internal separators. Each field is emitted
	// verbatim, so pre-style with pkg/ansi.CellColor (foreground-only) so
	// the selected-row background passes through. Empty fields disable
	// the corresponding separator. Theme.Table() sets sensible defaults.
	Borders Borders

	// Keys is the table's keymap. Leave zero to use DefaultKeys; set
	// individual bindings to override (others fall back to defaults via
	// fillDefaults). theme.Table() pre-populates this.
	Keys Keys
}

// Keys is the table's keymap. Each binding carries both its dispatch
// keys (WithKeys) and its help label (WithHelp) — Update and Help()
// read from the same struct, so a custom binding propagates everywhere.
// The embedded pane.Keys covers horizontal scroll; mutate fields on
// Pane to override h-scroll without touching the rest.
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
	MarkRange          key.Binding
	SortPrev, SortNext key.Binding
	SortDir            key.Binding
	ColPrev, ColNext   key.Binding
	Pane               pane.Keys
}

// DefaultKeys returns the table's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:        key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:     key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfUp:     key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ up")),
		HalfDown:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ down")),
		Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Mark:       key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "mark")),
		MarkAll:    key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "mark all")),
		ClearMarks: key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "drop marks")),
		MarkRange:  key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "mark to here")),
		SortPrev:   key.NewBinding(key.WithKeys("["), key.WithHelp("[", "sort col-")),
		SortNext:   key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "sort col+")),
		SortDir:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort dir")),
		ColPrev:    key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("⇧←", "col-")),
		ColNext:    key.NewBinding(key.WithKeys("shift+right"), key.WithHelp("⇧→", "col+")),
		Pane:       pane.DefaultKeys(),
	}
}

// fillDefaults fills any zero-valued binding in k with its DefaultKeys()
// counterpart, so partial overrides on Options.Keys work without
// restating every field.
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
	if len(k.SortPrev.Keys()) == 0 {
		k.SortPrev = d.SortPrev
	}
	if len(k.SortNext.Keys()) == 0 {
		k.SortNext = d.SortNext
	}
	if len(k.SortDir.Keys()) == 0 {
		k.SortDir = d.SortDir
	}
	if len(k.ColPrev.Keys()) == 0 {
		k.ColPrev = d.ColPrev
	}
	if len(k.ColNext.Keys()) == 0 {
		k.ColNext = d.ColNext
	}
	k.Pane.FillDefaults()
}

// Borders controls the two interior separators a table draws — the
// inter-column glyph and the horizontal rule below the header. Both
// fields are pre-styled glyph strings; pass them through pkg/ansi.CellColor
// (foreground-only) so the selected row's background passes through
// unbroken (rule 17). Set a field to "" to disable it.
type Borders struct {
	// Vertical, when non-empty, replaces the single-space inter-column
	// separator with " <glyph> " (visible width 3). Typical values:
	// "│" (light), "┃" (heavy), "╎" (dashed). Pre-style the glyph.
	Vertical string
	// HeaderRule, when non-empty, draws a horizontal rule between the
	// header row and the first data row by repeating the field's first
	// visible rune to the table's full visible width. Typical values:
	// "─" (light), "═" (double), "·" (dotted). Pre-style the glyph; the
	// SGR escapes are extracted and re-applied around the repeated rune.
	HeaderRule string
}

// Model is the table widget. Embed as a value; mutate via the setters.
type Model struct {
	// glyphs is the resolved mark vocabulary this component draws with.
	glyphs glyph.Set

	cols     []Column
	widths   []int // effective per-column visible widths (base + flex share)
	rows     []Row
	rowKeys  []string
	markable bool
	marks    map[string]bool
	// markAnchor is the key of the most recently marked row — the fixed end
	// of a MarkRange. Held as a key, like the marks themselves, so a reorder
	// cannot slide the anchor onto a different row.
	markAnchor string
	markStyle  lipgloss.Style
	visible    []Row
	visibleIdx []int
	cursor     int
	viewStart  int

	filter     filter.Model
	filterable bool
	filterMode FilterMode
	sortMode   SortMode

	// Query tracking for QueryChangedMsg. qRaw/qSortCol/qSortDesc hold the
	// last emitted query, which doubles as the committed filter text while
	// the user is mid-edit. qSortCol starts at -1 to match "no sort", so a
	// freshly built table doesn't emit before the user touches anything.
	qRaw      string
	qSortCol  int
	qSortDesc bool
	qPending  bool

	// Windowing. When windowed, rows holds only [winStart, winStart+len)
	// of a logical set winTotal long (-1 when the source can't say), and
	// every cursor / scroll / count reads through rowCount and rowAt
	// rather than touching visible directly.
	windowed    bool
	winStart    int
	winTotal    int
	placeholder Row
	phGlyph     string

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

	colSep     string
	headerRule string

	keys Keys

	// token is this table's stable identity for focus requests. Update takes
	// a value receiver, so the model cannot name its own address.
	token focus.Token

	// filterRule{Active,Inactive} draw the line separating the inline filter
	// row from the content. The active one is used while the filter has
	// input — an inline filter has no border of its own to light up, so the
	// prompt and this rule carry that signal instead.
	filterRuleActive   lipgloss.Style
	filterRuleInactive lipgloss.Style

	// Viewport tracking for ViewportChangedMsg. vpFirst/Last/Total start at
	// -1 sentinels so the first real viewport always registers as a change.
	// vpPending is set by noteViewport when the tuple changes and cleared
	// by flushViewport, which every Update return path batches into its
	// tea.Cmd.
	vpFirst   int
	vpLast    int
	vpTotal   int
	vpPending bool

	// Focus tracking for RowFocusedMsg. focusInit flips true after the
	// first emit so we don't send an initial Empty msg for tables that
	// start empty. focusIdx/-Cells hold the last-emitted (row, cells) for
	// dedup; -1 means the last emit was Empty.
	focusInit    bool
	focusIdx     int
	focusCells   []string
	focusPending bool
}

// New constructs a table.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "Table"
	}
	opts.Keys.fillDefaults()
	cols := append([]Column(nil), opts.Columns...)
	colSep := " "
	if opts.Borders.Vertical != "" {
		colSep = " " + opts.Borders.Vertical + " "
	}
	m := Model{
		glyphs:        opts.Glyphs.Resolve(),
		cols:          cols,
		rows:          append([]Row(nil), opts.Rows...),
		filterable:    opts.Filterable,
		filterMode:    opts.FilterMode,
		sortMode:      opts.SortMode,
		sortCol:       -1,
		qSortCol:      -1,
		winTotal:      -1,
		phGlyph:       opts.Placeholder,
		headerStyle:   opts.HeaderStyle,
		selectedStyle: opts.SelectedStyle,
		cellStyle:     opts.CellStyle,
		markable:      opts.Markable,
		marks:         map[string]bool{},
		markStyle:     opts.MarkStyle,
		hScrollbar:    opts.HScrollbar,
		colSep:        colSep,
		headerRule:    opts.Borders.HeaderRule,
		keys:          opts.Keys,
		token:         focus.NewToken(),
		vpFirst:       -1,
		vpLast:        -1,
		vpTotal:       -1,
		focusIdx:      -1,
	}
	if m.phGlyph == "" {
		// Options.Placeholder still wins; the glyph set is the fallback, so a
		// theme can restyle filler rows without every caller restating it.
		m.phGlyph = m.glyphs.Placeholder
	}
	m.rebuildPlaceholder()
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
		Glyphs:         opts.Glyphs,
		SlotBrackets:   opts.SlotBrackets,
		HScrollbar:     opts.HScrollbar,
		SpinnerStyle:   opts.SpinnerStyle,
		LoadingLabel:   opts.LoadingLabel,
		Keys:           opts.Keys.Pane,
	})
	m.recomputeWidths()
	m.refresh()
	return m
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// Update consumes cursor + filter keys; non-key messages flow to the body
// pane so spinner ticks reach the loading-state animation.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	if mm, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(mm)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		m.body, cmd = m.body.Update(msg)
		return m, tea.Batch(cmd, m.flushMsgs())
	}
	if m.filterable && m.filter.Focused() {
		// Intercept tab before forwarding so the textinput doesn't insert a
		// literal tab — instead, complete the in-progress key:val term to
		// the longest common prefix of its remaining hint candidates.
		if km.String() == "tab" {
			if m.completeFilterTerm() {
				m.applyFilter()
				m.refresh()
				return m, m.flushMsgs()
			}
		}
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
		if m.cursor < m.rowCount()-1 {
			m.cursor++
			m.refresh()
		}
	case key.Matches(km, m.keys.Top):
		if m.cursor != 0 && m.rowCount() > 0 {
			m.cursor = 0
			m.refresh()
		}
	case key.Matches(km, m.keys.Bottom):
		if last := m.rowCount() - 1; last >= 0 && m.cursor != last {
			m.cursor = last
			m.refresh()
		}
	case key.Matches(km, m.keys.HalfUp):
		if m.cursor > 0 {
			m.cursor = max(0, m.cursor-m.halfPage())
			m.refresh()
		}
	case key.Matches(km, m.keys.HalfDown):
		if last := m.rowCount() - 1; last >= 0 && m.cursor < last {
			m.cursor = min(last, m.cursor+m.halfPage())
			m.refresh()
		}
	case key.Matches(km, m.keys.SortPrev):
		if m.stepSortColumn(-1) {
			m.applyFilter()
			m.refresh()
		}
	case key.Matches(km, m.keys.SortNext):
		if m.stepSortColumn(+1) {
			m.applyFilter()
			m.refresh()
		}
	case key.Matches(km, m.keys.SortDir):
		if m.toggleSortDir() {
			m.applyFilter()
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
	case key.Matches(km, m.keys.ColPrev):
		cur := m.body.XOffset()
		if e := m.prevColumnEdge(cur); e >= 0 {
			m.body.SetXOffset(e)
		} else if cur != 0 {
			m.body.SetXOffset(0)
		}
	case key.Matches(km, m.keys.ColNext):
		if e := m.nextColumnEdge(m.body.XOffset()); e >= 0 {
			m.body.SetXOffset(e)
		}
	default:
		m.body, cmd = m.body.Update(msg)
		return m, tea.Batch(cmd, m.flushMsgs())
	}
	return m, m.flushMsgs()
}

// View stacks filter (if filterable) and the body pane.
func (m Model) View() string { return m.body.View() }

// Help returns the keys this table responds to.
func (m Model) Help() []key.Binding { return help.Flatten(m.HelpSections()) }

// HelpSections groups the same bindings by what they do, for the `?`
// overlay: cursor movement, horizontal scroll, sort, filter and marking are
// separate questions, and the Keys struct already keeps them apart.
func (m Model) HelpSections() []help.Section {
	if m.filterable && m.filter.Focused() {
		out := m.filter.Help()
		if _, ok := m.activeTerm(); ok {
			out = append(out, key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "complete")))
		}
		return help.Sections(help.Group(help.SectionFilter, out...))
	}
	var scroll []key.Binding
	if m.hScrollbar {
		scroll = append(append([]key.Binding{}, m.body.HelpBindings()...), m.keys.ColPrev, m.keys.ColNext)
	}
	var sort []key.Binding
	if m.hasSortable() {
		sort = []key.Binding{m.keys.SortPrev, m.keys.SortNext, m.keys.SortDir}
	}
	var filter []key.Binding
	if m.filterable {
		filter = []key.Binding{m.keys.Filter}
	}
	var sel []key.Binding
	if m.markable {
		sel = []key.Binding{m.keys.Mark, m.keys.MarkRange, m.keys.MarkAll, m.keys.ClearMarks,
			key.NewBinding(key.WithKeys("mouse:mark"), key.WithHelp("click ✓", "mark")),
			key.NewBinding(key.WithKeys("mouse:shiftclick"), key.WithHelp("shift-click", "mark to here"))}
	}
	return help.Sections(
		help.Group(help.SectionNavigate,
			m.keys.Up, m.keys.Down,
			m.keys.HalfUp, m.keys.HalfDown,
			m.keys.Top, m.keys.Bottom),
		help.Group(help.SectionScroll, scroll...),
		help.Group(help.SectionSort, sort...),
		help.Group(help.SectionFilter, filter...),
		help.Group(help.SectionSelect, sel...),
	)
}

// SetRect places the table in the given rect. When filterable, the internal
// filter pane takes the top 3 rows and the body pane gets the rest, offset
// below it. Each child receives its own absolute rect so a click resolves to
// the right one.
func (m *Model) SetRect(r geom.Rect) {
	m.body.SetRect(r)
	if m.filterable {
		// The filter is a row inside the body pane, not a pane beside it, so
		// it reads as belonging to what it filters. Placing the pane first
		// gives the header its width; setting the header then re-measures.
		m.placeInlineFilter(r)
		m.body.SetHeader(m.filterHeader())
		m.placeInlineFilter(r)
	}
	m.recomputeWidths()
	m.refresh()
}

// placeInlineFilter puts the filter on the pane's first inner row.
func (m *Model) placeInlineFilter(r geom.Rect) {
	inner := m.body.ContentRect()
	m.filter.SetInlineRect(geom.Rect{X: inner.X, Y: m.body.Rect().Y + 1, W: inner.W, H: 1, Gen: r.Gen})
}

// filterHeader is the filter row plus a rule separating it from the content.
func (m Model) filterHeader() string {
	inner := m.body.ContentRect().W
	if inner <= 0 {
		return ""
	}
	rule := m.filterRuleInactive
	if m.filter.Focused() {
		rule = m.filterRuleActive
	}
	return m.filter.InlineView() + "\n" + rule.Render(strings.Repeat(m.glyphs.Rule, inner))
}

// rowCount is the number of rows the cursor can reach — the logical total
// when windowed (which is larger than what is resident), otherwise the
// post-filter visible count. Every bounds check goes through here; using
// len(visible) in windowed mode would pin the cursor to the page.
func (m Model) rowCount() int {
	if !m.windowed {
		return len(m.visible)
	}
	if m.winTotal >= 0 {
		return m.winTotal
	}
	// Unknown total: the user can reach the end of what has loaded, and
	// arriving there is what asks the source for more.
	return m.winStart + len(m.rows)
}

// rowAt returns the row at logical index i. ok is false when i is out of
// range, and also when the index is inside the logical range but outside
// the resident window — callers that need something to draw substitute
// the placeholder, callers that need real data must not.
func (m Model) rowAt(i int) (Row, bool) {
	if i < 0 || i >= m.rowCount() {
		return nil, false
	}
	if !m.windowed {
		return m.visible[i], true
	}
	if j := i - m.winStart; j >= 0 && j < len(m.rows) {
		return m.rows[j], true
	}
	return nil, false
}

// rebuildPlaceholder caches the filler row drawn for unresident indices,
// one glyph per column. Rebuilt whenever the column set changes.
func (m *Model) rebuildPlaceholder() {
	row := make(Row, len(m.cols))
	for i := range row {
		row[i] = m.phGlyph
	}
	m.placeholder = row
}

// SetWindow installs a sparse window: rows are the logical indices
// [offset, offset+len(rows)) of a set total rows long. Pass total < 0 when
// the source can't say (cursor-paginated APIs); the table then treats the
// end of what has loaded as the end, which grows as more arrives.
//
// The cursor is a logical index and does not move when a window lands, so
// scrolling to row 800 and having that window arrive leaves the cursor on
// row 800. Indices the window doesn't hold render as Placeholder and
// report ok=false from Selected, so a screen can't act on a row it hasn't
// actually received.
//
// Windowing implies the rows are the source's answer: filter and sort are
// never applied locally to a window, whatever FilterMode / SortMode say,
// because filtering one page of a larger set is not filtering. Pair it
// with FilterRemote / SortRemote so the query the user builds actually
// reaches the source. Prefer fixed or Flex column widths too —
// content-auto sizes to the widest resident cell, so columns reflow every
// time the window swaps.
func (m *Model) SetWindow(rows []Row, offset, total int) {
	if offset < 0 {
		offset = 0
	}
	m.windowed = true
	m.winStart = offset
	m.winTotal = total
	m.rows = append([]Row(nil), rows...)
	m.rowKeys = nil
	m.rebuildDistinct()
	m.recomputeWidths()
	m.applyFilter()
	m.refresh()
}

// Window reports the resident window: the logical index of its first row,
// how many rows are resident, and the logical total (-1 when unknown).
// A coordinator turning ViewportChangedMsg into fetches reads this to
// decide whether the rows on screen are already in hand.
func (m Model) Window() (offset, count, total int) {
	if !m.windowed {
		return 0, len(m.rows), len(m.rows)
	}
	return m.winStart, len(m.rows), m.winTotal
}

// clearWindow returns the table to holding its whole row set, so the
// non-windowed setters undo a previous SetWindow rather than leaving a
// stale offset behind.
func (m *Model) clearWindow() {
	m.windowed = false
	m.winStart = 0
	m.winTotal = -1
}

// SetRows replaces the row set, re-applies the current filter, redraws.
// Cursor is preserved by visible index — fine for static datasets, but
// callers polling a live source should prefer SetKeyedRows so the cursor
// rebinds to the same logical row even when neighbours come and go.
func (m *Model) SetRows(rows []Row) {
	m.clearWindow()
	m.rows = append([]Row(nil), rows...)
	m.rowKeys = nil
	m.rebuildDistinct()
	m.recomputeWidths()
	m.applyFilter()
	m.refresh()
}

// SetKeyedRows replaces the row set with each row carrying a stable Key,
// then snaps the cursor to whichever row in the new set shares the
// previously-selected Key. When the previous Key has disappeared (row
// removed from the source), the cursor falls back to the clamped
// previous index so the user lands near where they were. The filter and
// sort state are preserved across the swap. This is the primitive
// pkg/poll uses to keep the user's place across periodic refreshes.
func (m *Model) SetKeyedRows(rows []KeyedRow) {
	m.clearWindow()
	prevKey, hadKey := m.SelectedKey()
	prevCursor := m.cursor

	m.rows = make([]Row, len(rows))
	m.rowKeys = make([]string, len(rows))
	for i, r := range rows {
		m.rows[i] = append(Row(nil), r.Cells...)
		m.rowKeys[i] = r.Key
	}
	m.rebuildDistinct()
	m.recomputeWidths()
	m.applyFilter()

	if hadKey {
		for i, src := range m.visibleIdx {
			if src >= 0 && src < len(m.rowKeys) && m.rowKeys[src] == prevKey {
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

// SetColumns replaces the column layout. Cell text is preserved;
// effective widths recompute on the next refresh.
func (m *Model) SetColumns(cols []Column) {
	m.cols = append([]Column(nil), cols...)
	m.rebuildPlaceholder()
	m.rebuildDistinct()
	m.recomputeWidths()
	m.refresh()
}

// SetCursor moves the cursor (clamped) and scrolls to keep it on screen.
func (m *Model) SetCursor(n int) {
	m.cursor = max(0, min(n, m.rowCount()-1))
	m.refresh()
}

// SetValue overwrites the filter text (no-op when not filterable). Under
// FilterRemote this adopts the new text as the committed baseline without
// emitting QueryChangedMsg — it is the setter a SetTheme rebuild uses, and
// a rebuild is not a new query. Call Query() yourself if you set a filter
// programmatically and want it fetched.
func (m *Model) SetValue(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.applyFilter()
	m.refresh()
	m.syncQuery()
}

// SetDistinct supplies the candidate values behind col's filter hint and
// tab completion. This is the FilterRemote counterpart to scraping them
// from resident rows: feed it a facet endpoint, an enum, or a schema, and
// completion suggests values the source actually has rather than the
// handful that happen to be on this page. Values are normalized on the way
// in, so pass them however the server spells them.
//
// Under FilterLocal the candidates are recomputed from the rows on the next
// row or column change, which will overwrite anything set here.
func (m *Model) SetDistinct(col int, values []string) {
	if col < 0 || col >= len(m.cols) {
		return
	}
	m.ensureDistinct()
	m.distinct[col] = query.NormalizeValues(values)
}

// SetTitle updates the title rendered on the body pane's top border.
func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

// Title returns the label on the pane's border.
func (m Model) Title() string { return m.body.Title() }

// Focus gives the component the keyboard, highlighting the body pane.
//
// It deliberately does nothing when the filter already owns input: a click on
// the filter also asks the group for focus, and that grant arrives afterwards.
// Without this guard it would snatch the highlight back to the body while the
// filter kept the keystrokes.
func (m *Model) Focus() tea.Cmd {
	m.body.SetFocused(true)
	return nil
}

// Blur releases the keyboard, clearing *both* regions. Leaving a filter
// focused on a blurred component is what lets a second filterable pane end up
// invisibly eating keys.
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

// FocusFilter moves input to the filter and takes the highlight off the body,
// so exactly one region ever reads as active.
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

// SetActiveColor / SetInactiveColor update the body pane's border colors.
// Useful for theme swaps that don't rebuild the model.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor)   { m.body.SetActiveColor(c) }
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

// SetActiveBorder updates the border shape drawn while focused.
func (m *Model) SetActiveBorder(b lipgloss.Border) { m.body.SetActiveBorder(b) }

// SetInactiveBorder updates the border shape drawn while unfocused.
func (m *Model) SetInactiveBorder(b lipgloss.Border) { m.body.SetInactiveBorder(b) }

// SetHeaderStyle / SetSelectedStyle / SetCellStyle update row styling.
func (m *Model) SetHeaderStyle(s lipgloss.Style)   { m.headerStyle = s; m.refresh() }
func (m *Model) SetSelectedStyle(s lipgloss.Style) { m.selectedStyle = s; m.refresh() }
func (m *Model) SetCellStyle(s lipgloss.Style)     { m.cellStyle = s; m.refresh() }

// Selected returns the currently highlighted row. ok is false when the
// visible set (post-filter) is empty, and — under SetWindow — when the
// cursor sits on a logical index the resident window doesn't hold, so a
// screen never acts on a row it hasn't received.
func (m Model) Selected() (Row, bool) {
	return m.rowAt(m.cursor)
}

// SelectedIndex returns the highlighted row's index into the original
// (pre-filter) Rows() slice. Use this when callers maintain a parallel
// source slice and need to identify which source row is selected.
func (m Model) SelectedIndex() (int, bool) {
	if m.windowed {
		if m.cursor < 0 || m.cursor >= m.rowCount() {
			return 0, false
		}
		return m.cursor, true
	}
	if m.cursor < 0 || m.cursor >= len(m.visibleIdx) {
		return 0, false
	}
	return m.visibleIdx[m.cursor], true
}

// SelectedKey returns the highlighted row's Key when the table was
// populated via SetKeyedRows. ok is false when no row is selected, or
// when the rows were set via SetRows (which carries no keys). Callers
// that drive a polled source should track the selection by Key, not by
// SelectedIndex, so re-fetches that reorder rows don't shift the
// selection out from under the user.
func (m Model) SelectedKey() (string, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visibleIdx) {
		return "", false
	}
	src := m.visibleIdx[m.cursor]
	if src < 0 || src >= len(m.rowKeys) {
		return "", false
	}
	return m.rowKeys[src], true
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

// IsCapturingKeys reports whether the table currently swallows printable
// keys — true while its filter is focused. Satisfies focus.Capturer.
func (m Model) IsCapturingKeys() bool { return m.Filtering() }

// SortColumn returns the active sort column index (-1 when no sort).
func (m Model) SortColumn() int { return m.sortCol }

// SortDescending reports the active sort direction.
func (m Model) SortDescending() bool { return m.sortDesc }

// SetSort sets the sort column and direction. col == -1 disables sort;
// otherwise col must reference a Sortable column. Use this on rebuild
// (theme swap) to carry SortColumn/SortDescending across the new model.
// Under SortRemote it adopts the sort silently, for the same reason
// SetValue does: restoring state is not the user asking for a new sort.
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
	m.syncQuery()
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
	if !m.filterable || m.filterMode == FilterRemote || m.windowed {
		m.visible = append([]Row(nil), m.rows...)
		m.visibleIdx = identityIndex(len(m.rows))
	} else {
		q := strings.TrimSpace(m.filter.Value())
		if q == "" {
			m.visible = append([]Row(nil), m.rows...)
			m.visibleIdx = identityIndex(len(m.rows))
		} else {
			terms := query.Parse(q, m.columnTitles())
			out := make([]Row, 0, len(m.rows))
			idx := make([]int, 0, len(m.rows))
			for i, r := range m.rows {
				if query.MatchAll(r, terms) {
					out = append(out, r)
					idx = append(idx, i)
				}
			}
			m.visible = out
			m.visibleIdx = idx
		}
	}
	m.applySort()
	if m.cursor >= m.rowCount() {
		m.cursor = max(0, m.rowCount()-1)
	}
}

// applySort sorts visible (and visibleIdx in lockstep) by the active
// sort column. No-op when sortCol is unset, out of range, or not Sortable.
func (m *Model) applySort() {
	if m.sortMode == SortRemote || m.windowed {
		return
	}
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

// columnTitles returns the column titles in column order — the shape
// pkg/query resolves "key:value" scopes against. Hidden columns are
// included: they are filterable even though they aren't drawn.
func (m Model) columnTitles() []string {
	out := make([]string, len(m.cols))
	for i, c := range m.cols {
		out[i] = c.Title
	}
	return out
}

// activeTerm reports the in-progress "key:val" term in the filter input,
// or ok=false when the table isn't filterable or nothing is being typed.
func (m Model) activeTerm() (query.Active, bool) {
	if !m.filterable {
		return query.Active{}, false
	}
	return query.ActiveTerm(m.filter.Value(), m.columnTitles())
}

// halfPage is the cursor step for ctrl+u/ctrl+d — half the viewport
// (excluding the header row), floor 1.
func (m Model) halfPage() int {
	if n := (m.body.VisibleRows() - 1) / 2; n > 0 {
		return n
	}
	return 1
}

// columnEdges returns the screen-coordinate left edge of each visible
// column, accounting for widths and the inter-column separator. Hidden
// columns don't appear on screen so they're excluded — column-snap
// navigation steps between visible columns only.
func (m Model) columnEdges() []int {
	if len(m.widths) == 0 {
		return nil
	}
	sepW := ansi.StringWidth(m.colSep)
	edges := make([]int, 0, len(m.widths))
	x := 0
	for i, w := range m.widths {
		if m.cols[i].Hidden {
			continue
		}
		edges = append(edges, x)
		x += w + sepW
	}
	return edges
}

// nextColumnEdge returns the smallest column-left-edge strictly greater
// than x — the snap target for "step right one column". Returns -1 when
// x is at or past the last column's edge.
func (m Model) nextColumnEdge(x int) int {
	for _, e := range m.columnEdges() {
		if e > x {
			return e
		}
	}
	return -1
}

// prevColumnEdge returns the largest column-left-edge strictly less than
// x — the snap target for "step left one column". Returns -1 when x is
// already at or before column 0.
func (m Model) prevColumnEdge(x int) int {
	best := -1
	for _, e := range m.columnEdges() {
		if e < x {
			best = e
			continue
		}
		break
	}
	return best
}

// recomputeWidths fills m.widths with the effective per-column visible
// width. Each column gets a base width:
//   - Width > 0           → Width
//   - Width == 0, no rows → max(visible(Title), 4)
//   - Width == 0, rows    → max(visible(Title), max cell visible width, 4)
//
// Then any leftover horizontal space (body inner width minus base
// widths and inter-column separators) is distributed across columns
// with Flex > 0 in proportion to their Flex weights. Flex columns
// only ever grow — never shrink below their base. If the body is
// narrower than the sum of bases, no expansion happens; the pane's
// h-scroll handles overflow.
func (m *Model) recomputeWidths() {
	n := len(m.cols)
	m.widths = make([]int, n)
	visibleN := 0
	for i, c := range m.cols {
		if c.Hidden {
			m.widths[i] = 0
			continue
		}
		visibleN++
		if c.Width > 0 {
			m.widths[i] = c.Width
			continue
		}
		w := ansi.StringWidth(c.Title)
		for _, r := range m.rows {
			if i < len(r) {
				if cw := ansi.StringWidth(ansi.Strip(r[i])); cw > w {
					w = cw
				}
			}
		}
		if w < 4 {
			w = 4
		}
		m.widths[i] = w
	}

	hasFlex := false
	for _, c := range m.cols {
		if !c.Hidden && c.Flex > 0 {
			hasFlex = true
			break
		}
	}
	if !hasFlex {
		return
	}
	inner := m.body.VisibleWidth() - m.gutterW()
	if inner <= 0 {
		return
	}
	sepW := ansi.StringWidth(m.colSep)
	used := 0
	if visibleN > 1 {
		used = (visibleN - 1) * sepW
	}
	for _, w := range m.widths {
		used += w
	}
	leftover := inner - used
	if leftover <= 0 {
		return
	}
	uncapped := make([]int, 0, n)
	for i, c := range m.cols {
		if !c.Hidden && c.Flex > 0 {
			uncapped = append(uncapped, i)
		}
	}
	// Iterate: distribute proportionally; if a column would exceed its
	// MaxWidth, clamp it and remove from the pool; redistribute remaining
	// leftover among the rest. Terminates because each iteration either
	// reaches equilibrium (no cap hit) or removes ≥1 column.
	for len(uncapped) > 0 && leftover > 0 {
		totalFlex := 0
		for _, i := range uncapped {
			totalFlex += m.cols[i].Flex
		}
		if totalFlex == 0 {
			break
		}
		nextUncapped := uncapped[:0:0]
		distributed := 0
		anyCapped := false
		for k, i := range uncapped {
			var share int
			if k == len(uncapped)-1 {
				share = leftover - distributed
			} else {
				share = leftover * m.cols[i].Flex / totalFlex
			}
			if cap := m.cols[i].MaxWidth; cap > 0 && m.widths[i]+share > cap {
				share = cap - m.widths[i]
				if share < 0 {
					share = 0
				}
				anyCapped = true
			} else {
				nextUncapped = append(nextUncapped, i)
			}
			m.widths[i] += share
			distributed += share
		}
		leftover -= distributed
		if !anyCapped {
			break
		}
		uncapped = nextUncapped
	}
}

// viewport returns the current visible window into the filtered/sorted
// row slice. valid=false when the viewport isn't real yet — no dimensions
// applied, or no rows in view — so callers can skip emission until a
// meaningful window exists.
func (m Model) viewport() (first, last, total int, valid bool) {
	dr := m.dataRows()
	n := m.rowCount()
	if dr <= 0 || n == 0 {
		return 0, 0, 0, false
	}
	first = m.viewStart
	last = first + dr - 1
	if last >= n {
		last = n - 1
	}
	total = n
	return first, last, total, true
}

// noteViewport samples the current viewport and, if it differs from the
// last emitted tuple, marks a ViewportChangedMsg for the next flush. It
// no-ops when the viewport isn't valid yet, so an early SetRows before
// the first WindowSizeMsg produces no msg — the message only fires once
// the viewport is real.
func (m *Model) noteViewport() {
	first, last, total, valid := m.viewport()
	if !valid {
		return
	}
	if first == m.vpFirst && last == m.vpLast && total == m.vpTotal {
		return
	}
	m.vpFirst, m.vpLast, m.vpTotal = first, last, total
	m.vpPending = true
}

// flushViewport returns a tea.Cmd carrying the pending ViewportChangedMsg
// and clears the pending flag, or nil when nothing has changed since the
// last emission. Every Update return path batches this so the message
// reaches the parent one tick after the state change that produced it.
func (m *Model) flushViewport() tea.Cmd {
	if !m.vpPending {
		return nil
	}
	m.vpPending = false
	msg := ViewportChangedMsg{
		FirstVisible: m.vpFirst,
		LastVisible:  m.vpLast,
		TotalRows:    m.vpTotal,
	}
	return func() tea.Msg { return msg }
}

// rowCellsEqual is a length + element string equality for the last-
// emitted cells slice; used by noteFocus to dedup a SetRows that lands
// the same content under the cursor.
func rowCellsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// noteFocus samples the current focused row and, if it differs from the
// last emitted (row, cells) tuple, marks a RowFocusedMsg for the next
// flush. An empty visible slice is only reported once we've previously
// emitted a non-empty focus — an initially-empty table skips the msg so
// parents don't get an Empty ping before any rows exist.
func (m *Model) noteFocus() {
	cur, resident := m.rowAt(m.cursor)
	if !resident {
		if !m.focusInit || m.focusIdx == -1 {
			return
		}
		m.focusIdx = -1
		m.focusCells = nil
		m.focusPending = true
		return
	}
	cells := []string(cur)
	if m.focusInit && m.focusIdx == m.cursor && rowCellsEqual(m.focusCells, cells) {
		return
	}
	m.focusIdx = m.cursor
	m.focusCells = append([]string(nil), cells...)
	m.focusPending = true
}

// flushMsgs batches every pending emit (viewport, focus, query) into a
// single tea.Cmd. Update return paths call this so callers don't need to
// know which specific subset changed on any given tick.
func (m *Model) flushMsgs() tea.Cmd {
	return tea.Batch(m.flushViewport(), m.flushFocus(), m.flushQuery())
}

// currentQuery samples the query a remote source should be answering. A
// focused filter reports its last committed text rather than what is being
// typed, so a sort change mid-edit doesn't leak a half-written filter.
func (m Model) currentQuery() (raw string, sortCol int, sortDesc bool) {
	sortCol = -1
	if m.sortMode == SortRemote {
		sortCol, sortDesc = m.sortCol, m.sortDesc
	}
	if m.filterMode == FilterRemote && m.filterable {
		if m.filter.Focused() {
			raw = m.qRaw
		} else {
			raw = strings.TrimSpace(m.filter.Value())
		}
	}
	return raw, sortCol, sortDesc
}

// noteQuery marks a QueryChangedMsg when the committed query differs from
// the last emitted one. Called from refresh, so every state change samples
// it; duplicates are elided here rather than at the call sites.
func (m *Model) noteQuery() {
	if m.filterMode != FilterRemote && m.sortMode != SortRemote {
		return
	}
	raw, sortCol, sortDesc := m.currentQuery()
	if raw == m.qRaw && sortCol == m.qSortCol && sortDesc == m.qSortDesc {
		return
	}
	m.qRaw, m.qSortCol, m.qSortDesc = raw, sortCol, sortDesc
	m.qPending = true
}

// syncQuery adopts the current query as the emitted baseline without
// emitting. The state-restoration setters use it so a SetTheme rebuild —
// which replays the filter text and sort onto a fresh model (rule 4) —
// doesn't read as a user-driven change and trigger a refetch.
func (m *Model) syncQuery() {
	m.qRaw, m.qSortCol, m.qSortDesc = m.currentQuery()
	m.qPending = false
}

// flushQuery returns a tea.Cmd carrying the pending QueryChangedMsg and
// clears the pending flag, or nil when the query hasn't changed.
func (m *Model) flushQuery() tea.Cmd {
	if !m.qPending {
		return nil
	}
	m.qPending = false
	msg := m.Query()
	return func() tea.Msg { return msg }
}

// Query returns the query a remote source should currently be answering.
// Screens call it for the first fetch, before the user has touched
// anything, so the initial load runs through the same code path as every
// QueryChangedMsg that follows.
func (m Model) Query() QueryChangedMsg {
	raw, sortCol, sortDesc := m.currentQuery()
	out := QueryChangedMsg{Raw: raw, SortColumn: -1}
	if raw != "" {
		out.Terms = query.Parse(raw, m.columnTitles())
	}
	if sortCol >= 0 && sortCol < len(m.cols) {
		out.Sort = m.cols[sortCol].Title
		out.SortColumn = sortCol
		out.Desc = sortDesc
	}
	return out
}

// flushFocus returns a tea.Cmd carrying the pending RowFocusedMsg and
// clears the pending flag, or nil when nothing has changed. Every Update
// return path batches this alongside flushViewport.
func (m *Model) flushFocus() tea.Cmd {
	if !m.focusPending {
		return nil
	}
	m.focusPending = false
	m.focusInit = true
	if m.focusIdx < 0 {
		return func() tea.Msg { return RowFocusedMsg{Empty: true} }
	}
	cells := append([]string(nil), m.focusCells...)
	cols := make([]string, len(m.cols))
	for i, c := range m.cols {
		cols[i] = c.Title
	}
	idx := m.focusIdx
	return func() tea.Msg {
		return RowFocusedMsg{Row: idx, Cells: cells, Columns: cols}
	}
}

// FocusToken returns the table's stable focus identity. See focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// ActivatedMsg is emitted when the user opens the selected row with a double
// click — the mouse spelling of enter (rule 14). Row is the index into the
// post-filter visible set; Cells is that row's content.
//
// Token identifies which table sent it, so a screen holding several can tell
// them apart. Prefer IsActivate over matching this type directly unless you
// need the payload.
type ActivatedMsg struct {
	Row   int
	Cells []string
	Token focus.Token
}

// IsActivate reports whether msg means "open this table's selection" — enter
// from the keyboard while the filter isn't taking input, or this table's own
// double-click activation. See list.Model.IsActivate; rule 14 makes the two
// inputs one verb, and this predicate is what keeps them that way.
func (m Model) IsActivate(msg tea.Msg) bool {
	switch k := msg.(type) {
	case tea.KeyMsg:
		return !m.Filtering() && k.String() == "enter"
	case ActivatedMsg:
		return k.Token == m.token
	}
	return false
}

// headerRows is how many content lines the header occupies before the first
// data row: the header itself, plus the rule when Borders.HeaderRule is set.
func (m Model) headerRows() int {
	if m.headerRule != "" {
		return 2
	}
	return 1
}

// handleMouse routes a mouse event that may or may not belong to this table.
//
// The table windows its own rows rather than scrolling the pane, so a click
// maps through viewStart instead of the pane's scroll offset. Content line 0
// is the pinned header (line 1 too, with a rule); clicking there sorts by the
// column under the pointer rather than selecting a row.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if target, ok := m.body.HandleScrollbar(e); ok {
		m.scrollTo(target)
		return m, m.flushMsgs()
	}
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

	line, inBody := m.body.RowAt(e.X, e.Y)
	if !inBody {
		return m, nil
	}

	switch {
	case e.IsWheelUp():
		m.moveCursor(-1)
		return m, m.flushMsgs()

	case e.IsWheelDown():
		m.moveCursor(1)
		return m, m.flushMsgs()

	case e.IsPointPress():
		if line < m.headerRows() {
			// Sorting is chrome that acts, so it stays left-only; a right
			// press on the header still claims focus.
			var sort tea.Cmd
			if e.IsPress() {
				sort = m.clickHeader(e.X)
			}
			return m, tea.Batch(sort, focus.RequestSelf(m.token), m.flushMsgs())
		}
		cmds := []tea.Cmd{focus.RequestSelf(m.token)}
		row := m.viewStart + (line - m.headerRows())
		if row >= 0 && row < m.rowCount() {
			m.cursor = row
			// Clicking the ✓ gutter toggles the mark, the way a tree's ▸
			// toggles on a single click (rule 28). Returns before the
			// activate branch: a double click here would otherwise toggle
			// twice and open the row as well.
			if e.IsPress() && m.markable && m.onMarkColumn(e.X) {
				m.toggleMarkAt(row)
				cmds = append(cmds, m.flushMsgs())
				return m, tea.Batch(cmds...)
			}
			m.refresh()
			if cur, resident := m.rowAt(m.cursor); resident && e.IsDoubleClick() {
				idx, tok := m.cursor, m.token
				cells := append([]string(nil), cur...)
				cmds = append(cmds, func() tea.Msg {
					return ActivatedMsg{Row: idx, Cells: cells, Token: tok}
				})
			}
		}
		cmds = append(cmds, m.flushMsgs())
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// clickHeader sorts by the column under x, mirroring what "[", "]" and "s"
// do from the keyboard: clicking the active sort column flips direction,
// clicking a different Sortable column sorts by it ascending. Columns that
// aren't Sortable ignore the click.
func (m *Model) clickHeader(x int) tea.Cmd {
	col, ok := m.columnAt(x)
	if !ok || col >= len(m.cols) || !m.cols[col].Sortable {
		return nil
	}
	if m.sortCol == col {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortCol, m.sortDesc = col, false
	}
	m.applySort()
	m.refresh()
	return nil
}

// columnAt maps a terminal x to a column index, accounting for the pane's
// border, its horizontal scroll offset, hidden columns, and the separator
// between columns. Reports ok=false when x lands on a separator or past the
// last column.
func (m Model) columnAt(x int) (int, bool) {
	c := m.body.ContentRect()
	// Position within the rendered row, undoing the h-scroll offset.
	pos := (x - c.X) + m.body.XOffset() - m.gutterW()
	if pos < 0 {
		return 0, false
	}
	sepW := ansi.StringWidth(m.colSep)
	at := 0
	first := true
	for i, col := range m.cols {
		if col.Hidden {
			continue
		}
		if !first {
			at += sepW
		}
		first = false
		w := 0
		if i < len(m.widths) {
			w = m.widths[i]
		}
		if pos >= at && pos < at+w {
			return i, true
		}
		at += w
	}
	return 0, false
}

// scrollTo puts row at the top of the data area and moves the cursor onto
// it. The table windows its own rows from viewStart, so setting both keeps
// adjustViewStart from sliding the window back on the next refresh.
func (m *Model) scrollTo(row int) {
	n := m.rowCount()
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
	m.viewStart = row
	m.refresh()
}

// moveCursor steps the cursor by delta, clamped to the visible set.
func (m *Model) moveCursor(delta int) {
	next := m.cursor + delta
	if next < 0 || next >= m.rowCount() {
		return
	}
	m.cursor = next
	m.refresh()
}

// dataRows returns the body height available for data rows (the pane's
// inner viewport minus the one row reserved for the header, and one
// additional row when Borders.HeaderRule is set).
func (m Model) dataRows() int {
	reserved := 1
	if m.headerRule != "" {
		reserved = 2
	}
	return max(0, m.body.VisibleRows()-reserved)
}

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
	maxStart := max(0, m.rowCount()-dr)
	if m.viewStart > maxStart {
		m.viewStart = maxStart
	}
	if m.viewStart < 0 {
		m.viewStart = 0
	}
}

func (m *Model) refresh() {
	m.adjustViewStart()
	gutterPad := strings.Repeat(" ", m.gutterW())
	header := m.headerStyle.Render(gutterPad + renderRow(m.headerCells(), m.cols, m.widths, m.colSep))

	dr := m.dataRows()
	n := m.rowCount()
	end := m.viewStart + dr
	if end > n {
		end = n
	}

	var b strings.Builder
	b.WriteString(header)
	if m.headerRule != "" {
		b.WriteByte('\n')
		b.WriteString(buildRule(m.headerRule, ansi.StringWidth(header)))
	}
	for i := m.viewStart; i < end; i++ {
		b.WriteByte('\n')
		cells, resident := m.rowAt(i)
		if !resident {
			cells = m.placeholder
		}
		row := renderRow([]string(cells), m.cols, m.widths, m.colSep)
		switch {
		case i == m.cursor:
			// One styled run over the whole row, gutter included: a nested
			// mark style would close the highlight at its first reset and
			// punch a hole in the selected row's background (rule 19).
			b.WriteString(m.selectedStyle.Render(m.gutterFor(i) + row))
		case m.markable && m.isMarkedAt(i):
			b.WriteString(m.markStyle.Render(m.glyphs.Mark) + m.cellStyle.Render(" "+row))
		default:
			b.WriteString(m.cellStyle.Render(m.gutterFor(i) + row))
		}
	}
	if m.filterable {
		m.body.SetHeader(m.filterHeader())
	}
	m.body.SetContent(b.String())

	// Drive the pane's right-edge scrollbar from our logical row counts so
	// the thumb reflects position within the dataset, not within the
	// pane's in-viewport slice (which is always one window's worth).
	m.body.SetVirtualScroll(n, dr, m.viewStart)

	if m.filterable {
		// Windowed, the interesting ratio is how much of the set is in
		// hand; otherwise it's how much of it survived the filter.
		held := len(m.visible)
		if m.windowed {
			held = len(m.rows)
		}
		m.body.SetBottomLeft(fmt.Sprintf("%d / %s", held, m.totalLabel()))
		m.filter.SetBottomLeft(m.filterHint())
	}
	if n > 0 {
		m.body.SetBottomRight(fmt.Sprintf("%d / %s", m.cursor+1, m.totalLabel()))
	} else {
		m.body.SetBottomRight("")
	}

	m.noteViewport()
	m.noteFocus()
	m.noteQuery()
}

// totalLabel renders the logical row count for the pane's border slots.
// A window whose source didn't report a total gets a trailing "+", since
// what the table can currently reach is a floor, not the total.
func (m Model) totalLabel() string {
	if m.windowed && m.winTotal < 0 {
		return fmt.Sprintf("%d+", m.rowCount())
	}
	return fmt.Sprintf("%d", m.rowCount())
}

// rebuildDistinct recomputes the per-column candidate sets backing the
// filter hint and tab completion. Cheap to recompute; called only on
// row/column mutations, not per keystroke.
func (m *Model) rebuildDistinct() {
	if m.filterMode == FilterRemote || m.windowed {
		// Candidates come from SetDistinct, not from resident rows: one page
		// of a larger set completes to answers that are wrong rather than
		// merely incomplete. Keep whatever was fed, resized to the columns.
		m.ensureDistinct()
		return
	}
	raw := make([][]string, len(m.rows))
	for i, r := range m.rows {
		raw[i] = r
	}
	m.distinct = query.Distinct(raw, len(m.cols))
}

// ensureDistinct resizes the candidate table to the current column count,
// preserving the entries that survive the resize.
func (m *Model) ensureDistinct() {
	if len(m.distinct) == len(m.cols) {
		return
	}
	next := make([][]string, len(m.cols))
	copy(next, m.distinct)
	m.distinct = next
}

// filterHint formats the hint string written into the filter pane's
// bottom-left slot. Empty when there's no in-progress key:val term, when
// no distinct values match the prefix, or when the filter is blurred.
func (m Model) filterHint() string {
	if !m.filter.Focused() {
		return ""
	}
	act, ok := m.activeTerm()
	if !ok {
		return ""
	}
	cands := query.Candidates(m.distinct, act.Column, act.Value)
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
// common prefix of its remaining candidates and writes it back into the
// filter. Returns true when the value actually changed.
func (m *Model) completeFilterTerm() bool {
	if !m.filterable {
		return false
	}
	next, ok := query.Complete(m.filter.Value(), m.columnTitles(), m.distinct)
	if !ok {
		return false
	}
	m.filter.SetValue(next)
	return true
}

func (m Model) headerCells() []string {
	out := make([]string, len(m.cols))
	for i, c := range m.cols {
		title := c.Title
		if i == m.sortCol {
			if m.sortDesc {
				title += " " + m.glyphs.SortDesc
			} else {
				title += " " + m.glyphs.SortAsc
			}
		}
		out[i] = title
	}
	return out
}

// renderRow lays cells out across cols joined by sep. Each cell is
// ANSI-aware truncated/padded to widths[i] and aligned per col.Align.
// widths is the effective width per column (Column.Width plus any flex
// share); pass it from m.widths so the same row geometry is used by
// header, data rows, and the header rule. sep is " " by default; with
// Options.Borders.Vertical set, it becomes " <glyph> ". Columns with
// Hidden=true are skipped entirely — no cell, no separator around them.
func renderRow(cells []string, cols []Column, widths []int, sep string) string {
	parts := make([]string, 0, len(cols))
	for i, col := range cols {
		if col.Hidden {
			continue
		}
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		parts = append(parts, formatCell(cell, w, col.Align))
	}
	return strings.Join(parts, sep)
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

// buildRule expands a styled glyph string into a horizontal rule of the
// given visible width. The rule's first non-control rune is the glyph;
// any leading SGR escapes form the prefix and any trailing bytes form
// the suffix, so a value like "\x1b[38;5;240m─\x1b[39m" expands to one
// open SGR + repeated glyph + one close SGR. When the input has no
// visible rune, returns an empty string.
func buildRule(styledGlyph string, width int) string {
	if styledGlyph == "" || width <= 0 {
		return ""
	}
	prefix, glyph, suffix, ok := splitGlyphStyle(styledGlyph)
	if !ok {
		return ""
	}
	return prefix + strings.Repeat(string(glyph), width) + suffix
}

// splitGlyphStyle splits a styled-glyph string into its leading SGR
// prefix, first visible rune, and trailing suffix. CSI SGR sequences
// (\x1b[...m) before the first visible rune are absorbed into prefix;
// everything after the rune (including the closing \x1b[39m) is suffix.
func splitGlyphStyle(s string) (prefix string, glyph rune, suffix string, ok bool) {
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j >= len(s) {
				return "", 0, "", false
			}
			i = j + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError {
			return "", 0, "", false
		}
		return s[:i], r, s[i+size:], true
	}
	return "", 0, "", false
}
