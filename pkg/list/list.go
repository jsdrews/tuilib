// Package list provides a cursor-driven, optionally filterable list inside a
// bordered pane. It bundles item storage, cursor tracking, viewport auto-
// scroll, and a filter.Model together so parents can drop it in with one
// New + Update + View.
//
// Items are plain []string — callers format their data before passing it in.
// For filtering, the match is a case-insensitive substring across the item
// text. Anything richer (fuzzy match, per-field search, struct items) is out
// of scope and should be composed via pane + filter directly.
package list

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/pane"
)

// Options configures a new list. Zero-value fields fall back to sane defaults
// where that's meaningful; otherwise the pane/filter defaults apply.
type Options struct {
	Width, Height int
	// Title sits on the pane's top-left border slot. Defaults to "List".
	Title string
	// Items is the full item set. The list copies this slice so the caller
	// can mutate their source independently.
	Items []string
	// Filterable embeds a filter.Model above the body pane (three rows). If
	// false, "/" is ignored and the full height is used for items.
	Filterable bool

	// Pane pass-throughs. See pkg/pane.Options for defaults.
	ActiveColor    lipgloss.TerminalColor
	InactiveColor  lipgloss.TerminalColor
	ActiveBorder   lipgloss.Border
	InactiveBorder lipgloss.Border
	SlotBrackets   pane.SlotBracketStyle

	// SelectedColor foregrounds the highlighted row (bold).
	SelectedColor lipgloss.TerminalColor

	// Filter configures the embedded filter. Ignored when Filterable=false.
	Filter filter.Options
}

// Model is the list widget. Embed as a value; mutate via the setters.
type Model struct {
	items   []string
	visible []string
	cursor  int

	filter     filter.Model
	filterable bool

	body          pane.Pane
	selectedStyle lipgloss.Style
}

var keys = struct {
	Up, Down, Filter key.Binding
}{
	Up:     key.NewBinding(key.WithKeys("up", "k")),
	Down:   key.NewBinding(key.WithKeys("down", "j")),
	Filter: key.NewBinding(key.WithKeys("/")),
}

// New constructs a list. Call Update/View from the parent model.
func New(opts Options) Model {
	if opts.Title == "" {
		opts.Title = "List"
	}
	m := Model{
		items:         append([]string(nil), opts.Items...),
		filterable:    opts.Filterable,
		selectedStyle: lipgloss.NewStyle().Bold(true).Foreground(opts.SelectedColor),
	}
	m.visible = m.items

	bodyH := opts.Height
	if m.filterable {
		bodyH = max(0, opts.Height-3) // filter.Model is 3 rows tall.
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
	})
	m.refresh()
	return m
}

func (m *Model) applyFilter() {
	if !m.filterable {
		m.visible = m.items
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		m.visible = m.items
	} else {
		out := make([]string, 0, len(m.items))
		for _, it := range m.items {
			if strings.Contains(strings.ToLower(it), q) {
				out = append(out, it)
			}
		}
		m.visible = out
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func (m *Model) refresh() {
	var b strings.Builder
	for i, it := range m.visible {
		if i == m.cursor {
			b.WriteString(m.selectedStyle.Render("▸ " + it))
		} else {
			b.WriteString("  " + it)
		}
		b.WriteString("\n")
	}
	m.body.SetContent(strings.TrimRight(b.String(), "\n"))
	m.body.EnsureVisible(m.cursor)
	if m.filterable {
		m.body.SetBottomLeft(fmt.Sprintf("%d / %d", len(m.visible), len(m.items)))
	}
}

// Init satisfies tea.Model — nothing to kick off.
func (m Model) Init() tea.Cmd { return nil }

// Update consumes up/down/j/k and "/" (when filterable); while the filter is
// focused, every key is forwarded to it. Non-list keys pass through the
// caller untouched.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.filterable && m.filter.Focused() {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		m.refresh()
		return m, cmd
	}
	switch {
	case m.filterable && key.Matches(km, keys.Filter):
		return m, m.filter.Focus()
	case key.Matches(km, keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.refresh()
		}
	case key.Matches(km, keys.Down):
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.refresh()
		}
	}
	return m, nil
}

// View stacks filter (if filterable) and the body pane.
func (m Model) View() string {
	if m.filterable {
		return m.filter.View() + "\n" + m.body.View()
	}
	return m.body.View()
}

// Selected returns the currently highlighted item. ok is false when the
// visible set (post-filter) is empty.
func (m Model) Selected() (string, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return "", false
	}
	return m.visible[m.cursor], true
}

// Cursor returns the current cursor index into the visible (post-filter) set.
func (m Model) Cursor() int { return m.cursor }

// Visible returns the post-filter items, in display order.
func (m Model) Visible() []string { return m.visible }

// Items returns the full unfiltered item set.
func (m Model) Items() []string { return m.items }

// Filtering reports whether the embedded filter currently has focus —
// callers use this to decide whether to intercept global keys like "q".
func (m Model) Filtering() bool { return m.filterable && m.filter.Focused() }

// Help returns the keys this list responds to. While the embedded filter
// is focused it returns the filter's keys; otherwise ↑↓ (always) plus "/"
// when filterable.
func (m Model) Help() []key.Binding {
	if m.filterable && m.filter.Focused() {
		return m.filter.Help()
	}
	out := []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "move")),
	}
	if m.filterable {
		out = append(out, key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")))
	}
	return out
}

// Value returns the current filter text ("" when not filterable or empty).
func (m Model) Value() string {
	if m.filterable {
		return m.filter.Value()
	}
	return ""
}

// SetDimensions resizes the list in place. When filterable, the internal
// filter pane consumes 3 rows at the top and the body pane gets the rest;
// otherwise the body pane takes the full height.
func (m *Model) SetDimensions(w, h int) {
	bodyH := h
	if m.filterable {
		m.filter.SetWidth(w)
		bodyH = max(0, h-3)
	}
	m.body.SetDimensions(w, bodyH)
	m.refresh()
}

// SetItems replaces the item set, re-applies the current filter, and redraws.
func (m *Model) SetItems(items []string) {
	m.items = append([]string(nil), items...)
	m.applyFilter()
	m.refresh()
}

// SetCursor moves the cursor (clamped to the visible range) and scrolls to
// keep it on screen.
func (m *Model) SetCursor(n int) {
	m.cursor = max(0, min(n, len(m.visible)-1))
	m.refresh()
}

// SetValue overwrites the filter text (no-op when not filterable). Useful
// when rebuilding the list on theme swap / resize — carry the old Value().
func (m *Model) SetValue(s string) {
	if !m.filterable {
		return
	}
	m.filter.SetValue(s)
	m.applyFilter()
	m.refresh()
}

// SetFocused sets the body pane's focus state so its border reads as
// active or inactive. Useful when embedding a list inside a parent that
// owns focus (e.g. a form field that gates input).
func (m *Model) SetFocused(b bool) { m.body.SetFocused(b) }

// SetActiveColor updates the body pane's active border color. Useful when
// reacting to a theme swap without rebuilding the model.
func (m *Model) SetActiveColor(c lipgloss.TerminalColor) { m.body.SetActiveColor(c) }

// SetInactiveColor updates the body pane's inactive border color.
func (m *Model) SetInactiveColor(c lipgloss.TerminalColor) { m.body.SetInactiveColor(c) }

// SetSelectedColor updates the foreground color of the highlighted row.
func (m *Model) SetSelectedColor(c lipgloss.TerminalColor) {
	m.selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(c)
	m.refresh()
}
