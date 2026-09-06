// Package logview is a streaming text viewer with incremental search and
// keyboard navigation, wrapped in a pane.
//
// Use it for tailing logs, command output, or any append-mostly text where
// the user wants to scroll back, search for a substring, and jump between
// matches without losing the live tail. The body is a pkg/pane.Pane (so
// pgup/pgdn/arrows/mouse-wheel scrolling work out of the box) plus a
// pkg/filter.Model for the "/-to-search" overlay.
//
// Features:
//   - Append / AppendLines / Clear streaming API
//   - MaxLines ring-buffer cap (0 = unbounded)
//   - Auto-follow: stays glued to the bottom while parked there; pauses
//     when the user scrolls up; "G" jumps to bottom and re-engages follow
//   - "/" focuses an embedded filter input; typing highlights case-insensitive
//     substring matches inline; enter blurs (keeps the query active for n/N);
//     esc clears
//   - n / N step to the next / previous match
//   - g / G jump to top / bottom
//   - "\" toggles filter mode: when on (and a query is active), only lines
//     containing the query are shown — highlights and n/N jumping still work
//   - Pane bottom-left status: "FOLLOWING" or "PAUSED", plus "m/n" while
//     a query is active, plus "filter" while filter mode is on
//
// Items pushed in are plain strings, one per line. Render the highlight in
// place by setting MatchStyle (or via theme.Logview()).
package logview

import (
	"fmt"
	"sort"
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

// DefaultMaxLines is the safety cap applied when Options.MaxLines is 0 —
// roughly an hour of moderately busy log output at one line per second.
const DefaultMaxLines = 10000

// Options configures a new logview. Zero-value fields fall back to defaults.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "logs".
	Title string
	// Searchable embeds a filter.Model above the body pane (three rows). If
	// false, "/" is ignored and the full height is used for log content.
	Searchable bool
	// MaxLines caps the number of buffered lines. 0 (default) applies a
	// safety cap of DefaultMaxLines so an open-ended stream can't grow
	// without bound. A positive value sets an explicit cap; -1 disables
	// the cap entirely (use only when the producer is bounded).
	MaxLines int

	// MatchStyle is the lipgloss style applied to matched substrings while
	// a query is active. Pass via theme.Logview() for a sensible default;
	// the zero value renders matches without any visual highlight.
	MatchStyle lipgloss.Style

	// CurrentLineStyle is applied to the entire line holding the current
	// match (the one n/N steps onto), padded out to the pane's inner width
	// so a Background paints the whole row. Set fg/bg/bold/etc as you like;
	// the zero value leaves the row unstyled. theme.Logview() seeds a
	// subtle background by default.
	CurrentLineStyle lipgloss.Style

	// FilterMode controls the initial filter-only state. When true (and a
	// query is active), only matching lines are shown. Toggle at runtime
	// via "\" or SetFilterMode.
	FilterMode bool

	// Pane pass-throughs. See pkg/pane.Options for defaults.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle
	HScrollbar     bool

	// SpinnerStyle is applied to the spinner glyph rendered while the
	// logview is in its loading state (see SetLoading). Pass via
	// theme.Logview() for a sensible default.
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading.
	LoadingLabel string

	// Filter configures the embedded filter. Ignored when Searchable=false.
	Filter filter.Options

	// Keys is the logview's keymap. Leave zero to use DefaultKeys; set
	// individual bindings to override (others fall back to defaults via
	// fillDefaults). theme.Logview() pre-populates this.
	Keys Keys
}

// Keys is the logview's keymap. Each binding carries both its dispatch
// keys (WithKeys) and its help label (WithHelp) — Update and Help() read
// from the same struct, so a custom binding propagates everywhere. The
// embedded pane.Keys covers horizontal scroll; mutate fields on Pane to
// override h-scroll without touching the rest.
type Keys struct {
	Search, NextMatch, PrevMatch key.Binding
	Top, Bottom                  key.Binding
	Filter                       key.Binding
	Pane                         pane.Keys
}

// DefaultKeys returns the logview's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		NextMatch: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
		Top:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Filter:    key.NewBinding(key.WithKeys("\\"), key.WithHelp(`\`, "filter mode")),
		Pane:      pane.DefaultKeys(),
	}
}

// fillDefaults fills any zero-valued binding in k with its DefaultKeys()
// counterpart, so partial overrides on Options.Keys work without
// restating every field.
func (k *Keys) fillDefaults() {
	d := DefaultKeys()
	if len(k.Search.Keys()) == 0 {
		k.Search = d.Search
	}
	if len(k.NextMatch.Keys()) == 0 {
		k.NextMatch = d.NextMatch
	}
	if len(k.PrevMatch.Keys()) == 0 {
		k.PrevMatch = d.PrevMatch
	}
	if len(k.Top.Keys()) == 0 {
		k.Top = d.Top
	}
	if len(k.Bottom.Keys()) == 0 {
		k.Bottom = d.Bottom
	}
	if len(k.Filter.Keys()) == 0 {
		k.Filter = d.Filter
	}
	k.Pane.FillDefaults()
}

// Model is the logview widget. Embed by value; mutate via the setters.
type Model struct {
	lines    []string
	maxLines int
	follow   bool

	body       pane.Pane
	filter     filter.Model
	searchable bool

	matchStyle       lipgloss.Style
	currentLineStyle lipgloss.Style
	matches          []matchPos
	matchIdx         int    // -1 when no current match
	query            string // last applied query (lower-cased)

	filterMode bool  // when true and query != "", only matching lines render
	visibleIdx []int // line indices that match (sorted); valid only while query != ""

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
}

type matchPos struct {
	line       int
	start, end int // byte offsets inside the line
}

// New constructs a logview. Call Update/View from the parent model; push
// content via Append / AppendLines as it arrives.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "logs"
	}
	if opts.MaxLines == 0 {
		opts.MaxLines = DefaultMaxLines
	}
	opts.Keys.fillDefaults()

	m := Model{
		token:            focus.NewToken(),
		maxLines:         opts.MaxLines,
		follow:           true,
		searchable:       opts.Searchable,
		matchStyle:       opts.MatchStyle,
		currentLineStyle: opts.CurrentLineStyle,
		matchIdx:         -1,
		filterMode:       opts.FilterMode,
		keys:             opts.Keys,
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
	m.refresh()
	return m
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// Update handles search ("/"), match jumping (n/N), top/bottom (g/G), and
// forwards everything else to the body pane (which routes to the embedded
// viewport for pgup/pgdn/arrows/mouse-wheel). Auto-follow is recomputed
// from pane.AtBottom() after every key.
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
		return m, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case m.searchable && key.Matches(k, m.keys.Search):
			return m, m.FocusFilter()
		case key.Matches(k, m.keys.NextMatch):
			m.jumpMatch(+1)
			return m, nil
		case key.Matches(k, m.keys.PrevMatch):
			m.jumpMatch(-1)
			return m, nil
		case key.Matches(k, m.keys.Top):
			m.body.GotoTop()
			m.follow = false
			m.refreshStatus()
			return m, nil
		case key.Matches(k, m.keys.Bottom):
			m.body.GotoBottom()
			m.follow = true
			m.refreshStatus()
			return m, nil
		case m.searchable && key.Matches(k, m.keys.Filter):
			m.filterMode = !m.filterMode
			m.refresh()
			if m.matchIdx >= 0 {
				m.scrollToMatch(m.matchIdx)
			}
			m.follow = m.body.AtBottom()
			m.refreshStatus()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	if _, ok := msg.(tea.KeyMsg); ok {
		m.follow = m.body.AtBottom()
		m.refreshStatus()
	}
	return m, cmd
}

// View stacks the filter (when searchable) above the body pane.
func (m Model) View() string { return m.body.View() }

// Append adds one line and, when following, scrolls to the new bottom.
func (m *Model) Append(line string) {
	m.lines = append(m.lines, line)
	m.trim()
	m.recomputeMatches()
	m.refresh()
	if m.follow {
		m.body.GotoBottom()
	}
}

// AppendLines adds a batch of lines and, when following, scrolls to the
// new bottom — cheaper than calling Append in a loop for large bursts.
func (m *Model) AppendLines(lines []string) {
	if len(lines) == 0 {
		return
	}
	m.lines = append(m.lines, lines...)
	m.trim()
	m.recomputeMatches()
	m.refresh()
	if m.follow {
		m.body.GotoBottom()
	}
}

// Clear empties the buffer, drops any active query state, and re-engages
// auto-follow.
func (m *Model) Clear() {
	m.lines = nil
	m.matches = nil
	m.matchIdx = -1
	m.follow = true
	m.refresh()
}

// Lines returns the buffered lines (newest last). The returned slice
// aliases the internal buffer — copy it if you intend to retain it.
func (m Model) Lines() []string { return m.lines }

// Following reports whether the view is glued to the bottom.
func (m Model) Following() bool { return m.follow }

// SetFollow turns auto-follow on or off explicitly. SetFollow(true) jumps
// to the bottom; SetFollow(false) leaves the scroll position alone.
func (m *Model) SetFollow(b bool) {
	m.follow = b
	if b {
		m.body.GotoBottom()
	}
	m.refreshStatus()
}

// FilterMode reports whether non-matching lines are currently hidden.
func (m Model) FilterMode() bool { return m.filterMode }

// SetFilterMode turns filter-only rendering on or off. Has no visible
// effect until a query is set; takes effect immediately on the next refresh.
func (m *Model) SetFilterMode(b bool) {
	if m.filterMode == b {
		return
	}
	m.filterMode = b
	m.refresh()
	if m.matchIdx >= 0 {
		m.scrollToMatch(m.matchIdx)
	}
	m.follow = m.body.AtBottom()
	m.refreshStatus()
}

// Searching reports whether the embedded filter currently has focus.
// Mirror this from the enclosing screen's IsCapturingKeys() so the app
// shell keeps its global keys (q, t, esc) out of the search input.
func (m Model) Searching() bool { return m.searchable && m.filter.Focused() }

// IsCapturingKeys reports whether the logview currently swallows printable
// keys — true while its search filter is focused. Satisfies focus.Capturer.
func (m Model) IsCapturingKeys() bool { return m.Searching() }

// Query returns the current search text ("" when no search is active).
func (m Model) Query() string {
	if m.searchable {
		return m.filter.Value()
	}
	return ""
}

// SetQuery sets the search text programmatically (no-op when not Searchable).
func (m *Model) SetQuery(s string) {
	if !m.searchable {
		return
	}
	m.filter.SetValue(s)
	m.applyQuery()
}

// SetRect places the logview in the given rect. When searchable, the filter
// takes the top 3 rows and the body pane gets the rest, offset below it.
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

// SetCurrentLineStyle updates the style applied to the "▸ " marker that
// fronts the line holding the current match.
func (m *Model) SetCurrentLineStyle(s lipgloss.Style) {
	m.currentLineStyle = s
	m.refresh()
}

// Loading reports whether the logview is currently in its loading state.
func (m Model) Loading() bool { return m.body.Loading() }

// SetLoading toggles the loading state. When entering, returns the
// spinner's initial Tick command — propagate it back from your screen's
// Update so the spinner animates.
func (m *Model) SetLoading(b bool) tea.Cmd { return m.body.SetLoading(b) }

// SetLoadingLabel updates the text rendered next to the spinner while loading.
func (m *Model) SetLoadingLabel(s string) { m.body.SetLoadingLabel(s) }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
func (m *Model) SetSpinnerStyle(s lipgloss.Style) { m.body.SetSpinnerStyle(s) }

// Help returns the keys this logview responds to. While the embedded
// filter is focused, returns the filter's keys; otherwise the navigation
// + search bindings appropriate for the current state.
func (m Model) Help() []key.Binding {
	if m.searchable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "scroll")),
		key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page")),
		m.keys.Top, m.keys.Bottom,
	}
	out = append(out, m.body.HelpBindings()...)
	if m.searchable {
		out = append(out, m.keys.Search)
		if m.query != "" {
			out = append(out, m.keys.NextMatch, m.keys.PrevMatch)
			// Filter-mode label flips based on state; mutate a copy of the
			// keymap entry rather than fabricating a fresh binding so the
			// dispatch key still flows from m.keys.
			label := "filter mode"
			if m.filterMode {
				label = "show all"
			}
			fk := m.keys.Filter
			fk.SetHelp(fk.Help().Key, label)
			out = append(out, fk)
		}
	}
	return out
}

// ---- internals -----------------------------------------------------------

func (m *Model) trim() {
	if m.maxLines < 0 || len(m.lines) <= m.maxLines {
		return
	}
	m.lines = append([]string(nil), m.lines[len(m.lines)-m.maxLines:]...)
}

func (m *Model) applyQuery() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == m.query {
		// still recompute status (filter view may have changed cursor only)
		m.refreshStatus()
		return
	}
	m.query = q
	m.recomputeMatches()
	if len(m.matches) > 0 {
		m.matchIdx = 0
		m.scrollToMatch(m.matchIdx)
		m.follow = m.body.AtBottom()
	} else {
		m.matchIdx = -1
	}
	m.refresh()
}

func (m *Model) recomputeMatches() {
	m.matches = m.matches[:0]
	m.visibleIdx = m.visibleIdx[:0]
	if m.query == "" {
		m.matchIdx = -1
		return
	}
	for i, line := range m.lines {
		lower := strings.ToLower(line)
		offset := 0
		had := false
		for {
			idx := strings.Index(lower[offset:], m.query)
			if idx < 0 {
				break
			}
			start := offset + idx
			end := start + len(m.query)
			m.matches = append(m.matches, matchPos{line: i, start: start, end: end})
			offset = end
			had = true
		}
		if had {
			m.visibleIdx = append(m.visibleIdx, i)
		}
	}
	if m.matchIdx >= len(m.matches) {
		if len(m.matches) == 0 {
			m.matchIdx = -1
		} else {
			m.matchIdx = len(m.matches) - 1
		}
	}
}

func (m *Model) jumpMatch(step int) {
	if len(m.matches) == 0 {
		return
	}
	if m.matchIdx < 0 {
		m.matchIdx = 0
	} else {
		m.matchIdx = (m.matchIdx + step + len(m.matches)) % len(m.matches)
	}
	m.refresh()
	m.scrollToMatch(m.matchIdx)
	m.follow = m.body.AtBottom()
	m.refreshStatus()
}

func (m *Model) scrollToMatch(idx int) {
	if idx < 0 || idx >= len(m.matches) {
		return
	}
	line := m.matches[idx].line
	if m.filterMode && m.query != "" {
		line = sort.SearchInts(m.visibleIdx, line)
	}
	// Park the matched line about 1/3 from the top of the visible area.
	margin := m.body.Height() / 3
	target := max(0, line-margin)
	m.body.SetYOffset(target)
}

func (m *Model) refresh() {
	if m.searchable {
		m.body.SetHeader(m.filterHeader())
	}
	m.body.SetContent(m.renderContent())
	m.refreshStatus()
}

func (m *Model) renderContent() string {
	if len(m.lines) == 0 {
		return ""
	}
	if m.query == "" {
		return strings.Join(m.lines, "\n")
	}
	byLine := make(map[int][]matchPos, len(m.matches))
	for _, mp := range m.matches {
		byLine[mp.line] = append(byLine[mp.line], mp)
	}
	curLine := -1
	if m.matchIdx >= 0 && m.matchIdx < len(m.matches) {
		curLine = m.matches[m.matchIdx].line
	}
	if m.filterMode {
		if len(m.visibleIdx) == 0 {
			return ""
		}
		var b strings.Builder
		for n, i := range m.visibleIdx {
			if n > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(m.formatLine(m.lines[i], byLine[i], i == curLine))
		}
		return b.String()
	}
	var b strings.Builder
	for i, line := range m.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.formatLine(line, byLine[i], i == curLine))
	}
	return b.String()
}

func (m *Model) formatLine(line string, spans []matchPos, current bool) string {
	if !current {
		return m.renderLine(line, spans)
	}
	// For the current row we can't just wrap the pre-rendered line in
	// currentLineStyle.Render — embedded match spans emit their own ANSI
	// resets, which drop the outer background partway through the line.
	// Instead, render every segment (gaps and matches alike) with the row
	// style baked in so the background paints continuously.
	base := m.currentLineStyle
	matchOnRow := m.matchStyle.Inherit(base)

	var b strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp.start > cursor {
			b.WriteString(base.Render(line[cursor:sp.start]))
		}
		b.WriteString(matchOnRow.Render(line[sp.start:sp.end]))
		cursor = sp.end
	}
	if cursor < len(line) {
		b.WriteString(base.Render(line[cursor:]))
	}

	inner := max(0, m.body.Width()-2-pane.ScrollbarWidth)
	if pad := inner - lipgloss.Width(b.String()); pad > 0 {
		b.WriteString(base.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

func (m *Model) renderLine(line string, spans []matchPos) string {
	if len(spans) == 0 {
		return line
	}
	var b strings.Builder
	cursor := 0
	for _, sp := range spans {
		if sp.start > cursor {
			b.WriteString(line[cursor:sp.start])
		}
		b.WriteString(m.matchStyle.Render(line[sp.start:sp.end]))
		cursor = sp.end
	}
	if cursor < len(line) {
		b.WriteString(line[cursor:])
	}
	return b.String()
}

func (m *Model) refreshStatus() {
	state := "PAUSED"
	if m.follow {
		state = "FOLLOWING"
	}
	if m.query != "" {
		cur := m.matchIdx + 1
		if cur <= 0 {
			cur = 0
		}
		state = fmt.Sprintf("%s · %d/%d", state, cur, len(m.matches))
		if m.filterMode {
			state += " · filter"
		}
	}
	m.body.SetBottomLeft(state)
}

// FocusToken returns the logview's stable focus identity. See
// focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// handleMouse routes a mouse event that may or may not belong to this
// logview. There is no cursor to place, so a press only asks for focus; the
// wheel scrolls the pane under the pointer whether or not it has focus.
//
// Scrolling up necessarily disengages auto-follow, exactly as it does when
// the user scrolls with the keyboard — the pane is no longer parked at the
// bottom, and silently snapping back on the next appended line would fight
// the user.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if _, ok := m.body.HandleScrollbar(e); ok {
		m.follow = m.body.AtBottom()
		m.refreshStatus()
		return m, nil
	}
	if m.searchable && m.filter.Rect().Hit(e.X, e.Y) {
		if e.IsPointPress() {
			return m, tea.Batch(m.FocusFilter(), focus.RequestSelf(m.token))
		}
		return m, nil
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
	case e.IsWheelUp(), e.IsWheelDown():
		if !m.body.ScrollWheel(e.X, e.Y, e.IsWheelUp()) {
			return m, nil
		}
		m.follow = m.body.AtBottom()
		m.refreshStatus()
		return m, nil

	case e.IsPointPress():
		if !m.body.Rect().Hit(e.X, e.Y) {
			return m, nil
		}
		return m, focus.RequestSelf(m.token)
	}
	return m, nil
}
