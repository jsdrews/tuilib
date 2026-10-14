// Package theme collapses every color used by more than one tuilib component
// into a single struct. Components still accept raw lipgloss values — theme
// just makes the common case (one palette, many components) a one-liner.
//
// Usage:
//
//	th := theme.New(palette.Dark())
//	bc := breadcrumb.New(th.Breadcrumb())                     // header
//	h  := help.New(th.Help())                                 // inline hints
//	sb := statusbar.New(th.Statusbar(h.ShortView(), "v0.1.0")) // footer
//	p  := pane.New(th.Pane())                                 // body
//
// Theme fills in the chrome tokens — colors, and the surface's border shape
// and slot brackets — so every component drawn from one palette agrees. Anything you need to
// override for a single component (a custom width, a deliberately heavier
// border on one pane) you set on the returned Options before passing it to
// that component's New; a per-instance value always wins over the theme.
package theme

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/action"
	"github.com/jsdrews/tuilib/pkg/activity"
	"github.com/jsdrews/tuilib/pkg/alert"
	"github.com/jsdrews/tuilib/pkg/ansi"
	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/confirm"
	"github.com/jsdrews/tuilib/pkg/eventlog"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/glyph"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/inspector"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/palette"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/surface"
	"github.com/jsdrews/tuilib/pkg/table"
	"github.com/jsdrews/tuilib/pkg/textview"
	"github.com/jsdrews/tuilib/pkg/toggle"
	"github.com/jsdrews/tuilib/pkg/tree"
)

// Theme is what components are built from: a palette, drawn with a
// surface, using a set of glyphs. Nobody picks a theme directly — the user
// picks a palette and a surface, and the app shell composes them (Compose)
// and hands the result to every screen's SetTheme.
//
// The palette's colours are promoted, so t.Accent and t.BorderActive read as
// they always did.
type Theme struct {
	palette.Palette

	// Glyphs are the marks components draw — row cursors, expand arrows,
	// scrollbar thumbs, sort indicators. Empty fields fall back to
	// glyph.Default, so setting one arrow does not blank the rest.
	Glyphs glyph.Set

	// Surface is how components' edges and bodies are drawn, resolved
	// against this theme's palette and the terminal. The builders hand each
	// component the form it needs — the surface itself, its overlay form for
	// modals, its field form for single-line inputs.
	Surface surface.Resolved
}

// New is p drawn Outlined: the look of an app that never chose a surface.
func New(p palette.Palette) Theme {
	return Compose(p, surface.Outlined, surface.Caps{Profile: termenv.TrueColor, BlockGlyphs: true})
}

// Compose builds the theme for one palette drawn with one surface, on a
// terminal offering caps. The shell calls it whenever the user changes
// either axis.
func Compose(p palette.Palette, s surface.Surface, caps surface.Caps) Theme {
	t := Theme{Palette: p}
	t.Surface = surface.Resolve(s, p.Colours(caps.Profile), caps)
	role := t.Surface.Surface().Bar
	if bar := t.Surface.Colour(role, false); bar != nil {
		t.BarBG = bar
		// BarFG was chosen to read on the palette's own bar colour. Moved
		// onto another — the screen, for the filled surfaces — it can sink
		// into it; the surface's Text colour is chosen to read there.
		text := t.Surface.Colour(surface.Text, false)
		if role != surface.Bar && !surface.Legible(t.BarFG, bar) && surface.Legible(text, bar) {
			t.BarFG = text
		}
	}
	return t
}

// Resolve resolves s against this theme's palette and terminal — how a
// screen pins one component to a surface other than the app's:
//
//	opts := t.Logview()
//	opts.Surface = t.Resolve(surface.Outlined)
func (t Theme) Resolve(s surface.Surface) surface.Resolved { return t.Surface.With(s) }

// ---- Per-component Options builders ----------------------------------------

// shapeActive / shapeInactive / shapeOverlay are the line borders components
// draw with: the surface's own lines when it is lined (Outlined, rounded,
// thick), and the library defaults when it isn't — a filled surface's field
// form and the fallback path. Every builder goes through these so there is
// one place that decides the shape.
func (t Theme) shapeActive() lipgloss.Border {
	if b, lined := t.Surface.Surface().Lined(); lined {
		return b
	}
	return lipgloss.NormalBorder()
}

func (t Theme) shapeInactive() lipgloss.Border {
	if b, lined := t.Surface.Surface().Lined(); lined {
		return b
	}
	return lipgloss.NormalBorder()
}

func (t Theme) shapeOverlay() lipgloss.Border {
	if b, lined := t.Surface.Overlay().Surface().Lined(); lined {
		return b
	}
	return lipgloss.ThickBorder()
}

// selection is a selected row's style, focused and blurred: legacy, the
// component's own, while the surface keeps the theme's selection (Outlined
// does, so it never dims); otherwise the surface's Selection tone under its
// Text colour, dimming to SelectionDim when the pane loses focus.
func (t Theme) selection(legacy lipgloss.Style) (focused, blurred lipgloss.Style) {
	sel := t.Surface.Surface().Selection
	if t.Surface.IsZero() || sel.FromTheme {
		return legacy, lipgloss.Style{}
	}
	style := func(bg surface.Role) lipgloss.Style {
		st := lipgloss.NewStyle()
		if c := t.Surface.Colour(bg, true); c != nil {
			st = st.Background(c)
		}
		if c := t.Surface.Colour(sel.FG, true); c != nil {
			st = st.Foreground(c)
		}
		return st
	}
	blurredBG := sel.BGBlurred
	if blurredBG == surface.None {
		blurredBG = sel.BG
	}
	return style(sel.BG), style(blurredBG)
}

// glyphs resolves the theme's glyph set against the library defaults. Every
// builder goes through it so a partially-filled Set never reaches a component
// as empty strings.
func (t Theme) glyphs() glyph.Set { return t.Glyphs.Resolve() }

// cellColor renders text in c with a foreground-only escape, so a table's
// selected-row background passes through it unbroken (rule 19).
//
// It exists because pkg/ansi is dependency-free and takes a palette index,
// while a Theme carries a lipgloss color. Before it, theme.Table hardcoded
// index 240 for its separators, which meant table chrome rendered the same
// grey in every palette — light themes included — and was the one piece of
// chrome a theme could not reach.
//
// A color that is neither a palette index nor a hex literal is left unstyled
// rather than guessed at: an uncolored separator is legible, a wrong one is
// not.
func cellColor(c lipgloss.TerminalColor, text string) string {
	col, ok := c.(lipgloss.Color)
	if !ok {
		return text
	}
	v := string(col)
	if n, err := strconv.Atoi(v); err == nil {
		return ansi.CellColor(n, text)
	}
	if r, g, b, ok := parseHex(v); ok {
		return ansi.CellColorRGB(r, g, b, text)
	}
	return text
}

// parseHex reads "#rgb" and "#rrggbb".
func parseHex(v string) (r, g, b uint8, ok bool) {
	if !strings.HasPrefix(v, "#") {
		return 0, 0, 0, false
	}
	switch h := v[1:]; len(h) {
	case 3:
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return 0, 0, 0, false
		}
		r = uint8((n>>8)&0xf) * 17
		g = uint8((n>>4)&0xf) * 17
		b = uint8(n&0xf) * 17
		return r, g, b, true
	case 6:
		n, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return 0, 0, 0, false
		}
		return uint8(n >> 16), uint8(n >> 8), uint8(n), true
	}
	return 0, 0, 0, false
}

// Breadcrumb returns breadcrumb.Options pre-filled from the theme. Mutate the
// returned value for any non-theme fields (Width, Crumbs, Separator, …).
func (t Theme) Breadcrumb() breadcrumb.Options {
	return breadcrumb.Options{
		BarBackground: t.BarBG,
		BarForeground: t.BarFG,
	}
}

// Statusbar returns statusbar.Options pre-filled from the theme, with Left
// and Right passed through so you don't have to set them separately.
func (t Theme) Statusbar(left, right string) statusbar.Options {
	infoStyle := lipgloss.NewStyle().Padding(0, 1).Background(t.InfoBG).Foreground(t.InfoFG)
	errStyle := lipgloss.NewStyle().Padding(0, 1).Background(t.ErrorBG).Foreground(t.ErrorFG)
	return statusbar.Options{
		Left:          left,
		Right:         right,
		BarBackground: t.BarBG,
		BarForeground: t.BarFG,
		InfoStyle:     &infoStyle,
		ErrorStyle:    &errStyle,
	}
}

// Help returns help.Options whose KeyStyle and DescStyle carry the same
// Background as the bar, so help.ShortView() drops into Statusbar.Left with
// no banding. Override ShortSeparator or the styles as needed.
func (t Theme) Help() help.Options {
	return help.Options{
		KeyStyle:  lipgloss.NewStyle().Bold(true).Foreground(t.KeyFG).Background(t.BarBG),
		DescStyle: lipgloss.NewStyle().Foreground(t.BarFG).Background(t.BarBG),
	}
}

// HelpOverlay returns help.OverlayOptions pre-filled from the theme — the
// key column in KeyFG, descriptions in BarFG, section headings bold in
// Accent, and the overlay border shape the other modals use. Set Title and
// Searchable before passing to help.NewOverlay.
//
// The styles carry no Background: unlike Help(), which is embedded in the
// statusbar and has to match its band, the overlay draws on the pane's own
// ground.
func (t Theme) HelpOverlay() help.OverlayOptions {
	o := help.OverlayOptions{
		KeyStyle:           lipgloss.NewStyle().Bold(true).Foreground(t.KeyFG),
		DescStyle:          lipgloss.NewStyle().Foreground(t.BarFG),
		SectionStyle:       lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		EmptyStyle:         lipgloss.NewStyle().Foreground(t.Muted),
		ActiveColor:        t.BorderActive,
		InactiveColor:      t.BorderInactive,
		ActiveBorder:       t.shapeOverlay(),
		InactiveBorder:     t.shapeOverlay(),
		Glyphs:             t.glyphs(),
		SlotBrackets:       t.Surface.Surface().Brackets,
		FilterRuleActive:   lipgloss.NewStyle().Foreground(t.BorderActive),
		FilterRuleInactive: lipgloss.NewStyle().Foreground(t.BorderInactive),
		Filter:             t.Filter(),
		Keys:               help.DefaultOverlayKeys(),
	}
	o.Surface = t.Surface.Overlay()
	return o
}

// Pane returns pane.Options with only the color tokens applied. Border shape,
// title, title position, and slot brackets stay the caller's call. Includes
// a SpinnerStyle so panes set into a loading state render the spinner in the
// theme's accent color.
func (t Theme) Pane() pane.Options {
	o := pane.Options{
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   t.shapeActive(),
		InactiveBorder: t.shapeInactive(),
		Glyphs:         t.glyphs(),
		SlotBrackets:   t.Surface.Surface().Brackets,
		SpinnerStyle:   lipgloss.NewStyle().Foreground(t.Accent),
	}
	o.Surface = t.Surface
	return o
}

// Filter returns filter.Options pre-filled with theme colors — prompt in
// KeyFG, text in BarFG, placeholder in Subtle, cursor in Accent, border
// colors matching pane. Set Width and override anything else on the returned
// value before passing to filter.New.
func (t Theme) Filter() filter.Options {
	o := filter.Options{
		PromptStyle:      lipgloss.NewStyle().Bold(true).Foreground(t.KeyFG),
		TextStyle:        lipgloss.NewStyle().Foreground(t.BarFG),
		PlaceholderStyle: lipgloss.NewStyle().Foreground(t.Subtle),
		CursorColor:      t.Accent,
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     t.shapeActive(),
		InactiveBorder:   t.shapeInactive(),
		Glyphs:           t.glyphs(),
		SlotBrackets:     t.Surface.Surface().Brackets,
	}
	o.Surface = t.Surface.Field()
	return o
}

// List returns list.Options pre-filled from the theme, including nested
// Filter options (used only if Filterable=true). HScrollbar is enabled by
// default so long item labels stay readable via ←/→ / h/l. Set Width,
// Height, Title, Items, Filterable, and any placeholder via
// Filter.Placeholder.
func (t Theme) List() list.Options {
	o := list.Options{
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   t.shapeActive(),
		InactiveBorder: t.shapeInactive(),
		Glyphs:         t.glyphs(),
		SlotBrackets:   t.Surface.Surface().Brackets,
		SelectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		MarkStyle:      lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		HScrollbar:     true,
		SpinnerStyle:   lipgloss.NewStyle().Foreground(t.Accent),
		Activity:       t.Activity(),
		Filter:         t.Filter(),
		Keys:           list.DefaultKeys(),
	}
	o.Surface = t.Surface
	o.SelectedStyle, o.SelectedBlurredStyle = t.selection(o.SelectedStyle)
	return o
}

// Input returns input.Options pre-filled from the theme — text in BarFG,
// placeholder in Subtle, cursor in Accent, border colors matching pane.
// Set Width, Title (the field's label), and any placeholder before passing
// to input.New.
func (t Theme) Input() input.Options {
	o := input.Options{
		TextStyle:        lipgloss.NewStyle().Foreground(t.BarFG),
		PlaceholderStyle: lipgloss.NewStyle().Foreground(t.Subtle),
		CursorColor:      t.Accent,
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     t.shapeActive(),
		InactiveBorder:   t.shapeInactive(),
		Glyphs:           t.glyphs(),
		SlotBrackets:     t.Surface.Surface().Brackets,
	}
	o.Surface = t.Surface.Field()
	return o
}

// Toggle returns toggle.Options pre-filled from the theme — selected side in
// Accent (bold), unselected in Muted, border colors matching pane. Set
// Width, Title (the field's question), and Initial as needed.
func (t Theme) Toggle() toggle.Options {
	o := toggle.Options{
		SelectedStyle:   lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		UnselectedStyle: lipgloss.NewStyle().Foreground(t.Muted),
		ActiveColor:     t.BorderActive,
		InactiveColor:   t.BorderInactive,
		ActiveBorder:    t.shapeActive(),
		InactiveBorder:  t.shapeInactive(),
		Glyphs:          t.glyphs(),
		SlotBrackets:    t.Surface.Surface().Brackets,
	}
	o.Surface = t.Surface.Field()
	return o
}

// Confirm returns confirm.Options pre-filled from the theme — selected
// button in Accent (bold), unselected in Muted, message body in BarFG,
// border colors matching pane. Set Width, Height, Title, Message,
// Confirm/Cancel labels, and Initial as needed.
func (t Theme) Confirm() confirm.Options {
	o := confirm.Options{
		SelectedStyle:   lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		UnselectedStyle: lipgloss.NewStyle().Foreground(t.Muted),
		MessageStyle:    lipgloss.NewStyle().Foreground(t.BarFG),
		ActiveColor:     t.BorderActive,
		InactiveColor:   t.BorderInactive,
		ActiveBorder:    t.shapeOverlay(),
		InactiveBorder:  t.shapeOverlay(),
		Glyphs:          t.glyphs(),
		SlotBrackets:    t.Surface.Surface().Brackets,
	}
	o.Surface = t.Surface.Overlay()
	return o
}

// Alert returns alert.Options pre-filled from the theme — OK button in
// Accent (bold), message body in BarFG, neutral active/inactive border
// colors. The chrome is intentionally neutral; for an error-styled alert,
// override ActiveColor with t.ErrorBG (and OKStyle's foreground with
// t.ErrorBG) after this builder. Set Width, Height, Title, Message, and
// OK label as needed before passing to alert.New.
func (t Theme) Alert() alert.Options {
	o := alert.Options{
		OKStyle:        lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		MessageStyle:   lipgloss.NewStyle().Foreground(t.BarFG),
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   t.shapeOverlay(),
		InactiveBorder: t.shapeOverlay(),
		Glyphs:         t.glyphs(),
		SlotBrackets:   t.Surface.Surface().Brackets,
	}
	o.Surface = t.Surface.Overlay()
	return o
}

// Actions returns action.Options pre-filled from the theme — the label in
// BarFG, the gloss and the key column in Muted, the cursor row bold in Accent,
// and disabled rows in Subtle so an unavailable verb reads as present but out
// of reach rather than as ordinary text. Set Title and Set before passing to
// action.New.
//
// Unlike pkg/output, this follows rule 3's th.Component() convention: a Menu
// is a plain component rather than a screen.Screen, so pkg/action never
// imports pkg/screen and there is no import cycle to invert.
func (t Theme) Actions() action.Options {
	o := action.Options{
		LabelStyle:     lipgloss.NewStyle().Foreground(t.BarFG),
		DescStyle:      lipgloss.NewStyle().Foreground(t.Muted),
		KeyStyle:       lipgloss.NewStyle().Foreground(t.KeyFG),
		SelectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		DisabledStyle:  lipgloss.NewStyle().Foreground(t.Subtle),
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   t.shapeOverlay(),
		InactiveBorder: t.shapeOverlay(),
		Glyphs:         t.glyphs(),
		SlotBrackets:   t.Surface.Surface().Brackets,
		Keys:           action.DefaultKeys(),
	}
	o.Surface = t.Surface.Overlay()
	return o
}

// Table returns table.Options pre-filled from the theme — header bold +
// Current fg, selected row bold + Accent fg + Subtle bg, border colors
// matching pane, HScrollbar enabled (wide tables are common), spinner in
// Accent, and the embedded filter using theme.Filter(). Set Width,
// Height, Title, Columns, Rows, Filterable, and any Filter.Placeholder
// before passing to table.New.
func (t Theme) Table() table.Options {
	o := table.Options{
		HeaderStyle:    lipgloss.NewStyle().Bold(true).Foreground(t.Current),
		SelectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Background(t.Subtle),
		MarkStyle:      lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   t.shapeActive(),
		InactiveBorder: t.shapeInactive(),
		Glyphs:         t.glyphs(),
		SlotBrackets:   t.Surface.Surface().Brackets,
		HScrollbar:     true,
		SpinnerStyle:   lipgloss.NewStyle().Foreground(t.Accent),
		Activity:       t.activityCell(),
		Filter:         t.Filter(),
		Borders: table.Borders{
			Vertical:   cellColor(t.Subtle, t.glyphs().ColumnSep),
			HeaderRule: cellColor(t.Subtle, t.Surface.Surface().Divider.Glyph),
		},
		Keys: table.DefaultKeys(),
	}
	o.Surface = t.Surface
	o.SelectedStyle, o.SelectedBlurredStyle = t.selection(o.SelectedStyle)
	return o
}

// Logview returns logview.Options pre-filled from the theme — match
// highlight in Accent (bold + reverse), border colors matching pane,
// and the embedded filter using theme.Filter(). Set Width, Height, Title,
// Searchable, MaxLines, and any Filter.Placeholder before passing to
// logview.New.
func (t Theme) Logview() logview.Options {
	o := logview.Options{
		MatchStyle:       lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(t.Accent),
		CurrentLineStyle: lipgloss.NewStyle().Background(t.Subtle),
		GutterStyle:      lipgloss.NewStyle().Foreground(t.Muted),
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     t.shapeActive(),
		InactiveBorder:   t.shapeInactive(),
		Glyphs:           t.glyphs(),
		SlotBrackets:     t.Surface.Surface().Brackets,
		HScrollbar:       true,
		SpinnerStyle:     lipgloss.NewStyle().Foreground(t.Accent),
		Filter:           t.Filter(),
		Keys:             logview.DefaultKeys(),
	}
	o.Surface = t.Surface
	o.CurrentLineStyle, o.CurrentLineBlurredStyle = t.selection(o.CurrentLineStyle)
	return o
}

// Eventlog returns eventlog.Options pre-filled from the theme — the
// selected item in Accent on Subtle like a table row, match highlight like
// the logview's, border colors matching pane. Set Title, Anchored,
// Filterable and Searchable on the returned value before eventlog.New.
func (t Theme) Eventlog() eventlog.Options {
	o := eventlog.Options{
		MatchStyle:     lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(t.Accent),
		SelectedStyle:  lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Background(t.Subtle),
		GutterStyle:    lipgloss.NewStyle().Foreground(t.Muted),
		NewStyle:       lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   t.shapeActive(),
		InactiveBorder: t.shapeInactive(),
		Glyphs:         t.glyphs(),
		SlotBrackets:   t.Surface.Surface().Brackets,
		HScrollbar:     true,
		SpinnerStyle:   lipgloss.NewStyle().Foreground(t.Accent),
		Filter:         t.Filter(),
		Keys:           eventlog.DefaultKeys(),
	}
	o.Surface = t.Surface
	o.SelectedStyle, o.SelectedBlurredStyle = t.selection(o.SelectedStyle)
	return o
}

// TextView returns textview.Options pre-filled from the theme — match
// highlight in Accent (bold + reverse), current-line background in
// Subtle, border colors matching pane, and the embedded filter using
// theme.Filter(). Set Width, Height, Title, Content, Wrap, and
// Searchable on the returned value before passing to textview.New.
func (t Theme) TextView() textview.Options {
	o := textview.Options{
		MatchStyle:       lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(t.Accent),
		CurrentLineStyle: lipgloss.NewStyle().Background(t.Subtle),
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     t.shapeActive(),
		InactiveBorder:   t.shapeInactive(),
		Glyphs:           t.glyphs(),
		SlotBrackets:     t.Surface.Surface().Brackets,
		SpinnerStyle:     lipgloss.NewStyle().Foreground(t.Accent),
		Filter:           t.Filter(),
		Keys:             textview.DefaultKeys(),
		Wrap:             true,
	}
	o.Surface = t.Surface
	o.CurrentLineStyle, o.CurrentLineBlurredStyle = t.selection(o.CurrentLineStyle)
	return o
}

// Tree returns tree.Options pre-filled from the theme — match highlight
// in Accent (bold + reverse), current-line background in Subtle, border
// colors matching pane, and the embedded filter using theme.Filter().
// Set Width, Height, Title, Root, Searchable, and InitialDepth before
// passing to tree.New.
func (t Theme) Tree() tree.Options {
	o := tree.Options{
		MatchStyle:       lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(t.Accent),
		CurrentLineStyle: lipgloss.NewStyle().Background(t.Subtle),
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     t.shapeActive(),
		InactiveBorder:   t.shapeInactive(),
		Glyphs:           t.glyphs(),
		SlotBrackets:     t.Surface.Surface().Brackets,
		HScrollbar:       true,
		SpinnerStyle:     lipgloss.NewStyle().Foreground(t.Accent),
		MarkStyle:        lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		Activity:         t.Activity(),
		Filter:           t.Filter(),
		Keys:             tree.DefaultKeys(),
	}
	o.Surface = t.Surface
	o.CurrentLineStyle, o.CurrentLineBlurredStyle = t.selection(o.CurrentLineStyle)
	return o
}

// Inspector returns inspector.Options pre-filled with theme colors. Set
// Title, Fields, Filterable, and InitialDepth on the returned value.
func (t Theme) Inspector() inspector.Options {
	o := inspector.Options{
		// Labels use Muted (semantic "secondary text"), not Subtle —
		// CurrentLineStyle below sets bg=Subtle for the cursor row, and
		// dimming labels to Subtle too would make them invisible on that
		// row. Muted is reliably distinct from Subtle across themes.
		LabelStyle:       lipgloss.NewStyle().Foreground(t.Muted),
		ValueStyle:       lipgloss.NewStyle().Foreground(t.Current),
		MatchStyle:       lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(t.Accent),
		CurrentLineStyle: lipgloss.NewStyle().Background(t.Subtle),
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     t.shapeActive(),
		InactiveBorder:   t.shapeInactive(),
		Glyphs:           t.glyphs(),
		SlotBrackets:     t.Surface.Surface().Brackets,
		HScrollbar:       true,
		SpinnerStyle:     lipgloss.NewStyle().Foreground(t.Accent),
		Filter:           t.Filter(),
		Keys:             inspector.DefaultKeys(),
	}
	o.Surface = t.Surface
	o.CurrentLineStyle, o.CurrentLineBlurredStyle = t.selection(o.CurrentLineStyle)
	return o
}

// Form returns form.Options pre-filled with theme colors. Set Width, Height,
// Fields, and any SubmitText on the returned value before passing to form.New
// (or chain `.With(fields)`). Override individual Styles fields only if you
// need to deviate from the palette.
func (t Theme) Form() form.Options {
	return form.Options{
		Styles: form.Styles{
			Input:               lipgloss.NewStyle().Foreground(t.BarFG),
			Placeholder:         lipgloss.NewStyle().Foreground(t.Subtle),
			CursorColor:         t.Accent,
			Selected:            lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
			PaneActiveColor:     t.BorderActive,
			PaneInactiveColor:   t.BorderInactive,
			FieldBorderActive:   t.shapeActive(),
			FieldBorderInactive: t.shapeInactive(),
			ErrorColor:          t.ErrorBG,
			ErrorText:           lipgloss.NewStyle().Bold(true).Foreground(t.ErrorBG),
			Submit:              lipgloss.NewStyle().Foreground(t.BarFG),
			SubmitActive:        lipgloss.NewStyle().Bold(true).Foreground(t.BarBG).Background(t.Accent),
		},
	}
}

// Activity returns activity.Options pre-filled from the theme — the spinner
// and its label in Accent.
//
// Nested into List() and Tree(), which render their own rows and can use
// lipgloss freely. pkg/table needs the cell-safe form instead; see
// activityCell.
func (t Theme) Activity() activity.Options {
	running := lipgloss.NewStyle().Foreground(t.Accent)
	unknown := lipgloss.NewStyle().Foreground(t.Muted)
	return activity.Options{
		Style: func(st activity.State, text string) string {
			if st.Unknown {
				return unknown.Render(text)
			}
			return running.Render(text)
		},
	}
}

// activityCell is Activity with foreground-only escapes.
//
// A table draws its cursor row as one styled run over the whole row, so a
// lipgloss render inside a cell closes the highlight at its first reset and
// punches a hole in the selected row's background (rule 19). cellColor emits
// \x1b[39m instead, which the row's Background survives.
func (t Theme) activityCell() activity.Options {
	return activity.Options{
		Style: func(st activity.State, text string) string {
			if st.Unknown {
				return cellColor(t.Muted, text)
			}
			return cellColor(t.Accent, text)
		},
	}
}
