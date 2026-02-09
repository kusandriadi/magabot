// Zhipu GLM provider (ChatGLM) via Z.AI API
package llm

import (
	"context"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const glmAPIURL = "https://api.z.ai/api/paas/v4"

// GLM provider (Zhipu ChatGLM)
type GLM struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
}

// GLMConfig for GLM provider
type GLMConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	BaseURL     string
}

// NewGLM creates a new GLM provider
func NewGLM(cfg *GLMConfig) *GLM {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ZAI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GLM_API_KEY")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = glmAPIURL
	}

	model := cfg.Model
	if model == "" {
		model = "glm-4.7"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	return &GLM{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: cfg.Temperature,
		baseURL:     baseURL,
	}
}

// Name returns provider name
func (g *GLM) Name() string {
	return "glm"
}

// Available checks if provider is available
func (g *GLM) Available() bool {
	return g.apiKey != ""
}

// Complete sends a completion request
func (g *GLM) Complete(ctx context.Context, req *Request) (*Response, error) {
	client := openai.NewClient(
		option.WithAPIKey(g.apiKey),
		option.WithBaseURL(g.baseURL),
	)
	return completeOpenAICompatible(ctx, client, "glm", g.model, g.maxTokens, g.temperature, req)
}
