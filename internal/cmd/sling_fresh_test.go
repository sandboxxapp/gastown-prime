package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

// TestResolveTarget_FreshFlagPropagates verifies that ResolveTargetOptions.Fresh
// reaches SlingSpawnOptions when the target is a rig. This is the contract that
// lets --fresh bypass idle-polecat reuse in SpawnPolecatForSling.
//
// Regression target: sylveste/kr13 scenario (sbx-gastown-y9x2) — without this
// propagation, --fresh would still reuse idle polecats carrying stale state.
func TestResolveTarget_FreshFlagPropagates(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0o755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Register rig so IsRigName("gastown") succeeds.
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigs := &config.RigsConfig{
		Version: 1,
		Rigs: map[string]config.RigEntry{
			"gastown": {
				GitURL:  "git@github.com:test/gastown.git",
				AddedAt: time.Now().Truncate(time.Second),
				BeadsConfig: &config.BeadsConfig{
					Repo:   "local",
					Prefix: "gt-",
				},
			},
		},
	}
	if err := config.SaveRigsConfig(rigsPath, rigs); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}
	// rig.Manager.GetRig requires the rig directory to exist.
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown"), 0o755); err != nil {
		t.Fatalf("mkdir rig dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	prevSpawn := spawnPolecatForSling
	t.Cleanup(func() { spawnPolecatForSling = prevSpawn })

	var captured SlingSpawnOptions
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		captured = opts
		return &SpawnedPolecatInfo{
			RigName:     rigName,
			PolecatName: "Toast",
			ClonePath:   filepath.Join(townRoot, "fake-polecat"),
		}, nil
	}

	cases := []struct {
		name  string
		fresh bool
	}{
		{"fresh=true", true},
		{"fresh=false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured = SlingSpawnOptions{}
			_, err := resolveTarget("gastown", ResolveTargetOptions{
				Fresh:    tc.fresh,
				NoBoot:   true, // skip wakeRigAgents
				TownRoot: townRoot,
			})
			if err != nil {
				t.Fatalf("resolveTarget: %v", err)
			}
			if captured.Fresh != tc.fresh {
				t.Errorf("spawn opts Fresh = %v, want %v", captured.Fresh, tc.fresh)
			}
		})
	}
}

// TestSlingFreshFlagRegistered verifies that the --fresh flag is wired to the
// slingFresh package var so runSling can pick it up.
func TestSlingFreshFlagRegistered(t *testing.T) {
	flag := slingCmd.Flags().Lookup("fresh")
	if flag == nil {
		t.Fatal("--fresh flag not registered on slingCmd")
	}
	if flag.DefValue != "false" {
		t.Errorf("--fresh default = %q, want \"false\" (reuse remains the default)", flag.DefValue)
	}
}
