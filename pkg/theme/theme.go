// Package theme collapses every color used by more than one tuilib component
// into a single struct. Components still accept raw lipgloss values — theme
// just makes the common case (one palette, many components) a one-liner.
//
// Usage:
//
//	th := theme.Dark()
//	bc := breadcrumb.New(th.Breadcrumb())                     // header
//	h  := help.New(th.Help())                                 // inline hints
//	sb := statusbar.New(th.Statusbar(h.ShortView(), "v0.1.0")) // footer
//	p  := pane.New(th.Pane())                                 // body
//
// Anything you need to override — a non-default border shape, a slot-brackets
// style, a custom width — you set on the returned Options before passing it to
// each component's New. Theme only fills in the color/style tokens.
package theme

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/jsdrews/tuilib/pkg/breadcrumb"
	"github.com/jsdrews/tuilib/pkg/filter"
	"github.com/jsdrews/tuilib/pkg/form"
	"github.com/jsdrews/tuilib/pkg/help"
	"github.com/jsdrews/tuilib/pkg/input"
	"github.com/jsdrews/tuilib/pkg/list"
	"github.com/jsdrews/tuilib/pkg/logview"
	"github.com/jsdrews/tuilib/pkg/pane"
	"github.com/jsdrews/tuilib/pkg/statusbar"
	"github.com/jsdrews/tuilib/pkg/toggle"
)

// Theme is a named palette. Every field is a color token consumed by one or
// more components. Most tokens have a single semantic role so swapping themes
// is a matter of rewriting this struct.
type Theme struct {
	// Name is a short identifier for debugging / theme pickers.
	Name string

	// BarBG / BarFG are the shared surface colors for the breadcrumb header,
	// the status bar footer, and anything embedded in them (help hints).
	// Must be a single pair so the strip reads as one continuous band.
	BarBG lipgloss.TerminalColor
	BarFG lipgloss.TerminalColor

	// Current is the emphasis color for the active crumb / current selection.
	Current lipgloss.TerminalColor
	// Muted is for past crumbs and secondary text on the bar.
	Muted lipgloss.TerminalColor
	// Subtle is for separators and faint chrome.
	Subtle lipgloss.TerminalColor

	// KeyFG colors the key labels inside help hints (bold).
	KeyFG lipgloss.TerminalColor

	// BorderActive / BorderInactive color pane borders.
	BorderActive   lipgloss.TerminalColor
	BorderInactive lipgloss.TerminalColor

	// Info / Error style the statusbar's middle slot in each message state.
	InfoBG, InfoFG   lipgloss.TerminalColor
	ErrorBG, ErrorFG lipgloss.TerminalColor

	// Accent is a bold highlight for body content (not used by chrome).
	// Exposed here so screens can reach for it without redeclaring a color.
	Accent lipgloss.TerminalColor
}

// Dark is the default palette used across the examples — matches pug's look.
func Dark() Theme {
	return Theme{
		Name:           "dark",
		BarBG:          lipgloss.Color("236"),
		BarFG:          lipgloss.Color("252"),
		Current:        lipgloss.Color("255"),
		Muted:          lipgloss.Color("244"),
		Subtle:         lipgloss.Color("240"),
		KeyFG:          lipgloss.Color("75"),
		BorderActive:   lipgloss.Color("12"),
		BorderInactive: lipgloss.Color("240"),
		InfoBG:         lipgloss.Color("35"),
		InfoFG:         lipgloss.Color("0"),
		ErrorBG:        lipgloss.Color("160"),
		ErrorFG:        lipgloss.Color("15"),
		Accent:         lipgloss.Color("214"), // warm gold — pops against charcoal + cyan keys
	}
}

// Accent is a variant with a colored header strip — the same scheme as
// rendercheck variant 2. Good for apps where the header should pop.
func Accent() Theme {
	t := Dark()
	t.Name = "accent"
	t.BarBG = lipgloss.Color("24") // teal-blue
	t.BarFG = lipgloss.Color("195")
	t.Muted = lipgloss.Color("152")
	t.Subtle = lipgloss.Color("110")
	t.Current = lipgloss.Color("231")
	t.KeyFG = lipgloss.Color("195")
	t.Accent = lipgloss.Color("222") // pale amber — warm counterpoint to teal
	return t
}

// Light inverts Dark: pale bars with dark text. Key/border lean teal so the
// interactive bits stay legible against the light surface.
func Light() Theme {
	return Theme{
		Name:           "light",
		BarBG:          lipgloss.Color("254"),
		BarFG:          lipgloss.Color("236"),
		Current:        lipgloss.Color("232"),
		Muted:          lipgloss.Color("240"),
		Subtle:         lipgloss.Color("248"),
		KeyFG:          lipgloss.Color("24"),
		BorderActive:   lipgloss.Color("24"),
		BorderInactive: lipgloss.Color("248"),
		InfoBG:         lipgloss.Color("22"),
		InfoFG:         lipgloss.Color("255"),
		ErrorBG:        lipgloss.Color("124"),
		ErrorFG:        lipgloss.Color("255"),
		Accent:         lipgloss.Color("161"),
	}
}

// Solarized maps Ethan Schoonover's solarized-dark palette onto the 256-color
// approximation. Warm text on a deep blue-green surface.
func Solarized() Theme {
	return Theme{
		Name:           "solarized",
		BarBG:          lipgloss.Color("234"), // base03
		BarFG:          lipgloss.Color("245"), // base1
		Current:        lipgloss.Color("250"), // base2
		Muted:          lipgloss.Color("244"), // base0
		Subtle:         lipgloss.Color("240"), // base01
		KeyFG:          lipgloss.Color("37"),  // cyan
		BorderActive:   lipgloss.Color("33"),  // blue
		BorderInactive: lipgloss.Color("240"),
		InfoBG:         lipgloss.Color("64"),  // green
		InfoFG:         lipgloss.Color("234"),
		ErrorBG:        lipgloss.Color("160"), // red
		ErrorFG:        lipgloss.Color("255"),
		Accent:         lipgloss.Color("166"), // orange
	}
}

// Nord is the Nord palette (arcticicestudio/nord) mapped to 256-color.
// Cool frost tones; reads calmer than Dark.
func Nord() Theme {
	return Theme{
		Name:           "nord",
		BarBG:          lipgloss.Color("236"), // nord0
		BarFG:          lipgloss.Color("252"), // nord4
		Current:        lipgloss.Color("255"), // nord6
		Muted:          lipgloss.Color("109"), // nord8 (frost)
		Subtle:         lipgloss.Color("239"), // nord3
		KeyFG:          lipgloss.Color("110"), // nord9
		BorderActive:   lipgloss.Color("67"),  // nord10
		BorderInactive: lipgloss.Color("239"),
		InfoBG:         lipgloss.Color("150"), // nord14 aurora green
		InfoFG:         lipgloss.Color("236"),
		ErrorBG:        lipgloss.Color("131"), // nord11 aurora red
		ErrorFG:        lipgloss.Color("255"),
		Accent:         lipgloss.Color("139"), // nord15 purple
	}
}

// Dracula is the Dracula palette mapped to 256-color — saturated pinks and
// purples on a near-black surface. Good for "this tool is fun to use."
func Dracula() Theme {
	return Theme{
		Name:           "dracula",
		BarBG:          lipgloss.Color("235"),
		BarFG:          lipgloss.Color("252"),
		Current:        lipgloss.Color("255"),
		Muted:          lipgloss.Color("61"),  // comment
		Subtle:         lipgloss.Color("238"),
		KeyFG:          lipgloss.Color("141"), // purple
		BorderActive:   lipgloss.Color("212"), // pink
		BorderInactive: lipgloss.Color("238"),
		InfoBG:         lipgloss.Color("84"),  // green
		InfoFG:         lipgloss.Color("235"),
		ErrorBG:        lipgloss.Color("203"), // red
		ErrorFG:        lipgloss.Color("255"),
		Accent:         lipgloss.Color("117"), // cyan — on-brand against pink/purple chrome
	}
}

// Gruvbox maps Pavel Pertsev's gruvbox-dark palette — warm earthy tones.
func Gruvbox() Theme {
	return Theme{
		Name:           "gruvbox",
		BarBG:          lipgloss.Color("235"), // bg0
		BarFG:          lipgloss.Color("223"), // fg1
		Current:        lipgloss.Color("230"),
		Muted:          lipgloss.Color("245"), // fg4
		Subtle:         lipgloss.Color("239"), // bg2
		KeyFG:          lipgloss.Color("214"), // yellow
		BorderActive:   lipgloss.Color("208"), // orange
		BorderInactive: lipgloss.Color("239"),
		InfoBG:         lipgloss.Color("106"), // green
		InfoFG:         lipgloss.Color("235"),
		ErrorBG:        lipgloss.Color("124"), // red
		ErrorFG:        lipgloss.Color("223"),
		Accent:         lipgloss.Color("175"), // purple
	}
}

// All returns every built-in theme, in display order. Handy for theme
// pickers and examples/themecheck.
func All() []Theme {
	return []Theme{
		// core (this file)
		Dark(), Accent(), Light(), Solarized(), Nord(), Dracula(), Gruvbox(),
		// named palettes (named.go)
		CatppuccinMocha(), CatppuccinLatte(),
		TokyoNight(),
		RosePine(), RosePineDawn(),
		OneDark(), Monokai(), EverforestDark(),
		// base16 schemes (base16.go)
		Base16Ocean(), Base16Eighties(), Base16Railscasts(), Base16TomorrowNight(),
	}
}

// ---- Per-component Options builders ----------------------------------------

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
// no banding. Override ShortSeparator, ColumnSpacer, Width/Height, or the
// styles as needed.
func (t Theme) Help() help.Options {
	return help.Options{
		KeyStyle:    lipgloss.NewStyle().Bold(true).Foreground(t.KeyFG).Background(t.BarBG),
		DescStyle:   lipgloss.NewStyle().Foreground(t.BarFG).Background(t.BarBG),
		BorderColor: t.Subtle,
	}
}

// Pane returns pane.Options with only the color tokens applied. Border shape,
// title, title position, and slot brackets stay the caller's call.
func (t Theme) Pane() pane.Options {
	return pane.Options{
		ActiveColor:   t.BorderActive,
		InactiveColor: t.BorderInactive,
	}
}

// Filter returns filter.Options pre-filled with theme colors — prompt in
// KeyFG, text in BarFG, placeholder in Subtle, cursor in Accent, border
// colors matching pane. Set Width and override anything else on the returned
// value before passing to filter.New.
func (t Theme) Filter() filter.Options {
	return filter.Options{
		PromptStyle:      lipgloss.NewStyle().Bold(true).Foreground(t.KeyFG),
		TextStyle:        lipgloss.NewStyle().Foreground(t.BarFG),
		PlaceholderStyle: lipgloss.NewStyle().Foreground(t.Subtle),
		CursorColor:      t.Accent,
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		SlotBrackets:     pane.SlotBracketsNone,
	}
}

// List returns list.Options pre-filled from the theme, including nested
// Filter options (used only if Filterable=true). Set Width, Height, Title,
// Items, Filterable, and any placeholder via Filter.Placeholder.
func (t Theme) List() list.Options {
	return list.Options{
		ActiveColor:    t.BorderActive,
		InactiveColor:  t.BorderInactive,
		ActiveBorder:   lipgloss.NormalBorder(),
		InactiveBorder: lipgloss.NormalBorder(),
		SlotBrackets:   pane.SlotBracketsNone,
		SelectedColor:  t.Accent,
		Filter:         t.Filter(),
	}
}

// Input returns input.Options pre-filled from the theme — text in BarFG,
// placeholder in Subtle, cursor in Accent, border colors matching pane.
// Set Width, Title (the field's label), and any placeholder before passing
// to input.New.
func (t Theme) Input() input.Options {
	return input.Options{
		TextStyle:        lipgloss.NewStyle().Foreground(t.BarFG),
		PlaceholderStyle: lipgloss.NewStyle().Foreground(t.Subtle),
		CursorColor:      t.Accent,
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		SlotBrackets:     pane.SlotBracketsNone,
	}
}

// Toggle returns toggle.Options pre-filled from the theme — selected side in
// Accent (bold), unselected in Muted, border colors matching pane. Set
// Width, Title (the field's question), and Initial as needed.
func (t Theme) Toggle() toggle.Options {
	return toggle.Options{
		SelectedStyle:   lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
		UnselectedStyle: lipgloss.NewStyle().Foreground(t.Muted),
		ActiveColor:     t.BorderActive,
		InactiveColor:   t.BorderInactive,
		SlotBrackets:    pane.SlotBracketsNone,
	}
}

// Logview returns logview.Options pre-filled from the theme — match
// highlight in Accent (bold + reverse), border colors matching pane,
// and the embedded filter using theme.Filter(). Set Width, Height, Title,
// Searchable, MaxLines, and any Filter.Placeholder before passing to
// logview.New.
func (t Theme) Logview() logview.Options {
	return logview.Options{
		MatchStyle:       lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(t.Accent),
		CurrentLineStyle: lipgloss.NewStyle().Background(t.Subtle),
		ActiveColor:      t.BorderActive,
		InactiveColor:    t.BorderInactive,
		ActiveBorder:     lipgloss.NormalBorder(),
		InactiveBorder:   lipgloss.NormalBorder(),
		SlotBrackets:     pane.SlotBracketsNone,
		HScrollbar:       true,
		Filter:           t.Filter(),
	}
}

// Form returns form.Options pre-filled with theme colors. Set Width, Height,
// Fields, and any SubmitText on the returned value before passing to form.New
// (or chain `.With(fields)`). Override individual Styles fields only if you
// need to deviate from the palette.
func (t Theme) Form() form.Options {
	return form.Options{
		Styles: form.Styles{
			Input:        lipgloss.NewStyle().Foreground(t.BarFG),
			Placeholder:  lipgloss.NewStyle().Foreground(t.Subtle),
			CursorColor:  t.Accent,
			Selected:     lipgloss.NewStyle().Bold(true).Foreground(t.Accent),
			PaneActive:   t.BorderActive,
			PaneInactive: t.BorderInactive,
			Submit:       lipgloss.NewStyle().Foreground(t.BarFG),
			SubmitActive: lipgloss.NewStyle().Bold(true).Foreground(t.BarBG).Background(t.Accent),
		},
	}
}
