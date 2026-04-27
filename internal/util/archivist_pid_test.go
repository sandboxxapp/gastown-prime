package util

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRegisterArchivist_CreatesMarker(t *testing.T) {
	town := t.TempDir()
	if err := RegisterArchivist(town, 12345); err != nil {
		t.Fatalf("RegisterArchivist: %v", err)
	}
	path := filepath.Join(town, "daemon", "archivist-pids", "12345")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("marker not created: %v", err)
	}
}

func TestRegisterArchivist_WritesTimestamp(t *testing.T) {
	town := t.TempDir()
	if err := RegisterArchivist(town, 42); err != nil {
		t.Fatalf("RegisterArchivist: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(town, "daemon", "archivist-pids", "42"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("marker should contain a timestamp")
	}
}

func TestRegisterArchivist_CreatesSubdirIfMissing(t *testing.T) {
	town := t.TempDir()
	// No daemon/ or daemon/archivist-pids/ yet — RegisterArchivist must create both.
	if err := RegisterArchivist(town, 1); err != nil {
		t.Fatalf("RegisterArchivist: %v", err)
	}
	info, err := os.Stat(filepath.Join(town, "daemon", "archivist-pids"))
	if err != nil {
		t.Fatalf("subdir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("archivist-pids path is not a directory")
	}
}

func TestUnregisterArchivist_RemovesMarker(t *testing.T) {
	town := t.TempDir()
	if err := RegisterArchivist(town, 7); err != nil {
		t.Fatal(err)
	}
	if err := UnregisterArchivist(town, 7); err != nil {
		t.Fatalf("UnregisterArchivist: %v", err)
	}
	path := filepath.Join(town, "daemon", "archivist-pids", "7")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("marker should be gone, stat err=%v", err)
	}
}

func TestUnregisterArchivist_Idempotent(t *testing.T) {
	town := t.TempDir()
	// Never registered — Unregister should still succeed.
	if err := UnregisterArchivist(town, 999); err != nil {
		t.Errorf("unregister of missing pid should be nil, got %v", err)
	}
	// Register then unregister twice.
	if err := RegisterArchivist(town, 1); err != nil {
		t.Fatal(err)
	}
	if err := UnregisterArchivist(town, 1); err != nil {
		t.Fatal(err)
	}
	if err := UnregisterArchivist(town, 1); err != nil {
		t.Errorf("second unregister should be nil, got %v", err)
	}
}

func TestIsRegisteredArchivist(t *testing.T) {
	town := t.TempDir()
	if IsRegisteredArchivist(town, 100) {
		t.Error("unregistered pid should report false")
	}
	if err := RegisterArchivist(town, 100); err != nil {
		t.Fatal(err)
	}
	if !IsRegisteredArchivist(town, 100) {
		t.Error("registered pid should report true")
	}
	if IsRegisteredArchivist(town, 101) {
		t.Error("only-100-registered should not cover 101")
	}
	if err := UnregisterArchivist(town, 100); err != nil {
		t.Fatal(err)
	}
	if IsRegisteredArchivist(town, 100) {
		t.Error("after unregister pid should report false")
	}
}

func TestIsRegisteredArchivist_EmptyInputs(t *testing.T) {
	if IsRegisteredArchivist("", 1) {
		t.Error("empty townRoot must report false")
	}
	if IsRegisteredArchivist(t.TempDir(), 0) {
		t.Error("zero pid must report false")
	}
	if IsRegisteredArchivist(t.TempDir(), -1) {
		t.Error("negative pid must report false")
	}
}

func TestRegisterArchivist_FilenameIsCanonicalDecimal(t *testing.T) {
	// Ensure marker file uses decimal pid with no leading zeros, no padding —
	// the orphan-cleanup read path computes the same filename from pid.
	town := t.TempDir()
	if err := RegisterArchivist(town, 98765); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(town, "daemon", "archivist-pids", strconv.Itoa(98765))
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected canonical decimal filename %q, err=%v", path, err)
	}
}

func TestIsRegisteredArchivist_StalePidfileTreatedAsRegistered(t *testing.T) {
	// A marker whose pid no longer corresponds to a live process is still
	// reported as registered. Liveness is not checked at the util layer —
	// the registry is file-presence semantics, and stale-file cleanup is
	// the dispatcher's job (rigs/deacon/archivist.py handles the Python
	// side; Go's archivist_dog unregisters via deferred goroutine on exit).
	//
	// Rationale: orphan cleanup reads this to decide which pids to skip.
	// A stale pid is dead — ps won't list it as a candidate anyway — so
	// treating it as "registered" is safe.
	town := t.TempDir()
	deadPid := 99999999 // well above any realistic live pid
	if err := RegisterArchivist(town, deadPid); err != nil {
		t.Fatal(err)
	}
	if !IsRegisteredArchivist(town, deadPid) {
		t.Error("stale pidfile should still report as registered")
	}
}
