package llm

import (
	"context"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// DeepSeek API is OpenAI-compatible
const (
	deepseekAPIURL = "https://api.deepseek.com/v1"
)

// DeepSeekConfig holds DeepSeek configuration
type DeepSeekConfig struct {
	APIKey      string  `yaml:"api_key" json:"api_key"`
	Model       string  `yaml:"model" json:"model"`
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
	BaseURL     string  `yaml:"base_url" json:"base_url"`
}

// DeepSeek implements the Provider interface for DeepSeek
type DeepSeek struct {
	config DeepSeekConfig
}

// NewDeepSeek creates a new DeepSeek provider
func NewDeepSeek(config *DeepSeekConfig) *DeepSeek {
	// Copy to avoid mutating caller's config
	cfg := *config

	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = deepseekAPIURL
	}

	return &DeepSeek{
		config: cfg,
	}
}

// Name returns the provider name
func (d *DeepSeek) Name() string {
	return "deepseek"
}

// Available returns true if the provider is configured
func (d *DeepSeek) Available() bool {
	return d.config.APIKey != ""
}

// Complete sends a completion request to DeepSeek
func (d *DeepSeek) Complete(ctx context.Context, req *Request) (*Response, error) {
	client := openai.NewClient(
		option.WithAPIKey(d.config.APIKey),
		option.WithBaseURL(d.config.BaseURL),
	)
	return completeOpenAICompatible(ctx, client, "deepseek", d.config.Model, d.config.MaxTokens, d.config.Temperature, req)
}

// Models returns available DeepSeek models
func (d *DeepSeek) Models() []string {
	return []string{
		"deepseek-chat",
		"deepseek-coder",
		"deepseek-reasoner",
	}
}
