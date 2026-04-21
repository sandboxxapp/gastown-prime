package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTown creates a town root with mayor/town.json for a FindRigFromCwd test.
func setupTown(t *testing.T) string {
	t.Helper()
	root := realPath(t, t.TempDir())
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	return root
}

// writeRigMarker creates <dir>/rig.json so FindRigFromCwd treats dir as a rig.
func writeRigMarker(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}
	content := `{"name":"` + name + `"}`
	if err := os.WriteFile(filepath.Join(dir, RigMarker), []byte(content), 0o644); err != nil {
		t.Fatalf("write rig.json: %v", err)
	}
}

// TestFindRigFromCwd_RigsSubdir covers the primary bug: from
// <townRoot>/rigs/<rig>/ the detector must return <rig>, not "rigs".
func TestFindRigFromCwd_RigsSubdir(t *testing.T) {
	root := setupTown(t)
	rigDir := filepath.Join(root, "rigs", "sandboxx-backend")
	writeRigMarker(t, rigDir, "sandboxx-backend")

	name, path := FindRigFromCwd(rigDir, root)
	if name != "sandboxx-backend" {
		t.Errorf("name = %q, want %q", name, "sandboxx-backend")
	}
	if path != rigDir {
		t.Errorf("path = %q, want %q", path, rigDir)
	}
}

// TestFindRigFromCwd_NestedInsideRig verifies that cwd deep inside a rig
// (e.g. a polecat worktree) walks up to the rig.json marker.
func TestFindRigFromCwd_NestedInsideRig(t *testing.T) {
	root := setupTown(t)
	rigDir := filepath.Join(root, "rigs", "waypoints-api")
	writeRigMarker(t, rigDir, "waypoints-api")

	nested := filepath.Join(rigDir, "polecats", "worker", "repo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	name, path := FindRigFromCwd(nested, root)
	if name != "waypoints-api" {
		t.Errorf("name = %q, want %q", name, "waypoints-api")
	}
	if path != rigDir {
		t.Errorf("path = %q, want %q", path, rigDir)
	}
}

// TestFindRigFromCwd_LegacyTopLevel covers the legacy layout where rigs live
// directly under townRoot without a rig.json marker. The first path component
// is the rig name.
func TestFindRigFromCwd_LegacyTopLevel(t *testing.T) {
	root := setupTown(t)
	rigDir := filepath.Join(root, "legacy-rig")
	if err := os.MkdirAll(rigDir, 0o755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}

	name, path := FindRigFromCwd(rigDir, root)
	if name != "legacy-rig" {
		t.Errorf("name = %q, want %q", name, "legacy-rig")
	}
	if path != rigDir {
		t.Errorf("path = %q, want %q", path, rigDir)
	}
}

// TestFindRigFromCwd_RigsContainer verifies that the "rigs/" container
// directory itself is not treated as a rig.
func TestFindRigFromCwd_RigsContainer(t *testing.T) {
	root := setupTown(t)
	rigsDir := filepath.Join(root, "rigs")
	if err := os.MkdirAll(rigsDir, 0o755); err != nil {
		t.Fatalf("mkdir rigs: %v", err)
	}

	name, path := FindRigFromCwd(rigsDir, root)
	if name != "" || path != "" {
		t.Errorf("FindRigFromCwd(rigs/) = (%q, %q), want empty", name, path)
	}
}

// TestFindRigFromCwd_TownRoot verifies cwd==townRoot returns empty.
func TestFindRigFromCwd_TownRoot(t *testing.T) {
	root := setupTown(t)

	name, path := FindRigFromCwd(root, root)
	if name != "" || path != "" {
		t.Errorf("FindRigFromCwd(townRoot) = (%q, %q), want empty", name, path)
	}
}

// TestFindRigFromCwd_OutsideTown returns empty when cwd is outside townRoot.
func TestFindRigFromCwd_OutsideTown(t *testing.T) {
	root := setupTown(t)
	outside := realPath(t, t.TempDir())

	name, path := FindRigFromCwd(outside, root)
	if name != "" || path != "" {
		t.Errorf("FindRigFromCwd(outside) = (%q, %q), want empty", name, path)
	}
}

// TestFindRigFromCwd_MarkerWinsOverPath verifies that a rig.json marker at an
// ancestor is preferred over path-based parsing. If the user is in
// <townRoot>/rigs/<rig>/deep/subdir/, we return <rig> via marker — which is
// the fix this test guards against regressing.
func TestFindRigFromCwd_MarkerWinsOverPath(t *testing.T) {
	root := setupTown(t)
	rigDir := filepath.Join(root, "rigs", "mocker")
	writeRigMarker(t, rigDir, "mocker")

	deep := filepath.Join(rigDir, "internal", "pkg", "foo")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	name, path := FindRigFromCwd(deep, root)
	if name != "mocker" {
		t.Errorf("name = %q, want %q", name, "mocker")
	}
	if path != rigDir {
		t.Errorf("path = %q, want %q", path, rigDir)
	}
}
