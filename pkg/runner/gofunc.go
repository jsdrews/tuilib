package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Go runs fn on its own goroutine and streams whatever it writes back as the
// same messages a subprocess Capture produces: CaptureStarted, a CapturedLine
// per line, then exactly one Captured carrying fn's error.
//
// This is Capture's counterpart for work that is not a subprocess — an HTTP
// call, a k8s API request, a file write. Everything downstream already knows
// how to handle it: the app shell logs the run into the output console,
// counts it as one event in the statusbar badge, and lists it in the kill
// picker, without a line of new code on either side.
//
//	return runner.Go("restart api", func(ctx context.Context, out io.Writer) error {
//	    fmt.Fprintln(out, "scaling to 0")
//	    return api.Restart(ctx, "api")
//	})
//
// # Cancellation is cooperative
//
// runner.Kill cancels the context. Whether that stops anything is up to fn:
// one that selects on ctx.Done() or hands ctx to an HTTP request stops
// promptly; one that ignores it runs to completion and its result is
// reported normally. There is no way to preempt a goroutine in Go, so the
// honest contract is "the request is delivered", not "the work is stopped".
//
// # Backpressure
//
// out shares the capture buffer, so a consumer that stops calling Next
// eventually blocks fn's next write rather than growing memory without limit
// — the same trade a subprocess capture makes.
func Go(label string, fn func(ctx context.Context, out io.Writer) error) tea.Cmd {
	return GoWith(GoOptions{Label: label, Run: fn})
}

// GoOptions configures GoWith.
type GoOptions struct {
	// Label names the run in the log and in the kill picker. Defaults to
	// "task".
	Label string

	// Detail is the head line for the run. Defaults to Label. Set it when
	// the label is a short name but the head should say more.
	Detail string

	// Run is the work. Required; a nil Run reports a run that started and
	// immediately finished, so a consumer's start/end bookkeeping stays
	// symmetric.
	Run func(ctx context.Context, out io.Writer) error

	// Tag is an opaque correlation token echoed on every message from this
	// run. See CaptureStarted.Tag.
	Tag string

	// Context is the parent for the run's cancellable context. Defaults to
	// context.Background().
	Context context.Context
}

// GoWith runs Go work with the given options. See Go.
func GoWith(opts GoOptions) tea.Cmd {
	return func() tea.Msg {
		label := opts.Label
		if label == "" {
			label = "task"
		}
		detail := opts.Detail
		if detail == "" {
			detail = label
		}
		parent := opts.Context
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithCancel(parent)

		st := &stream{
			runID:  captureSeq.Add(1),
			label:  label,
			detail: detail,
			tag:    opts.Tag,
			cancel: cancel,
			ch:     make(chan tea.Msg, captureBuffer),
		}

		if opts.Run == nil {
			cancel()
			st.finish(nil)
			return st.started()
		}

		go func() {
			w := &lineWriter{st: st}
			// A panic in fn would otherwise take the program down and leave
			// the run in flight forever — the badge showing ⟳ for something
			// that will never finish. Report it as the failure it is.
			defer func() {
				if r := recover(); r != nil {
					w.flush()
					cancel()
					st.finish(panicError(r))
				}
			}()
			err := opts.Run(ctx, w)
			w.flush()
			cancel()
			st.finish(err)
		}()

		return st.started()
	}
}

// maxLineBytes caps a single unbroken line, matching the scanner's limit on
// the subprocess path. Output past it is emitted as its own line rather than
// buffered forever, so a writer that never sends a newline cannot grow the
// buffer without bound.
const maxLineBytes = 1024 * 1024

// lineWriter turns arbitrary Writes into one CapturedLine per newline.
//
// It is mutex-guarded because fn is free to hand out the writer: pointing an
// exec.Cmd's Stdout at it, or writing from two goroutines, is a reasonable
// thing to do and would otherwise interleave into corrupted lines.
type lineWriter struct {
	st  *stream
	mu  sync.Mutex
	buf []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) >= maxLineBytes {
		w.emit(w.buf)
		w.buf = nil
	}
	return len(p), nil
}

// flush emits a trailing partial line. fn is under no obligation to end its
// output with a newline, and dropping the last line because of that would be
// a silent loss of exactly the summary people write last.
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
}

func (w *lineWriter) emit(b []byte) {
	w.st.line(string(bytes.TrimSuffix(b, []byte("\r"))), false)
}

// panicError wraps a recovered panic value as an error, so a run that blew up
// is reported through the same channel as one that returned an error.
func panicError(r any) error {
	if err, ok := r.(error); ok {
		return fmt.Errorf("panic: %w", err)
	}
	return fmt.Errorf("panic: %v", r)
}
