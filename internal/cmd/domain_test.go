package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDomainTOC(t *testing.T) {
	t.Parallel()

	t.Run("emits header and entries", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "rigA", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "auth.md"),
			[]byte("# Auth\n\nFirebase JWT verification.\n"), 0644); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := writeDomainTOC(&buf, townRoot, "rigA"); err != nil {
			t.Fatalf("writeDomainTOC: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Domain Library — rigA") {
			t.Errorf("expected rig name in header, got: %s", out)
		}
		if !strings.Contains(out, "`auth.md`") {
			t.Errorf("expected auth.md row, got: %s", out)
		}
		if !strings.Contains(out, "Firebase JWT") {
			t.Errorf("expected summary in TOC, got: %s", out)
		}
	})

	t.Run("handles empty domain dir", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		if err := writeDomainTOC(&buf, t.TempDir(), "rigA"); err != nil {
			t.Fatalf("writeDomainTOC: %v", err)
		}
		if !strings.Contains(buf.String(), "No domain docs") {
			t.Errorf("expected 'No domain docs' message, got: %s", buf.String())
		}
	})
}

func TestWriteDomainDoc(t *testing.T) {
	t.Parallel()

	t.Run("renders doc with header", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "rigA", "domain", "auth")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		body := "# Token\n\nRefresh contract details."
		if err := os.WriteFile(filepath.Join(domainDir, "token.md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := writeDomainDoc(&buf, townRoot, "rigA", filepath.Join("auth", "token.md")); err != nil {
			t.Fatalf("writeDomainDoc: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Refresh contract details") {
			t.Errorf("expected body in output, got: %s", out)
		}
		if !strings.Contains(out, filepath.Join("auth", "token.md")) {
			t.Errorf("expected relpath in header, got: %s", out)
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		if err := os.MkdirAll(filepath.Join(townRoot, "rigA", "domain"), 0755); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		err := writeDomainDoc(&buf, townRoot, "rigA", "../../etc/passwd")
		if err == nil {
			t.Errorf("expected error for path traversal, got nil")
		}
	})

	t.Run("lists linked raw notes", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "rigA", "domain")
		notesDir := filepath.Join(domainDir, "notes")
		if err := os.MkdirAll(notesDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "auth.md"), []byte("# Auth\n\nBody."), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(notesDir, "auth-2026-05-19.md"),
			[]byte("note body"), 0644); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := writeDomainDoc(&buf, townRoot, "rigA", "auth.md"); err != nil {
			t.Fatalf("writeDomainDoc: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Related raw notes") {
			t.Errorf("expected related-notes section, got: %s", out)
		}
		if !strings.Contains(out, "auth-2026-05-19.md") {
			t.Errorf("expected linked note filename, got: %s", out)
		}
	})
}
