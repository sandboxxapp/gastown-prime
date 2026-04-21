package util

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// archivistPidsSubdir is the directory under a town root where dispatchers
// register running bridge-local archivists. Orphan/zombie cleanup reads this
// directory and exempts listed pids from signaling.
//
// Without this protection, CleanupOrphanedClaudeProcesses / CleanupZombieClaudeProcesses
// SIGTERM any claude process with TTY "?" that is older than minOrphanAge and
// rooted in a Gas Town workspace — which is exactly what bridge-local
// archivists look like. Marker file presence opts them out.
const archivistPidsSubdir = "daemon/archivist-pids"

// ArchivistPidsDir returns the registry directory for a town root.
func ArchivistPidsDir(townRoot string) string {
	return filepath.Join(townRoot, archivistPidsSubdir)
}

// RegisterArchivist records that pid is a legitimate archivist process
// running in townRoot. The marker is a single file named after the decimal
// pid; its contents are a UTC RFC3339 timestamp for diagnostic dumps.
// Callers should UnregisterArchivist when the process exits.
func RegisterArchivist(townRoot string, pid int) error {
	dir := ArchivistPidsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, strconv.Itoa(pid))
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0644)
}

// UnregisterArchivist removes the marker for pid. Idempotent: returns nil
// if the marker was never written or has already been removed.
func UnregisterArchivist(townRoot string, pid int) error {
	path := filepath.Join(ArchivistPidsDir(townRoot), strconv.Itoa(pid))
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsRegisteredArchivist reports whether pid has a marker file in townRoot.
// Returns false for empty townRoot or non-positive pid.
func IsRegisteredArchivist(townRoot string, pid int) bool {
	if townRoot == "" || pid <= 0 {
		return false
	}
	path := filepath.Join(ArchivistPidsDir(townRoot), strconv.Itoa(pid))
	_, err := os.Stat(path)
	return err == nil
}
