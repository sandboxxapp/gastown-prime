// Package auth provides OAuth token management for upstream MCP servers.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// UpstreamConfig is the config for one upstream MCP server.
// Inlined from the bridge's upstream package to avoid porting the full
// upstream process manager (gt manages upstreams natively).
type UpstreamConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	Startup string            `json:"startup,omitempty"`
}

// SecretsFile maps MCP names to their upstream configs.
type SecretsFile map[string]UpstreamConfig

// LoadSecrets reads a secrets JSON file, extracting entries with a "command" field.
func LoadSecrets(path string) (SecretsFile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from config
	if err != nil {
		return nil, fmt.Errorf("read secrets: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse secrets: %w", err)
	}
	secrets := make(SecretsFile)
	for name, entry := range raw {
		var cfg UpstreamConfig
		if err := json.Unmarshal(entry, &cfg); err != nil || cfg.Command == "" {
			continue // skip non-MCP entries (e.g., gcp_profiles)
		}
		secrets[name] = cfg
	}
	return secrets, nil
}

// TokenProvider supplies auth tokens for upstream MCP servers.
type TokenProvider interface {
	EnvForUpstream(name string, config UpstreamConfig) (map[string]string, error)
	Save() error
}

// OAuthConfig holds OAuth settings for a provider.
type OAuthConfig struct {
	AuthURL  string `json:"auth_url"`
	TokenURL string `json:"token_url"`
	ClientID string `json:"client_id"`
	Scopes   string `json:"scopes"`
}

// Token is a stored OAuth token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-5 * time.Minute))
}

// Store persists OAuth tokens to disk and implements TokenProvider.
type Store struct {
	Path    string            `json:"-"`
	Tokens  map[string]*Token `json:"tokens"`
	Configs map[string]OAuthConfig
	Secrets SecretsFile
}

// NewStore creates a token store.
func NewStore(path string, configs map[string]OAuthConfig, secrets SecretsFile) (*Store, error) {
	store := &Store{
		Path:    path,
		Tokens:  make(map[string]*Token),
		Configs: configs,
		Secrets: secrets,
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from config
	if err == nil {
		json.Unmarshal(data, store)
	}
	store.Path = path
	store.Configs = configs
	store.Secrets = secrets
	return store, nil
}

// EnvForUpstream returns extra env vars for an upstream, injecting OAuth tokens if available.
func (s *Store) EnvForUpstream(name string, config UpstreamConfig) (map[string]string, error) {
	token, ok := s.Tokens[name]
	if !ok {
		return nil, nil
	}
	if token.IsExpired() {
		oauthCfg, known := s.Configs[name]
		if !known {
			return nil, fmt.Errorf("%s token expired, no refresh config", name)
		}
		newToken, err := refreshToken(oauthCfg, config, token)
		if err != nil {
			return nil, fmt.Errorf("%s token refresh failed: %w — run: gt authz login %s", name, err, name)
		}
		s.Tokens[name] = newToken
		s.Save()
		token = newToken
	}
	env := map[string]string{}
	switch name {
	case "linear":
		env["LINEAR_API_KEY"] = token.AccessToken
	default:
		env[strings.ToUpper(name)+"_ACCESS_TOKEN"] = token.AccessToken
	}
	return env, nil
}

func (s *Store) Save() error {
	data, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(s.Path, data, 0600)
}

// KnownProviders maps MCP names to their OAuth configs.
var KnownProviders = map[string]OAuthConfig{
	"linear": {
		AuthURL:  "https://linear.app/oauth/authorize",
		TokenURL: "https://api.linear.app/oauth/token",
		Scopes:   "read,write,issues:create,comments:create",
	},
}

// RunOAuthFlow performs an interactive browser-based OAuth flow.
func RunOAuthFlow(provider string, oauthCfg OAuthConfig, secretsCfg UpstreamConfig) (*Token, error) {
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	listener, err := net.Listen("tcp", "127.0.0.1:19876")
	if err != nil {
		return nil, fmt.Errorf("listen on port 19876: %w (is another auth flow running?)", err)
	}
	callbackURL := "http://127.0.0.1:19876/callback"

	clientID := oauthCfg.ClientID
	if id := secretsCfg.Env["OAUTH_CLIENT_ID"]; id != "" {
		clientID = id
	}
	if clientID == "" {
		return nil, fmt.Errorf("no client_id for %s", provider)
	}
	clientSecret := secretsCfg.Env["OAUTH_CLIENT_SECRET"]

	params := url.Values{
		"response_type": {"code"}, "client_id": {clientID},
		"redirect_uri": {callbackURL}, "state": {state},
		"scope": {oauthCfg.Scopes}, "prompt": {"consent"},
	}
	authURL := oauthCfg.AuthURL + "?" + params.Encode()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("OAuth error: %s", e)
			return
		}
		codeCh <- r.URL.Query().Get("code")
		fmt.Fprint(w, "<h1>Authenticated!</h1><p>Close this tab.</p>")
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	fmt.Printf("\nOpening browser for %s OAuth...\n%s\n\n", provider, authURL)
	openBrowser(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-time.After(120 * time.Second):
		return nil, fmt.Errorf("timeout")
	}

	tokenData := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callbackURL}, "client_id": {clientID}}
	if clientSecret != "" {
		tokenData.Set("client_secret", clientSecret)
	}
	return exchangeToken(oauthCfg.TokenURL, tokenData)
}

func refreshToken(oauthCfg OAuthConfig, config UpstreamConfig, token *Token) (*Token, error) {
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token")
	}
	clientID := oauthCfg.ClientID
	if id := config.Env["OAUTH_CLIENT_ID"]; id != "" {
		clientID = id
	}
	data := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token.RefreshToken}, "client_id": {clientID}}
	if secret := config.Env["OAUTH_CLIENT_SECRET"]; secret != "" {
		data.Set("client_secret", secret)
	}
	newToken, err := exchangeToken(oauthCfg.TokenURL, data)
	if err != nil {
		return nil, err
	}
	if newToken.RefreshToken == "" {
		newToken.RefreshToken = token.RefreshToken
	}
	return newToken, nil
}

func exchangeToken(tokenURL string, data url.Values) (*Token, error) {
	resp, err := http.PostForm(tokenURL, data) //nolint:gosec // URL from config
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&tr)
	if tr.Error != "" {
		return nil, fmt.Errorf("token error: %s", tr.Error)
	}
	exp := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.ExpiresIn == 0 {
		exp = time.Now().Add(90 * 24 * time.Hour)
	}
	return &Token{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken, TokenType: tr.TokenType, ExpiresAt: exp}, nil
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "linux":
		cmd = exec.Command("xdg-open", u)
	default:
		log.Printf("open %s manually", u)
		return
	}
	cmd.Start()
}
