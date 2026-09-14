// Package integration drives whole screens against demoapi.
//
// The distinction from internal/componenttest is the one docs/demoapi.md
// decision 14 draws: if an assertion can be written by calling a setter, it
// belongs there — synchronous, no I/O, no clock. What lives here needs a
// server, because what it asserts is about *arrival order*, and an ordering a
// test constructs by hand is an ordering the test invented.
//
// Two pieces of the library exist for orderings only a real client produces:
//
//   - source.Deliver carries a generation so a slow reply to an abandoned
//     filter cannot paint itself under the current one.
//   - pkg/activity prefers a local entry over a derived one so a poll already
//     in flight when the user acted cannot wipe their spinner.
//
// Both were previously asserted by handing a component a message sequence the
// test wrote. These drive real requests through a real handler with real
// per-request latency, and let the replies land in whatever order they land.
package integration

import (
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jsdrews/tuilib/pkg/geom"
	"github.com/jsdrews/tuilib/pkg/screen"
)

const (
	scrW = 90
	scrH = 24
)

// TestMain forces a colour profile. Without a TTY lipgloss strips every style,
// so any assertion about styled output would pass for the wrong reason.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// harness is a small bubbletea event loop: it runs commands on their own
// goroutines and feeds their messages back in the order they actually
// complete.
//
// The concurrency is the point. A loop that ran each command to completion
// before starting the next would serialise every reply and could never produce
// the interleavings these tests exist for.
type harness struct {
	t    *testing.T
	msgs chan tea.Msg
	wg   sync.WaitGroup

	// step applies a message and hands back its command; draw renders a frame.
	// Closures rather than a concrete type because the same loop drives a bare
	// screen.Screen and a whole app.Model, and the concurrency below is the
	// part worth having exactly once.
	step func(tea.Msg) tea.Cmd
	draw func() string
}

func newHarness(t *testing.T, s screen.Screen) *harness {
	t.Helper()
	cur := s
	h := &harness{
		t:    t,
		msgs: make(chan tea.Msg, 256),
		step: func(msg tea.Msg) tea.Cmd {
			var cmd tea.Cmd
			cur, cmd = cur.Update(msg)
			return cmd
		},
		draw: func() string { return cur.Layout().Render(geom.New(0, 0, scrW, scrH)) },
	}
	h.render()
	h.exec(s.Init())
	return h
}

// newAppHarness drives a whole app.Model — the shell, its overlays, its output
// console — rather than a screen in isolation.
func newAppHarness(t *testing.T, m tea.Model) *harness {
	t.Helper()
	cur := m
	h := &harness{
		t:    t,
		msgs: make(chan tea.Msg, 256),
		step: func(msg tea.Msg) tea.Cmd {
			var cmd tea.Cmd
			cur, cmd = cur.Update(msg)
			return cmd
		},
		draw: func() string { return cur.View() },
	}
	h.exec(m.Init())
	h.send(tea.WindowSizeMsg{Width: scrW, Height: scrH})
	return h
}

// key sends a keypress the way a terminal would.
func (h *harness) key(s string) {
	h.t.Helper()
	if len(s) == 1 {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
		return
	}
	switch s {
	case "enter":
		h.send(tea.KeyMsg{Type: tea.KeyEnter})
	case "esc":
		h.send(tea.KeyMsg{Type: tea.KeyEsc})
	default:
		h.t.Fatalf("unhandled key %q", s)
	}
}

// exec runs a command off the main goroutine, flattening batches, and posts
// whatever it produces.
func (h *harness) exec(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		msg := cmd()
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				h.exec(c)
			}
			return
		}
		h.msgs <- msg
	}()
}

// send delivers one message and starts whatever it asks for. Rendering after
// every update matters: layout.Sized is what hands the table a rect, and a
// table with no rect reports no viewport, so nothing would ever be fetched.
func (h *harness) send(msg tea.Msg) {
	cmd := h.step(msg)
	h.render()
	h.exec(cmd)
}

func (h *harness) render() string { return h.draw() }

// pumpFor delivers messages for a fixed window. A window rather than
// "until quiet" because these tests are specifically waiting out a reply that
// is deliberately late, and quiet is exactly what precedes it.
func (h *harness) pumpFor(d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case m := <-h.msgs:
			h.send(m)
		case <-deadline:
			return
		}
	}
}

// pumpUntil delivers messages until cond holds or the deadline passes.
func (h *harness) pumpUntil(d time.Duration, cond func() bool) bool {
	deadline := time.After(d)
	for {
		if cond() {
			return true
		}
		select {
		case m := <-h.msgs:
			h.send(m)
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			return cond()
		}
	}
}
