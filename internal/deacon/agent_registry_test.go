package deacon

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRegistry creates a town root with a .gastown/deacon/agents.jsonl
// containing the given raw JSONL lines and returns the town root.
func writeRegistry(t *testing.T, lines ...string) string {
	t.Helper()
	townRoot := t.TempDir()
	dir := filepath.Join(townRoot, ".gastown", "deacon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "agents.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return townRoot
}

// readRegistryEvents parses every line of the town's agents.jsonl.
func readRegistryEvents(t *testing.T, townRoot string) []agentRegistryEvent {
	t.Helper()
	f, err := os.Open(agentRegistryPath(townRoot))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer f.Close()
	var out []agentRegistryEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var ev agentRegistryEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal %q: %v", sc.Text(), err)
		}
		out = append(out, ev)
	}
	return out
}

func startLine(t *testing.T, aid, name, rig string) string {
	t.Helper()
	b, _ := json.Marshal(agentRegistryEvent{Event: "start", AgentID: aid, Name: name, Rig: rig, Timestamp: "2026-06-19T00:00:00Z"})
	return string(b)
}

func stopLine(t *testing.T, aid string) string {
	t.Helper()
	b, _ := json.Marshal(agentRegistryEvent{Event: "stop", AgentID: aid, Timestamp: "2026-06-19T00:01:00Z"})
	return string(b)
}

func TestActiveRegistryAgentIDsFor_MatchByNameAndRig(t *testing.T) {
	townRoot := writeRegistry(t,
		startLine(t, "a1", "brin2", "gastown-prime"),
		startLine(t, "a2", "other", "gastown-prime"), // wrong name
		startLine(t, "a3", "brin2", "other-rig"),     // wrong rig
		startLine(t, "a4", "", "bridge"),             // generic subagent
	)

	ids, err := activeRegistryAgentIDsFor(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "a1" {
		t.Errorf("ids = %v, want [a1]", ids)
	}
}

func TestActiveRegistryAgentIDsFor_StoppedNotGhost(t *testing.T) {
	townRoot := writeRegistry(t,
		startLine(t, "a1", "brin2", "gastown-prime"),
		stopLine(t, "a1"), // already stopped — not a ghost
	)
	ids, err := activeRegistryAgentIDsFor(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want none (already stopped)", ids)
	}
}

func TestActiveRegistryAgentIDsFor_MissingFileNoError(t *testing.T) {
	townRoot := t.TempDir() // no .gastown/deacon/agents.jsonl
	ids, err := activeRegistryAgentIDsFor(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
}

func TestActiveRegistryAgentIDsFor_EmptyNameNeverMatches(t *testing.T) {
	// A reaped polecat with an empty name must not match generic subagents.
	townRoot := writeRegistry(t, startLine(t, "a4", "", "bridge"))
	ids, err := activeRegistryAgentIDsFor(townRoot, "bridge", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want none (empty polecat name must not match)", ids)
	}
}

func TestActiveRegistryAgentIDsFor_SkipsMalformedLines(t *testing.T) {
	townRoot := writeRegistry(t,
		"{not json",
		startLine(t, "a1", "brin2", "gastown-prime"),
		"",
	)
	ids, err := activeRegistryAgentIDsFor(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "a1" {
		t.Errorf("ids = %v, want [a1]", ids)
	}
}

func TestEmitRegistryStops_AppendsStopForGhost(t *testing.T) {
	pinClock(t, time.Date(2026, 6, 19, 4, 35, 0, 0, time.UTC))
	townRoot := writeRegistry(t, startLine(t, "a1", "brin2", "gastown-prime"))

	ids, err := emitRegistryStops(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "a1" {
		t.Fatalf("ids = %v, want [a1]", ids)
	}

	events := readRegistryEvents(t, townRoot)
	if len(events) != 2 {
		t.Fatalf("expected 2 events (start+stop), got %d", len(events))
	}
	stop := events[1]
	if stop.Event != "stop" || stop.AgentID != "a1" {
		t.Errorf("stop event = %+v, want event=stop agent_id=a1", stop)
	}
	if stop.Timestamp != "2026-06-19T04:35:00Z" {
		t.Errorf("timestamp = %q, want pinned 2026-06-19T04:35:00Z", stop.Timestamp)
	}

	// The crew board must now treat the agent as inactive.
	left, _ := activeRegistryAgentIDsFor(townRoot, "gastown-prime", "brin2")
	if len(left) != 0 {
		t.Errorf("ghost not cleared, still active: %v", left)
	}
}

func TestEmitRegistryStops_Idempotent(t *testing.T) {
	pinClock(t, time.Date(2026, 6, 19, 4, 35, 0, 0, time.UTC))
	townRoot := writeRegistry(t, startLine(t, "a1", "brin2", "gastown-prime"))

	if _, err := emitRegistryStops(townRoot, "gastown-prime", "brin2"); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	ids, err := emitRegistryStops(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("second emit: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("second emit returned %v, want none (idempotent)", ids)
	}
	// Exactly one stop should have been appended across both calls.
	stops := 0
	for _, ev := range readRegistryEvents(t, townRoot) {
		if ev.Event == "stop" {
			stops++
		}
	}
	if stops != 1 {
		t.Errorf("appended %d stop events, want exactly 1", stops)
	}
}

func TestEmitRegistryStops_NoMatchNoWrite(t *testing.T) {
	townRoot := writeRegistry(t, startLine(t, "a1", "other", "gastown-prime"))
	before, _ := os.ReadFile(agentRegistryPath(townRoot))

	ids, err := emitRegistryStops(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ids = %v, want none", ids)
	}
	after, _ := os.ReadFile(agentRegistryPath(townRoot))
	if string(before) != string(after) {
		t.Error("registry should be untouched when nothing matches")
	}
}

func TestEmitRegistryStops_MissingRegistryNoError(t *testing.T) {
	townRoot := t.TempDir()
	ids, err := emitRegistryStops(townRoot, "gastown-prime", "brin2")
	if err != nil {
		t.Fatalf("missing registry should be a no-op, got %v", err)
	}
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
}

func TestClearCrewBoardGhost_RecordsStops(t *testing.T) {
	pinClock(t, time.Date(2026, 6, 19, 4, 35, 0, 0, time.UTC))
	townRoot := writeRegistry(t,
		startLine(t, "a1", "brin2", "gastown-prime"),
		startLine(t, "a2", "brin2", "gastown-prime"),
	)
	r := &ReapResult{}
	clearCrewBoardGhost(townRoot, "gastown-prime", "brin2", r)
	if r.RegistryStops != "a1,a2" {
		t.Errorf("RegistryStops = %q, want a1,a2", r.RegistryStops)
	}
}

func TestReapEventPayload_JournalsRegistryStops(t *testing.T) {
	r := &ReapResult{RegistryStops: "a1,a2", ClaudeSignaled: "none"}
	p := reapEventPayload("gastown-prime", "brin2", "", true, r)
	if p["registry_stops"] != "a1,a2" {
		t.Errorf("registry_stops = %v, want a1,a2", p["registry_stops"])
	}

	r2 := &ReapResult{ClaudeSignaled: "none"}
	p2 := reapEventPayload("gastown-prime", "brin2", "", true, r2)
	if _, ok := p2["registry_stops"]; ok {
		t.Error("registry_stops should be omitted when no stops were emitted")
	}
}

// pinClock pins registryNow to a fixed instant for the duration of the test.
func pinClock(t *testing.T, at time.Time) {
	t.Helper()
	orig := registryNow
	t.Cleanup(func() { registryNow = orig })
	registryNow = func() time.Time { return at }
}
