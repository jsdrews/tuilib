// Package ansi holds small, dependency-free ANSI helpers for the corners
// of the library where lipgloss's full \x1b[0m reset is the wrong tool —
// most notably bubbles/table cells, where an inner reset clobbers the
// selected-row background.
//
// Components that own their own pane (pkg/list, pkg/tree, pkg/logview, …)
// render ANSI-aware and you should keep using lipgloss inside them. Reach
// for this package only when something downstream is going to nest your
// content inside its own SGR wrapper.
package ansi

import "fmt"

// CellColor wraps text in a foreground SGR with a foreground-only reset
// (\x1b[39m). The 39 reset preserves any outer background, making it safe
// to nest inside a bubbles/table cell where the selected-row Background
// would otherwise be clobbered by lipgloss's full \x1b[0m reset.
//
// n is a 256-color palette index (0–255):
//
//	0–7    standard 8-color palette       (emitted as \x1b[3Nm — 4 bytes open)
//	8–15   bright variants                (emitted as \x1b[9Nm — 4 bytes open)
//	16–255 256-color cube + grayscale     (emitted as \x1b[38;5;Nm — 8–10 bytes open)
//
// CellColor picks the shortest valid form so cells stay under bubbles/table's
// column-width budget for as long as possible.
//
// Important: bubbles/table's column truncation is not ANSI-aware (upstream
// uses runewidth.Truncate, which counts escape bytes as visible width). For
// the basic 16-color forms above, the escape budget is 8 visible-equivalent
// bytes (4 open + 4 close) — so a 16-wide column has only 8 chars left for
// the actual text. Size columns at least visible-text-width + 8 (for n<16)
// or + ~14 (for 256-color) to keep the closing reset intact and avoid color
// bleed into adjacent cells.
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
