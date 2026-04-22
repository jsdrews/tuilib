// Package help renders a toggleable help overlay showing key bindings,
// modeled on pug's help widget. Pass in a []key.Binding and get back a
// bordered string that shows key/description columns flowed to fit the
// available width.
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

// Model renders a help overlay. Call SetBindings to change what it shows and
// SetDimensions on window resize. Visibility is the parent's concern: render
// the View() string only when help should appear.
type Model struct {
	width, height int
	bindings      []key.Binding

	keyStyle, descStyle lipgloss.Style
	border              lipgloss.Border
	borderColor         lipgloss.TerminalColor
	spacer              string
	shortSep            string
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
