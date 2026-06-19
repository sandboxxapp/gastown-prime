package deacon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The deacon agent registry (.gastown/deacon/agents.jsonl) is an append-only
// JSONL feed written by the bridge's SubagentStart/SubagentStop hook
// (.claude/hooks/agent-registry.sh) and read by the crew board
// (scripts/rig-crew.sh). The crew board builds its "active agents" set by
// agent_id: a "start" line adds an entry, a "stop" line with the same agent_id
// removes it. A start with no matching stop is a permanent "active" ghost.
//
// Rig polecats spawned with prereg metadata get a start line carrying their
// name+rig, but when the deacon reaps one (kills the session / orphan claude)
// the polecat never fires its own SubagentStop — so the ghost lingers. The
// reaper closes the loop by emitting a stop line for the reaped polecat's
// agent_id(s) as part of the reap (sbx-gastown-wwmgc).

// registryNow returns the timestamp stamped on emitted stop events. It is a var
// so tests can pin it; production always uses the real clock. The format mirrors
// the hook's `date -u +%Y-%m-%dT%H:%M:%SZ`.
var registryNow = func() time.Time { return time.Now().UTC() }

const registryTimeFormat = "2006-01-02T15:04:05Z"

// agentRegistryEvent mirrors one line of agents.jsonl. Only the fields the
// reaper needs to match and emit are modeled; unknown fields are ignored.
type agentRegistryEvent struct {
	Event     string `json:"event"`
	AgentID   string `json:"agent_id"`
	Name      string `json:"name,omitempty"`
	Rig       string `json:"rig,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// agentRegistryPath returns the path to the deacon agent registry for a town.
// Mirrors scripts/rig-crew.sh: <townRoot>/.gastown/deacon/agents.jsonl.
func agentRegistryPath(townRoot string) string {
	return filepath.Join(townRoot, ".gastown", "deacon", "agents.jsonl")
}

// activeRegistryAgentIDsFor scans the registry and returns the agent_id(s) of
// still-active (start-without-stop) entries that belong to the given rig+polecat
// — matched on the registry's name and rig fields exactly as the crew board
// resolves them. The match is strict: both name and rig must be non-empty and
// equal, so generic Agent-tool subagents (name="" rig="bridge") are never
// touched. Order is stable (first-seen) for deterministic journaling/tests.
//
// A missing registry file is not an error (returns no IDs) — the hook may never
// have run on this host.
func activeRegistryAgentIDsFor(townRoot, rig, polecat string) ([]string, error) {
	if rig == "" || polecat == "" {
		return nil, nil
	}
	path := agentRegistryPath(townRoot)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening agent registry: %w", err)
	}
	defer f.Close()

	// Track active entries by agent_id and remember which ones match this
	// polecat, preserving first-seen order.
	matched := make(map[string]bool)
	var order []string
	active := make(map[string]bool)

	sc := bufio.NewScanner(f)
	// Registry lines can be long once metadata accumulates; raise the cap.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev agentRegistryEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // skip malformed lines, same as the crew board
		}
		if ev.AgentID == "" {
			continue
		}
		switch ev.Event {
		case "start":
			active[ev.AgentID] = true
			if ev.Name == polecat && ev.Rig == rig {
				if !matched[ev.AgentID] {
					order = append(order, ev.AgentID)
				}
				matched[ev.AgentID] = true
			}
		case "stop":
			delete(active, ev.AgentID)
			// A re-used agent_id that later stopped is no longer a ghost.
			matched[ev.AgentID] = false
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scanning agent registry: %w", err)
	}

	var ids []string
	for _, id := range order {
		if active[id] && matched[id] {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// emitRegistryStops clears crew-board ghosts for a reaped polecat by appending a
// "stop" line for each of its still-active registry agent_id(s). It returns the
// agent_id(s) it stopped (empty if none matched). The operation is idempotent:
// once a stop is appended the entry is no longer active, so a subsequent reap
// pass finds nothing to do.
//
// Best-effort by contract — a registry that can't be read or appended must not
// fail the reap, so callers log the error and continue.
func emitRegistryStops(townRoot, rig, polecat string) ([]string, error) {
	ids, err := activeRegistryAgentIDsFor(townRoot, rig, polecat)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	path := agentRegistryPath(townRoot)
	// O_APPEND keeps each write atomic against the concurrently-appending hook.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening agent registry for append: %w", err)
	}
	defer f.Close()

	ts := registryNow().Format(registryTimeFormat)
	for _, id := range ids {
		entry := agentRegistryEvent{Event: "stop", AgentID: id, Timestamp: ts}
		b, err := json.Marshal(entry)
		if err != nil {
			return ids, fmt.Errorf("marshaling stop event: %w", err)
		}
		b = append(b, '\n')
		if _, err := f.Write(b); err != nil {
			return ids, fmt.Errorf("appending stop event: %w", err)
		}
	}
	return ids, nil
}
