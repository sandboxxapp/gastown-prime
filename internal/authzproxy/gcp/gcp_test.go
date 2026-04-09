package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// mockTokenSource returns a fixed token or error.
type mockTokenSource struct {
	token *oauth2.Token
	err   error
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	return m.token, m.err
}

// newTestMinter creates a TokenMinter wired to a fixed token source factory.
func newTestMinter(baseURL string, client *http.Client, ts oauth2.TokenSource, tsErr error) *TokenMinter {
	return &TokenMinter{
		baseURL: baseURL,
		client:  client,
		tokenSource: func(_ context.Context, _ ...string) (oauth2.TokenSource, error) {
			if tsErr != nil {
				return nil, tsErr
			}
			return ts, nil
		},
	}
}

// --- Impersonate mode tests ---

func TestMintToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path == "" {
			t.Error("empty path")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": "ya29.mock-token-12345",
			"expireTime":  time.Now().Add(time.Hour).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	minter := newTestMinter(server.URL, server.Client(), nil, nil)

	profile := Profile{
		TargetSA: "terraform-plan@sandboxx-prod-01.iam.gserviceaccount.com",
		Scopes:   []string{"https://www.googleapis.com/auth/compute.readonly"},
		Lifetime: "3600s",
	}

	token, expiry, err := minter.MintToken(profile)
	if err != nil {
		t.Fatalf("MintToken failed: %v", err)
	}
	if token != "ya29.mock-token-12345" {
		t.Errorf("token = %q, want ya29.mock-token-12345", token)
	}
	if expiry.Before(time.Now()) {
		t.Error("token already expired")
	}
}

func TestMintToken_IAMError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    403,
				"message": "Permission denied on resource",
			},
		})
	}))
	defer server.Close()

	minter := newTestMinter(server.URL, server.Client(), nil, nil)

	profile := Profile{
		TargetSA: "terraform-plan@sandboxx-prod-01.iam.gserviceaccount.com",
		Scopes:   []string{"https://www.googleapis.com/auth/compute"},
		Lifetime: "3600s",
	}

	_, _, err := minter.MintToken(profile)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestMintToken_EmptyTargetSA(t *testing.T) {
	minter := newTestMinter(defaultIAMBaseURL, http.DefaultClient, nil, nil)

	_, _, err := minter.MintToken(Profile{})
	if err == nil {
		t.Error("expected error for empty target SA")
	}
}

// --- Downscope mode tests ---

func TestMintToken_Downscope_Success(t *testing.T) {
	wantToken := "ya29.downscoped-adc-token"
	wantExpiry := time.Now().Add(time.Hour).Truncate(time.Second)

	ts := &mockTokenSource{
		token: &oauth2.Token{
			AccessToken: wantToken,
			Expiry:      wantExpiry,
		},
	}

	minter := newTestMinter(defaultIAMBaseURL, http.DefaultClient, ts, nil)

	profile := Profile{
		Mode:   ModeDownscope,
		Scopes: []string{"https://www.googleapis.com/auth/cloud-platform.read-only"},
	}

	token, expiry, err := minter.MintToken(profile)
	if err != nil {
		t.Fatalf("MintToken downscope failed: %v", err)
	}
	if token != wantToken {
		t.Errorf("token = %q, want %q", token, wantToken)
	}
	if !expiry.Equal(wantExpiry) {
		t.Errorf("expiry = %v, want %v", expiry, wantExpiry)
	}
}

func TestDownscopeToken_Direct(t *testing.T) {
	wantToken := "ya29.direct-downscoped"
	wantExpiry := time.Now().Add(30 * time.Minute).Truncate(time.Second)

	ts := &mockTokenSource{
		token: &oauth2.Token{
			AccessToken: wantToken,
			Expiry:      wantExpiry,
		},
	}

	minter := newTestMinter(defaultIAMBaseURL, http.DefaultClient, ts, nil)

	token, expiry, err := minter.DownscopeToken([]string{"https://www.googleapis.com/auth/bigquery.readonly"})
	if err != nil {
		t.Fatalf("DownscopeToken failed: %v", err)
	}
	if token != wantToken {
		t.Errorf("token = %q, want %q", token, wantToken)
	}
	if !expiry.Equal(wantExpiry) {
		t.Errorf("expiry = %v, want %v", expiry, wantExpiry)
	}
}

func TestDownscopeToken_ADCError(t *testing.T) {
	minter := newTestMinter(defaultIAMBaseURL, http.DefaultClient, nil, fmt.Errorf("no ADC configured"))

	_, _, err := minter.DownscopeToken([]string{"https://www.googleapis.com/auth/cloud-platform.read-only"})
	if err == nil {
		t.Error("expected error when ADC is unavailable")
	}
}

func TestDownscopeToken_TokenError(t *testing.T) {
	ts := &mockTokenSource{err: fmt.Errorf("token refresh failed")}
	minter := newTestMinter(defaultIAMBaseURL, http.DefaultClient, ts, nil)

	_, _, err := minter.DownscopeToken([]string{"https://www.googleapis.com/auth/cloud-platform.read-only"})
	if err == nil {
		t.Error("expected error when token fetch fails")
	}
}

// --- Profile.Validate tests ---

func TestProfile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
		wantErr bool
	}{
		{
			name:    "impersonate valid",
			profile: Profile{TargetSA: "sa@proj.iam.gserviceaccount.com", Scopes: []string{"scope"}, Lifetime: "3600s"},
			wantErr: false,
		},
		{
			name:    "impersonate default mode",
			profile: Profile{TargetSA: "sa@proj.iam.gserviceaccount.com", Scopes: []string{"scope"}},
			wantErr: false,
		},
		{
			name:    "impersonate no SA",
			profile: Profile{Scopes: []string{"scope"}, Lifetime: "3600s"},
			wantErr: true,
		},
		{
			name:    "impersonate no scopes",
			profile: Profile{TargetSA: "sa@proj.iam.gserviceaccount.com", Lifetime: "3600s"},
			wantErr: true,
		},
		{
			name:    "downscope valid",
			profile: Profile{Mode: ModeDownscope, Scopes: []string{"https://www.googleapis.com/auth/cloud-platform.read-only"}},
			wantErr: false,
		},
		{
			name:    "downscope no scopes",
			profile: Profile{Mode: ModeDownscope},
			wantErr: true,
		},
		{
			name:    "downscope no SA required",
			profile: Profile{Mode: ModeDownscope, Scopes: []string{"scope"}},
			wantErr: false, // TargetSA not required in downscope mode
		},
		{
			name:    "invalid mode",
			profile: Profile{Mode: "invalid", Scopes: []string{"scope"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
