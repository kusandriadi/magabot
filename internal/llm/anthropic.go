// Anthropic Claude provider
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

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// Anthropic provider
type Anthropic struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
	client      *http.Client
}

// AnthropicConfig for Anthropic provider
type AnthropicConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	BaseURL     string
}

// NewAnthropic creates a new Anthropic provider
func NewAnthropic(cfg *AnthropicConfig) *Anthropic {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = anthropicAPIURL
	}

	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	return &Anthropic{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: cfg.Temperature,
		baseURL:     baseURL,
		client:      &http.Client{Timeout: 120 * time.Second},
	}
}

// Name returns provider name
func (a *Anthropic) Name() string {
	return "anthropic"
}

// Available checks if provider is available
func (a *Anthropic) Available() bool {
	return a.apiKey != ""
}

// Complete sends a completion request
func (a *Anthropic) Complete(ctx context.Context, req *Request) (*Response, error) {
	start := time.Now()

	// Convert messages to Anthropic format
	var systemPrompt string
	var messages []map[string]string

	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		messages = append(messages, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	// Build request body
	body := map[string]interface{}{
		"model":      a.model,
		"max_tokens": a.maxTokens,
		"messages":   messages,
	}

	if systemPrompt != "" {
		body["system"] = systemPrompt
	}

	if a.temperature > 0 {
		body["temperature"] = a.temperature
	}

	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	resp, err := a.client.Do(httpReq)
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

	// Parse response
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	content := ""
	if len(result.Content) > 0 {
		content = result.Content[0].Text
	}

	return &Response{
		Content:      content,
		Provider:     "anthropic",
		Model:        a.model,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		Latency:      time.Since(start),
	}, nil
}
