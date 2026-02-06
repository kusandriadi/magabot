// Package secrets provides secrets management with multiple backends
package secrets

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("secret not found")
	ErrBackendError = errors.New("backend error")
)

// Backend interface for secrets storage
type Backend interface {
	// Name returns the backend name
	Name() string

	// Get retrieves a secret
	Get(ctx context.Context, key string) (string, error)

	// Set stores a secret
	Set(ctx context.Context, key, value string) error

	// Delete removes a secret
	Delete(ctx context.Context, key string) error

	// List returns all secret keys (not values)
	List(ctx context.Context) ([]string, error)

	// Ping checks if the backend is available
	Ping(ctx context.Context) error
}

// Manager manages secrets with fallback support
type Manager struct {
	primary  Backend
	fallback Backend
}

// NewManager creates a new secrets manager
func NewManager(primary Backend) *Manager {
	return &Manager{
		primary: primary,
	}
}

// SetFallback sets a fallback backend
func (m *Manager) SetFallback(fallback Backend) {
	m.fallback = fallback
}

// Get retrieves a secret, trying fallback if primary fails
func (m *Manager) Get(ctx context.Context, key string) (string, error) {
	value, err := m.primary.Get(ctx, key)
	if err == nil {
		return value, nil
	}

	if m.fallback != nil {
		return m.fallback.Get(ctx, key)
	}

	return "", err
}

// Set stores a secret
func (m *Manager) Set(ctx context.Context, key, value string) error {
	return m.primary.Set(ctx, key, value)
}

// Delete removes a secret
func (m *Manager) Delete(ctx context.Context, key string) error {
	return m.primary.Delete(ctx, key)
}

// Backend returns the primary backend name
func (m *Manager) Backend() string {
	return m.primary.Name()
}

// Ping checks if the primary backend is available
func (m *Manager) Ping(ctx context.Context) error {
	return m.primary.Ping(ctx)
}

// Common secret keys
const (
	KeyEncryptionKey    = "magabot/encryption_key"
	KeyTelegramToken    = "magabot/telegram/bot_token"
	KeySlackBotToken    = "magabot/slack/bot_token"
	KeySlackAppToken    = "magabot/slack/app_token"
	KeyAnthropicAPIKey  = "magabot/llm/anthropic_api_key"
	KeyOpenAIAPIKey     = "magabot/llm/openai_api_key"
	KeyGeminiAPIKey     = "magabot/llm/gemini_api_key"
	KeyGLMAPIKey        = "magabot/llm/glm_api_key"
	KeyBraveAPIKey      = "magabot/tools/brave_api_key"
)

// Config for secrets manager
type Config struct {
	Backend     string            `yaml:"backend"` // local, vault, consul
	VaultConfig *VaultConfig      `yaml:"vault,omitempty"`
	LocalConfig *LocalConfig      `yaml:"local,omitempty"`
}

// NewFromConfig creates a secrets manager from config
func NewFromConfig(cfg *Config) (*Manager, error) {
	if cfg == nil {
		cfg = &Config{Backend: "local"}
	}

	var backend Backend
	var err error

	switch cfg.Backend {
	case "vault":
		if cfg.VaultConfig == nil {
			return nil, fmt.Errorf("vault config required")
		}
		backend, err = NewVault(cfg.VaultConfig)
	case "local", "":
		localCfg := cfg.LocalConfig
		if localCfg == nil {
			localCfg = &LocalConfig{}
		}
		backend, err = NewLocal(localCfg)
	default:
		return nil, fmt.Errorf("unknown backend: %s", cfg.Backend)
	}

	if err != nil {
		return nil, err
	}

	return NewManager(backend), nil
}
