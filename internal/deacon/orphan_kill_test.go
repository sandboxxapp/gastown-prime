package deacon

import (
	"sync"
	"testing"
	"time"
)

func TestPolecatProcessMarker(t *testing.T) {
	got := polecatProcessMarker("gastown-prime", "brin2")
	want := "[GAS TOWN] polecat brin2 (rig: gastown-prime)"
	if got != want {
		t.Errorf("marker = %q, want %q", got, want)
	}
}

// fakeProcTable simulates the OS process table for the kill seams. Each call to
// list returns the current PIDs; terminate/kill mutate the table per the
// configured behavior.
type fakeProcTable struct {
	mu        sync.Mutex
	pids      []string
	diesOnTRM bool // process exits on SIGTERM
	diesOnKIL bool // process exits on SIGKILL
	termCount int
	killCount int
}

func (f *fakeProcTable) install(t *testing.T) {
	t.Helper()
	origList, origTerm, origKill, origGrace := listPolecatProcessesFn, terminateProcessFn, killProcessFn, polecatKillGrace
	t.Cleanup(func() {
		listPolecatProcessesFn, terminateProcessFn, killProcessFn, polecatKillGrace = origList, origTerm, origKill, origGrace
	})
	polecatKillGrace = 600 * time.Millisecond // keep the poll loop fast
	listPolecatProcessesFn = func(string) []string {
		f.mu.Lock()
		defer f.mu.Unlock()
		return append([]string(nil), f.pids...)
	}
	terminateProcessFn = func(string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.termCount++
		if f.diesOnTRM {
			f.pids = nil
		}
	}
	killProcessFn = func(string) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.killCount++
		if f.diesOnKIL {
			f.pids = nil
		}
	}
}

func TestKillOrphan_NoMatch(t *testing.T) {
	f := &fakeProcTable{pids: nil}
	f.install(t)

	res := killOrphanedPolecatProcess("gastown-prime", "brin2")
	if res.Signaled != "none" {
		t.Errorf("Signaled = %q, want none", res.Signaled)
	}
	if res.PID != "" || res.Alive {
		t.Errorf("unexpected result %+v", res)
	}
	if f.termCount != 0 || f.killCount != 0 {
		t.Errorf("should not signal when nothing matches (term=%d kill=%d)", f.termCount, f.killCount)
	}
}

func TestKillOrphan_GracefulTerm(t *testing.T) {
	f := &fakeProcTable{pids: []string{"4242"}, diesOnTRM: true}
	f.install(t)

	res := killOrphanedPolecatProcess("gastown-prime", "brin2")
	if res.Signaled != "TERM" {
		t.Errorf("Signaled = %q, want TERM", res.Signaled)
	}
	if res.PID != "4242" {
		t.Errorf("PID = %q, want 4242", res.PID)
	}
	if res.Alive {
		t.Error("process should be reported dead after graceful TERM")
	}
	if f.killCount != 0 {
		t.Errorf("SIGKILL should not be sent when TERM works (kill=%d)", f.killCount)
	}
}

func TestKillOrphan_EscalatesToKill(t *testing.T) {
	f := &fakeProcTable{pids: []string{"4242"}, diesOnTRM: false, diesOnKIL: true}
	f.install(t)

	res := killOrphanedPolecatProcess("gastown-prime", "brin2")
	if res.Signaled != "KILL" {
		t.Errorf("Signaled = %q, want KILL", res.Signaled)
	}
	if res.Alive {
		t.Error("process should be dead after SIGKILL")
	}
	if f.termCount == 0 || f.killCount == 0 {
		t.Errorf("expected both TERM and KILL (term=%d kill=%d)", f.termCount, f.killCount)
	}
}

func TestKillOrphan_SurvivesKill(t *testing.T) {
	f := &fakeProcTable{pids: []string{"4242"}, diesOnTRM: false, diesOnKIL: false}
	f.install(t)

	res := killOrphanedPolecatProcess("gastown-prime", "brin2")
	if res.Signaled != "KILL" {
		t.Errorf("Signaled = %q, want KILL", res.Signaled)
	}
	if !res.Alive {
		t.Error("process that survives SIGKILL must be reported Alive for journaling")
	}
}

func TestReapEventPayload_JournalsClaudeFields(t *testing.T) {
	r := &ReapResult{
		WorktreeRemoved: true,
		ClaudePID:       "4242",
		ClaudeSignaled:  "KILL",
		PostRemoveAlive: true,
	}
	p := reapEventPayload("gastown-prime", "brin2", "sbx-gastown-2bq4h", true, r)

	if p["claude_pid"] != "4242" {
		t.Errorf("claude_pid = %v, want 4242", p["claude_pid"])
	}
	if p["claude_signaled"] != "KILL" {
		t.Errorf("claude_signaled = %v, want KILL", p["claude_signaled"])
	}
	if p["post_remove_alive"] != true {
		t.Errorf("post_remove_alive = %v, want true", p["post_remove_alive"])
	}
}

func TestReapEventPayload_OmitsEmptyClaudeFields(t *testing.T) {
	r := &ReapResult{WorktreeRemoved: true, ClaudeSignaled: "none"}
	p := reapEventPayload("gastown-prime", "brin2", "", true, r)

	if _, ok := p["claude_pid"]; ok {
		t.Error("claude_pid should be omitted when no PID matched")
	}
	if p["claude_signaled"] != "none" {
		t.Errorf("claude_signaled = %v, want none recorded", p["claude_signaled"])
	}
	if _, ok := p["post_remove_alive"]; ok {
		t.Error("post_remove_alive should be omitted when false")
	}
}
