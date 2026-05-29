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

	// HelpKey toggles the expanded help panel. The panel appears as a
	// multi-row strip above the statusbar showing every binding the
	// active screen currently exposes via Help() — useful when the
	// inline hints don't all fit in one row. Defaults to "?". Set to an
	// empty binding (key.NewBinding()) to disable the panel.
	HelpKey key.Binding

	// HelpMaxRows caps how many rows the expanded help panel may grow
	// to. Defaults to 6. The panel uses only as many rows as needed to
	// fit every binding at the current width, up to this cap.
	HelpMaxRows int

	// HelpVerbose restores the legacy footer behavior: bindings are
	// tight-packed inline in the statusbar's left slot until they
	// overflow. The default (zero value) is minimal mode — the footer
	// shows only the "? help" affordance and pressing HelpKey opens the
	// expanded panel with every binding. Minimal cuts clutter on screens
	// with many bindings (a deep component composition can easily
	// produce 15+) at the cost of inline discoverability.
	HelpVerbose bool

	// ThemeEnvVar names an environment variable consulted for the initial
	// theme during app.New (e.g. "MYAPP_THEME"). When the var is set to a
	// theme's Name, that theme becomes Themes[0]. Empty string disables
	// env-var lookup but the user config file is still consulted; see
	// SkipConfig to disable both.
	ThemeEnvVar string

	// SkipConfig disables the automatic theme.Resolve call inside app.New.
	// By default app.New reorders Themes so Themes[0] reflects the user's
	// $XDG_CONFIG_HOME/tuilib/config.yaml `theme:` field (and ThemeEnvVar,
	// when set) — set this to true if you've already called theme.Resolve
	// yourself, or if you want to pin the app to Themes[0] regardless.
	SkipConfig bool

	// DisableAutoEscPop turns off automatic esc→pop handling. When false
	// (the default) esc pops the stack whenever depth > 1 and the active
	// screen is not capturing keys.
	DisableAutoEscPop bool
}

// SetThemeMsg asks the app to switch to the named theme and rebroadcast it
// across the stack. Emit via SetTheme(name) from any screen.
type SetThemeMsg struct{ Name string }

// SetTheme returns a command that tells the app shell to switch to the theme
// whose Name matches. Useful for a theme-picker screen that wants to preview
// palettes live as the cursor moves. No-op if no theme with that name is in
// the app's Themes list.
func SetTheme(name string) tea.Cmd {
	return func() tea.Msg { return SetThemeMsg{Name: name} }
}

// StatusInfoMsg asks the app to show s as an info message in the statusbar's
// center slot. The message auto-clears on the next KeyMsg (matching the
// statusbar's own Update behavior). Emit via Info(s) from any screen.
type StatusInfoMsg struct{ Text string }

// StatusErrorMsg asks the app to show s as an error message in the
// statusbar's center slot. Same auto-clear semantics as StatusInfoMsg. Emit
// via Error(s) from any screen.
type StatusErrorMsg struct{ Text string }

// StatusClearMsg asks the app to clear any active statusbar message
// immediately. Useful for screens that want to wipe a stale message on a
// non-key event (e.g. a fetch result that resolves cleanly).
type StatusClearMsg struct{}

// Info returns a command that posts an info message to the statusbar.
func Info(s string) tea.Cmd {
	return func() tea.Msg { return StatusInfoMsg{Text: s} }
}

// Error returns a command that posts an error message to the statusbar.
func Error(s string) tea.Cmd {
	return func() tea.Msg { return StatusErrorMsg{Text: s} }
}

// ClearStatus returns a command that clears any active statusbar message.
func ClearStatus() tea.Cmd {
	return func() tea.Msg { return StatusClearMsg{} }
}

// Model is the app shell. Instantiate with New and pass to tea.NewProgram.
type Model struct {
	w, h int

	themes   []theme.Theme
	themeIdx int
	version  string

	stack screen.Stack

	quitKey, themeKey, helpKey key.Binding
	helpMaxRows                int
	helpMinimal                bool
	autoEscPop                 bool

	bc breadcrumb.Model
	sb statusbar.Model

	help         help.Model
	helpExpanded bool
	helpOverflow bool
}

// New constructs an app shell. By default it reorders Themes via
// theme.Resolve so Themes[0] reflects the user's config file (and
// ThemeEnvVar, when set) — pass SkipConfig=true to opt out. The root
// screen's SetTheme is then called with the resulting Themes[0] so the
// root renders in the initial palette immediately.
func New(opts Options) Model {
	if opts.Root == nil {
		panic("app.New: Options.Root is required")
	}
	if len(opts.Themes) == 0 {
		opts.Themes = []theme.Theme{theme.Dark()}
	}
	if !opts.SkipConfig {
		opts.Themes = theme.Resolve(opts.Themes, opts.ThemeEnvVar)
	}
	if opts.QuitKey.Keys() == nil {
		opts.QuitKey = key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		)
	}
	if opts.HelpKey.Keys() == nil {
		opts.HelpKey = key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		)
	}
	if opts.HelpMaxRows <= 0 {
		opts.HelpMaxRows = 6
	}

	t := opts.Themes[0]
	opts.Root.SetTheme(t)

	m := Model{
		themes:      opts.Themes,
		themeIdx:    0,
		version:     opts.Version,
		stack:       screen.NewStack(opts.Root),
		quitKey:     opts.QuitKey,
		themeKey:    opts.ThemeKey,
		helpKey:     opts.HelpKey,
		helpMaxRows: opts.HelpMaxRows,
		helpMinimal: !opts.HelpVerbose,
		autoEscPop:  !opts.DisableAutoEscPop,
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
// breadcrumb and help hints stay in sync. Any in-flight info/error message
// on the statusbar is captured before the rebuild and re-applied after, so
// stack updates don't wipe a screen-emitted status message.
func (m *Model) apply() {
	t := m.theme()

	bcOpts := t.Breadcrumb()
	bcOpts.Width = m.w
	bcOpts.Crumbs = m.stack.Crumbs()
	m.bc = breadcrumb.New(bcOpts)

	helpOpts := t.Help()
	helpOpts.Minimal = m.helpMinimal
	m.help = help.New(helpOpts)
	if cur := m.stack.Current(); cur != nil {
		m.help.SetBindings(cur.Help())
	}

	// Probe the inline (collapsed) flow first to detect overflow: that's
	// the source of truth for whether ? should toggle anything. If the
	// inline flow has no overflow, the panel has nothing to show and we
	// auto-collapse (also covers theme swaps and screen changes that
	// shrink the binding set).
	m.help.SetExpanded(false)
	_, _, overflow := m.helpStrip()
	m.helpOverflow = overflow
	if !overflow {
		m.helpExpanded = false
	}
	m.help.SetExpanded(m.helpExpanded)
	short, _, _ := m.helpStrip()

	prevMsg, prevKind := m.sb.Message()
	sbOpts := t.Statusbar(short, m.version)
	sbOpts.Width = m.w
	m.sb = statusbar.New(sbOpts)
	switch prevKind {
	case statusbar.MessageInfo:
		m.sb.SetInfo(prevMsg)
	case statusbar.MessageError:
		m.sb.SetError(prevMsg)
	}
}

// shortViewBudget returns the visible width available to the statusbar's
// left slot. Reserves room for the right slot (version string + the
// left+right Padding(0,1) the bar adds around each slot) so the inline
// help line can detect overflow before the bar truncates it visually.
func (m Model) shortViewBudget() int {
	const slotPadding = 2 // left+right padding around the left slot
	right := 0
	if m.version != "" {
		right = len(m.version) + 2 // matching padding around right slot
	}
	b := m.w - right - slotPadding
	if b < 0 {
		return 0
	}
	return b
}

// helpStrip renders the inline help line against the statusbar's left
// slot budget and reports how many bindings landed on it. When the
// budget is non-positive, falls back to the full ShortView with no
// overflow reporting.
func (m Model) helpStrip() (line string, consumed int, overflow bool) {
	budget := m.shortViewBudget()
	if budget <= 0 {
		return m.help.ShortView(), m.help.Count(), false
	}
	return m.help.ShortViewBudget(budget)
}

// Update handles resize, global keys, and forwards everything else to the
// stack. Global keys are suppressed when the active screen reports
// IsCapturingKeys so text input isn't hijacked.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Forward to the statusbar so its KeyMsg auto-clear runs before any
	// status message a screen emits in response to the same key. Returns
	// nil cmd; only the KeyMsg branch mutates state.
	m.sb, _ = m.sb.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.apply()
		return m, nil

	case SetThemeMsg:
		for i, t := range m.themes {
			if t.Name == msg.Name {
				m.themeIdx = i
				m.stack.SetTheme(m.theme())
				m.apply()
				break
			}
		}
		return m, nil

	case StatusInfoMsg:
		m.sb.SetInfo(msg.Text)
		return m, nil

	case StatusErrorMsg:
		m.sb.SetError(msg.Text)
		return m, nil

	case StatusClearMsg:
		m.sb.ClearMessage()
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
			if m.helpKey.Keys() != nil && key.Matches(msg, m.helpKey) && m.helpOverflow {
				m.helpExpanded = !m.helpExpanded
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

	items := []layout.Item{
		layout.Fixed(1, layout.RenderFunc(func(w, _ int) string {
			m.bc.SetWidth(w)
			return m.bc.View()
		})),
		layout.Flex(1, body),
	}
	if m.helpExpanded {
		budget := m.shortViewBudget()
		if rows := m.help.ExpandedRows(budget, m.helpMaxRows); rows > 0 {
			h := m.help
			// Match the statusbar's left-slot Padding(0,1) so panel
			// columns align with the footer's row 0.
			leftPad := 1
			items = append(items, layout.Fixed(rows, layout.RenderFunc(func(w, _ int) string {
				rightPad := w - leftPad - budget
				if rightPad < 0 {
					rightPad = 0
				}
				return h.PadLines(h.ExpandedView(budget, rows), leftPad, rightPad)
			})))
		}
	}
	items = append(items, layout.Fixed(1, layout.RenderFunc(func(w, _ int) string {
		m.sb.SetWidth(w)
		return m.sb.View()
	})))
	return layout.VStack(items...).Render(m.w, m.h)
}

// Theme exposes the app's current palette for screens that need it outside
// of SetTheme (rare — most screens just cache the theme they were last told
// about).
func (m Model) Theme() theme.Theme { return m.theme() }
