// Zhipu GLM provider (ChatGLM)
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const glmAPIURL = "https://open.bigmodel.cn/api/paas/v4/chat/completions"

// GLM provider (Zhipu ChatGLM)
type GLM struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
	client      *http.Client
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
		client:      &http.Client{Timeout: 120 * time.Second},
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
	start := time.Now()

	// GLM uses OpenAI-compatible format
	var messages []map[string]string
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	// Build request body
	body := map[string]interface{}{
		"model":      g.model,
		"max_tokens": g.maxTokens,
		"messages":   messages,
	}

	if g.temperature > 0 {
		body["temperature"] = g.temperature
	}

	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", g.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)

	// Send request
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response (OpenAI-compatible format)
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	content := ""
	if len(result.Choices) > 0 {
		content = result.Choices[0].Message.Content
	}

	return &Response{
		Content:      content,
		Provider:     "glm",
		Model:        g.model,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		Latency:      time.Since(start),
	}, nil
}
