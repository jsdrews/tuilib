// Package tab provides a tabbed container that hosts multiple screen.Screen
// bodies inside one parent screen. Each tab keeps its own state across
// switches; the active tab's body fills the area below a single-row strip.
//
// A tab pane is not a navigation level — it lives entirely inside one screen
// and never touches the screen.Stack. The host screen forwards a small set of
// callbacks to tab.Model:
//
//	host.Update(msg)         → tabs.Update(msg)
//	host.OnEnter(result)     → tabs.OnEnterActive(result)
//	host.IsCapturingKeys()   → tabs.IsCapturingKeys()
//	host.SetTheme(t)         → tabs.SetTheme(t)        (fans to every body)
//	host.Help()              → host bindings + tabs.Help()
//	host.Layout()            → layout.Sized(&s.tabs)
//
// If a tab body returns a screen.Push command from its Update, the cmd
// bubbles up through the host into the app stack, the new screen lands on
// top, and the host (with the tab pane intact) is preserved underneath.
// When that child later pops with a result, app.Stack delivers it to the
// host's OnEnter, which forwards via OnEnterActive so the body that
// initiated the push is the one that receives the result.
//
// Tab bodies are full screen.Screen implementations, so anything you'd build
// as a top-level screen drops in unchanged. Only Title() and (usually) Init()
// are unused — Label is taken from the Tab struct, and most bodies do their
// activation work in OnEnter.
package tab

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Tab is one entry in a tab pane.
type Tab struct {
	// Label is rendered in the tab strip.
	Label string
	// Body is the screen rendered below the strip when this tab is active.
	// Title()/Init() are largely unused — see package doc.
	Body screen.Screen
}

// Options configures a tab pane. Theme is used to derive default styles;
// override any of the Style/Separator fields to deviate from the palette.
type Options struct {
	Tabs []Tab
	// Theme drives default strip styling. Required when style overrides
	// are not provided.
	Theme theme.Theme
	// Initial is the starting tab index. Defaults to 0; out-of-range values
	// are clamped to 0.
	Initial int

	// PrevKey / NextKey switch tabs. Defaults: shift+left / shift+right.
	// We deliberately avoid tab/shift+tab — those are reserved for inner
	// pane focus cycling across tuilib examples.
	PrevKey key.Binding
	NextKey key.Binding
	// DisableNumberKeys turns off the 1–9 jump-to-tab shortcut. By default
	// "1" through "9" map to tabs[0]..tabs[8] (when at least 2 tabs exist).
	DisableNumberKeys bool

	// ActiveStyle styles the active tab label (with surrounding spaces).
	ActiveStyle *lipgloss.Style
	// InactiveStyle styles non-active tab labels.
	InactiveStyle *lipgloss.Style
	// SeparatorStyle styles the inter-tab separator.
	SeparatorStyle *lipgloss.Style
	// BarStyle is the strip's outer style — supplies the background fill
	// across the full width so the strip reads as one band.
	BarStyle *lipgloss.Style
	// Separator goes between tab labels. Defaults to " │ ".
	Separator string
}

// Model is the tab pane.
type Model struct {
	tabs   []Tab
	active int
	w, h   int

	activeStyle    lipgloss.Style
	inactiveStyle  lipgloss.Style
	separatorStyle lipgloss.Style
	barStyle       lipgloss.Style
	separator      string

	prevKey    key.Binding
	nextKey    key.Binding
	numberKeys bool

	t theme.Theme
}

// New constructs a tab pane. Panics on empty Tabs (a tab pane with zero
// entries has no body to render).
func New(opts Options) Model {
	if len(opts.Tabs) == 0 {
		panic("tab.New: Options.Tabs must contain at least one entry")
	}
	if opts.Initial < 0 || opts.Initial >= len(opts.Tabs) {
		opts.Initial = 0
	}
	if opts.PrevKey.Keys() == nil {
		opts.PrevKey = key.NewBinding(
			key.WithKeys("shift+left"),
			key.WithHelp("⇧←", "prev tab"),
		)
	}
	if opts.NextKey.Keys() == nil {
		opts.NextKey = key.NewBinding(
			key.WithKeys("shift+right"),
			key.WithHelp("⇧→", "next tab"),
		)
	}
	if opts.Separator == "" {
		opts.Separator = " │ "
	}

	m := Model{
		tabs:       append([]Tab(nil), opts.Tabs...),
		active:     opts.Initial,
		separator:  opts.Separator,
		prevKey:    opts.PrevKey,
		nextKey:    opts.NextKey,
		numberKeys: !opts.DisableNumberKeys,
		t:          opts.Theme,
	}
	m.applyStyles(opts)
	return m
}

// applyStyles fills the four style fields, falling back to theme-derived
// defaults when overrides aren't supplied.
func (m *Model) applyStyles(opts Options) {
	t := opts.Theme

	def := func(p *lipgloss.Style, fallback lipgloss.Style) lipgloss.Style {
		if p != nil {
			return *p
		}
		return fallback
	}

	bg := t.BarBG
	if bg == nil {
		bg = lipgloss.Color("236")
	}
	fg := t.BarFG
	if fg == nil {
		fg = lipgloss.Color("252")
	}
	muted := t.Muted
	if muted == nil {
		muted = lipgloss.Color("244")
	}
	subtle := t.Subtle
	if subtle == nil {
		subtle = lipgloss.Color("240")
	}
	accent := t.Accent
	if accent == nil {
		accent = fg
	}

	defActive := lipgloss.NewStyle().Bold(true).Foreground(accent).Background(bg)
	defInactive := lipgloss.NewStyle().Foreground(muted).Background(bg)
	defSep := lipgloss.NewStyle().Foreground(subtle).Background(bg)
	defBar := lipgloss.NewStyle().Background(bg).Foreground(fg).Padding(0, 1)

	m.activeStyle = def(opts.ActiveStyle, defActive)
	m.inactiveStyle = def(opts.InactiveStyle, defInactive)
	m.separatorStyle = def(opts.SeparatorStyle, defSep)
	m.barStyle = def(opts.BarStyle, defBar)
}

// Init batches every body's Init plus the initial tab's OnEnter(nil).
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.tabs)+1)
	for _, t := range m.tabs {
		if c := t.Body.Init(); c != nil {
			cmds = append(cmds, c)
		}
	}
	if c := m.tabs[m.active].Body.OnEnter(nil); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

// Update handles tab-switch keys (prev/next/numbers, gated by
// IsCapturingKeys on the active body) and forwards messages to the bodies.
//
// Routing differs by message kind:
//
//   - tea.KeyMsg goes to the active body only. Sending keystrokes to every
//     tab would race "/" filters, "j/k" cursors, and similar shortcuts
//     across panes the user can't see.
//   - Every other message fans out to every body, so timers, async fetches,
//     and other background streams in inactive tabs stay alive across
//     switches. (A body's tea.Tick re-arm cmd that fired while another tab
//     was active still reaches it.)
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if !m.tabs[m.active].Body.IsCapturingKeys() {
			switch {
			case key.Matches(k, m.prevKey):
				return m.switchTo(m.active - 1)
			case key.Matches(k, m.nextKey):
				return m.switchTo(m.active + 1)
			}
			if m.numberKeys {
				s := k.String()
				if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
					idx := int(s[0] - '1')
					if idx < len(m.tabs) {
						return m.switchTo(idx)
					}
				}
			}
		}
		body := m.tabs[m.active].Body
		next, cmd := body.Update(msg)
		m.tabs[m.active].Body = next
		return m, cmd
	}

	cmds := make([]tea.Cmd, 0, len(m.tabs))
	for i := range m.tabs {
		next, cmd := m.tabs[i].Body.Update(msg)
		m.tabs[i].Body = next
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// switchTo wraps i into [0, len(tabs)) and fires OnEnter(nil) on the new
// body if the active index actually changed.
func (m Model) switchTo(i int) (Model, tea.Cmd) {
	n := len(m.tabs)
	i = ((i % n) + n) % n
	if i == m.active {
		return m, nil
	}
	m.active = i
	return m, m.tabs[i].Body.OnEnter(nil)
}

// SetDimensions stores the available rect; renderStrip and the body share it.
func (m *Model) SetDimensions(w, h int) { m.w, m.h = w, h }

// View renders the tab strip on the first row and the active body's layout
// in the remaining rows.
func (m Model) View() string {
	strip := m.renderStrip(m.w)
	bodyH := m.h - 1
	if bodyH < 0 {
		bodyH = 0
	}
	body := m.tabs[m.active].Body.Layout().Render(m.w, bodyH)
	if bodyH == 0 {
		return strip
	}
	return strip + "\n" + body
}

func (m Model) renderStrip(w int) string {
	parts := make([]string, 0, len(m.tabs))
	for i, t := range m.tabs {
		label := " " + t.Label + " "
		if i == m.active {
			parts = append(parts, m.activeStyle.Render(label))
		} else {
			parts = append(parts, m.inactiveStyle.Render(label))
		}
	}
	line := strings.Join(parts, m.separatorStyle.Render(m.separator))
	return m.barStyle.Width(w).Render(line)
}

// ActiveTab returns the active tab index.
func (m Model) ActiveTab() int { return m.active }

// ActiveLabel returns the active tab's label — useful for surfacing the tab
// in the host screen's breadcrumb Title().
func (m Model) ActiveLabel() string { return m.tabs[m.active].Label }

// SetActiveTab switches tabs and returns the new body's OnEnter(nil) cmd.
// No-op (returns nil) when i resolves to the already-active tab.
func (m *Model) SetActiveTab(i int) tea.Cmd {
	next, cmd := m.switchTo(i)
	*m = next
	return cmd
}

// IsCapturingKeys reports whether the active body is consuming keystrokes
// (e.g. a focused filter). The host should expose this from its own
// IsCapturingKeys so app's global keys yield correctly.
func (m Model) IsCapturingKeys() bool { return m.tabs[m.active].Body.IsCapturingKeys() }

// OnEnterActive forwards a result into the active body's OnEnter. Use this
// from the host's OnEnter so a child screen popped via screen.Pop(value)
// reaches the body that initiated the push.
func (m Model) OnEnterActive(result any) tea.Cmd {
	return m.tabs[m.active].Body.OnEnter(result)
}

// SetTheme rebuilds the strip styles and fans the new theme out to every
// body, including inactive ones, so a tab the user hasn't visited yet still
// renders in the current palette when they switch to it.
func (m *Model) SetTheme(t theme.Theme) {
	m.t = t
	m.applyStyles(Options{Theme: t})
	for _, tb := range m.tabs {
		tb.Body.SetTheme(t)
	}
}

// Help returns the tab-switch bindings plus the active body's Help, deduped.
func (m Model) Help() []key.Binding {
	own := []key.Binding{m.prevKey, m.nextKey}
	if m.numberKeys && len(m.tabs) > 1 {
		own = append(own, key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
			key.WithHelp("1-9", "tab"),
		))
	}
	return help.Compile(own, m.tabs[m.active].Body.Help())
}
