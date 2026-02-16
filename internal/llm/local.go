// Local/self-hosted LLM provider (OpenAI-compatible API)
//
// Works with any server that exposes an OpenAI-compatible chat completions API:
//   - Ollama (http://localhost:11434/v1)
//   - vLLM (http://localhost:8000/v1)
//   - llama.cpp server (http://localhost:8080/v1)
//   - LocalAI (http://localhost:8080/v1)
//   - text-generation-webui with openai extension
//
// No API key required by default. Set LOCAL_LLM_URL to override the base URL.
package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/kusa/magabot/internal/util"
)

const (
	defaultLocalURL = "http://localhost:11434/v1" // Ollama default
)

// LocalConfig holds configuration for a local/self-hosted LLM provider.
type LocalConfig struct {
	Enabled     bool    `yaml:"enabled"`
	BaseURL     string  `yaml:"base_url"` // Server URL (default: http://localhost:11434/v1)
	Model       string  `yaml:"model"`    // Model name (default: llama3)
	APIKey      string  `yaml:"api_key"`  // Optional API key (some servers require it)
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
}

// Local implements the Provider interface for self-hosted OpenAI-compatible LLMs.
type Local struct {
	config LocalConfig
}

// NewLocal creates a new local LLM provider.
func NewLocal(cfg *LocalConfig) *Local {
	c := *cfg

	if c.BaseURL == "" {
		c.BaseURL = os.Getenv("LOCAL_LLM_URL")
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultLocalURL
	}

	if c.Model == "" {
		c.Model = os.Getenv("LOCAL_LLM_MODEL")
	}
	if c.Model == "" {
		c.Model = "llama3"
	}

	if c.APIKey == "" {
		c.APIKey = os.Getenv("LOCAL_LLM_API_KEY")
	}
	if c.APIKey == "" {
		// Many local servers accept any non-empty key or "none"
		c.APIKey = "local"
	}

	if c.MaxTokens == 0 {
		c.MaxTokens = 4096
	}

	return &Local{config: c}
}

// Name returns the provider name.
func (l *Local) Name() string {
	return "local"
}

// Available checks if the local LLM server is reachable.
// Unlike cloud providers that check for an API key, this does a quick HTTP
// health check against the base URL. The result is not cached — each call
// makes a fresh check so hot-swapping servers works.
func (l *Local) Available() bool {
	if !l.config.Enabled {
		return false
	}

	client := util.NewHTTPClient(2 * time.Second)
	resp, err := client.Get(l.config.BaseURL + "/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// Complete sends a chat completion request to the local LLM server.
func (l *Local) Complete(ctx context.Context, req *Request) (*Response, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(l.config.APIKey),
		option.WithBaseURL(l.config.BaseURL),
	}

	client := openai.NewClient(opts...)

	resp, err := completeOpenAICompatible(ctx, client, "local", l.config.Model, l.config.MaxTokens, l.config.Temperature, req)
	if err != nil {
		return nil, fmt.Errorf("local LLM (%s): %w", l.config.BaseURL, err)
	}
	return resp, nil
}
