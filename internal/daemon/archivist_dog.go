package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultArchivistDogInterval is the patrol interval for scanning rig notes.
	defaultArchivistDogInterval = 5 * time.Minute

	// archivistDogCooldownPerRig is the minimum time between dispatches for a single rig.
	archivistDogCooldownPerRig = 10 * time.Minute

	// archivistDogDiagEveryN logs a diagnostic summary every N scans.
	archivistDogDiagEveryN = 10
)

// archivistDogScanCount tracks how many scans have run.
var archivistDogScanCount atomic.Int64

// ArchivistDogConfig holds configuration for the archivist_dog patrol.
// This patrol scans rigs for unprocessed domain notes and dispatches
// archivists to extract knowledge from them.
type ArchivistDogConfig struct {
	Enabled     bool   `json:"enabled"`
	IntervalStr string `json:"interval,omitempty"`
}

// archivistDogCooldowns tracks per-rig dispatch cooldowns.
// Only accessed from the daemon's heartbeat goroutine, but wrapped
// in a mutex for safety in case that invariant changes.
type archivistDogCooldowns struct {
	mu          sync.Mutex
	lastDispatched map[string]time.Time // rig name -> last dispatch time
}

func newArchivistDogCooldowns() *archivistDogCooldowns {
	return &archivistDogCooldowns{
		lastDispatched: make(map[string]time.Time),
	}
}

// canDispatch returns true if enough time has passed since the last dispatch for this rig.
func (c *archivistDogCooldowns) canDispatch(rig string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	last, ok := c.lastDispatched[rig]
	if !ok {
		return true
	}
	return time.Since(last) >= archivistDogCooldownPerRig
}

// markDispatched records that a dispatch was made for this rig.
func (c *archivistDogCooldowns) markDispatched(rig string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastDispatched[rig] = time.Now()
}

// archivistDogInterval returns the configured interval, or the default.
func archivistDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.ArchivistDog != nil {
		if config.Patrols.ArchivistDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.ArchivistDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultArchivistDogInterval
}

// rigNotes holds scan results for a single rig.
type rigNotes struct {
	Rig   string
	Files []string // relative paths within domain/notes/
}

// scanRigNotes scans townRoot for rigs with unprocessed domain notes.
// Returns a list of rigs that have .md files in rigs/<rig>/domain/notes/.
func scanRigNotes(townRoot string) []rigNotes {
	var results []rigNotes

	entries, err := os.ReadDir(filepath.Join(townRoot, "rigs"))
	if err != nil {
		// Try top-level rig directories (symlink layout)
		entries, err = os.ReadDir(townRoot)
		if err != nil {
			return nil
		}
	}

	for _, entry := range entries {
		rigName := entry.Name()
		if strings.HasPrefix(rigName, ".") {
			continue
		}

		// Check both rigs/<rig>/domain/notes/ and <rig>/domain/notes/ layouts
		notesDir := filepath.Join(townRoot, "rigs", rigName, "domain", "notes")
		info, err := os.Stat(notesDir)
		if err != nil || !info.IsDir() {
			// Try top-level layout
			notesDir = filepath.Join(townRoot, rigName, "domain", "notes")
			info, err = os.Stat(notesDir)
			if err != nil || !info.IsDir() {
				continue
			}
		}

		noteEntries, err := os.ReadDir(notesDir)
		if err != nil {
			continue
		}

		var files []string
		for _, ne := range noteEntries {
			if ne.IsDir() || strings.HasPrefix(ne.Name(), ".") {
				continue
			}
			if strings.HasSuffix(ne.Name(), ".md") {
				files = append(files, ne.Name())
			}
		}

		if len(files) > 0 {
			results = append(results, rigNotes{Rig: rigName, Files: files})
		}
	}

	return results
}

// runArchivistDog is the daemon patrol method that scans for unprocessed rig notes
// and dispatches archivists. Called on each ticker fire.
func (d *Daemon) runArchivistDog() {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Printf("archivist_dog: recovered from panic: %v", r)
		}
	}()

	if !d.isPatrolActive("archivist_dog") {
		return
	}

	scanNum := archivistDogScanCount.Add(1)

	rigs := scanRigNotes(d.config.TownRoot)

	if len(rigs) == 0 {
		if scanNum%archivistDogDiagEveryN == 1 {
			d.logger.Printf("archivist_dog: alive (scan #%d, no unprocessed notes)", scanNum)
		}
		return
	}

	// Log what was found
	for _, rn := range rigs {
		d.logger.Printf("archivist_dog: %s has %d unprocessed notes: %s",
			rn.Rig, len(rn.Files), strings.Join(rn.Files, ", "))
	}

	// Surface for mayor: log actionable summary.
	// The mayor dispatches bridge-local Opus archivists via Agent tool when they
	// see pending notes. This keeps the mayor in the loop until we need to scale.
	totalNotes := 0
	for _, rn := range rigs {
		totalNotes += len(rn.Files)
	}
	d.logger.Printf("archivist_dog: %d note(s) pending across %d rig(s) — mayor dispatch needed",
		totalNotes, len(rigs))
}
