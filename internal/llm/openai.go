// OpenAI GPT provider
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

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

// OpenAI provider
type OpenAI struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
	client      *http.Client
}

// OpenAIConfig for OpenAI provider
type OpenAIConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	BaseURL     string // For Azure or proxy
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(cfg *OpenAIConfig) *OpenAI {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = openaiAPIURL
	}

	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	return &OpenAI{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: cfg.Temperature,
		baseURL:     baseURL,
		client:      &http.Client{Timeout: 120 * time.Second},
	}
}

// Name returns provider name
func (o *OpenAI) Name() string {
	return "openai"
}

// Available checks if provider is available
func (o *OpenAI) Available() bool {
	return o.apiKey != ""
}

// Complete sends a completion request
func (o *OpenAI) Complete(ctx context.Context, req *Request) (*Response, error) {
	start := time.Now()

	// Convert messages to OpenAI format
	var messages []map[string]string
	for _, m := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	// Build request body
	body := map[string]interface{}{
		"model":      o.model,
		"max_tokens": o.maxTokens,
		"messages":   messages,
	}

	if o.temperature > 0 {
		body["temperature"] = o.temperature
	}

	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	// Send request
	resp, err := o.client.Do(httpReq)
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
		Provider:     "openai",
		Model:        o.model,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		Latency:      time.Since(start),
	}, nil
}
