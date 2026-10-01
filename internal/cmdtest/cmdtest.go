// Package cmdtest runs tea commands in tests without waiting on timers.
//
// A component's Update returns commands of two kinds: work that answers at
// once (a query, a focus request, a message) and timers that sleep first
// (a cursor blink, a spinner frame). Tests want the first and not the
// second. tuilib's own timers go through internal/tick, which tick.Instant
// makes return at once; timers from other libraries — bubbles' cursor
// blink — cannot be reached that way, so Run gives each command a moment
// and drops the ones still sleeping.
package cmdtest

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Grace is how long a command may take before Run treats it as a timer.
const Grace = 20 * time.Millisecond

// Run runs cmd, flattening batches, and returns the messages of the
// commands that answered within Grace. Each command runs at most once.
func Run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg, ok := quick(cmd)
	if !ok {
		return nil
	}
	if b, isBatch := msg.(tea.BatchMsg); isBatch {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, Run(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func quick(c tea.Cmd) (tea.Msg, bool) {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	select {
	case m := <-ch:
		return m, true
	case <-time.After(Grace):
		return nil, false
	}
}
