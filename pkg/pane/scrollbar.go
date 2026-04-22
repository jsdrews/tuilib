package pane

import (
	"math"
	"strings"
)

const (
	ScrollbarWidth = 1

	scrollThumb = "█"
	scrollTrack = "░"
)

// Scrollbar renders a single-column vertical scrollbar of the given height.
// total is the total number of lines in the content, visible is how many fit
// in the viewport, and offset is the current scroll offset from the top.
// When content fits entirely, returns a blank column of the given height.
func Scrollbar(height, total, visible, offset int) string {
	if height <= 0 {
		return ""
	}
	if total <= visible {
		return strings.TrimRight(strings.Repeat(" \n", height), "\n")
	}
	ratio := float64(height) / float64(total)
	thumb := max(1, int(math.Round(float64(visible)*ratio)))
	off := max(0, min(height-thumb, int(math.Round(float64(offset)*ratio))))
	return strings.TrimRight(
		strings.Repeat(scrollTrack+"\n", off)+
			strings.Repeat(scrollThumb+"\n", thumb)+
			strings.Repeat(scrollTrack+"\n", max(0, height-off-thumb)),
		"\n",
	)
}
