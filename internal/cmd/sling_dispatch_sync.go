package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/style"
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

// syncFormulasToRig updates the per-rig .beads/formulas/ with the latest
// embedded formulas from the gt binary. This ensures that when CookFormula
// runs in the rig's directory, it finds the current formula versions rather
// than stale copies from a prior gt install.
//
// Uses formula.UpdateFormulas which:
//   - Provisions new formulas that don't exist yet
//   - Updates outdated formulas (embedded changed, user hasn't modified)
//   - Preserves user-modified formulas (won't overwrite customizations)
//
// Non-fatal by convention: callers log and continue on error.
func syncFormulasToRig(townRoot, rigName string) error {
	if rigName == "" {
		return nil
	}

	rigPath := filepath.Join(townRoot, rigName)
	rigBeadsDir := filepath.Join(rigPath, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		return nil // No rig-level beads database — nothing to sync to
	}

	updated, _, reinstalled, err := formula.UpdateFormulas(rigPath)
	if err != nil {
		return fmt.Errorf("updating rig formulas: %w", err)
	}
	if updated+reinstalled > 0 {
		fmt.Printf("  %s Synced %d formula(s) to rig %s\n",
			style.Bold.Render("→"), updated+reinstalled, rigName)
	}
	return nil
}
