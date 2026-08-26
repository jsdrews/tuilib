// Package textview is a read-only viewer for static text content, wrapped
// in a pane. Reach for it when the payload is a rendered document — a
// README, a diff, a kubectl describe, a PR body — where logview's
// streaming shape (append / follow / filter mode / MaxLines trim) is
// noise. textview drops all of that and keeps the machinery that read-
// static-text actually needs: scroll, search, wrap.
//
// The body is a pkg/pane.Pane (so ↑↓/j/k/PgUp/PgDn scrolling work out of
// the box), plus an optional pkg/filter.Model for the "/-to-search"
// overlay. Content is replaced wholesale via SetContent; there is no
// Append or Clear — see pkg/logview for the streaming case.
//
// Behavior:
//   - SetContent replaces the buffer and resets scroll to the top.
//   - Wrap is on by default: long lines wrap to the pane's inner width
//     (ANSI-aware via x/ansi.Wrap). Toggle at runtime with `w`. Wrap-off
//     mode falls back to pane's own truncation + horizontal scroll (rule 16).
//   - "/" focuses an embedded filter input; typing highlights case-
//     insensitive substring matches inline; enter blurs (keeps the query
//     active for n/N); esc clears.
//   - n / N step to the next / previous match.
//   - g / G jump to top / bottom (rule 23: no `gg` — single g means top
//     library-wide).
//   - ctrl+u / ctrl+d half-page; pgup / pgdown full-page.
//   - No filter mode. No follow. No MaxLines. If you want streaming +
//     filter drop + tail-mode, use pkg/logview.
package textview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Options configures a new textview. Zero-value fields fall back to
// defaults; start from theme.TextView() to fill in the color tokens.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "text".
	Title string
	// Content is the initial body text. Replace at runtime via SetContent.
	Content string
	// Wrap enables word-wrapping to the pane's inner width. Default is
	// true; when false the pane's built-in truncation + horizontal scroll
	// handles overflow (rule 16).
	Wrap bool
	// Searchable embeds a filter.Model above the body pane (three rows).
	// When false, "/" is ignored and the full height is used for content.
	Searchable bool

	// MatchStyle is the lipgloss style applied to matched substrings while
	// a query is active. Pass via theme.TextView() for a sensible default.
	MatchStyle lipgloss.Style

	// CurrentLineStyle is applied to the entire line holding the current
	// match, padded to the pane's inner width so a Background paints the
	// whole row. Zero value leaves the row unstyled.
	CurrentLineStyle lipgloss.Style

	// Pane pass-throughs. See pkg/pane.Options for defaults.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle

	// SpinnerStyle is applied to the spinner glyph rendered while the
	// textview is in its loading state (see SetLoading).
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading.
	LoadingLabel string

	// Filter configures the embedded filter. Ignored when Searchable=false.
	Filter filter.Options

	// Keys is the textview's keymap. Leave zero to use DefaultKeys.
	Keys Keys
}

// Keys is the textview's keymap. Each binding carries both its dispatch
// keys (WithKeys) and its help label (WithHelp) — Update and Help() read
// from the same struct, so a custom binding propagates everywhere. Pane
// covers horizontal scroll and pgup/pgdn/half-page; textview owns the
// search + wrap-toggle bindings.
type Keys struct {
	Search, NextMatch, PrevMatch key.Binding
	Top, Bottom                  key.Binding
	Wrap                         key.Binding
	Pane                         pane.Keys
}

// DefaultKeys returns the textview's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		NextMatch: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
		Top:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Wrap:      key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "wrap")),
		Pane:      pane.DefaultKeys(),
	}
}

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
	if len(k.Wrap.Keys()) == 0 {
		k.Wrap = d.Wrap
	}
	k.Pane.FillDefaults()
}

// Model is the textview widget. Embed by value; mutate via the setters.
type Model struct {
	raw   string   // original content as passed in
	lines []string // display lines after wrap (or raw lines when unwrapped)
	wrap  bool

	body       pane.Pane
	filter     filter.Model
	searchable bool

	matchStyle       lipgloss.Style
	currentLineStyle lipgloss.Style
	matches          []matchPos // over m.lines
	matchIdx         int        // -1 when no current match
	query            string     // last applied query (lower-cased)

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
	start, end int
}

// New constructs a textview.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "text"
	}
	opts.Keys.fillDefaults()

	m := Model{
		token:            focus.NewToken(),
		raw:              opts.Content,
		wrap:             opts.Wrap,
		searchable:       opts.Searchable,
		matchStyle:       opts.MatchStyle,
		currentLineStyle: opts.CurrentLineStyle,
		matchIdx:         -1,
		keys:             opts.Keys,
	}

	bodyH := opts.Height
	if m.searchable {
		bodyH = max0(opts.Height - 3)
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
		HScrollbar:     !opts.Wrap,
		SpinnerStyle:   opts.SpinnerStyle,
		LoadingLabel:   opts.LoadingLabel,
		Keys:           opts.Keys.Pane,
	})
	m.reflow()
	m.refresh()
	return m
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update dispatches search, match jumping, top/bottom jumps, wrap toggle,
// and forwards everything else to the body pane.
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
			m.refreshStatus()
			return m, nil
		case key.Matches(k, m.keys.Bottom):
			m.body.GotoBottom()
			m.refreshStatus()
			return m, nil
		case key.Matches(k, m.keys.Wrap):
			m.wrap = !m.wrap
			m.reflow()
			m.recomputeMatches()
			m.refresh()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	if _, ok := msg.(tea.KeyMsg); ok {
		m.refreshStatus()
	}
	return m, cmd
}

// View stacks the filter (when searchable) above the body pane.
func (m Model) View() string { return m.body.View() }

// SetContent replaces the buffer and resets scroll to the top. Any active
// search query is preserved; matches are recomputed against the new
// content and the current match resets to the first hit.
func (m *Model) SetContent(s string) {
	m.raw = s
	m.reflow()
	m.recomputeMatches()
	if len(m.matches) > 0 {
		m.matchIdx = 0
	} else {
		m.matchIdx = -1
	}
	m.body.GotoTop()
	m.refresh()
}

// Content returns the raw text (pre-wrap) currently loaded.
func (m Model) Content() string { return m.raw }

// Wrap reports whether word-wrap is currently enabled.
func (m Model) Wrap() bool { return m.wrap }

// SetWrap flips wrap on or off. When enabled, content re-wraps to the
// pane's inner width; when disabled, pane's own horizontal scroll takes over.
func (m *Model) SetWrap(b bool) {
	if m.wrap == b {
		return
	}
	m.wrap = b
	m.reflow()
	m.recomputeMatches()
	m.refresh()
}

// SetRect places the textview in the given rect. When searchable, the filter
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
	m.reflow()
	m.recomputeMatches()
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

// SetMatchStyle updates the highlight style applied to matched substrings.
func (m *Model) SetMatchStyle(s lipgloss.Style) {
	m.matchStyle = s
	m.refresh()
}

// SetCurrentLineStyle updates the style applied to the line holding the
// current match.
func (m *Model) SetCurrentLineStyle(s lipgloss.Style) {
	m.currentLineStyle = s
	m.refresh()
}

// Searching reports whether the embedded filter currently has focus.
// Mirror this from the enclosing screen's IsCapturingKeys() so the app
// shell keeps its global keys (q, t, esc) out of the search input.
func (m Model) Searching() bool { return m.searchable && m.filter.Focused() }

// IsCapturingKeys reports whether the textview currently swallows printable
// keys — true while its search filter is focused. Satisfies focus.Capturer.
func (m Model) IsCapturingKeys() bool { return m.Searching() }

// Query returns the current search text ("" when no search is active).
func (m Model) Query() string {
	if m.searchable {
		return m.filter.Value()
	}
	return ""
}

// SetQuery sets the search text programmatically (no-op when not
// Searchable).
func (m *Model) SetQuery(s string) {
	if !m.searchable {
		return
	}
	m.filter.SetValue(s)
	m.applyQuery()
}

// Loading reports whether the textview is currently in its loading state.
func (m Model) Loading() bool { return m.body.Loading() }

// SetLoading toggles the loading state. When entering, returns the
// spinner's initial Tick command — propagate it back from your screen's
// Update so the spinner animates.
func (m *Model) SetLoading(b bool) tea.Cmd { return m.body.SetLoading(b) }

// SetLoadingLabel updates the text rendered next to the spinner while loading.
func (m *Model) SetLoadingLabel(s string) { m.body.SetLoadingLabel(s) }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
func (m *Model) SetSpinnerStyle(s lipgloss.Style) { m.body.SetSpinnerStyle(s) }

// Help returns the keys this textview responds to. While the embedded
// filter is focused, returns the filter's keys; otherwise the navigation
// + search + wrap bindings appropriate for the current state.
func (m Model) Help() []key.Binding {
	if m.searchable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "scroll")),
		key.NewBinding(key.WithKeys("pgup", "pgdown"), key.WithHelp("pgup/pgdn", "page")),
		m.keys.Top, m.keys.Bottom, m.keys.Wrap,
	}
	out = append(out, m.body.HelpBindings()...)
	if m.searchable {
		out = append(out, m.keys.Search)
		if m.query != "" {
			out = append(out, m.keys.NextMatch, m.keys.PrevMatch)
		}
	}
	return out
}

// ---- internals -----------------------------------------------------------

// reflow re-splits m.raw into m.lines according to the current wrap
// setting and the pane's inner width. Called on SetContent, SetWrap,
// SetRect.
func (m *Model) reflow() {
	if m.raw == "" {
		m.lines = nil
		return
	}
	if !m.wrap {
		m.lines = strings.Split(m.raw, "\n")
		return
	}
	inner := m.body.VisibleWidth()
	if inner < 1 {
		m.lines = strings.Split(m.raw, "\n")
		return
	}
	m.lines = strings.Split(ansi.Wrap(m.raw, inner, " -"), "\n")
}

func (m *Model) applyQuery() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == m.query {
		m.refreshStatus()
		return
	}
	m.query = q
	m.recomputeMatches()
	if len(m.matches) > 0 {
		m.matchIdx = 0
		m.scrollToMatch(m.matchIdx)
	} else {
		m.matchIdx = -1
	}
	m.refresh()
}

func (m *Model) recomputeMatches() {
	m.matches = m.matches[:0]
	if m.query == "" {
		m.matchIdx = -1
		return
	}
	for i, line := range m.lines {
		lower := strings.ToLower(ansi.Strip(line))
		offset := 0
		for {
			idx := strings.Index(lower[offset:], m.query)
			if idx < 0 {
				break
			}
			start := offset + idx
			end := start + len(m.query)
			m.matches = append(m.matches, matchPos{line: i, start: start, end: end})
			offset = end
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
	m.refreshStatus()
}

func (m *Model) scrollToMatch(idx int) {
	if idx < 0 || idx >= len(m.matches) {
		return
	}
	line := m.matches[idx].line
	margin := m.body.Height() / 3
	target := max0(line - margin)
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
	var b strings.Builder
	for i, line := range m.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.formatLine(line, byLine[i], i == curLine))
	}
	return b.String()
}

// formatLine renders one line, applying MatchStyle to any matched spans
// and CurrentLineStyle to the entire line when it holds the current match.
// Mirrors pkg/logview's approach so the row background paints continuously
// across match highlights (see logview.formatLine for the rationale).
func (m *Model) formatLine(line string, spans []matchPos, current bool) string {
	if !current {
		return m.renderLine(line, spans)
	}
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

	inner := max0(m.body.Width() - 2 - pane.ScrollbarWidth)
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
	if m.query == "" {
		m.body.SetBottomLeft("")
		return
	}
	cur := m.matchIdx + 1
	if cur <= 0 {
		cur = 0
	}
	m.body.SetBottomLeft(fmt.Sprintf("%d/%d", cur, len(m.matches)))
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// FocusToken returns the textview's stable focus identity. See
// focus.Identified.
func (m Model) FocusToken() focus.Token { return m.token }

// handleMouse routes a mouse event that may or may not belong to this
// textview. A document has no cursor to place, so a press only asks for
// focus; the wheel scrolls the pane under the pointer regardless of focus.
func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if _, ok := m.body.HandleScrollbar(e); ok {
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
