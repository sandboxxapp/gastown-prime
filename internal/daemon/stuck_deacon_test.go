package daemon

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/tmux"
)

// Regression test for sbx-gastown-6ayz (cherry-pick of upstream e4fac780):
// when the Deacon heartbeat is very stale (>= 20 min) and the agent is NOT
// in a crash loop, checkDeaconHeartbeat must invoke restartStuckDeacon,
// which kills the tmux session. Before the cherry-pick the call sites
// only logged "Detection only" and never killed the session — the bug
// behind the 40-hour outage in sbx-gastown-853x.
func TestCheckDeaconHeartbeat_KillsVeryStaleSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — fake tmux requires bash")
	}
	townRoot := t.TempDir()
	fakeBinDir := t.TempDir()
	tmuxLog := filepath.Join(t.TempDir(), "tmux.log")
	if err := os.WriteFile(tmuxLog, []byte{}, 0o644); err != nil {
		t.Fatalf("create tmux log: %v", err)
	}

	writeFakeTmuxCrashLoop(t, fakeBinDir)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX_LOG", tmuxLog)

	// Heartbeat is 25 min stale (>= 20 min triggers very-stale tier).
	if err := deacon.WriteHeartbeat(townRoot, &deacon.Heartbeat{
		Timestamp: time.Now().Add(-25 * time.Minute),
		Cycle:     1,
	}); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	rt := NewRestartTracker(townRoot, RestartTrackerConfig{})

	d := &Daemon{
		config:         &Config{TownRoot: townRoot},
		ctx:            context.Background(),
		logger:         log.New(io.Discard, "", 0),
		tmux:           tmux.NewTmux(),
		restartTracker: rt,
	}

	d.checkDeaconHeartbeat()

	data, err := os.ReadFile(tmuxLog)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}

	kills := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "kill-session ") {
			kills++
		}
	}
	if kills == 0 {
		t.Fatalf("expected at least 1 kill-session call for very-stale heartbeat, got 0.\nlog:\n%s", string(data))
	}
}

// Regression test for sbx-gastown-6ayz: restartStuckDeacon must respect
// the RestartTracker backoff window. When the agent is in backoff, no
// kill should happen — exponential backoff would otherwise be defeated.
func TestRestartStuckDeacon_RespectsBackoff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — fake tmux requires bash")
	}
	townRoot := t.TempDir()
	fakeBinDir := t.TempDir()
	tmuxLog := filepath.Join(t.TempDir(), "tmux.log")
	if err := os.WriteFile(tmuxLog, []byte{}, 0o644); err != nil {
		t.Fatalf("create tmux log: %v", err)
	}

	writeFakeTmuxCrashLoop(t, fakeBinDir)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX_LOG", tmuxLog)

	rt := NewRestartTracker(townRoot, RestartTrackerConfig{})
	rt.state.Agents["deacon"] = &AgentRestartInfo{
		LastRestart:  time.Now(),
		RestartCount: 1,
		BackoffUntil: time.Now().Add(1 * time.Hour), // still in backoff
	}

	d := &Daemon{
		config:         &Config{TownRoot: townRoot},
		ctx:            context.Background(),
		logger:         log.New(io.Discard, "", 0),
		tmux:           tmux.NewTmux(),
		restartTracker: rt,
	}

	d.restartStuckDeacon("hq-deacon", "test-backoff")

	data, _ := os.ReadFile(tmuxLog)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "kill-session ") {
			t.Fatalf("backoff-gated restart still killed session.\nlog:\n%s", string(data))
		}
	}
}
