package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLogger_Log(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	logger, err := NewFileLogger(path)
	if err != nil {
		t.Fatalf("NewFileLogger: %v", err)
	}
	defer logger.Close()

	logger.Log(Entry{
		Upstream:       "github",
		Tool:           "get_file_contents",
		Classification: "read",
		PolicyMode:     "read",
		Allowed:        true,
		DurationMs:     42,
		ResultStatus:   "ok",
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse log: %v", err)
	}
	if entry.Upstream != "github" {
		t.Errorf("upstream = %q", entry.Upstream)
	}
	if entry.Tool != "get_file_contents" {
		t.Errorf("tool = %q", entry.Tool)
	}
	if !entry.Allowed {
		t.Error("expected allowed=true")
	}
	if entry.Timestamp == "" {
		t.Error("timestamp should be auto-filled")
	}
}

func TestNopLogger(t *testing.T) {
	logger, err := NewFileLogger("")
	if err != nil {
		t.Fatalf("NewFileLogger empty: %v", err)
	}
	// Should not panic
	logger.Log(Entry{Tool: "test"})
	logger.Close()
}

func TestHashArgs(t *testing.T) {
	h := HashArgs(json.RawMessage(`{"repo":"sandboxx"}`))
	if !strings.HasPrefix(h, "sha256:") {
		t.Errorf("hash = %q, want sha256: prefix", h)
	}
	if HashArgs(nil) != "" {
		t.Error("nil args should return empty")
	}
}
