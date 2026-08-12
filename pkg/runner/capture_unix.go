//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the subprocess in its own process group.
//
// Without this, killing a capture kills only the process we started. A shell
// running `make build` forwards nothing: the compiler keeps running, keeps
// the pipe's write end open, and the capture never reports Captured — so the
// statusbar badge shows a run in flight that the user can no longer stop,
// because the thing it knows how to signal is already dead.
//
// The trade is that the group no longer receives the terminal's signals
// (a Ctrl-C aimed at the TUI won't reach it). For a captured subprocess that
// is what we want anyway: it never had the terminal, and bubbletea handles
// Ctrl-C itself.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcess signals the whole process group, falling back to the single
// process when the group send fails (which happens if setProcessGroup was
// bypassed, or the group is already gone).
//
// A run that has already exited reports nil rather than an error: the user
// pressing "x" on a capture that finished a moment earlier got what they
// asked for, and surfacing "process already finished" as a failed kill would
// be a lie about something they cannot act on.
func killProcess(cmd *exec.Cmd) error {
	switch err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err {
	case nil:
		return nil
	case syscall.ESRCH:
		return nil
	}
	return ignoreDone(cmd.Process.Kill())
}
