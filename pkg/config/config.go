// Package config carries the user-supplied YAML config that any tuilib
// component can consult. Today only the theme name lives here; as other
// packages grow user-tunable knobs (logview defaults, runner clear-screen,
// custom keymaps, custom palettes), their fields land on Config too.
//
// The library never writes the file and never creates the directory —
// config is opt-in. A missing file is the steady state for users who
// haven't set anything; Load returns (Config{}, nil) in that case. Parse
// errors on a malformed file are surfaced so silent corruption doesn't
// mask itself.
//
// Cross-package usage:
//
//	cfg, _ := config.Load()
//	themes := theme.Resolve(theme.All(), "TUILIB_THEME") // already calls Load
//	// later, hypothetically:
//	lvOpts := logview.FromConfig(cfg)                    // reads cfg.Logview
//
// Each consumer pulls just the fields it cares about; this package owns
// only the file shape and I/O.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the unmarshaled form of $XDG_CONFIG_HOME/tuilib/config.yaml.
// Unknown YAML keys are tolerated by yaml.Unmarshal so adding new fields is
// backwards-compatible.
type Config struct {
	// Theme names a theme to use as the initial palette. Matched against
	// theme.Theme.Name by theme.Resolve. Empty string disables the lookup.
	Theme string `yaml:"theme"`
}

// Path returns the path tuilib will read from. Honors $XDG_CONFIG_HOME,
// falling back to $HOME/.config/tuilib/config.yaml. The returned path may
// not exist — callers are expected to stat-and-skip.
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tuilib", "config.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tuilib", "config.yaml")
}

// Load reads Path(). When the file doesn't exist, returns (Config{}, nil) —
// opt-in is the design, so file-not-found is not an error condition.
//
// Returns an error only on permission denied, malformed YAML, or other I/O
// problems that aren't "file not found".
func Load() (Config, error) {
	return LoadFrom(Path())
}

// LoadFrom reads a specific path. Same semantics as Load — file-not-found
// returns (Config{}, nil); other errors propagate. Useful for per-app
// override paths or tests.
func LoadFrom(path string) (Config, error) {
	if path == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}
