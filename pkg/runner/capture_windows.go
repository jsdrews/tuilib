//go:build windows

package runner

import "os/exec"

// setProcessGroup is a no-op on Windows: process groups work differently and
// there is no portable equivalent of signalling one. A killed capture stops
// the process we started; anything it spawned is left to the OS.
func setProcessGroup(*exec.Cmd) {}

// killProcess stops the subprocess. See setProcessGroup for what this does
// not cover on Windows.
func killProcess(cmd *exec.Cmd) error { return ignoreDone(cmd.Process.Kill()) }
