package daemon

import (
	"log"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/steveyegge/gastown/internal/wisp"
)

// TestIsRigOperational_FlatBeadNamespace_SkipsBeadLookup verifies that when
// flatBeadNamespace is true, isRigOperational skips the Dolt rig-bead lookup
// and returns true based on wisp config alone. This prevents the fail-safe
// from excluding all rigs when rig beads don't exist in a flat namespace.
func TestIsRigOperational_FlatBeadNamespace_SkipsBeadLookup(t *testing.T) {
	townRoot := t.TempDir()

	// No Dolt, no rig beads — in the old code this would fail-safe to false.
	d := &Daemon{
		config:            &Config{TownRoot: townRoot},
		logger:            log.New(os.Stderr, "[test] ", 0),
		flatBeadNamespace: true,
	}

	// Create minimal rig directory so wisp config path resolves
	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		t.Fatalf("mkdir rig dir: %v", err)
	}

	operational, reason := d.isRigOperational("myrig")
	if !operational {
		t.Fatalf("isRigOperational() = false (%s), want true (flat namespace should skip bead lookup)", reason)
	}
}

// TestIsRigOperational_FlatBeadNamespace_StillRespectsWispParked verifies that
// even with flatBeadNamespace, wisp-level parked/docked status is still checked.
func TestIsRigOperational_FlatBeadNamespace_StillRespectsWispParked(t *testing.T) {
	townRoot := t.TempDir()

	d := &Daemon{
		config:            &Config{TownRoot: townRoot},
		logger:            log.New(os.Stderr, "[test] ", 0),
		flatBeadNamespace: true,
	}

	// Park the rig via wisp config
	if err := wisp.NewConfig(townRoot, "myrig").Set("status", "parked"); err != nil {
		t.Fatalf("set parked: %v", err)
	}

	operational, reason := d.isRigOperational("myrig")
	if operational {
		t.Fatal("isRigOperational() = true, want false (wisp parked should still block)")
	}
	if reason != "rig is parked" {
		t.Fatalf("reason = %q, want %q", reason, "rig is parked")
	}
}

// TestGetPatrolRigs_FlatBeadNamespace_IncludesRigsWithoutDolt verifies that
// getPatrolRigs returns operational rigs when flatBeadNamespace is true,
// even without Dolt connectivity.
func TestGetPatrolRigs_FlatBeadNamespace_IncludesRigsWithoutDolt(t *testing.T) {
	townRoot := t.TempDir()

	// Seed known rigs
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("mkdir mayor dir: %v", err)
	}
	rigsJSON := `{"rigs":{"alpha":{},"beta":{},"gamma":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0o644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	// Park beta via wisp
	if err := wisp.NewConfig(townRoot, "beta").Set("status", "parked"); err != nil {
		t.Fatalf("set beta parked: %v", err)
	}

	d := &Daemon{
		config:            &Config{TownRoot: townRoot},
		logger:            log.New(os.Stderr, "[test] ", 0),
		flatBeadNamespace: true,
	}

	got := d.getPatrolRigs("witness")
	slices.Sort(got)
	// alpha and gamma should be included (not parked, no bead lookup needed)
	// beta should be excluded (parked via wisp)
	want := []string{"alpha", "gamma"}
	if !slices.Equal(got, want) {
		t.Fatalf("getPatrolRigs() = %v, want %v", got, want)
	}
}
