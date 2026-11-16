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
// Uses Setpgid only, safe for every subprocess call. For session detachment,
// use SetDaemonProcessGroup (Setsid only; Setpgid with Setsid is EPERM).
func SetDetachedProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// SetDaemonProcessGroup configures a command for full daemon detachment.
// Uses Setsid for session independence — only use for detached daemon-style
// children (gt up's daemon, gt exit's _reap-session). Setsid alone suffices:
// a new session leader is already its own process-group leader. Do not add
// Setpgid: the child runs setsid() first, and setpgid() on a session leader
// fails with EPERM, so every Start fails (sbx-gastown-slvou6).
func SetDaemonProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
