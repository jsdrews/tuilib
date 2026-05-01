// Theme-name resolution: env var → user config → first-of-themes fallback.
// File loading lives in pkg/config; this file is just the theme-aware
// glue around it.

package theme

import (
	"os"

	"github.com/jsdrews/tuilib/pkg/config"
)

// ByName looks up a theme by its Name field. Returns false when no theme in
// themes matches — caller decides what fallback means (typically: leave the
// existing default in place).
func ByName(themes []Theme, name string) (Theme, bool) {
	for _, t := range themes {
		if t.Name == name {
			return t, true
		}
	}
	return Theme{}, false
}

// Resolve picks an initial theme from themes using, in order:
//
//  1. The named environment variable (envVar) if set.
//  2. The Theme field in $XDG_CONFIG_HOME/tuilib/config.yaml (via
//     config.Load).
//  3. themes[0] (no change).
//
// Returns themes reordered so the picked theme is first, satisfying
// app.Options.Themes[0] = initial. Lookup misses (unknown theme name,
// unreadable config) fall through silently — config and env vars are hints,
// not errors. To surface a malformed config file, call config.Load
// directly.
//
// Pass envVar = "" to disable env-var resolution.
func Resolve(themes []Theme, envVar string) []Theme {
	if len(themes) == 0 {
		return themes
	}

	pick := ""
	if envVar != "" {
		pick = os.Getenv(envVar)
	}
	if pick == "" {
		if cfg, err := config.Load(); err == nil {
			pick = cfg.Theme
		}
	}
	if pick == "" {
		return themes
	}

	for i, t := range themes {
		if t.Name == pick {
			if i == 0 {
				return themes
			}
			out := make([]Theme, 0, len(themes))
			out = append(out, t)
			out = append(out, themes[:i]...)
			out = append(out, themes[i+1:]...)
			return out
		}
	}
	return themes
}
