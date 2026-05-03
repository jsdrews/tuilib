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
