package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/util"
)

// syncBeadToRig runs `bd repo sync` inside the rig's directory so the rig's
// per-rig .beads/ database can pull the target bead from the town-root .beads/.
//
// This is the town→rig direction of the per-rig beads sync cycle:
//
//	Dispatch:  town-root .beads/  →  rig .beads/   (this function)
//	Reap:      rig .beads/        →  town-root .beads/  (syncBeadsToTown in deacon)
//
// Without this sync, InstantiateFormulaOnBead and hookBeadWithRetry fail because
// both route bd commands to the rig's directory via ResolveHookDir, but the bead
// was created at town-root level and doesn't exist in the rig's Dolt database.
//
// Non-fatal by convention: callers log and continue on error (the bead may still
// exist in the rig DB from a prior sync, or bd may not be available in all envs).
func syncBeadToRig(townRoot, rigName string) error {
	if rigName == "" {
		return nil
	}

	rigBeadsDir := filepath.Join(townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		return nil // No rig-level beads database — nothing to sync to
	}

	townBeadsDir := filepath.Join(townRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); os.IsNotExist(err) {
		return nil // No town-root beads database — nothing to sync from
	}

	rigPath := filepath.Join(townRoot, rigName)
	cmd := exec.Command("bd", "repo", "sync")
	cmd.Dir = rigPath
	cmd.Env = append(os.Environ(), "BEADS_DIR="+rigBeadsDir)
	util.SetDetachedProcessGroup(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd repo sync: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
