// Package ansi holds small, dependency-free ANSI helpers for the corners
// of the library where lipgloss's full \x1b[0m reset is the wrong tool —
// most notably cells inside a styled row (e.g. pkg/table's selected row)
// where an inner reset would clobber the row's background.
//
// Components that own their own pane (pkg/list, pkg/tree, pkg/logview, …)
// render with their own per-row styling and you can keep using lipgloss
// inside them. Reach for this package only when downstream rendering will
// wrap your content in its own SGR with a background you want preserved.
package ansi

import "fmt"

// CellColor wraps text in a foreground SGR with a foreground-only reset
// (\x1b[39m). The 39 reset preserves any outer background, making it safe
// to nest inside a styled row (e.g. pkg/table's selected row) where the
// row's Background would otherwise be clobbered by lipgloss's full
// \x1b[0m reset.
//
// n is a 256-color palette index (0–255):
//
//	0–7    standard 8-color palette       (emitted as \x1b[3Nm — 4 bytes open)
//	8–15   bright variants                (emitted as \x1b[9Nm — 4 bytes open)
//	16–255 256-color cube + grayscale     (emitted as \x1b[38;5;Nm — 8–10 bytes open)
//
// CellColor picks the shortest valid form to keep escape overhead small.
func CellColor(n int, text string) string {
	switch {
	case n < 0 || n > 255:
		return text
	case n < 8:
		return fmt.Sprintf("\x1b[%dm%s\x1b[39m", 30+n, text)
	case n < 16:
		return fmt.Sprintf("\x1b[%dm%s\x1b[39m", 90+(n-8), text)
	default:
		return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[39m", n, text)
	}
}

// Hyperlink wraps text in an OSC 8 hyperlink escape so terminals that
// support it (alacritty, tmux, kitty, wezterm, iTerm2) launch url on
// click. The wrapper is invisible to terminal width calculation and
// x/ansi.Cut preserves both the open and close sequences across
// truncation, so a long URL stays attached to its cell even when the
// visible label is cut to fit a narrow column.
//
// Use this in pkg/table cells when you want shift-click / cmd-click to
// open the full URL regardless of column width. Bare URL text would be
// truncated mid-string and break the launched URL; the OSC 8 envelope
// decouples display text from the underlying link.
func Hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// ExtractHyperlink pulls the url and visible text out of a string
// wrapped by Hyperlink. ok is false when s isn't an OSC 8 envelope, so
// callers can treat the cell as a bare string in that case. Useful in a
// screen's Update when "press o to open the focused row's URL" needs
// the original target — shift-click handles the common case at the
// terminal level, but programmatic open paths need the URL back.
func ExtractHyperlink(s string) (url, text string, ok bool) {
	const open = "\x1b]8;;"
	const sep = "\x1b\\"
	const close = "\x1b]8;;\x1b\\"
	if len(s) < len(open)+len(sep)+len(close) || s[:len(open)] != open {
		return "", "", false
	}
	rest := s[len(open):]
	i := indexOf(rest, sep)
	if i < 0 {
		return "", "", false
	}
	url = rest[:i]
	rest = rest[i+len(sep):]
	if len(rest) < len(close) || rest[len(rest)-len(close):] != close {
		return "", "", false
	}
	text = rest[:len(rest)-len(close)]
	return url, text, true
}

// indexOf is a small substring search local to this file so the package
// stays dependency-free (no "strings" import for one call).
func indexOf(haystack, needle string) int {
	n, m := len(haystack), len(needle)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if haystack[i:i+m] == needle {
			return i
		}
	}
	return -1
}
