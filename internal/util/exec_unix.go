//go:build !windows

package util

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup configures a command to run in its own process group so that
// context cancellation kills the entire process tree, preventing orphaned children.
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}

// SetDetachedProcessGroup configures a command to run in its own process group.
// Uses Setpgid only — Setsid causes "operation not permitted" on macOS when
// the calling process is already a session leader. For daemon detachment,
// use SetDaemonProcessGroup which adds Setsid.
func SetDetachedProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// SetDaemonProcessGroup configures a command for full daemon detachment.
// Adds Setsid for session independence — only use for the daemon subprocess.
func SetDaemonProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Setsid: true}
}
