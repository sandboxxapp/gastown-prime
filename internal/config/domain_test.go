package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadDomainDocsTOC(t *testing.T) {
	t.Parallel()

	t.Run("returns lightweight catalog without content", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		body := "# Auth Flow\n\nFirebase issues a JWT; backend verifies via google-auth-library.\n"
		if err := os.WriteFile(filepath.Join(domainDir, "auth.md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}

		docs := LoadDomainDocsTOC(townRoot, "myrig")
		if len(docs) != 1 {
			t.Fatalf("expected 1 doc, got %d", len(docs))
		}
		if docs[0].Content != "" {
			t.Errorf("expected empty Content in TOC variant, got %q", docs[0].Content)
		}
		if docs[0].Title != "Auth" {
			t.Errorf("expected title 'Auth', got %q", docs[0].Title)
		}
		if !strings.Contains(docs[0].Summary, "Firebase issues a JWT") {
			t.Errorf("expected summary to come from first body line, got %q", docs[0].Summary)
		}
	})

	t.Run("returns nil when rig empty", func(t *testing.T) {
		t.Parallel()
		if docs := LoadDomainDocsTOC(t.TempDir(), ""); docs != nil {
			t.Errorf("expected nil, got %d docs", len(docs))
		}
	})
}

func TestExtractSummary(t *testing.T) {
	t.Parallel()

	t.Run("frontmatter summary wins", func(t *testing.T) {
		t.Parallel()
		content := "---\nsummary: Token refresh contract for Firebase\nauthor: alice\n---\n\n# Token\n\nDetails here.\n"
		got := extractSummary(content)
		if got != "Token refresh contract for Firebase" {
			t.Errorf("expected frontmatter summary, got %q", got)
		}
	})

	t.Run("strips quotes around frontmatter summary", func(t *testing.T) {
		t.Parallel()
		content := "---\nsummary: \"Quoted summary\"\n---\n\nBody\n"
		got := extractSummary(content)
		if got != "Quoted summary" {
			t.Errorf("expected unquoted summary, got %q", got)
		}
	})

	t.Run("falls back to first non-heading line", func(t *testing.T) {
		t.Parallel()
		content := "# Title\n\n## Subhead\n\nThe first prose line is the summary.\n"
		got := extractSummary(content)
		if got != "The first prose line is the summary." {
			t.Errorf("got %q", got)
		}
	})

	t.Run("skips code fences", func(t *testing.T) {
		t.Parallel()
		content := "# Title\n\n```bash\nfoo bar\n```\n\nReal prose here.\n"
		got := extractSummary(content)
		if got != "Real prose here." {
			t.Errorf("got %q", got)
		}
	})

	t.Run("empty document returns empty", func(t *testing.T) {
		t.Parallel()
		if got := extractSummary(""); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("truncates long summaries", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("x", 200)
		got := extractSummary("# T\n\n" + long)
		if len(got) > 120 {
			t.Errorf("expected truncation to 120 chars, got %d", len(got))
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("expected ellipsis suffix, got %q", got)
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
