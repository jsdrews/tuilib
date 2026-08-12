package runner

import (
	"bufio"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// Capture runs cmd without handing the terminal over, streaming its stdout
// and stderr back as messages while the TUI stays live.
//
// This is the counterpart to Run, not a mode of it. Run suspends the program
// and gives the subprocess the real TTY, which is right for an editor, a
// pager, or htop — and which means the output is gone the moment the TUI
// repaints. Capture is for the other kind of subprocess: the one whose output
// you actually want to read. Teeing a full-screen program would be
// meaningless, so the two cannot be the same call.
//
// The message sequence is CaptureStarted, then a CapturedLine per line, then
// exactly one Captured. Each message carries the handle needed to ask for the
// next one — see Next — so the consumer drives the read at its own pace:
//
//	case runner.CaptureStarted, runner.CapturedLine:
//	    return s, runner.Next(msg)
//
// The app shell does this for you when app.Options.OutputKey is set, and
// forwards every message on to the active screen besides. Nothing here
// depends on tuilib: the shell translates these messages into log records,
// because the log format, the source attribution and the read-marker are
// shell knowledge, not runner knowledge.
//
// Backpressure is real and deliberate: the stream buffers a bounded number of
// lines and a consumer that stops calling Next will eventually stall the
// subprocess rather than grow memory without limit.
func Capture(cmd *exec.Cmd) tea.Cmd {
	return CaptureWith(CaptureOptions{Cmd: cmd})
}

// CaptureOptions configures CaptureWith.
type CaptureOptions struct {
	// Cmd is the subprocess to run. Required.
	Cmd *exec.Cmd
	// Label names the run in the log and in the kill picker. Defaults to
	// the command's base name, which is also what the log uses as the
	// line's Source — for captured output the honest answer to "what
	// produced this line" is the command, not the screen that launched it.
	Label string
}

// CaptureWith runs a subprocess with the given options. See Capture.
func CaptureWith(opts CaptureOptions) tea.Cmd {
	return func() tea.Msg {
		cmd := opts.Cmd
		label := opts.Label
		if label == "" && cmd != nil && cmd.Path != "" {
			label = filepath.Base(cmd.Path)
		}

		st := &stream{
			runID: captureSeq.Add(1),
			label: label,
			cmd:   cmd,
			ch:    make(chan tea.Msg, captureBuffer),
		}

		if cmd == nil {
			st.finish(nil)
			return st.started()
		}

		outR, outW := io.Pipe()
		errR, errW := io.Pipe()
		cmd.Stdout = outW
		cmd.Stderr = errW

		if err := cmd.Start(); err != nil {
			outW.Close()
			errW.Close()
			// Report it as a run that started and immediately ended, so a
			// consumer's start/end bookkeeping stays symmetric and the
			// failure still reaches the log.
			st.finish(err)
			return st.started()
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go st.scan(outR, false, &wg)
		go st.scan(errR, true, &wg)

		go func() {
			// Wait returns once the copy goroutines exec created for our
			// non-*os.File writers have drained, so closing the writers
			// afterwards is what gives the scanners their EOF.
			err := cmd.Wait()
			outW.Close()
			errW.Close()
			wg.Wait()
			st.finish(err)
		}()

		return st.started()
	}
}

// captureBuffer bounds how far the subprocess may run ahead of the UI.
const captureBuffer = 512

var captureSeq atomic.Int64

// CaptureStarted is delivered when a capture begins. It carries the *exec.Cmd
// so a consumer can retain a kill handle: nothing else in the sequence offers
// one, and by the time Captured arrives there is nothing left to signal.
type CaptureStarted struct {
	RunID int64
	Label string
	Cmd   *exec.Cmd

	stream *stream
}

// CapturedLine is one line of subprocess output.
type CapturedLine struct {
	RunID  int64
	Label  string
	Text   string
	Stderr bool

	stream *stream
}

// Captured is delivered once, after the last CapturedLine, when the
// subprocess has exited. Err is the *exec.ExitError for a non-zero exit, or
// the start error when the process never ran.
type Captured struct {
	RunID int64
	Label string
	Cmd   *exec.Cmd
	Err   error

	stream *stream
}

// Next returns the command that reads the next message from the capture that
// msg belongs to, or nil for anything else — including Captured, after which
// there is nothing more to read.
func Next(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case CaptureStarted:
		return m.stream.next()
	case CapturedLine:
		return m.stream.next()
	}
	return nil
}

// next reads one message from the stream. The nil check covers messages
// built by hand rather than by a real capture — tests, mostly — which would
// otherwise panic only once the returned command was run, well away from
// the line that constructed them.
func (s *stream) next() tea.Cmd {
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-s.ch
		if !ok {
			return nil
		}
		return msg
	}
}

// Kill signals the subprocess behind a CaptureStarted. Safe to call on a run
// that never started or has already exited — both report nil, since in either
// case there is nothing left to stop.
func Kill(m CaptureStarted) error {
	if m.Cmd == nil || m.Cmd.Process == nil {
		return nil
	}
	return m.Cmd.Process.Kill()
}

// stream is the live side of a capture: a bounded channel the reader
// goroutines push into and Next pulls from.
type stream struct {
	runID int64
	label string
	cmd   *exec.Cmd
	ch    chan tea.Msg

	once sync.Once
}

func (s *stream) started() CaptureStarted {
	return CaptureStarted{RunID: s.runID, Label: s.label, Cmd: s.cmd, stream: s}
}

func (s *stream) scan(r io.Reader, stderr bool, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		s.ch <- CapturedLine{
			RunID:  s.runID,
			Label:  s.label,
			Text:   sc.Text(),
			Stderr: stderr,
			stream: s,
		}
	}
}

// finish posts the terminal message and closes the channel. Guarded by a
// sync.Once because the start-failure path and the normal exit path both
// lead here, and a double close would panic.
func (s *stream) finish(err error) {
	s.once.Do(func() {
		go func() {
			s.ch <- Captured{
				RunID:  s.runID,
				Label:  s.label,
				Cmd:    s.cmd,
				Err:    err,
				stream: s,
			}
			close(s.ch)
		}()
	})
}
