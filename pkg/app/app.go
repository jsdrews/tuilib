// Package app is the standard TUI shell: a breadcrumb header, a flex body
// that renders the active screen's layout, and a statusbar footer. It owns
// the nav stack, cycles themes, and routes global keys (quit, theme swap)
// around any screen-owned input focus.
//
// Callers provide a root screen and a theme list; the app handles the rest.
// Screens implement pkg/screen.Screen and return their own layout trees,
// so each screen can have a different body composition without any shell
// changes.
package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/screen"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// Options configures the app shell.
type Options struct {
	// Root is the root screen — the bottom of the nav stack. Required.
	Root screen.Screen

	// Themes is the list the user cycles through. At least one entry is
	// required; Themes[0] is the initial theme.
	Themes []theme.Theme

	// Version is rendered on the right side of the statusbar.
	Version string

	// QuitKey quits the program when the stack depth is 1 and no screen
	// is capturing keys. Defaults to "q" (and "ctrl+c").
	QuitKey key.Binding

	// ThemeKey cycles themes. Leave zero to disable cycling (useful when
	// the app is pinned to a single theme).
	ThemeKey key.Binding

	// DisableAutoEscPop turns off automatic esc→pop handling. When false
	// (the default) esc pops the stack whenever depth > 1 and the active
	// screen is not capturing keys.
	DisableAutoEscPop bool
}

// Model is the app shell. Instantiate with New and pass to tea.NewProgram.
type Model struct {
	w, h int

	themes   []theme.Theme
	themeIdx int
	version  string

	stack screen.Stack

	quitKey, themeKey key.Binding
	autoEscPop        bool

	bc breadcrumb.Model
	sb statusbar.Model
}

// New constructs an app shell. The root screen's SetTheme is called with
// Themes[0] so the root renders in the initial palette immediately.
func New(opts Options) Model {
	if opts.Root == nil {
		panic("app.New: Options.Root is required")
	}
	if len(opts.Themes) == 0 {
		opts.Themes = []theme.Theme{theme.Dark()}
	}
	if opts.QuitKey.Keys() == nil {
		opts.QuitKey = key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		)
	}

	t := opts.Themes[0]
	opts.Root.SetTheme(t)

	m := Model{
		themes:     opts.Themes,
		themeIdx:   0,
		version:    opts.Version,
		stack:      screen.NewStack(opts.Root),
		quitKey:    opts.QuitKey,
		themeKey:   opts.ThemeKey,
		autoEscPop: !opts.DisableAutoEscPop,
	}
	m.apply()
	return m
}

// Init runs the root screen's Init + OnEnter(nil).
func (m Model) Init() tea.Cmd { return m.stack.Init() }

// theme returns the currently-active palette.
func (m Model) theme() theme.Theme { return m.themes[m.themeIdx] }

// apply rebuilds breadcrumb + statusbar from the current theme and stack.
// Called on init, resize, theme swap, and any stack mutation so the
// breadcrumb and help hints stay in sync.
func (m *Model) apply() {
	t := m.theme()

	bcOpts := t.Breadcrumb()
	bcOpts.Width = m.w
	bcOpts.Crumbs = m.stack.Crumbs()
	m.bc = breadcrumb.New(bcOpts)

	h := help.New(t.Help())
	if cur := m.stack.Current(); cur != nil {
		h.SetBindings(cur.Help())
	}

	sbOpts := t.Statusbar(h.ShortView(), m.version)
	sbOpts.Width = m.w
	m.sb = statusbar.New(sbOpts)
}

// Update handles resize, global keys, and forwards everything else to the
// stack. Global keys are suppressed when the active screen reports
// IsCapturingKeys so text input isn't hijacked.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.apply()
		return m, nil

	case tea.KeyMsg:
		cur := m.stack.Current()
		capturing := cur != nil && cur.IsCapturingKeys()
		if !capturing {
			if key.Matches(msg, m.quitKey) && m.stack.Depth() == 1 {
				return m, tea.Quit
			}
			if m.themeKey.Keys() != nil && key.Matches(msg, m.themeKey) {
				m.themeIdx = (m.themeIdx + 1) % len(m.themes)
				m.stack.SetTheme(m.theme())
				m.apply()
				return m, nil
			}
			if m.autoEscPop && msg.String() == "esc" && m.stack.Depth() > 1 {
				var cmd tea.Cmd
				m.stack, cmd = m.stack.Update(screen.PopMsg{Result: nil})
				m.apply()
				return m, cmd
			}
		}
	}

	var cmd tea.Cmd
	m.stack, cmd = m.stack.Update(msg)
	m.apply()
	return m, cmd
}

// View composes the standard shell as a vertical stack: breadcrumb (1 row),
// active screen's body layout (flex), statusbar (1 row). The screen never
// knows its own terminal dimensions — the layout engine hands it a body
// rect to fill.
func (m Model) View() string {
	if m.w == 0 {
		return ""
	}

	var body layout.Node
	if cur := m.stack.Current(); cur != nil {
		body = cur.Layout()
	} else {
		body = layout.RenderFunc(func(w, h int) string { return "" })
	}

	root := layout.VStack(
		layout.Fixed(1, layout.RenderFunc(func(w, _ int) string {
			m.bc.SetWidth(w)
			return m.bc.View()
		})),
		layout.Flex(1, body),
		layout.Fixed(1, layout.RenderFunc(func(w, _ int) string {
			m.sb.SetWidth(w)
			return m.sb.View()
		})),
	)
	return root.Render(m.w, m.h)
}

// Theme exposes the app's current palette for screens that need it outside
// of SetTheme (rare — most screens just cache the theme they were last told
// about).
func (m Model) Theme() theme.Theme { return m.theme() }
