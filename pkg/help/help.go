// Package help renders key-binding hints in three shapes:
//
//   - ShortView / ShortViewBudget — a single inline line, "key desc • key
//     desc • …", sized to fit a width budget. The app shell paints this
//     into the statusbar's left slot. When the bindings don't all fit
//     and m.Expanded() is false, ShortViewBudget tight-packs bindings
//     and appends a "? help" affordance. When m.Expanded() is true, the
//     line becomes row 0 of a column-aligned grid whose last cell is
//     the "? close" affordance; rows 1+ continue in ExpandedView with
//     the same column widths, so labels line up vertically across the
//     footer and the panel.
//   - ExpandedView / ExpandedRows — the rows-1-and-onward continuation
//     of the unified grid: same column widths as the footer's row 0,
//     same " • " column separator, row count clamped to maxRows. The
//     app shell renders this in a Fixed row above the statusbar when
//     m.Expanded() is true.
//   - View — a legacy bordered overlay (kept for parents that want a
//     standalone popup); uses key/desc column-pair flowing.
//
// The footer + panel together act as one component: when expanded, the
// footer is row 0 of the grid and the panel is rows 1+, so a single
// planGridFull pass at the statusbar's left-slot width drives both.
// Pressing the app's HelpKey ("?" by default) flips m.Expanded(); the
// shell only listens to the key when the inline flow has overflow, so
// when everything fits the affordance is hidden and the key is inert.
//
// Components that want to contribute their own bindings can implement the
// Provider interface; the parent collects bindings from the focused child
// and passes them in via SetBindings before rendering.
package help

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Provider is implemented by components that want to surface extra key
// bindings in the help overlay when focused.
type Provider interface {
	HelpBindings() []key.Binding
}

// Options configures a help overlay. Zero-value fields fall back to defaults.
type Options struct {
	// Width and Height are the outer dimensions of the overlay, including the
	// border.
	Width, Height int
	// KeyStyle is applied to the key column (left side of each pair).
	KeyStyle lipgloss.Style
	// DescStyle is applied to the description column.
	DescStyle lipgloss.Style
	// Border is the overlay's border. Defaults to lipgloss.NormalBorder().
	Border lipgloss.Border
	// BorderColor colors the border. Defaults to "240" (dim grey).
	BorderColor lipgloss.TerminalColor
	// ColumnSpacer is placed between column-pairs in the overlay. Defaults
	// to "   ".
	ColumnSpacer string
	// ShortSeparator is placed between bindings in ShortView. Defaults to
	// "  •  ".
	ShortSeparator string
}

// Model renders the help strip + expanded panel + bordered overlay. Call
// SetBindings whenever the active binding set changes; the three render
// methods (ShortView/ShortViewBudget, ExpandedView, View) all read from
// the same compiled list. Visibility of the bordered View is the parent's
// concern; the footer + panel are wired by pkg/app and follow the
// Expanded() state plus overflow detection from ShortViewBudget.
type Model struct {
	width, height int
	bindings      []key.Binding

	keyStyle, descStyle lipgloss.Style
	border              lipgloss.Border
	borderColor         lipgloss.TerminalColor
	spacer              string
	shortSep            string

	expanded bool
}

// New constructs a help overlay.
func New(opts Options) Model {
	if (opts.Border == lipgloss.Border{}) {
		opts.Border = lipgloss.NormalBorder()
	}
	if opts.BorderColor == nil {
		opts.BorderColor = lipgloss.Color("240")
	}
	if opts.ColumnSpacer == "" {
		opts.ColumnSpacer = "   "
	}
	if opts.ShortSeparator == "" {
		opts.ShortSeparator = "  •  "
	}
	if opts.Width == 0 {
		opts.Width = 80
	}
	if opts.Height == 0 {
		opts.Height = 6
	}
	return Model{
		width:       opts.Width,
		height:      opts.Height,
		keyStyle:    opts.KeyStyle,
		descStyle:   opts.DescStyle,
		border:      opts.Border,
		borderColor: opts.BorderColor,
		spacer:      opts.ColumnSpacer,
		shortSep:    opts.ShortSeparator,
	}
}

// ShortView renders the current bindings as a single inline line —
// "key desc <sep> key desc <sep> ..." — using KeyStyle and DescStyle.
//
// The separator and the space between each key and its description are
// rendered through DescStyle so that any background color set on DescStyle
// extends across the whole line with no gaps. When embedding in a colored
// status bar, give KeyStyle and DescStyle the same Background as the bar.
func (m Model) ShortView() string {
	if len(m.bindings) == 0 {
		return ""
	}
	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)
	parts := make([]string, 0, len(m.bindings))
	for _, b := range m.bindings {
		parts = append(parts,
			m.keyStyle.Render(b.Help().Key)+spacer+m.descStyle.Render(b.Help().Desc))
	}
	return strings.Join(parts, sep)
}

// ShortViewBudget renders the footer line fitting within width visible
// cells. Behavior depends on m.Expanded():
//
//   - When collapsed (m.Expanded() == false), bindings are tight-packed
//     inline ("key desc  •  key desc  •  …") until the next one would
//     not fit; a "? help" affordance is appended on overflow.
//   - When expanded (m.Expanded() == true), the line becomes row 0 of a
//     column-aligned grid whose last cell is the "? close" affordance.
//     ExpandedView renders rows 1+ of the same grid at the same width
//     so footer and panel columns align vertically.
//
// consumed reports how many bindings were placed on the line. overflow
// reports whether bindings were dropped because they didn't fit (in
// expanded mode, true when the grid spills into rows 1+). Width 0 or
// less skips budgeting and falls back to ShortView.
func (m Model) ShortViewBudget(width int) (line string, consumed int, overflow bool) {
	if len(m.bindings) == 0 {
		return "", 0, false
	}
	if width <= 0 {
		return m.ShortView(), len(m.bindings), false
	}
	if m.expanded {
		return m.gridRow0(width)
	}

	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)
	sepVis := lipgloss.Width(sep)

	affordanceText := m.keyStyle.Render("?") + spacer + m.descStyle.Render("help")
	// Reserve space for whichever affordance label is wider ("close" today)
	// so toggling expanded doesn't shuffle which bindings the footer shows
	// at the boundary between collapsed and expanded states.
	helpW := lipgloss.Width(m.keyStyle.Render("?") + spacer + m.descStyle.Render("help"))
	closeW := lipgloss.Width(m.keyStyle.Render("?") + spacer + m.descStyle.Render("close"))
	affordanceCost := sepVis + max(helpW, closeW)

	used := 0
	var parts []string
	for i, b := range m.bindings {
		pair := m.keyStyle.Render(b.Help().Key) + spacer + m.descStyle.Render(b.Help().Desc)
		add := lipgloss.Width(pair)
		if i > 0 {
			add += sepVis
		}
		budget := width
		if i < len(m.bindings)-1 {
			budget -= affordanceCost
		}
		if used+add > budget {
			overflow = true
			break
		}
		parts = append(parts, pair)
		used += add
	}
	consumed = len(parts)
	line = strings.Join(parts, sep)
	if overflow {
		if line != "" {
			line += sep
		}
		line += affordanceText
	}
	return line, consumed, overflow
}

// gridRow0 renders row 0 of the unified grid: bindings 0..row0Count-1
// followed by the affordance cell, all sized to per-column widths from
// planGridFull. Returns line padded to width so any background color
// from DescStyle extends to the full slot width.
func (m Model) gridRow0(width int) (line string, consumed int, overflow bool) {
	cols, keyW, descW, row0Count := m.planGridFull(width)
	if cols == 0 {
		return "", 0, false
	}
	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)

	var cells []string
	for c := 0; c < row0Count; c++ {
		b := m.bindings[c]
		cells = append(cells, m.keyStyle.Render(padRight(b.Help().Key, keyW[c]))+
			spacer+
			m.descStyle.Render(padRight(b.Help().Desc, descW[c])))
	}
	// Affordance cell at the last column.
	affCol := cols - 1
	cells = append(cells, m.keyStyle.Render(padRight("?", keyW[affCol]))+
		spacer+
		m.descStyle.Render(padRight("close", descW[affCol])))

	line = strings.Join(cells, sep)
	line = m.fillToWidth(line, width)
	overflow = row0Count < len(m.bindings)
	return line, row0Count, overflow
}

// Expanded reports whether the multi-row panel view should be rendered
// (the app shell consults this when deciding whether to reserve rows
// for ExpandedView above the statusbar).
func (m Model) Expanded() bool { return m.expanded }

// SetExpanded controls whether the model is in the expanded state.
// Use ToggleExpanded for the keyboard-driven flip.
func (m *Model) SetExpanded(b bool) { m.expanded = b }

// ToggleExpanded flips the expanded state and reports the new value.
func (m *Model) ToggleExpanded() bool {
	m.expanded = !m.expanded
	return m.expanded
}

// ExpandedRows reports how many panel rows ExpandedView needs at width,
// clamped to maxRows. The unified grid puts the footer at row 0 and the
// panel at rows 1+; this returns the number of rows 1+ that hold the
// remaining bindings. Width or maxRows ≤ 0, model not expanded, or all
// bindings fit in row 0 yields 0.
func (m Model) ExpandedRows(width, maxRows int) int {
	if !m.expanded || width <= 0 || maxRows <= 0 {
		return 0
	}
	cols, _, _, row0Count := m.planGridFull(width)
	if cols == 0 {
		return 0
	}
	remaining := len(m.bindings) - row0Count
	if remaining <= 0 {
		return 0
	}
	rows := (remaining + cols - 1) / cols
	if rows > maxRows {
		return maxRows
	}
	return rows
}

// ExpandedView renders rows 1+ of the unified grid at width. Column
// widths come from the same planGridFull pass that ShortViewBudget
// uses for row 0, so labels line up vertically across the footer and
// panel. Each row is padded to width with DescStyle so any background
// color set on DescStyle fills the row edge to edge. Stops when it has
// filled rows lines or run out of bindings, whichever comes first.
// Returns empty when not expanded or when all bindings fit on row 0.
func (m Model) ExpandedView(width, rows int) string {
	if !m.expanded || width <= 0 || rows <= 0 {
		return ""
	}
	cols, keyW, descW, row0Count := m.planGridFull(width)
	if cols == 0 {
		return ""
	}
	n := len(m.bindings)
	remaining := n - row0Count
	if remaining <= 0 {
		return ""
	}
	panelRows := (remaining + cols - 1) / cols
	if panelRows > rows {
		panelRows = rows
	}
	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)

	var lines []string
	for r := 0; r < panelRows; r++ {
		var cells []string
		for c := 0; c < cols; c++ {
			idx := row0Count + r*cols + c
			if idx >= n {
				break
			}
			b := m.bindings[idx]
			cells = append(cells, m.keyStyle.Render(padRight(b.Help().Key, keyW[c]))+
				spacer+
				m.descStyle.Render(padRight(b.Help().Desc, descW[c])))
		}
		lines = append(lines, m.fillToWidth(strings.Join(cells, sep), width))
	}
	return strings.Join(lines, "\n")
}

// PadLines wraps each line of s with leftPad and rightPad DescStyle-
// rendered spaces so a background color from DescStyle extends across
// the padded edges. Use it in the app shell to align the panel with
// the statusbar's left-slot padding (so panel columns sit at the same
// visible x as the footer's row 0) and to fill the row's right edge
// to full terminal width.
func (m Model) PadLines(s string, leftPad, rightPad int) string {
	if leftPad <= 0 && rightPad <= 0 {
		return s
	}
	left := ""
	if leftPad > 0 {
		left = m.descStyle.Render(strings.Repeat(" ", leftPad))
	}
	right := ""
	if rightPad > 0 {
		right = m.descStyle.Render(strings.Repeat(" ", rightPad))
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = left + ln + right
	}
	return strings.Join(lines, "\n")
}

// planGridFull picks the largest column count that fits all bindings
// plus an affordance cell (always the last cell of row 0) inside width
// visible cells. Returns per-column key/desc widths and row0Count, the
// number of regular binding cells on row 0 (= cols-1). Bindings beyond
// row 0 flow row-major across rows 1+. cols=0 when there are no
// bindings.
func (m Model) planGridFull(width int) (cols int, keyW, descW []int, row0Count int) {
	n := len(m.bindings)
	if n == 0 {
		return 0, nil, nil, 0
	}
	sepVis := lipgloss.Width(m.descStyle.Render(m.shortSep))

	// Reserve affordance widths from the wider of "? help" / "? close"
	// so the column doesn't reshuffle when the label flips between them.
	affKeyW := lipgloss.Width("?")
	affDescW := lipgloss.Width("close")
	if w := lipgloss.Width("help"); w > affDescW {
		affDescW = w
	}

	chosen := 0
	var chosenKW, chosenDW []int
	chosenRow0 := 0

	for try := 1; try <= n+1; try++ {
		kw := make([]int, try)
		dw := make([]int, try)

		row0BindingCount := try - 1
		if row0BindingCount > n {
			row0BindingCount = n
		}
		for c := 0; c < row0BindingCount; c++ {
			b := m.bindings[c]
			if w := lipgloss.Width(b.Help().Key); w > kw[c] {
				kw[c] = w
			}
			if w := lipgloss.Width(b.Help().Desc); w > dw[c] {
				dw[c] = w
			}
		}
		// Affordance always occupies the last cell of row 0.
		affCol := try - 1
		if affKeyW > kw[affCol] {
			kw[affCol] = affKeyW
		}
		if affDescW > dw[affCol] {
			dw[affCol] = affDescW
		}

		remaining := n - row0BindingCount
		if remaining > 0 {
			extraRows := (remaining + try - 1) / try
			for r := 0; r < extraRows; r++ {
				for c := 0; c < try; c++ {
					idx := row0BindingCount + r*try + c
					if idx >= n {
						break
					}
					b := m.bindings[idx]
					if w := lipgloss.Width(b.Help().Key); w > kw[c] {
						kw[c] = w
					}
					if w := lipgloss.Width(b.Help().Desc); w > dw[c] {
						dw[c] = w
					}
				}
			}
		}

		total := 0
		for c := 0; c < try; c++ {
			total += kw[c] + 1 + dw[c] // 1 for key↔desc spacer
		}
		total += (try - 1) * sepVis
		if total > width {
			break
		}
		chosen = try
		chosenKW = kw
		chosenDW = dw
		chosenRow0 = row0BindingCount
	}
	return chosen, chosenKW, chosenDW, chosenRow0
}

// fillToWidth pads s on the right with DescStyle-rendered spaces so
// its visible width matches width. Used by row renderers so the
// background color persists across the full row.
func (m Model) fillToWidth(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + m.descStyle.Render(strings.Repeat(" ", gap))
}

// padRight pads s with plain spaces to reach width visible cells.
// Used for in-cell key/desc alignment where the surrounding style is
// applied to the whole cell.
func padRight(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// Count reports how many bindings the model currently holds.
func (m Model) Count() int { return len(m.bindings) }

func (m Model) Init() tea.Cmd                           { return nil }
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd)       { return m, nil }
func (m *Model) SetDimensions(w, h int)                 { m.width, m.height = w, h }
func (m *Model) SetBindings(b []key.Binding)            { m.bindings = Compile(b) }
func (m Model) Width() int                              { return m.width }
func (m Model) Height() int                             { return m.height }

// View renders the overlay as a bordered box.
func (m Model) View() string {
	innerW := max(0, m.width-2)
	rows := max(1, m.height-2)

	var (
		pairs []string
		used  int
	)
	for i := 0; i < len(m.bindings); i += rows {
		end := min(i+rows, len(m.bindings))
		var keys, descs []string
		for _, b := range m.bindings[i:end] {
			keys = append(keys, m.keyStyle.Render(b.Help().Key))
			descs = append(descs, m.descStyle.Render(b.Help().Desc))
		}
		var cols []string
		if len(pairs) > 0 {
			cols = append(cols, m.spacer)
		}
		cols = append(cols,
			strings.Join(keys, "\n"),
			strings.Join(descs, "\n"),
		)
		pair := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
		if used+lipgloss.Width(pair) > innerW {
			break
		}
		pairs = append(pairs, pair)
		used += lipgloss.Width(pair)
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, pairs...)
	return lipgloss.NewStyle().
		Border(m.border).
		BorderForeground(m.borderColor).
		Height(rows).
		Width(innerW).
		Render(content)
}

// Compile flattens multiple binding groups into one, removing duplicates.
// Bindings are considered duplicates when their Keys() match.
func Compile(groups ...[]key.Binding) []key.Binding {
	seen := make(map[string]struct{})
	out := make([]key.Binding, 0)
	for _, g := range groups {
		for _, b := range g {
			k := strings.Join(b.Keys(), " ")
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, b)
		}
	}
	return out
}
