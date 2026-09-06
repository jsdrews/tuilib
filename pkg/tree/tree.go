// Package tree provides a searchable, expand/collapse hierarchical view
// inside a bordered pane. It accepts any data shape that satisfies the
// minimal Node interface (Label + Children) and renders a flat list of
// visible rows with depth-aware indentation, a "▸/▾" expand glyph, and a
// cursor.
//
// Use it for file trees, JSON browsers, nested categories — anything where
// the user wants to drill into a hierarchy. The body is a pkg/pane.Pane
// (so pgup/pgdn/arrows/mouse-wheel and horizontal scroll work out of the
// box) plus an optional pkg/filter.Model for the "/-to-search" overlay.
//
// Features:
//   - Cursor + scroll across the flat visible-row slice
//   - Space toggles the cursor's expand state, descending into children
//     when expanded; enter selects; E expands every node in the tree,
//     C collapses everything back to root. Arrow keys and hjkl follow the
//     library-wide scroll convention (rule 23) and are reserved for
//     vertical/horizontal scroll — they do not expand/collapse.
//   - "/" focuses an embedded filter (case-insensitive substring against
//     Label()); matches highlight inline with MatchStyle
//   - "\" toggles filter mode: when on, only matching nodes (and their
//     ancestors, so the path stays readable) are shown; n/N step through
//     matches
//   - InitialDepth controls how deep the tree starts pre-expanded
//
// Items are caller-owned via the Node interface — copy from your own
// data shape without forcing tuilib's structs into your domain model.
package tree

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

// Node is the minimal contract a caller's data must satisfy. Label is the
// row's display text; Children returns the immediate descendants (nil or
// empty for leaves). Tree never mutates the returned slice.
type Node interface {
	Label() string
	Children() []Node
}

// SelectedChangedMsg is emitted by the tree when the cursor lands on a
// different node than the last emit — after cursor movement, SetRoot
// swaps that change the focused node, expand/collapse ops that shift the
// cursor, or on the initial view. Parents subscribe to it to drive
// "detail on hover" patterns: refetch a parameterized detail source
// keyed on the focused node's path. Dedup is on the path (label-based,
// same identity scheme SetRoot uses to preserve expand state); an
// identical-structure SetRoot doesn't re-emit. Empty=true fires only as
// a transition (had focus → no focus) — an initially-empty tree never
// emits.
type SelectedChangedMsg struct {
	// Path is the label-based path from root to the focused node. Nil
	// when Empty is true. Uses the same path scheme as SetRoot's
	// expand-state preservation — sibling labels with a duplicate-
	// suffix ("Pod", "Pod:2") are included in the array.
	Path []string
	// Label is the focused node's own label — Path[len(Path)-1] for
	// convenience. Empty when Empty is true.
	Label string
	// Depth is 0 for the root, 1 for its children, etc. Zero when Empty.
	Depth int
	// Empty is true when no node is focused. Path / Label / Depth are
	// zero-valued.
	Empty bool
}

// Options configures a new tree. Zero-value fields fall back to defaults.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "tree".
	Title string
	// Root is the top-level node. Its own label is rendered as the first
	// row; pass a synthetic root if your data has multiple top-level items.
	Root Node
	// Searchable embeds a filter.Model above the body pane (three rows). If
	// false, "/" is ignored and the full height is used for the tree.
	Searchable bool
	// InitialDepth pre-expands every node whose depth is < InitialDepth.
	// 0 (default) shows only the root collapsed, 1 expands the root,
	// 2 expands the root and its direct children, etc.
	InitialDepth int

	// MatchStyle is the lipgloss style applied to matched substrings while
	// a query is active. Pass via theme.Tree() for a sensible default.
	MatchStyle lipgloss.Style

	// CurrentLineStyle is applied to the entire row holding the cursor,
	// padded out to the pane's inner width. theme.Tree() seeds a subtle
	// background.
	CurrentLineStyle lipgloss.Style

	// Pane pass-throughs. See pkg/pane.Options for defaults.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
	HScrollbar     bool

	// SpinnerStyle is applied to the spinner glyph rendered while the
	// tree is in its loading state (see SetLoading). Pass via theme.Tree()
	// for a sensible default.
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading.
	LoadingLabel string

	// Markable adds a mark gutter and binds x / X / A / D, so the
	// user can build a multi-selection the screen reads back with
	// Selection(). Off by default and free when off: the gutter takes no
	// columns. Unlike list and table, marking needs no keyed setter — a
	// node's path is already its identity. See mark.go.
	Markable bool

	// MarkStyle colors the ✓ on a marked row that is not under the cursor.
	MarkStyle lipgloss.Style

	// Filter configures the embedded filter. Ignored when Searchable=false.
	Filter filter.Options

	// Keys overrides the default keymap. Fields left zero fall back to
	// DefaultKeys(). theme.Tree() pre-populates this; mutate Keys.X
	// in-place to override individual actions.
	Keys Keys
}

// Keys is the tree's keymap. Each binding carries both its dispatch
// keys (WithKeys) and its help label (WithHelp). Update and Help() read
// from the same struct, so custom bindings propagate to the hint strip
// automatically. The embedded pane.Keys covers horizontal scroll.
type Keys struct {
	Up, Down                     key.Binding
	Top, Bottom                  key.Binding
	Toggle, Enter                key.Binding
	ExpandAll, CollapseAll       key.Binding
	NextSibling, PrevSibling     key.Binding
	NextLeaf, PrevLeaf           key.Binding
	Search, NextMatch, PrevMatch key.Binding
	Filter                       key.Binding
	Mark, MarkRange              key.Binding
	MarkAll, ClearMarks          key.Binding
	Pane                         pane.Keys
}

// DefaultKeys returns the tree's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:         key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:      key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Toggle:      key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "select")),
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
		Mark:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "mark")),
		MarkRange:   key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "mark to here")),
		MarkAll:     key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "mark all")),
		ClearMarks:  key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "drop marks")),
		Pane:        pane.DefaultKeys(),
	}
}

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
	if len(k.Toggle.Keys()) == 0 {
		k.Toggle = d.Toggle
	}
	if len(k.Enter.Keys()) == 0 {
		k.Enter = d.Enter
	}
	if len(k.ExpandAll.Keys()) == 0 {
		k.ExpandAll = d.ExpandAll
	}
	if len(k.CollapseAll.Keys()) == 0 {
		k.CollapseAll = d.CollapseAll
	}
	if len(k.NextSibling.Keys()) == 0 {
		k.NextSibling = d.NextSibling
	}
	if len(k.PrevSibling.Keys()) == 0 {
		k.PrevSibling = d.PrevSibling
	}
	if len(k.NextLeaf.Keys()) == 0 {
		k.NextLeaf = d.NextLeaf
	}
	if len(k.PrevLeaf.Keys()) == 0 {
		k.PrevLeaf = d.PrevLeaf
	}
	if len(k.Search.Keys()) == 0 {
		k.Search = d.Search
	}
	if len(k.NextMatch.Keys()) == 0 {
		k.NextMatch = d.NextMatch
	}
	if len(k.PrevMatch.Keys()) == 0 {
		k.PrevMatch = d.PrevMatch
	}
	if len(k.Filter.Keys()) == 0 {
		k.Filter = d.Filter
	}
	if len(k.Mark.Keys()) == 0 {
		k.Mark = d.Mark
	}
	if len(k.MarkRange.Keys()) == 0 {
		k.MarkRange = d.MarkRange
	}
	if len(k.MarkAll.Keys()) == 0 {
		k.MarkAll = d.MarkAll
	}
	if len(k.ClearMarks.Keys()) == 0 {
		k.ClearMarks = d.ClearMarks
	}
	k.Pane.FillDefaults()
}

// Model is the tree widget. Embed by value; mutate via the setters.
type Model struct {
	root     Node
	expanded map[string]bool
	rows     []row
	cursor   int

	body       pane.Pane
	filter     filter.Model
	searchable bool

	markable bool
	marks    map[string]bool
	// markAnchor is the path of the most recently marked row — the fixed
	// end of a MarkRange.
	markAnchor string
	markStyle  lipgloss.Style

	matchStyle       lipgloss.Style
	currentLineStyle lipgloss.Style

	keys Keys

	// token is this component's stable identity for focus requests. Update
	// takes a value receiver, so the model cannot name its own address.
	token focus.Token

	// filterRule{Active,Inactive} draw the line separating the inline filter
	// row from the content. The active one is used while the filter has
	// input — an inline filter has no border of its own to light up, so the
	// prompt and this rule carry that signal instead.
	filterRuleActive   lipgloss.Style
	filterRuleInactive lipgloss.Style

	query      string // lower-cased
	matchRows  []int  // indices into rows that contain a match
	matchIdx   int    // -1 when no current match
	filterMode bool   // when true and query != "", non-matching subtrees are hidden

	// Focus tracking for SelectedChangedMsg. focusInit flips true after
	// the first emit so an initially-empty tree doesn't send an Empty
	// message before any nodes exist. focusPath holds the last emitted
	// path for dedup; a nil slice with focusInit=true means the last
	// emit was Empty.
	focusInit    bool
	focusPath    []string
	focusPending bool
}

// row is one rendered line in the flattened view.
type row struct {
	node     Node
	depth    int
	path     string
	isLeaf   bool
	expanded bool
}

// New constructs a tree. Call Update/View from the parent model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "tree"
	}
	opts.Keys.fillDefaults()
	m := Model{
		token:            focus.NewToken(),
		root:             opts.Root,
		expanded:         map[string]bool{},
		searchable:       opts.Searchable,
		markable:         opts.Markable,
		markStyle:        opts.MarkStyle,
		matchStyle:       opts.MatchStyle,
		currentLineStyle: opts.CurrentLineStyle,
		keys:             opts.Keys,
		matchIdx:         -1,
	}

	bodyH := opts.Height
	if m.searchable {
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

	if opts.Root != nil {
		m.preExpand(opts.Root, rootPath(opts.Root), 0, opts.InitialDepth)
	}
	m.refresh()
	return m
}

// rootPath returns the identifier for the root node. Label-based so
// SetRoot can preserve state across a full-tree swap; unique within the
// tree because every other path is prefixed with root.Label() + "/".
func rootPath(n Node) string { return n.Label() }

// childPath composes a child path from its parent path, the child's label,
// and its 1-indexed occurrence count among earlier siblings sharing that
// label. First occurrence: parent/label; subsequent: parent/label:N.
// Making paths stable across sibling reordering by keying on Label is the
// point — an index-based scheme would invalidate every deeper path on any
// reorder. The occurrence suffix keeps paths unique when two siblings
// share a label.
func childPath(parent, label string, occurrence int) string {
	if occurrence <= 1 {
		return parent + "/" + label
	}
	return fmt.Sprintf("%s/%s:%d", parent, label, occurrence)
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles cursor movement, expand/collapse, search, and forwards
// everything else to the body pane (so pgup/pgdn/arrows/mouse-wheel and
// horizontal scroll keep working).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if mm, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(mm)
	}
	if m.searchable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		// enter commits and esc cancels, both of which blur the filter from
		// the inside; the body takes the highlight back when they do.
		if !m.filter.Focused() {
			m.body.SetFocused(true)
		}
		m.applyQuery()
		return m, tea.Batch(cmd, m.flushMsgs())
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.refresh()
			}
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.Down):
			if m.cursor < len(m.rows)-1 {
				m.cursor++
				m.refresh()
			}
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.Toggle):
			m.toggleCursor()
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.Top):
			m.cursor = 0
			m.refresh()
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.Bottom):
			m.cursor = max(0, len(m.rows)-1)
			m.refresh()
			return m, m.flushMsgs()
		case m.markable && key.Matches(k, m.keys.Mark):
			m.ToggleMark()
		case m.markable && key.Matches(k, m.keys.MarkRange):
			m.MarkRange()
		case m.markable && key.Matches(k, m.keys.MarkAll):
			m.ToggleMarkAll()
		case m.markable && key.Matches(k, m.keys.ClearMarks):
			m.ClearMarks()
		case key.Matches(k, m.keys.ExpandAll):
			m.expandAll()
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.CollapseAll):
			m.collapseAll()
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.NextSibling):
			m.jumpSibling(+1)
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.PrevSibling):
			m.jumpSibling(-1)
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.NextLeaf):
			m.jumpLeaf(+1)
			return m, m.flushMsgs()
		case key.Matches(k, m.keys.PrevLeaf):
			m.jumpLeaf(-1)
			return m, m.flushMsgs()
		case m.searchable && key.Matches(k, m.keys.Search):
			return m, tea.Batch(m.FocusFilter(), m.flushMsgs())
		case m.searchable && key.Matches(k, m.keys.NextMatch):
			m.jumpMatch(+1)
			return m, m.flushMsgs()
		case m.searchable && key.Matches(k, m.keys.PrevMatch):
			m.jumpMatch(-1)
			return m, m.flushMsgs()
		case m.searchable && key.Matches(k, m.keys.Filter):
			m.filterMode = !m.filterMode
			m.refresh()
			return m, m.flushMsgs()
		}
	}

	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, tea.Batch(cmd, m.flushMsgs())
}

// View stacks the filter (when searchable) above the body pane.
func (m Model) View() string { return m.body.View() }

// Selected returns the node under the cursor. ok is false when the visible
// row set is empty (e.g. filter mode with no matches).
func (m Model) Selected() (Node, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil, false
	}
	return m.rows[m.cursor].node, true
}

// Cursor returns the current row index.
func (m Model) Cursor() int { return m.cursor }

// SetCursor moves the cursor (clamped to the visible range) and scrolls
// to keep it on screen.
func (m *Model) SetCursor(n int) {
	m.cursor = max(0, min(n, len(m.rows)-1))
	m.refresh()
}

// Searching reports whether the embedded filter currently has focus.
func (m Model) Searching() bool { return m.searchable && m.filter.Focused() }

// IsCapturingKeys reports whether the tree currently swallows printable
// keys — true while its search filter is focused. Satisfies focus.Capturer.
func (m Model) IsCapturingKeys() bool { return m.Searching() }

// FilterMode reports whether non-matching subtrees are currently hidden.
func (m Model) FilterMode() bool { return m.filterMode }

// SetFilterMode turns filter-only rendering on or off.
func (m *Model) SetFilterMode(b bool) {
	if m.filterMode == b {
		return
	}
	m.filterMode = b
	m.refresh()
}

// Query returns the current search text.
func (m Model) Query() string {
	if m.searchable {
		return m.filter.Value()
	}
	return ""
}

// SetQuery sets the search text programmatically.
func (m *Model) SetQuery(s string) {
	if !m.searchable {
		return
	}
	m.filter.SetValue(s)
	m.applyQuery()
}

// SetRoot swaps the underlying tree in place, preserving as much user
// state as possible so periodic polling doesn't clobber the view under
// the user. Behavior:
//
//   - Expand/collapse state is keyed on label-based paths (see childPath).
//     Any surviving path stays expanded; new nodes appear collapsed
//     (unless a prior generation of the map had them expanded); paths
//     that disappeared are pruned from the expanded map.
//   - Cursor pins to the same node (by path) when it survives. When the
//     previous node is gone, cursor walks up its ancestors until a
//     surviving path is found; failing that, cursor snaps to 0.
//   - Nil root clears expand state and cursor.
//
// This is the auto-refresh primitive: fetch new tree, call SetRoot, done.
func (m *Model) SetRoot(n Node) {
	var prevPath string
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		prevPath = m.rows[m.cursor].path
	}
	m.root = n
	if n == nil {
		m.expanded = map[string]bool{}
		m.cursor = 0
		m.refresh()
		return
	}
	m.refresh()
	m.pruneExpanded()
	m.restoreCursor(prevPath)
	m.refresh()
}

// collectAllPaths walks the tree unconditionally (ignoring expand state)
// and populates out with every reachable path. Used to garbage-collect
// stale entries from m.expanded on SetRoot.
func (m *Model) collectAllPaths(n Node, path string, out map[string]bool) {
	out[path] = true
	seen := map[string]int{}
	for _, c := range n.Children() {
		label := c.Label()
		seen[label]++
		m.collectAllPaths(c, childPath(path, label, seen[label]), out)
	}
}

// pruneExpanded drops entries from m.expanded whose paths no longer
// resolve in the current tree, so a long-running Model that swaps roots
// many times doesn't leak memory into the map.
func (m *Model) pruneExpanded() {
	if m.root == nil {
		m.expanded = map[string]bool{}
		return
	}
	reachable := map[string]bool{}
	m.collectAllPaths(m.root, rootPath(m.root), reachable)
	for k := range m.expanded {
		if !reachable[k] {
			delete(m.expanded, k)
		}
	}
}

// restoreCursor tries to point the cursor at the row whose path matches
// prev, then walks ancestors until it finds a surviving row, then falls
// back to 0.
func (m *Model) restoreCursor(prev string) {
	if prev == "" || len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	idx := make(map[string]int, len(m.rows))
	for i, r := range m.rows {
		idx[r.path] = i
	}
	for {
		if i, ok := idx[prev]; ok {
			m.cursor = i
			return
		}
		slash := strings.LastIndex(prev, "/")
		if slash < 0 {
			m.cursor = 0
			return
		}
		prev = prev[:slash]
	}
}

// SetRect places the tree in the given rect. When searchable, the internal
// filter pane takes the top 3 rows and the body pane gets the rest, offset
// below it.
func (m *Model) SetRect(r geom.Rect) {
	m.body.SetRect(r)
	if m.searchable {
		// The filter is a row inside the body pane, not a pane beside it, so
		// it reads as belonging to what it filters. Placing the pane first
		// gives the header its width; setting the header then re-measures.
		m.placeInlineFilter(r)
		m.body.SetHeader(m.filterHeader())
		m.placeInlineFilter(r)
	}
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
	return m.filter.InlineView() + "\n" + rule.Render(strings.Repeat("─", inner))
}

// SetTitle sets the pane's top-left title.
func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

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
	if m.searchable {
		m.filter.Blur()
		m.body.SetHeader(m.filterHeader())
	}
}

// Focused reports whether either of the component's regions owns input.
func (m Model) Focused() bool {
	return m.body.Focused() || (m.searchable && m.filter.Focused())
}

// FocusFilter moves input to the filter and takes the highlight off the body,
// so exactly one region ever reads as active.
func (m *Model) FocusFilter() tea.Cmd {
	if !m.searchable {
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
	if !m.searchable {
		return
	}
	m.filter.Blur()
	m.body.SetFocused(true)
	m.body.SetHeader(m.filterHeader())
}

// SetActiveColor updates the body pane's active border color.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.body.SetActiveColor(c) }

// SetInactiveColor updates the body pane's inactive border color.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

// SetActiveBorder updates the border shape drawn while focused.
func (m *Model) SetActiveBorder(b lipgloss.Border) { m.body.SetActiveBorder(b) }

// SetInactiveBorder updates the border shape drawn while unfocused.
func (m *Model) SetInactiveBorder(b lipgloss.Border) { m.body.SetInactiveBorder(b) }

// SetMatchStyle updates the highlight style applied to matched substrings.
func (m *Model) SetMatchStyle(s lipgloss.Style) {
	m.matchStyle = s
	m.refresh()
}

// SetCurrentLineStyle updates the row style applied under the cursor.
func (m *Model) SetCurrentLineStyle(s lipgloss.Style) {
	m.currentLineStyle = s
	m.refresh()
}

// Loading reports whether the tree is currently in its loading state.
func (m Model) Loading() bool { return m.body.Loading() }

// SetLoading toggles the loading state. When entering, returns the
// spinner's initial Tick command — propagate it back from your screen's
// Update so the spinner animates.
func (m *Model) SetLoading(b bool) tea.Cmd { return m.body.SetLoading(b) }

// SetLoadingLabel updates the text rendered next to the spinner while loading.
func (m *Model) SetLoadingLabel(s string) { m.body.SetLoadingLabel(s) }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
func (m *Model) SetSpinnerStyle(s lipgloss.Style) { m.body.SetSpinnerStyle(s) }

// Help returns the keys this tree responds to. Each entry comes from
// m.keys (the same bindings Update dispatches against), so custom
// keymaps propagate to the hint strip automatically. While the embedded
// filter is focused, returns the filter's keys.
func (m Model) Help() []key.Binding {
	if m.searchable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		m.keys.Up, m.keys.Down,
		m.keys.Top, m.keys.Bottom,
		m.keys.Toggle,
		m.keys.ExpandAll, m.keys.CollapseAll,
		m.keys.NextSibling, m.keys.PrevSibling,
		m.keys.NextLeaf, m.keys.PrevLeaf,
		m.keys.Enter,
	}
	if m.markable {
		out = append(out, m.keys.Mark, m.keys.MarkRange, m.keys.MarkAll, m.keys.ClearMarks,
			key.NewBinding(key.WithKeys("mouse:mark"), key.WithHelp("click ✓", "mark")))
	}
	out = append(out, m.body.HelpBindings()...)
	if m.searchable {
		out = append(out, m.keys.Search)
		if m.query != "" {
			out = append(out, m.keys.NextMatch, m.keys.PrevMatch)
			// Filter-mode binding's help label flips with state.
			label := "filter"
			if m.filterMode {
				label = "show all"
			}
			out = append(out, key.NewBinding(
				key.WithKeys(m.keys.Filter.Keys()...),
				key.WithHelp(`\`, label),
			))
		}
	}
	return out
}

// ---- internals -----------------------------------------------------------

func (m *Model) preExpand(n Node, path string, depth, want int) {
	if depth >= want {
		return
	}
	kids := n.Children()
	if len(kids) == 0 {
		return
	}
	m.expanded[path] = true
	seen := map[string]int{}
	for _, c := range kids {
		label := c.Label()
		seen[label]++
		m.preExpand(c, childPath(path, label, seen[label]), depth+1, want)
	}
}

// expandAll marks every non-leaf node in the tree as expanded. Walks from
// the root using the same path scheme as preExpand so paths stay stable
// across collapse/expand cycles.
func (m *Model) expandAll() {
	if m.root == nil {
		return
	}
	m.markExpanded(m.root, rootPath(m.root))
	m.refresh()
}

func (m *Model) markExpanded(n Node, path string) {
	kids := n.Children()
	if len(kids) == 0 {
		return
	}
	m.expanded[path] = true
	seen := map[string]int{}
	for _, c := range kids {
		label := c.Label()
		seen[label]++
		m.markExpanded(c, childPath(path, label, seen[label]))
	}
}

// collapseAll clears every expand mark, leaving only the root visible.
// Cursor snaps to 0 since deeper rows are no longer in the visible set.
func (m *Model) collapseAll() {
	if len(m.expanded) == 0 {
		m.cursor = 0
		m.refresh()
		return
	}
	m.expanded = map[string]bool{}
	m.cursor = 0
	m.refresh()
}

func (m *Model) toggleCursor() {
	if m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.isLeaf {
		return
	}
	if r.expanded {
		delete(m.expanded, r.path)
	} else {
		m.expanded[r.path] = true
	}
	m.refresh()
}

func (m *Model) applyQuery() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == m.query {
		m.refreshStatus()
		return
	}
	m.query = q
	m.refresh()
	if len(m.matchRows) > 0 {
		m.matchIdx = 0
		m.cursor = m.matchRows[0]
		m.body.EnsureVisible(m.cursor)
	} else {
		m.matchIdx = -1
	}
	m.refresh()
}

func (m *Model) jumpMatch(step int) {
	if len(m.matchRows) == 0 {
		return
	}
	if m.matchIdx < 0 {
		m.matchIdx = 0
	} else {
		m.matchIdx = (m.matchIdx + step + len(m.matchRows)) % len(m.matchRows)
	}
	m.cursor = m.matchRows[m.matchIdx]
	m.refresh()
}

// jumpSibling moves the cursor to the next (step=+1) or previous
// (step=-1) visible row at the same depth as the current cursor row.
// Cousins under sibling parents count as same-depth neighbors — this is
// the same-level skip-past-children semantics, not strict same-parent.
// No-op when no such row exists on the requested side.
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
// visible leaf row (one with no children). A collapsed parent is not a
// leaf — its hidden subtree still has content, so jumpLeaf skips past
// it. No-op when no such row exists on the requested side.
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

func (m *Model) refresh() {
	m.rows = m.rows[:0]
	if m.root != nil {
		matchSet := map[string]bool{}
		if m.filterMode && m.query != "" {
			m.collectMatchPaths(m.root, rootPath(m.root), matchSet)
		}
		m.flatten(m.root, rootPath(m.root), 0, matchSet)
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}

	m.matchRows = m.matchRows[:0]
	if m.query != "" {
		for i, r := range m.rows {
			if strings.Contains(strings.ToLower(r.node.Label()), m.query) {
				m.matchRows = append(m.matchRows, i)
			}
		}
		if m.matchIdx >= len(m.matchRows) {
			if len(m.matchRows) == 0 {
				m.matchIdx = -1
			} else {
				m.matchIdx = len(m.matchRows) - 1
			}
		}
	} else {
		m.matchIdx = -1
	}

	if m.searchable {
		m.body.SetHeader(m.filterHeader())
	}
	m.body.SetContent(m.renderContent())
	m.body.EnsureVisible(m.cursor)
	m.refreshStatus()
	m.noteFocus()
}

// noteFocus samples the current focused node's path and, if it differs
// from the last-emitted path, marks a SelectedChangedMsg for the next
// flush. Empty rows are only reported once we've previously emitted a
// non-empty focus — an initially-empty tree skips the msg so parents
// don't get an Empty ping before any nodes exist.
func (m *Model) noteFocus() {
	empty := len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows)
	if empty {
		if !m.focusInit || m.focusPath == nil {
			return
		}
		m.focusPath = nil
		m.focusPending = true
		return
	}
	path := splitPath(m.rows[m.cursor].path)
	if m.focusInit && stringSliceEqual(m.focusPath, path) {
		return
	}
	m.focusPath = path
	m.focusPending = true
}

// flushMsgs returns a tea.Cmd carrying the pending SelectedChangedMsg
// (or nil when nothing has changed) and clears the pending flag.
func (m *Model) flushMsgs() tea.Cmd {
	if !m.focusPending {
		return nil
	}
	m.focusPending = false
	m.focusInit = true
	if m.focusPath == nil {
		return func() tea.Msg { return SelectedChangedMsg{Empty: true} }
	}
	path := append([]string(nil), m.focusPath...)
	label := path[len(path)-1]
	depth := len(path) - 1
	return func() tea.Msg {
		return SelectedChangedMsg{Path: path, Label: label, Depth: depth}
	}
}

// splitPath breaks the "/"-separated internal path string into segments,
// each containing the label (or "label:N" for duplicate siblings).
func splitPath(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func stringSliceEqual(a, b []string) bool {
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

func (m *Model) collectMatchPaths(n Node, path string, out map[string]bool) bool {
	hit := strings.Contains(strings.ToLower(n.Label()), m.query)
	seen := map[string]int{}
	for _, c := range n.Children() {
		label := c.Label()
		seen[label]++
		if m.collectMatchPaths(c, childPath(path, label, seen[label]), out) {
			hit = true
		}
	}
	if hit {
		out[path] = true
	}
	return hit
}

func (m *Model) flatten(n Node, path string, depth int, matchSet map[string]bool) {
	kids := n.Children()
	isLeaf := len(kids) == 0
	exp := !isLeaf && m.expanded[path]
	// In filter mode, force-expand any ancestor of a match so its match is
	// reachable. Leaves that aren't in the match set are skipped entirely.
	if m.filterMode && m.query != "" {
		if !matchSet[path] {
			return
		}
		if !isLeaf {
			exp = true
		}
	}
	m.rows = append(m.rows, row{
		node:     n,
		depth:    depth,
		path:     path,
		isLeaf:   isLeaf,
		expanded: exp,
	})
	if !exp {
		return
	}
	seen := map[string]int{}
	for _, c := range kids {
		label := c.Label()
		seen[label]++
		m.flatten(c, childPath(path, label, seen[label]), depth+1, matchSet)
	}
}

func (m *Model) renderContent() string {
	if len(m.rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range m.rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.formatRow(r, i == m.cursor))
	}
	return b.String()
}

// FocusToken returns the tree's stable focus identity. See focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// handleMouse routes a mouse event that may or may not belong to this tree.
//
// Clicking the ▸/▾ glyph expands or collapses that node directly, which is
// the one place a single click does more than select — the glyph is drawn
// precisely to say "this opens". Anywhere else on a row selects it, and a
// double click toggles, matching what space does from the keyboard. Rule 23
// keeps ←/→ and h/l out of it: they scroll, here as everywhere.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if target, ok := m.body.HandleScrollbar(e); ok {
		m.scrollTo(target)
		return m, m.flushMsgs()
	}
	if m.searchable && m.filter.Rect().Hit(e.X, e.Y) {
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
		if _, ok := m.body.RowAt(e.X, e.Y); !ok {
			return m, nil
		}
		m.moveCursor(-1)
		return m, m.flushMsgs()

	case e.IsWheelDown():
		if _, ok := m.body.RowAt(e.X, e.Y); !ok {
			return m, nil
		}
		m.moveCursor(1)
		return m, m.flushMsgs()

	case e.IsPointPress():
		if !m.body.Rect().Hit(e.X, e.Y) {
			return m, nil
		}
		// A press anywhere in the pane claims focus, including blank space
		// below the last node. Landing on a row additionally moves the
		// cursor — but "this pane is now active" must not depend on hitting
		// a row, or clicking the empty half of a short tree does nothing.
		m.body.SetFocused(true)
		if row, ok := m.body.RowAt(e.X, e.Y); ok && row < len(m.rows) {
			m.cursor = row
			// The ✓ gutter toggles on a single click, like the ▸ glyph.
			if e.IsPress() && m.markable && m.onMarkColumn(e.X) {
				m.toggleMarkAt(row)
				return m, tea.Batch(focus.RequestSelf(m.token), m.flushMsgs())
			}
			// Chrome that acts stays left-only: a right press is asking a
			// question about the row, not expanding it.
			if (e.IsPress() && m.onGlyph(e.X, m.rows[row])) || e.IsDoubleClick() {
				m.toggleCursor()
			} else {
				m.refresh()
			}
		}
		return m, tea.Batch(focus.RequestSelf(m.token), m.flushMsgs())
	}
	return m, nil
}

// onGlyph reports whether x falls on r's expand/collapse glyph. The glyph
// sits at column 2*depth (two cells of indent per level) and is one cell
// wide; leaves draw blank there and are never a target.
func (m Model) onGlyph(x int, r row) bool {
	if r.isLeaf {
		return false
	}
	c := m.body.ContentRect()
	pos := (x - c.X) + m.body.XOffset()
	return pos == m.gutterW()+2*r.depth
}

// scrollTo puts row at the top of the view and moves the cursor onto it.
//
// Moving the cursor is what makes the scroll stick. refresh re-asserts "the
// cursor is visible" on every frame, and layout.Sized calls SetRect — hence
// refresh — on every render, so a viewport moved on its own is undone one
// frame later and the view snaps back to wherever the cursor still was.
func (m *Model) scrollTo(row int) {
	n := len(m.rows)
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

// moveCursor steps the cursor by delta, clamped to the visible rows.
func (m *Model) moveCursor(delta int) {
	next := m.cursor + delta
	if next < 0 || next >= len(m.rows) {
		return
	}
	m.cursor = next
	m.refresh()
}

func (m *Model) formatRow(r row, current bool) string {
	indent := strings.Repeat("  ", r.depth)
	glyph := "  "
	switch {
	case r.isLeaf:
		glyph = "  "
	case r.expanded:
		glyph = "▾ "
	default:
		glyph = "▸ "
	}

	label := r.node.Label()
	// The gutter is leftmost, before the indent: a ✓ that indented with its
	// row would not read as a column.
	gutter := m.gutterForRow(r)
	if !current {
		if m.markable && m.marks[r.path] {
			return m.markStyle.Render(markGlyph) + " " + indent + glyph + m.highlightLabel(label)
		}
		return gutter + indent + glyph + m.highlightLabel(label)
	}

	base := m.currentLineStyle
	matchOnRow := m.matchStyle.Inherit(base)
	var b strings.Builder
	b.WriteString(renderPreserving(base, gutter+indent+glyph))
	b.WriteString(m.renderHighlightedSegments(label, base, matchOnRow))

	inner := max(0, m.body.Width()-2-pane.ScrollbarWidth)
	if pad := inner - lipgloss.Width(b.String()); pad > 0 {
		b.WriteString(renderPreserving(base, strings.Repeat(" ", pad)))
	}
	return b.String()
}

func (m *Model) renderHighlightedSegments(label string, base, matchOnRow lipgloss.Style) string {
	if m.query == "" {
		return renderPreserving(base, label)
	}
	spans := matchSpans(label, m.query)
	if len(spans) == 0 {
		return renderPreserving(base, label)
	}
	var b strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp[0] > cursor {
			b.WriteString(renderPreserving(base, label[cursor:sp[0]]))
		}
		b.WriteString(renderPreserving(matchOnRow, label[sp[0]:sp[1]]))
		cursor = sp[1]
	}
	if cursor < len(label) {
		b.WriteString(renderPreserving(base, label[cursor:]))
	}
	return b.String()
}

// renderPreserving wraps text with style s, like s.Render, but re-emits
// the open SGR after every embedded `\x1b[0m` reset inside text. This
// keeps the outer background (or any other styling) intact when the
// inner text includes ANSI-styled segments — e.g. a Node label that
// returns lipgloss-colored status icons. Without this, the inner reset
// would clobber the row-level highlight bg from the colored point on.
func renderPreserving(s lipgloss.Style, text string) string {
	if !strings.ContainsRune(text, '\x1b') {
		return s.Render(text)
	}
	open, close := styleSGR(s)
	if open == "" {
		return s.Render(text)
	}
	fixed := strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+open)
	fixed = strings.ReplaceAll(fixed, "\x1b[m", "\x1b[m"+open)
	return open + fixed + close
}

// styleSGR returns the open and close SGR sequences a style emits around
// its content. Works by rendering a unique probe and slicing on either
// side. Returns "", "" for styles that produce no SGR codes.
func styleSGR(s lipgloss.Style) (open, close string) {
	const probe = "\x00\x00tuilib-tree-probe\x00\x00"
	r := s.Render(probe)
	i := strings.Index(r, probe)
	if i < 0 {
		return "", ""
	}
	return r[:i], r[i+len(probe):]
}

func (m *Model) highlightLabel(label string) string {
	if m.query == "" {
		return label
	}
	spans := matchSpans(label, m.query)
	if len(spans) == 0 {
		return label
	}
	var b strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp[0] > cursor {
			b.WriteString(label[cursor:sp[0]])
		}
		b.WriteString(m.matchStyle.Render(label[sp[0]:sp[1]]))
		cursor = sp[1]
	}
	if cursor < len(label) {
		b.WriteString(label[cursor:])
	}
	return b.String()
}

func matchSpans(label, query string) [][2]int {
	if query == "" {
		return nil
	}
	lower := strings.ToLower(label)
	var out [][2]int
	off := 0
	for {
		idx := strings.Index(lower[off:], query)
		if idx < 0 {
			break
		}
		out = append(out, [2]int{off + idx, off + idx + len(query)})
		off += idx + len(query)
	}
	return out
}

func (m *Model) refreshStatus() {
	if m.query == "" {
		m.body.SetBottomLeft("")
		return
	}
	cur := m.matchIdx + 1
	if cur < 0 {
		cur = 0
	}
	state := fmt.Sprintf("%d/%d", cur, len(m.matchRows))
	if m.filterMode {
		state += " · filter"
	}
	m.body.SetBottomLeft(state)
}
