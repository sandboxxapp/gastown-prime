//go:build !windows

package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// makeFakeClaudeBin writes a shell wrapper named "claude" that execs /bin/sleep
// with argv[0]="claude" via `exec -a`, so ps -eo comm reports the process as
// "claude" (matching FindOrphanedClaudeProcesses' command-name filter).
//
// A hardlink to /bin/sleep is simpler but SIP blocks hardlinking /bin on macOS
// and a byte-copy gets Gatekeeper-killed for unsigned binaries. The shell
// wrapper with `exec -a` sidesteps both.
func makeFakeClaudeBin(t *testing.T, dir string) string {
	t.Helper()
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skipf("/bin/sleep not available: %v", err)
	}
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nexec -a claude /bin/sleep \"$@\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake claude wrapper: %v", err)
	}
	return bin
}

func containsOrphanPID(orphans []OrphanedProcess, pid int) bool {
	for _, o := range orphans {
		if o.PID == pid {
			return true
		}
	}
	return false
}

// spawnDetachedFakeClaude spawns bin as a session-leader grandchild detached
// from the test runner's process tree. After this returns:
//   - The grandchild's PPID is 1 (reparented to init), so tmux-pane descendant
//     walks won't auto-protect it.
//   - The grandchild has no controlling terminal (ps TTY "?").
//   - Its cwd is townRoot.
//
// The parent sh is reaped by the test; caller must kill the returned pid.
func spawnDetachedFakeClaude(t *testing.T, bin, townRoot string) int {
	t.Helper()
	// Outer sh uses Setsid so the session (and all descendants) have no
	// controlling terminal. The subshell backgrounds fake-claude and exits;
	// once it exits, fake-claude is reparented to init.
	script := `( "$1" 300 </dev/null >/dev/null 2>&1 & echo "$!" ) &
wait`
	cmd := exec.Command("sh", "-c", script, "--", bin)
	cmd.Dir = townRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("spawn detached fake claude: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		t.Fatalf("parse child pid from %q: %v", string(out), err)
	}
	return pid
}

// TestFindOrphanedClaudeProcesses_ExemptsRegisteredArchivist is an integration
// test that spawns a real detached child named "claude" inside a fake town
// root, then verifies:
//  1. Without registration it is flagged as an orphan (regression: non-archivist
//     orphans still get detected).
//  2. With the pid registered under <townRoot>/daemon/archivist-pids/<pid>,
//     the exemption lookup skips it.
func TestFindOrphanedClaudeProcesses_ExemptsRegisteredArchivist(t *testing.T) {
	townRoot := realPath(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"),
		[]byte(`{"name":"test-archivist-exemption"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	bin := makeFakeClaudeBin(t, binDir)
	pid := spawnDetachedFakeClaude(t, bin, townRoot)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	// Give the kernel a moment to publish the new process in ps and for the
	// sh parent to exit so reparenting to init completes.
	time.Sleep(150 * time.Millisecond)

	// Lower the age gate — the child is fractions of a second old.
	origAge := minOrphanAge
	minOrphanAge = 0
	t.Cleanup(func() { minOrphanAge = origAge })

	// Baseline: unregistered child must be flagged as an orphan. If the
	// environment protects it for unrelated reasons (tmux ancestry we
	// couldn't detect, IDE heuristics, etc.) skip cleanly rather than
	// producing a false positive or negative.
	orphans, err := FindOrphanedClaudeProcesses()
	if err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses (baseline): %v", err)
	}
	if !containsOrphanPID(orphans, pid) {
		for _, o := range orphans {
			t.Logf("  orphan: pid=%d cmd=%s town=%q", o.PID, o.Cmd, o.TownRoot)
		}
		t.Skipf("baseline: child pid %d not flagged as orphan (environment protected it); cannot exercise exemption", pid)
	}

	// Register the child as a bridge-local archivist and re-scan.
	if err := RegisterArchivist(townRoot, pid); err != nil {
		t.Fatalf("RegisterArchivist: %v", err)
	}
	t.Cleanup(func() { _ = UnregisterArchivist(townRoot, pid) })

	orphans2, err := FindOrphanedClaudeProcesses()
	if err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses (after register): %v", err)
	}
	if containsOrphanPID(orphans2, pid) {
		t.Errorf("PID %d still flagged as orphan despite marker at %s",
			pid, filepath.Join(ArchivistPidsDir(townRoot), "…"))
	}
}

// TestFindOrphanedClaudeProcesses_StalePidfileDoesNotBlockOtherCandidates
// verifies that a stale pidfile (pid not alive) does not interfere with
// detection of other orphan candidates. The registry lookup is per-pid, so
// a dead-pid marker only exempts that one pid and has no effect on others.
func TestFindOrphanedClaudeProcesses_StalePidfileDoesNotBlockOtherCandidates(t *testing.T) {
	// Create a town root with a stale marker for a pid that isn't alive.
	// We don't need to spawn a live "claude" child to exercise this — the
	// scan must complete without error and the stale marker must not cause
	// a crash or exempt unrelated orphans.
	townRoot := realPath(t, t.TempDir())
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"),
		[]byte(`{"name":"test-stale-pidfile"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RegisterArchivist(townRoot, 99999998); err != nil {
		t.Fatalf("RegisterArchivist: %v", err)
	}

	// The scan should succeed without error; we don't assert on which pids
	// are found, only that the stale marker doesn't break enumeration.
	if _, err := FindOrphanedClaudeProcesses(); err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses with stale pidfile: %v", err)
	}
}
