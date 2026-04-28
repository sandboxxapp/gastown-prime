package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DoctorStatus is the most recent snapshot recorded by the doctor_dog patrol.
// It is persisted to daemon/doctor.json so `gt daemon status` can surface the
// metrics without having to rerun the probes itself.
type DoctorStatus struct {
	// TickAt is when the snapshot was taken (UTC).
	TickAt time.Time `json:"tick_at"`

	// Interval is the configured patrol cadence (e.g. "5m"), recorded for context.
	Interval string `json:"interval,omitempty"`

	// LatencyMs is the Dolt round-trip latency in milliseconds.
	LatencyMs int64 `json:"latency_ms"`

	// OrphanCount is the number of orphaned databases observed at tick time.
	OrphanCount int `json:"orphan_count"`

	// BackupAgeSec is the age in seconds of the newest Dolt filesystem backup.
	// Zero means no backup directory present.
	BackupAgeSec int64 `json:"backup_age_sec"`

	// LastMolID is the wisp root ID of the mol-dog-doctor molecule poured this tick.
	LastMolID string `json:"last_mol_id,omitempty"`

	// ProbeError captures any non-fatal error encountered during probing
	// (e.g. Dolt unreachable). Empty when probes succeeded.
	ProbeError string `json:"probe_error,omitempty"`
}

// DoctorStatusPath returns the location of the doctor_dog snapshot file.
func DoctorStatusPath(townRoot string) string {
	return filepath.Join(townRoot, "daemon", "doctor.json")
}

// LoadDoctorStatus reads the most recent doctor_dog snapshot.
// Returns (nil, nil) if the file does not exist; an error only on parse failure.
func LoadDoctorStatus(townRoot string) (*DoctorStatus, error) {
	data, err := os.ReadFile(DoctorStatusPath(townRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var status DoctorStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("parse doctor.json: %w", err)
	}
	return &status, nil
}

// SaveDoctorStatus atomically writes the doctor_dog snapshot to daemon/doctor.json.
// Creates the daemon/ subdirectory if missing.
func SaveDoctorStatus(townRoot string, status *DoctorStatus) error {
	if status == nil {
		return nil
	}
	dir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	tmp := DoctorStatusPath(townRoot) + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, DoctorStatusPath(townRoot))
}

// FormatDoctorStatusLine renders a compact one-line summary of the snapshot
// suitable for `gt daemon status`. Returns empty string when status is nil.
func FormatDoctorStatusLine(s *DoctorStatus) string {
	if s == nil {
		return ""
	}
	backup := "n/a"
	if s.BackupAgeSec > 0 {
		backup = (time.Duration(s.BackupAgeSec) * time.Second).String()
	}
	if s.ProbeError != "" {
		return fmt.Sprintf("latency=%dms orphans=%d backup=%s (probe error: %s)",
			s.LatencyMs, s.OrphanCount, backup, s.ProbeError)
	}
	return fmt.Sprintf("latency=%dms orphans=%d backup=%s",
		s.LatencyMs, s.OrphanCount, backup)
}
