// OAuth token management for LLM providers
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// ClaudeCliCredentials represents ~/.claude/.credentials.json format
type ClaudeCliCredentials struct {
	ClaudeAiOAuth *ClaudeAiOAuth `json:"claudeAiOauth"`
}

type ClaudeAiOAuth struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"`
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscriptionType"`
	RateLimitTier    string   `json:"rateLimitTier"`
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
		log.Printf("[SECURITY] WARNING: credential file %s has unsafe permissions %o, expected 0600", path, mode)
		return fmt.Errorf("credential file %s has unsafe permissions %o (should be 0600)", path, mode)
	}
	return nil
}

// LoadClaudeCliCredentials loads credentials from ~/.claude/.credentials.json
func (m *OAuthManager) LoadClaudeCliCredentials() (*OAuthCredentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	credPath := filepath.Join(home, ".claude", ".credentials.json")

	if err := checkFilePermissions(credPath); err != nil {
		return nil, fmt.Errorf("credential file security check failed: %w", err)
	}

	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read claude credentials: %w", err)
	}

	var creds ClaudeCliCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse claude credentials: %w", err)
	}

	if creds.ClaudeAiOAuth == nil {
		return nil, fmt.Errorf("no claudeAiOauth in credentials")
	}

	oauth := &OAuthCredentials{
		AccessToken:  creds.ClaudeAiOAuth.AccessToken,
		RefreshToken: creds.ClaudeAiOAuth.RefreshToken,
		ExpiresAt:    creds.ClaudeAiOAuth.ExpiresAt,
		Provider:     "anthropic",
	}

	m.mu.Lock()
	m.credentials["anthropic"] = oauth
	m.mu.Unlock()

	return oauth, nil
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
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("no credentials for provider: %s", provider)
	}

	// Check if token is expired or about to expire (5 min buffer)
	bufferMs := int64(5 * 60 * 1000) // 5 minutes
	if time.Now().UnixMilli()+bufferMs >= creds.ExpiresAt {
		// Need to refresh
		newCreds, err := m.refreshToken(provider, creds)
		if err != nil {
			return "", fmt.Errorf("refresh token: %w", err)
		}
		creds = newCreds
	}

	return creds.AccessToken, nil
}

// refreshToken refreshes OAuth token for a provider
func (m *OAuthManager) refreshToken(provider string, creds *OAuthCredentials) (*OAuthCredentials, error) {
	switch provider {
	case "anthropic":
		return m.refreshAnthropicToken(creds)
	case "openai":
		return m.refreshOpenAIToken(creds)
	default:
		return nil, fmt.Errorf("unsupported provider for refresh: %s", provider)
	}
}

// refreshAnthropicToken refreshes Anthropic OAuth token
func (m *OAuthManager) refreshAnthropicToken(creds *OAuthCredentials) (*OAuthCredentials, error) {
	// Anthropic OAuth refresh endpoint
	refreshURL := "https://console.anthropic.com/v1/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", creds.RefreshToken)
	data.Set("client_id", "claude-code") // Claude CLI client ID

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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"` // seconds
		TokenType    string `json:"token_type"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	newCreds := &OAuthCredentials{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + (result.ExpiresIn * 1000),
		Provider:     "anthropic",
	}

	// Update stored credentials
	m.mu.Lock()
	m.credentials["anthropic"] = newCreds
	m.mu.Unlock()

	// Also update the file
	m.saveClaudeCliCredentials(newCreds)

	return newCreds, nil
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

	body, _ := io.ReadAll(resp.Body)

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

// saveClaudeCliCredentials saves updated credentials back to file
func (m *OAuthManager) saveClaudeCliCredentials(creds *OAuthCredentials) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	credPath := filepath.Join(home, ".claude", ".credentials.json")

	// Read existing file to preserve other fields
	data, err := os.ReadFile(credPath)
	if err != nil {
		return err
	}

	var existing ClaudeCliCredentials
	if err := json.Unmarshal(data, &existing); err != nil {
		return err
	}

	if existing.ClaudeAiOAuth == nil {
		existing.ClaudeAiOAuth = &ClaudeAiOAuth{}
	}

	existing.ClaudeAiOAuth.AccessToken = creds.AccessToken
	existing.ClaudeAiOAuth.RefreshToken = creds.RefreshToken
	existing.ClaudeAiOAuth.ExpiresAt = creds.ExpiresAt

	newData, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(credPath, newData, 0600)
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
