// Package eventlog shows Events and Lines read in order from a remote
// source too large to hold: an AWX job's events, an Elasticsearch index, a
// Prefect run's logs. It is pkg/logview's counterpart for data that is
// fetched in pages rather than streamed — logview stays the component for a
// stream held whole (CLAUDE.md rule 34).
//
// The item is the unit. An Item has a Key, the Lines it draws as, and
// optional Data for an inspector: a multi-line event is drawn as a block, a
// 0-line item draws as a dim "· no output" line — it still happened, and
// may carry fields worth opening — and the cursor moves item by item. There is no
// sort — the order is the timeline.
//
// Two kinds of source, one component:
//
//   - Seekable data (item N fetchable, with a total) arrives through
//     SetPage at logical offsets. Adjacent pages merge into one contiguous
//     range; a far jump replaces it. Drive it with source.Model and
//     Options.MaxHeld.
//   - Anchored data (walked from an anchor, no total) is a Span: New with
//     Options.Anchored, then Append and Prepend. Items are keyed by their
//     cursors, and Edges gives the cursors to extend from. Drive it with
//     source.Anchored.
//
// Either way it holds at most Options.MaxItems, trimming the end furthest
// from the viewport, and reports what it holds in ViewportChangedMsg so the
// source can mirror it.
//
// It shares its remote machinery with pkg/table: items that answer a query
// other than the committed one stay on screen dimmed, the border names what
// they answer and what is loading, and nothing blanks after the first
// answer. On top of that:
//
//   - Follow keeps the view on the newest item while data is Growing. Move
//     away and new items are counted instead ("↓ 42 new"); G returns.
//   - Search jumps. "/" highlights resident items as you type; n/N jump
//     among them, and past what is held emit FindMsg for the source to
//     answer. The filter narrows the query instead (QueryChangedMsg) —
//     they are separate controls.
//   - Enter or a double click emits ActivatedMsg with the item's Key and
//     Data, for the screen to open in an inspector.
package eventlog

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/internal/remoteview"
	"github.com/jsdrews/tuilib/internal/tick"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/focus"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/query"
)

// DefaultMaxItems is the resident cap when Options.MaxItems is zero.
const DefaultMaxItems = 5000

// Item is one entry in the timeline: an event or a line.
type Item struct {
	// Key identifies the item across fetches — for Anchored data, its
	// cursor. Merges skip keys already held.
	Key string
	// Lines is what the item draws as, oldest first. With none, the item
	// draws as a dim "· no output" line.
	Lines []string
	// Data is carried to ActivatedMsg for the screen's inspector.
	Data any
	// Hole marks a position the source has no item for yet (an AWX event
	// saved out of order). It draws as a placeholder and is not resident.
	Hole bool
	// Mark labels the item in the gutter: its timestamp, id or counter —
	// what a reader would quote to find it again. Without one, Seekable
	// items show their position; Anchored items, which have none, show
	// nothing.
	//
	// A timestamp here is always the source's own — Elasticsearch's
	// @timestamp, the kubelet's line time — never when the client fetched
	// it. When the line's text already carries the time the application
	// wrote, leave Mark empty rather than show two times for one event.
	Mark string
}

// Answer names the query a set of items answers. See table.Answer.
type Answer = remoteview.Answer

// ViewportChangedMsg reports what is on screen and what is held. Feed the
// visible range to source.Model.Viewport and the held range to SetHeld —
// or, for a span, ToOlder/ToNewer to source.Anchored.Viewport.
type ViewportChangedMsg struct {
	FirstVisible, LastVisible int
	HeldStart, HeldCount      int
	// ToOlder and ToNewer count resident items between the viewport and
	// each edge of what is held.
	ToOlder, ToNewer int
}

// QueryChangedMsg reports a committed filter: enter, esc or blur, never per
// keystroke. Hand it to the source's SetQuery.
type QueryChangedMsg struct {
	Raw   string
	Terms []query.Term
}

// FindMsg asks for the next match beyond what is held — n or N ran out of
// resident items. Hand it to the source's Find; answer with Found.
type FindMsg struct {
	Term  string
	Newer bool
	// From is the logical index to search beyond (Seekable). Anchored
	// sources start from the edge in the requested direction.
	From int
	// FromKey is the key of the item at From — what an API that searches
	// by its own ids (AWX's counter__gt) needs, since positions stop
	// matching ids once a filter is applied.
	FromKey string
	Token   focus.Token
}

// ActivatedMsg reports enter or a double click on an item.
type ActivatedMsg struct {
	Key   string
	Data  any
	Token focus.Token
}

// Options configures an eventlog. Start from theme.Eventlog().
type Options struct {
	Width, Height int
	// Title labels the pane's border. Defaults to "events".
	Title string
	// Anchored makes the eventlog hold a span of Anchored data (Append /
	// Prepend) rather than a range at logical offsets (SetPage).
	Anchored bool
	// Filterable adds a filter that narrows the query (f by default).
	Filterable bool
	// Searchable adds search (/ by default), which jumps to matches.
	Searchable bool
	// MaxItems caps what is resident. Zero means DefaultMaxItems.
	MaxItems int
	// Placeholder is drawn for an item not yet fetched. Defaults to the
	// glyph set's placeholder.
	Placeholder string

	MatchStyle    lipgloss.Style
	SelectedStyle lipgloss.Style
	CellStyle     lipgloss.Style
	// GutterStyle draws the gutter of positions or marks down the left.
	GutterStyle lipgloss.Style
	// NoGutter hides the gutter. It is on by default: in a long timeline
	// the reader needs somewhere fixed to measure the cursor against.
	NoGutter bool
	// NewStyle draws the numbers of items that just loaded beside ones
	// already on screen — a page reached by scrolling or by a search, or
	// new items of Growing data — so the reader can tell what came in from
	// what was there. A load that replaces everything marks nothing.
	NewStyle lipgloss.Style
	// NewFor is how long loaded items stay marked. Zero means 2s;
	// negative never marks.
	NewFor time.Duration

	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	Glyphs         glyph.Set
	SlotBrackets   pane.SlotBracketStyle
	HScrollbar     bool
	SpinnerStyle   lipgloss.Style
	LoadingLabel   string

	// Filter styles both the filter and the search field.
	Filter filter.Options
	Keys   Keys
}

// Keys is the eventlog's keymap. Horizontal scroll lives on Pane.
type Keys struct {
	Up, Down, Top, Bottom, HalfUp, HalfDown key.Binding
	Open                                    key.Binding
	Filter, Search, NextMatch, PrevMatch    key.Binding
	Pane                                    pane.Keys
}

// DefaultKeys returns the eventlog's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:       key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "newest · follow")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		Open:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Filter:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		NextMatch: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
		Pane:      pane.DefaultKeys(),
	}
}

func (k *Keys) fillDefaults() {
	d := DefaultKeys()
	fill := func(b *key.Binding, def key.Binding) {
		if len(b.Keys()) == 0 {
			*b = def
		}
	}
	fill(&k.Up, d.Up)
	fill(&k.Down, d.Down)
	fill(&k.Top, d.Top)
	fill(&k.Bottom, d.Bottom)
	fill(&k.HalfUp, d.HalfUp)
	fill(&k.HalfDown, d.HalfDown)
	fill(&k.Open, d.Open)
	fill(&k.Filter, d.Filter)
	fill(&k.Search, d.Search)
	fill(&k.NextMatch, d.NextMatch)
	fill(&k.PrevMatch, d.PrevMatch)
	k.Pane.FillDefaults()
}

// Model is the eventlog. Embed by value; mutate through the setters.
type Model struct {
	glyphs glyph.Set
	token  focus.Token
	keys   Keys

	rng      remoteview.Range[Item]
	st       remoteview.Status
	maxItems int

	cursor int
	top    int

	following bool
	growing   bool
	// unseenFrom is the count when follow was left; items past it are new.
	unseenFrom   int
	probeNew     int
	probeMore    bool
	pollsFailing bool

	body       pane.Pane
	filterable bool
	filter     filter.Model
	qRaw       string
	searchable bool
	search     filter.Model
	term       string

	finding   bool
	findNewer bool
	// landAt is a find's hit whose page hasn't arrived: the cursor moves
	// there only once the match can be seen, or -1.
	landAt int

	newStyle lipgloss.Style
	newFor   time.Duration
	// arrivals are batches of items marked new, oldest first: each fades
	// NewFor after it landed, on its own clock.
	arrivals   []arrival
	freshSeq   int
	freshArmd  bool
	noMore     int // 0 none, +1 nothing newer, -1 nothing older
	matchTotal int
	expectKey  string
	// replace makes the next page replace the span rather than merge
	// into it — set by Reanchor.
	replace bool

	placeholder   string
	gutter        bool
	gutterStyle   lipgloss.Style
	matchStyle    lipgloss.Style
	selectedStyle lipgloss.Style
	cellStyle     lipgloss.Style
	ruleActive    lipgloss.Style
	ruleInactive  lipgloss.Style

	baseTitle string
	loadCmd   tea.Cmd
	vp        ViewportChangedMsg
	vpPending bool
	qPending  bool
	pending   []tea.Msg
}

// New constructs an eventlog.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "events"
	}
	if opts.MaxItems <= 0 {
		opts.MaxItems = DefaultMaxItems
	}
	opts.Keys.fillDefaults()
	m := Model{
		glyphs:        opts.Glyphs.Resolve(),
		token:         focus.NewToken(),
		keys:          opts.Keys,
		maxItems:      opts.MaxItems,
		following:     true,
		filterable:    opts.Filterable,
		searchable:    opts.Searchable,
		placeholder:   opts.Placeholder,
		gutter:        !opts.NoGutter,
		gutterStyle:   opts.GutterStyle,
		matchStyle:    opts.MatchStyle,
		selectedStyle: opts.SelectedStyle,
		cellStyle:     opts.CellStyle,
		baseTitle:     opts.Title,
		matchTotal:    -1,
		landAt:        -1,
		newStyle:      opts.NewStyle,
		newFor:        opts.NewFor,
	}
	if m.placeholder == "" {
		m.placeholder = m.glyphs.Placeholder
	}
	switch {
	case m.newFor == 0:
		m.newFor = 2 * time.Second
	case m.newFor < 0:
		m.newFor = 0
	}
	if opts.Anchored {
		m.rng = remoteview.NewSpan[Item]()
	} else {
		m.rng = remoteview.NewRange[Item]()
	}
	if m.filterable {
		fo := opts.Filter
		fo.Prompt = "filter › "
		m.filter = filter.New(fo)
	}
	if m.searchable {
		m.search = filter.New(opts.Filter)
	}
	m.ruleActive = lipgloss.NewStyle().Foreground(opts.ActiveColor)
	m.ruleInactive = lipgloss.NewStyle().Foreground(opts.InactiveColor)
	m.body = pane.New(pane.Options{
		Width:          opts.Width,
		Height:         opts.Height,
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
	m.refresh()
	return m
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// ---- data in ------------------------------------------------------------

// SetPage installs Seekable items at logical offset of a set total long,
// answering the query named by answered. A page touching what is held
// extends it; a far one replaces it. When the committed query's first
// answer lands, the cursor goes to its anchor: the newest item while
// following, else the top.
func (m *Model) SetPage(items []Item, offset, total int, answered Answer) {
	keys := keysOf(items)
	had := m.st.HasAnswer
	start, n := m.rng.Held()
	fresh := m.st.SetAnswer(answered, m.committed())
	if fresh {
		m.rng.Replace(items, keys, offset, total)
	} else {
		m.rng.Merge(items, keys, offset, total)
	}
	// Mark what the page added beside what was already held, so the
	// reader can tell the rows that just loaded from the ones that were
	// there. A page that replaced everything marks nothing: every row
	// would light up and say nothing.
	if s2, n2 := m.rng.Held(); had && !fresh && n > 0 && s2 <= start && s2+n2 >= start+n {
		if s2 < start {
			m.markNew(s2, start)
		}
		if s2+n2 > start+n {
			m.markNew(start+n, s2+n2)
		}
	}
	m.afterData(fresh, 0)
}

// Append adds items at the newer edge of a span, oldest first, merged by
// key: known keys are skipped and a late item lands where the page puts it.
func (m *Model) Append(items []Item, answered Answer) {
	m.addToSpan(items, answered, false)
}

// Prepend adds items at the older edge of a span, oldest first.
func (m *Model) Prepend(items []Item, answered Answer) {
	m.addToSpan(items, answered, true)
}

func (m *Model) addToSpan(items []Item, answered Answer, front bool) {
	had := m.st.HasAnswer
	fresh := m.st.SetAnswer(answered, m.committed()) || m.replace
	if fresh {
		m.rng.Clear()
		m.arrivals = nil
		m.cursor, m.top = 0, 0
		m.replace, had = false, false
	}
	oldLen := m.rng.Len()
	var before int
	if front {
		before = m.rng.Prepend(items, keysOf(items), m.cursor)
		added := m.rng.Len() - oldLen
		for j := range m.arrivals {
			m.arrivals[j].from += added
			m.arrivals[j].to += added
		}
		if had && oldLen > 0 && added > 0 {
			m.markNew(0, added)
		}
	} else {
		before = m.rng.Append(items, keysOf(items), m.cursor)
		if had && oldLen > 0 && m.rng.Len() > oldLen {
			m.markNew(oldLen, m.rng.Len())
		}
	}
	if fresh && m.expectKey == "" {
		// A new answer lands at its anchor. A first page arriving at the
		// older edge was paged back from the newest item (the Newest
		// anchor): land there, following. One arriving at the newer edge
		// began at the oldest.
		m.following = front
	}
	// An empty span's first items land as a fresh answer does: where
	// follow says, not shifted by everything inserted before index 0.
	m.afterData(fresh || oldLen == 0, before)
}

// Reanchor says the source is re-anchoring (source.Anchored.SetAnchor):
// the next page replaces the span instead of extending it. What is on
// screen stays until then, so a jump to a search hit never blanks.
func (m *Model) Reanchor() { m.replace = true }

// SetMore sets whether each edge of a span has more beyond it — from the
// source after each page.
func (m *Model) SetMore(older, newer bool) {
	m.rng.SetMore(older, newer)
	m.refresh()
}

func (m *Model) afterData(fresh bool, shifted int) {
	m.pollsFailing = false
	m.probeNew = 0
	switch {
	case m.landAt >= 0 && m.resident(m.landAt):
		m.cursor, m.landAt = m.landAt, -1
		m.finding, m.following = false, false
		m.park()
	case m.expectKey != "" && m.rng.IndexOf(m.expectKey) >= 0:
		m.cursor = m.rng.IndexOf(m.expectKey)
		m.expectKey = ""
		m.following = false
		m.park()
	case fresh && m.following:
		m.cursor = max(0, m.rng.Count()-1)
	case fresh:
		m.cursor, m.top = 0, 0
	default:
		m.cursor += shifted
		m.top += shifted
	}
	if m.following {
		m.cursor = max(0, m.rng.Count()-1)
	}
	// Trim against where the view will be, not where it was.
	m.adjustTop()
	m.trim()
	if m.following {
		m.cursor = max(0, m.rng.Count()-1)
	}
	m.refresh()
}

func (m *Model) trim() {
	first, last := m.visibleRange()
	if front := m.rng.Trim(m.maxItems, first, last); front > 0 && m.rng.IsSpan() {
		m.cursor = max(0, m.cursor-front)
		m.top = max(0, m.top-front)
	}
}

// SetFailed tells the eventlog the fetch for q failed. It takes effect for
// the committed query only. While following, a failure is a failed poll
// and the border says so until the next success.
func (m *Model) SetFailed(q Answer) {
	if q.Normalized() != m.committed() {
		return
	}
	if m.following && m.st.HasAnswer && !m.Stale() && !m.missing().OK {
		m.pollsFailing = true
	} else {
		m.st.SetFailed(q, m.committed(), m.missing())
	}
	m.finding, m.landAt = false, -1
	m.refresh()
}

// SetGrowing says whether items are still arriving at the newest end.
func (m *Model) SetGrowing(b bool) {
	m.growing = b
	m.refresh()
}

// SetNew reports what a probe found while the user is not following: n
// items newer than those held, and whether there are more than that.
func (m *Model) SetNew(n int, more bool) {
	m.probeNew, m.probeMore = n, more
	m.refresh()
}

// Found answers a FindMsg for Seekable data: the match is at logical index
// at, or there is none when ok is false. The cursor goes there and the
// source fetches around it.
func (m *Model) Found(ok bool, at int) {
	m.finding = false
	if !ok {
		m.noMore = m.dirSign()
		m.refresh()
		return
	}
	m.following = false
	if !m.resident(at) {
		// Land when the match can be seen: moving the cursor onto a
		// placeholder shows a hit nobody can read, and the next n moves
		// on before its line appears. The border keeps "searching…".
		m.finding, m.landAt = true, at
		m.refresh()
		return
	}
	m.cursor = max(0, at)
	m.park()
	m.refresh()
}

// arrival is a run of items that loaded together, beside ones already
// held.
type arrival struct {
	from, to int
	at       time.Time
}

// markNew marks items [from, to) as just loaded.
func (m *Model) markNew(from, to int) {
	if m.newFor <= 0 || to <= from {
		return
	}
	m.arrivals = append(m.arrivals, arrival{from: from, to: to, at: time.Now()})
}

// expireArrivals drops runs older than NewFor.
func (m *Model) expireArrivals() {
	cut := 0
	for cut < len(m.arrivals) && time.Since(m.arrivals[cut].at) >= m.newFor {
		cut++
	}
	m.arrivals = m.arrivals[cut:]
}

// isNew reports whether item i loaded within the last NewFor.
func (m Model) isNew(i int) bool {
	for _, a := range m.arrivals {
		if i >= a.from && i < a.to && time.Since(a.at) < m.newFor {
			return true
		}
	}
	return false
}

type newExpiredMsg struct {
	token focus.Token
	seq   int
}

// resident reports whether item i is loaded and drawable.
func (m Model) resident(i int) bool {
	it, ok := m.rng.At(i)
	return ok && !it.Hole
}

// FoundAt answers a FindMsg for Anchored data. Re-anchor the source at the
// match's cursor; the cursor lands on the item keyed k when it arrives.
func (m *Model) FoundAt(ok bool, k string) {
	m.finding = false
	if !ok {
		m.noMore = m.dirSign()
		m.refresh()
		return
	}
	m.following = false
	m.expectKey = k
	m.refresh()
}

func (m Model) dirSign() int {
	if m.findNewer {
		return 1
	}
	return -1
}

// SetMatchTotal supplies an exact match count from a source that can
// count; -1 clears it.
func (m *Model) SetMatchTotal(n int) {
	m.matchTotal = n
	m.refresh()
}

// ---- state out ----------------------------------------------------------

// Edges returns the cursors of the oldest and newest items held.
func (m Model) Edges() (older, newer string) { return m.rng.Edges() }

// Stale reports whether the items answer a query other than the committed
// one. Stale items are real records of the previous query.
func (m Model) Stale() bool { return m.st.Stale(m.committed()) }

// Failed reports whether the committed query's fetch failed.
func (m Model) Failed() bool { return m.st.Failed }

// Answered returns the query the items answer.
func (m Model) Answered() (Answer, bool) { return m.st.Answer, m.st.HasAnswer }

// Committed returns the query the source was last asked for.
func (m Model) Committed() Answer { return m.committed() }

// Following reports whether the view is on the newest item.
func (m Model) Following() bool { return m.following }

// SetFollow pins the view to the newest item, or releases it. Before the
// first answer it decides where that answer lands: following (the default)
// lands on the newest item, not following on the first — the place to
// start reading a job that has already finished.
func (m *Model) SetFollow(b bool) {
	m.following = b
	if b {
		m.cursor = max(0, m.rng.Count()-1)
	}
	m.refresh()
}

// Cursor returns the cursor's logical index.
func (m Model) Cursor() int { return m.cursor }

// SetCursor moves the cursor, leaving follow unless it lands on the newest.
func (m *Model) SetCursor(i int) {
	m.cursor = max(0, min(i, m.rng.Count()-1))
	m.following = m.rng.Count() > 0 && m.cursor >= m.rng.Count()-1
	m.refresh()
}

// Selected returns the item under the cursor, and false when it isn't
// resident.
func (m Model) Selected() (Item, bool) {
	it, ok := m.rng.At(m.cursor)
	if !ok || it.Hole {
		return Item{}, false
	}
	return it, true
}

// Items returns the resident items, oldest first, with where they start.
func (m Model) Items() (start int, items []Item) {
	start, n := m.rng.Held()
	for i := start; i < start+n; i++ {
		it, _ := m.rng.At(i)
		items = append(items, it)
	}
	return start, items
}

// Value returns the filter text.
func (m Model) Value() string {
	if !m.filterable {
		return ""
	}
	return m.filter.Value()
}

// SetValue restores the filter text silently, as a SetTheme rebuild does.
func (m *Model) SetValue(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.qRaw = strings.TrimSpace(s)
	m.refresh()
}

// Term returns the search term.
func (m Model) Term() string { return m.term }

// SetTerm restores the search term.
func (m *Model) SetTerm(s string) {
	if !m.searchable {
		return
	}
	m.search.SetValue(s)
	m.term = strings.ToLower(strings.TrimSpace(s))
	m.refresh()
}

// Query returns the committed filter as a QueryChangedMsg, for the first
// fetch.
func (m Model) Query() QueryChangedMsg {
	msg := QueryChangedMsg{Raw: m.qRaw}
	if m.qRaw != "" {
		msg.Terms = query.Parse(m.qRaw, nil)
	}
	return msg
}

func (m Model) committed() Answer { return Answer{Raw: m.qRaw}.Normalized() }

func keysOf(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}

// ---- update -------------------------------------------------------------

// Update handles keys, mouse and the eventlog's own timers.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if e, ok := msg.(newExpiredMsg); ok {
		if e.token != m.token || e.seq != m.freshSeq {
			return m, nil
		}
		m.freshArmd = false
		m.expireArrivals()
		m.refresh()
		return m, m.flush()
	}
	if handled, advanced := m.st.Handle(m.token, msg); handled {
		if advanced {
			m.refresh()
		}
		return m, m.flush()
	}
	if mm, ok := msg.(mouse.Msg); ok {
		return m.handleMouse(mm)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, tea.Batch(cmd, m.flush())
	}
	if m.filterable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if !m.filter.Focused() {
			m.commitFilter(km.String() == "enter")
		}
		m.refresh()
		return m, tea.Batch(cmd, m.flush())
	}
	if m.searchable && m.search.Focused() {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.term = strings.ToLower(strings.TrimSpace(m.search.Value()))
		m.noMore = 0
		m.refresh()
		return m, tea.Batch(cmd, m.flush())
	}
	m.noMore = 0
	switch {
	case m.filterable && key.Matches(km, m.keys.Filter):
		return m, tea.Batch(m.FocusFilter(), m.flush())
	case m.searchable && key.Matches(km, m.keys.Search):
		return m, tea.Batch(m.FocusSearch(), m.flush())
	case m.searchable && m.term != "" && key.Matches(km, m.keys.NextMatch):
		m.jump(true)
	case m.searchable && m.term != "" && key.Matches(km, m.keys.PrevMatch):
		m.jump(false)
	case key.Matches(km, m.keys.Up):
		m.move(-1)
	case key.Matches(km, m.keys.Down):
		m.move(1)
	case key.Matches(km, m.keys.HalfUp):
		m.move(-max(1, m.body.VisibleRows()/2))
	case key.Matches(km, m.keys.HalfDown):
		m.move(max(1, m.body.VisibleRows()/2))
	case key.Matches(km, m.keys.Top):
		m.following = false
		m.cursor, m.top = 0, 0
		m.markLeftFollow()
		m.refresh()
	case key.Matches(km, m.keys.Bottom):
		if m.growing && !m.following && m.unseen() > 0 && !m.rng.IsSpan() {
			m.markNew(m.unseenFrom, m.rng.Count())
		}
		m.following = true
		m.cursor = max(0, m.rng.Count()-1)
		m.refresh()
	case key.Matches(km, m.keys.Open):
		return m, tea.Batch(m.activate(), m.flush())
	default:
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		m.refresh()
		return m, tea.Batch(cmd, m.flush())
	}
	return m, m.flush()
}

// move steps the cursor by n items. Moving off the newest item leaves
// follow.
func (m *Model) move(n int) {
	count := m.rng.Count()
	if count == 0 {
		return
	}
	m.cursor = max(0, min(m.cursor+n, count-1))
	was := m.following
	m.following = m.cursor >= count-1
	if was && !m.following {
		m.markLeftFollow()
	}
	m.refresh()
}

func (m *Model) markLeftFollow() { m.unseenFrom = m.rng.Count() }

// jump finds the next match among resident items, or asks the source.
// While the cursor sits on an item whose page is still on its way — where
// the last find landed — it waits: asking again would only find the same
// hit, and would hold that page back.
func (m *Model) jump(newer bool) {
	if m.waitingToJump() {
		// The hit being fetched is the answer to this press too: asking
		// again would find the same hit and hold its page back, and moving
		// on once it lands would skip the match the user was waiting for.
		return
	}
	step := 1
	if !newer {
		step = -1
	}
	start, n := m.rng.Held()
	for i := m.cursor + step; i >= start && i < start+n; i += step {
		if it, ok := m.rng.At(i); ok && !it.Hole && m.matches(it) {
			m.cursor = i
			m.following = false
			m.park()
			m.refresh()
			return
		}
	}
	more := false
	if m.rng.IsSpan() {
		older, newerMore := m.rng.More()
		more = (newer && newerMore) || (!newer && older)
	} else {
		more = (newer && start+n < m.rng.Count()) || (!newer && start > 0)
	}
	if !more {
		m.noMore = map[bool]int{true: 1, false: -1}[newer]
		m.refresh()
		return
	}
	// Search beyond whichever is further along: the edge of what is held,
	// or the cursor — which a previous find may have put past that edge,
	// before its page has arrived. Otherwise the same match comes back.
	from := min(start, m.cursor)
	if newer {
		from = max(start+n-1, m.cursor)
	}
	m.finding, m.findNewer = true, newer
	msg := FindMsg{Term: m.term, Newer: newer, From: from, FromKey: m.rng.KeyAt(from), Token: m.token}
	m.pending = append(m.pending, msg)
	m.refresh()
}

// waitingToJump reports whether a find, or the page its hit is on, is
// still on the way.
func (m Model) waitingToJump() bool {
	if m.finding {
		return true
	}
	_, ok := m.rng.At(m.cursor)
	return !ok && m.rng.Count() > 0
}

func (m Model) matches(it Item) bool {
	for _, l := range it.Lines {
		if strings.Contains(strings.ToLower(xansi.Strip(l)), m.term) {
			return true
		}
	}
	return false
}

// commitFilter reports the filter when its text changed, or when it is
// committed again on a failed query — which is how the user retries.
func (m *Model) commitFilter(enter bool) {
	raw := strings.TrimSpace(m.filter.Value())
	if raw == m.qRaw && !(enter && m.st.Failed) {
		return
	}
	m.qRaw = raw
	m.st.Failed = false
	m.finding, m.landAt = false, -1 // a new query supersedes any find
	m.qPending = true
}

func (m *Model) activate() tea.Cmd {
	it, ok := m.Selected()
	if !ok {
		return nil
	}
	msg := ActivatedMsg{Key: it.Key, Data: it.Data, Token: m.token}
	return func() tea.Msg { return msg }
}

// IsActivate reports whether msg opens the selection: enter, or this
// eventlog's own ActivatedMsg.
func (m Model) IsActivate(msg tea.Msg) bool {
	switch v := msg.(type) {
	case ActivatedMsg:
		return v.Token == m.token
	case tea.KeyMsg:
		return !m.IsCapturingKeys() && key.Matches(v, m.keys.Open)
	}
	return false
}

// flush emits whatever the last change produced: a viewport report, a
// committed query, a find, a spinner tick.
func (m *Model) flush() tea.Cmd {
	var cmds []tea.Cmd
	if m.vpPending {
		m.vpPending = false
		vp := m.vp
		cmds = append(cmds, func() tea.Msg { return vp })
	}
	if m.qPending {
		m.qPending = false
		q := m.Query()
		cmds = append(cmds, func() tea.Msg { return q })
	}
	for _, p := range m.pending {
		p := p
		cmds = append(cmds, func() tea.Msg { return p })
	}
	m.pending = nil
	if m.loadCmd != nil {
		cmds = append(cmds, m.loadCmd)
		m.loadCmd = nil
	}
	cmds = append(cmds, m.st.Arm(m.token, m.waiting()))
	if len(m.arrivals) > 0 && !m.freshArmd {
		// One timer at a time, for whichever batch fades first.
		m.freshArmd = true
		m.freshSeq++
		msg := newExpiredMsg{token: m.token, seq: m.freshSeq}
		wait := max(time.Millisecond, m.newFor-time.Since(m.arrivals[0].at))
		cmds = append(cmds, tick.After(wait, func(time.Time) tea.Msg { return msg }))
	}
	return tea.Batch(cmds...)
}

func (m Model) waiting() bool {
	return m.finding || m.st.Waiting(m.committed(), m.missing())
}

// ---- layout & rendering -------------------------------------------------

// SetRect places the eventlog.
func (m *Model) SetRect(r geom.Rect) {
	m.body.SetRect(r)
	if m.hasHeader() {
		m.placeHeader(r)
		m.body.SetHeader(m.header())
		m.placeHeader(r)
	}
	m.refresh()
}

func (m Model) hasHeader() bool { return m.filterable || m.searchable }

func (m *Model) placeHeader(r geom.Rect) {
	inner := m.body.ContentRect()
	row := geom.Rect{X: inner.X, Y: m.body.Rect().Y + 1, W: inner.W, H: 1, Gen: r.Gen}
	if m.filterable {
		m.filter.SetInlineRect(row)
	}
	if m.searchable {
		m.search.SetInlineRect(row)
	}
}

// header is one input row plus a rule: whichever field has input, or a
// summary of both when neither does.
func (m Model) header() string {
	w := m.body.ContentRect().W
	if w <= 0 {
		return ""
	}
	rule := m.ruleInactive
	var row string
	switch {
	case m.filterable && m.filter.Focused():
		row, rule = m.filter.InlineView(), m.ruleActive
	case m.searchable && m.search.Focused():
		row, rule = m.search.InlineView(), m.ruleActive
	default:
		var parts []string
		if m.filterable {
			f := m.qRaw
			if f == "" {
				f = "—"
			}
			parts = append(parts, "filter › "+f)
		}
		if m.searchable && m.term != "" {
			parts = append(parts, "/ "+m.term)
		}
		row = strings.Join(parts, "   ")
	}
	return xansi.Truncate(row, w, "…") + "\n" + rule.Render(strings.Repeat(m.glyphs.Rule, w))
}

// height is how many lines item i draws as — never fewer than one.
func (m Model) height(i int) int {
	it, ok := m.rng.At(i)
	if !ok || it.Hole {
		return 1
	}
	return max(1, len(it.Lines))
}

// noOutput is drawn for an item with no lines.
const noOutput = "· no output"

// visibleRange reports the first and last items drawn.
func (m Model) visibleRange() (first, last int) {
	rows := max(1, m.body.VisibleRows())
	last = m.top
	used := 0
	for i := m.top; i < m.rng.Count(); i++ {
		used += m.height(i)
		last = i
		if used >= rows {
			break
		}
	}
	return m.top, last
}

// park puts the cursor's item about a third of the way down the screen —
// where a search jump lands, so what follows a match is on screen as well
// as what led to it.
func (m *Model) park() {
	want := max(1, m.body.VisibleRows()) / 3
	top, used := m.cursor, 0
	for i := m.cursor - 1; i >= 0; i-- {
		used += m.height(i)
		if used > want {
			break
		}
		top = i
	}
	m.top = top
}

// adjustTop keeps the cursor on screen; while following, the newest item
// sits at the bottom.
func (m *Model) adjustTop() {
	count := m.rng.Count()
	if count == 0 {
		m.top, m.cursor = 0, 0
		return
	}
	m.cursor = max(0, min(m.cursor, count-1))
	rows := max(1, m.body.VisibleRows())
	if m.cursor < m.top {
		m.top = m.cursor
	}
	// Walk back from the cursor to find the highest top that still shows it.
	used, lowest := 0, m.cursor
	for i := m.cursor; i >= 0; i-- {
		used += m.height(i)
		if used > rows {
			break
		}
		lowest = i
	}
	if m.following || m.top < lowest {
		m.top = max(m.top, lowest)
		if m.following {
			m.top = lowest
		}
	}
	m.top = max(0, min(m.top, count-1))
}

func (m Model) missing() remoteview.Missing {
	var miss remoteview.Missing
	if !m.st.HasAnswer {
		return miss
	}
	first, last := m.visibleRange()
	for i := first; i <= last && i < m.rng.Count(); i++ {
		if it, ok := m.rng.At(i); ok && !it.Hole {
			continue
		}
		if !miss.OK {
			miss.First, miss.OK = i, true
		}
		miss.Last = i
	}
	return miss
}

func (m *Model) refresh() {
	m.adjustTop()
	if m.hasHeader() {
		m.body.SetHeader(m.header())
	}
	if cmd := m.body.SetLoading(m.st.Loading()); cmd != nil {
		m.loadCmd = cmd
	}
	m.body.SetContent(m.render())
	first, last := m.visibleRange()
	m.body.SetVirtualScroll(max(1, m.rng.Count()), max(1, last-first+1), first)

	title := m.baseTitle
	if s := m.suffix(); s != "" {
		title += " — " + s
	}
	m.body.SetTitle(title)
	m.body.SetBottomLeft(m.status())
	if n := m.rng.Count(); n > 0 {
		total := fmt.Sprint(n)
		if m.rng.IsSpan() || m.rng.Total() < 0 {
			total += "+"
		}
		m.body.SetBottomRight(fmt.Sprintf("%d / %s", m.cursor+1, total))
	} else {
		m.body.SetBottomRight("")
	}
	m.noteViewport(first, last)
}

func (m Model) suffix() string {
	if m.st.Failed && !m.st.HasAnswer {
		return m.st.Suffix(m.committed(), m.missing(), "items", "", "")
	}
	if m.rng.IsSpan() && !m.Stale() && m.st.HasAnswer {
		first, last := m.visibleRange()
		older, newer := m.rng.More()
		switch {
		case older && first == 0:
			return m.st.Spin() + " loading older…"
		case newer && !m.growing && last >= m.rng.Count()-1:
			return m.st.Spin() + " loading newer…"
		}
		return ""
	}
	return m.st.Suffix(m.committed(), m.missing(), "items", "", "")
}

func (m Model) status() string {
	var parts []string
	switch {
	case m.growing && m.following && m.pollsFailing:
		parts = append(parts, "FOLLOWING · ✗ polls failing")
	case m.growing && m.following:
		parts = append(parts, "FOLLOWING")
	case m.growing:
		if n := m.unseen(); n > 0 {
			more := ""
			if m.probeMore {
				more = "+"
			}
			parts = append(parts, fmt.Sprintf("↓ %d%s new", n, more))
		} else {
			parts = append(parts, "PAUSED")
		}
	}
	switch {
	case m.finding && m.findNewer:
		parts = append(parts, m.st.Spin()+" searching newer…")
	case m.finding:
		parts = append(parts, m.st.Spin()+" searching older…")
	case m.noMore > 0:
		parts = append(parts, "no more matches ↓")
	case m.noMore < 0:
		parts = append(parts, "no more matches ↑")
	case m.term != "":
		parts = append(parts, m.matchLabel())
	}
	return strings.Join(parts, " · ")
}

// unseen counts items that arrived since follow was left.
func (m Model) unseen() int {
	if m.rng.IsSpan() {
		return m.probeNew
	}
	return max(0, m.rng.Count()-m.unseenFrom)
}

func (m Model) matchLabel() string {
	start, n := m.rng.Held()
	idx, total := 0, 0
	for i := start; i < start+n; i++ {
		if it, ok := m.rng.At(i); ok && !it.Hole && m.matches(it) {
			total++
			if i <= m.cursor {
				idx = total
			}
		}
	}
	if m.matchTotal >= 0 {
		return fmt.Sprintf("match %d of %d", idx, m.matchTotal)
	}
	beyond := ""
	if m.rng.IsSpan() {
		if o, nw := m.rng.More(); o || nw {
			beyond = "+"
		}
	} else if start > 0 || start+n < m.rng.Count() {
		beyond = "+"
	}
	return fmt.Sprintf("match %d of %d%s here", idx, total, beyond)
}

func (m *Model) noteViewport(first, last int) {
	start, n := m.rng.Held()
	vp := ViewportChangedMsg{
		FirstVisible: first, LastVisible: last,
		HeldStart: start, HeldCount: n,
		ToOlder: max(0, first-start), ToNewer: max(0, start+n-1-last),
	}
	if vp != m.vp {
		m.vp, m.vpPending = vp, true
	}
}

func (m Model) render() string {
	if m.st.Failed && !m.st.HasAnswer {
		c := m.body.ContentRect()
		return lipgloss.Place(max(1, c.W), max(1, c.H), lipgloss.Center, lipgloss.Center, "✗ failed to load")
	}
	stale := m.Stale()
	sel, cell := m.selectedStyle, m.cellStyle
	if stale {
		sel, cell = sel.Faint(true), cell.Faint(true)
	}
	rows := max(1, m.body.VisibleRows())
	width := m.gutterWidth(rows)
	var out []string
	for i := m.top; i < m.rng.Count() && len(out) < rows; i++ {
		it, ok := m.rng.At(i)
		lines := it.Lines
		style := cell
		if i == m.cursor {
			style = sel
		}
		switch {
		case !ok || it.Hole:
			lines = []string{m.placeholder}
		case len(lines) == 0:
			lines = []string{noOutput}
			style = style.Faint(true)
		}
		for n, l := range lines {
			if len(out) >= rows {
				break
			}
			row := style.Render(m.highlight(l))
			if width > 0 {
				label := ""
				if n == 0 {
					label = m.label(i)
				}
				row = m.gutterCell(label, width, n == 0 && i == m.cursor, m.isNew(i)) + row
			}
			out = append(out, row)
		}
	}
	return strings.Join(out, "\n")
}

// label is what the gutter shows for item i: its Mark, else its position
// when the data has positions, else nothing.
func (m Model) label(i int) string {
	if it, ok := m.rng.At(i); ok && it.Mark != "" {
		return it.Mark
	}
	if m.rng.IsSpan() {
		return ""
	}
	return fmt.Sprint(i + 1)
}

// gutterWidth is the widest label on screen, or 0 when the gutter is off
// or nothing on screen has a label.
func (m Model) gutterWidth(rows int) int {
	if !m.gutter {
		return 0
	}
	w, used := 0, 0
	for i := m.top; i < m.rng.Count() && used < rows; i++ {
		w = max(w, xansi.StringWidth(m.label(i)))
		used += max(1, m.height(i))
	}
	return w
}

func (m Model) gutterCell(label string, width int, cursor, fresh bool) string {
	style, mark := m.gutterStyle, " "
	if fresh {
		style = m.newStyle
	}
	if cursor {
		mark = "▌"
	}
	pad := strings.Repeat(" ", max(0, width-xansi.StringWidth(label)))
	return style.Render(pad+label) + m.gutterStyle.Render(mark+"│ ")
}

func (m Model) highlight(line string) string {
	if m.term == "" {
		return line
	}
	plain := xansi.Strip(line)
	lower := strings.ToLower(plain)
	if !strings.Contains(lower, m.term) {
		return line
	}
	var b strings.Builder
	for off := 0; ; {
		i := strings.Index(lower[off:], m.term)
		if i < 0 {
			b.WriteString(plain[off:])
			break
		}
		b.WriteString(plain[off : off+i])
		b.WriteString(m.matchStyle.Render(plain[off+i : off+i+len(m.term)]))
		off += i + len(m.term)
	}
	return b.String()
}

// View renders the eventlog.
func (m Model) View() string { return m.body.View() }

// ---- focus, titles, help ------------------------------------------------

// SetTitle sets the label on the border; while stale or loading an edge,
// a suffix follows it.
func (m *Model) SetTitle(s string) {
	m.baseTitle = s
	m.refresh()
}

// Title returns the label alone.
func (m Model) Title() string { return m.baseTitle }

// Loading reports whether the body shows its Loading spinner.
func (m Model) Loading() bool { return m.body.Loading() }

// FocusToken returns the eventlog's focus identity.
func (m Model) FocusToken() focus.Token { return m.token }

// Focus gives the eventlog the keyboard.
func (m *Model) Focus() tea.Cmd {
	m.body.SetFocused(true)
	return nil
}

// Blur releases the keyboard, clearing both input fields.
func (m *Model) Blur() {
	m.body.SetFocused(false)
	if m.filterable {
		m.filter.Blur()
	}
	if m.searchable {
		m.search.Blur()
	}
	m.refresh()
}

// Focused reports whether the eventlog or one of its fields has input.
func (m Model) Focused() bool { return m.body.Focused() || m.IsCapturingKeys() }

// IsCapturingKeys reports whether a field is swallowing printable keys.
func (m Model) IsCapturingKeys() bool {
	return (m.filterable && m.filter.Focused()) || (m.searchable && m.search.Focused())
}

// FocusFilter moves input to the filter.
func (m *Model) FocusFilter() tea.Cmd {
	if !m.filterable {
		return nil
	}
	if m.searchable {
		m.search.Blur()
	}
	m.body.SetFocused(true)
	cmd := m.filter.Focus()
	m.refresh()
	return cmd
}

// FocusSearch moves input to the search field.
func (m *Model) FocusSearch() tea.Cmd {
	if !m.searchable {
		return nil
	}
	if m.filterable {
		m.filter.Blur()
	}
	m.body.SetFocused(true)
	cmd := m.search.Focus()
	m.refresh()
	return cmd
}

// BlurFilter returns input from either field to the body.
func (m *Model) BlurFilter() {
	if m.filterable && m.filter.Focused() {
		m.filter.Blur()
		m.commitFilter(false)
	}
	if m.searchable {
		m.search.Blur()
	}
	m.body.SetFocused(true)
	m.refresh()
}

// Help returns the bindings the eventlog currently responds to.
func (m Model) Help() []key.Binding { return help.Flatten(m.HelpSections()) }

// HelpSections groups the bindings by what they do (rule 10).
func (m Model) HelpSections() []help.Section {
	switch {
	case m.filterable && m.filter.Focused():
		return help.Sections(help.Group(help.SectionFilter, m.filter.Help()...))
	case m.searchable && m.search.Focused():
		return help.Sections(help.Group(help.SectionSearch, m.search.Help()...))
	}
	var search []key.Binding
	if m.searchable {
		search = append(search, m.keys.Search)
		if m.term != "" {
			search = append(search, m.keys.NextMatch, m.keys.PrevMatch)
		}
	}
	var filt []key.Binding
	if m.filterable {
		filt = append(filt, m.keys.Filter)
	}
	return help.Sections(
		help.Group(help.SectionNavigate, m.keys.Up, m.keys.Down, m.keys.HalfUp, m.keys.HalfDown, m.keys.Top, m.keys.Bottom),
		help.Group(help.SectionView, m.keys.Open,
			key.NewBinding(key.WithKeys("mouse:dblclick"), key.WithHelp("double-click", "open"))),
		help.Group(help.SectionFilter, filt...),
		help.Group(help.SectionSearch, search...),
		help.Group(help.SectionScroll, m.keys.Pane.Left, m.keys.Pane.Right),
	)
}

// ---- mouse --------------------------------------------------------------

func (m Model) handleMouse(e mouse.Msg) (Model, tea.Cmd) {
	if row, ok := m.body.HandleScrollbar(e); ok {
		m.following = false
		m.cursor = row
		m.refresh()
		return m, m.flush()
	}
	if !m.body.Rect().Hit(e.X, e.Y) {
		return m, nil
	}
	if m.hasHeader() && e.Y == m.body.Rect().Y+1 {
		if e.IsPointPress() {
			if m.searchable && !m.filterable {
				return m, tea.Batch(m.FocusSearch(), focus.RequestSelf(m.token), m.flush())
			}
			return m, tea.Batch(m.FocusFilter(), focus.RequestSelf(m.token), m.flush())
		}
		return m, nil
	}
	switch {
	case e.IsWheelUp():
		m.move(-1)
		return m, m.flush()
	case e.IsWheelDown():
		m.move(1)
		return m, m.flush()
	case e.IsPointPress():
		m.BlurFilter()
		cmds := []tea.Cmd{focus.RequestSelf(m.token)}
		if i, ok := m.itemAtY(e.Y); ok {
			m.cursor = i
			m.following = i >= m.rng.Count()-1
			m.refresh()
			if e.IsDoubleClick() {
				cmds = append(cmds, m.activate())
			}
		}
		return m, tea.Batch(append(cmds, m.flush())...)
	}
	return m, nil
}

// itemAtY maps a screen row to the item drawn there.
func (m Model) itemAtY(y int) (int, bool) {
	c := m.body.ContentRect()
	row := y - c.Y
	if row < 0 || row >= c.H {
		return 0, false
	}
	used := 0
	for i := m.top; i < m.rng.Count(); i++ {
		used += m.height(i)
		if row < used {
			return i, true
		}
	}
	return 0, false
}
