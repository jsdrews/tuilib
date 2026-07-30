// Package breadcrumb renders a single-line breadcrumb trail like
// "Root › Modules › dev" for use as a TUI header bar. It is a pure view
// component — feed it []string and it returns a styled line.
//
// The convention is: all crumbs except the last are "past" (drawn with the
// crumb style); the last is "current" (drawn with the current style, usually
// bold). When the rendered line exceeds width, leftmost crumbs are replaced
// with an ellipsis so the current crumb is always visible.
package breadcrumb

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jsdrews/tuilib/pkg/geom"
)

// Options configures a breadcrumb bar. Style fields are pointers; pass nil
// to accept the default.
type Options struct {
	Width  int
	Crumbs []string

	// Separator is rendered between crumbs. Defaults to " › ".
	Separator string
	// Ellipsis is substituted for truncated leading crumbs. Defaults to "…".
	Ellipsis string

	// BarBackground / BarForeground set the single color pair used for the
	// whole strip; default styles derive from them so the bar reads as one
	// continuous band. Defaults: bg=236, fg=252.
	BarBackground lipgloss.TerminalColor
	BarForeground lipgloss.TerminalColor

	// Style overrides. nil = use defaults derived from BarBackground /
	// BarForeground.
	BarStyle       *lipgloss.Style // outer padding + bg
	CrumbStyle     *lipgloss.Style // past crumbs
	CurrentStyle   *lipgloss.Style // last crumb
	SeparatorStyle *lipgloss.Style // between-crumb separator
}

// Model is a breadcrumb bar. Call SetCrumbs when the navigation stack changes
// and SetWidth on window resize.
type Model struct {
	rect geom.Rect

	width     int
	crumbs    []string
	separator string
	ellipsis  string

	barStyle       lipgloss.Style
	crumbStyle     lipgloss.Style
	currentStyle   lipgloss.Style
	separatorStyle lipgloss.Style

	barBG lipgloss.TerminalColor
	barFG lipgloss.TerminalColor
}

// New constructs a breadcrumb bar.
func New(opts Options) Model {
	if opts.Separator == "" {
		opts.Separator = " › "
	}
	if opts.Ellipsis == "" {
		opts.Ellipsis = "…"
	}
	if opts.BarBackground == nil {
		opts.BarBackground = lipgloss.Color("236")
	}
	if opts.BarForeground == nil {
		opts.BarForeground = lipgloss.Color("252")
	}

	defBar := lipgloss.NewStyle().
		Padding(0, 1).
		Background(opts.BarBackground).
		Foreground(opts.BarForeground)
	defCrumb := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		Background(opts.BarBackground)
	defCurrent := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("255")).
		Background(opts.BarBackground)
	defSep := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Background(opts.BarBackground)

	pick := func(p *lipgloss.Style, def lipgloss.Style) lipgloss.Style {
		if p != nil {
			return *p
		}
		return def
	}

	return Model{
		width:          opts.Width,
		crumbs:         opts.Crumbs,
		separator:      opts.Separator,
		ellipsis:       opts.Ellipsis,
		barStyle:       pick(opts.BarStyle, defBar),
		crumbStyle:     pick(opts.CrumbStyle, defCrumb),
		currentStyle:   pick(opts.CurrentStyle, defCurrent),
		separatorStyle: pick(opts.SeparatorStyle, defSep),
		barBG:          opts.BarBackground,
		barFG:          opts.BarForeground,
	}
}

func (m Model) Init() tea.Cmd                     { return nil }
func (m Model) Update(_ tea.Msg) (Model, tea.Cmd) { return m, nil }
func (m *Model) SetCrumbs(c []string)             { m.crumbs = c }
func (m *Model) SetRect(r geom.Rect)              { m.rect = r; m.width = r.W }
func (m Model) Rect() geom.Rect                   { return m.rect }
func (m Model) Width() int                        { return m.width }

// BarBackground reports the bar's base background, useful if you're matching
// another component (e.g. a footer) to the same color.
func (m Model) BarBackground() lipgloss.TerminalColor { return m.barBG }
func (m Model) BarForeground() lipgloss.TerminalColor { return m.barFG }

// View renders the breadcrumb as a single line, padded and styled to Width.
func (m Model) View() string {
	if len(m.crumbs) == 0 {
		return m.barStyle.Width(m.width).Render("")
	}

	styled := m.renderCrumbs(m.crumbs)

	// Truncate from the left if too wide. Account for bar padding (Padding
	// 0,1 on each side => 2 chars overhead) so the final line fits the width.
	avail := m.width - 2
	if avail > 0 && lipgloss.Width(styled) > avail {
		styled = m.truncateLeft(m.crumbs, avail)
	}

	return m.barStyle.Width(m.width).Render(styled)
}

// renderCrumbs styles crumbs and joins them with the separator.
func (m Model) renderCrumbs(crumbs []string) string {
	parts := make([]string, 0, len(crumbs))
	for i, c := range crumbs {
		if i == len(crumbs)-1 {
			parts = append(parts, m.currentStyle.Render(c))
		} else {
			parts = append(parts, m.crumbStyle.Render(c))
		}
	}
	return strings.Join(parts, m.separatorStyle.Render(m.separator))
}

// elideStart returns the index of the first crumb that fits when leading
// crumbs are dropped in favour of an ellipsis, and whether any such split
// works at all. It is the single decision behind both what View draws and
// what CrumbAt resolves a click to — computing it twice is how a breadcrumb
// ends up navigating somewhere other than the crumb the user clicked.
func (m Model) elideStart(crumbs []string, avail int) (start int, ok bool) {
	for s := 1; s < len(crumbs); s++ {
		kept := append([]string{m.ellipsis}, crumbs[s:]...)
		if lipgloss.Width(m.renderCrumbs(kept)) <= avail {
			return s, true
		}
	}
	return 0, false
}

// truncateLeft drops leading crumbs and prepends an ellipsis until the result
// fits within avail. The current (last) crumb is always preserved.
func (m Model) truncateLeft(crumbs []string, avail int) string {
	if start, ok := m.elideStart(crumbs, avail); ok {
		// The ellipsis renders in crumb style — it's a "past" placeholder,
		// which is what renderCrumbs applies to every entry but the last.
		return m.renderCrumbs(append([]string{m.ellipsis}, crumbs[start:]...))
	}
	// Last resort: just the current crumb, possibly further truncated.
	last := crumbs[len(crumbs)-1]
	if lipgloss.Width(last) > avail {
		if avail > 1 {
			last = last[:avail-1] + m.ellipsis
		} else {
			last = m.ellipsis
		}
	}
	return m.currentStyle.Render(last)
}

// CrumbAt maps a position to an index into the crumbs the breadcrumb was
// last given, accounting for the bar's padding and for any leading crumbs
// elided behind the ellipsis. Reports ok=false for the separators, the
// ellipsis itself, the trailing empty stretch, and any position outside the
// bar — clicking dead space should do nothing rather than navigate.
func (m Model) CrumbAt(x, y int) (int, bool) {
	if len(m.crumbs) == 0 || !m.rect.Hit(x, y) {
		return 0, false
	}

	// Mirror View: draw everything unless it overflows, then elide.
	labels, src, elided := m.crumbs, 0, false
	if avail := m.width - 2; avail > 0 && lipgloss.Width(m.renderCrumbs(m.crumbs)) > avail {
		start, ok := m.elideStart(m.crumbs, avail)
		if !ok {
			// Only the current crumb is drawn, and it is where we already
			// are — nothing to navigate to.
			return 0, false
		}
		labels = append([]string{m.ellipsis}, m.crumbs[start:]...)
		// labels[0] is the ellipsis, so labels[i] maps to crumbs[start+i-1].
		src, elided = start-1, true
	}

	at := m.rect.X + m.barStyle.GetPaddingLeft()
	sepW := lipgloss.Width(m.separator)
	for i, label := range labels {
		w := lipgloss.Width(label)
		if x >= at && x < at+w {
			// The ellipsis stands for several elided crumbs, so it has no
			// single target — even though src+0 happens to be in range.
			if elided && i == 0 {
				return 0, false
			}
			return src + i, true
		}
		at += w + sepW
	}
	return 0, false
}
