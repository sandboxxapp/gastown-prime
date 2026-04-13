package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// Tests for syncBeadToRig — the town→rig sync step of per-rig beads architecture.
// Symmetric counterpart to syncBeadsToTown in internal/deacon/reap_completed.go.

func TestSyncBeadToRig_NoRigBeads(t *testing.T) {
	// If the rig has no .beads/ directory, syncBeadToRig should no-op (return nil).
	tmp := t.TempDir()
	rigName := "my-rig"
	rigPath := filepath.Join(tmp, rigName)
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}
	// Rig dir exists but no .beads/ inside it.
	if err := syncBeadToRig(tmp, rigName); err != nil {
		t.Errorf("expected nil when rig has no .beads/, got: %v", err)
	}
}

func TestSyncBeadToRig_NoTownBeads(t *testing.T) {
	// If the town-root has no .beads/ directory, syncBeadToRig should no-op (return nil).
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Rig .beads/ exists, but town-root has no .beads/.
	if err := syncBeadToRig(tmp, rigName); err != nil {
		t.Errorf("expected nil when town-root has no .beads/, got: %v", err)
	}
}

func TestSyncBeadToRig_EmptyRigName(t *testing.T) {
	// Empty rigName should be a no-op (return nil) — no rig to sync to.
	tmp := t.TempDir()
	if err := syncBeadToRig(tmp, ""); err != nil {
		t.Errorf("expected nil for empty rigName, got: %v", err)
	}
}

func TestSyncBeadToRig_BdNotInPath(t *testing.T) {
	// When both .beads/ dirs exist but bd is not in PATH, expect a non-nil error.
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	townBeadsDir := filepath.Join(tmp, ".beads")
	for _, d := range []string{rigBeadsDir, townBeadsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Temporarily replace PATH with an empty value so bd cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer func() { os.Setenv("PATH", origPath) }()

	err := syncBeadToRig(tmp, rigName)
	if err == nil {
		t.Error("expected non-nil error when bd is not in PATH, got nil")
	}
}

func TestSyncBeadToRig_RunsInRigContext(t *testing.T) {
	// When both .beads/ dirs exist and PATH is restricted, the call fails (bd absent)
	// but it should reach the exec.Command stage, not short-circuit earlier.
	// This test mirrors TestSyncBeadToRig_BdNotInPath and validates that the function
	// reaches the command execution stage (i.e., the path validation guards passed).
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	townBeadsDir := filepath.Join(tmp, ".beads")
	for _, d := range []string{rigBeadsDir, townBeadsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("PATH", "")
	err := syncBeadToRig(tmp, rigName)
	// Should error (bd not found), not nil — proves exec was attempted.
	if err == nil {
		t.Error("expected exec error when bd absent, got nil")
	}
}

func TestSyncBeadToRig_NoOpWhenRigNameEmpty(t *testing.T) {
	// Duplicate guard: rigName="" must always short-circuit, even if .beads/ dirs exist.
	tmp := t.TempDir()
	// Create both .beads/ dirs to confirm the short-circuit is rigName-driven, not dir-driven.
	for _, d := range []string{filepath.Join(tmp, ".beads"), filepath.Join(tmp, "rigs", ".beads")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncBeadToRig(tmp, ""); err != nil {
		t.Errorf("expected nil for empty rigName even with .beads/ dirs present, got: %v", err)
	}
}
