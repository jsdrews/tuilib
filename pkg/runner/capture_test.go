package runner

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// drain runs a capture to completion the way the app shell does: take the
// start message, then keep asking for the next one until Captured arrives.
func drain(t *testing.T, cmd tea.Cmd) (started CaptureStarted, lines []CapturedLine, done Captured) {
	t.Helper()

	msg := cmd()
	started, ok := msg.(CaptureStarted)
	if !ok {
		t.Fatalf("first message is %T, want CaptureStarted", msg)
	}

	deadline := time.After(10 * time.Second)
	next := Next(started)
	for next != nil {
		type result struct{ msg tea.Msg }
		ch := make(chan result, 1)
		go func(c tea.Cmd) { ch <- result{c()} }(next)

		select {
		case r := <-ch:
			switch m := r.msg.(type) {
			case CapturedLine:
				lines = append(lines, m)
				next = Next(m)
			case Captured:
				return started, lines, m
			case nil:
				t.Fatal("stream closed before Captured arrived")
			default:
				t.Fatalf("unexpected message %T", r.msg)
			}
		case <-deadline:
			t.Fatal("capture did not finish within 10s")
		}
	}
	t.Fatal("stream ended without a Captured message")
	return
}

func TestCaptureStreamsStdoutAndStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo out-one; echo err-one 1>&2; echo out-two")
	started, lines, done := drain(t, Capture(cmd))

	if started.Label != "sh" {
		t.Errorf("label = %q, want the command's base name", started.Label)
	}
	if done.Err != nil {
		t.Errorf("Captured.Err = %v, want nil for a clean exit", done.Err)
	}

	var out, errs []string
	for _, l := range lines {
		if l.RunID != started.RunID {
			t.Errorf("line carries RunID %d, want %d", l.RunID, started.RunID)
		}
		if l.Stderr {
			errs = append(errs, l.Text)
		} else {
			out = append(out, l.Text)
		}
	}
	if strings.Join(out, ",") != "out-one,out-two" {
		t.Errorf("stdout lines = %v", out)
	}
	if strings.Join(errs, ",") != "err-one" {
		t.Errorf("stderr lines = %v", errs)
	}
}

func TestCaptureReportsNonZeroExit(t *testing.T) {
	_, _, done := drain(t, Capture(exec.Command("sh", "-c", "echo nope 1>&2; exit 3")))

	if done.Err == nil {
		t.Fatal("Captured.Err is nil for a non-zero exit")
	}
	if !strings.Contains(done.Err.Error(), "3") {
		t.Errorf("Err = %v, want it to mention the exit status", done.Err)
	}
}

// A command that never starts still has to produce a start and an end, or a
// consumer's in-flight bookkeeping leaks a run that will never finish.
func TestCaptureOfAMissingBinaryStillCompletes(t *testing.T) {
	started, lines, done := drain(t, Capture(exec.Command("/nonexistent/definitely-not-here")))

	if len(lines) != 0 {
		t.Errorf("got %d lines from a command that never ran", len(lines))
	}
	if done.Err == nil {
		t.Error("Captured.Err is nil for a command that failed to start")
	}
	if done.RunID != started.RunID {
		t.Errorf("RunID mismatch: started %d, finished %d", started.RunID, done.RunID)
	}
}

// Output longer than the channel buffer must not deadlock: the consumer
// drains at its own pace and the producer waits.
func TestCaptureHandlesMoreLinesThanTheBuffer(t *testing.T) {
	n := captureBuffer * 3
	cmd := exec.Command("sh", "-c", "i=0; while [ $i -lt "+itoa(n)+" ]; do echo line-$i; i=$((i+1)); done")

	_, lines, done := drain(t, Capture(cmd))
	if done.Err != nil {
		t.Fatalf("Captured.Err = %v", done.Err)
	}
	if len(lines) != n {
		t.Errorf("got %d lines, want %d", len(lines), n)
	}
}

func TestKillStopsALongRun(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	msg := Capture(cmd)()
	started := msg.(CaptureStarted)

	if err := Kill(started); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	deadline := time.After(10 * time.Second)
	next := Next(started)
	for {
		ch := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { ch <- c() }(next)
		select {
		case m := <-ch:
			switch v := m.(type) {
			case Captured:
				if v.Err == nil {
					t.Error("killed run reported a clean exit")
				}
				return
			case CapturedLine:
				next = Next(v)
			default:
				t.Fatalf("unexpected %T", m)
			}
		case <-deadline:
			t.Fatal("killed run never reported Captured")
		}
	}
}

func TestKillIsSafeOnARunThatNeverStarted(t *testing.T) {
	if err := Kill(CaptureStarted{}); err != nil {
		t.Errorf("Kill on an empty start = %v, want nil", err)
	}
}

func TestNextIsNilAfterCaptured(t *testing.T) {
	if got := Next(Captured{}); got != nil {
		t.Error("Next(Captured) returned a command; there is nothing left to read")
	}
	if got := Next(CaptureStarted{}); got != nil {
		t.Error("Next on a hand-built message should be nil rather than panic later")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// A shell that backgrounds work and exits leaves a grandchild holding the
// pipe's write end. The process is gone, so the run is over — but nothing
// will ever close that pipe, and a capture that waits for EOF before
// reporting Captured would leave the run wedged "in flight" forever: still
// counted by the statusbar badge, and no longer killable, because the only
// process the consumer has a handle for is already dead.
//
// This is what failed on Linux CI. macOS reaches it via the explicit orphan
// below rather than through `sh -c "sleep 30"`, whose shell execs.
func TestCaptureFinishesWhenADescendantHoldsThePipe(t *testing.T) {
	start := time.Now()
	_, lines, done := drain(t, Capture(exec.Command("sh", "-c",
		`echo before-orphan; sleep 30 & exit 0`)))

	if done.Err != nil {
		t.Errorf("Captured.Err = %v, want nil for a clean exit", done.Err)
	}
	if elapsed := time.Since(start); elapsed > drainGrace+5*time.Second {
		t.Errorf("took %v to report Captured — the orphan is gating the run", elapsed)
	}
	// Output written before the shell exited must survive the grace path.
	var got []string
	for _, l := range lines {
		got = append(got, l.Text)
	}
	if strings.Join(got, ",") != "before-orphan" {
		t.Errorf("lines = %v, want the output written before the orphan", got)
	}
}

// Killing a capture has to stop what the capture spawned. A shell wrapping a
// build forwards nothing, so signalling only the shell leaves the real work
// running — holding the pipe, off the badge, with no handle left to stop it.
func TestKillStopsDescendantsNotJustTheShell(t *testing.T) {
	cmd := exec.Command("sh", "-c", `sleep 30 & sleep 30 & wait`)
	started := Capture(cmd)().(CaptureStarted)

	// Let the shell actually spawn its children before signalling.
	time.Sleep(200 * time.Millisecond)
	if err := Kill(started); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	start := time.Now()
	deadline := time.After(10 * time.Second)
	next := Next(started)
	for {
		ch := make(chan tea.Msg, 1)
		go func(c tea.Cmd) { ch <- c() }(next)
		select {
		case m := <-ch:
			switch v := m.(type) {
			case Captured:
				if elapsed := time.Since(start); elapsed > drainGrace+3*time.Second {
					t.Errorf("took %v to finish — descendants survived the kill", elapsed)
				}
				return
			case CapturedLine:
				next = Next(v)
			default:
				t.Fatalf("unexpected %T", m)
			}
		case <-deadline:
			t.Fatal("killed run never reported Captured; its children outlived the signal")
		}
	}
}

// Pressing "x" on a run that finished a moment earlier is not a failure —
// the user got what they asked for, and "process already finished" is not
// something they can act on.
func TestKillOfAnAlreadyFinishedRunIsNotAnError(t *testing.T) {
	started, _, _ := drain(t, Capture(exec.Command("sh", "-c", "exit 0")))

	if err := Kill(started); err != nil {
		t.Errorf("Kill after exit = %v, want nil", err)
	}
}
