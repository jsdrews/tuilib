package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// Terminal builds a Theme from the user's terminal colorscheme — the actual
// foreground and background are queried via OSC 10/11 (termenv does the
// raw-mode handshake), and the rest of the Theme uses ANSI color indices
// 0–15, which the terminal renders using its configured 16-color palette.
// So a user with Catppuccin configured at the terminal level gets a
// Catppuccin-colored tuilib UI, with no further config.
//
// Falls back to Dark() or Light() (chosen by lipgloss's background-
// brightness guess) if stdin/stdout isn't a TTY or the terminal doesn't
// reply — typical failure modes: piped output, CI, Apple Terminal.app.
//
// Call BEFORE tea.NewProgram.Run(). termenv needs stdin to read the
// replies, and bubbletea takes ownership of stdin once Run begins.
func Terminal() Theme {
	if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
		return fallback()
	}

	out := termenv.NewOutput(os.Stdout)
	fg, fgOK := hex(out.ForegroundColor())
	bg, bgOK := hex(out.BackgroundColor())
	if !fgOK || !bgOK {
		return fallback()
	}

	// ANSI index colors ("0"–"15") are rendered by the terminal using its
	// configured palette — so these four picks inherit the user's blue,
	// green, red, magenta, etc.
	return Theme{
		Name:           "terminal",
		BarBG:          lipgloss.Color(bg),
		BarFG:          lipgloss.Color(fg),
		Current:        lipgloss.Color(fg),
		Muted:          lipgloss.Color("8"),  // bright black — "dim fg"
		Subtle:         lipgloss.Color("8"),
		KeyFG:          lipgloss.Color("12"), // bright blue
		BorderActive:   lipgloss.Color("4"),  // blue
		BorderInactive: lipgloss.Color("8"),
		InfoBG:         lipgloss.Color("2"),  // green
		InfoFG:         lipgloss.Color(bg),
		ErrorBG:        lipgloss.Color("1"),  // red
		ErrorFG:        lipgloss.Color(fg),
		Accent:         lipgloss.Color("5"),  // magenta
	}
}

// hex converts a termenv.Color to "#rrggbb". Returns ok=false for NoColor
// (queries that failed / terminals that don't respond).
func hex(c termenv.Color) (string, bool) {
	if _, isNone := c.(termenv.NoColor); isNone {
		return "", false
	}
	rgb := termenv.ConvertToRGB(c)
	h := rgb.Hex()
	if h == "" {
		return "", false
	}
	return h, true
}

// fallback picks Dark or Light based on lipgloss's background-brightness
// guess (which uses its own OSC 11 query with a short timeout — and has
// its own per-terminal fallbacks).
func fallback() Theme {
	if lipgloss.HasDarkBackground() {
		t := Dark()
		t.Name = "terminal (fallback → dark)"
		return t
	}
	t := Light()
	t.Name = "terminal (fallback → light)"
	return t
}
