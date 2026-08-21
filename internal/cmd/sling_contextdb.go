package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"golang.org/x/oauth2/google"
)

// One-way context-db credential injection.
//
// A dispatched polecat's FIRST law is "orient through the context-db before you
// scope the build" (laws-polecat, law #0). The corpus sits behind an
// authenticated Cloud Run front-end, so that law is only executable if the
// polecat holds an audience-scoped ID token. This file mints that token in the
// SLING process — which runs as the operator — and injects it into the
// dispatched session's env. The polecat never authenticates; it only receives.
//
// WHY INJECTION AND NOT SELF-MINT. Both candidate identities (polecat-ro and
// ctx-db-invoker) grant roles/iam.serviceAccountTokenCreator to exactly one
// principal: user:eric@sandboxx.us. A polecat that mints its own token is
// therefore not using a polecat identity at all — it is silently inheriting the
// operator's ADC, which happens to work only because the session is local. That
// inheritance also breaks the moment it should: under `gt sling --gcp`, which
// deliberately sets GOOGLE_APPLICATION_CREDENTIALS=/dev/null and sandboxes
// CLOUDSDK_CONFIG (see MintGCPToken). Minting operator-side and injecting fixes
// identity, inheritance and --gcp in one move.
//
// SECURITY: CONTEXT_DB_TOKEN is a bearer credential. It is never logged, never
// echoed, never written to a bead, a formula render, prime output or a PR body.
// Only its length and the minting SA are reported. TestContextDBEnvNeverLogsToken
// asserts this.
//
// EXPIRY: the ID token lives ~1h (IAM's fixed lifetime for generateIdToken;
// measured exp-iat = 3600). There is deliberately NO refresh machinery. Corpus
// access is orientation-only and fail-open, so an expired token degrades to the
// same place a missing one does: the on-disk laws and rigs/<rig>/domain/. Do not
// "fix" this by adding a refresher — a long-lived refresh loop would hand a
// polecat a renewable credential, which is exactly what one-way injection is
// designed to avoid.

const (
	// EnvContextDBInject disables injection for a town when set to a falsey
	// value ("0", "off", "false", "no"). Mirrors EnvPrimeLaws: default-on,
	// because a town that cannot reach the corpus already fails open.
	EnvContextDBInject = "GT_CONTEXT_DB_INJECT"

	// EnvContextDBURL is the context-db base URL, and doubles as the token
	// audience unless EnvContextDBAudience overrides it. Already a recognised
	// var elsewhere in gt (see internal/config/env.go, prime_context_db.go).
	EnvContextDBURL = "CONTEXT_DB_URL"

	// EnvContextDBToken is the injected bearer token. NEVER read from the
	// operator's env — only ever written into the dispatched session.
	EnvContextDBToken = "CONTEXT_DB_TOKEN"

	// EnvContextDBSA overrides the service account the token is minted as.
	EnvContextDBSA = "GT_CONTEXT_DB_SA"

	// EnvContextDBAudience overrides the token audience. Defaults to the URL,
	// which is what Cloud Run's front-end validates against.
	EnvContextDBAudience = "GT_CONTEXT_DB_AUDIENCE"
)

// Compiled-in defaults. These are DEFAULTS, not constants of the mechanism:
// every one is overridable by settings/config.json `context_db` and then by env.
// A town with different values sets them and nothing here needs to change; a
// town with no context-db at all fails the mint once per dispatch and proceeds.
const (
	defaultContextDBURL = "https://context-db-2hazwepeiq-uc.a.run.app"
	defaultContextDBSA  = "polecat-ro@sandboxx-prod-01.iam.gserviceaccount.com"
)

// contextDBMintTimeout caps the whole mint (ADC token + IAM round-trip). The
// IAM generateIdToken call measures ~100-170ms; 3s is a ceiling that keeps a
// hung network well inside the dispatch-latency budget rather than a target.
const contextDBMintTimeout = 3 * time.Second

// contextDBTarget is the resolved injection configuration for one dispatch.
type contextDBTarget struct {
	URL            string
	ServiceAccount string
	Audience       string
}

// contextDBMintFn is the seam tests replace. Production is mintContextDBIDToken.
var contextDBMintFn = mintContextDBIDToken

// contextDBInjectSuppressed reports whether injection is turned off for this
// town, either by GT_CONTEXT_DB_INJECT or by `"context_db": {"disabled": true}`.
func contextDBInjectSuppressed(cfg *config.ContextDBConfig) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvContextDBInject))) {
	case "0", "off", "false", "no":
		return true
	}
	return cfg != nil && cfg.Disabled
}

// resolveContextDBTarget resolves URL / service account / audience with
// precedence env > town settings > compiled default, and reports whether
// injection should be attempted at all.
//
// A settings-load failure is not an error here: a town with no
// settings/config.json is a normal town, and injection still works off the
// defaults. That is deliberate — this path must never be able to fail a sling.
func resolveContextDBTarget(townRoot string) (contextDBTarget, bool) {
	var cfg *config.ContextDBConfig
	if townRoot != "" {
		if settings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(townRoot)); err == nil {
			cfg = settings.ContextDB
		}
	}

	if contextDBInjectSuppressed(cfg) {
		return contextDBTarget{}, false
	}

	t := contextDBTarget{URL: defaultContextDBURL, ServiceAccount: defaultContextDBSA}
	if cfg != nil {
		if cfg.URL != "" {
			t.URL = cfg.URL
		}
		if cfg.ServiceAccount != "" {
			t.ServiceAccount = cfg.ServiceAccount
		}
		if cfg.Audience != "" {
			t.Audience = cfg.Audience
		}
	}
	if v := strings.TrimSpace(os.Getenv(EnvContextDBURL)); v != "" {
		t.URL = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvContextDBSA)); v != "" {
		t.ServiceAccount = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvContextDBAudience)); v != "" {
		t.Audience = v
	}

	// Cloud Run validates the ID token's `aud` against the service URL, so the
	// URL is the audience unless a town deliberately splits them (e.g. a custom
	// domain fronting the run.app service).
	if t.Audience == "" {
		t.Audience = t.URL
	}

	// An explicitly blanked URL or SA is an opt-out, not a misconfiguration.
	if t.URL == "" || t.ServiceAccount == "" {
		return contextDBTarget{}, false
	}
	return t, true
}

// mintContextDBIDToken mints an audience-scoped Google ID token AS the given
// service account, using the calling process's ADC to impersonate it.
//
// This is a direct IAM Credentials REST call, matching the in-repo pattern in
// internal/authzproxy/gcp_mint.go — deliberately NOT a shell-out to
// `gcloud auth print-identity-token --impersonate-service-account`. gcloud
// writes an impersonation WARNING to stderr on every mint, and a caller that
// folds stderr into stdout (`2>&1`) silently concatenates that warning onto the
// token: measured 808 clean chars vs 964 corrupt, whose only symptom is curl
// exit 43 — a failure that reads like TLS and sends you debugging the wrong
// layer (context-db/scripts/mint-ctx-token.sh header). Speaking the API
// directly makes that class of corruption unrepresentable: there is no second
// stream to confuse with the token.
func mintContextDBIDToken(ctx context.Context, serviceAccount, audience string) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("finding default credentials: %w", err)
	}
	srcToken, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("getting ADC token: %w", err)
	}

	// includeEmail is required for the ID token to carry the SA identity in the
	// `email` claim; without it the corpus cannot attribute the caller.
	body, err := json.Marshal(map[string]any{
		"audience":     audience,
		"includeEmail": true,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateIdToken",
		serviceAccount)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+srcToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("IAM generateIdToken: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The IAM error body names the missing role/SA and contains no secret.
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("IAM generateIdToken %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	if tokenResp.Token == "" {
		return "", fmt.Errorf("IAM returned an empty token")
	}
	return tokenResp.Token, nil
}

// ResolveContextDBEnv mints a read-only context-db token for a dispatched
// polecat and returns the env vars to layer onto its spawn environment.
//
// FAIL OPEN, LOUDLY. Every failure mode — no ADC, no tokenCreator on the SA,
// offline, a town with no corpus — returns (nil, err). Callers MUST print the
// error and continue: corpus access is orientation-only and degrades to
// rigs/<rig>/domain/laws/<role>.md, which the laws name unconditionally. A
// dispatch must never be aborted because the corpus was unreachable.
//
// Returns (nil, nil) when injection is suppressed or unconfigured for the town.
//
// MASKING: logs the token LENGTH and the minting SA, never the token.
func ResolveContextDBEnv(townRoot string) (map[string]string, error) {
	target, ok := resolveContextDBTarget(townRoot)
	if !ok {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), contextDBMintTimeout)
	defer cancel()

	token, err := contextDBMintFn(ctx, target.ServiceAccount, target.Audience)
	if err != nil {
		return nil, fmt.Errorf("minting context-db token as %s: %w", target.ServiceAccount, err)
	}

	// All-or-nothing: a URL without a token is worse than nothing, because the
	// Cloud Run front-end rejects unauthenticated calls and the polecat would
	// burn its first orientation attempt on a 403 instead of going straight to
	// the on-disk fallback.
	fmt.Printf("  %s context-db read token injected (%d chars, as %s, ~1h, no refresh)\n",
		style.Bold.Render("✓"), len(token), target.ServiceAccount)

	return map[string]string{
		EnvContextDBURL:   target.URL,
		EnvContextDBToken: token,
	}, nil
}

// injectContextDBEnv is the shared call-site helper: mint, report, merge, never
// fail the dispatch. Both the interactive (runSling) and batch (executeSling)
// paths use it so the two cannot drift — the --gcp mint gap documented in
// rigs/gastown-prime/domain/secrets-injection.md exists precisely because they did.
func injectContextDBEnv(townRoot string, extraEnv map[string]string) map[string]string {
	env, err := ResolveContextDBEnv(townRoot)
	if err != nil {
		// Loud, single line, and explicitly non-fatal — the polecat is about to
		// run law #0 and needs to know why the door is shut.
		fmt.Printf("  %s context-db token NOT injected: %v\n", style.Warning.Render("⚠"), err)
		fmt.Printf("  %s Polecat orientation degrades to rigs/<rig>/domain/ (dispatch continues)\n",
			style.Dim.Render("→"))
		return extraEnv
	}
	if env == nil {
		return extraEnv
	}
	return mergeEnv(extraEnv, env)
}
