// Package runner runs work from inside a Bubble Tea program, in three shapes
// that differ in what happens to the terminal and to the output.
//
//   - Run hands the real TTY to a subprocess, suspending the TUI for the
//     duration. Right for $EDITOR, less, htop — and the reason its output is
//     unrecoverable, since the subprocess owns the screen.
//   - Capture runs a subprocess without suspending anything, streaming its
//     stdout and stderr back as messages while the TUI stays live.
//   - Go does the same for work that is not a subprocess at all — an HTTP
//     call, an API request, a file write — streaming whatever the function
//     writes to an io.Writer.
//
// Capture and Go emit the identical message sequence, so everything
// downstream handles them the same way: the app shell logs both into the
// output console, counts each as one event, and offers both in the kill
// picker. See capture.go and gofunc.go.
//
// This package imports nothing from tuilib, which is what makes it safe for
// anything to depend on. Its messages are deliberately neutral; pkg/app is
// what turns them into log records, because the log format and the source
// attribution are shell knowledge.
//
// # Run
//
// Run suspends the TUI (releasing the terminal so the subprocess can take
// over stdin/stdout/stderr), executes the command, then re-enters the
// alt-screen once the subprocess exits.
//
// Use it for editors ($EDITOR), pagers (less, man), full-screen TUIs
// (htop, k9s), or one-shot interactive commands (ssh, kubectl exec). For
// the duration of the run the TUI is fully suspended — the subprocess
// owns the terminal.
//
// Usage:
//
//	// dispatch from a screen's Update on some key:
//	cmd := exec.Command(os.Getenv("EDITOR"), "/tmp/scratch")
//	return s, runner.Run(cmd)
//
//	// receive the result on a later Update tick:
//	case runner.Result:
//	    s.last = msg // msg.Cmd.ProcessState is populated; msg.Err is the run error
package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Result is delivered to your screen's Update when the subprocess exits.
// Cmd is the same *exec.Cmd you submitted (its ProcessState is populated
// by the OS); Err is non-nil when the process failed to start or exited
// with a non-zero status (the typical *exec.ExitError).
type Result struct {
	Cmd *exec.Cmd
	Err error
}

// Options configures RunWith. The zero value clears the screen and prints
// no notice — the right defaults for typical interactive subprocesses.
type Options struct {
	// Cmd is the subprocess to run. Required.
	Cmd *exec.Cmd
	// Notice, when non-empty, is printed once to stderr after the TUI
	// suspends and before the subprocess starts. Use it for slow handoffs
	// (kubectl exec, ssh, anything with a perceptible connect latency) so
	// the user sees feedback instead of a blank gap. The subprocess is
	// free to clear the screen on startup; that's fine, the goal is
	// feedback during the handoff, not a persistent banner.
	Notice string
	// NoClear suppresses the screen clear that normally precedes the
	// subprocess. By default the terminal is cleared so the alt-screen
	// exit doesn't leave TUI artifacts visible during commands that
	// don't repaint (sh -c, echo, short scripts). Set NoClear=true to
	// preserve whatever was on the normal screen prior to the TUI.
	NoClear bool
}

// Run returns a tea.Cmd that suspends the program, runs cmd connected to
// the controlling terminal, and posts a Result when the subprocess exits.
// The screen is cleared before the subprocess starts (use RunWith with
// NoClear=true to opt out).
//
// Plumbing the runner takes care of:
//
//   - Stdin/Stdout/Stderr default to os.Stdin/Stdout/Stderr (real TTY
//     file descriptors) when not already set, so the subprocess gets
//     direct terminal access and TIOCGWINSZ works.
//   - LINES and COLUMNS env vars are populated from the current terminal
//     size, as a fallback for ncurses-style programs that miss the
//     post-resume SIGWINCH on some terminal emulators (htop, top, less
//     are the usual suspects).
func Run(cmd *exec.Cmd) tea.Cmd {
	return RunWith(Options{Cmd: cmd})
}

// RunWithNotice is shorthand for RunWith(Options{Cmd: cmd, Notice: notice}).
// The screen is cleared before the notice is printed.
func RunWithNotice(cmd *exec.Cmd, notice string) tea.Cmd {
	return RunWith(Options{Cmd: cmd, Notice: notice})
}

// RunWith runs an interactive subprocess with the given options. See Options
// for the available knobs (notice, screen-clear).
func RunWith(opts Options) tea.Cmd {
	prepCmd(opts.Cmd)
	return tea.Exec(&wrappedCmd{
		cmd:    opts.Cmd,
		notice: opts.Notice,
		clear:  !opts.NoClear,
	}, func(err error) tea.Msg {
		return Result{Cmd: opts.Cmd, Err: err}
	})
}

func prepCmd(cmd *exec.Cmd) {
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
		cmd.Env = appendOrReplaceEnv(cmd.Env, "LINES", fmt.Sprintf("%d", h))
		cmd.Env = appendOrReplaceEnv(cmd.Env, "COLUMNS", fmt.Sprintf("%d", w))
	}
}

type wrappedCmd struct {
	cmd    *exec.Cmd
	notice string
	clear  bool
}

func (c *wrappedCmd) Run() error {
	if c.clear {
		// ESC[2J clears the screen; ESC[H homes the cursor.
		fmt.Fprint(os.Stderr, "\x1b[2J\x1b[H")
	}
	if c.notice != "" {
		fmt.Fprintln(os.Stderr, c.notice)
	}
	return c.cmd.Run()
}

func (c *wrappedCmd) SetStdin(r io.Reader) {
	if c.cmd.Stdin == nil {
		c.cmd.Stdin = r
	}
}

func (c *wrappedCmd) SetStdout(w io.Writer) {
	if c.cmd.Stdout == nil {
		c.cmd.Stdout = w
	}
}

func (c *wrappedCmd) SetStderr(w io.Writer) {
	if c.cmd.Stderr == nil {
		c.cmd.Stderr = w
	}
}

func appendOrReplaceEnv(env []string, key, value string) []string {
	if env == nil {
		env = os.Environ()
	}
	prefix := key + "="
	for i, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
