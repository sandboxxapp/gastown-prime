package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTownSecretsConfig writes a minimal town settings/config.json pointing
// authz_proxy.secrets_path at the given secrets file, and returns the townRoot.
func writeTownSecretsConfig(t *testing.T, secretsPath string) string {
	t.Helper()
	townRoot := t.TempDir()
	settingsDir := filepath.Join(townRoot, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
  "authz_proxy": {
    "binary": "/tmp/authz-proxy",
    "socket": "/tmp/mcp-proxy.sock",
    "secrets_path": "` + secretsPath + `"
  }
}`
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return townRoot
}

func TestResolveSecretsEnv(t *testing.T) {
	dir := t.TempDir()

	sourcePath := filepath.Join(dir, "prod.env")
	source := `COMMUNITY_ADMIN_V2_TOKEN=tok-v2
COMMUNITY_ID=999
OTHER=nope
`
	if err := os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}

	secretsPath := filepath.Join(dir, ".mcp-secrets.json")
	secrets := `{
  "secret_profiles": {
    "community-admin": {
      "source": "` + sourcePath + `",
      "vars": ["COMMUNITY_ADMIN_V2_TOKEN", "COMMUNITY_ID"]
    }
  }
}`
	if err := os.WriteFile(secretsPath, []byte(secrets), 0600); err != nil {
		t.Fatal(err)
	}

	townRoot := writeTownSecretsConfig(t, secretsPath)

	t.Run("no profiles returns nil", func(t *testing.T) {
		env, err := ResolveSecretsEnv(townRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env, got %v", env)
		}
	})

	t.Run("resolves profile into env (minimal set only)", func(t *testing.T) {
		env, err := ResolveSecretsEnv(townRoot, []string{"community-admin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env["COMMUNITY_ADMIN_V2_TOKEN"] != "tok-v2" {
			t.Errorf("v2 token mismatch: %q", env["COMMUNITY_ADMIN_V2_TOKEN"])
		}
		if env["COMMUNITY_ID"] != "999" {
			t.Errorf("community id mismatch: %q", env["COMMUNITY_ID"])
		}
		if _, ok := env["OTHER"]; ok {
			t.Error("OTHER leaked — only profile.Vars should be injected")
		}
	})

	t.Run("unknown profile errors", func(t *testing.T) {
		if _, err := ResolveSecretsEnv(townRoot, []string{"ghost"}); err == nil {
			t.Fatal("expected error for unknown profile")
		}
	})
}

func TestMergeEnv(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		if got := mergeEnv(nil, nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("union with add winning", func(t *testing.T) {
		base := map[string]string{"A": "1", "B": "2"}
		add := map[string]string{"B": "override", "C": "3"}
		got := mergeEnv(base, add)
		want := map[string]string{"A": "1", "B": "override", "C": "3"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
		// base must not be mutated
		if base["B"] != "2" {
			t.Error("mergeEnv mutated base map")
		}
	})

	t.Run("nil base", func(t *testing.T) {
		got := mergeEnv(nil, map[string]string{"X": "y"})
		if got["X"] != "y" {
			t.Errorf("expected X=y, got %v", got)
		}
	})
}

// TestSlingSecretsFlag verifies the --secrets flag is registered as a repeatable
// string array on the sling command, mirroring --gcp.
func TestSlingSecretsFlag(t *testing.T) {
	f := slingCmd.Flags().Lookup("secrets")
	if f == nil {
		t.Fatal("--secrets flag not registered on slingCmd")
	}
	if f.Value.Type() != "stringArray" {
		t.Errorf("--secrets type = %q, want stringArray (repeatable)", f.Value.Type())
	}
}
