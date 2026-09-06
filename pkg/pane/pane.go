// Package pane provides a bordered, titled, scrollable region for Bubble Tea
// TUIs. A Pane owns a viewport and renders a vertical scrollbar along its
// right edge. Any string content can be placed inside — render a child model
// to a string via its View() method and pass it to SetContent. While
// SetLoading(true) is in effect, the body is replaced by a centered spinner
// (with optional LoadingLabel) until SetLoading(false) restores the content.
package pane

import (
	"fmt"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/mouse"
)

// Keys is the pane's keymap for horizontal scroll. Each binding carries
// both its dispatch keys (WithKeys) and its help label (WithHelp);
// embedding components surface these in their own Help() so the hint
// strip stays honest when callers override defaults.
type Keys struct {
	Left, Right key.Binding
	LeftEdge    key.Binding
	RightEdge   key.Binding
}

// DefaultKeys returns the pane's stock h-scroll keymap.
func DefaultKeys() Keys {
	return Keys{
		Left:      key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "scroll left")),
		Right:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "scroll right")),
		LeftEdge:  key.NewBinding(key.WithKeys("0", "home"), key.WithHelp("0/home", "start")),
		RightEdge: key.NewBinding(key.WithKeys("$", "end"), key.WithHelp("$/end", "end")),
	}
}

// FillDefaults fills any zero-valued binding in k with its DefaultKeys()
// counterpart. Exported so embedders (list/table/tree/inspector/logview)
// can call it on their nested pane.Keys field without re-implementing
// the merge.
func (k *Keys) FillDefaults() {
	d := DefaultKeys()
	if len(k.Left.Keys()) == 0 {
		k.Left = d.Left
	}
	if len(k.Right.Keys()) == 0 {
		k.Right = d.Right
	}
	if len(k.LeftEdge.Keys()) == 0 {
		k.LeftEdge = d.LeftEdge
	}
	if len(k.RightEdge.Keys()) == 0 {
		k.RightEdge = d.RightEdge
	}
}

// HScrollStep is how many cells left/right scroll the pane horizontally.
const HScrollStep = 4

// Pane is a bordered region with a title, six metadata slots around the
// border, and a vertical scrollbar down the right edge.
type Pane struct {
	viewport viewport.Model

	width, height int
	rect          geom.Rect

	rawLines []string
	maxLineW int
	xOffset  int

	title       string
	topLeft     string
	topRight    string
	bottomLeft  string
	bottomMid   string
	bottomRight string // empty => auto-filled with scroll percent

	focused    bool
	hScrollbar bool

	// Scrollbar drag state. Tracked here because the release that ends a
	// drag can land anywhere, including outside the pane.
	vDragging bool
	hDragging bool

	// header is pinned under the top border, above the scrolling content.
	header   string
	titlePos BorderPosition

	loading      bool
	spinner      spinner.Model
	loadingLabel string

	// Virtual scroll metrics override the right-edge scrollbar's source data
	// when set externally (virtualTotal > 0). Used by components that window
	// their own content (e.g. pkg/table reserves the top row for a sticky
	// header) so the bar still reports an accurate scroll position over the
	// caller's logical row count rather than the in-viewport line count.
	// Auto-fill of bottom-right "%" is suppressed while virtual is active.
	virtualTotal   int
	virtualVisible int
	virtualOffset  int

	activeColor    lipgloss.TerminalColor
	inactiveColor  lipgloss.TerminalColor
	activeBorder   lipgloss.Border
	inactiveBorder lipgloss.Border
	slotBrackets   SlotBracketStyle
	glyphs         glyph.Set

	keys Keys
}

// Options configures a new Pane. Zero-value fields fall back to defaults.
type Options struct {
	Width, Height int
	Title         string
	// TitlePosition picks which border slot the title occupies. Defaults to
	// TopLeftBorder (the zero value).
	TitlePosition BorderPosition
	Focused       bool
	ActiveColor   lipgloss.TerminalColor
	InactiveColor lipgloss.TerminalColor
	// ActiveBorder is drawn when the Pane is focused, InactiveBorder when
	// it is not. Both default to lipgloss.NormalBorder(): focus is carried
	// by ActiveColor, and a pane that also changed weight on focus would
	// shift the surrounding layout's visual weight for no reason the user
	// asked for. The same two defaults are what theme.Theme resolves an
	// unset BorderShapeActive / BorderShapeInactive to, so a themed pane and
	// a bare pane.New(pane.Options{}) agree.
	//
	// Overlays are the deliberate exception — see Theme.BorderShapeOverlay.
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	// Glyphs supplies the scrollbar thumb and track. Empty fields fall back
	// to glyph.Default.
	Glyphs glyph.Set
	// SlotBrackets controls how the title and other border slot text are
	// bracketed against the border line. Defaults to SlotBracketsNone
	// (text sits inline on the border with no surrounding glyphs).
	SlotBrackets SlotBracketStyle
	// HScrollbar reserves a single row at the bottom of the inner content
	// area for a horizontal scrollbar. The thumb tracks xOffset against
	// the longest line; when content fits, the track renders blank.
	HScrollbar bool
	// Spinner picks the spinner frames used while the pane is in a
	// loading state (see SetLoading). When zero, defaults to spinner.Dot.
	Spinner *spinner.Spinner
	// SpinnerStyle is applied to the spinner glyph. The zero value
	// renders without any style; pass via theme.Pane() for a sensible
	// foreground.
	SpinnerStyle lipgloss.Style
	// LoadingLabel is rendered next to the spinner while loading. Use it
	// to give the user context — e.g. "loading cities…" or "fetching".
	LoadingLabel string

	// Keys overrides the pane's h-scroll keymap. Fields left zero fall
	// back to DefaultKeys(); embedders typically forward their own Keys
	// here so the user-facing override surface stays one struct per
	// component.
	Keys Keys
}

// New constructs a Pane. SetContent must be called separately to populate it.
func New(opts Options) Pane {
	if opts.ActiveColor == nil {
		opts.ActiveColor = lipgloss.Color("12")
	}
	if opts.InactiveColor == nil {
		opts.InactiveColor = lipgloss.Color("240")
	}
	if (opts.ActiveBorder == lipgloss.Border{}) {
		opts.ActiveBorder = lipgloss.NormalBorder()
	}
	if (opts.InactiveBorder == lipgloss.Border{}) {
		opts.InactiveBorder = lipgloss.NormalBorder()
	}
	frames := spinner.Dot
	if opts.Spinner != nil {
		frames = *opts.Spinner
	}
	sp := spinner.New(spinner.WithSpinner(frames), spinner.WithStyle(opts.SpinnerStyle))
	opts.Keys.FillDefaults()

	p := Pane{
		viewport:       viewport.New(0, 0),
		title:          opts.Title,
		titlePos:       opts.TitlePosition,
		focused:        opts.Focused,
		hScrollbar:     opts.HScrollbar,
		spinner:        sp,
		loadingLabel:   opts.LoadingLabel,
		activeColor:    opts.ActiveColor,
		inactiveColor:  opts.InactiveColor,
		activeBorder:   opts.ActiveBorder,
		inactiveBorder: opts.InactiveBorder,
		slotBrackets:   opts.SlotBrackets,
		glyphs:         opts.Glyphs.Resolve(),
		keys:           opts.Keys,
	}
	p.SetRect(geom.Rect{W: opts.Width, H: opts.Height})
	return p
}

func (p Pane) Init() tea.Cmd { return nil }

// Update forwards key/mouse events to the embedded viewport so vertical
// scroll keys (pgup/pgdn/up/down/mouse wheel) work by default. Horizontal
// scroll keys are intercepted: left/h and right/l step by HScrollStep;
// 0 and home jump to the left edge; $ and end jump to the right edge.
// The content is re-cut to the visible window via ansi.Cut so ANSI
// styles stay intact across the slice. While loading, spinner.TickMsg
// events are consumed to advance the spinner; the chained next-tick
// command is returned so the animation keeps running.
func (p Pane) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok {
		if !p.loading {
			return p, nil
		}
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return p, cmd
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, p.keys.Left):
			p.xOffset -= HScrollStep
			p.pushContent()
			return p, nil
		case key.Matches(km, p.keys.Right):
			p.xOffset += HScrollStep
			p.pushContent()
			return p, nil
		case key.Matches(km, p.keys.LeftEdge):
			if p.xOffset != 0 {
				p.xOffset = 0
				p.pushContent()
			}
			return p, nil
		case key.Matches(km, p.keys.RightEdge):
			if mx := p.MaxXOffset(); p.xOffset != mx {
				p.xOffset = mx
				p.pushContent()
			}
			return p, nil
		}
	}
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

// View renders the pane: content inside viewport, scrollbar on the right,
// both wrapped in a titled border with metadata slots. While loading, the
// inner content area is replaced with a centered spinner glyph (plus an
// optional label) and scroll chrome is suppressed.
func (p Pane) View() string {
	innerW := max(0, p.width-2)
	innerH := max(0, p.height-2)

	var body string
	br := p.bottomRight
	if p.loading {
		// The header is chrome, not content: a filter typed while data is
		// in flight has to stay visible, so the spinner replaces only the
		// scrolling area beneath it.
		body = lipgloss.Place(innerW, max(0, innerH-p.headerRows()),
			lipgloss.Center, lipgloss.Center, p.loadingView())
	} else {
		total, visible, offset := p.viewport.TotalLineCount(), p.viewport.VisibleLineCount(), p.viewport.YOffset
		if p.virtualTotal > 0 {
			total, visible, offset = p.virtualTotal, p.virtualVisible, p.virtualOffset
		}
		bar := ScrollbarWith(p.viewport.Height, total, visible, offset, p.glyphs)
		body = lipgloss.JoinHorizontal(lipgloss.Top, p.viewport.View(), bar)
		if p.hScrollbar {
			inner := p.viewport.Width
			hbar := HScrollbarWith(inner, p.maxLineW, inner, p.xOffset, p.glyphs)
			body = lipgloss.JoinVertical(lipgloss.Left, body, hbar+strings.Repeat(" ", ScrollbarWidth))
		}
		// Auto-fill bottom-right with scroll percent only when content actually
		// overflows. Panes used as input strips (filter bars, one-liners) would
		// otherwise show a meaningless "100%". When virtual scroll is active
		// the caller paints its own indicator (e.g. "5 / 100") in
		// SetBottomRight, so skip the auto-fill there too.
		if br == "" && p.virtualTotal == 0 && p.viewport.TotalLineCount() > p.viewport.VisibleLineCount() {
			br = fmt.Sprintf("%d%%", int(p.viewport.ScrollPercent()*100))
		}
	}

	slots := map[BorderPosition]string{
		TopLeftBorder:      pad(p.topLeft),
		TopMiddleBorder:    pad(""),
		TopRightBorder:     pad(p.topRight),
		BottomLeftBorder:   pad(p.bottomLeft),
		BottomMiddleBorder: pad(p.bottomMid),
		BottomRightBorder:  pad(br),
	}
	if p.header != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, p.header, body)
	}

	// Title overrides whichever slot it's assigned to.
	if p.title != "" {
		slots[p.titlePos] = pad(p.title)
	}

	border := p.inactiveBorder
	color := p.inactiveColor
	if p.focused {
		border = p.activeBorder
		color = p.activeColor
	}
	return Borderize(body, border, color, slots, p.slotBrackets)
}

// loadingView returns the spinner glyph plus an optional " loading…"
// suffix, all in the spinner's style.
func (p Pane) loadingView() string {
	v := p.spinner.View()
	if p.loadingLabel != "" {
		v += " " + p.spinner.Style.Render(p.loadingLabel)
	}
	return v
}

// Loading reports whether the pane is currently in its loading state.
func (p Pane) Loading() bool { return p.loading }

// SetLoading toggles the loading state. When entering the loading state,
// returns the spinner's initial Tick command — propagate it back to
// bubbletea (typically by returning it from your screen's Update or
// batching with other commands) so the spinner animates. When leaving
// the loading state, returns nil; any in-flight TickMsg is silently
// dropped on the next Update.
func (p *Pane) SetLoading(b bool) tea.Cmd {
	if p.loading == b {
		return nil
	}
	p.loading = b
	if b {
		return p.spinner.Tick
	}
	return nil
}

// SetLoadingLabel updates the text shown next to the spinner while loading.
func (p *Pane) SetLoadingLabel(s string) { p.loadingLabel = s }

// SetSpinnerStyle updates the lipgloss style applied to the spinner glyph.
// Useful for re-theming without rebuilding the pane.
func (p *Pane) SetSpinnerStyle(s lipgloss.Style) { p.spinner.Style = s }

// SetSlotBrackets controls how the title and other slot text meet the border.
func (p *Pane) SetSlotBrackets(s SlotBracketStyle) { p.slotBrackets = s }

// SetContent replaces the pane's content. Pass any string — a child model's
// View() output, a pre-rendered table, a log, raw text — and the pane will
// scroll it. Long lines are truncated to the inner width so terminal wrap
// can't break row counting; use left/right (or SetXOffset) to scroll
// horizontally past the cut.
func (p *Pane) SetContent(s string) {
	if s == "" {
		p.rawLines = nil
	} else {
		p.rawLines = strings.Split(s, "\n")
	}
	p.maxLineW = 0
	for _, l := range p.rawLines {
		if w := lipgloss.Width(l); w > p.maxLineW {
			p.maxLineW = w
		}
	}
	p.pushContent()
}

// SetRect places the Pane at an absolute position and outer size (including
// border). The inner content area is sized as (width-2-scrollbar) ×
// (height-2), shrunk by one more row when HScrollbar is enabled.
//
// The rect is retained so the Pane and the components embedding it can map a
// mouse position back to a content row — see Rect and ContentRect.
func (p *Pane) SetRect(r geom.Rect) {
	p.rect = r
	p.applySize()
}

// SetHeader pins lines directly under the top border, above the scrolling
// content. They never scroll, and the viewport shrinks to make room.
//
// This is how a component puts its own chrome inside its pane rather than
// beside it — a filterable list draws its filter row here, so the filter is
// visibly *inside* the thing it filters instead of floating above it as an
// equal-weight sibling. Pass "" to remove it.
//
// The header is rendered as given; a component wanting a rule beneath it
// should include one (the inner width is available from ContentRect).
func (p *Pane) SetHeader(s string) {
	if p.header == s {
		return
	}
	p.header = s
	p.applySize()
}

// Header returns the pinned header lines.
func (p Pane) Header() string { return p.header }

// headerRows is how many rows the pinned header consumes.
func (p Pane) headerRows() int {
	if p.header == "" {
		return 0
	}
	return strings.Count(p.header, "\n") + 1
}

// applySize recomputes the viewport from the outer rect, the scrollbars, and
// the pinned header. Called whenever any of those change.
func (p *Pane) applySize() {
	p.width, p.height = p.rect.W, p.rect.H
	p.viewport.Width = max(0, p.rect.W-2-ScrollbarWidth)
	innerH := p.rect.H - 2 - p.headerRows()
	if p.hScrollbar {
		innerH -= ScrollbarHeight
	}
	p.viewport.Height = max(0, innerH)
	p.pushContent()
}

// Rect returns the outer rect the Pane was last placed at, border included.
func (p Pane) Rect() geom.Rect { return p.rect }

// ContentRect returns the rect covering the Pane's inner content area — the
// outer rect less the border, the right-edge scrollbar, and the horizontal
// scrollbar row when enabled. It is the region row 0 of the content occupies,
// so RowAt inverts against it.
func (p Pane) ContentRect() geom.Rect {
	r := geom.Rect{
		X:   p.rect.X + 1,
		Y:   p.rect.Y + 1 + p.headerRows(),
		W:   p.viewport.Width,
		H:   p.viewport.Height,
		Gen: p.rect.Gen,
	}
	if r.W < 0 {
		r.W = 0
	}
	if r.H < 0 {
		r.H = 0
	}
	return r
}

// scrollMetrics returns the (total, visible, offset) triple the scrollbar is
// drawn from — the viewport's own numbers, unless a component windows its
// rows itself and pushed virtual metrics through SetVirtualScroll.
func (p Pane) scrollMetrics() (total, visible, offset int) {
	if p.virtualTotal > 0 {
		return p.virtualTotal, p.virtualVisible, p.virtualOffset
	}
	return p.viewport.TotalLineCount(), p.viewport.VisibleLineCount(), p.viewport.YOffset
}

// VScrollbarRect returns the rect of the vertical scrollbar column, which
// View draws immediately right of the viewport.
func (p Pane) VScrollbarRect() geom.Rect {
	c := p.ContentRect()
	return geom.Rect{X: c.X + p.viewport.Width, Y: c.Y, W: ScrollbarWidth, H: p.viewport.Height, Gen: p.rect.Gen}
}

// HScrollbarRect returns the rect of the horizontal scrollbar row, drawn
// below the body when HScrollbar is enabled. Empty when it isn't.
func (p Pane) HScrollbarRect() geom.Rect {
	if !p.hScrollbar {
		return geom.Rect{}
	}
	c := p.ContentRect()
	return geom.Rect{X: c.X, Y: c.Y + p.viewport.Height, W: p.viewport.Width, H: ScrollbarHeight, Gen: p.rect.Gen}
}

// ScrollbarDrag reports whether a scrollbar drag is in progress.
func (p Pane) ScrollbarDrag() bool { return p.vDragging || p.hDragging }

// HandleScrollbar processes a mouse event against the Pane's scrollbars.
// Components call it first from their own mouse handling, so a click on the
// bar scrolls rather than selecting the row behind it.
//
// ok reports whether the event was a scrollbar interaction at all. row is
// the content row the interaction targets — meaningful only when ok is true.
//
// For a pane that scrolls its own viewport, the scroll is already applied and
// row can be ignored. For a component that windows its rows itself (pkg/table
// and pkg/inspector push metrics through SetVirtualScroll, and derive their
// window from the cursor), nothing is applied: moving the viewport under such
// a component would be undone on its next render, so it must move its cursor
// to row instead.
//
// Pressing anywhere on the track jumps there, the thumb included — grabbing
// the thumb is a jump to where it already is, which leaves it put. Motion
// while the button is held keeps jumping, which is what makes it a drag. The
// pane tracks that state itself because the release can land outside the bar,
// or outside the pane entirely, and must still end the drag.
func (p *Pane) HandleScrollbar(e mouse.Msg) (row int, ok bool) {
	if e.Action == tea.MouseActionRelease {
		if p.vDragging || p.hDragging {
			p.vDragging, p.hDragging = false, false
			// Report where the scroll already is, not zero. The release ends
			// the drag; it does not move anything. A caller that scrolls to
			// the reported row would otherwise slam back to the top the
			// instant the button came up — which is exactly what a dragged
			// scrollbar snapping to the start looks like.
			_, _, offset := p.scrollMetrics()
			return offset, true
		}
		return 0, false
	}

	held := e.Action == tea.MouseActionMotion && e.Button == tea.MouseButtonLeft
	if !e.IsPress() && !held {
		return 0, false
	}

	switch {
	case p.vDragging && held:
		return p.jumpV(e.Y), true
	case p.hDragging && held:
		p.jumpH(e.X)
		return 0, true
	case e.IsPress() && p.VScrollbarRect().Hit(e.X, e.Y):
		p.vDragging = true
		return p.jumpV(e.Y), true
	case e.IsPress() && p.HScrollbarRect().Hit(e.X, e.Y):
		p.hDragging = true
		p.jumpH(e.X)
		return 0, true
	}
	return 0, false
}

// jumpV maps a position on the vertical track to a content row, scrolling
// the viewport there when the pane owns its own scroll. Inverts what
// Scrollbar draws.
func (p *Pane) jumpV(y int) int {
	bar := p.VScrollbarRect()
	total, visible, _ := p.scrollMetrics()
	if bar.H <= 0 || total <= visible {
		return 0
	}
	rel := clampInt(y-bar.Y, 0, bar.H-1)
	offset := clampInt(rel*total/bar.H, 0, max(0, total-visible))
	if p.virtualTotal == 0 {
		p.viewport.SetYOffset(offset)
	}
	return offset
}

// jumpH scrolls horizontally to the column at position x within the track.
func (p *Pane) jumpH(x int) {
	bar := p.HScrollbarRect()
	if bar.W <= 0 || p.maxLineW <= p.viewport.Width {
		return
	}
	rel := clampInt(x-bar.X, 0, bar.W-1)
	p.SetXOffset(clampInt(rel*p.maxLineW/bar.W, 0, p.MaxXOffset()))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ScrollWheel applies a wheel event to the Pane's vertical scroll and reports
// whether it was consumed. It is the shared implementation for components
// that scroll the pane rather than moving a cursor — logview, textview, and a
// bare pane. Events outside the content area, or aimed at a Pane that wasn't
// drawn this frame, are declined so a sibling can claim them.
//
// bubbles' viewport handles the wheel on tea.MouseMsg, but tuilib components
// receive mouse.Msg (which carries the resolved click count), so that path
// never fires — this is where wheel scrolling actually happens.
func (p *Pane) ScrollWheel(x, y int, up bool) bool {
	if !p.ContentRect().Hit(x, y) {
		return false
	}
	delta := WheelStep
	if up {
		delta = -delta
	}
	p.viewport.SetYOffset(p.viewport.YOffset + delta)
	return true
}

// WheelStep is how many lines one wheel notch scrolls. Three matches the
// convention terminals and GUI toolkits share.
const WheelStep = 3

// RowAt maps a terminal position to a zero-based index into the Pane's
// content, accounting for the border inset and the current vertical scroll.
// It reports ok=false when the position is outside the content area or the
// Pane wasn't drawn in the current frame, so a caller can forward the event
// on rather than claiming it.
func (p Pane) RowAt(x, y int) (row int, ok bool) {
	c := p.ContentRect()
	if !c.Hit(x, y) {
		return 0, false
	}
	return (y - c.Y) + p.viewport.YOffset, true
}

// XOffset returns the current horizontal scroll column.
func (p Pane) XOffset() int { return p.xOffset }

// Keys returns the pane's h-scroll keymap. Embedding components include
// these in their own Help() so the hint strip surfaces h-scroll keys
// honestly (and reflects any caller overrides).
func (p Pane) Keys() Keys { return p.keys }

// HelpBindings returns the bindings to display in a component's hint
// strip — just the four pane keys (left/right move + edge jumps). The
// embedder appends these to its own Help() slice.
func (p Pane) HelpBindings() []key.Binding {
	return []key.Binding{p.keys.Left, p.keys.Right, p.keys.LeftEdge, p.keys.RightEdge}
}

// MaxXOffset returns the largest meaningful horizontal scroll column —
// 0 when content fits in the inner width.
func (p Pane) MaxXOffset() int {
	return max(0, p.maxLineW-p.viewport.Width)
}

// SetXOffset jumps to the given horizontal scroll column, clamped into
// [0, MaxXOffset()].
func (p *Pane) SetXOffset(n int) {
	p.xOffset = n
	p.pushContent()
}

// pushContent re-cuts every raw line to the visible window and pushes
// the result to the viewport. Skips the cut entirely when xOffset is 0
// and every line fits — a fast path for the common case.
func (p *Pane) pushContent() {
	maxOff := p.MaxXOffset()
	if p.xOffset > maxOff {
		p.xOffset = maxOff
	}
	if p.xOffset < 0 {
		p.xOffset = 0
	}
	if len(p.rawLines) == 0 {
		p.viewport.SetContent("")
		return
	}
	inner := p.viewport.Width
	if inner <= 0 || (p.xOffset == 0 && p.maxLineW <= inner) {
		p.viewport.SetContent(strings.Join(p.rawLines, "\n"))
		return
	}
	var b strings.Builder
	for i, line := range p.rawLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ansi.Cut(line, p.xOffset, p.xOffset+inner))
	}
	p.viewport.SetContent(b.String())
}

// Width returns the Pane's outer width.
func (p Pane) Width() int { return p.width }

// Height returns the Pane's outer height.
func (p Pane) Height() int { return p.height }

// VisibleRows returns the inner viewport height — the number of content
// rows the pane can display at once, after subtracting borders and the
// horizontal scrollbar (when enabled). Useful for components that need to
// move a cursor by a window-relative amount (e.g. half-page jumps).
func (p Pane) VisibleRows() int { return p.viewport.Height }

// VisibleWidth returns the inner viewport width — the number of visible
// cells the pane can display per row, after subtracting borders and the
// vertical scrollbar gutter. Useful for components that distribute
// horizontal space across sub-elements (e.g. flex columns).
func (p Pane) VisibleWidth() int { return p.viewport.Width }

// Focused reports whether the pane is drawn in its active style.
func (p Pane) Focused() bool { return p.focused }

func (p *Pane) SetFocused(b bool)                   { p.focused = b }
func (p *Pane) SetTitle(s string)                   { p.title = s }
func (p *Pane) SetTitlePosition(pos BorderPosition) { p.titlePos = pos }

// SetActiveColor updates the border color used when the pane is focused.
// Useful when reacting to a theme swap without rebuilding the model.
func (p *Pane) SetActiveColor(c lipgloss.TerminalColor) { p.activeColor = c }

// SetInactiveColor updates the border color used when the pane is unfocused.
func (p *Pane) SetInactiveColor(c lipgloss.TerminalColor) { p.inactiveColor = c }

// SetActiveBorder updates the border shape drawn when the pane is focused.
func (p *Pane) SetActiveBorder(b lipgloss.Border) {
	if (b == lipgloss.Border{}) {
		return
	}
	p.activeBorder = b
}

// SetInactiveBorder updates the border shape drawn when the pane is unfocused.
func (p *Pane) SetInactiveBorder(b lipgloss.Border) {
	if (b == lipgloss.Border{}) {
		return
	}
	p.inactiveBorder = b
}
func (p *Pane) SetTopLeft(s string)      { p.topLeft = s }
func (p *Pane) SetTopRight(s string)     { p.topRight = s }
func (p *Pane) SetBottomLeft(s string)   { p.bottomLeft = s }
func (p *Pane) SetBottomMiddle(s string) { p.bottomMid = s }

// SetBottomRight overrides the auto-generated scroll percentage. Pass "" to
// restore the default.
func (p *Pane) SetBottomRight(s string) { p.bottomRight = s }

// GotoTop scrolls the viewport to the first line.
func (p *Pane) GotoTop() { p.viewport.GotoTop() }

// GotoBottom scrolls the viewport to the last line.
func (p *Pane) GotoBottom() { p.viewport.GotoBottom() }

// AtBottom reports whether the viewport is scrolled to the last line —
// useful for streaming-content components that auto-follow new output
// only while the user is parked at the bottom.
func (p Pane) AtBottom() bool { return p.viewport.AtBottom() }

// YOffset returns the current vertical scroll offset (top visible line).
func (p Pane) YOffset() int { return p.viewport.YOffset }

// SetYOffset jumps to the given vertical scroll offset.
func (p *Pane) SetYOffset(n int) { p.viewport.SetYOffset(n) }

// SetVirtualScroll overrides the right-edge scrollbar's source data.
// Instead of computing thumb size and position from the viewport's
// in-memory line count, the bar uses (total, visible, offset) directly —
// in any units the caller chooses, commonly logical row counts. Used by
// components that window their own content outside the viewport so the
// scrollbar still reflects the full dataset rather than the in-viewport
// slice (e.g. pkg/table reserves the top inner row for a sticky header
// and feeds the bar with len(rows) / dataRowsWindow / firstVisibleRow).
//
// While virtual scroll is active the bottom-right "%" auto-fill is
// suppressed so the caller can paint a more meaningful indicator (e.g.
// "5 / 100") via SetBottomRight. Pass total <= 0 to disable and revert
// to viewport-driven metrics.
func (p *Pane) SetVirtualScroll(total, visible, offset int) {
	if total <= 0 {
		p.virtualTotal, p.virtualVisible, p.virtualOffset = 0, 0, 0
		return
	}
	p.virtualTotal, p.virtualVisible, p.virtualOffset = total, visible, offset
}

// EnsureVisible scrolls the viewport the minimum amount needed to put line
// `n` inside the visible window. Useful for cursor-driven list views, where
// moving the cursor past the viewport's bottom should pull the view with it.
func (p *Pane) EnsureVisible(n int) {
	top := p.viewport.YOffset
	bottom := top + p.viewport.Height - 1
	switch {
	case n < top:
		p.viewport.SetYOffset(n)
	case n > bottom:
		p.viewport.SetYOffset(n - p.viewport.Height + 1)
	}
}

func pad(s string) string {
	if s == "" {
		return ""
	}
	return " " + s + " "
}
