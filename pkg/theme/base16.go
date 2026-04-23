package theme

import "github.com/charmbracelet/lipgloss"

// Base16 is Chris Kempson's 16-color scheme format (github.com/chriskempson/
// base16). Each slot has a well-defined semantic role, which is what makes it
// clean to map onto our Theme struct.
//
// Slot reference (abridged):
//   base00  default background
//   base01  lighter background — status bars, line numbers
//   base02  selection background
//   base03  comments, invisibles, line highlighting
//   base04  dark foreground (status bars)
//   base05  default foreground
//   base06  light foreground
//   base07  lightest background
//   base08  red       (variables, errors)
//   base09  orange    (integers, constants)
//   base0A  yellow    (classes, search hits)
//   base0B  green     (strings, diff inserted)
//   base0C  cyan      (support, regex, escapes)
//   base0D  blue      (functions, methods)
//   base0E  magenta   (keywords, storage)
//   base0F  brown     (deprecated, embedded tags)
//
// Hex values are stored WITHOUT the leading "#" — the converter adds it.
type Base16 struct {
	Base00, Base01, Base02, Base03, Base04, Base05, Base06, Base07,
	Base08, Base09, Base0A, Base0B, Base0C, Base0D, Base0E, Base0F string
}

// FromBase16 maps a Base16 palette onto a Theme. The mapping is opinionated:
//
//   BarBG   base01   lighter bg — the canonical status-bar surface
//   BarFG   base05   default fg
//   Current base06   light fg for emphasis (bolded by the crumb style)
//   Muted   base04   dark fg (statusbar-muted)
//   Subtle  base03   comment grey
//   KeyFG   base0D   blue (functions/links/URLs — reads as "interactive")
//   Info    base0B (green) on base00
//   Error   base08 (red)   on base07
//   Accent  base0E   keyword color — body-content highlight
//
// If the resulting theme reads wrong for a specific palette (some base16
// schemes have an unusually bright base04 or near-identical base01/base00),
// derive a one-off Theme by hand instead of using this converter.
func FromBase16(name string, p Base16) Theme {
	hex := func(s string) lipgloss.Color { return lipgloss.Color("#" + s) }
	return Theme{
		Name:           name,
		BarBG:          hex(p.Base01),
		BarFG:          hex(p.Base05),
		Current:        hex(p.Base06),
		Muted:          hex(p.Base04),
		Subtle:         hex(p.Base03),
		KeyFG:          hex(p.Base0D),
		BorderActive:   hex(p.Base0D),
		BorderInactive: hex(p.Base03),
		InfoBG:         hex(p.Base0B),
		InfoFG:         hex(p.Base00),
		ErrorBG:        hex(p.Base08),
		ErrorFG:        hex(p.Base07),
		Accent:         hex(p.Base0E),
	}
}

// ---- A handful of base16 schemes (canonical hex values from the base16
// reference schemes). Extend by dropping more Base16{} literals into this
// file.

// Base16Ocean — Chris Kempson's "ocean" scheme, blue-grey neutral.
func Base16Ocean() Theme {
	return FromBase16("base16-ocean", Base16{
		Base00: "2b303b", Base01: "343d46", Base02: "4f5b66", Base03: "65737e",
		Base04: "a7adba", Base05: "c0c5ce", Base06: "dfe1e8", Base07: "eff1f5",
		Base08: "bf616a", Base09: "d08770", Base0A: "ebcb8b", Base0B: "a3be8c",
		Base0C: "96b5b4", Base0D: "8fa1b3", Base0E: "b48ead", Base0F: "ab7967",
	})
}

// Base16Eighties — Chris Kempson, bright retro palette.
func Base16Eighties() Theme {
	return FromBase16("base16-eighties", Base16{
		Base00: "2d2d2d", Base01: "393939", Base02: "515151", Base03: "747369",
		Base04: "a09f93", Base05: "d3d0c8", Base06: "e8e6df", Base07: "f2f0ec",
		Base08: "f2777a", Base09: "f99157", Base0A: "ffcc66", Base0B: "99cc99",
		Base0C: "66cccc", Base0D: "6699cc", Base0E: "cc99cc", Base0F: "d27b53",
	})
}

// Base16Railscasts — warm muted palette derived from the classic TextMate theme.
func Base16Railscasts() Theme {
	return FromBase16("base16-railscasts", Base16{
		Base00: "2b2b2b", Base01: "272935", Base02: "3a4055", Base03: "5a647e",
		Base04: "d4cfc9", Base05: "e6e1dc", Base06: "f4f1ed", Base07: "f9f7f3",
		Base08: "da4939", Base09: "cc7833", Base0A: "ffc66d", Base0B: "a5c261",
		Base0C: "519f50", Base0D: "6d9cbe", Base0E: "b6b3eb", Base0F: "bc9458",
	})
}

// Base16TomorrowNight — Chris Kempson's "tomorrow-night". Balanced dark.
func Base16TomorrowNight() Theme {
	return FromBase16("base16-tomorrow-night", Base16{
		Base00: "1d1f21", Base01: "282a2e", Base02: "373b41", Base03: "969896",
		Base04: "b4b7b4", Base05: "c5c8c6", Base06: "e0e0e0", Base07: "ffffff",
		Base08: "cc6666", Base09: "de935f", Base0A: "f0c674", Base0B: "b5bd68",
		Base0C: "8abeb7", Base0D: "81a2be", Base0E: "b294bb", Base0F: "a3685a",
	})
}
