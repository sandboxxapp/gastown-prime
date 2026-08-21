package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// sentinelToken stands in for a real bearer credential. It is deliberately a
// long, distinctive, greppable string: every no-leak assertion in this file
// works by proving it never reaches stdout.
const sentinelToken = "SENTINEL.eyJhbGciOiJSUzI1NiJ9.NEVER-LOG-THIS-BEARER-CREDENTIAL.zzz"

// stubContextDBMint replaces the live IAM call for the duration of a test.
func stubContextDBMint(t *testing.T, token string, err error) {
	t.Helper()
	prev := contextDBMintFn
	contextDBMintFn = func(context.Context, string, string) (string, error) {
		return token, err
	}
	t.Cleanup(func() { contextDBMintFn = prev })
}

// writeTownSettings writes a settings/config.json carrying a context_db block
// and returns the town root.
func writeTownSettings(t *testing.T, cfg map[string]any) string {
	t.Helper()
	townRoot := t.TempDir()
	path := filepath.Join(townRoot, "settings", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := map[string]any{"type": "town-settings", "version": 1}
	for k, v := range cfg {
		body[k] = v
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return townRoot
}

// Output assertions in this file go through captureStdout (prime_test.go) — the
// production code reports via fmt.Printf, so redirecting os.Stdout is the only
// way to assert on what an operator (and daemon.log) actually sees.

// clearContextDBEnv unsets every var that steers resolution, so a test starts
// from a known state regardless of the operator's shell.
func clearContextDBEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvContextDBInject, EnvContextDBURL, EnvContextDBSA, EnvContextDBAudience} {
		t.Setenv(k, "")
	}
}

// TestResolveContextDBTargetDefaults pins the out-of-the-box behaviour: a town
// with no settings still resolves, and the audience defaults to the URL because
// that is what Cloud Run's front-end validates.
func TestResolveContextDBTargetDefaults(t *testing.T) {
	clearContextDBEnv(t)

	got, ok := resolveContextDBTarget(t.TempDir())
	if !ok {
		t.Fatal("injection should be enabled by default")
	}
	if got.URL != defaultContextDBURL {
		t.Errorf("URL = %q, want %q", got.URL, defaultContextDBURL)
	}
	if got.ServiceAccount != defaultContextDBSA {
		t.Errorf("ServiceAccount = %q, want %q", got.ServiceAccount, defaultContextDBSA)
	}
	if got.Audience != got.URL {
		t.Errorf("Audience = %q, want it to default to URL %q", got.Audience, got.URL)
	}

	// An empty town root must not be a special case — it is what a caller that
	// could not resolve the town passes, and it must still fail open, not panic.
	if _, ok := resolveContextDBTarget(""); !ok {
		t.Error("empty town root should still resolve to the compiled defaults")
	}
}

// TestResolveContextDBTargetPrecedence is the anti-hardcoding guarantee: a town
// that is not this one must be able to point every knob somewhere else. A
// hardcoded prod SA or run.app URL breaks every other town.
func TestResolveContextDBTargetPrecedence(t *testing.T) {
	clearContextDBEnv(t)

	townRoot := writeTownSettings(t, map[string]any{
		"context_db": map[string]any{
			"url":             "https://corpus.example.town",
			"service_account": "reader@other-project.iam.gserviceaccount.com",
			"audience":        "https://audience.example.town",
		},
	})

	// Settings beat the compiled defaults.
	got, ok := resolveContextDBTarget(townRoot)
	if !ok {
		t.Fatal("settings-configured town should resolve")
	}
	if got.URL != "https://corpus.example.town" {
		t.Errorf("settings URL not honoured: %q", got.URL)
	}
	if got.ServiceAccount != "reader@other-project.iam.gserviceaccount.com" {
		t.Errorf("settings SA not honoured: %q", got.ServiceAccount)
	}
	if got.Audience != "https://audience.example.town" {
		t.Errorf("settings audience not honoured (must not be overwritten by URL): %q", got.Audience)
	}

	// Env beats settings.
	t.Setenv(EnvContextDBURL, "https://env.example.town")
	t.Setenv(EnvContextDBSA, "env-reader@env-project.iam.gserviceaccount.com")
	t.Setenv(EnvContextDBAudience, "https://env-audience.example.town")
	got, ok = resolveContextDBTarget(townRoot)
	if !ok {
		t.Fatal("env-configured town should resolve")
	}
	if got.URL != "https://env.example.town" ||
		got.ServiceAccount != "env-reader@env-project.iam.gserviceaccount.com" ||
		got.Audience != "https://env-audience.example.town" {
		t.Errorf("env did not win over settings: %+v", got)
	}
}

// TestResolveContextDBTargetSuppression covers both opt-outs and, just as
// importantly, that nothing else opts a town out by accident.
func TestResolveContextDBTargetSuppression(t *testing.T) {
	t.Run("env falsey values suppress", func(t *testing.T) {
		clearContextDBEnv(t)
		for _, v := range []string{"0", "off", "false", "no", "OFF", " False "} {
			t.Setenv(EnvContextDBInject, v)
			if _, ok := resolveContextDBTarget(t.TempDir()); ok {
				t.Errorf("%s=%q should suppress injection", EnvContextDBInject, v)
			}
		}
	})

	t.Run("unset and truthy values inject", func(t *testing.T) {
		clearContextDBEnv(t)
		for _, v := range []string{"", "1", "on", "true", "auto"} {
			t.Setenv(EnvContextDBInject, v)
			if _, ok := resolveContextDBTarget(t.TempDir()); !ok {
				t.Errorf("%s=%q should inject", EnvContextDBInject, v)
			}
		}
	})

	t.Run("settings disabled suppresses", func(t *testing.T) {
		clearContextDBEnv(t)
		townRoot := writeTownSettings(t, map[string]any{
			"context_db": map[string]any{"disabled": true},
		})
		if _, ok := resolveContextDBTarget(townRoot); ok {
			t.Error(`"context_db": {"disabled": true} should suppress injection`)
		}
	})

	t.Run("blanked url or sa is an opt-out, not a broken mint", func(t *testing.T) {
		clearContextDBEnv(t)
		townRoot := writeTownSettings(t, map[string]any{
			"context_db": map[string]any{"url": "https://corpus.example.town"},
		})
		t.Setenv(EnvContextDBSA, " ")
		// A whitespace-only override is treated as absent, so the default SA
		// still applies — blanking must be done in settings, not by env noise.
		if _, ok := resolveContextDBTarget(townRoot); !ok {
			t.Error("whitespace env override should fall back, not disable")
		}
	})
}

// TestResolveContextDBEnvInjectsBothVars is the consumer contract: context-db/ctx
// reads CONTEXT_DB_TOKEN and the laws' curl recipe reads CONTEXT_DB_URL. Neither
// is useful alone, so they ship together or not at all.
func TestResolveContextDBEnvInjectsBothVars(t *testing.T) {
	clearContextDBEnv(t)
	stubContextDBMint(t, sentinelToken, nil)

	var env map[string]string
	var err error
	captureStdout(t, func() { env, err = ResolveContextDBEnv(t.TempDir()) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env[EnvContextDBToken] != sentinelToken {
		t.Errorf("%s not injected", EnvContextDBToken)
	}
	if env[EnvContextDBURL] != defaultContextDBURL {
		t.Errorf("%s = %q, want %q", EnvContextDBURL, env[EnvContextDBURL], defaultContextDBURL)
	}
	if len(env) != 2 {
		t.Errorf("injected %d vars, want exactly 2 (url + token): %v", len(env), keysOf(env))
	}
}

// TestResolveContextDBEnvFailsOpen is the load-bearing safety property. Corpus
// access is orientation-only: no ADC, no tokenCreator, or an offline laptop must
// degrade a polecat to rigs/<rig>/domain/, never abort its dispatch.
func TestResolveContextDBEnvFailsOpen(t *testing.T) {
	clearContextDBEnv(t)
	stubContextDBMint(t, "", fmt.Errorf("could not find default credentials"))

	var env map[string]string
	var err error
	out := captureStdout(t, func() { env, err = ResolveContextDBEnv(t.TempDir()) })
	if err == nil {
		t.Fatal("mint failure must be reported to the caller")
	}
	if env != nil {
		t.Errorf("no env may be injected on failure, got %v", keysOf(env))
	}
	// All-or-nothing: a URL with no token is worse than nothing — the polecat
	// would burn its first orientation attempt on a 403 instead of falling back.
	if strings.Contains(out, EnvContextDBURL+"=") {
		t.Errorf("URL must not be injected without a token: %q", out)
	}
	// The error must name the SA so an operator can fix the IAM binding.
	if !strings.Contains(err.Error(), defaultContextDBSA) {
		t.Errorf("error should name the minting SA, got %q", err)
	}

	// And the call-site helper must swallow it: extraEnv comes back intact and
	// the dispatch is free to continue.
	existing := map[string]string{"CLOUDSDK_AUTH_ACCESS_TOKEN": "gcp-token", "FOO": "bar"}
	var got map[string]string
	out = captureStdout(t, func() { got = injectContextDBEnv(t.TempDir(), existing) })
	if len(got) != 2 || got["FOO"] != "bar" || got["CLOUDSDK_AUTH_ACCESS_TOKEN"] != "gcp-token" {
		t.Errorf("failed injection must leave extraEnv untouched, got %v", got)
	}
	if !strings.Contains(out, "NOT injected") {
		t.Errorf("failure must be reported LOUDLY on stdout, got %q", out)
	}
	if !strings.Contains(out, "dispatch continues") {
		t.Errorf("operator must be told the dispatch is not aborted, got %q", out)
	}
}

// TestInjectContextDBEnvMergesWithoutClobbering guards composition with --gcp
// and --secrets, which write the same extraEnv map.
func TestInjectContextDBEnvMergesWithoutClobbering(t *testing.T) {
	clearContextDBEnv(t)
	stubContextDBMint(t, sentinelToken, nil)

	existing := map[string]string{
		"CLOUDSDK_AUTH_ACCESS_TOKEN":     "gcp-token",
		"GOOGLE_APPLICATION_CREDENTIALS": "/dev/null",
	}
	var got map[string]string
	captureStdout(t, func() { got = injectContextDBEnv(t.TempDir(), existing) })

	// The --gcp keys survive: injection works precisely BECAUSE the token was
	// minted operator-side, so blocked ADC in the session is irrelevant to it.
	if got["GOOGLE_APPLICATION_CREDENTIALS"] != "/dev/null" || got["CLOUDSDK_AUTH_ACCESS_TOKEN"] != "gcp-token" {
		t.Errorf("--gcp env clobbered: %v", keysOf(got))
	}
	if got[EnvContextDBToken] != sentinelToken || got[EnvContextDBURL] == "" {
		t.Errorf("context-db env not merged in: %v", keysOf(got))
	}

	// A nil extraEnv (the common case — no --gcp, no --secrets) must still work.
	captureStdout(t, func() { got = injectContextDBEnv(t.TempDir(), nil) })
	if got[EnvContextDBToken] != sentinelToken {
		t.Error("injection onto a nil extraEnv should still produce the token")
	}
}

// TestContextDBEnvNeverLogsToken is a security assertion, not a style check.
// CONTEXT_DB_TOKEN is a bearer credential; anything printed here lands in the
// operator's terminal and in daemon.log. The length and the SA are reportable;
// the token is not.
func TestContextDBEnvNeverLogsToken(t *testing.T) {
	clearContextDBEnv(t)
	stubContextDBMint(t, sentinelToken, nil)

	out := captureStdout(t, func() {
		if _, err := ResolveContextDBEnv(t.TempDir()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		_ = injectContextDBEnv(t.TempDir(), nil)
	})

	if strings.Contains(out, sentinelToken) {
		t.Fatal("THE TOKEN WAS LOGGED. CONTEXT_DB_TOKEN is a bearer credential.")
	}
	// Not just the whole string: no substantial fragment may leak either, which
	// is how a "helpfully truncated" preview would slip through review.
	for _, frag := range []string{
		sentinelToken[:24],
		sentinelToken[len(sentinelToken)-24:],
		"NEVER-LOG-THIS",
	} {
		if strings.Contains(out, frag) {
			t.Fatalf("token fragment %q leaked to stdout: %q", frag, out)
		}
	}
	// Positive control: the confirmation line must still be informative enough
	// to debug with — length, identity, and the no-refresh expiry contract.
	if !strings.Contains(out, strconv.Itoa(len(sentinelToken))+" chars") {
		t.Errorf("expected the token LENGTH to be reported, got %q", out)
	}
	if !strings.Contains(out, defaultContextDBSA) {
		t.Errorf("expected the minting SA to be reported, got %q", out)
	}
	if !strings.Contains(out, "no refresh") {
		t.Errorf("expected the no-refresh expiry contract to be stated, got %q", out)
	}
}

// TestResolveContextDBEnvSuppressedIsSilent: a town that opted out should get no
// env and no noise — an unexplained warning on every dispatch trains operators
// to ignore the channel that carries the real failures.
func TestResolveContextDBEnvSuppressedIsSilent(t *testing.T) {
	clearContextDBEnv(t)
	t.Setenv(EnvContextDBInject, "0")
	stubContextDBMint(t, sentinelToken, fmt.Errorf("must not be called"))

	var env map[string]string
	var err error
	out := captureStdout(t, func() { env, err = ResolveContextDBEnv(t.TempDir()) })
	if err != nil || env != nil {
		t.Errorf("suppressed injection should be a silent no-op, got env=%v err=%v", keysOf(env), err)
	}
	if out != "" {
		t.Errorf("suppressed injection should print nothing, got %q", out)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
