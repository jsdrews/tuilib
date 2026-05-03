// Package metrics renders small inline summaries — status badges, ratios,
// progress bars, sparklines — sized to fit inside list rows, table cells,
// or inspector values without breaking outer SGR state. Every helper
// returns a fixed-width string of ANSI-colored text using foreground-only
// escapes (via pkg/ansi.CellColor) so embedding inside a table row keeps
// the selected-row background intact.
//
// The package is rendering-only: callers own data, history, and any
// aggregation. For sparklines that need a moving window across refreshes,
// the caller maintains a fixed-size ring buffer and passes it in fresh
// each frame; metrics.Spark resamples to fit the requested width.
//
// Defaults use a small, opinionated severity palette (green/yellow/red/blue)
// tuned for the common monitoring case. Callers that need different colors
// should use the *Styled variants which take an explicit ANSI color index,
// or assemble their own from pkg/ansi.CellColor.
package metrics

import (
	"fmt"
	"math"
	"strings"

	"github.com/jsdrews/tuilib/pkg/ansi"
)

// Severity palette — ANSI 256-color indices. These match the convention
// used elsewhere in the library (see examples/data/poll, polltable).
const (
	colorRed    = 1
	colorGreen  = 2
	colorYellow = 3
	colorBlue   = 4
	colorGray   = 8
)

// Default glyph sets.
var (
	sparkGlyphs = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	barFilled   = "▰"
	barEmpty    = "▱"
)

// Badge renders an inline status-count summary like "12✓ 3⚠ 1✗", with
// each segment colored by severity. Zero buckets are omitted; an all-zero
// input renders as "". Visible width is variable (depends on which buckets
// are non-zero) — pad the cell yourself if you need a fixed column.
func Badge(ok, warn, down int) string {
	parts := make([]string, 0, 3)
	if ok > 0 {
		parts = append(parts, ansi.CellColor(colorGreen, fmt.Sprintf("%d✓", ok)))
	}
	if warn > 0 {
		parts = append(parts, ansi.CellColor(colorYellow, fmt.Sprintf("%d⚠", warn)))
	}
	if down > 0 {
		parts = append(parts, ansi.CellColor(colorRed, fmt.Sprintf("%d✗", down)))
	}
	return strings.Join(parts, " ")
}

// Ratio renders "done/total" colored by completion: green when full,
// yellow when partial, red when zero, gray when total is zero. Use for
// replica counts, job-completion counters, or any "N of M" indicator
// where the threshold is "all-or-nothing." Visible width is the natural
// width of the formatted string.
func Ratio(done, total int) string {
	s := fmt.Sprintf("%d/%d", done, total)
	color := colorGray
	switch {
	case total == 0:
		color = colorGray
	case done >= total:
		color = colorGreen
	case done == 0:
		color = colorRed
	default:
		color = colorYellow
	}
	return ansi.CellColor(color, s)
}

// Bar renders a fixed-width progress bar. The fill ratio is value/max
// coerced to [0, 1]; width is the total visible cell width (must be > 0).
// Color is inferred from severity: green when full, yellow when partial,
// red when empty. For a non-severity bar (e.g. CPU usage that should
// always be blue), use BarStyled.
func Bar(value, max float64, width int) string {
	color := colorGray
	switch r := safeRatio(value, max); {
	case r >= 0.999:
		color = colorGreen
	case r <= 0:
		color = colorRed
	default:
		color = colorYellow
	}
	return BarStyled(value, max, width, color)
}

// BarStyled is Bar with an explicit ANSI color index instead of severity
// inference. Useful for non-severity progress (CPU, memory, fill level)
// where the threshold doesn't carry "good/bad" meaning.
func BarStyled(value, max float64, width int, color int) string {
	if width <= 0 {
		return ""
	}
	ratio := safeRatio(value, max)
	filled := int(math.Round(ratio * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	s := strings.Repeat(barFilled, filled) + strings.Repeat(barEmpty, width-filled)
	return ansi.CellColor(color, s)
}

// Spark renders a fixed-width sparkline using 8-step block glyphs
// (▁▂▃▄▅▆▇█), normalized to the local series min/max. Resamples by
// averaging buckets when len(values) > width and by nearest-index sampling
// when len(values) < width. Returns "" when values is empty or width <= 0.
// Color is blue (informational); use SparkStyled for an explicit color.
func Spark(values []float64, width int) string {
	return SparkStyled(values, width, colorBlue)
}

// SparkStyled is Spark with an explicit ANSI color index.
func SparkStyled(values []float64, width int, color int) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}
	samples := resample(values, width)
	min, max := samples[0], samples[0]
	for _, v := range samples {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	var b strings.Builder
	b.Grow(width * 4)
	for _, v := range samples {
		var idx int
		if span == 0 {
			idx = 0
		} else {
			t := (v - min) / span
			idx = int(math.Round(t * float64(len(sparkGlyphs)-1)))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(sparkGlyphs) {
				idx = len(sparkGlyphs) - 1
			}
		}
		b.WriteRune(sparkGlyphs[idx])
	}
	return ansi.CellColor(color, b.String())
}

func safeRatio(value, max float64) float64 {
	if max <= 0 {
		return 0
	}
	r := value / max
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

func resample(values []float64, width int) []float64 {
	out := make([]float64, width)
	n := len(values)
	if n >= width {
		for i := 0; i < width; i++ {
			start := i * n / width
			end := (i + 1) * n / width
			if end <= start {
				end = start + 1
			}
			if end > n {
				end = n
			}
			sum := 0.0
			for _, v := range values[start:end] {
				sum += v
			}
			out[i] = sum / float64(end-start)
		}
		return out
	}
	for i := 0; i < width; i++ {
		idx := i * n / width
		out[i] = values[idx]
	}
	return out
}
