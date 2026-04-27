package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorStatusPath(t *testing.T) {
	townRoot := "/tmp/test-town"
	got := DoctorStatusPath(townRoot)
	want := filepath.Join(townRoot, "daemon", "doctor.json")
	if got != want {
		t.Errorf("DoctorStatusPath = %q, want %q", got, want)
	}
}

func TestLoadDoctorStatus_Missing(t *testing.T) {
	tmpDir := t.TempDir()

	got, err := LoadDoctorStatus(tmpDir)
	if err != nil {
		t.Fatalf("LoadDoctorStatus on missing file should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %+v", got)
	}
}

func TestLoadDoctorStatus_Corrupt(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "daemon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DoctorStatusPath(tmpDir), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadDoctorStatus(tmpDir)
	if err == nil {
		t.Error("expected error on corrupt file, got nil")
	}
	if got != nil {
		t.Errorf("expected nil status on corrupt file, got %+v", got)
	}
}

func TestSaveLoadDoctorStatus_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	tickAt := time.Now().UTC().Truncate(time.Second)
	want := &DoctorStatus{
		TickAt:        tickAt,
		Interval:      "5m",
		LatencyMs:     42,
		OrphanCount:   3,
		BackupAgeSec:  120,
		LastMolID:     "sbx-gastown-doctor-x1",
		ProbeError:    "",
	}

	if err := SaveDoctorStatus(tmpDir, want); err != nil {
		t.Fatalf("SaveDoctorStatus: %v", err)
	}

	got, err := LoadDoctorStatus(tmpDir)
	if err != nil {
		t.Fatalf("LoadDoctorStatus: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil status after save")
	}

	if !got.TickAt.Equal(want.TickAt) {
		t.Errorf("TickAt: got %v, want %v", got.TickAt, want.TickAt)
	}
	if got.Interval != want.Interval {
		t.Errorf("Interval: got %q, want %q", got.Interval, want.Interval)
	}
	if got.LatencyMs != want.LatencyMs {
		t.Errorf("LatencyMs: got %d, want %d", got.LatencyMs, want.LatencyMs)
	}
	if got.OrphanCount != want.OrphanCount {
		t.Errorf("OrphanCount: got %d, want %d", got.OrphanCount, want.OrphanCount)
	}
	if got.BackupAgeSec != want.BackupAgeSec {
		t.Errorf("BackupAgeSec: got %d, want %d", got.BackupAgeSec, want.BackupAgeSec)
	}
	if got.LastMolID != want.LastMolID {
		t.Errorf("LastMolID: got %q, want %q", got.LastMolID, want.LastMolID)
	}
}

func TestSaveDoctorStatus_CreatesDaemonDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Note: no daemon/ subdir pre-created.
	status := &DoctorStatus{TickAt: time.Now().UTC(), Interval: "5m"}
	if err := SaveDoctorStatus(tmpDir, status); err != nil {
		t.Fatalf("SaveDoctorStatus should create daemon dir, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "daemon", "doctor.json")); err != nil {
		t.Errorf("doctor.json should exist after save: %v", err)
	}
}

func TestFormatDoctorStatusLine_Compact(t *testing.T) {
	tickAt := time.Date(2026, 4, 27, 10, 30, 0, 0, time.UTC)
	status := &DoctorStatus{
		TickAt:       tickAt,
		Interval:     "5m",
		LatencyMs:    42,
		OrphanCount:  3,
		BackupAgeSec: 120,
		LastMolID:    "sbx-gastown-doctor-x1",
	}

	line := FormatDoctorStatusLine(status)
	// Expect human-readable line that includes the three tracked metrics.
	for _, want := range []string{"42ms", "orphans=3", "backup="} {
		if !strings.Contains(line, want) {
			t.Errorf("FormatDoctorStatusLine output %q missing %q", line, want)
		}
	}
}

func TestFormatDoctorStatusLine_NilSafe(t *testing.T) {
	if got := FormatDoctorStatusLine(nil); got != "" {
		t.Errorf("FormatDoctorStatusLine(nil) = %q, want empty", got)
	}
}
