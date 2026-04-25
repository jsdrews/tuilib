// Package runner runs an interactive subprocess from inside a Bubble Tea
// program: it suspends the TUI (releasing the terminal so the subprocess
// can take over stdin/stdout/stderr), executes the command, then re-enters
// the alt-screen once the subprocess exits.
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

// Run returns a tea.Cmd that suspends the program, runs cmd connected to
// the controlling terminal, and posts a Result when the subprocess exits.
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
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return Result{Cmd: cmd, Err: err}
	})
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
