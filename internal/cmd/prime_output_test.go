package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputRoleDirectives(t *testing.T) {
	t.Parallel()

	t.Run("no directives emits nothing visible", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}

		var buf bytes.Buffer
		outputRoleDirectives(ctx, &buf, false)
		out := buf.String()

		if strings.Contains(out, "Directives") {
			t.Errorf("expected no header when no directives, got: %s", out)
		}
	})

	t.Run("town-level directive emits town header", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		dir := filepath.Join(townRoot, "directives")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "polecat.md"), []byte("Always be polite."), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}

		var buf bytes.Buffer
		outputRoleDirectives(ctx, &buf, false)
		out := buf.String()

		if !strings.Contains(out, "## Town Directives") {
			t.Errorf("expected Town Directives header, got: %s", out)
		}
		if !strings.Contains(out, "Always be polite.") {
			t.Errorf("expected directive content, got: %s", out)
		}
	})

	t.Run("rig-level directive emits rig header", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		dir := filepath.Join(townRoot, "myrig", "directives")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "witness.md"), []byte("Watch closely."), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RoleWitness,
			TownRoot: townRoot,
			Rig:      "myrig",
		}

		var buf bytes.Buffer
		outputRoleDirectives(ctx, &buf, false)
		out := buf.String()

		if !strings.Contains(out, "## Rig Directives") {
			t.Errorf("expected Rig Directives header, got: %s", out)
		}
		if !strings.Contains(out, "Watch closely.") {
			t.Errorf("expected directive content, got: %s", out)
		}
	})

	t.Run("both levels emits combined header", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()

		townDir := filepath.Join(townRoot, "directives")
		if err := os.MkdirAll(townDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(townDir, "polecat.md"), []byte("Town rule."), 0644); err != nil {
			t.Fatal(err)
		}

		rigDir := filepath.Join(townRoot, "myrig", "directives")
		if err := os.MkdirAll(rigDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rigDir, "polecat.md"), []byte("Rig rule."), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}

		var buf bytes.Buffer
		outputRoleDirectives(ctx, &buf, false)
		out := buf.String()

		if !strings.Contains(out, "## Town & Rig Directives") {
			t.Errorf("expected combined header, got: %s", out)
		}
		if !strings.Contains(out, "Town rule.") {
			t.Errorf("expected town content, got: %s", out)
		}
		if !strings.Contains(out, "Rig rule.") {
			t.Errorf("expected rig content, got: %s", out)
		}
	})

	t.Run("explain mode shows file paths", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}

		var buf bytes.Buffer
		outputRoleDirectives(ctx, &buf, true)
		out := buf.String()

		if !strings.Contains(out, "[EXPLAIN]") {
			t.Errorf("expected EXPLAIN output, got: %s", out)
		}
		if !strings.Contains(out, filepath.Join("directives", "polecat.md")) {
			t.Errorf("expected file path in explain output, got: %s", out)
		}
	})

	t.Run("empty rig name skips rig path", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()

		townDir := filepath.Join(townRoot, "directives")
		if err := os.MkdirAll(townDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(townDir, "mayor.md"), []byte("Mayor directive."), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RoleMayor,
			TownRoot: townRoot,
			Rig:      "",
		}

		var buf bytes.Buffer
		outputRoleDirectives(ctx, &buf, false)
		out := buf.String()

		if !strings.Contains(out, "## Town Directives") {
			t.Errorf("expected Town Directives header, got: %s", out)
		}
		if !strings.Contains(out, "Mayor directive.") {
			t.Errorf("expected directive content, got: %s", out)
		}
	})
}

func TestOutputDomainDocs(t *testing.T) {
	t.Parallel()

	t.Run("no domain dir emits nothing", func(t *testing.T) {
		t.Parallel()
		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: t.TempDir(),
			Rig:      "myrig",
		}
		var buf bytes.Buffer
		outputDomainDocs(ctx, &buf, false)
		if buf.Len() != 0 {
			t.Errorf("expected empty output, got: %s", buf.String())
		}
	})

	t.Run("emits TOC table not full content", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		body := "# Auth Flow\n\nOAuth2 flow details.\n\n## Deep section\n\nThis paragraph would have been inlined under the old behavior.\n"
		if err := os.WriteFile(filepath.Join(domainDir, "auth-flow.md"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}
		var buf bytes.Buffer
		outputDomainDocs(ctx, &buf, false)
		out := buf.String()

		if !strings.Contains(out, "## Domain Library (SME Reference)") {
			t.Errorf("expected library header, got: %s", out)
		}
		// TOC table headers and entry
		if !strings.Contains(out, "| Path | Topic | Last-touched |") {
			t.Errorf("expected TOC table header, got: %s", out)
		}
		if !strings.Contains(out, "`auth-flow.md`") {
			t.Errorf("expected doc path in TOC, got: %s", out)
		}
		if !strings.Contains(out, "OAuth2 flow details") {
			t.Errorf("expected summary in TOC, got: %s", out)
		}
		// Must NOT inline content past the first-paragraph summary
		if strings.Contains(out, "would have been inlined") {
			t.Errorf("expected TOC to not inline full body, got: %s", out)
		}
		if strings.Contains(out, "Deep section") {
			t.Errorf("expected TOC to not inline deeper headings, got: %s", out)
		}
		if !strings.Contains(out, "gt domain read") {
			t.Errorf("expected on-demand instructions, got: %s", out)
		}
	})

	t.Run("lists subdirectory docs with category in path", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		authDir := filepath.Join(townRoot, "myrig", "domain", "auth")
		if err := os.MkdirAll(authDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(authDir, "token.md"), []byte("# Token\n\nToken refresh contract.\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}
		var buf bytes.Buffer
		outputDomainDocs(ctx, &buf, false)
		out := buf.String()

		if !strings.Contains(out, filepath.Join("auth", "token.md")) {
			t.Errorf("expected category-prefixed relpath, got: %s", out)
		}
	})

	t.Run("explain mode shows debug info", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(domainDir, "api.md"), []byte("# API\n\nAPI docs"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}
		var buf bytes.Buffer
		outputDomainDocs(ctx, &buf, true)
		out := buf.String()

		if !strings.Contains(out, "[EXPLAIN]") {
			t.Errorf("expected explain output, got: %s", out)
		}
		if !strings.Contains(out, "indexed 1 files") {
			t.Errorf("expected file count in explain, got: %s", out)
		}
	})

	t.Run("TOC for 40 docs stays under 5KB", func(t *testing.T) {
		t.Parallel()
		townRoot := t.TempDir()
		domainDir := filepath.Join(townRoot, "myrig", "domain")
		if err := os.MkdirAll(domainDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Simulate a mature rig: 40 docs, each ~9KB of body (typical real size).
		bigBody := strings.Repeat("This is a long body line that would bloat a polecat's context window.\n", 130)
		for i := 0; i < 40; i++ {
			name := filepath.Join(domainDir, "doc-"+string(rune('a'+(i%26)))+string(rune('0'+(i/26)))+".md")
			if err := os.WriteFile(name, []byte("# Doc\n\n"+bigBody), 0644); err != nil {
				t.Fatal(err)
			}
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: townRoot,
			Rig:      "myrig",
		}
		var buf bytes.Buffer
		outputDomainDocs(ctx, &buf, false)
		if buf.Len() >= 5*1024 {
			t.Errorf("expected TOC <5KB for 40 docs, got %d bytes", buf.Len())
		}
	})

	t.Run("explain mode when no docs", func(t *testing.T) {
		t.Parallel()
		ctx := RoleContext{
			Role:     RolePolecat,
			TownRoot: t.TempDir(),
			Rig:      "myrig",
		}
		var buf bytes.Buffer
		outputDomainDocs(ctx, &buf, true)
		out := buf.String()

		if !strings.Contains(out, "[EXPLAIN]") {
			t.Errorf("expected explain output, got: %s", out)
		}
		if !strings.Contains(out, "none found") {
			t.Errorf("expected 'none found' in explain, got: %s", out)
		}
	})
}
