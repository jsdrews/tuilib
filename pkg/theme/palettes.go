package theme

import "github.com/jsdrews/tuilib/pkg/palette"

// The built-in palettes live in pkg/palette. These wrappers keep the
// theme.Dark()-style constructors working until apps move to choosing a
// palette and a surface separately (docs/surfaces.md, build step 7).

// FromPalette builds a Theme drawn in p's colours, with today's chrome.
func FromPalette(p palette.Palette) Theme {
	return Theme{
		Name:           p.Name,
		BarBG:          p.BarBG,
		BarFG:          p.BarFG,
		Current:        p.Current,
		Muted:          p.Muted,
		Subtle:         p.Subtle,
		KeyFG:          p.KeyFG,
		BorderActive:   p.BorderActive,
		BorderInactive: p.BorderInactive,
		InfoBG:         p.InfoBG,
		InfoFG:         p.InfoFG,
		ErrorBG:        p.ErrorBG,
		ErrorFG:        p.ErrorFG,
		Accent:         p.Accent,
	}
}

// Base16 is palette.Base16.
type Base16 = palette.Base16

// FromBase16 is palette.FromBase16 as a Theme.
func FromBase16(name string, p Base16) Theme { return FromPalette(palette.FromBase16(name, p)) }

// All returns every built-in palette as a Theme, in display order.
func All() []Theme {
	var ts []Theme
	for _, p := range palette.All() {
		ts = append(ts, FromPalette(p))
	}
	return ts
}

// Dark is palette.Dark as a Theme.
func Dark() Theme { return FromPalette(palette.Dark()) }

// Accent is palette.Accent as a Theme.
func Accent() Theme { return FromPalette(palette.Accent()) }

// Light is palette.Light as a Theme.
func Light() Theme { return FromPalette(palette.Light()) }

// Solarized is palette.Solarized as a Theme.
func Solarized() Theme { return FromPalette(palette.Solarized()) }

// Nord is palette.Nord as a Theme.
func Nord() Theme { return FromPalette(palette.Nord()) }

// Dracula is palette.Dracula as a Theme.
func Dracula() Theme { return FromPalette(palette.Dracula()) }

// Gruvbox is palette.Gruvbox as a Theme.
func Gruvbox() Theme { return FromPalette(palette.Gruvbox()) }

// CatppuccinMocha is palette.CatppuccinMocha as a Theme.
func CatppuccinMocha() Theme { return FromPalette(palette.CatppuccinMocha()) }

// CatppuccinLatte is palette.CatppuccinLatte as a Theme.
func CatppuccinLatte() Theme { return FromPalette(palette.CatppuccinLatte()) }

// TokyoNight is palette.TokyoNight as a Theme.
func TokyoNight() Theme { return FromPalette(palette.TokyoNight()) }

// RosePine is palette.RosePine as a Theme.
func RosePine() Theme { return FromPalette(palette.RosePine()) }

// RosePineDawn is palette.RosePineDawn as a Theme.
func RosePineDawn() Theme { return FromPalette(palette.RosePineDawn()) }

// OneDark is palette.OneDark as a Theme.
func OneDark() Theme { return FromPalette(palette.OneDark()) }

// Monokai is palette.Monokai as a Theme.
func Monokai() Theme { return FromPalette(palette.Monokai()) }

// EverforestDark is palette.EverforestDark as a Theme.
func EverforestDark() Theme { return FromPalette(palette.EverforestDark()) }

// Base16Ocean is palette.Base16Ocean as a Theme.
func Base16Ocean() Theme { return FromPalette(palette.Base16Ocean()) }

// Base16Eighties is palette.Base16Eighties as a Theme.
func Base16Eighties() Theme { return FromPalette(palette.Base16Eighties()) }

// Base16Railscasts is palette.Base16Railscasts as a Theme.
func Base16Railscasts() Theme { return FromPalette(palette.Base16Railscasts()) }

// Base16TomorrowNight is palette.Base16TomorrowNight as a Theme.
func Base16TomorrowNight() Theme { return FromPalette(palette.Base16TomorrowNight()) }

// Terminal is palette.Terminal as a Theme.
func Terminal() Theme { return FromPalette(palette.Terminal()) }
