package authzproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseMCPPolicy(t *testing.T) {
	tests := []struct {
		spec     string
		wantName string
		wantMode string
		wantErr  bool
	}{
		{"github:read", "github", "read", false},
		{"linear:read,write", "linear", "read,write", false},
		{"vanta:write", "vanta", "write", false},
		{"github", "github", "read", false},      // default mode
		{"context7:", "context7", "read", false}, // empty mode = default
		{"", "", "", true},                       // empty spec
		{":read", "", "", true},                  // empty name
		{"github:invalid", "", "", true},         // invalid mode
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			name, policy, err := ParseMCPPolicy(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMCPPolicy(%q): err=%v, wantErr=%v", tt.spec, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if name != tt.wantName {
				t.Errorf("name=%q, want=%q", name, tt.wantName)
			}
			if policy.Mode != tt.wantMode {
				t.Errorf("mode=%q, want=%q", policy.Mode, tt.wantMode)
			}
			if len(policy.Tools) != 1 || policy.Tools[0] != "*" {
				t.Errorf("tools=%v, want=[*]", policy.Tools)
			}
		})
	}
}

func TestGenerateAuthzFile(t *testing.T) {
	dir := t.TempDir()
	authzDir := filepath.Join(dir, "bridge")

	ctx := AuthzContext{
		Role:    "polecat",
		AgentID: "gastown/polecats/Toast",
		Bead:    "gt-abc123",
		MCPs: map[string]MCPPolicy{
			"github": {Mode: "read", Tools: []string{"*"}},
			"linear": {Mode: "read,write", Tools: []string{"*"}},
		},
		GCP: &GCPAuthz{
			Profiles: map[string]GCPProfile{
				"terraform-plan": {
					TargetSA: "terraform-plan@proj.iam.gserviceaccount.com",
					Scopes:   []string{"https://www.googleapis.com/auth/compute.readonly"},
					Lifetime: "3600s",
				},
			},
		},
	}

	path, err := GenerateAuthzFile(authzDir, ctx)
	if err != nil {
		t.Fatalf("GenerateAuthzFile: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("authz file not found: %v", err)
	}

	// Parse and verify contents
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading authz file: %v", err)
	}
	var got AuthzContext
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing authz file: %v", err)
	}

	if got.Role != "polecat" {
		t.Errorf("role=%q, want=polecat", got.Role)
	}
	if got.AgentID != "gastown/polecats/Toast" {
		t.Errorf("agent_id=%q, want=gastown/polecats/Toast", got.AgentID)
	}
	if got.Bead != "gt-abc123" {
		t.Errorf("bead=%q, want=gt-abc123", got.Bead)
	}
	if len(got.MCPs) != 2 {
		t.Errorf("len(mcps)=%d, want=2", len(got.MCPs))
	}
	if got.MCPs["github"].Mode != "read" {
		t.Errorf("github mode=%q, want=read", got.MCPs["github"].Mode)
	}
	if got.MCPs["linear"].Mode != "read,write" {
		t.Errorf("linear mode=%q, want=read,write", got.MCPs["linear"].Mode)
	}
	if got.GCP == nil || len(got.GCP.Profiles) != 1 {
		t.Fatalf("gcp profiles missing or wrong count")
	}
	if got.GCP.Profiles["terraform-plan"].TargetSA != "terraform-plan@proj.iam.gserviceaccount.com" {
		t.Errorf("gcp target_sa mismatch")
	}
}

func TestGenerateMCPConfig(t *testing.T) {
	dir := t.TempDir()
	authzDir := filepath.Join(dir, "bridge")
	worktreeRoot := filepath.Join(dir, "worktree")
	if err := os.MkdirAll(worktreeRoot, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := AuthzContext{
		Role:    "polecat",
		AgentID: "gastown/polecats/Toast",
		Bead:    "gt-abc",
		MCPs: map[string]MCPPolicy{
			"github": {Mode: "read", Tools: []string{"*"}},
			"linear": {Mode: "read,write", Tools: []string{"*"}},
		},
	}

	authzPath, err := GenerateAuthzFile(authzDir, ctx)
	if err != nil {
		t.Fatalf("GenerateAuthzFile: %v", err)
	}

	cfg := Config{
		Binary: "/usr/local/bin/authz-proxy",
		Socket: "/tmp/mcp-proxy.sock",
	}
	mcpPath, err := GenerateMCPConfig(worktreeRoot, authzPath, cfg)
	if err != nil {
		t.Fatalf("GenerateMCPConfig: %v", err)
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("reading .mcp.json: %v", err)
	}
	var got MCPConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}

	if len(got.MCPServers) != 2 {
		t.Fatalf("len(mcpServers)=%d, want=2", len(got.MCPServers))
	}

	for _, name := range []string{"github", "linear"} {
		entry, ok := got.MCPServers[name]
		if !ok {
			t.Errorf("missing MCP server entry for %q", name)
			continue
		}
		if entry.Command != cfg.Binary {
			t.Errorf("%s command=%q, want=%q", name, entry.Command, cfg.Binary)
		}
		wantArgs := []string{"frontend", "--authz", authzPath, "--socket", cfg.Socket}
		if len(entry.Args) != len(wantArgs) {
			t.Errorf("%s args=%v, want=%v", name, entry.Args, wantArgs)
		} else {
			for i, a := range entry.Args {
				if a != wantArgs[i] {
					t.Errorf("%s args[%d]=%q, want=%q", name, i, a, wantArgs[i])
				}
			}
		}
	}
}

func TestMCPToolPermissions(t *testing.T) {
	perms := MCPToolPermissions([]string{"github", "linear"})
	if len(perms) != 2 {
		t.Fatalf("len=%d, want=2", len(perms))
	}
	if perms[0] != "mcp__github__*" {
		t.Errorf("perms[0]=%q, want=mcp__github__*", perms[0])
	}
	if perms[1] != "mcp__linear__*" {
		t.Errorf("perms[1]=%q, want=mcp__linear__*", perms[1])
	}
}

func TestResolveSecretProfiles(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, ".mcp-secrets.json")

	secrets := SecretsFile{
		SecretProfiles: map[string]SecretProfile{
			"community-admin": {
				Source: "/some/prod.env",
				Vars:   []string{"COMMUNITY_ADMIN_V2_TOKEN", "COMMUNITY_ID"},
			},
		},
	}
	data, _ := json.Marshal(secrets)
	if err := os.WriteFile(secretsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("resolve existing profile", func(t *testing.T) {
		profiles, err := ResolveSecretProfiles(secretsPath, []string{"community-admin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(profiles) != 1 {
			t.Fatalf("len=%d, want=1", len(profiles))
		}
		p := profiles["community-admin"]
		if p.Source != "/some/prod.env" {
			t.Errorf("source mismatch: %q", p.Source)
		}
		if len(p.Vars) != 2 {
			t.Errorf("vars len=%d, want=2", len(p.Vars))
		}
	})

	t.Run("missing profile", func(t *testing.T) {
		if _, err := ResolveSecretProfiles(secretsPath, []string{"nope"}); err == nil {
			t.Fatal("expected error for missing profile")
		}
	})

	t.Run("empty profile names", func(t *testing.T) {
		profiles, err := ResolveSecretProfiles(secretsPath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profiles != nil {
			t.Errorf("expected nil, got %v", profiles)
		}
	})

	t.Run("missing secrets path", func(t *testing.T) {
		if _, err := ResolveSecretProfiles("", []string{"community-admin"}); err == nil {
			t.Fatal("expected error for empty secrets path")
		}
	})
}

func TestLoadProfileEnv(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "prod.env")
	content := `# prod credentials
export COMMUNITY_ADMIN_V2_TOKEN="tok-v2-secret"
COMMUNITY_ID=12345
COMMUNITY_ADMIN_V1_TOKEN='tok-v1-secret'
UNRELATED_VAR=ignore-me
`
	if err := os.WriteFile(sourcePath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("loads only listed vars", func(t *testing.T) {
		env, err := LoadProfileEnv("community-admin", SecretProfile{
			Source: sourcePath,
			Vars:   []string{"COMMUNITY_ADMIN_V2_TOKEN", "COMMUNITY_ID"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env["COMMUNITY_ADMIN_V2_TOKEN"] != "tok-v2-secret" {
			t.Errorf("v2 token mismatch: %q", env["COMMUNITY_ADMIN_V2_TOKEN"])
		}
		if env["COMMUNITY_ID"] != "12345" {
			t.Errorf("community id mismatch: %q", env["COMMUNITY_ID"])
		}
		// Vars not in the profile must NOT be injected (minimal set guarantee).
		if _, ok := env["UNRELATED_VAR"]; ok {
			t.Error("UNRELATED_VAR leaked — profile must inject only its listed vars")
		}
		if _, ok := env["COMMUNITY_ADMIN_V1_TOKEN"]; ok {
			t.Error("COMMUNITY_ADMIN_V1_TOKEN leaked — not in profile.Vars")
		}
	})

	t.Run("missing var in source errors", func(t *testing.T) {
		_, err := LoadProfileEnv("community-admin", SecretProfile{
			Source: sourcePath,
			Vars:   []string{"COMMUNITY_ADMIN_V2_TOKEN", "DOES_NOT_EXIST"},
		})
		if err == nil {
			t.Fatal("expected error when a listed var is absent from source")
		}
	})

	t.Run("unreadable source errors", func(t *testing.T) {
		_, err := LoadProfileEnv("x", SecretProfile{
			Source: filepath.Join(dir, "no-such-file.env"),
			Vars:   []string{"FOO"},
		})
		if err == nil {
			t.Fatal("expected error for unreadable source")
		}
	})

	t.Run("empty source errors", func(t *testing.T) {
		if _, err := LoadProfileEnv("x", SecretProfile{Vars: []string{"FOO"}}); err == nil {
			t.Fatal("expected error for empty source")
		}
	})

	t.Run("no vars errors", func(t *testing.T) {
		if _, err := LoadProfileEnv("x", SecretProfile{Source: sourcePath}); err == nil {
			t.Fatal("expected error for profile with no vars")
		}
	})
}

func TestParseDotenv(t *testing.T) {
	content := `# a comment
export FOO=bar
QUOTED="has spaces"
SINGLE='single'
EMPTY=
WITH_HASH=value#notacomment
  SPACED = trimmed
malformed line with no equals
=novalue
`
	env := ParseDotenv(content)

	cases := map[string]string{
		"FOO":       "bar",
		"QUOTED":    "has spaces",
		"SINGLE":    "single",
		"EMPTY":     "",
		"WITH_HASH": "value#notacomment",
		"SPACED":    "trimmed",
	}
	for k, want := range cases {
		if got, ok := env[k]; !ok || got != want {
			t.Errorf("%s = %q (ok=%v), want %q", k, got, ok, want)
		}
	}
	if _, ok := env["malformed line with no equals"]; ok {
		t.Error("malformed line should be skipped")
	}
	if _, ok := env[""]; ok {
		t.Error("empty key should be skipped")
	}
}

func TestResolveGCPProfiles(t *testing.T) {
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, ".mcp-secrets.json")

	secrets := SecretsFile{
		GCPProfiles: map[string]GCPProfile{
			"terraform-plan": {
				TargetSA: "terraform-plan@proj.iam.gserviceaccount.com",
				Scopes:   []string{"https://www.googleapis.com/auth/compute.readonly"},
				Lifetime: "3600s",
			},
			"deploy": {
				TargetSA: "deploy@proj.iam.gserviceaccount.com",
				Scopes:   []string{"https://www.googleapis.com/auth/cloud-platform"},
			},
		},
	}
	data, _ := json.Marshal(secrets)
	if err := os.WriteFile(secretsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("resolve existing profiles", func(t *testing.T) {
		profiles, err := ResolveGCPProfiles(secretsPath, []string{"terraform-plan"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(profiles) != 1 {
			t.Fatalf("len=%d, want=1", len(profiles))
		}
		if profiles["terraform-plan"].TargetSA != "terraform-plan@proj.iam.gserviceaccount.com" {
			t.Error("target_sa mismatch")
		}
	})

	t.Run("missing profile", func(t *testing.T) {
		_, err := ResolveGCPProfiles(secretsPath, []string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error for missing profile")
		}
	})

	t.Run("empty profiles", func(t *testing.T) {
		profiles, err := ResolveGCPProfiles(secretsPath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profiles != nil {
			t.Errorf("expected nil for empty profile names, got %v", profiles)
		}
	})

	t.Run("missing secrets path", func(t *testing.T) {
		_, err := ResolveGCPProfiles("", []string{"terraform-plan"})
		if err == nil {
			t.Fatal("expected error for empty secrets path")
		}
	})
}

func TestGenerateAuthzFile_NoGCP(t *testing.T) {
	dir := t.TempDir()

	ctx := AuthzContext{
		Role:    "polecat",
		AgentID: "gastown/polecats/Fern",
		Bead:    "gt-xyz",
		MCPs: map[string]MCPPolicy{
			"github": {Mode: "read", Tools: []string{"*"}},
		},
	}

	path, err := GenerateAuthzFile(dir, ctx)
	if err != nil {
		t.Fatalf("GenerateAuthzFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	var got AuthzContext
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if got.GCP != nil {
		t.Error("expected nil GCP when not set")
	}
	if len(got.MCPs) != 1 {
		t.Errorf("len(mcps)=%d, want=1", len(got.MCPs))
	}
}
