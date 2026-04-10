package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDomainDocs(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when rig is empty", func(t *testing.T) {
		t.Parallel()
		docs := LoadDomainDocs(t.TempDir(), "")
		if docs != nil {
			t.Errorf("expected nil, got %d docs", len(docs))
		}
	})

	t.Run("returns nil when domain dir missing", func(t *testing.T) {
		t.Parallel()
		docs := LoadDomainDocs(t.TempDir(), "myrig")
		if docs != nil {
			t.Errorf("expected nil, got %d docs", len(docs))
		}
	})

	t.Run("loads top-level md files", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "auth-flow.md"), []byte("OAuth2 flow docs"), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocs(townRoot, "myrig")
		if len(docs) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(docs))
		}
		if docs[0].Title != "Auth Flow" {
			t.Errorf("expected title 'Auth Flow', got %q", docs[0].Title)
		}
		if docs[0].Category != "" {
			t.Errorf("expected empty category, got %q", docs[0].Category)
		}
		if docs[0].Content != "OAuth2 flow docs" {
			t.Errorf("expected content 'OAuth2 flow docs', got %q", docs[0].Content)
		}
	})

	t.Run("loads subdirectory files with category", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain", "auth")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "token.md"), []byte("Token refresh details"), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocs(townRoot, "myrig")
		if len(docs) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(docs))
		}
		if docs[0].Category != "auth" {
			t.Errorf("expected category 'auth', got %q", docs[0].Category)
		}
		if docs[0].RelPath != filepath.Join("auth", "token.md") {
			t.Errorf("expected relpath 'auth/token.md', got %q", docs[0].RelPath)
		}
	})

	t.Run("skips README.md", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "README.md"), []byte("Index file"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "auth.md"), []byte("Auth docs"), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocs(townRoot, "myrig")
		if len(docs) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(docs))
		}
		if docs[0].Title != "Auth" {
			t.Errorf("expected 'Auth', got %q", docs[0].Title)
		}
	})

	t.Run("skips empty files", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "empty.md"), []byte("   \n  "), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocs(townRoot, "myrig")
		if len(docs) != 0 {
			t.Errorf("expected 0 docs, got %d", len(docs))
		}
	})

	t.Run("sorts by relative path", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		authDir := filepath.Join(domainDir, "auth")
		if err := os.MkdirAll(authDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Create in reverse order
		if err := os.WriteFile(filepath.Join(domainDir, "zzz.md"), []byte("last"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(authDir, "token.md"), []byte("token"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "aaa.md"), []byte("first"), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocs(townRoot, "myrig")
		if len(docs) != 3 {
			t.Fatalf("expected 3 docs, got %d", len(docs))
		}
		if docs[0].RelPath != "aaa.md" {
			t.Errorf("expected first doc 'aaa.md', got %q", docs[0].RelPath)
		}
		if docs[1].RelPath != filepath.Join("auth", "token.md") {
			t.Errorf("expected second doc 'auth/token.md', got %q", docs[1].RelPath)
		}
		if docs[2].RelPath != "zzz.md" {
			t.Errorf("expected third doc 'zzz.md', got %q", docs[2].RelPath)
		}
	})

	t.Run("skips non-md files", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "notes.txt"), []byte("not markdown"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "auth.md"), []byte("Auth docs"), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocs(townRoot, "myrig")
		if len(docs) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(docs))
		}
	})
}

func TestTitleCaseHyphens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, want string
	}{
		{"auth", "Auth"},
		{"referral-edge-cases", "Referral Edge Cases"},
		{"fac", "Fac"},
		{"api-v2-endpoints", "Api V2 Endpoints"},
	}
	for _, tt := range tests {
		if got := TitleCaseHyphens(tt.input); got != tt.want {
			t.Errorf("TitleCaseHyphens(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
