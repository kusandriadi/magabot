// Environment-based secrets backend
package secrets

import (
	"context"
	"os"
	"strings"
)

// Env is an environment variable-based secrets backend
type Env struct{}

// NewEnv creates a new environment secrets backend
func NewEnv() *Env {
	return &Env{}
}

// Name returns the backend name
func (e *Env) Name() string {
	return "env"
}

// Get retrieves a secret from environment variables
func (e *Env) Get(ctx context.Context, key string) (string, error) {
	// Map magabot secret keys to standard environment variable names
	envKey := mapSecretToEnvVar(key)

	value := os.Getenv(envKey)
	if value == "" {
		return "", ErrNotFound
	}
	return value, nil
}

// Set stores a secret (not supported for env backend)
func (e *Env) Set(ctx context.Context, key, value string) error {
	return ErrBackendError
}

// Delete removes a secret (not supported for env backend)
func (e *Env) Delete(ctx context.Context, key string) error {
	return ErrBackendError
}

// List returns all secret keys (not supported for env backend)
func (e *Env) List(ctx context.Context) ([]string, error) {
	return nil, ErrBackendError
}

// Ping checks if the backend is available
func (e *Env) Ping(ctx context.Context) error {
	// Environment is always available
	return nil
}

// mapSecretToEnvVar converts magabot secret keys to standard environment variable names
func mapSecretToEnvVar(key string) string {
	switch key {
	case KeyAnthropicAPIKey:
		return "ANTHROPIC_API_KEY"
	case KeyOpenAIAPIKey:
		return "OPENAI_API_KEY"
	case KeyGeminiAPIKey:
		return "GEMINI_API_KEY"
	case KeyGLMAPIKey:
		return "GLM_API_KEY"
	case KeyDeepSeekAPIKey:
		return "DEEPSEEK_API_KEY"
	case KeyBraveAPIKey:
		return "BRAVE_API_KEY"
	case KeyTelegramToken:
		return "TELEGRAM_BOT_TOKEN"
	case KeySlackBotToken:
		return "SLACK_BOT_TOKEN"
	case KeySlackAppToken:
		return "SLACK_APP_TOKEN"
	case KeyEncryptionKey:
		return "MAGABOT_ENCRYPTION_KEY"
	default:
		// Generic fallback: convert "magabot/llm/provider_api_key" to "PROVIDER_API_KEY"
		parts := strings.Split(key, "/")
		if len(parts) > 0 {
			return strings.ToUpper(parts[len(parts)-1])
		}
		return strings.ToUpper(strings.ReplaceAll(key, "/", "_"))
	}
}
