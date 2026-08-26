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
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/config"
	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/layout"
	"github.com/jsdrews/tuilib/pkg/mouse"
	"github.com/jsdrews/tuilib/pkg/output"
	"github.com/jsdrews/tuilib/pkg/runner"
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

	// SuspendKey suspends the program (ctrl+z semantics), returning to the
	// shell until the user foregrounds it again. Defaults to "ctrl+z"; see
	// DisableSuspend to turn it off.
	//
	// Bubbletea does not bind this itself — it delivers ctrl+z as an
	// ordinary key and expects the app to ask for the suspend — so without
	// this the key does nothing. Suspending is unsupported on Windows,
	// where the request is ignored.
	SuspendKey key.Binding

	// DisableSuspend turns off the suspend key entirely.
	//
	// A zero SuspendKey means "unset" and gets the default, so it cannot
	// also mean "disabled" — key.Binding has no way to tell an empty
	// binding from an absent one. This flag is the explicit off switch,
	// matching DisableAutoEscPop.
	DisableSuspend bool

	// HelpMaxRows caps how many rows the expanded help panel may grow
	// to. Defaults to 6. The panel uses only as many rows as needed to
	// fit every binding at the current width, up to this cap.
	HelpMaxRows int

	// OutputKey opens the app-wide output console (pkg/output) and is the
	// single switch for the whole feature: leave it zero and no buffer, no
	// statusbar badge and no console screen exist.
	//
	// Opt-in rather than on-by-default, for the same reason ThemeKey is: it
	// claims a key permanently, in every app that links the shell, and a
	// key the shell takes is a key no component may ever bind. Spending one
	// should be a line the app author writes on purpose.
	//
	//	OutputKey: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "output"))
	//
	// The console captures every Info/Error, every InfoDetail/ErrorDetail,
	// runner.Result exit statuses, and everything runner.Capture streams.
	// Capture itself works with this unset — the calling screen still
	// receives every line; you lose the console, not the pipeline.
	OutputKey key.Binding

	// ActionsKey opens the action menu (pkg/action) for the active screen,
	// and is the single switch for the whole feature: leave it zero and no
	// menu, no right-click handling and no key exist.
	//
	// Opt-in for the same reason OutputKey is — a key the shell claims is a
	// key no component may ever bind, in every app that links the shell.
	//
	//	ActionsKey: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions"))
	//
	// A screen supplies its verbs by implementing action.Provider. Screens
	// that don't have none, and the key stays inert on them — the hint is
	// only advertised where the menu would actually open.
	ActionsKey key.Binding

	// Actions configures the menu. Leave zero for theme.Actions() against
	// the current theme; colors come back from the theme on every swap, as
	// everywhere else in the library.
	Actions action.Options

	// Output configures the console. Leave zero for output.OptionsFrom
	// against the initial theme. Non-visual fields (MaxRecords, ExportDir,
	// SourceWidth, Keys) survive theme swaps; colors come back from the
	// theme, as everywhere else in the library.
	Output output.Options

	// HelpVerbose restores the legacy footer behavior: bindings are
	// tight-packed inline in the statusbar's left slot until they
	// overflow. The default (zero value) is minimal mode — the footer
	// shows only the "? help" affordance and pressing HelpKey opens the
	// expanded panel with every binding. Minimal cuts clutter on screens
	// with many bindings (a deep component composition can easily
	// produce 15+) at the cost of inline discoverability.
	HelpVerbose bool

	// Mouse selects how much mouse input the shell enables. Defaults to
	// MouseOff, so an app gains mouse support only by asking for it — the
	// terminal's own click-drag text selection stops working the moment
	// mouse reporting is on, and that trade is the app author's to make.
	//
	// When set to MouseClick the shell enables cell-motion reporting from
	// Init, translates each event into a mouse.Msg (resolving double
	// clicks), and forwards it to the active screen. Components hit-test it
	// against the rect layout gave them; see pkg/geom and pkg/mouse.
	Mouse MouseMode

	// DoubleClickInterval is the window in which a second press in the same
	// cell counts as a double click. Zero falls back to the user's config
	// file, then to mouse.DefaultDoubleClickInterval.
	DoubleClickInterval time.Duration

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

// MouseMode selects how much mouse input the app shell enables. It is an
// enum rather than a bool so a hover tier can be added later without
// breaking callers who already set the field.
type MouseMode int

const (
	// MouseOff disables mouse reporting entirely, leaving the terminal's
	// native click-drag text selection intact. This is the default: turning
	// reporting on takes selection away from the user, so it should be a
	// decision the app makes rather than something it inherits.
	MouseOff MouseMode = iota

	// MouseClick enables cell-motion reporting — presses, releases, the
	// wheel, and motion while a button is held. Bare pointer movement is
	// not reported, so nothing re-renders while the pointer merely crosses
	// the screen.
	MouseClick
)

// enableCmd returns the bubbletea command that turns this mode on, or nil
// for MouseOff. Enabling from Init rather than via a tea.NewProgram option
// keeps mouse configuration on app.Options, so callers never have to keep
// two places in sync.
func (m MouseMode) enableCmd() tea.Cmd {
	if m == MouseClick {
		return tea.EnableMouseCellMotion
	}
	return nil
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

// StatusInfoMsg asks the app to show Text as an info message in the
// statusbar's center slot. The message auto-clears on the next KeyMsg
// (matching the statusbar's own Update behavior). Emit via Info(s) from any
// screen.
//
// Body is the verbose half: it never touches the statusbar, and goes to the
// output console (when OutputKey is set) as continuation lines under Text.
// Emit via InfoDetail.
type StatusInfoMsg struct {
	Text string
	Body string
}

// StatusErrorMsg asks the app to show Text as an error message in the
// statusbar's center slot. Same auto-clear and Body semantics as
// StatusInfoMsg. Emit via Error(s) or ErrorDetail(summary, body).
type StatusErrorMsg struct {
	Text string
	Body string
}

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

// InfoDetail posts an info message whose summary paints the statusbar, as
// Info does, while body goes to the output console as continuation lines
// beneath it.
//
// This is the channel for output that was never going to fit in a footer —
// a command's stderr, a multi-line API response. An empty body behaves
// exactly like Info, so migrating a call site is mechanical.
func InfoDetail(summary, body string) tea.Cmd {
	return func() tea.Msg { return StatusInfoMsg{Text: summary, Body: body} }
}

// ErrorDetail is InfoDetail's error counterpart. See InfoDetail.
func ErrorDetail(summary, body string) tea.Cmd {
	return func() tea.Msg { return StatusErrorMsg{Text: summary, Body: body} }
}

// ErrorOf posts err as an error message: err.Error() paints the statusbar
// and the unwrapped %w chain goes to the console, one wrap per line.
//
// Without this the chain gets flattened to its outermost message, because
// hand-formatting it at every call site is work nobody actually does. An
// error that wraps nothing degrades to a plain Error, so there is no reason
// to choose between them.
func ErrorOf(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	var chain []string
	for e := err; e != nil; e = errors.Unwrap(e) {
		chain = append(chain, e.Error())
	}
	if len(chain) <= 1 {
		return Error(err.Error())
	}
	return ErrorDetail(err.Error(), strings.Join(chain, "\n"))
}

// OutputClosed is the value the output console pops with, aliased from
// pkg/output so a screen matching it in OnEnter needs only this import.
//
// A screen whose OnEnter kicks off a fetch should early-return on it —
// otherwise glancing at the log silently refetches whatever was underneath:
//
//	func (s *Screen) OnEnter(result any) tea.Cmd {
//	    if _, ok := result.(app.OutputClosed); ok {
//	        return nil
//	    }
//	    return s.fetch()
//	}
type OutputClosed = output.Closed

// Model is the app shell. Instantiate with New and pass to tea.NewProgram.
type Model struct {
	w, h int

	themes   []theme.Theme
	themeIdx int
	version  string

	stack screen.Stack

	quitKey, themeKey, helpKey key.Binding
	suspendKey                 key.Binding
	helpMaxRows                int
	helpMinimal                bool
	autoEscPop                 bool

	bc breadcrumb.Model
	sb statusbar.Model

	help         help.Model
	helpExpanded bool
	helpOverflow bool

	// out* are nil/zero unless Options.OutputKey was set. outScreen is
	// retained across opens rather than rebuilt, so the console keeps its
	// scroll position and search query between visits.
	outputKey key.Binding
	outBuf    *output.Buffer
	outOpts   output.Options
	outScreen *output.Screen
	// rightSlot is the composed statusbar right slot (badge + version).
	// Cached because the inline-help budget is measured against it.
	rightSlot string
	badgeW    int
	// msgReserve is the width held back from the help line for an active
	// status message. See reserveForMessage.
	msgReserve int

	mouseMode MouseMode
	mouse     mouse.Tracker

	// act* are zero unless Options.ActionsKey was set. The menu and the
	// confirm modal are pointers because View has a value receiver: a
	// layout.Sized over a value field would place a copy that is discarded
	// when View returns, and the overlay would never hit-test (see
	// placeChrome for the same problem in the chrome).
	actionsKey key.Binding
	actOpts    action.Options
	menu       *action.Menu
	menuUp     bool
	conf       *confirm.Model
	confUp     bool
	pendingAct action.Action
	pendingTgt string

	// running maps a live action run's Tag (an action.RunKey) to nothing in
	// particular — it is a set. The shell owns it because it launches the
	// action, so it holds both the RunKey and the run at once; a screen
	// doing this has to bridge the two halves by hand.
	running map[string]bool
}

// actionsEnabled reports whether the action menu exists for this app.
func (m Model) actionsEnabled() bool { return m.actionsKey.Keys() != nil }

// overlayUp reports whether the shell is showing the menu or its confirm.
func (m Model) overlayUp() bool { return m.menuUp || m.confUp }

// outputEnabled reports whether the console exists for this app.
func (m Model) outputEnabled() bool { return m.outBuf != nil }

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
	if opts.SuspendKey.Keys() == nil {
		opts.SuspendKey = key.NewBinding(
			key.WithKeys("ctrl+z"),
			key.WithHelp("^z", "suspend"),
		)
	}
	if opts.DisableSuspend {
		opts.SuspendKey = key.Binding{}
	}
	if opts.HelpMaxRows <= 0 {
		opts.HelpMaxRows = 6
	}
	// Double-click speed is a per-machine preference, so an unset option
	// defers to the user's config file before falling back to the default.
	if opts.DoubleClickInterval <= 0 && !opts.SkipConfig {
		if cfg, err := config.Load(); err == nil && cfg.DoubleClickMs > 0 {
			opts.DoubleClickInterval = time.Duration(cfg.DoubleClickMs) * time.Millisecond
		}
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
		suspendKey:  opts.SuspendKey,
		helpMaxRows: opts.HelpMaxRows,
		helpMinimal: !opts.HelpVerbose,
		autoEscPop:  !opts.DisableAutoEscPop,
		mouseMode:   opts.Mouse,
		mouse:       mouse.NewTracker(opts.DoubleClickInterval),
	}
	if opts.ActionsKey.Keys() != nil {
		m.actionsKey = opts.ActionsKey
		m.actOpts = mergeActionOptions(opts.Actions, t)
		menu := action.New(m.actOpts)
		m.menu = &menu
		m.running = map[string]bool{}
	}
	if opts.OutputKey.Keys() != nil {
		m.outputKey = opts.OutputKey
		m.outOpts = mergeOutputOptions(opts.Output, t)
		m.outBuf = output.NewBuffer(m.outOpts.MaxRecords)
	}
	m.apply()
	return m
}

// mergeOutputOptions rebuilds the console's themed options while carrying
// the caller's non-visual knobs across. Colors are the theme's job by
// definition (CLAUDE.md rule 4), so this is also what runs on a theme swap.
func mergeOutputOptions(prev output.Options, t theme.Theme) output.Options {
	next := output.OptionsFrom(t)
	if prev.MaxRecords != 0 {
		next.MaxRecords = prev.MaxRecords
	}
	if prev.ExportDir != "" {
		next.ExportDir = prev.ExportDir
	}
	if prev.SourceWidth != 0 {
		next.SourceWidth = prev.SourceWidth
	}
	keys := prev.Keys
	keys.FillDefaults()
	next.Keys = keys
	return next
}

// mergeActionOptions rebuilds the menu's themed options, carrying the
// caller's non-visual knobs across. Same shape as mergeOutputOptions, and for
// the same reason: colors belong to the theme (rule 4), everything else to the
// app author.
func mergeActionOptions(prev action.Options, t theme.Theme) action.Options {
	next := t.Actions()
	if prev.Title != "" {
		next.Title = prev.Title
	}
	if prev.MultiReason != "" {
		next.MultiReason = prev.MultiReason
	}
	if prev.RunningReason != "" {
		next.RunningReason = prev.RunningReason
	}
	keys := prev.Keys
	keys.FillDefaults()
	next.Keys = keys
	return next
}

// Init runs the root screen's Init + OnEnter(nil), and turns on mouse
// reporting when Options.Mouse asks for it.
func (m Model) Init() tea.Cmd {
	if enable := m.mouseMode.enableCmd(); enable != nil {
		return tea.Batch(m.stack.Init(), enable)
	}
	return m.stack.Init()
}

// theme returns the currently-active palette.
func (m Model) theme() theme.Theme { return m.themes[m.themeIdx] }

// retheme pushes the current palette through the stack and the console, then
// rebuilds the chrome.
//
// The console needs its own call: its options carry the badge styles and the
// line colors, and its screen may be off the stack entirely (the buffer
// outlives any particular visit), so stack.SetTheme alone would leave both
// stale.
func (m *Model) retheme() {
	t := m.theme()
	m.stack.SetTheme(t)
	if m.actionsEnabled() {
		// Rebuild over the same state (rule 4): the cursor survives, and so
		// does whatever the menu was opened against.
		cursor, set := m.menu.Cursor(), m.menu.Set()
		m.actOpts = mergeActionOptions(m.actOpts, t)
		menu := action.New(m.actOpts)
		m.menu = &menu
		m.menu.SetActions(set)
		m.menu.SetRunning(m.running)
		m.menu.SetCursor(cursor)
	}
	if m.outputEnabled() {
		m.outOpts = mergeOutputOptions(m.outOpts, t)
		if m.outScreen != nil && !m.outputOnTop() {
			m.outScreen.SetTheme(t)
		}
	}
	m.apply()
}

// apply rebuilds breadcrumb + statusbar from the current theme and stack.
// Called on init, resize, theme swap, and any stack mutation so the
// breadcrumb and help hints stay in sync. Any in-flight info/error message
// on the statusbar is captured before the rebuild and re-applied after, so
// stack updates don't wipe a screen-emitted status message.
func (m *Model) apply() {
	t := m.theme()

	// The right slot and the message allowance have to be computed first:
	// the inline help budget is measured against what they leave, so a badge
	// or a status message appearing mid-session has to be able to push the
	// help line into overflow.
	m.rightSlot, m.badgeW = m.composeRight()

	prevMsg, prevKind := m.sb.Message()
	m.msgReserve = m.reserveForMessage(prevMsg, prevKind)

	bcOpts := t.Breadcrumb()
	bcOpts.Width = m.w
	bcOpts.Crumbs = m.stack.Crumbs()
	m.bc = breadcrumb.New(bcOpts)

	helpOpts := t.Help()
	helpOpts.Minimal = m.helpMinimal
	m.help = help.New(helpOpts)
	if cur := m.stack.Current(); cur != nil {
		m.help.SetBindings(m.helpBindings(cur))
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

	sbOpts := t.Statusbar(short, m.rightSlot)
	sbOpts.Width = m.w
	m.sb = statusbar.New(sbOpts)
	switch prevKind {
	case statusbar.MessageInfo:
		m.sb.SetInfo(prevMsg)
	case statusbar.MessageError:
		m.sb.SetError(prevMsg)
	}
}

// composeRight builds the statusbar's right slot — the output badge ahead of
// the version string — and reports the badge's visible width so clickChrome
// can tell a click on the badge from one on the version.
//
// The badge goes right rather than left because the left slot is rendered
// whole by pkg/help and its one clickable region is located by asking help
// for a character offset. A second affordance in there would mean either
// teaching pkg/help what a log is, or the shell doing its own span
// arithmetic inside a string it did not lay out.
func (m Model) composeRight() (slot string, badgeW int) {
	if !m.outputEnabled() {
		return m.version, 0
	}
	badge := m.outOpts.RenderBadge(m.outBuf)
	if badge == "" {
		return m.version, 0
	}
	badgeW = lipgloss.Width(badge)
	if m.version == "" {
		return badge, badgeW
	}
	return badge + " " + m.version, badgeW
}

// helpBindings returns the active screen's bindings plus the output key.
//
// The shell advertises this one itself rather than leaving it to screens.
// Every other global follows the opposite convention — a screen lists q and t
// in its own Help() — and that works because those keys are universal enough
// that an author writing a screen already knows about them. The output key is
// not: it is opt-in, so it exists in some apps and not others, and a screen
// author copying an existing screen has no reason to add it. Left to the
// screens it would be advertised on the one screen whose author remembered,
// which is worse than not advertising it at all.
//
// While the console is open the same key closes it, so the description says
// so; the key label stays whatever the app chose.
//
// It goes *first*, not last. The expanded panel is capped at HelpMaxRows, so
// on a binding-heavy screen the tail of the list is simply not drawn — and a
// binding appended at the end is the first thing dropped. That is how this
// shipped broken: it rendered fine on a sparse screen at 120 columns and
// vanished on a table at 80. Everything else in the list belongs to whatever
// component is on screen and has some other route to discovery; the output
// key has none, so it takes the slot that always survives.
func (m Model) helpBindings(cur screen.Screen) []key.Binding {
	// While an overlay owns the keyboard, the hints are its own — the
	// screen's bindings do nothing until it closes.
	switch {
	case m.confUp:
		return m.conf.Help()
	case m.menuUp:
		return m.menu.Help()
	}

	own := cur.Help()
	if m.actionsEnabled() && !m.currentActions().Empty() {
		// Advertised by the shell rather than by each screen, for the same
		// reason the output key is (rule 14): it is opt-in, so a screen
		// author copying an existing screen has no reason to know it exists.
		// Only where it would actually open something — a key that opens an
		// empty box teaches users the feature is broken.
		own = append([]key.Binding{m.actionsKey}, own...)
	}
	if !m.outputEnabled() || m.outputKey.Keys() == nil {
		return own
	}

	b := m.outputKey
	if m.outputOnTop() {
		b = key.NewBinding(
			key.WithKeys(b.Keys()...),
			key.WithHelp(b.Help().Key, "close output"),
		)
	}

	// Copy rather than prepend in place: the screen owns its slice.
	out := make([]key.Binding, 0, len(own)+1)
	out = append(out, b)
	return append(out, own...)
}

// reserveForMessage returns the width the bar's center slot needs for an
// active info/error message.
//
// The help line pads out to whatever budget it is given, so without this the
// center slot is sized to zero and a message has nowhere to go. It used to
// render anyway — lipgloss's Width is a minimum, not a maximum — by
// overflowing and shoving the right slot off the end of the bar, which is
// why a long error used to eat the version string. Now that the right slot
// also carries the output badge, "the message hides the badge" is precisely
// the wrong trade: the badge is what tells you the full text is recoverable.
//
// Capped at half the bar so a long message can never squeeze the help hints
// out entirely; the message itself is cut to fit by the statusbar.
func (m Model) reserveForMessage(msg string, kind statusbar.MessageKind) int {
	if kind == statusbar.MessageNone || msg == "" {
		return 0
	}
	const slotPadding = 2
	want := lipgloss.Width(msg) + slotPadding
	if half := m.w / 2; want > half {
		want = half
	}
	return want
}

// shortViewBudget returns the visible width available to the statusbar's
// left slot. Reserves room for the right slot (its content plus the
// left+right Padding(0,1) the bar adds around each slot) and for any active
// status message, so the inline help line can detect overflow before the bar
// truncates it visually.
func (m Model) shortViewBudget() int {
	const slotPadding = 2 // left+right padding around the left slot
	right := 0
	if m.rightSlot != "" {
		right = lipgloss.Width(m.rightSlot) + 2 // matching padding around right slot
	}
	b := m.w - right - slotPadding - m.msgReserve
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

	// Menu results first, so a dismissed overlay never sees a stale
	// follow-up key.
	switch a := msg.(type) {
	case action.ChosenMsg:
		m.closeOverlay()
		if a.Action.Confirm != "" {
			m.armConfirm(a.Action, a.Target)
			m.apply()
			return m, nil
		}
		cmd := m.runAction(a.Action, a.Target)
		m.apply()
		return m, cmd

	case action.CancelledMsg:
		m.closeOverlay()
		m.apply()
		return m, nil

	case confirm.ConfirmedMsg:
		// Guarded on confUp so a screen hosting its own confirm modal keeps
		// receiving its own results — the shell claims these only while it
		// is the one showing the dialog.
		if m.confUp {
			act, tgt := m.pendingAct, m.pendingTgt
			m.closeOverlay()
			cmd := m.runAction(act, tgt)
			m.apply()
			return m, cmd
		}

	case confirm.CancelledMsg:
		if m.confUp {
			m.closeOverlay()
			m.apply()
			return m, nil
		}

	case action.RetargetMsg:
		// A right-press that missed the menu means "ask me about this one
		// instead" — one gesture, not dismiss-then-click-again.
		m.closeOverlay()
		cmd := m.rightClick(a.Event)
		m.apply()
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.apply()
		return m, nil

	case runner.Result, tea.ResumeMsg:
		// A suspended subprocess owned the real TTY, so its output is gone —
		// but the exit status isn't, and until now a failing subprocess
		// produced nothing at all unless the screen author checked Err.
		if r, ok := msg.(runner.Result); ok {
			m.logResult(r)
		}
		// Both of these mean the terminal was handed away and given back: a
		// subprocess taking it over, or the program being suspended and
		// foregrounded. Either way mouse reporting is off, and bubbletea's
		// RestoreTerminal does not bring it back — it restores the alt
		// screen, bracketed paste and focus reporting, but has no notion of
		// mouse state. Nothing else would ever re-enable it, so the TUI
		// comes back mouse-dead until restart.
		if enable := m.mouseMode.enableCmd(); enable != nil {
			var cmd tea.Cmd
			m.stack, cmd = m.stack.Update(msg)
			m.apply()
			return m, tea.Batch(cmd, enable)
		}

	case tea.MouseMsg:
		// Screens never see the raw event. Resolving the click count here
		// keeps double-click timing in one place instead of duplicating a
		// threshold across every component (see pkg/mouse).
		if m.mouseMode == MouseOff {
			return m, nil
		}
		e := m.mouse.Track(msg, time.Now())

		// While an overlay is up it owns the pointer. ZStack hands both
		// layers the same rect, so a component underneath still believes it
		// owns the cells the menu covers — occluding it is the host's job,
		// and here the shell is the host.
		if m.overlayUp() {
			cmd := m.updateOverlay(e)
			m.apply()
			return m, cmd
		}

		// Right-click asks "what can I do to this?", so the screen sees the
		// event first and moves its cursor, and only then does the menu open
		// against what the pointer actually landed on. Reversed, the menu
		// would describe whatever happened to be selected before.
		if e.IsRightPress() && m.actionsEnabled() {
			cmd := m.rightClick(e)
			m.apply()
			return m, cmd
		}

		// The shell owns its own chrome, exactly as it owns the keys that
		// drive it — a screen should no more handle a crumb click than it
		// handles esc-pop.
		if e.IsPress() {
			if cmd, handled := m.clickChrome(e); handled {
				return m, cmd
			}
		}
		var cmd tea.Cmd
		m.stack, cmd = m.stack.Update(e)
		m.apply()
		return m, cmd

	case SetThemeMsg:
		for i, t := range m.themes {
			if t.Name == msg.Name {
				m.themeIdx = i
				m.retheme()
				break
			}
		}
		return m, nil

	case StatusInfoMsg:
		m.sb.SetInfo(msg.Text)
		m.logEntry("", msg.Text, msg.Body, output.LevelInfo)
		m.apply()
		return m, nil

	case StatusErrorMsg:
		m.sb.SetError(msg.Text)
		m.logEntry("", msg.Text, msg.Body, output.LevelError)
		m.apply()
		return m, nil

	case StatusClearMsg:
		m.sb.ClearMessage()
		return m, nil

	case output.Notice:
		// The console's own screen can't return app.Info — pkg/app imports
		// pkg/output, so it routes through here instead and lands in the
		// buffer like any other message.
		if msg.Level == output.LevelError {
			m.sb.SetError(msg.Text)
		} else {
			m.sb.SetInfo(msg.Text)
		}
		m.logEntry("", msg.Text, "", msg.Level)
		m.apply()
		return m, nil

	case runner.CaptureStarted:
		// The badge counts this now rather than on completion: a five-minute
		// build that signals nothing until it finishes turns "keep working
		// while it runs" into "keep working, blind."
		if m.outputEnabled() {
			started := msg
			m.outBuf.StartRun(msg.RunID, msg.Label, func() error { return runner.Kill(started) })
			m.outBuf.Append(output.Record{
				Level:  output.LevelInfo,
				Source: msg.Label,
				Text:   captureHead(msg),
				Head:   true,
				RunID:  msg.RunID,
			})
		}
		return m, m.forwardCapture(msg)

	case runner.CapturedLine:
		if m.outputEnabled() {
			m.outBuf.Append(output.Record{
				Level:  output.LevelInfo,
				Source: msg.Label,
				Text:   msg.Text,
				Stderr: msg.Stderr,
				RunID:  msg.RunID,
			})
		}
		return m, m.forwardCapture(msg)

	case runner.Captured:
		// An action the shell launched carries a Tag. Releasing the gate and
		// posting the receipt are scoped to those: a bare runner.Capture
		// keeps behaving exactly as it always has, which matters because it
		// is a shipped feature with callers of its own.
		if msg.Tag != "" {
			delete(m.running, msg.Tag)
			if m.menuUp {
				m.menu.SetRunning(m.running)
			}
			if msg.Err != nil {
				m.sb.SetError(msg.Label + " failed: " + msg.Err.Error())
			} else {
				m.sb.SetInfo(msg.Label + " completed")
			}
		}
		if m.outputEnabled() {
			m.outBuf.EndRun(msg.RunID)
			// A continuation, not a head: one run is one event however many
			// lines it emitted. Its level is what tints the badge, which is
			// why stderr lines alone don't.
			lvl, text := output.LevelInfo, msg.Label+" completed"
			if msg.Err != nil {
				lvl, text = output.LevelError, msg.Label+" failed: "+msg.Err.Error()
			}
			m.outBuf.Append(output.Record{
				Level:  lvl,
				Source: msg.Label,
				Text:   text,
				RunID:  msg.RunID,
			})
		}
		return m, m.forwardCapture(msg)

	case tea.KeyMsg:
		if m.overlayUp() {
			// The overlay owns the keyboard completely while it is up, so
			// there is no partial-routing state to reason about — the same
			// contract a screen hosting pkg/confirm lives under.
			cmd := m.updateOverlay(msg)
			m.apply()
			return m, cmd
		}
		cur := m.stack.Current()
		capturing := cur != nil && cur.IsCapturingKeys()
		if !capturing {
			if m.actionsEnabled() && key.Matches(msg, m.actionsKey) {
				m.openMenu(-1, -1)
				m.apply()
				return m, nil
			}
			if key.Matches(msg, m.quitKey) && m.stack.Depth() == 1 {
				return m, tea.Quit
			}
			if m.themeKey.Keys() != nil && key.Matches(msg, m.themeKey) {
				m.themeIdx = (m.themeIdx + 1) % len(m.themes)
				m.retheme()
				return m, nil
			}
			if m.helpKey.Keys() != nil && key.Matches(msg, m.helpKey) && m.helpOverflow {
				m.helpExpanded = !m.helpExpanded
				m.apply()
				return m, nil
			}
			if m.suspendKey.Keys() != nil && key.Matches(msg, m.suspendKey) {
				return m, tea.Suspend
			}
			if m.outputEnabled() && key.Matches(msg, m.outputKey) {
				return m, m.toggleOutput()
			}
			if m.autoEscPop && msg.String() == "esc" && m.stack.Depth() > 1 {
				// esc closes the console too, and must close it the same way
				// the key does — with the sentinel, or the screen underneath
				// can't tell a glance at the log from a fresh activation.
				if m.outputOnTop() {
					return m, m.popOutput()
				}
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

// forwardCapture passes a capture message on to the active screen and asks
// for the next one.
//
// Both halves matter. The screen sees every line so a view that wants to
// stream output in place can do it from the same pipeline the console uses,
// instead of the library shipping two. And the read is chained here rather
// than by the screen, unconditionally — including when the console is
// disabled — because a capture nobody drains eventually stalls the
// subprocess.
func (m *Model) forwardCapture(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.stack, cmd = m.stack.Update(msg)
	m.apply()
	return tea.Batch(cmd, runner.Next(msg))
}

// logEntry appends a status message to the console: the summary as a head
// record, the body (when there is one) as continuation lines beneath it.
//
// src empty means "attribute to the screen on top right now". That is
// occasionally a lie — a fetch kicked off two screens ago that fails after
// the user has navigated gets stamped with wherever they are standing — but
// app.Info is a closure with no idea who created it, and a tea.Cmd cannot
// introspect its creator. The line still carries its time, level and full
// text; what's lost is a hint.
func (m *Model) logEntry(src, text, body string, lvl output.Level) {
	if !m.outputEnabled() {
		return
	}
	if src == "" {
		if cur := m.stack.Current(); cur != nil {
			src = cur.Title()
		}
	}
	recs := []output.Record{{Level: lvl, Source: src, Text: text, Head: true}}
	if body != "" {
		for _, line := range strings.Split(body, "\n") {
			recs = append(recs, output.Record{Level: lvl, Source: src, Text: line})
		}
	}
	m.outBuf.AppendAll(recs)
}

// logResult records a suspended subprocess's exit status.
func (m *Model) logResult(r runner.Result) {
	if !m.outputEnabled() || r.Cmd == nil {
		return
	}
	label := filepath.Base(r.Cmd.Path)
	if r.Err != nil {
		m.logEntry(label, label+" failed: "+r.Err.Error(), "", output.LevelError)
		return
	}
	m.logEntry(label, label+" completed", "", output.LevelInfo)
}

// captureHead is the head line for a run: whatever the runner described it as,
// falling back to the command line.
//
// The fallback is what keeps messages built by hand (tests, mostly) rendering
// as they always did; a real capture and a real Go run both arrive with Detail
// already set, and a Go run has no Cmd to derive one from.
func captureHead(m runner.CaptureStarted) string {
	if m.Detail != "" {
		return m.Detail
	}
	return commandLine(m.Cmd)
}

// commandLine renders the command a capture is running, as the head line of
// its event.
func commandLine(cmd *exec.Cmd) string {
	if cmd == nil {
		return "(no command)"
	}
	if len(cmd.Args) > 0 {
		return "$ " + strings.Join(cmd.Args, " ")
	}
	return "$ " + cmd.Path
}

// outputOnTop reports whether the console is the active screen.
func (m Model) outputOnTop() bool {
	_, ok := m.stack.Current().(*output.Screen)
	return ok
}

// toggleOutput opens the console, or closes it if it is already on top.
//
// The screen instance is retained rather than rebuilt so scroll position and
// search query survive between visits; it is re-themed on the way in, since
// stack-wide theme swaps can't reach it while it is off the stack.
func (m *Model) toggleOutput() tea.Cmd {
	if m.outputOnTop() {
		return m.popOutput()
	}

	// Opening retires the statusbar message. It is a truncated echo of
	// something the console is about to show in full, so leaving it up means
	// a stale sliver sitting under the very log that supersedes it.
	//
	// Doing it here rather than relying on the statusbar's own KeyMsg
	// auto-clear is what makes the two entry points agree: the key path was
	// already clearing as a side effect of being a keypress, while clicking
	// the badge left the message behind.
	//
	// Only on the way in. Closing must not clear, or a notice raised *by*
	// the console — the path "w" reports after an export — would be wiped on
	// the way out by a user who clicked the badge to leave.
	m.sb.ClearMessage()

	if m.outScreen == nil {
		m.outScreen = output.NewScreen(m.outBuf, m.outOpts)
	}
	m.outScreen.SetTheme(m.theme())

	var cmd tea.Cmd
	m.stack, cmd = m.stack.Update(screen.PushMsg{Screen: m.outScreen})
	m.apply()
	return cmd
}

// popOutput closes the console, marking everything read on the way out.
//
// Read is marked on close rather than on open so records arriving while the
// user sits on the screen don't come back as unread the moment they leave.
func (m *Model) popOutput() tea.Cmd {
	m.outBuf.MarkRead()
	var cmd tea.Cmd
	m.stack, cmd = m.stack.Update(screen.PopMsg{Result: output.Closed{}})
	m.apply()
	return cmd
}

// clickChrome handles clicks on the shell's own furniture — the breadcrumb
// trail and the statusbar's help affordance — and reports whether it
// consumed the event.
//
// Clicking a crumb unwinds to that depth in one step, the mouse counterpart
// of pressing esc repeatedly. Clicking the "? help" affordance toggles the
// expanded panel, and only while the affordance is actually shown: it is the
// source of truth for whether the help key does anything, so the click
// follows the same rule rather than inventing a second one.
func (m *Model) clickChrome(e mouse.Msg) (tea.Cmd, bool) {
	m.placeChrome()

	if i, ok := m.bc.CrumbAt(e.X, e.Y); ok {
		if depth := i + 1; depth < m.stack.Depth() {
			var cmd tea.Cmd
			m.stack, cmd = m.stack.Update(screen.PopToMsg{Depth: depth})
			m.apply()
			return cmd, true
		}
		// The current crumb — already here, nothing to do, but the click
		// belongs to the breadcrumb and shouldn't fall through to the body.
		return nil, true
	}

	if m.badgeW > 0 {
		slot := m.sb.RightContentRect()
		if slot.Hit(e.X, e.Y) && e.X-slot.X < m.badgeW {
			return m.toggleOutput(), true
		}
	}

	// Clicking the status message opens the console. The message is a
	// truncated echo of something the log holds in full, so "show me the
	// rest" is the obvious thing to want from it — and the colored band it
	// renders as spans the whole center slot, which makes the target
	// something the user can actually see.
	//
	// Open, never toggle. The console raises messages of its own (the export
	// path), and clicking one of those to *close* the view that produced it
	// would be backwards.
	if m.outputEnabled() && m.sb.MessageKind() != statusbar.MessageNone {
		if m.sb.MiddleContentRect().Hit(e.X, e.Y) {
			if m.outputOnTop() {
				return nil, true
			}
			return m.toggleOutput(), true
		}
	}

	if m.helpOverflow && m.helpKey.Keys() != nil {
		slot := m.sb.LeftContentRect()
		if at, aw, ok := m.help.AffordanceSpan(m.shortViewBudget()); ok && slot.Hit(e.X, e.Y) {
			rel := e.X - slot.X
			if rel >= at && rel < at+aw {
				m.helpExpanded = !m.helpExpanded
				m.apply()
				return nil, true
			}
		}
	}
	return nil, false
}

// placeChrome gives the breadcrumb and statusbar the rects they occupy, so
// they can hit-test a click.
//
// They can't get these from rendering the way components do. View has a value
// receiver — bubbletea's Model interface requires it — so the layout.Bar
// wrappers there call SetRect on a copy of the Model that is discarded when
// View returns. Screens escape this because screen.Screen is an interface
// holding a pointer, but these two are plain value fields on the shell.
//
// Deriving the rects here instead is not a workaround so much as an
// admission of where the knowledge lives: the shell defines this layout, so
// it already knows the breadcrumb is the first row and the statusbar the
// last. View builds the same two Fixed(1, …) slots from the same facts.
func (m *Model) placeChrome() {
	m.bc.SetRect(geom.New(0, 0, m.w, 1))
	m.sb.SetRect(geom.New(0, max(0, m.h-1), m.w, 1))
}

// View composes the standard shell as a vertical stack: breadcrumb (1 row),
// active screen's body layout (flex), statusbar (1 row). The screen never
// knows its own terminal dimensions — the layout engine hands it a body
// rect to fill.
func (m Model) View() string {
	if m.w == 0 {
		return ""
	}

	// One generation per frame. Every rect handed down inherits this value,
	// so a component that isn't drawn this frame keeps an older stamp and
	// declines mouse events (see pkg/geom).
	geom.NextGen()

	var body layout.Node
	if cur := m.stack.Current(); cur != nil {
		body = cur.Layout()
	} else {
		body = layout.RenderFunc(func(geom.Rect) string { return "" })
	}

	// The overlay goes over the body only, never the chrome: the breadcrumb
	// still says where you are and the statusbar still carries the hints
	// while the menu is up.
	switch {
	case m.confUp:
		body = layout.ZStack(body, layout.Center(52, 7, layout.Sized(m.conf)))
	case m.menuUp:
		// No layout.Center wrapper — the menu treats the rect as outer
		// bounds and places itself inside them.
		body = layout.ZStack(body, layout.Sized(m.menu))
	}

	items := []layout.Item{
		layout.Fixed(1, layout.Bar(&m.bc)),
		layout.Flex(1, body),
	}
	if m.helpExpanded {
		budget := m.shortViewBudget()
		if rows := m.help.ExpandedRows(budget, m.helpMaxRows); rows > 0 {
			h := m.help
			// Match the statusbar's left-slot Padding(0,1) so panel
			// columns align with the footer's row 0.
			leftPad := 1
			items = append(items, layout.Fixed(rows, layout.RenderFunc(func(r geom.Rect) string {
				rightPad := r.W - leftPad - budget
				if rightPad < 0 {
					rightPad = 0
				}
				return h.PadLines(h.ExpandedView(budget, rows), leftPad, rightPad)
			})))
		}
	}
	items = append(items, layout.Fixed(1, layout.Bar(&m.sb)))
	return layout.VStack(items...).Render(geom.New(0, 0, m.w, m.h))
}

// Theme exposes the app's current palette for screens that need it outside
// of SetTheme (rare — most screens just cache the theme they were last told
// about).
func (m Model) Theme() theme.Theme { return m.theme() }

// currentActions asks the active screen for its verbs. Screens that don't
// implement action.Provider have none, which is the honest answer for a screen
// that never declared any.
//
// Called on menu open and from apply(), so it runs about as often as Help() —
// the same contract applies: cheap, allocation-light, no I/O.
func (m Model) currentActions() action.Set {
	if !m.actionsEnabled() {
		return action.Set{}
	}
	p, ok := m.stack.Current().(action.Provider)
	if !ok {
		return action.Set{}
	}
	return p.Actions()
}

// openMenu shows the action menu for the active screen, anchored at (x, y) or
// centered when x is negative. Reports whether it opened: a screen with no
// verbs opens nothing, which is what keeps the key inert rather than showing
// an empty box.
func (m *Model) openMenu(x, y int) bool {
	set := m.currentActions()
	if set.Empty() {
		return false
	}
	m.menu.SetActions(set)
	m.menu.SetRunning(m.running)
	if x < 0 {
		m.menu.Center()
	} else {
		m.menu.Anchor(x, y)
	}
	m.menuUp = true
	return true
}

// closeOverlay drops whatever is on top of the body.
func (m *Model) closeOverlay() {
	m.menuUp, m.confUp = false, false
	m.pendingAct, m.pendingTgt = action.Action{}, ""
}

// updateOverlay routes one message to the menu or its confirm modal, and
// handles their results. It sees only keys and mouse — everything else keeps
// flowing to the screen underneath, so a capture still logs and a poll still
// ticks while the menu is open.
func (m *Model) updateOverlay(msg tea.Msg) tea.Cmd {
	if m.confUp {
		c, cmd := m.conf.Update(msg)
		*m.conf = c
		return cmd
	}

	var cmd tea.Cmd
	menu, cmd := m.menu.Update(msg)
	*m.menu = menu
	return cmd
}

// armConfirm puts the yes/no modal between the pick and the run, so a
// destructive verb states what it is about to do before it does it.
func (m *Model) armConfirm(a action.Action, target string) {
	m.pendingAct, m.pendingTgt = a, target
	opts := m.theme().Confirm()
	opts.Title = "confirm"
	opts.Message = a.Confirm
	c := confirm.New(opts)
	m.conf = &c
	m.menuUp, m.confUp = false, true
}

// runAction executes a chosen action.
//
// A Do action gets an invocation line, because it is the only line the shell
// can promise: it returns an opaque tea.Cmd that might push a screen or hand
// over the terminal, and the library cannot narrate what it does not own.
//
// A Run action gets none, because it would be the second head record for one
// piece of news — runner.Go already opens the event with the action's Detail,
// and everything the action writes lands underneath it. The badge counts
// events, so logging the invocation separately would make every action report
// twice (rule 17).
func (m *Model) runAction(a action.Action, target string) tea.Cmd {
	if a.Do != nil {
		m.logEntry("", "action: "+a.Label, "", output.LevelInfo)
		return a.Do()
	}
	if a.Run == nil {
		m.logEntry("", "action: "+a.Label, "", output.LevelInfo)
		return nil
	}

	tag := action.RunKey(a, target)
	m.running[tag] = true

	detail := a.Label
	if target != "" {
		detail = a.Label + " · " + target
	}
	return runner.GoWith(runner.GoOptions{
		Label:  a.Label,
		Detail: detail,
		Tag:    tag,
		Run:    a.Run,
	})
}

// rightClick forwards the press to the screen, then opens the menu against
// whatever it selected. Returns the screen's own command so a component that
// wanted to react to the press still gets to.
func (m *Model) rightClick(e mouse.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.stack, cmd = m.stack.Update(e)
	m.openMenu(e.X, e.Y)
	return cmd
}
