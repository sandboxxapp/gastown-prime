//go:build !windows

package util

import (
	"os/exec"
	"testing"
)

// TestSetDaemonProcessGroupSetsSetsid verifies that daemon processes are
// fully detached into their own session (Setsid=true), not just a new process
// group (Setpgid=true). Without Setsid, the daemon shares a session with its
// parent and can receive SIGHUP when the parent terminal exits.
// See sbx-gastown-6d7d bug #1: daemon stops when sling exits.
//
// Setpgid must NOT be set alongside it: the child calls setsid() first and
// becomes a session leader, and setpgid() on a session leader fails with
// EPERM, so the pair makes every Start fail (sbx-gastown-slvou6).
func TestSetDaemonProcessGroupSetsSetsid(t *testing.T) {
	cmd := exec.Command("true")
	SetDaemonProcessGroup(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr should be set")
	}
	if !cmd.SysProcAttr.Setsid {
		t.Error("SetDaemonProcessGroup must set Setsid=true for full session detach")
	}
	if cmd.SysProcAttr.Setpgid {
		t.Error("SetDaemonProcessGroup must not set Setpgid with Setsid: setpgid on a session leader is EPERM")
	}
}

// TestSetDaemonProcessGroupStartsChild actually starts a child with the
// daemon flags. The gt exit seam test stubs scheduleSelfTerminate, so before
// this test nothing exercised the real SysProcAttr, and Setpgid+Setsid failed
// every `gt _reap-session` and `gt up` daemon start with
// "fork/exec ...: operation not permitted" (sbx-gastown-slvou6).
func TestSetDaemonProcessGroupStartsChild(t *testing.T) {
	cmd := exec.Command("true")
	SetDaemonProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start child with SetDaemonProcessGroup: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait child: %v", err)
	}
}

// TestSetDetachedProcessGroupStartsChild guards the same failure for the
// Setpgid-only helper (7dc0429a removed Setsid from it for the same EPERM).
func TestSetDetachedProcessGroupStartsChild(t *testing.T) {
	cmd := exec.Command("true")
	SetDetachedProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start child with SetDetachedProcessGroup: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait child: %v", err)
	}
}
