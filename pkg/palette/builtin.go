package palette

import "github.com/charmbracelet/lipgloss"

// Dark is the default palette used across the examples — matches pug's look.
func Dark() Palette {
	return Palette{
		Name:           "dark",
		Background:     lipgloss.Color("234"),
		Foreground:     lipgloss.Color("252"),
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
func Accent() Palette {
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
func Light() Palette {
	return Palette{
		Name:           "light",
		Background:     lipgloss.Color("255"),
		Foreground:     lipgloss.Color("236"),
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
func Solarized() Palette {
	return Palette{
		Name:           "solarized",
		Background:     lipgloss.Color("#002b36"),
		Foreground:     lipgloss.Color("#839496"),
		BarBG:          lipgloss.Color("234"), // base03
		BarFG:          lipgloss.Color("245"), // base1
		Current:        lipgloss.Color("250"), // base2
		Muted:          lipgloss.Color("244"), // base0
		Subtle:         lipgloss.Color("240"), // base01
		KeyFG:          lipgloss.Color("37"),  // cyan
		BorderActive:   lipgloss.Color("33"),  // blue
		BorderInactive: lipgloss.Color("240"),
		InfoBG:         lipgloss.Color("64"), // green
		InfoFG:         lipgloss.Color("234"),
		ErrorBG:        lipgloss.Color("160"), // red
		ErrorFG:        lipgloss.Color("255"),
		Accent:         lipgloss.Color("166"), // orange
	}
}

// Nord is the Nord palette (arcticicestudio/nord) mapped to 256-color.
// Cool frost tones; reads calmer than Dark.
func Nord() Palette {
	return Palette{
		Name:           "nord",
		Background:     lipgloss.Color("#2e3440"),
		Foreground:     lipgloss.Color("#d8dee9"),
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
func Dracula() Palette {
	return Palette{
		Name:           "dracula",
		Background:     lipgloss.Color("#282a36"),
		Foreground:     lipgloss.Color("#f8f8f2"),
		BarBG:          lipgloss.Color("235"),
		BarFG:          lipgloss.Color("252"),
		Current:        lipgloss.Color("255"),
		Muted:          lipgloss.Color("61"), // comment
		Subtle:         lipgloss.Color("238"),
		KeyFG:          lipgloss.Color("141"), // purple
		BorderActive:   lipgloss.Color("212"), // pink
		BorderInactive: lipgloss.Color("238"),
		InfoBG:         lipgloss.Color("84"), // green
		InfoFG:         lipgloss.Color("235"),
		ErrorBG:        lipgloss.Color("203"), // red
		ErrorFG:        lipgloss.Color("255"),
		Accent:         lipgloss.Color("117"), // cyan — on-brand against pink/purple chrome
	}
}

// Gruvbox maps Pavel Pertsev's gruvbox-dark palette — warm earthy tones.
func Gruvbox() Palette {
	return Palette{
		Name:           "gruvbox",
		Background:     lipgloss.Color("#282828"),
		Foreground:     lipgloss.Color("#ebdbb2"),
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

// All returns every built-in palette, in display order. Handy for pickers
// and examples/shell/themes.
func All() []Palette {
	return []Palette{
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
