// Claude Code credentials backend
package secrets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Claude is a backend that reads from Claude Code's credentials
type Claude struct {
	path string
	mu   sync.RWMutex
}

// ClaudeCredentials represents the structure of .claude/.credentials.json
type ClaudeCredentials struct {
	ClaudeAiOauth struct {
		AccessToken      string   `json:"accessToken"`
		RefreshToken     string   `json:"refreshToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
		RateLimitTier    string   `json:"rateLimitTier"`
	} `json:"claudeAiOauth"`
}

// NewClaude creates a new Claude Code credentials backend
func NewClaude() (*Claude, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(home, ".claude", ".credentials.json")

	return &Claude{
		path: path,
	}, nil
}

// Name returns the backend name
func (c *Claude) Name() string {
	return "claude"
}

// Get retrieves a secret from Claude Code credentials
func (c *Claude) Get(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Only support Anthropic API key for now
	if key != KeyAnthropicAPIKey {
		return "", ErrNotFound
	}

	creds, err := c.loadCredentials()
	if err != nil {
		return "", ErrNotFound
	}

	// Return the access token
	if creds.ClaudeAiOauth.AccessToken == "" {
		return "", ErrNotFound
	}

	return creds.ClaudeAiOauth.AccessToken, nil
}

// Set stores a secret (not supported for Claude backend)
func (c *Claude) Set(ctx context.Context, key, value string) error {
	return ErrBackendError
}

// Delete removes a secret (not supported for Claude backend)
func (c *Claude) Delete(ctx context.Context, key string) error {
	return ErrBackendError
}

// List returns all secret keys (not supported for Claude backend)
func (c *Claude) List(ctx context.Context) ([]string, error) {
	return nil, ErrBackendError
}

// Ping checks if the backend is available
func (c *Claude) Ping(ctx context.Context) error {
	_, err := c.loadCredentials()
	return err
}

// loadCredentials loads and parses the Claude Code credentials file
func (c *Claude) loadCredentials() (*ClaudeCredentials, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}

	var creds ClaudeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}
