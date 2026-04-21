package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePolecatExitEvent_AppendsValidJSONL_Success(t *testing.T) {
	townRoot := t.TempDir()
	ev := newPolecatExitEvent("sandboxx-backend", "portia", "sbx-gastown-npu7", true, "")

	if err := writePolecatExitEvent(townRoot, ev); err != nil {
		t.Fatalf("writePolecatExitEvent: %v", err)
	}

	path := filepath.Join(townRoot, "daemon", "polecat-events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Errorf("events file must end with newline, got %q", string(data))
	}

	var got polecatExitEvent
	if err := json.Unmarshal([]byte(strings.TrimRight(string(data), "\n")), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, string(data))
	}
	if got.Rig != "sandboxx-backend" || got.Polecat != "portia" || got.Bead != "sbx-gastown-npu7" {
		t.Errorf("field mismatch: %+v", got)
	}
	if !got.Success {
		t.Errorf("expected success=true, got false")
	}
	if got.Event != "exit" {
		t.Errorf("expected event=exit, got %q", got.Event)
	}
	if got.Reason != "" {
		t.Errorf("expected empty reason on success, got %q", got.Reason)
	}
	if got.TS == "" {
		t.Errorf("expected non-empty timestamp")
	}
}

func TestWritePolecatExitEvent_FailurePathIncludesReason(t *testing.T) {
	townRoot := t.TempDir()
	ev := newPolecatExitEvent("waypoints-api", "wp-1", "sbx-gastown-xyz", false, "push failed: auth denied")

	if err := writePolecatExitEvent(townRoot, ev); err != nil {
		t.Fatalf("writePolecatExitEvent: %v", err)
	}

	path := filepath.Join(townRoot, "daemon", "polecat-events.jsonl")
	data, _ := os.ReadFile(path)
	var got polecatExitEvent
	if err := json.Unmarshal([]byte(strings.TrimRight(string(data), "\n")), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Success {
		t.Errorf("expected success=false")
	}
	if got.Reason != "push failed: auth denied" {
		t.Errorf("reason mismatch: %q", got.Reason)
	}
}

func TestWritePolecatExitEvent_MultipleAppendsPreserved(t *testing.T) {
	townRoot := t.TempDir()
	for i := 0; i < 3; i++ {
		ev := newPolecatExitEvent("rig", "p", fmt.Sprintf("bead-%d", i), true, "")
		if err := writePolecatExitEvent(townRoot, ev); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	path := filepath.Join(townRoot, "daemon", "polecat-events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var count int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev polecatExitEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Errorf("line %d invalid JSON: %v", count, err)
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

func TestRotatePolecatEventsFile_BelowCapNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	lines := []string{`{"a":1}`, `{"a":2}`, `{"a":3}`}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rotatePolecatEventsFile(path, 500); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != content {
		t.Errorf("below-cap rotate altered file:\nwant %q\ngot  %q", content, string(got))
	}
}

func TestRotatePolecatEventsFile_AboveCapKeepsTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// 10 lines, cap at 4 → expect lines 7..10
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, `{"n":%d}`+"\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rotatePolecatEventsFile(path, 4); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := `{"n":7}` + "\n" + `{"n":8}` + "\n" + `{"n":9}` + "\n" + `{"n":10}` + "\n"
	if string(got) != want {
		t.Errorf("rotate tail mismatch:\nwant %q\ngot  %q", want, string(got))
	}

	// Each remaining line must still be valid JSON — rotation must not
	// split a line mid-object.
	scanner := bufio.NewScanner(strings.NewReader(string(got)))
	for scanner.Scan() {
		var v map[string]int
		if err := json.Unmarshal(scanner.Bytes(), &v); err != nil {
			t.Errorf("invalid JSONL line after rotate: %q", scanner.Text())
		}
	}
}

func TestRotatePolecatEventsFile_MissingFileNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.jsonl")

	if err := rotatePolecatEventsFile(path, 100); err != nil {
		t.Errorf("missing file should not error: %v", err)
	}
}

func TestWriteThenRotate_AtCap(t *testing.T) {
	// End-to-end: append past cap and confirm the writer-triggered rotation
	// keeps exactly polecatEventsMaxLines entries.
	townRoot := t.TempDir()

	// Hand-seed near the cap to avoid noise.
	path := filepath.Join(townRoot, "daemon", "polecat-events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < polecatEventsMaxLines; i++ {
		fmt.Fprintf(&b, `{"seed":%d}`+"\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		ev := newPolecatExitEvent("rig", "p", fmt.Sprintf("new-%d", i), true, "")
		if err := writePolecatExitEvent(townRoot, ev); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != polecatEventsMaxLines {
		t.Errorf("expected %d lines after rotate, got %d", polecatEventsMaxLines, len(lines))
	}
	for i, line := range lines[len(lines)-3:] {
		var ev polecatExitEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("tail line %d not valid JSON: %v\n%s", i, err, line)
		}
		wantBead := fmt.Sprintf("new-%d", i)
		if ev.Bead != wantBead {
			t.Errorf("tail line %d: bead=%q, want %q", i, ev.Bead, wantBead)
		}
	}
}

func TestNewPolecatExitEvent_Timestamp(t *testing.T) {
	ev := newPolecatExitEvent("r", "p", "b", true, "")
	if ev.TS == "" {
		t.Error("timestamp must not be empty")
	}
	if !strings.Contains(ev.TS, "T") || !strings.Contains(ev.TS, "Z") {
		t.Errorf("timestamp not RFC3339 UTC: %q", ev.TS)
	}
}
