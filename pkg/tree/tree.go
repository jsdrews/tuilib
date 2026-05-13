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
//   - Right/l expand or descend into the cursor's children; left/h collapse
//     or jump to the parent; space toggles the cursor; enter selects;
//     E expands every node in the tree, C collapses everything back to root
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
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Node is the minimal contract a caller's data must satisfy. Label is the
// row's display text; Children returns the immediate descendants (nil or
// empty for leaves). Tree never mutates the returned slice.
type Node interface {
	Label() string
	Children() []Node
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

	// Filter configures the embedded filter. Ignored when Searchable=false.
	Filter filter.Options
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

	matchStyle       lipgloss.Style
	currentLineStyle lipgloss.Style

	query      string // lower-cased
	matchRows  []int  // indices into rows that contain a match
	matchIdx   int    // -1 when no current match
	filterMode bool   // when true and query != "", non-matching subtrees are hidden
}

// row is one rendered line in the flattened view.
type row struct {
	node     Node
	depth    int
	path     string
	isLeaf   bool
	expanded bool
}

var keys = struct {
	Up, Down, Expand, Collapse, Toggle, Enter             key.Binding
	Search, NextMatch, PrevMatch, Filter, Top, End        key.Binding
	ExpandAll, CollapseAll                                key.Binding
}{
	Up:          key.NewBinding(key.WithKeys("up", "k")),
	Down:        key.NewBinding(key.WithKeys("down", "j")),
	Expand:      key.NewBinding(key.WithKeys("right", "l")),
	Collapse:    key.NewBinding(key.WithKeys("left", "h")),
	Toggle:      key.NewBinding(key.WithKeys(" ")),
	Enter:       key.NewBinding(key.WithKeys("enter")),
	Search:      key.NewBinding(key.WithKeys("/")),
	NextMatch:   key.NewBinding(key.WithKeys("n")),
	PrevMatch:   key.NewBinding(key.WithKeys("N")),
	Filter:      key.NewBinding(key.WithKeys("\\")),
	Top:         key.NewBinding(key.WithKeys("g")),
	End:         key.NewBinding(key.WithKeys("G")),
	ExpandAll:   key.NewBinding(key.WithKeys("E")),
	CollapseAll: key.NewBinding(key.WithKeys("C")),
}

// New constructs a tree. Call Update/View from the parent model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "tree"
	}
	m := Model{
		root:             opts.Root,
		expanded:         map[string]bool{},
		searchable:       opts.Searchable,
		matchStyle:       opts.MatchStyle,
		currentLineStyle: opts.CurrentLineStyle,
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
	})

	if opts.Root != nil {
		m.preExpand(opts.Root, "0", 0, opts.InitialDepth)
	}
	m.refresh()
	return m
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update handles cursor movement, expand/collapse, search, and forwards
// everything else to the body pane (so pgup/pgdn/arrows/mouse-wheel and
// horizontal scroll keep working).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.searchable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyQuery()
		return m, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.refresh()
			}
			return m, nil
		case key.Matches(k, keys.Down):
			if m.cursor < len(m.rows)-1 {
				m.cursor++
				m.refresh()
			}
			return m, nil
		case key.Matches(k, keys.Expand):
			m.expandOrDescend()
			return m, nil
		case key.Matches(k, keys.Collapse):
			m.collapseOrAscend()
			return m, nil
		case key.Matches(k, keys.Toggle):
			m.toggleCursor()
			return m, nil
		case key.Matches(k, keys.Top):
			m.cursor = 0
			m.refresh()
			return m, nil
		case key.Matches(k, keys.End):
			m.cursor = max(0, len(m.rows)-1)
			m.refresh()
			return m, nil
		case key.Matches(k, keys.ExpandAll):
			m.expandAll()
			return m, nil
		case key.Matches(k, keys.CollapseAll):
			m.collapseAll()
			return m, nil
		case m.searchable && key.Matches(k, keys.Search):
			return m, m.filter.Focus()
		case m.searchable && key.Matches(k, keys.NextMatch):
			m.jumpMatch(+1)
			return m, nil
		case m.searchable && key.Matches(k, keys.PrevMatch):
			m.jumpMatch(-1)
			return m, nil
		case m.searchable && key.Matches(k, keys.Filter):
			m.filterMode = !m.filterMode
			m.refresh()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

// View stacks the filter (when searchable) above the body pane.
func (m Model) View() string {
	if m.searchable {
		return m.filter.View() + "\n" + m.body.View()
	}
	return m.body.View()
}

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

// SetRoot replaces the underlying tree, resets the expand state to
// InitialDepth=1 (root expanded), and rebuilds.
func (m *Model) SetRoot(n Node) {
	m.root = n
	m.expanded = map[string]bool{}
	if n != nil {
		m.preExpand(n, "0", 0, 1)
	}
	m.cursor = 0
	m.refresh()
}

// SetDimensions resizes the tree in place.
func (m *Model) SetDimensions(w, h int) {
	bodyH := h
	if m.searchable {
		m.filter.SetWidth(w)
		bodyH = max(0, h-3)
	}
	m.body.SetDimensions(w, bodyH)
	m.refresh()
}

// SetTitle sets the pane's top-left title.
func (m *Model) SetTitle(s string) { m.body.SetTitle(s) }

// SetFocused flips the body pane's focus state.
func (m *Model) SetFocused(b bool) { m.body.SetFocused(b) }

// SetActiveColor updates the body pane's active border color.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.body.SetActiveColor(c) }

// SetInactiveColor updates the body pane's inactive border color.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

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

// Help returns the keys this tree responds to. While the embedded filter
// is focused, returns the filter's keys.
func (m Model) Help() []key.Binding {
	if m.searchable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
		key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→", "expand")),
		key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "collapse")),
		key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		key.NewBinding(key.WithKeys("E", "C"), key.WithHelp("E/C", "expand/collapse all")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "select")),
	}
	if m.searchable {
		out = append(out, key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")))
		if m.query != "" {
			out = append(out,
				key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
				key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
			)
			label := "filter"
			if m.filterMode {
				label = "show all"
			}
			out = append(out, key.NewBinding(key.WithKeys("\\"), key.WithHelp(`\`, label)))
		}
	}
	return out
}

// ---- internals -----------------------------------------------------------

func (m *Model) preExpand(n Node, path string, depth, want int) {
	if depth >= want {
		return
	}
	if len(n.Children()) == 0 {
		return
	}
	m.expanded[path] = true
	for i, c := range n.Children() {
		m.preExpand(c, fmt.Sprintf("%s/%d", path, i), depth+1, want)
	}
}

func (m *Model) expandOrDescend() {
	if m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.isLeaf {
		return
	}
	if !r.expanded {
		m.expanded[r.path] = true
		m.refresh()
		return
	}
	// Already expanded — step into the first child.
	if m.cursor+1 < len(m.rows) && m.rows[m.cursor+1].depth > r.depth {
		m.cursor++
		m.refresh()
	}
}

func (m *Model) collapseOrAscend() {
	if m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if !r.isLeaf && r.expanded {
		delete(m.expanded, r.path)
		m.refresh()
		return
	}
	// Jump to parent: walk back to first row with smaller depth.
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].depth < r.depth {
			m.cursor = i
			m.refresh()
			return
		}
	}
}

// expandAll marks every non-leaf node in the tree as expanded. Walks from
// the root using the same path scheme as preExpand so paths stay stable
// across collapse/expand cycles.
func (m *Model) expandAll() {
	if m.root == nil {
		return
	}
	m.markExpanded(m.root, "0")
	m.refresh()
}

func (m *Model) markExpanded(n Node, path string) {
	children := n.Children()
	if len(children) == 0 {
		return
	}
	m.expanded[path] = true
	for i, c := range children {
		m.markExpanded(c, fmt.Sprintf("%s/%d", path, i))
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

func (m *Model) refresh() {
	m.rows = m.rows[:0]
	if m.root != nil {
		matchSet := map[string]bool{}
		if m.filterMode && m.query != "" {
			m.collectMatchPaths(m.root, "0", matchSet)
		}
		m.flatten(m.root, "0", 0, matchSet)
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

	m.body.SetContent(m.renderContent())
	m.body.EnsureVisible(m.cursor)
	m.refreshStatus()
}

func (m *Model) collectMatchPaths(n Node, path string, out map[string]bool) bool {
	hit := strings.Contains(strings.ToLower(n.Label()), m.query)
	for i, c := range n.Children() {
		childPath := fmt.Sprintf("%s/%d", path, i)
		if m.collectMatchPaths(c, childPath, out) {
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
	for i, c := range kids {
		m.flatten(c, fmt.Sprintf("%s/%d", path, i), depth+1, matchSet)
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
	if !current {
		return indent + glyph + m.highlightLabel(label)
	}

	base := m.currentLineStyle
	matchOnRow := m.matchStyle.Inherit(base)
	var b strings.Builder
	b.WriteString(renderPreserving(base, indent+glyph))
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

