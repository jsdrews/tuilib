// Package help renders key-binding hints in two shapes:
//
//   - Model — the one-line footer the app shell paints into the
//     statusbar's left slot. In minimal mode (the default) it is just the
//     "? help" affordance; in verbose mode bindings are tight-packed
//     inline until they overflow, and the affordance is appended when
//     they do. The affordance is the source of truth for whether the help
//     key does anything: when it isn't drawn, the key is inert.
//   - Overlay — the modal reference the affordance opens: a bordered,
//     scrollable, searchable list of every binding the active screen
//     exposes, grouped into Sections. See overlay.go.
//
// The split is deliberate. A footer answers "what can I press right now"
// in the space of one line, and stops being able to answer it somewhere
// around a dozen bindings; the overlay answers "what can I press at all",
// which needs grouping and room to scroll. Trying to make one shape do
// both is what the expanded footer panel was, and it inherited the
// footer's flat binding list — the part that doesn't scale.
//
// Components that want to contribute their own bindings can implement the
// Provider interface; the parent collects bindings from the focused child
// and passes them in via SetBindings before rendering.
package help

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Provider is implemented by components that want to surface extra key
// bindings in the help overlay when focused.
type Provider interface {
	HelpBindings() []key.Binding
}

// Options configures the footer. Zero-value fields fall back to defaults.
type Options struct {
	// KeyStyle is applied to the key column (left side of each pair).
	KeyStyle lipgloss.Style
	// DescStyle is applied to the description column.
	DescStyle lipgloss.Style
	// ShortSeparator is placed between bindings in ShortView. Defaults to
	// "  •  ".
	ShortSeparator string
	// Minimal collapses the footer to just the "? help" / "? close"
	// affordance regardless of how many bindings the model holds — the
	// inline strip is hidden and pressing the help key is the only way
	// to see hints. Set via SetMinimal at runtime.
	Minimal bool
}

// Model renders the footer strip. Call SetBindings whenever the active
// binding set changes; both render methods read from the same compiled
// list. Whether the overlay is open is the host's state, mirrored here via
// SetOpen so the affordance can say "close".
type Model struct {
	bindings []key.Binding

	keyStyle, descStyle lipgloss.Style
	shortSep            string

	open    bool
	minimal bool
}

// New constructs a footer.
func New(opts Options) Model {
	if opts.ShortSeparator == "" {
		opts.ShortSeparator = "  •  "
	}
	return Model{
		keyStyle:  opts.KeyStyle,
		descStyle: opts.DescStyle,
		shortSep:  opts.ShortSeparator,
		minimal:   opts.Minimal,
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
// cells. In minimal mode the line is the affordance alone. Otherwise
// bindings are tight-packed inline until the next one would not fit, and
// the affordance is appended on overflow.
//
// consumed reports how many bindings were placed on the line. overflow
// reports whether any were dropped — in minimal mode, whether there are
// any at all, since none of them are shown. Width 0 or less skips
// budgeting and falls back to ShortView.
func (m Model) ShortViewBudget(width int) (line string, consumed int, overflow bool) {
	if len(m.bindings) == 0 {
		return "", 0, false
	}
	if width <= 0 {
		return m.ShortView(), len(m.bindings), false
	}
	if m.minimal {
		return m.minimalRow(width)
	}

	spacer := m.descStyle.Render(" ")
	sep := m.descStyle.Render(m.shortSep)
	sepVis := lipgloss.Width(sep)

	affordanceText := m.keyStyle.Render("?") + spacer + m.descStyle.Render(m.affordanceLabel())
	// Reserve space for whichever affordance label is wider ("close" today)
	// so opening the overlay doesn't shuffle which bindings the footer
	// shows underneath it.
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

// minimalRow renders the footer in minimal mode: just the affordance
// pair ("? help", or "? close" while the overlay is up), padded to width
// so the statusbar's background fills the full slot. Overflow is true
// whenever there are any bindings, so the app shell keeps the help key
// live as the only path to them.
func (m Model) minimalRow(width int) (line string, consumed int, overflow bool) {
	spacer := m.descStyle.Render(" ")
	cell := m.keyStyle.Render("?") + spacer + m.descStyle.Render(m.affordanceLabel())
	line = m.fillToWidth(cell, width)
	return line, 0, len(m.bindings) > 0
}

func (m Model) affordanceLabel() string {
	if m.open {
		return "close"
	}
	return "help"
}

// Open reports whether the host is showing the overlay.
func (m Model) Open() bool { return m.open }

// SetOpen tells the footer whether the overlay is up, which is all the
// affordance needs to know to say "close" instead of "help".
func (m *Model) SetOpen(b bool) { m.open = b }

// SetMinimal flips minimal-footer mode at runtime. See Options.Minimal.
func (m *Model) SetMinimal(b bool) { m.minimal = b }

// Minimal reports whether the footer is in minimal mode.
func (m Model) Minimal() bool { return m.minimal }

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

// SetBindings replaces the footer's binding list, deduped by keys.
func (m *Model) SetBindings(b []key.Binding) { m.bindings = Compile(b) }

// AffordanceSpan reports where the "? help" / "? close" affordance sits
// within the footer line rendered at width: its start offset in cells and
// its width. ok is false when no affordance is drawn. The app shell uses it
// to route a click there to the same toggle the help key drives.
//
// The offset is not simply "the end of the line": minimal mode renders the
// affordance first and pads after it, while the verbose flow appends it
// last. Getting this from one place keeps a click landing on the glyph the
// user sees.
func (m Model) AffordanceSpan(width int) (start, w int, ok bool) {
	if len(m.bindings) == 0 || width <= 0 {
		return 0, 0, false
	}
	spacerW := lipgloss.Width(m.descStyle.Render(" "))
	naturalW := lipgloss.Width(m.keyStyle.Render("?")) + spacerW +
		lipgloss.Width(m.descStyle.Render(m.affordanceLabel()))

	if m.minimal {
		return 0, naturalW, true
	}
	line, _, overflow := m.ShortViewBudget(width)
	if !overflow {
		return 0, 0, false
	}
	return lipgloss.Width(line) - naturalW, naturalW, true
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
