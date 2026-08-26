package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestGoStreamsLinesThenFinishes(t *testing.T) {
	started, lines, done := drain(t, Go("build", func(_ context.Context, out io.Writer) error {
		fmt.Fprintln(out, "step one")
		fmt.Fprintln(out, "step two")
		return nil
	}))

	if started.Label != "build" {
		t.Errorf("Label = %q, want build", started.Label)
	}
	if len(lines) != 2 || lines[0].Text != "step one" || lines[1].Text != "step two" {
		t.Fatalf("lines = %+v, want two in order", lines)
	}
	if done.Err != nil {
		t.Errorf("Err = %v, want nil", done.Err)
	}
	if done.RunID != started.RunID {
		t.Errorf("RunID %d != %d — the run must be correlatable end to end", done.RunID, started.RunID)
	}
}

// The head line is the whole reason CaptureStarted grew Detail: a Go run has
// no Cmd, so a consumer deriving the head from the command renders
// "(no command)" for every one of them.
func TestGoSetsADetailHeadLine(t *testing.T) {
	started, _, _ := drain(t, Go("restart api", func(context.Context, io.Writer) error { return nil }))
	if started.Detail != "restart api" {
		t.Errorf("Detail = %q, want the label", started.Detail)
	}
	if started.Cmd != nil {
		t.Errorf("Cmd = %v, want nil for a Go run", started.Cmd)
	}
}

func TestGoWithOverridesTheDetail(t *testing.T) {
	started, _, _ := drain(t, GoWith(GoOptions{
		Label:  "restart",
		Detail: "restart api-server (3 replicas)",
		Run:    func(context.Context, io.Writer) error { return nil },
	}))
	if started.Detail != "restart api-server (3 replicas)" {
		t.Errorf("Detail = %q", started.Detail)
	}
}

// A subprocess capture must keep describing itself as its command line.
func TestCaptureStillSetsItsCommandAsTheDetail(t *testing.T) {
	started, _, _ := drain(t, Capture(exec.Command("sh", "-c", "echo hi")))
	if !strings.HasPrefix(started.Detail, "$ ") || !strings.Contains(started.Detail, "echo hi") {
		t.Errorf("Detail = %q, want the command line", started.Detail)
	}
}

func TestGoReportsTheError(t *testing.T) {
	boom := errors.New("no route to host")
	_, _, done := drain(t, Go("fetch", func(context.Context, io.Writer) error { return boom }))
	if !errors.Is(done.Err, boom) {
		t.Errorf("Err = %v, want %v", done.Err, boom)
	}
}

// fn is under no obligation to end with a newline, and losing the last line
// would silently drop exactly the summary people write last.
func TestGoEmitsATrailingPartialLine(t *testing.T) {
	_, lines, _ := drain(t, Go("x", func(_ context.Context, out io.Writer) error {
		fmt.Fprint(out, "done, no newline")
		return nil
	}))
	if len(lines) != 1 || lines[0].Text != "done, no newline" {
		t.Errorf("lines = %+v, want the unterminated line", lines)
	}
}

func TestGoSplitsOnNewlinesAcrossWrites(t *testing.T) {
	_, lines, _ := drain(t, Go("x", func(_ context.Context, out io.Writer) error {
		out.Write([]byte("alpha\nbra"))
		out.Write([]byte("vo\ncharlie\n"))
		return nil
	}))
	want := []string{"alpha", "bravo", "charlie"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(lines), len(want), lines)
	}
	for i, w := range want {
		if lines[i].Text != w {
			t.Errorf("line %d = %q, want %q", i, lines[i].Text, w)
		}
	}
}

func TestGoStripsCarriageReturns(t *testing.T) {
	_, lines, _ := drain(t, Go("x", func(_ context.Context, out io.Writer) error {
		fmt.Fprint(out, "windows\r\nline\r\n")
		return nil
	}))
	for _, l := range lines {
		if strings.HasSuffix(l.Text, "\r") {
			t.Errorf("line %q kept its carriage return", l.Text)
		}
	}
}

// Kill cancels the context. Whether that stops anything is fn's business, but
// the request has to be delivered.
func TestKillCancelsAGoRun(t *testing.T) {
	release := make(chan struct{})
	cmd := Go("slow", func(ctx context.Context, out io.Writer) error {
		fmt.Fprintln(out, "working")
		close(release)
		<-ctx.Done()
		return ctx.Err()
	})

	msg := cmd()
	started := msg.(CaptureStarted)
	<-release

	if err := Kill(started); err != nil {
		t.Fatalf("Kill returned %v", err)
	}

	// Drain to the terminal message.
	next := Next(started)
	deadline := time.After(5 * time.Second)
	for next != nil {
		ch := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { ch <- c() }(next)
		select {
		case m := <-ch:
			switch v := m.(type) {
			case CapturedLine:
				next = Next(v)
			case Captured:
				if !errors.Is(v.Err, context.Canceled) {
					t.Errorf("Err = %v, want context.Canceled", v.Err)
				}
				return
			default:
				t.Fatalf("unexpected %T", m)
			}
		case <-deadline:
			t.Fatal("killed run never finished")
		}
	}
}

// A panic would otherwise take the program down and leave the badge showing ⟳
// for a run that will never finish.
func TestGoReportsAPanicAsAFailure(t *testing.T) {
	_, _, done := drain(t, Go("boom", func(context.Context, io.Writer) error {
		panic("kaboom")
	}))
	if done.Err == nil || !strings.Contains(done.Err.Error(), "kaboom") {
		t.Errorf("Err = %v, want the panic reported as an error", done.Err)
	}
}

func TestGoWithNilRunStillReportsSymmetrically(t *testing.T) {
	started, lines, done := drain(t, GoWith(GoOptions{Label: "empty"}))
	if len(lines) != 0 {
		t.Errorf("lines = %+v, want none", lines)
	}
	if done.RunID != started.RunID {
		t.Error("a nil Run must still produce a matching start/end pair")
	}
}

// fn may hand the writer to anything — an exec.Cmd's Stdout, another
// goroutine — so concurrent writes must not interleave into corrupted lines.
func TestGoWriterIsSafeForConcurrentWrites(t *testing.T) {
	_, lines, _ := drain(t, Go("x", func(_ context.Context, out io.Writer) error {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					fmt.Fprintf(out, "writer-%d-line-%d\n", n, j)
				}
			}(i)
		}
		wg.Wait()
		return nil
	}))

	if len(lines) != 40 {
		t.Fatalf("got %d lines, want 40", len(lines))
	}
	for _, l := range lines {
		if !strings.HasPrefix(l.Text, "writer-") || strings.Count(l.Text, "writer-") != 1 {
			t.Errorf("line %q is interleaved", l.Text)
		}
	}
}

// Go output is stdout-shaped. Severity comes from the run's error, never from
// the stream — the same rule that keeps the badge from going permanently red.
func TestGoLinesAreNotMarkedStderr(t *testing.T) {
	_, lines, _ := drain(t, Go("x", func(_ context.Context, out io.Writer) error {
		fmt.Fprintln(out, "progress")
		return errors.New("failed anyway")
	}))
	for _, l := range lines {
		if l.Stderr {
			t.Errorf("line %q marked as stderr", l.Text)
		}
	}
}
