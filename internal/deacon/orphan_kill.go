package deacon

import (
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/session"
)

// defaultPolecatKillGrace is the SIGTERM→SIGKILL grace window for an orphaned
// polecat claude process, per the reaper spec (sbx-gastown-2bq4h): SIGTERM,
// wait 5s, SIGKILL if still alive.
const defaultPolecatKillGrace = 5 * time.Second

// polecatKillGrace is the live grace window. A var (not const) so tests can
// shrink it to keep them fast.
var polecatKillGrace = defaultPolecatKillGrace

// Overridable seams for the OS-level process operations. Defaults are wired in
// the platform-specific files (orphan_kill_unix.go / orphan_kill_windows.go).
// Tests substitute fakes so the kill logic can be exercised without real
// processes.
var (
	// listPolecatProcessesFn returns the PIDs of live processes whose command
	// line contains the strict marker (exact substring match).
	listPolecatProcessesFn func(marker string) []string
	// terminateProcessFn sends SIGTERM to pid.
	terminateProcessFn func(pid string)
	// killProcessFn sends SIGKILL to pid.
	killProcessFn func(pid string)
)

// orphanKillResult reports what the strict-marker claude kill did, for
// journaling in the reap event.
type orphanKillResult struct {
	// PID is the matched claude PID(s), comma-joined (empty if none found).
	PID string
	// Signaled is the strongest signal sent: "TERM", "KILL", or "none".
	Signaled string
	// Alive reports whether a matching process survived even SIGKILL.
	Alive bool
}

// polecatProcessMarker returns the exact command-line marker that identifies a
// polecat's claude process: "[GAS TOWN] polecat <name> (rig: <rig>)". This
// mirrors the startup beacon (session.BeaconRecipient + the "[GAS TOWN]"
// banner, internal/session/startup.go) so the reaper can find the process by a
// STRICT substring match. The marker embeds both name and rig, so it uniquely
// identifies one polecat — never use a loose "polecat"/"GAS TOWN" grep, which
// could collide with and kill an unrelated process.
func polecatProcessMarker(rig, polecat string) string {
	return "[GAS TOWN] " + session.BeaconRecipient("polecat", polecat, rig)
}

// killOrphanedPolecatProcess finds a polecat's claude process by strict marker
// match and terminates it: SIGTERM, poll up to polecatKillGrace for a graceful
// exit, then SIGKILL any survivor. It is safe to call when nothing matches
// (returns Signaled="none") and is the belt-and-suspenders that catches a
// claude which called setsid() and reparented to init, escaping the tmux
// pane-tree kill (evidence: PIDs survived tmux kill, sbx-gastown-2bq4h).
func killOrphanedPolecatProcess(rig, polecat string) orphanKillResult {
	if listPolecatProcessesFn == nil {
		return orphanKillResult{Signaled: "none"}
	}
	marker := polecatProcessMarker(rig, polecat)

	pids := listPolecatProcessesFn(marker)
	if len(pids) == 0 {
		return orphanKillResult{Signaled: "none"}
	}

	res := orphanKillResult{PID: strings.Join(pids, ","), Signaled: "TERM"}
	for _, pid := range pids {
		terminateProcessFn(pid)
	}

	// Poll up to the grace window; bail the moment the marker is gone so a
	// well-behaved process doesn't cost the reaper the full grace period.
	const step = 250 * time.Millisecond
	for waited := time.Duration(0); waited < polecatKillGrace; waited += step {
		if len(listPolecatProcessesFn(marker)) == 0 {
			return res // graceful SIGTERM worked
		}
		time.Sleep(step)
	}

	// Still alive after the grace window — escalate to SIGKILL.
	res.Signaled = "KILL"
	for _, pid := range listPolecatProcessesFn(marker) {
		killProcessFn(pid)
	}
	time.Sleep(step)
	res.Alive = len(listPolecatProcessesFn(marker)) > 0
	return res
}
