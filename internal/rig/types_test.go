package rig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBeadsPath_AlwaysReturnsRigRoot(t *testing.T) {
	t.Parallel()

	// BeadsPath should always return the rig root path, regardless of HasMayor.
	// The redirect system at <rig>/.beads/redirect handles finding the actual
	// beads location (either local at <rig>/.beads/ or tracked at mayor/rig/.beads/).
	//
	// This ensures:
	// 1. We don't write files to the user's repo clone (mayor/rig/)
	// 2. The redirect architecture is respected
	// 3. All code paths use the same beads resolution logic

	tests := []struct {
		name     string
		rig      Rig
		wantPath string
	}{
		{
			name: "rig with mayor only",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/gt/testrig",
				HasMayor: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with witness only",
			rig: Rig{
				Name:       "testrig",
				Path:       "/home/user/gt/testrig",
				HasWitness: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with refinery only",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/gt/testrig",
				HasRefinery: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with no agents",
			rig: Rig{
				Name: "testrig",
				Path: "/home/user/gt/testrig",
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with mayor and witness",
			rig: Rig{
				Name:       "testrig",
				Path:       "/home/user/gt/testrig",
				HasMayor:   true,
				HasWitness: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with mayor and refinery",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/gt/testrig",
				HasMayor:    true,
				HasRefinery: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with witness and refinery",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/gt/testrig",
				HasWitness:  true,
				HasRefinery: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with all agents",
			rig: Rig{
				Name:        "fullrig",
				Path:        "/tmp/gt/fullrig",
				HasMayor:    true,
				HasWitness:  true,
				HasRefinery: true,
			},
			wantPath: "/tmp/gt/fullrig",
		},
		{
			name: "rig with polecats",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/gt/testrig",
				HasMayor: true,
				Polecats: []string{"polecat1", "polecat2"},
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with crew",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/gt/testrig",
				HasMayor: true,
				Crew:     []string{"crew1", "crew2"},
			},
			wantPath: "/home/user/gt/testrig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.rig.BeadsPath()
			if got != tt.wantPath {
				t.Errorf("BeadsPath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestDefaultBranch_FallsBackToMain(t *testing.T) {
	t.Parallel()

	// DefaultBranch should return "main" when config cannot be loaded
	rig := Rig{
		Name: "testrig",
		Path: "/nonexistent/path",
	}

	got := rig.DefaultBranch()
	if got != "main" {
		t.Errorf("DefaultBranch() = %q, want %q", got, "main")
	}
}

func TestDefaultBranch_UsesRegistryFallback(t *testing.T) {
	t.Parallel()

	// When rig config.json doesn't exist but RegistryDefaultBranch is set,
	// DefaultBranch should return the registry value instead of "main".
	r := Rig{
		Name:                  "backend",
		Path:                  "/nonexistent/path",
		RegistryDefaultBranch: "develop",
	}

	got := r.DefaultBranch()
	if got != "develop" {
		t.Errorf("DefaultBranch() = %q, want %q", got, "develop")
	}
}

func TestDefaultBranch_RigConfigTakesPrecedence(t *testing.T) {
	t.Parallel()

	// Create temp dir with config.json that has default_branch
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := os.WriteFile(configPath, []byte(`{"type":"rig","version":1,"name":"test","default_branch":"staging"}`), 0644); err != nil {
		t.Fatal(err)
	}

	r := Rig{
		Name:                  "test",
		Path:                  tmpDir,
		RegistryDefaultBranch: "develop",
	}

	got := r.DefaultBranch()
	if got != "staging" {
		t.Errorf("DefaultBranch() = %q, want %q (rig config should take precedence over registry)", got, "staging")
	}
}

// TestResolveDefaultBranch_RegistryFallback covers sbx-gastown-fb7x: gt sling
// must fall through to mayor/rigs.json when the rig-level config.json is
// missing or has no default_branch. Without this fallback, gt sling defaulted
// to "main" even when the registry recorded a non-main branch (e.g. "develop").
func TestResolveDefaultBranch_RegistryFallback(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Rig directory exists but has no config.json (the sandboxx-backend case
	// that triggered the bug — config.json lived in a parallel rigs/ tree the
	// gt code path didn't consult).
	rigName := "backend"
	if err := os.MkdirAll(filepath.Join(townRoot, rigName), 0755); err != nil {
		t.Fatal(err)
	}

	// Registry has the right answer.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"backend":{"git_url":"","default_branch":"develop"}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDefaultBranch(townRoot, rigName); got != "develop" {
		t.Errorf("ResolveDefaultBranch() = %q, want %q (registry should be consulted when rig config.json is absent)", got, "develop")
	}
}

func TestResolveDefaultBranch_RigConfigBeatsRegistry(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	rigName := "backend"
	rigDir := filepath.Join(townRoot, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigCfg := `{"type":"rig","version":1,"name":"backend","default_branch":"staging"}`
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(rigCfg), 0644); err != nil {
		t.Fatal(err)
	}
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"backend":{"git_url":"","default_branch":"develop"}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	if got := ResolveDefaultBranch(townRoot, rigName); got != "staging" {
		t.Errorf("ResolveDefaultBranch() = %q, want %q (rig config.json must beat registry)", got, "staging")
	}
}

func TestResolveDefaultBranch_FallsBackToMain(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	if got := ResolveDefaultBranch(townRoot, "missing"); got != "main" {
		t.Errorf("ResolveDefaultBranch() = %q, want %q", got, "main")
	}
}
