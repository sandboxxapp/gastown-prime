package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_NewStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store, err := NewStore(path, KnownProviders, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store.Path != path {
		t.Errorf("path = %q", store.Path)
	}
	if len(store.Tokens) != 0 {
		t.Errorf("tokens should be empty, got %d", len(store.Tokens))
	}
}

func TestStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")

	store, _ := NewStore(path, KnownProviders, nil)
	store.Tokens["linear"] = &Token{
		AccessToken: "lin_test_123",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload
	store2, _ := NewStore(path, KnownProviders, nil)
	if _, ok := store2.Tokens["linear"]; !ok {
		t.Fatal("linear token not loaded")
	}
	if store2.Tokens["linear"].AccessToken != "lin_test_123" {
		t.Errorf("token = %q", store2.Tokens["linear"].AccessToken)
	}
}

func TestToken_IsExpired(t *testing.T) {
	fresh := &Token{ExpiresAt: time.Now().Add(time.Hour)}
	if fresh.IsExpired() {
		t.Error("fresh token should not be expired")
	}

	old := &Token{ExpiresAt: time.Now().Add(-time.Hour)}
	if !old.IsExpired() {
		t.Error("old token should be expired")
	}

	// Within 5-minute grace period
	grace := &Token{ExpiresAt: time.Now().Add(3 * time.Minute)}
	if !grace.IsExpired() {
		t.Error("token within 5-min grace should be considered expired")
	}
}

func TestStore_EnvForUpstream_ValidToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	store, _ := NewStore(path, KnownProviders, nil)
	store.Tokens["linear"] = &Token{
		AccessToken: "lin_valid",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	env, err := store.EnvForUpstream("linear", UpstreamConfig{})
	if err != nil {
		t.Fatalf("EnvForUpstream: %v", err)
	}
	if env["LINEAR_API_KEY"] != "lin_valid" {
		t.Errorf("env = %v", env)
	}
}

func TestStore_EnvForUpstream_NoToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	store, _ := NewStore(path, KnownProviders, nil)

	env, err := store.EnvForUpstream("github", UpstreamConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != nil {
		t.Errorf("expected nil env, got %v", env)
	}
}

func TestStore_EnvForUpstream_DefaultEnvKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	store, _ := NewStore(path, nil, nil)
	store.Tokens["github"] = &Token{
		AccessToken: "ghp_test",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	env, err := store.EnvForUpstream("github", UpstreamConfig{})
	if err != nil {
		t.Fatalf("EnvForUpstream: %v", err)
	}
	if env["GITHUB_ACCESS_TOKEN"] != "ghp_test" {
		t.Errorf("env = %v", env)
	}
}

func TestLoadSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	secrets := map[string]interface{}{
		"github": map[string]interface{}{
			"command": "npx",
			"args":    []string{"-y", "@modelcontextprotocol/server-github"},
			"env":     map[string]string{"GITHUB_TOKEN": "ghp_xxx"},
		},
		"gcp_profiles": map[string]interface{}{
			"terraform-plan": map[string]string{"target_sa": "sa@proj"},
		},
	}
	data, _ := json.Marshal(secrets)
	os.WriteFile(path, data, 0644)

	loaded, err := LoadSecrets(path)
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}
	if len(loaded) != 1 {
		t.Errorf("expected 1 MCP entry, got %d", len(loaded))
	}
	if _, ok := loaded["github"]; !ok {
		t.Error("github entry missing")
	}
	if _, ok := loaded["gcp_profiles"]; ok {
		t.Error("gcp_profiles should be filtered out (no command)")
	}
}

func TestKnownProviders(t *testing.T) {
	if _, ok := KnownProviders["linear"]; !ok {
		t.Error("linear should be a known provider")
	}
}
