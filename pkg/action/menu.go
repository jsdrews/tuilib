package action

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Sizing caps. A menu is a picker, not a document: it takes only the width
// its widest row needs, capped well short of the full pane so the rows it
// acts on stay visible behind it.
const (
	menuMinWidth  = 20
	menuWidthNum  = 3
	menuWidthDen  = 5
	menuMinHeight = 3
	menuHeightNum = 3
	menuHeightDen = 5

	cursorW = 2 // "▸ " / "  "
	colGap  = 2
)

// Stock reasons the menu fills into Action.Disabled on its own.
const (
	DefaultMultiReason   = "one item at a time"
	DefaultRunningReason = "already running"
)

// Keys is the menu's keymap. Vertical movement follows rule 25 exactly — a
// menu is one more thing that scrolls, not a place to invent a vocabulary.
type Keys struct {
	Up, Down         key.Binding
	Top, Bottom      key.Binding
	HalfUp, HalfDown key.Binding
	Choose           key.Binding
	Cancel           key.Binding
}

// DefaultKeys returns the menu's stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "½ up")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "½ down")),
		Choose:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "run")),
		Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// FillDefaults fills any zero-valued binding with its DefaultKeys counterpart,
// so partial overrides work without restating every field.
func (k *Keys) FillDefaults() {
	d := DefaultKeys()
	for _, p := range []struct {
		dst *key.Binding
		src key.Binding
	}{
		{&k.Up, d.Up}, {&k.Down, d.Down},
		{&k.Top, d.Top}, {&k.Bottom, d.Bottom},
		{&k.HalfUp, d.HalfUp}, {&k.HalfDown, d.HalfDown},
		{&k.Choose, d.Choose}, {&k.Cancel, d.Cancel},
	} {
		if len(p.dst.Keys()) == 0 {
			*p.dst = p.src
		}
	}
}

// Options configures a Menu. Build it from theme.Actions() and set Title and
// Set before passing to New.
type Options struct {
	// Title labels the menu. Defaults to "Actions"; the Set's Target is
	// appended when it has one, so a menu over a multi-selection announces
	// its blast radius on its own border.
	Title string

	// Set is the actions to offer.
	Set Set

	LabelStyle    lipgloss.Style
	DescStyle     lipgloss.Style
	KeyStyle      lipgloss.Style
	SelectedStyle lipgloss.Style
	DisabledStyle lipgloss.Style

	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	// Glyphs are the marks this component draws, plus the scrollbar
	// thumb and track it hands to its pane. Empty fields fall back to
	// glyph.Default.
	Glyphs       glyph.Set
	SlotBrackets pane.SlotBracketStyle

	// MultiReason and RunningReason override the stock text the menu fills
	// into Disabled for a non-Multi action under a multi-selection, and for
	// an Exclusive action already in flight.
	MultiReason   string
	RunningReason string

	Keys Keys
}

// Menu is the action picker: a bordered list of verbs that sizes itself to
// its content and places itself where it was asked to.
//
// It is hosted the way pkg/confirm and pkg/alert are — a host owns show/hide,
// forwards every message while it is up, and matches ChosenMsg / CancelledMsg
// in its own Update. Composition is a bare layout.Sized inside a ZStack: the
// menu treats the rect it is given as outer bounds and draws itself inside
// them, so there is no layout.Center wrapper to keep in sync with its size.
type Menu struct {
	// glyphs is the resolved mark vocabulary this menu draws with.
	glyphs glyph.Set

	pane pane.Pane
	set  Set
	keys Keys

	title   string
	cursor  int
	running map[string]bool

	// outer is the bounds last handed to SetRect; placed is where the box
	// actually landed inside them, which is what it hit-tests against.
	outer, placed geom.Rect

	anchored bool
	ax, ay   int

	labelW, descW, keyW  int
	menuW, menuH         int
	multiRsn, runningRsn string

	labelStyle, descStyle, keyStyle lipgloss.Style
	selectedStyle, disabledStyle    lipgloss.Style
}

// New constructs a menu.
func New(opts Options) Menu {
	if opts.Title == "" {
		opts.Title = "Actions"
	}
	if opts.MultiReason == "" {
		opts.MultiReason = DefaultMultiReason
	}
	if opts.RunningReason == "" {
		opts.RunningReason = DefaultRunningReason
	}
	opts.Keys.FillDefaults()

	p := pane.New(pane.Options{
		Focused:        true,
		Glyphs:         opts.Glyphs,
		SlotBrackets:   opts.SlotBrackets,
		ActiveColor:    opts.ActiveColor,
		InactiveColor:  opts.InactiveColor,
		ActiveBorder:   opts.ActiveBorder,
		InactiveBorder: opts.InactiveBorder,
	})

	m := Menu{
		glyphs:        opts.Glyphs.Resolve(),
		pane:          p,
		set:           opts.Set,
		keys:          opts.Keys,
		title:         opts.Title,
		running:       map[string]bool{},
		multiRsn:      opts.MultiReason,
		runningRsn:    opts.RunningReason,
		labelStyle:    opts.LabelStyle,
		descStyle:     opts.DescStyle,
		keyStyle:      opts.KeyStyle,
		selectedStyle: opts.SelectedStyle,
		disabledStyle: opts.DisabledStyle,
	}
	m.pane.SetTitle(m.paneTitle())
	m.cursor = m.firstEnabled()
	return m
}

// Init satisfies tea.Model — nothing to kick off.
func (m Menu) Init() tea.Cmd { return nil }

// SetActions swaps the offered actions and resets the cursor to the first
// selectable row.
func (m *Menu) SetActions(s Set) {
	m.set = s
	m.cursor = m.firstEnabled()
	m.pane.SetTitle(m.paneTitle())
	m.reflow()
}

// Set returns the actions currently offered.
func (m Menu) Set() Set { return m.set }

// SetRunning tells the menu which actions are in flight, so Exclusive ones
// render as unavailable with a reason instead of silently declining the press.
// Keys come from RunKey.
//
// The menu is told rather than asked because it has no view of the run
// registry — whoever launched the work does. That keeps this component free
// of any dependency on how actions are executed, which is what lets it be
// tested without a goroutine in sight.
func (m *Menu) SetRunning(keys map[string]bool) {
	// Copied, not aliased: a host that keeps mutating the map it handed over
	// would otherwise change what the menu reports between frames, with
	// nothing in the menu's own code to explain it.
	m.running = make(map[string]bool, len(keys))
	for k, v := range keys {
		if v {
			m.running[k] = true
		}
	}
	if m.cursor >= 0 && m.reasonAt(m.cursor) != "" {
		m.cursor = m.firstEnabled()
	}
	m.reflow()
}

// Anchor places the menu's top-left at (x, y) — where a right-click landed —
// clamped so it stays on screen. Call before the frame that shows it.
func (m *Menu) Anchor(x, y int) {
	m.anchored, m.ax, m.ay = true, x, y
	m.reflow()
}

// Center places the menu in the middle of the bounds it is given — the
// keyboard path, where there is no pointer to place it under.
//
// A top-anchored variant was tried and reverted: it sat closer to the rows on
// a tall terminal, but it covered the pane's filter row and its rule, which
// reads as a broken frame. Centering leaves the pane's chrome intact, and a
// modal in the middle of the pane is where a modal is expected to be.
func (m *Menu) Center() {
	m.anchored = false
	m.reflow()
}

// Cursor is the highlighted row, or -1 when nothing is selectable.
func (m Menu) Cursor() int { return m.cursor }

// SetCursor moves the highlight, skipping to the next selectable row when the
// requested one is disabled. Carries state across a theme rebuild (rule 4).
func (m *Menu) SetCursor(i int) {
	if i < 0 || i >= len(m.set.Actions) {
		return
	}
	if m.reasonAt(i) != "" {
		return
	}
	m.cursor = i
	m.reflow()
}

// Selected returns the highlighted action.
func (m Menu) Selected() (Action, bool) {
	if m.cursor < 0 || m.cursor >= len(m.set.Actions) {
		return Action{}, false
	}
	return m.set.Actions[m.cursor], true
}

// Rect is where the box actually landed, for a host that needs to know.
func (m Menu) Rect() geom.Rect { return m.placed }

// SetRect treats r as outer bounds: the menu measures its rows, sizes itself
// against caps derived from r, and places itself inside it.
//
// Same contract as an autosize alert, and for the same reason — a picker whose
// size depends on its content cannot be given a size by the layout engine
// without the caller duplicating the measurement.
func (m *Menu) SetRect(r geom.Rect) {
	m.outer = r
	m.reflow()
}

// reflow re-measures, re-places, and re-renders. Every mutator ends here.
func (m *Menu) reflow() {
	if m.outer.W <= 0 || m.outer.H <= 0 {
		return
	}
	m.measure()
	m.pane.SetRect(m.placed)
	m.pane.SetContent(m.renderRows())
	if m.cursor >= 0 {
		m.pane.EnsureVisible(m.cursor)
	}
}

// measure computes the column widths, the box size, and where it lands.
func (m *Menu) measure() {
	m.labelW, m.descW, m.keyW = 0, 0, 0
	for i := range m.set.Actions {
		a := m.set.Actions[i]
		m.labelW = max(m.labelW, ansi.StringWidth(a.Label))
		if d := m.descAt(i); d != "" {
			m.descW = max(m.descW, ansi.StringWidth(d))
		}
		if k := keyLabel(a); k != "" {
			m.keyW = max(m.keyW, ansi.StringWidth(k))
		}
	}

	maxW := clampCap(m.outer.W, menuWidthNum, menuWidthDen, menuMinWidth)
	maxH := clampCap(m.outer.H, menuHeightNum, menuHeightDen, menuMinHeight)

	// The pane spends two columns on borders and always reserves one more
	// for its vertical scrollbar (see pane.applySize).
	chromeW := 2 + pane.ScrollbarWidth
	maxInner := max(1, maxW-chromeW)
	m.fitColumns(maxInner)

	inner := cursorW + m.labelW
	if m.descW > 0 {
		inner += colGap + m.descW
	}
	if m.keyW > 0 {
		inner += colGap + m.keyW
	}
	// The title sits on the top border and is the one piece of content that
	// is not a row; a box narrower than its own title reads as truncated.
	inner = max(inner, ansi.StringWidth(m.paneTitle())+2)
	inner = min(inner, maxInner)

	m.menuW = min(inner+chromeW, m.outer.W)
	m.menuH = min(max(len(m.set.Actions)+2, menuMinHeight), min(maxH, m.outer.H))

	if m.anchored {
		m.placed = geom.AnchorIn(m.outer, m.ax, m.ay, m.menuW, m.menuH)
		return
	}
	m.placed = geom.CenterIn(m.outer, m.menuW, m.menuH)
}

// fitColumns shrinks the content columns to fit maxInner, giving up the
// description before the label and the key column before either — the label
// is the only part the user cannot do without.
func (m *Menu) fitColumns(maxInner int) {
	avail := maxInner - cursorW
	if m.keyW > 0 {
		if avail-colGap-m.keyW < 1 {
			m.keyW = 0
		} else {
			avail -= colGap + m.keyW
		}
	}
	if avail < 1 {
		avail = 1
	}
	need := m.labelW
	if m.descW > 0 {
		need += colGap + m.descW
	}
	if need <= avail {
		return
	}
	if m.descW > 0 {
		m.descW = max(0, avail-m.labelW-colGap)
	}
	m.labelW = min(m.labelW, max(1, avail))
}

// clampCap returns size*num/den, floored at min and capped at size.
func clampCap(size, num, den, minimum int) int {
	v := size * num / den
	if v < minimum {
		v = minimum
	}
	return min(v, size)
}

// paneTitle is the border label: the title, plus the target when there is one
// so a destructive verb states what it is about to hit.
func (m Menu) paneTitle() string {
	if m.set.Target == "" {
		return m.title
	}
	return m.title + " · " + m.set.Target
}

// descAt is the text in the second column: the disabled reason when there is
// one, otherwise the action's own gloss. The reason wins because a row the
// user cannot pick needs to say why more than it needs to say what.
func (m Menu) descAt(i int) string {
	if r := m.reasonAt(i); r != "" {
		return r
	}
	return m.set.Actions[i].Desc
}

// reasonAt is why row i is unavailable, or "" when it is fine.
func (m Menu) reasonAt(i int) string {
	if i < 0 || i >= len(m.set.Actions) {
		return ""
	}
	return m.reason(m.set.Actions[i])
}

func (m Menu) reason(a Action) string {
	if a.Disabled != "" {
		return a.Disabled
	}
	if m.set.Count > 1 && !a.Multi {
		return m.multiRsn
	}
	if a.Exclusive && m.running[RunKey(a, m.set.Target)] {
		return m.runningRsn
	}
	return ""
}

func keyLabel(a Action) string {
	if len(a.Key.Keys()) == 0 {
		return ""
	}
	if h := a.Key.Help().Key; h != "" {
		return h
	}
	return a.Key.Keys()[0]
}

// View renders the box padded into the outer bounds it was given, so the host
// composes with layout.Sized alone. Rows the box does not cover are emitted
// empty, which ZStack passes straight through to the base layer.
func (m Menu) View() string {
	if m.outer.W <= 0 || m.outer.H <= 0 {
		return m.pane.View()
	}
	dx := max(0, m.placed.X-m.outer.X)
	dy := max(0, m.placed.Y-m.outer.Y)
	indent := strings.Repeat(" ", dx)

	out := make([]string, 0, m.outer.H)
	for i := 0; i < dy && i < m.outer.H; i++ {
		out = append(out, "")
	}
	for _, ln := range strings.Split(m.pane.View(), "\n") {
		if len(out) >= m.outer.H {
			break
		}
		out = append(out, indent+ln)
	}
	for len(out) < m.outer.H {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// renderRows lays out every action against the pane's inner width.
func (m Menu) renderRows() string {
	innerW := m.pane.VisibleWidth()
	if innerW <= 0 || len(m.set.Actions) == 0 {
		return ""
	}
	rows := make([]string, len(m.set.Actions))
	for i := range m.set.Actions {
		rows[i] = m.renderRow(i, innerW)
	}
	return strings.Join(rows, "\n")
}

// seg is one styled run of a row. Rows are assembled as segments so the
// cursor and disabled rows can be styled *whole* while an ordinary row styles
// its columns separately.
//
// That split is not cosmetic. A per-column lipgloss.Render nested inside a
// row-wide one closes the outer style at the first reset — rule 19's hazard,
// which here would drop the highlight halfway across the selected row. Since
// the cursor never lands on a disabled row, no row ever needs both.
type seg struct {
	text  string
	style lipgloss.Style
}

func plain(text string) seg { return seg{text, lipgloss.NewStyle()} }

// styled leaves blank runs unstyled. A column an action did not fill is
// padding, and wrapping padding in a foreground escape emits a colour change
// with nothing to colour — noise in every row of the rendered output, and one
// more escape for ZStack to splice around.
func styled(text string, st lipgloss.Style) seg {
	if strings.TrimSpace(text) == "" {
		return plain(text)
	}
	return seg{text, st}
}

func (m Menu) renderRow(i, innerW int) string {
	a := m.set.Actions[i]
	disabled := m.reasonAt(i) != ""

	prefix := "  "
	if i == m.cursor {
		prefix = m.glyphs.Cursor + " "
	}

	segs := []seg{
		plain(prefix),
		{fit(a.Label, m.labelW), m.labelStyle},
	}
	if m.descW > 0 {
		segs = append(segs, plain(strings.Repeat(" ", colGap)),
			styled(fit(m.descAt(i), m.descW), m.descStyle))
	}

	w := 0
	for _, s := range segs {
		w += ansi.StringWidth(s.text)
	}
	if m.keyW > 0 {
		gap := max(1, innerW-w-m.keyW)
		segs = append(segs, plain(strings.Repeat(" ", gap)),
			styled(padLeft(keyLabel(a), m.keyW), m.keyStyle))
		w += gap + m.keyW
	}
	if w < innerW {
		segs = append(segs, plain(strings.Repeat(" ", innerW-w)))
	}

	if i == m.cursor || disabled {
		var plain strings.Builder
		for _, s := range segs {
			plain.WriteString(s.text)
		}
		if disabled {
			return m.disabledStyle.Render(plain.String())
		}
		return m.selectedStyle.Render(plain.String())
	}

	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// fit pads s to w, or truncates it with an ellipsis when it is longer.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if n := ansi.StringWidth(s); n > w {
		return ansi.Truncate(s, w, "…")
	} else if n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func padLeft(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return ansi.Truncate(s, w, "")
}

// Update routes keys and mouse. The host forwards every message while the
// menu is up and none while it is down (the ZStack occlusion contract).
func (m Menu) Update(msg tea.Msg) (Menu, tea.Cmd) {
	switch msg := msg.(type) {
	case mouse.Msg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Cancel):
			return m, cancelled()
		case key.Matches(msg, m.keys.Choose):
			return m, m.choose()
		case key.Matches(msg, m.keys.Up):
			m.move(-1)
			return m, nil
		case key.Matches(msg, m.keys.Down):
			m.move(1)
			return m, nil
		case key.Matches(msg, m.keys.Top):
			m.jump(0, 1)
			return m, nil
		case key.Matches(msg, m.keys.Bottom):
			m.jump(len(m.set.Actions)-1, -1)
			return m, nil
		case key.Matches(msg, m.keys.HalfUp):
			m.move(-m.halfPage())
			return m, nil
		case key.Matches(msg, m.keys.HalfDown):
			m.move(m.halfPage())
			return m, nil
		}

		// Action shortcuts are matched after the built-ins, so a menu's own
		// vocabulary can never be shadowed by a verb that bound over it.
		for _, a := range m.set.Actions {
			if len(a.Key.Keys()) == 0 || !key.Matches(msg, a.Key) {
				continue
			}
			if m.reason(a) != "" {
				return m, nil
			}
			return m, chosen(a, m.set.Target)
		}
	}
	return m, nil
}

func (m Menu) handleMouse(e mouse.Msg) (Menu, tea.Cmd) {
	// A menu that was not drawn this frame declines everything, so a host
	// that forwards while it is down cannot make it cancel itself.
	if !m.placed.Fresh() {
		return m, nil
	}

	if target, ok := m.pane.HandleScrollbar(e); ok {
		m.scrollTo(target)
		return m, nil
	}

	inside := m.pane.Rect().Hit(e.X, e.Y)

	switch {
	case e.IsWheelUp():
		if inside {
			m.move(-1)
		}
		return m, nil

	case e.IsWheelDown():
		if inside {
			m.move(1)
		}
		return m, nil

	case e.IsRightPress():
		// A right-press outside the menu is the same gesture that opened it,
		// aimed somewhere else: "not that one, this one." Dismissing and
		// making the user click again would charge two gestures for one
		// intent. The menu cannot retarget itself — it has no idea what is
		// under the pointer in the layer below — so it declines the event and
		// reports where it went, and the host reopens against whatever that
		// turns out to be.
		//
		// Inside its own rect a right-press does nothing: there is no second
		// level of menu to ask about.
		if inside {
			return m, nil
		}
		return m, retarget(e)

	case e.IsPress():
		// A left press outside dismisses. A menu is a question, and clicking
		// away from a question is how every other menu on the machine
		// answers it.
		if !inside {
			return m, cancelled()
		}
		row, ok := m.pane.RowAt(e.X, e.Y)
		if !ok || row >= len(m.set.Actions) {
			return m, nil
		}
		if m.reasonAt(row) != "" {
			return m, nil
		}
		// One click commits. Rule 28 makes a single click a selection for
		// data rows, but a menu row is library-drawn chrome — the same
		// clause that has confirm's buttons and tab's labels acting on the
		// first press.
		m.cursor = row
		m.reflow()
		return m, chosen(m.set.Actions[row], m.set.Target)
	}
	return m, nil
}

// move steps the cursor by delta, skipping disabled rows and stopping at the
// ends rather than wrapping — matching list and table.
func (m *Menu) move(delta int) {
	n := len(m.set.Actions)
	if n == 0 || delta == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	remaining := delta
	if remaining < 0 {
		remaining = -remaining
	}
	i := m.cursor
	if i < 0 {
		i = -step // so the first step lands on 0 or n-1
		if step < 0 {
			i = n
		}
	}
	for remaining > 0 {
		next := i + step
		for next >= 0 && next < n && m.reasonAt(next) != "" {
			next += step
		}
		if next < 0 || next >= n {
			break
		}
		i = next
		remaining--
	}
	if i >= 0 && i < n && m.reasonAt(i) == "" {
		m.cursor = i
		m.reflow()
	}
}

// jump moves to the first selectable row at or after from, walking by step.
func (m *Menu) jump(from, step int) {
	for i := from; i >= 0 && i < len(m.set.Actions); i += step {
		if m.reasonAt(i) == "" {
			m.cursor = i
			m.reflow()
			return
		}
	}
}

// scrollTo is the scrollbar's entry point. It moves the cursor as well as the
// view: reflow re-asserts "the cursor is visible" on every frame, so a
// viewport moved on its own is undone one frame later.
func (m *Menu) scrollTo(row int) {
	n := len(m.set.Actions)
	if n == 0 {
		return
	}
	row = min(max(row, 0), n-1)
	if m.reasonAt(row) != "" {
		return
	}
	m.cursor = row
	m.reflow()
}

func (m Menu) choose() tea.Cmd {
	a, ok := m.Selected()
	if !ok || m.reason(a) != "" {
		return nil
	}
	return chosen(a, m.set.Target)
}

func (m Menu) halfPage() int {
	return max(1, m.pane.VisibleRows()/2)
}

func (m Menu) firstEnabled() int {
	for i := range m.set.Actions {
		if m.reasonAt(i) == "" {
			return i
		}
	}
	return -1
}

// CanScroll reports whether the actions overflow the box.
func (m Menu) CanScroll() bool {
	return m.pane.VisibleRows() > 0 && len(m.set.Actions) > m.pane.VisibleRows()
}

// Help returns the bindings the menu currently responds to, including the
// mouse affordance as a sentinel binding (rule 10) so it can be advertised in
// the expanded panel without ever matching a real key.
func (m Menu) Help() []key.Binding {
	out := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Choose, m.keys.Cancel}
	if m.CanScroll() {
		out = append(out, m.keys.Top, m.keys.Bottom, m.keys.HalfUp, m.keys.HalfDown)
	}
	return append(out,
		key.NewBinding(key.WithKeys("mouse:click"), key.WithHelp("click", "run")))
}
