package tick

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type msg struct{}

func TestRunningTwiceIsSafe(t *testing.T) {
	cmd := After(5*time.Millisecond, func(time.Time) tea.Msg { return msg{} })
	for i := 0; i < 2; i++ {
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("run %d blocked — tea.Tick's failure mode", i+1)
		}
	}
}

func TestAfterWaits(t *testing.T) {
	start := time.Now()
	After(30*time.Millisecond, func(time.Time) tea.Msg { return msg{} })()
	if time.Since(start) < 30*time.Millisecond {
		t.Error("After should wait its duration")
	}
}

func TestInstant(t *testing.T) {
	t.Run("instant", func(t *testing.T) {
		Instant(t)
		start := time.Now()
		After(time.Hour, func(time.Time) tea.Msg { return msg{} })()
		if time.Since(start) > time.Second {
			t.Error("Instant should skip the wait")
		}
	})
	if instant.Load() {
		t.Error("Instant should end with its test")
	}
}
