// Package palette holds the colours an app is drawn in, and the built-in
// palettes: Dark, Nord, Catppuccin, the base16 schemes and the rest.
//
// A palette is one of two axes a user picks; the other is the surface (see
// pkg/surface). The two are independent, so any palette can be drawn with any
// surface: a palette supplies two seeds, Background and Foreground, and every
// surface tone — card, band, shadow, selection — is derived from them.
//
// The package is data. It imports pkg/surface for the role names and nothing
// else from tuilib, so a CLI or a test can use a palette without pulling in a
// single component. See docs/surfaces.md.
package palette

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/surface"
)

// Palette is a named set of colours. Every field is a colour token with one
// semantic role, so switching palettes is a matter of switching values.
type Palette struct {
	// Name is a short identifier for config files and pickers.
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

	// Success is a foreground for a good outcome — Running, Ready, Succeeded,
	// Healthy — so a status value needn't reach for a literal green that
	// stays put when the palette changes. Unset, theme.Compose uses InfoBG.
	Success lipgloss.TerminalColor

	// Warning is a foreground for emphasis short of an error — a write or
	// destructive group's header in the action menu, say — so it needn't
	// borrow the error band. Unset, theme.Compose uses ErrorBG.
	Warning lipgloss.TerminalColor

	// Accent is a bold highlight for body content (not used by chrome).
	Accent lipgloss.TerminalColor

	// Background is the palette's canonical background — base16's base00,
	// Catppuccin's base, Nord's nord0 — which the screen is drawn in and
	// every surface tone lifts away from. Unset, BarBG stands in for it.
	Background lipgloss.TerminalColor
	// Foreground is the palette's canonical text colour, used for body text
	// on a filled surface. Unset, BarFG stands in for it.
	Foreground lipgloss.TerminalColor

	// Surface overrides derived surface tones by role. A missing role is
	// derived from the seeds; an override is used as given.
	Surface map[surface.Role]lipgloss.TerminalColor
}

// Seeds are what surface tones are derived from.
func (p Palette) Seeds() surface.Seeds {
	return surface.Seeds{
		Background: p.Background, Foreground: p.Foreground,
		Bar: p.BarBG, BarText: p.BarFG, Muted: p.Muted, Subtle: p.Subtle,
		Edge: p.BorderInactive, EdgeActive: p.BorderActive, Accent: p.Accent,
	}
}

// Colours resolves every surface role for this palette, rounded to profile.
func (p Palette) Colours(profile termenv.Profile) surface.Colours {
	return surface.Derive(p.Seeds(), p.Surface, profile)
}

// ByName finds the palette called name.
func ByName(palettes []Palette, name string) (Palette, bool) {
	for _, p := range palettes {
		if p.Name == name {
			return p, true
		}
	}
	return Palette{}, false
}
