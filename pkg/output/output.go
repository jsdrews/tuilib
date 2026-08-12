// Package output is the app-wide console: one ring buffer of everything the
// app has said, and a screen for reading it.
//
// The statusbar's center slot holds one line and wipes it on the next
// keypress, which makes it useless for anything you might want to read twice
// — a failing command's stderr, a wrapped error chain, an API response body.
// This package is where that output goes instead. The app shell (pkg/app)
// owns a Buffer, feeds it from every app.Info / app.Error, from the
// InfoDetail / ErrorDetail channel, and from runner.Capture, and pushes a
// Screen over the stack when the user asks for it.
//
// The whole feature is opt-in: it exists only when app.Options.OutputKey is
// set. See pkg/app.
//
// # Records, not strings
//
// The buffer holds Records — one per rendered line, flat, with a body line
// being a Record with Head=false rather than a field on its parent. Lines are
// formatted at render time rather than on the way in, because pre-rendered
// ANSI would be baked with whatever palette was active when it was written:
// swap themes and the log becomes a stratigraphy of old ones.
//
// # Options and pkg/theme
//
// Options is built with OptionsFrom(t) rather than a theme.Output() method,
// which is a deliberate break from the th.Component() convention every other
// component follows (CLAUDE.md rule 3). Screen implements screen.Screen,
// pkg/screen imports pkg/theme for SetTheme, so a Theme method returning
// output.Options would close an import cycle. Inverting it — output imports
// theme, theme knows nothing about output — is the only arrangement that
// keeps the screen in this package, and keeping it here is what makes it
// testable without standing up an app shell.
package output

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/theme"
)

// DefaultMaxRecords is the ring cap applied when Options.MaxRecords is 0,
// matching logview.DefaultMaxLines.
const DefaultMaxRecords = 10000

// DefaultSourceWidth is the column the source name is padded to, so the
// ›/│ glyphs line up down the log regardless of which screen or command
// produced each line.
const DefaultSourceWidth = 10

// Level is an entry's severity. It drives the level tag in the rendered
// line and the statusbar badge's error tint.
type Level int

const (
	LevelInfo Level = iota
	LevelError
)

// Tag is the three-character level marker rendered into each line.
func (l Level) Tag() string {
	if l == LevelError {
		return "ERR"
	}
	return "INF"
}

// Record is one line's worth of structure. The buffer is flat: a detail
// body or a captured stdout line is a Record with Head=false, not a field
// hanging off the summary above it. That keeps every line self-describing,
// which is what lets logview's filter mode (\) hide non-matching lines
// without orphaning a body line from the command that produced it.
type Record struct {
	// Time is wall-clock, stamped on arrival at the buffer.
	Time time.Time
	// Level drives the level tag and the badge tint.
	Level Level
	// Source is the screen title for Info/Error entries, or the command
	// label for lines from a capture.
	Source string
	// Text is the line itself, unprefixed.
	Text string
	// Head marks a summary line (rendered with ›). Continuation lines
	// (rendered with │) have Head=false. Unread events count Heads.
	Head bool
	// Stderr marks a captured line that arrived on stderr, rendered with a
	// heavier gutter glyph.
	//
	// It is deliberately not expressed as LevelError. Plenty of well-behaved
	// tools write progress to stderr, and folding that into severity would
	// leave the statusbar badge permanently red — the tint comes from the
	// run's exit status, which is the thing that actually means failure.
	Stderr bool
	// RunID is non-zero for records belonging to a runner.Capture run.
	RunID int64
}

// Closed is the value the output screen pops with.
//
// screen.Pop fires OnEnter on the screen it uncovers, and OnEnter is the
// documented place to kick off a fetch — so without a sentinel, glancing at
// the log would silently refetch whatever was underneath. A parent that
// cares can early-return on this; one that doesn't can ignore it.
//
// It lives here rather than in pkg/app because pkg/app imports pkg/output,
// so the reverse would be a cycle. pkg/app aliases it as app.OutputClosed
// for screens that would rather not take a second import.
type Closed struct{}

// Options configures the buffer and its screen. Build it with OptionsFrom
// and override individual fields.
type Options struct {
	// MaxRecords caps the ring. 0 applies DefaultMaxRecords. Trimming is
	// event-aware: whole events are dropped from the front, so a surviving
	// body line always still has the head naming its command.
	MaxRecords int

	// ExportDir is where "w" writes. Empty falls back to os.TempDir().
	ExportDir string

	// SourceWidth is the column the source name is padded to. 0 applies
	// DefaultSourceWidth.
	SourceWidth int

	// Line colors. These are applied as foreground-only SGR (closing with
	// \x1b[39m) rather than through lipgloss, because logview's
	// CurrentLineStyle pads the current row to the pane width to paint a
	// background — a full \x1b[0m reset inside the prefix would punch a
	// hole in it. Same reasoning as CLAUDE.md rule 17 for table cells.
	TimeColor   lipgloss.TerminalColor
	InfoColor   lipgloss.TerminalColor
	ErrorColor  lipgloss.TerminalColor
	SourceColor lipgloss.TerminalColor
	GutterColor lipgloss.TerminalColor

	// BadgeStyle and BadgeErrorStyle render the statusbar affordance. They
	// live here rather than in pkg/app so the badge's colors sit with the
	// rest of the console's palette; pkg/app reads them when composing the
	// bar's right slot.
	BadgeStyle      lipgloss.Style
	BadgeErrorStyle lipgloss.Style

	// Logview configures the screen's body.
	Logview logview.Options
	// Confirm configures the kill confirmation modal.
	Confirm confirm.Options
	// Picker configures the run picker shown when more than one capture is
	// in flight.
	Picker list.Options

	// Keys is the screen's own keymap — the actions layered on top of the
	// logview's. Zero-valued bindings fall back to DefaultKeys.
	Keys Keys
}

// Keys is the output screen's keymap, following CLAUDE.md rule 24: every
// binding the screen dispatches against lives here, carrying both its
// dispatch keys and its help label so Update and Help() read from one
// source.
//
// Kill is "x" rather than the obvious "k": k is scroll-up everywhere in the
// library and rule 23 does not bend for it.
type Keys struct {
	Clear, Kill, Export key.Binding
}

// DefaultKeys returns the stock keymap.
func DefaultKeys() Keys {
	return Keys{
		Clear:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
		Kill:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "kill run")),
		Export: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "export")),
	}
}

// FillDefaults fills any zero-valued binding with its DefaultKeys
// counterpart, so a partial override works without restating the rest.
// Exported to match pane.Keys, since pkg/app merges a caller's keymap
// across theme swaps.
func (k *Keys) FillDefaults() {
	d := DefaultKeys()
	if len(k.Clear.Keys()) == 0 {
		k.Clear = d.Clear
	}
	if len(k.Kill.Keys()) == 0 {
		k.Kill = d.Kill
	}
	if len(k.Export.Keys()) == 0 {
		k.Export = d.Export
	}
}

// OptionsFrom returns Options pre-filled from a theme — the console's
// equivalent of the th.Component() builders, inverted for the import-cycle
// reason described in the package doc.
func OptionsFrom(t theme.Theme) Options {
	lv := t.Logview()
	lv.Title = "Output"
	lv.Searchable = true
	// The buffer owns the cap (and trims on event boundaries); a second cap
	// inside the logview would cut the same stream at different places.
	lv.MaxLines = -1

	conf := t.Confirm()
	conf.Title = "Kill run"

	picker := t.List()
	picker.Title = "Kill which run?"

	badge := lipgloss.NewStyle().Padding(0, 1).
		Background(t.BarBG).Foreground(t.BarFG)
	badgeErr := lipgloss.NewStyle().Padding(0, 1).
		Background(t.ErrorBG).Foreground(t.ErrorFG)

	return Options{
		MaxRecords:      DefaultMaxRecords,
		SourceWidth:     DefaultSourceWidth,
		TimeColor:       t.Subtle,
		InfoColor:       t.Muted,
		ErrorColor:      t.ErrorBG,
		SourceColor:     t.Accent,
		GutterColor:     t.Subtle,
		BadgeStyle:      badge,
		BadgeErrorStyle: badgeErr,
		Logview:         lv,
		Confirm:         conf,
		Picker:          picker,
		Keys:            DefaultKeys(),
	}
}

// fillDefaults applies fallbacks for the numeric knobs so a hand-built
// Options is usable without restating them.
func (o *Options) fillDefaults() {
	if o.MaxRecords == 0 {
		o.MaxRecords = DefaultMaxRecords
	}
	if o.SourceWidth <= 0 {
		o.SourceWidth = DefaultSourceWidth
	}
}
