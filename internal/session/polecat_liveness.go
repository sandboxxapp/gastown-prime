package session

// scanProcessesByMarker returns the PIDs of live processes whose full command
// line contains marker (an exact substring match). It is wired per-platform in
// polecat_liveness_unix.go / polecat_liveness_windows.go, and is a var so tests
// can substitute a fake without spawning real processes.
var scanProcessesByMarker func(marker string) []string

// PolecatProcessMarker returns the strict command-line marker that identifies a
// polecat's agent process: "[GAS TOWN] polecat <name> (rig: <rig>)". It mirrors
// the startup beacon (the "[GAS TOWN]" banner + BeaconRecipient, see
// startup.go) so the process can be found by a STRICT substring match. The
// marker embeds both name and rig, so it uniquely identifies one polecat — a
// loose "polecat"/"GAS TOWN" match could collide with an unrelated process.
//
// This is the single source of truth for the marker, shared by the deacon
// reaper's orphan kill (deacon.killOrphanedPolecatProcess) and the namepool's
// settled-name allocation gate (sbx-gastown-gsyki).
func PolecatProcessMarker(rig, name string) string {
	return "[GAS TOWN] " + BeaconRecipient("polecat", name, rig)
}

// PolecatProcessAlive reports whether a live agent process exists for the
// (rig, name) polecat, matched by the strict argv marker. Unlike a tmux-session
// check, this signal survives session death and worktree removal: a claude that
// called setsid()/reparented to init after gt exit stays visible here until it
// is fully reaped (sbx-gastown-xpuv, sbx-gastown-2bq4h). The namepool uses it to
// avoid reallocating a name whose prior occupant is still settling — the real
// stranding window is "directory gone AND session gone, process still alive"
// (sbx-gastown-gsyki).
func PolecatProcessAlive(rig, name string) bool {
	if scanProcessesByMarker == nil {
		return false
	}
	return len(scanProcessesByMarker(PolecatProcessMarker(rig, name))) > 0
}
