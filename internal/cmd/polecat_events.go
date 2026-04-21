package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/util"
)

// polecatEventsMaxLines is the line-count cap for daemon/polecat-events.jsonl.
// Past this cap the file is truncated from the top so the tail is preserved.
const polecatEventsMaxLines = 500

// polecatExitEvent is one line in daemon/polecat-events.jsonl. It captures
// a single `gt exit` invocation so the town deacon can wake on polecat
// lifecycle instead of polling a stale graphs snapshot.
type polecatExitEvent struct {
	TS      string `json:"ts"`
	Rig     string `json:"rig,omitempty"`
	Polecat string `json:"polecat,omitempty"`
	Bead    string `json:"bead,omitempty"`
	Event   string `json:"event"`
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

// polecatEventsPath returns the town-scoped wake signal file path.
func polecatEventsPath(townRoot string) string {
	return filepath.Join(townRoot, "daemon", "polecat-events.jsonl")
}

// writePolecatExitEvent appends ev to the town wake signal file, creating
// the parent directory if needed, and then rotates the file if it exceeds
// polecatEventsMaxLines. mtime advances on every successful append so the
// deacon's poll loop detects wake activity.
func writePolecatExitEvent(townRoot string, ev polecatExitEvent) error {
	path := polecatEventsPath(townRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if _, werr := f.Write(append(line, '\n')); werr != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, werr)
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("close %s: %w", path, cerr)
	}
	return rotatePolecatEventsFile(path, polecatEventsMaxLines)
}

// rotatePolecatEventsFile truncates path to its last maxLines lines when the
// file exceeds the cap. The rewrite is atomic via a temp file + rename so a
// concurrent reader never sees a partially-written file.
func rotatePolecatEventsFile(path string, maxLines int) error {
	if maxLines <= 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= maxLines {
		return nil
	}
	tail := lines[len(lines)-maxLines:]
	out := strings.Join(tail, "\n") + "\n"

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %s → %s: %w", tmpPath, path, err)
	}
	return nil
}

// newPolecatExitEvent builds the event struct with a UTC RFC3339 timestamp.
func newPolecatExitEvent(rig, polecat, bead string, success bool, reason string) polecatExitEvent {
	return polecatExitEvent{
		TS:      time.Now().UTC().Format(time.RFC3339),
		Rig:     rig,
		Polecat: polecat,
		Bead:    bead,
		Event:   "exit",
		Success: success,
		Reason:  reason,
	}
}

// signalDeaconOfExit runs `gt nudge deacon` to wake the sleeping deacon
// immediately. The wake signal file is primary; this nudge is redundant
// insurance since the deacon sleeps long for token conservation.
func signalDeaconOfExit(townRoot, bead string) {
	msg := "polecat exit"
	if bead != "" {
		msg = fmt.Sprintf("polecat exit: %s", bead)
	}
	nudgeCmd := exec.Command("gt", "nudge", "deacon", "-m", msg)
	nudgeCmd.Dir = townRoot
	nudgeCmd.Env = os.Environ()
	util.SetDetachedProcessGroup(nudgeCmd)
	if err := nudgeCmd.Run(); err != nil {
		// Non-fatal: the signal file is the primary wake mechanism.
		style.PrintWarning("could not nudge deacon: %v", err)
	}
}

// escalatePolecatExitFailure pages the mayor via `gt escalate -s HIGH` so
// failed exits are visible even if the deacon misses the signal file.
func escalatePolecatExitFailure(townRoot, polecat, reason string) {
	who := polecat
	if who == "" {
		who = "(unknown)"
	}
	desc := fmt.Sprintf("Polecat %s exit failed: %s", who, reason)
	escCmd := exec.Command("gt", "escalate", desc, "-s", "HIGH")
	escCmd.Dir = townRoot
	escCmd.Env = os.Environ()
	util.SetDetachedProcessGroup(escCmd)
	if err := escCmd.Run(); err != nil {
		style.PrintWarning("could not escalate exit failure: %v", err)
	}
}
