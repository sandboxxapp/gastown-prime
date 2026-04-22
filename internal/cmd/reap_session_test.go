package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// TestReapSession_KillsTmuxSession verifies that `gt _reap-session` kills the
// named tmux session after the configured delay. This is the behavior gt exit
// depends on — the in-process goroutine it replaces died with the parent
// process and never reaped the session (sbx-gastown-xpuv).
func TestReapSession_KillsTmuxSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	// Isolate from the user's tmux server with a per-test socket.
	socket := fmt.Sprintf("gt-reap-test-%d", os.Getpid())
	sessionName := "gt-reap-under-test"

	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})

	newSess := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", sessionName, "sleep 300")
	if out, err := newSess.CombinedOutput(); err != nil {
		t.Fatalf("create tmux session: %v (%s)", err, out)
	}

	// Point NewTmux() at our test socket.
	origSocket := tmux.GetDefaultSocket()
	tmux.SetDefaultSocket(socket)
	t.Cleanup(func() { tmux.SetDefaultSocket(origSocket) })

	origName, origDelay := reapSessionName, reapSessionDelay
	t.Cleanup(func() { reapSessionName, reapSessionDelay = origName, origDelay })

	reapSessionName = sessionName
	reapSessionDelay = 200 * time.Millisecond

	start := time.Now()
	if err := runReapSession(nil, nil); err != nil {
		t.Fatalf("runReapSession: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Errorf("returned before delay elapsed (took %v)", elapsed)
	}

	if err := exec.Command("tmux", "-L", socket, "has-session", "-t", sessionName).Run(); err == nil {
		t.Fatalf("session %q still alive after reap", sessionName)
	}
}

// TestReapSession_MissingSessionIsIdempotent verifies that reaping an already-
// dead session is a no-op. KillSessionWithProcesses is documented idempotent
// and gt exit relies on that in the "no tmux server" / re-entry paths.
func TestReapSession_MissingSessionIsIdempotent(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	socket := fmt.Sprintf("gt-reap-test-missing-%d", os.Getpid())
	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})

	origSocket := tmux.GetDefaultSocket()
	tmux.SetDefaultSocket(socket)
	t.Cleanup(func() { tmux.SetDefaultSocket(origSocket) })

	origName, origDelay := reapSessionName, reapSessionDelay
	t.Cleanup(func() { reapSessionName, reapSessionDelay = origName, origDelay })

	reapSessionName = "gt-never-existed"
	reapSessionDelay = 0

	if err := runReapSession(nil, nil); err != nil {
		t.Errorf("reaping missing session should be a no-op, got: %v", err)
	}
}

// TestDefaultScheduleSelfTerminate_ChildOutlivesParent is the regression test
// for sbx-gastown-xpuv. Under the old code, the self-terminate was an
// in-process goroutine killed when runExit returned; the session never died.
// Here we spawn the detached child exactly as defaultScheduleSelfTerminate
// does, release the handle, and confirm the tmux session is still reaped.
func TestDefaultScheduleSelfTerminate_ChildOutlivesParent(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	if testing.Short() {
		t.Skip("builds a gt binary; skipped in -short mode")
	}

	gtBin := filepath.Join(t.TempDir(), "gt-test")
	// Set BuiltProperly so the binary doesn't self-abort in persistentPreRun
	// (which enforces `make build` in production).
	build := exec.Command("go", "build",
		"-ldflags", "-X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1",
		"-o", gtBin,
		"github.com/steveyegge/gastown/cmd/gt")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build cmd/gt: %v (%s)", err, b)
	}

	socket := fmt.Sprintf("gt-reap-survive-%d", os.Getpid())
	sessionName := "gt-reap-survive"
	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})

	newSess := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", sessionName, "sleep 300")
	if out, err := newSess.CombinedOutput(); err != nil {
		t.Fatalf("create tmux session: %v (%s)", err, out)
	}

	// Mirror defaultScheduleSelfTerminate: detached child, closed stdio, run
	// against our per-test tmux socket. GT_TMUX_SOCKET is honored by
	// session.InitRegistry during the child's persistentPreRun.
	cmd := exec.Command(gtBin, "_reap-session",
		"--session", sessionName,
		"--delay", "300ms")
	cmd.Env = append(os.Environ(), "GT_TMUX_SOCKET="+socket)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	// NOTE: production uses Setpgid+Setsid via util.SetDaemonProcessGroup.
	// macOS rejects Setsid with EPERM when the caller is already a session
	// leader (as `go test` is), so this test uses Setpgid only. That still
	// exercises the pattern the bug requires: an out-of-process child that
	// is not a Go goroutine inside the original `gt` process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start detached reaper: %v", err)
	}
	// Release the handle so the child is reparented on test-goroutine exit,
	// simulating gt returning while the child is still mid-sleep.
	_ = cmd.Process.Release()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := exec.Command("tmux", "-L", socket, "has-session", "-t", sessionName).Run(); err != nil {
			return // success: session gone
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("session %q never reaped by detached child", sessionName)
}
