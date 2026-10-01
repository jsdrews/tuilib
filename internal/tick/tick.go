// Package tick is the timer every tuilib component schedules through, and
// the seam tests use to stop waiting on real time.
//
// After replaces tea.Tick in library code for one reason beyond tests:
// tea.Tick starts its timer when the command is created, so running the
// same command twice blocks forever on the second run. After starts the
// wait when the command runs, so a command is safe to run again.
//
// Instant makes every After fire at once for the rest of a test — a
// debounce, a settle delay, a poll interval or a spinner frame no longer
// costs its duration. Code that feeds a timer's message straight back into
// Update and runs the next one should not use it: a spinner that re-arms
// instantly never stops.
package tick

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var instant atomic.Bool

// After returns a command that waits d, then returns fn(now).
func After(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if !instant.Load() && d > 0 {
			time.Sleep(d)
		}
		return fn(time.Now())
	}
}

// Cleaner is what Instant needs from a test: testing.T and testing.B are
// both one, without this package importing testing.
type Cleaner interface{ Cleanup(func()) }

// Instant makes every After return at once until the test ends.
func Instant(t Cleaner) {
	prev := instant.Swap(true)
	t.Cleanup(func() { instant.Store(prev) })
}
