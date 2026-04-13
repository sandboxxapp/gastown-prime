package daemon

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestReapCompletedPolecats_RecoversPanic verifies that the daemon's
// reapCompletedPolecats method recovers from panics in the scan path
// instead of crashing the entire daemon process.
//
// Root cause: the daemon's main select loop has no recover() wrapper,
// so a panic in any patrol handler (including the polecat reaper) kills
// the daemon. This test ensures the reaper catches panics and logs them.
func TestReapCompletedPolecats_RecoversPanic(t *testing.T) {
	// Create a minimal daemon with a logger we can inspect
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	d := &Daemon{
		config: &Config{
			TownRoot: "/nonexistent/town/root/that/will/cause/issues",
		},
		patrolConfig: DefaultLifecycleConfig(),
		logger:       logger,
	}

	// This should NOT panic — the reaper should recover from any internal panic
	// and log the error instead of crashing the daemon.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("reapCompletedPolecats panicked (should have recovered internally): %v", r)
			}
		}()
		d.reapCompletedPolecats()
	}()

	// Check that the reaper logged something (either a scan error or a recovery message)
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("expected reaper to log something (scan error or recovery), got empty log")
	}

	// Should NOT contain "panic" in an unrecovered sense — if it recovered,
	// the log should contain a structured error message
	if strings.Contains(logOutput, "runtime error") && !strings.Contains(logOutput, "recovered") {
		t.Errorf("log suggests unrecovered panic: %s", logOutput)
	}
}
