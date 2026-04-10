package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectSenderFromCwdUsesAgentFileWitnessIdentity(t *testing.T) {
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	tmp := t.TempDir()
	witnessDir := filepath.Join(tmp, "x267", "witness")
	if err := os.MkdirAll(filepath.Join(witnessDir, "rig"), 0o755); err != nil {
		t.Fatalf("mkdir witness dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(witnessDir, ".gt-agent"),
		[]byte(`{"role":"witness","rig":"x267"}`),
		0o644,
	); err != nil {
		t.Fatalf("write .gt-agent: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(filepath.Join(witnessDir, "rig")); err != nil {
		t.Fatalf("chdir witness rig dir: %v", err)
	}

	got := detectSender()
	if got != "x267/witness" {
		t.Fatalf("detectSender() = %q, want %q", got, "x267/witness")
	}
}

// TestFindLocalBeadsDir_PrefersWorkspaceTownRoot verifies that findLocalBeadsDir
// returns the town-root .beads/ when running from a nested rig directory that has
// its own .beads/. This is the core bug: the CWD walk-up finds <rig>/.beads/ first,
// but the correct answer is the outermost workspace root's .beads/.
//
// Regression test for sbx-gastown-7zvk.
func TestFindLocalBeadsDir_PrefersWorkspaceTownRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	// Clear env vars that could interfere
	t.Setenv("BEADS_DIR", "")
	t.Setenv("GT_TOWN_ROOT", "")
	t.Setenv("GT_ROOT", "")

	// Build nested layout:
	//   town/mayor/town.json   (town root marker)
	//   town/.beads/           (correct beads)
	//   town/rigs/myrig/mayor/town.json  (rig-level marker)
	//   town/rigs/myrig/.beads/          (wrong beads — the bug)
	town := t.TempDir()
	rigDir := filepath.Join(town, "rigs", "myrig")

	for _, dir := range []string{
		filepath.Join(town, "mayor"),
		filepath.Join(town, ".beads"),
		filepath.Join(rigDir, "mayor"),
		filepath.Join(rigDir, ".beads"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for _, f := range []string{
		filepath.Join(town, "mayor", "town.json"),
		filepath.Join(rigDir, "mayor", "town.json"),
	} {
		if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	// CWD is the rig directory — walk-up would find rig/.beads first
	if err := os.Chdir(rigDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := findLocalBeadsDir()
	if err != nil {
		t.Fatalf("findLocalBeadsDir() error: %v", err)
	}

	// Normalize for macOS /private/var symlink differences
	gotReal, _ := filepath.EvalSymlinks(got)
	wantReal, _ := filepath.EvalSymlinks(town)

	if gotReal != wantReal {
		t.Errorf("findLocalBeadsDir() = %q, want %q (should use town root, not rig)", got, town)
	}
}

// TestFindLocalBeadsDir_BeadsDirEnvTakesPriority verifies that BEADS_DIR env var
// still wins over workspace detection.
func TestFindLocalBeadsDir_BeadsDirEnvTakesPriority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	// Set up a custom beads dir via env var
	customBeads := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(customBeads, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("BEADS_DIR", customBeads)

	got, err := findLocalBeadsDir()
	if err != nil {
		t.Fatalf("findLocalBeadsDir() error: %v", err)
	}

	// BEADS_DIR points to .beads dir, function returns its parent
	gotReal, _ := filepath.EvalSymlinks(got)
	wantReal, _ := filepath.EvalSymlinks(filepath.Dir(customBeads))

	if gotReal != wantReal {
		t.Errorf("findLocalBeadsDir() = %q, want %q (BEADS_DIR should take priority)", got, filepath.Dir(customBeads))
	}
}

// TestFindLocalBeadsDir_FallbackWalkUpWhenNoWorkspace verifies that the CWD walk-up
// fallback still works when there are no workspace markers (no mayor/town.json).
func TestFindLocalBeadsDir_FallbackWalkUpWhenNoWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	t.Setenv("BEADS_DIR", "")
	t.Setenv("GT_TOWN_ROOT", "")
	t.Setenv("GT_ROOT", "")

	// Layout with no workspace markers, just .beads
	root := t.TempDir()
	beadsDir := filepath.Join(root, ".beads")
	subDir := filepath.Join(root, "sub", "deep")

	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := findLocalBeadsDir()
	if err != nil {
		t.Fatalf("findLocalBeadsDir() error: %v", err)
	}

	gotReal, _ := filepath.EvalSymlinks(got)
	wantReal, _ := filepath.EvalSymlinks(root)

	if gotReal != wantReal {
		t.Errorf("findLocalBeadsDir() = %q, want %q", got, root)
	}
}

func TestDetectSenderFromCwdUsesAgentFileRefineryIdentity(t *testing.T) {
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	tmp := t.TempDir()
	refineryDir := filepath.Join(tmp, "x267", "refinery")
	if err := os.MkdirAll(filepath.Join(refineryDir, "rig"), 0o755); err != nil {
		t.Fatalf("mkdir refinery dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(refineryDir, ".gt-agent"),
		[]byte(`{"role":"refinery","rig":"x267"}`),
		0o644,
	); err != nil {
		t.Fatalf("write .gt-agent: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(filepath.Join(refineryDir, "rig")); err != nil {
		t.Fatalf("chdir refinery rig dir: %v", err)
	}

	got := detectSender()
	if got != "x267/refinery" {
		t.Fatalf("detectSender() = %q, want %q", got, "x267/refinery")
	}
}
