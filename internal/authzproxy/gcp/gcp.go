// Package gcp provides GCP service account impersonation and ADC downscope token minting.
package gcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultIAMBaseURL = "https://iamcredentials.googleapis.com/v1"
	defaultLifetime   = "3600s"

	// ModeImpersonate uses SA impersonation via iam.generateAccessToken (default).
	ModeImpersonate = "impersonate"
	// ModeDownscope uses the caller's ADC credentials restricted to the specified scopes.
	ModeDownscope = "downscope"
)

// Profile defines a GCP token minting target.
type Profile struct {
	// Mode selects the minting strategy: "impersonate" (default) or "downscope".
	Mode string `json:"mode,omitempty"`
	// TargetSA is the service account to impersonate. Required when Mode is "impersonate".
	TargetSA string `json:"target_sa,omitempty"`
	// Scopes lists the OAuth2 scopes to request.
	Scopes []string `json:"scopes"`
	// Lifetime controls token duration for impersonation mode (e.g. "3600s").
	Lifetime string `json:"lifetime,omitempty"`
	// Project is the GCP project ID; optional, reserved for future CAB/STS scoping.
	Project string `json:"project,omitempty"`
}

// Validate checks that the profile has required fields.
func (p *Profile) Validate() error {
	if p.Mode == "" {
		p.Mode = ModeImpersonate
	}
	if p.Mode != ModeImpersonate && p.Mode != ModeDownscope {
		return fmt.Errorf("mode must be %q or %q, got %q", ModeImpersonate, ModeDownscope, p.Mode)
	}
	if p.Mode == ModeImpersonate && p.TargetSA == "" {
		return fmt.Errorf("target_sa is required for impersonate mode")
	}
	if len(p.Scopes) == 0 {
		return fmt.Errorf("at least one scope is required")
	}
	if p.Lifetime == "" {
		p.Lifetime = defaultLifetime
	}
	return nil
}

// TokenMinter mints short-lived GCP access tokens via SA impersonation or ADC downscope.
type TokenMinter struct {
	baseURL     string
	client      *http.Client
	tokenSource func(ctx context.Context, scopes ...string) (oauth2.TokenSource, error)
}

// NewTokenMinter creates a minter using Application Default Credentials.
func NewTokenMinter() *TokenMinter {
	return &TokenMinter{
		baseURL: defaultIAMBaseURL,
		client:  http.DefaultClient,
		tokenSource: func(ctx context.Context, scopes ...string) (oauth2.TokenSource, error) {
			creds, err := google.FindDefaultCredentials(ctx, scopes...)
			if err != nil {
				return nil, err
			}
			return creds.TokenSource, nil
		},
	}
}

// MintToken generates a short-lived access token. It routes to impersonation or downscope
// based on the profile's Mode field.
func (m *TokenMinter) MintToken(profile Profile) (string, time.Time, error) {
	if err := profile.Validate(); err != nil {
		return "", time.Time{}, fmt.Errorf("invalid profile: %w", err)
	}

	if profile.Mode == ModeDownscope {
		return m.DownscopeToken(profile.Scopes)
	}
	return m.impersonateToken(profile)
}

// impersonateToken generates a token by calling iam.generateAccessToken on the target SA.
// Requires the caller to have roles/iam.serviceAccountTokenCreator on the target SA.
func (m *TokenMinter) impersonateToken(profile Profile) (string, time.Time, error) {
	url := fmt.Sprintf("%s/projects/-/serviceAccounts/%s:generateAccessToken",
		m.baseURL, profile.TargetSA)

	body, _ := json.Marshal(map[string]interface{}{
		"scope":    profile.Scopes,
		"lifetime": profile.Lifetime,
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("IAM API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return "", time.Time{}, fmt.Errorf("IAM API %d: %s", resp.StatusCode, errResp.Error.Message)
	}

	var tokenResp struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  string `json:"expireTime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parse token response: %w", err)
	}

	expiry, err := time.Parse(time.RFC3339, tokenResp.ExpireTime)
	if err != nil {
		expiry = time.Now().Add(time.Hour) // fallback
	}

	return tokenResp.AccessToken, expiry, nil
}

// DownscopeToken returns a token from ADC restricted to the given OAuth2 scopes.
// The token is scoped to exactly the requested permissions — no SA impersonation occurs.
// ADC must be configured in the environment (GOOGLE_APPLICATION_CREDENTIALS or gcloud login).
func (m *TokenMinter) DownscopeToken(scopes []string) (string, time.Time, error) {
	ctx := context.Background()
	ts, err := m.tokenSource(ctx, scopes...)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("find default credentials: %w", err)
	}
	token, err := ts.Token()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("obtain ADC token: %w", err)
	}
	return token.AccessToken, token.Expiry, nil
}
