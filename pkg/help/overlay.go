package help

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Section is a named group of bindings. Grouping is the whole reason the
// overlay exists: a screen composed of an app shell, a focus.Group and
// three components can easily reach thirty bindings, and thirty bindings
// in one flat list is a wall of text whether it is drawn in a footer or a
// modal. Sections say where each key comes from — "Global", "Results",
// "Filter" — which is the question a user reading the list actually has.
type Section struct {
	Title    string
	Bindings []key.Binding
}

// Sectioned is the interface a component, a composite (focus.Group) or a
// screen implements to describe its bindings as groups rather than as one
// flat list. Anything that doesn't implement it still works: the host wraps
// its Help() in a single unnamed Section.
type Sectioned interface {
	HelpSections() []Section
}

// The section vocabulary. Groups are named by what the keys *do*, not by
// what holds them — a heading naming the owner ends up over every binding
// that owner has, which is how "Multi-select" came to sit above a table's
// scroll keys. Owners are a qualifier instead (see Qualify), applied only
// when more than one of them is on screen.
//
// Components share these names so a group means the same thing wherever it
// appears: "Navigate" is cursor movement in a list, a table and a tree, and
// a user who has read it once has read it everywhere.
const (
	// SectionNavigate is cursor or line movement along the vertical axis.
	SectionNavigate = "Navigate"
	// SectionScroll is the horizontal axis, which is the pane's (rule 25).
	SectionScroll = "Scroll"
	// SectionFilter narrows what is displayed; SectionSearch finds within
	// it. Components that do one call it that; logview and tree do both
	// and keep the distinction.
	SectionFilter = "Filter"
	SectionSearch = "Search"
	// SectionSelect is marking (rule 32) — the keys and the click that
	// build a selection.
	SectionSelect = "Select"
	// SectionSort is column sorting.
	SectionSort = "Sort"
	// SectionExpand is opening and closing branches in a tree or inspector.
	SectionExpand = "Expand"
	// SectionView changes how content is rendered rather than which part of
	// it is on screen — wrap, follow.
	SectionView = "View"
	// SectionEdit is typing into a field or flipping a toggle.
	SectionEdit = "Edit"
	// SectionSubmit is committing or abandoning — a form, a confirm modal.
	SectionSubmit = "Submit"
	// SectionTabs is switching between tabbed bodies.
	SectionTabs = "Tabs"
)

// Group builds a section list, dropping any group whose bindings are all
// empty. Components build their sections with it so an unconfigured
// feature — a list that isn't markable, a table with no sortable column —
// contributes no heading rather than an empty one.
func Group(title string, bindings ...key.Binding) Section {
	return Section{Title: title, Bindings: bindings}
}

// Sections assembles a component's groups, discarding the empty ones.
func Sections(secs ...Section) []Section {
	out := make([]Section, 0, len(secs))
	for _, s := range secs {
		if len(s.Bindings) == 0 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// SectionsOf collects sections from a mixed list of parts, in order. A part
// may be a Sectioned (contributes its own groups), anything with a
// Help() []key.Binding (contributes one unnamed group), a Section, or a
// []Section. Anything else is skipped.
//
// This is how a screen composes: the same shape as building Help() from its
// components, with the grouping kept.
//
//	func (s *Screen) HelpSections() []help.Section {
//	    return help.SectionsOf(&s.table, help.Group("Deployments", s.verbs()...))
//	}
func SectionsOf(parts ...any) []Section {
	var out []Section
	for _, p := range parts {
		switch v := p.(type) {
		case Sectioned:
			out = append(out, v.HelpSections()...)
		case Section:
			out = append(out, v)
		case []Section:
			out = append(out, v...)
		case interface{ Help() []key.Binding }:
			out = append(out, Section{Bindings: v.Help()})
		}
	}
	return Sections(out...)
}

// OwnerWidth is the widest an owner prefix may be before it is truncated.
// A qualifier repeats on every one of that owner's headings, so it has to
// stay short enough not to become the widest thing in the column.
const OwnerWidth = 20

// Qualify prefixes each section title with an owner — "files · Navigate".
//
// Only worth doing when more than one owner is on screen: with a single
// component the owner is not in question, and repeating it on every heading
// is noise. focus.Group applies this rule for the screens it holds.
//
// The owner is normalized first, because the natural source for it is a
// pane's title and a pane title is often "name · hint" ("files · / to
// filter"). The hint is an affordance for the pane, not part of its name,
// and carrying it into every heading is what turns a qualifier into a
// paragraph — so everything from the first "·" on is dropped, and what
// survives is truncated to OwnerWidth.
func Qualify(owner string, secs []Section) []Section {
	owner = ownerName(owner)
	if owner == "" {
		return secs
	}
	out := make([]Section, len(secs))
	for i, s := range secs {
		out[i] = s
		if s.Title == "" {
			out[i].Title = owner
			continue
		}
		out[i].Title = owner + " · " + s.Title
	}
	return out
}

// ownerName reduces a pane title to the name a heading can carry.
func ownerName(s string) string {
	if i := strings.Index(s, "·"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	return ansi.Truncate(s, OwnerWidth, "…")
}

// Flatten concatenates every section's bindings back into one list.
// Components implement HelpSections and derive Help from it, so the
// grouping and the flat list can never disagree.
func Flatten(secs []Section) []key.Binding {
	var out []key.Binding
	for _, s := range secs {
		out = append(out, s.Bindings...)
	}
	return out
}

// ClosedMsg is emitted when the overlay asks to be dismissed — its Close
// key, or a press outside its bounds. The host clears its own "overlay is
// up" flag on receipt; the overlay holds no visibility state of its own,
// matching pkg/confirm and pkg/alert.
type ClosedMsg struct{}

func closed() tea.Cmd { return func() tea.Msg { return ClosedMsg{} } }

// Sizing caps. The overlay is a document you read, not a picker that must
// leave its subject visible, so it takes more of the screen than the
// action menu does — capped so the body it covers is still framed by it.
const (
	overlayMinWidth   = 34
	overlayWidthNum   = 4
	overlayWidthDen   = 5
	overlayMinHeight  = 5
	overlayHeightNum  = 4
	overlayHeightDen  = 5
	overlayIndent     = 2 // leading spaces on a binding row
	overlayKeyDescGap = 2 // between the key column and the description
)

// OverlayKeys is the overlay's keymap. Vertical scroll comes from the
// embedded pane and its viewport (rule 25); the overlay itself binds only
// search, the top/bottom jumps and close.
type OverlayKeys struct {
	Search      key.Binding
	Top, Bottom key.Binding
	Close       key.Binding
	Pane        pane.Keys
}

// DefaultOverlayKeys returns the overlay's stock keymap.
func DefaultOverlayKeys() OverlayKeys {
	return OverlayKeys{
		Search: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Top:    key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Close:  key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "close")),
		Pane:   pane.DefaultKeys(),
	}
}

// FillDefaults fills any zero-valued binding with its DefaultOverlayKeys
// counterpart, so partial overrides work without restating every field.
func (k *OverlayKeys) FillDefaults() {
	d := DefaultOverlayKeys()
	for _, p := range []struct {
		dst *key.Binding
		src key.Binding
	}{
		{&k.Search, d.Search}, {&k.Top, d.Top},
		{&k.Bottom, d.Bottom}, {&k.Close, d.Close},
	} {
		if len(p.dst.Keys()) == 0 {
			*p.dst = p.src
		}
	}
	k.Pane.FillDefaults()
}

// OverlayOptions configures the modal. Zero-value fields fall back to
// defaults; start from theme.HelpOverlay() to fill in the color tokens.
type OverlayOptions struct {
	// Title sits on the pane's top-left border slot. Defaults to "keys".
	Title string
	// Searchable embeds a filter on the pane's first inner row. Typing
	// reduces the list to matching bindings — on a thirty-binding screen
	// "mark" is a faster route to the four marking keys than reading.
	Searchable bool

	// KeyStyle is applied to the key column, DescStyle to the
	// description, SectionStyle to each section heading.
	KeyStyle     lipgloss.Style
	DescStyle    lipgloss.Style
	SectionStyle lipgloss.Style
	// EmptyStyle is applied to the "no matching keys" line.
	EmptyStyle lipgloss.Style

	// Pane pass-throughs. See pkg/pane.Options for defaults.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	Glyphs         glyph.Set
	SlotBrackets   pane.SlotBracketStyle

	// FilterRule styles the line separating the inline filter row from the
	// content — an inline filter has no border of its own to light up, so
	// the rule carries the "input goes here" signal (rule 27).
	FilterRuleActive   lipgloss.Style
	FilterRuleInactive lipgloss.Style

	// Filter configures the embedded filter. Ignored when Searchable=false.
	Filter filter.Options

	// Keys is the overlay's keymap. Leave zero to use DefaultOverlayKeys.
	Keys OverlayKeys
}

// Overlay is the expanded key-binding reference: a bordered, scrollable,
// optionally searchable modal listing every binding the active screen
// exposes, grouped into sections.
//
// It sizes itself to its content and centers inside whatever rect it is
// given, so a host composes it with layout.Sized alone — no layout.Center
// wrapper, the same shape pkg/alert uses in autosize mode:
//
//	layout.ZStack(base, layout.Sized(&s.keys))
//
// Under pkg/app none of that is the screen's problem: the shell owns the
// overlay, opens it on HelpKey, and routes every key and mouse event to it
// while it is up.
type Overlay struct {
	glyphs glyph.Set

	sections []Section
	rect     geom.Rect

	body       pane.Pane
	filter     filter.Model
	searchable bool
	query      string

	keyStyle     lipgloss.Style
	descStyle    lipgloss.Style
	sectionStyle lipgloss.Style
	emptyStyle   lipgloss.Style

	filterRuleActive   lipgloss.Style
	filterRuleInactive lipgloss.Style

	keys OverlayKeys
}

// NewOverlay constructs the modal. Call SetSections to give it content and
// SetRect to place it.
func NewOverlay(opts OverlayOptions) Overlay {
	if opts.Title == "" {
		opts.Title = "keys"
	}
	opts.Keys.FillDefaults()

	o := Overlay{
		glyphs:             opts.Glyphs.Resolve(),
		searchable:         opts.Searchable,
		keyStyle:           opts.KeyStyle,
		descStyle:          opts.DescStyle,
		sectionStyle:       opts.SectionStyle,
		emptyStyle:         opts.EmptyStyle,
		filterRuleActive:   opts.FilterRuleActive,
		filterRuleInactive: opts.FilterRuleInactive,
		keys:               opts.Keys,
	}
	if o.searchable {
		o.filter = filter.New(opts.Filter)
	}
	o.body = pane.New(pane.Options{
		Title:          opts.Title,
		Focused:        true,
		ActiveColor:    opts.ActiveColor,
		InactiveColor:  opts.InactiveColor,
		ActiveBorder:   opts.ActiveBorder,
		InactiveBorder: opts.InactiveBorder,
		Glyphs:         opts.Glyphs,
		SlotBrackets:   opts.SlotBrackets,
		Keys:           opts.Keys.Pane,
	})
	return o
}

// SetSections replaces the binding groups and resets scroll to the top.
// Empty sections and bindings with no help text are dropped, and a key
// already listed in an earlier section is not repeated — the same
// dedupe-by-keys rule Compile applies within one group, extended across
// them so a screen restating a global doesn't print it twice.
func (o *Overlay) SetSections(secs []Section) {
	o.sections = CompileSections(secs)
	o.body.GotoTop()
	o.refresh()
}

// Sections returns the compiled groups the overlay is showing.
func (o Overlay) Sections() []Section { return o.sections }

// Query returns the active search string.
func (o Overlay) Query() string { return o.query }

// Init satisfies the component shape; the overlay starts nothing.
func (o Overlay) Init() tea.Cmd { return nil }

// Update handles search, scroll, and dismissal. A host that is not the app
// shell forwards every message here while the overlay is up and matches
// ClosedMsg to take it down.
func (o Overlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if mm, ok := msg.(mouse.Msg); ok {
		return o.handleMouse(mm)
	}
	if o.searchable && o.filter.Focused() {
		var cmd tea.Cmd
		o.filter, cmd = o.filter.Update(msg)
		// enter commits and esc cancels, both from inside the filter. Esc
		// therefore steps out of the search before it closes the overlay,
		// which is what stops a half-typed query taking the whole modal
		// with it.
		o.applyQuery()
		return o, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, o.keys.Close):
			return o, closed()
		case o.searchable && key.Matches(k, o.keys.Search):
			return o, o.FocusFilter()
		case key.Matches(k, o.keys.Top):
			o.body.GotoTop()
			return o, nil
		case key.Matches(k, o.keys.Bottom):
			o.body.GotoBottom()
			return o, nil
		}
	}
	var cmd tea.Cmd
	o.body, cmd = o.body.Update(msg)
	return o, cmd
}

// View renders the modal centered in the bounds it was given. The
// placement mirrors geom.CenterIn, which is what the pane's rect was
// computed with, so the modal hit-tests where it is actually drawn.
func (o Overlay) View() string {
	if o.rect.W <= 0 || o.rect.H <= 0 {
		return o.body.View()
	}
	return lipgloss.Place(o.rect.W, o.rect.H,
		lipgloss.Center, lipgloss.Center,
		o.body.View())
}

// SetRect treats r as the outer bounds — typically the body area the shell
// hands the overlay — measures the content, and centers itself inside them.
func (o *Overlay) SetRect(r geom.Rect) {
	o.rect = r
	_, w, h := o.plan(r)
	o.body.SetRect(geom.CenterIn(r, w, h))
	if o.searchable {
		o.placeInlineFilter(r)
		o.body.SetHeader(o.filterHeader())
		o.placeInlineFilter(r)
	}
	o.refresh()
}

// Rect returns the bounds the overlay was last given (not the smaller rect
// it centered itself into — see Bounds).
func (o Overlay) Rect() geom.Rect { return o.rect }

// Bounds returns the rect the modal actually occupies, which is what a
// host tests a click against to decide it landed outside.
func (o Overlay) Bounds() geom.Rect { return o.body.Rect() }

// FocusFilter moves input to the search field.
func (o *Overlay) FocusFilter() tea.Cmd {
	if !o.searchable {
		return nil
	}
	cmd := o.filter.Focus()
	o.body.SetHeader(o.filterHeader())
	return cmd
}

// BlurFilter returns input from the search field to the body.
func (o *Overlay) BlurFilter() {
	if !o.searchable {
		return
	}
	o.filter.Blur()
	o.body.SetHeader(o.filterHeader())
}

// IsCapturingKeys reports whether the search field is swallowing keys.
func (o Overlay) IsCapturingKeys() bool { return o.searchable && o.filter.Focused() }

// Help returns the bindings the overlay itself responds to, for the footer
// beneath it. The close binding comes first: it is the one key a user who
// opened this by accident needs.
func (o Overlay) Help() []key.Binding {
	out := []key.Binding{o.keys.Close}
	if o.searchable {
		if o.filter.Focused() {
			return append(out, o.filter.Help()...)
		}
		out = append(out, o.keys.Search)
	}
	return append(out, o.keys.Top, o.keys.Bottom)
}

// SetTitle sets the pane's top-left title.
func (o *Overlay) SetTitle(s string) { o.body.SetTitle(s) }

// handleMouse scrolls under the pointer, focuses the search field, and
// treats a press outside the modal as a dismissal — the mouse spelling of
// esc, and the reason the shell's own "? close" affordance needs no
// special case: it is outside.
func (o Overlay) handleMouse(e mouse.Msg) (Overlay, tea.Cmd) {
	if _, ok := o.body.HandleScrollbar(e); ok {
		return o, nil
	}
	if o.searchable && o.filter.Rect().Hit(e.X, e.Y) {
		if e.IsPointPress() {
			return o, o.FocusFilter()
		}
		return o, nil
	}
	switch {
	case e.IsWheelUp(), e.IsWheelDown():
		o.body.ScrollWheel(e.X, e.Y, e.IsWheelUp())
		return o, nil
	case e.IsPointPress():
		if !o.body.Rect().Hit(e.X, e.Y) {
			return o, closed()
		}
		// Any press inside the pane but off the filter row hands input back
		// to the body (rule 27).
		o.BlurFilter()
		return o, nil
	}
	return o, nil
}

// placeInlineFilter puts the search field on the pane's first inner row.
func (o *Overlay) placeInlineFilter(r geom.Rect) {
	inner := o.body.ContentRect()
	o.filter.SetInlineRect(geom.Rect{X: inner.X, Y: o.body.Rect().Y + 1, W: inner.W, H: 1, Gen: r.Gen})
}

// filterHeader is the search row plus the rule separating it from the list.
func (o Overlay) filterHeader() string {
	inner := o.body.ContentRect().W
	if inner <= 0 {
		return ""
	}
	rule := o.filterRuleInactive
	if o.filter.Focused() {
		rule = o.filterRuleActive
	}
	return o.filter.InlineView() + "\n" + rule.Render(strings.Repeat(o.glyphs.Rule, inner))
}

// applyQuery re-reads the filter and rebuilds the body when it changed.
func (o *Overlay) applyQuery() {
	q := strings.ToLower(strings.TrimSpace(o.filter.Value()))
	if q != o.query {
		o.query = q
		o.body.GotoTop()
	}
	o.body.SetHeader(o.filterHeader())
	o.refresh()
}

// refresh re-renders the body from the current sections and query, at the
// size the current bounds allow.
func (o *Overlay) refresh() {
	rows, _, _ := o.plan(o.rect)
	o.body.SetContent(strings.Join(rows, "\n"))
}

// visible returns the sections surviving the active query, dropping
// sections left with nothing.
func (o Overlay) visible() []Section {
	if o.query == "" {
		return o.sections
	}
	var out []Section
	for _, s := range o.sections {
		var kept []key.Binding
		for _, b := range s.Bindings {
			if matches(b, o.query) {
				kept = append(kept, b)
			}
		}
		if len(kept) > 0 {
			out = append(out, Section{Title: s.Title, Bindings: kept})
		}
	}
	return out
}

// matches reports whether a binding answers the query. Both halves are
// searched: users look for a key ("ctrl") about as often as for what it
// does ("mark").
func matches(b key.Binding, query string) bool {
	h := b.Help()
	return strings.Contains(strings.ToLower(h.Key), query) ||
		strings.Contains(strings.ToLower(h.Desc), query) ||
		strings.Contains(strings.ToLower(strings.Join(b.Keys(), " ")), query)
}

// block is one section's worth of content: a heading and its key/description
// pairs, unrendered. Blocks are kept whole when the list flows into columns —
// a heading in one column with its keys in the next would be worse than
// scrolling — and are laid out before they are rendered so each column can
// size its own key gutter.
type block struct {
	title string
	rows  [][2]string
}

// height is the rows the block occupies, heading included.
func (b block) height() int {
	h := len(b.rows)
	if b.title != "" {
		h++
	}
	return h
}

// blocks converts the visible sections into layout blocks.
func (o Overlay) blocks() []block {
	secs := o.visible()
	if len(secs) == 0 {
		return []block{{rows: [][2]string{{"", "no matching keys"}}}}
	}
	out := make([]block, 0, len(secs))
	for _, s := range secs {
		b := block{title: s.Title}
		for _, bd := range s.Bindings {
			h := bd.Help()
			b.rows = append(b.rows, [2]string{h.Key, h.Desc})
		}
		out = append(out, b)
	}
	return out
}

// renderColumn lays the blocks out against one key gutter, and reports the
// widest line it produced.
func (o Overlay) renderColumn(col []block) (lines []string, width int) {
	keyW := 0
	for _, b := range col {
		for _, r := range b.rows {
			if w := lipgloss.Width(r[0]); w > keyW {
				keyW = w
			}
		}
	}
	indent := strings.Repeat(" ", overlayIndent)
	gap := strings.Repeat(" ", overlayKeyDescGap)

	for i, b := range col {
		if i > 0 {
			lines = append(lines, "")
		}
		if b.title != "" {
			lines = append(lines, o.sectionStyle.Render(b.title))
			if w := lipgloss.Width(b.title); w > width {
				width = w
			}
		}
		for _, r := range b.rows {
			key, desc := r[0], r[1]
			style := o.descStyle
			if key == "" && b.title == "" {
				style = o.emptyStyle // the "no matching keys" line
			}
			lines = append(lines, indent+o.keyStyle.Render(padRight(key, keyW))+gap+style.Render(desc))
			if w := overlayIndent + keyW + overlayKeyDescGap + lipgloss.Width(desc); w > width {
				width = w
			}
		}
	}
	return lines, width
}

// plan renders the blocks and returns the body rows plus the modal's outer
// size.
//
// One column, top to bottom, scrolling when it doesn't fit. Flowing into
// columns fits more on a page but costs the reader the one thing a
// reference gives them: a fixed place for each group. Scroll position is
// something they control; a layout that reshuffles at every width is not.
func (o Overlay) plan(r geom.Rect) (rows []string, w, h int) {
	rows, contentW := o.renderColumn(o.blocks())

	chromeH := 2 // borders
	if o.searchable {
		chromeH += 2 // filter row + rule
	}
	// The pane's border takes two columns and it always reserves one more for
	// the scrollbar track, drawn or not — content measured against the outer
	// width alone loses its last cell to it.
	chromeW := 2 + pane.ScrollbarWidth

	w = clampSize(contentW+chromeW, overlayMinWidth, r.W, overlayWidthNum, overlayWidthDen)
	h = clampSize(len(rows)+chromeH, overlayMinHeight, r.H, overlayHeightNum, overlayHeightDen)
	return rows, w, h
}

// clampSize caps want at num/den of avail (never below min, never above
// avail), which is the sizing rule pkg/alert's autosize mode uses.
func clampSize(want, floor, avail, num, den int) int {
	limit := avail * num / den
	if limit < floor {
		limit = floor
	}
	if limit > avail {
		limit = avail
	}
	if want > limit {
		want = limit
	}
	if want < floor {
		want = floor
	}
	if want > avail {
		want = avail
	}
	return want
}

// CompileSections drops empty groups and bindings with no help text, and
// removes a binding repeated *within* one group.
//
// Deduping stops at the group boundary on purpose. Two panes on one screen
// bind ↑/k to "up" in each of them, and those are not duplicates — they are
// the same verb aimed at different components, and dropping the second
// leaves a pane looking as though it cannot be scrolled. The one genuine
// overlap, a screen restating the shell's globals, is handled by Suppress,
// which the host applies with the bindings it actually owns.
func CompileSections(secs []Section) []Section {
	out := make([]Section, 0, len(secs))
	for _, s := range secs {
		seen := make(map[string]struct{}, len(s.Bindings))
		kept := make([]key.Binding, 0, len(s.Bindings))
		for _, b := range s.Bindings {
			if b.Help().Key == "" && b.Help().Desc == "" {
				continue
			}
			k := strings.Join(b.Keys(), " ")
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			continue
		}
		out = append(out, Section{Title: s.Title, Bindings: kept})
	}
	return out
}

// Suppress removes from secs any binding whose keys are already claimed
// elsewhere, and drops the groups that empties.
//
// The app shell applies it with its own globals: screens list q and t in
// their Help() by convention, and a shell that prepends a Global group would
// otherwise print them twice under two headings. It takes the claimed
// bindings explicitly rather than assuming the first group owns everything —
// only keys that really are app-wide should silence a component's.
func Suppress(claimed []key.Binding, secs []Section) []Section {
	if len(claimed) == 0 {
		return secs
	}
	taken := make(map[string]struct{}, len(claimed))
	for _, b := range claimed {
		taken[strings.Join(b.Keys(), " ")] = struct{}{}
	}
	out := make([]Section, 0, len(secs))
	for _, s := range secs {
		kept := make([]key.Binding, 0, len(s.Bindings))
		for _, b := range s.Bindings {
			if _, dup := taken[strings.Join(b.Keys(), " ")]; dup {
				continue
			}
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			continue
		}
		out = append(out, Section{Title: s.Title, Bindings: kept})
	}
	return out
}
