// Package help renders key-binding hints in three shapes:
//
//   - ShortView / ShortViewBudget — a single inline line, "key desc • key
//     desc • …", sized to fit a width budget. The app shell paints this
//     into the statusbar's left slot. When the bindings don't all fit,
//     ShortViewBudget truncates and appends a "? help" affordance and
//     reports overflow + how many bindings landed on the line (consumed).
//   - ExpandedView / ExpandedRows — a multi-row continuation of the
//     footer, picking up at startIdx and wrapping with the same " • "
//     separator across as many rows as needed (clamped to maxRows). The
//     app shell renders this in a Fixed row above the statusbar when
//     m.Expanded() is true.
//   - View — a legacy bordered overlay (kept for parents that want a
//     standalone popup); uses key/desc column-pair flowing.
//
// The footer + panel together act as one component: the panel literally
// continues the bindings the footer ran out of room for. Pressing the
// app's HelpKey ("?" by default) flips m.Expanded(); the shell only
// listens to the key when there's overflow, so when everything fits the
// affordance is hidden and the key is inert.
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

// ShortViewBudget renders bindings inline, fitting within width visible
// cells. When some bindings would not fit, truncates and appends a
// "? help" affordance to signal the rest is available via the expanded
// panel; when m.Expanded() is already true the affordance reads
// "? close" regardless of whether truncation happened. consumed reports
// how many bindings were placed on the line — ExpandedView uses it as
// the starting index when continuing the strip across wrap rows.
// overflow reports whether bindings were dropped because they didn't
// fit. Width 0 or less skips truncation and falls back to ShortView.
func (m Model) ShortViewBudget(width int) (line string, consumed int, overflow bool) {
	if len(m.bindings) == 0 {
		return "", 0, false
	}
	if width <= 0 {
		return m.ShortView(), len(m.bindings), false
	}
	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)
	sepVis := lipgloss.Width(sep)

	affordanceDesc := "help"
	if m.expanded {
		affordanceDesc = "close"
	}
	affordanceText := m.keyStyle.Render("?") + spacer + m.descStyle.Render(affordanceDesc)
	// Reserve space for whichever affordance label is wider ("close" today)
	// so toggling expanded doesn't shuffle which bindings the footer shows.
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
		// Reserve room for the affordance only when more bindings could
		// follow — fitting the final binding without an affordance is
		// the non-overflow case.
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
	if overflow || m.expanded {
		if line != "" {
			line += sep
		}
		line += affordanceText
	}
	return line, consumed, overflow
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

// ExpandedRows reports how many rows ExpandedView needs to render the
// remaining bindings starting at startIdx, flowing them across width
// using the same " • " separator as the inline footer line. The result
// is clamped to maxRows; when the remainder fits in fewer rows, the
// smaller count is returned. Width or maxRows ≤ 0, or startIdx past
// the end of the binding list, yields 0.
func (m Model) ExpandedRows(width, startIdx, maxRows int) int {
	if width <= 0 || maxRows <= 0 || startIdx >= len(m.bindings) {
		return 0
	}
	spacer := m.descStyle.Render(" ")
	sepVis := lipgloss.Width(m.descStyle.Render(m.shortSep))

	rows := 1
	used := 0
	onLine := 0
	for i := startIdx; i < len(m.bindings); i++ {
		b := m.bindings[i]
		pair := m.keyStyle.Render(b.Help().Key) + spacer + m.descStyle.Render(b.Help().Desc)
		add := lipgloss.Width(pair)
		if onLine > 0 {
			add += sepVis
		}
		if used+add > width {
			rows++
			if rows > maxRows {
				return maxRows
			}
			used = lipgloss.Width(pair)
			onLine = 1
			continue
		}
		used += add
		onLine++
	}
	return rows
}

// ExpandedView renders the bindings from startIdx onward as wrapped
// " • "-separated rows that visually continue the inline footer. Uses
// the same KeyStyle / DescStyle / shortSep as ShortView so the panel
// reads as one flowing strip, and pads each row to width with
// DescStyle so any background color set on DescStyle fills the row
// (matching how the statusbar's slot style fills its column). Stops
// when it has filled rows lines or run out of bindings, whichever
// comes first. Draws no border — the caller composes it directly
// (typically a Fixed row above the statusbar).
func (m Model) ExpandedView(width, rows, startIdx int) string {
	if width <= 0 || rows <= 0 || startIdx >= len(m.bindings) {
		return ""
	}
	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)
	sepVis := lipgloss.Width(sep)

	fill := func(s string) string {
		pad := width - lipgloss.Width(s)
		if pad <= 0 {
			return s
		}
		return s + m.descStyle.Render(strings.Repeat(" ", pad))
	}

	var lines []string
	var current []string
	used := 0
	for i := startIdx; i < len(m.bindings); i++ {
		b := m.bindings[i]
		pair := m.keyStyle.Render(b.Help().Key) + spacer + m.descStyle.Render(b.Help().Desc)
		add := lipgloss.Width(pair)
		if len(current) > 0 {
			add += sepVis
		}
		if used+add > width {
			if len(current) > 0 {
				lines = append(lines, fill(strings.Join(current, sep)))
			}
			if len(lines) >= rows {
				break
			}
			current = []string{pair}
			used = lipgloss.Width(pair)
			continue
		}
		current = append(current, pair)
		used += add
	}
	if len(current) > 0 && len(lines) < rows {
		lines = append(lines, fill(strings.Join(current, sep)))
	}
	return strings.Join(lines, "\n")
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
