// OAuth token management for LLM providers
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// OAuthCredentials represents OAuth tokens
type OAuthCredentials struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"` // Unix milliseconds
	Provider     string `json:"provider,omitempty"`
}

// CodexCliCredentials represents ~/.codex/auth.json format
type CodexCliCredentials struct {
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
	LastRefresh string `json:"last_refresh"`
}

// OAuthManager manages OAuth tokens with auto-refresh
type OAuthManager struct {
	credentials map[string]*OAuthCredentials
	mu          sync.RWMutex
	httpClient  *http.Client
}

// NewOAuthManager creates a new OAuth manager
func NewOAuthManager() *OAuthManager {
	return &OAuthManager{
		credentials: make(map[string]*OAuthCredentials),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

// checkFilePermissions warns if a credential file is world-readable
func checkFilePermissions(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	// Reject if group or others have any access (should be 0600 or stricter)
	if mode&0077 != 0 {
		slog.Warn("credential file has unsafe permissions", "path", path, "mode", fmt.Sprintf("%o", mode))
		return fmt.Errorf("credential file %s has unsafe permissions %o (should be 0600)", path, mode)
	}
	return nil
}

// LoadCodexCliCredentials loads credentials from ~/.codex/auth.json
func (m *OAuthManager) LoadCodexCliCredentials() (*OAuthCredentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Check CODEX_HOME env first
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}

	credPath := filepath.Join(codexHome, "auth.json")

	if err := checkFilePermissions(credPath); err != nil {
		return nil, fmt.Errorf("credential file security check failed: %w", err)
	}

	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read codex credentials: %w", err)
	}

	var creds CodexCliCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse codex credentials: %w", err)
	}

	if creds.Tokens.AccessToken == "" {
		return nil, fmt.Errorf("no access token in codex credentials")
	}

	// Calculate expiry (1 hour from last refresh or now)
	var expiresAt int64
	if creds.LastRefresh != "" {
		t, err := time.Parse(time.RFC3339, creds.LastRefresh)
		if err == nil {
			expiresAt = t.Add(time.Hour).UnixMilli()
		}
	}
	if expiresAt == 0 {
		expiresAt = time.Now().Add(time.Hour).UnixMilli()
	}

	oauth := &OAuthCredentials{
		AccessToken:  creds.Tokens.AccessToken,
		RefreshToken: creds.Tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		Provider:     "openai",
	}

	m.mu.Lock()
	m.credentials["openai"] = oauth
	m.mu.Unlock()

	return oauth, nil
}

// GetAccessToken returns a valid access token, refreshing if needed
func (m *OAuthManager) GetAccessToken(provider string) (string, error) {
	m.mu.RLock()
	creds, ok := m.credentials[provider]
	if !ok {
		m.mu.RUnlock()
		return "", fmt.Errorf("no credentials for provider: %s", provider)
	}

	// Check if token is expired or about to expire (5 min buffer)
	bufferMs := int64(5 * 60 * 1000) // 5 minutes
	needsRefresh := time.Now().UnixMilli()+bufferMs >= creds.ExpiresAt
	token := creds.AccessToken
	m.mu.RUnlock()

	if !needsRefresh {
		return token, nil
	}

	// Upgrade to write lock for refresh to prevent duplicate refreshes
	m.mu.Lock()
	// Re-check under write lock — another goroutine may have refreshed already
	creds = m.credentials[provider]
	if creds == nil {
		m.mu.Unlock()
		return "", fmt.Errorf("credentials removed for provider: %s", provider)
	}
	if time.Now().UnixMilli()+bufferMs < creds.ExpiresAt {
		token = creds.AccessToken
		m.mu.Unlock()
		return token, nil
	}
	m.mu.Unlock()

	// Refresh outside the lock (network call)
	newCreds, err := m.refreshToken(provider, creds)
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	return newCreds.AccessToken, nil
}

// refreshToken refreshes OAuth token for a provider
func (m *OAuthManager) refreshToken(provider string, creds *OAuthCredentials) (*OAuthCredentials, error) {
	switch provider {
	case "openai":
		return m.refreshOpenAIToken(creds)
	default:
		return nil, fmt.Errorf("unsupported provider for refresh: %s", provider)
	}
}

// refreshOpenAIToken refreshes OpenAI Codex OAuth token
func (m *OAuthManager) refreshOpenAIToken(creds *OAuthCredentials) (*OAuthCredentials, error) {
	// OpenAI Codex OAuth refresh endpoint
	refreshURL := "https://auth.openai.com/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", creds.RefreshToken)
	data.Set("client_id", "app_codex") // OpenAI Codex client ID

	req, err := http.NewRequest("POST", refreshURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	newCreds := &OAuthCredentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + (result.ExpiresIn * 1000),
		Provider:     "openai",
	}

	m.mu.Lock()
	m.credentials["openai"] = newCreds
	m.mu.Unlock()

	return newCreds, nil
}

// IsTokenExpired checks if the token is expired
func (m *OAuthManager) IsTokenExpired(provider string) bool {
	m.mu.RLock()
	creds, ok := m.credentials[provider]
	m.mu.RUnlock()

	if !ok {
		return true
	}

	return time.Now().UnixMilli() >= creds.ExpiresAt
}

// GetExpiryTime returns the token expiry time
func (m *OAuthManager) GetExpiryTime(provider string) time.Time {
	m.mu.RLock()
	creds, ok := m.credentials[provider]
	m.mu.RUnlock()

	if !ok {
		return time.Time{}
	}

	return time.UnixMilli(creds.ExpiresAt)
}

// HasCredentials checks if credentials exist for a provider
func (m *OAuthManager) HasCredentials(provider string) bool {
	m.mu.RLock()
	_, ok := m.credentials[provider]
	m.mu.RUnlock()
	return ok
}

// Global OAuth manager instance
var globalOAuthManager *OAuthManager
var oauthOnce sync.Once

// GetOAuthManager returns the global OAuth manager
func GetOAuthManager() *OAuthManager {
	oauthOnce.Do(func() {
		globalOAuthManager = NewOAuthManager()
	})
	return globalOAuthManager
}
