package session

import "testing"

// TestPolecatProcessMarker pins the strict argv marker format so it stays in
// lockstep with the startup beacon the reaper matches against.
func TestPolecatProcessMarker(t *testing.T) {
	got := PolecatProcessMarker("gastown-prime", "charlock")
	want := "[GAS TOWN] polecat charlock (rig: gastown-prime)"
	if got != want {
		t.Errorf("PolecatProcessMarker = %q, want %q", got, want)
	}
}

// TestPolecatProcessAlive exercises the liveness gate through the injectable
// scanProcessesByMarker seam without spawning real processes.
func TestPolecatProcessAlive(t *testing.T) {
	orig := scanProcessesByMarker
	defer func() { scanProcessesByMarker = orig }()

	var lastMarker string

	// A matching process exists.
	scanProcessesByMarker = func(marker string) []string {
		lastMarker = marker
		return []string{"4242"}
	}
	if !PolecatProcessAlive("gastown-prime", "charlock") {
		t.Errorf("expected alive when a marked process is present")
	}
	if lastMarker != "[GAS TOWN] polecat charlock (rig: gastown-prime)" {
		t.Errorf("scanned with wrong marker: %q", lastMarker)
	}

	// No matching process.
	scanProcessesByMarker = func(string) []string { return nil }
	if PolecatProcessAlive("gastown-prime", "charlock") {
		t.Errorf("expected not-alive when no marked process is present")
	}

	// Nil scanner (e.g. unwired platform) must be safe and report not-alive.
	scanProcessesByMarker = nil
	if PolecatProcessAlive("gastown-prime", "charlock") {
		t.Errorf("expected not-alive with nil scanner")
	}
}
