package theme

import "github.com/charmbracelet/lipgloss"

// This file holds hand-curated themes. Each constructor maps a published
// palette's canonical hex values onto our semantic Theme slots. When a
// palette's upstream role doesn't match ours exactly (e.g. base16 has no
// "key hint" color), the constructor makes a judgment call — tweak the
// field if it reads wrong on your terminal.

// CatppuccinMocha — catppuccin.com, dark "mocha" flavor.
func CatppuccinMocha() Theme {
	return Theme{
		Name:           "catppuccin-mocha",
		BarBG:          lipgloss.Color("#313244"), // surface0
		BarFG:          lipgloss.Color("#cdd6f4"), // text
		Current:        lipgloss.Color("#cdd6f4"),
		Muted:          lipgloss.Color("#a6adc8"), // subtext0
		Subtle:         lipgloss.Color("#6c7086"), // overlay0
		KeyFG:          lipgloss.Color("#89dceb"), // sky
		BorderActive:   lipgloss.Color("#cba6f7"), // mauve
		BorderInactive: lipgloss.Color("#585b70"), // surface2
		InfoBG:         lipgloss.Color("#a6e3a1"), // green
		InfoFG:         lipgloss.Color("#1e1e2e"), // base
		ErrorBG:        lipgloss.Color("#f38ba8"), // red
		ErrorFG:        lipgloss.Color("#1e1e2e"),
		Accent:         lipgloss.Color("#f5c2e7"), // pink
	}
}

// CatppuccinLatte — catppuccin.com, light "latte" flavor.
func CatppuccinLatte() Theme {
	return Theme{
		Name:           "catppuccin-latte",
		BarBG:          lipgloss.Color("#ccd0da"), // surface0
		BarFG:          lipgloss.Color("#4c4f69"), // text
		Current:        lipgloss.Color("#4c4f69"),
		Muted:          lipgloss.Color("#6c6f85"), // subtext0
		Subtle:         lipgloss.Color("#9ca0b0"), // overlay0
		KeyFG:          lipgloss.Color("#1e66f5"), // blue
		BorderActive:   lipgloss.Color("#8839ef"), // mauve
		BorderInactive: lipgloss.Color("#acb0be"), // surface2
		InfoBG:         lipgloss.Color("#40a02b"), // green
		InfoFG:         lipgloss.Color("#eff1f5"), // base
		ErrorBG:        lipgloss.Color("#d20f39"), // red
		ErrorFG:        lipgloss.Color("#eff1f5"),
		Accent:         lipgloss.Color("#ea76cb"), // pink
	}
}

// TokyoNight — enkia/tokyo-night, "night" variant.
func TokyoNight() Theme {
	return Theme{
		Name:           "tokyo-night",
		BarBG:          lipgloss.Color("#292e42"), // bg_highlight
		BarFG:          lipgloss.Color("#c0caf5"), // fg
		Current:        lipgloss.Color("#c0caf5"),
		Muted:          lipgloss.Color("#737aa2"), // dark5
		Subtle:         lipgloss.Color("#414868"), // terminal_black
		KeyFG:          lipgloss.Color("#7dcfff"), // cyan
		BorderActive:   lipgloss.Color("#7aa2f7"), // blue
		BorderInactive: lipgloss.Color("#414868"),
		InfoBG:         lipgloss.Color("#9ece6a"), // green
		InfoFG:         lipgloss.Color("#1a1b26"), // bg
		ErrorBG:        lipgloss.Color("#f7768e"), // red
		ErrorFG:        lipgloss.Color("#1a1b26"),
		Accent:         lipgloss.Color("#bb9af7"), // magenta
	}
}

// RosePine — rosepinetheme.com, main "moon" is excluded; this is the dark
// default.
func RosePine() Theme {
	return Theme{
		Name:           "rose-pine",
		BarBG:          lipgloss.Color("#1f1d2e"), // surface
		BarFG:          lipgloss.Color("#e0def4"), // text
		Current:        lipgloss.Color("#e0def4"),
		Muted:          lipgloss.Color("#908caa"), // subtle
		Subtle:         lipgloss.Color("#6e6a86"), // muted
		KeyFG:          lipgloss.Color("#9ccfd8"), // foam
		BorderActive:   lipgloss.Color("#c4a7e7"), // iris
		BorderInactive: lipgloss.Color("#403d52"), // highlight_med
		InfoBG:         lipgloss.Color("#31748f"), // pine
		InfoFG:         lipgloss.Color("#e0def4"),
		ErrorBG:        lipgloss.Color("#eb6f92"), // love
		ErrorFG:        lipgloss.Color("#191724"), // base
		Accent:         lipgloss.Color("#ebbcba"), // rose
	}
}

// RosePineDawn — rosepinetheme.com, light variant.
func RosePineDawn() Theme {
	return Theme{
		Name:           "rose-pine-dawn",
		BarBG:          lipgloss.Color("#fffaf3"), // surface
		BarFG:          lipgloss.Color("#575279"), // text
		Current:        lipgloss.Color("#575279"),
		Muted:          lipgloss.Color("#797593"), // subtle
		Subtle:         lipgloss.Color("#9893a5"), // muted
		KeyFG:          lipgloss.Color("#56949f"), // foam
		BorderActive:   lipgloss.Color("#907aa9"), // iris
		BorderInactive: lipgloss.Color("#dfdad9"), // highlight_med
		InfoBG:         lipgloss.Color("#286983"), // pine
		InfoFG:         lipgloss.Color("#faf4ed"), // base
		ErrorBG:        lipgloss.Color("#b4637a"), // love
		ErrorFG:        lipgloss.Color("#faf4ed"),
		Accent:         lipgloss.Color("#d7827e"), // rose
	}
}

// OneDark — Atom's classic "One Dark" palette.
func OneDark() Theme {
	return Theme{
		Name:           "one-dark",
		BarBG:          lipgloss.Color("#21252b"), // bar bg (common in atom)
		BarFG:          lipgloss.Color("#abb2bf"), // fg
		Current:        lipgloss.Color("#ffffff"),
		Muted:          lipgloss.Color("#5c6370"), // comment grey
		Subtle:         lipgloss.Color("#3e4451"),
		KeyFG:          lipgloss.Color("#61afef"), // blue
		BorderActive:   lipgloss.Color("#61afef"),
		BorderInactive: lipgloss.Color("#3e4451"),
		InfoBG:         lipgloss.Color("#98c379"), // green
		InfoFG:         lipgloss.Color("#282c34"), // bg
		ErrorBG:        lipgloss.Color("#e06c75"), // red
		ErrorFG:        lipgloss.Color("#282c34"),
		Accent:         lipgloss.Color("#c678dd"), // purple
	}
}

// Monokai — the classic Sublime Text / TextMate palette.
func Monokai() Theme {
	return Theme{
		Name:           "monokai",
		BarBG:          lipgloss.Color("#3e3d32"),
		BarFG:          lipgloss.Color("#f8f8f2"),
		Current:        lipgloss.Color("#f8f8f2"),
		Muted:          lipgloss.Color("#75715e"), // comment
		Subtle:         lipgloss.Color("#49483e"),
		KeyFG:          lipgloss.Color("#66d9ef"), // blue
		BorderActive:   lipgloss.Color("#f92672"), // pink
		BorderInactive: lipgloss.Color("#49483e"),
		InfoBG:         lipgloss.Color("#a6e22e"), // green
		InfoFG:         lipgloss.Color("#272822"),
		ErrorBG:        lipgloss.Color("#f92672"), // pink/red
		ErrorFG:        lipgloss.Color("#272822"),
		Accent:         lipgloss.Color("#fd971f"), // orange
	}
}

// EverforestDark — sainnhe/everforest, medium-contrast dark.
func EverforestDark() Theme {
	return Theme{
		Name:           "everforest-dark",
		BarBG:          lipgloss.Color("#343f44"), // bg1
		BarFG:          lipgloss.Color("#d3c6aa"), // fg
		Current:        lipgloss.Color("#d3c6aa"),
		Muted:          lipgloss.Color("#9da9a0"), // grey2
		Subtle:         lipgloss.Color("#7a8478"), // grey0
		KeyFG:          lipgloss.Color("#83c092"), // aqua
		BorderActive:   lipgloss.Color("#a7c080"), // green
		BorderInactive: lipgloss.Color("#475258"), // bg3
		InfoBG:         lipgloss.Color("#a7c080"),
		InfoFG:         lipgloss.Color("#2d353b"), // bg0
		ErrorBG:        lipgloss.Color("#e67e80"), // red
		ErrorFG:        lipgloss.Color("#2d353b"),
		Accent:         lipgloss.Color("#d699b6"), // purple
	}
}
