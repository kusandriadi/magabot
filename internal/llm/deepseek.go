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

// DeepSeek API is OpenAI-compatible
const (
	deepseekAPIURL = "https://api.deepseek.com/v1/chat/completions"
)

// DeepSeekConfig holds DeepSeek configuration
type DeepSeekConfig struct {
	APIKey      string  `yaml:"api_key" json:"api_key"`
	Model       string  `yaml:"model" json:"model"`               // deepseek-chat, deepseek-coder
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
	BaseURL     string  `yaml:"base_url" json:"base_url"`         // Custom endpoint
}

// DeepSeek implements the Provider interface for DeepSeek
type DeepSeek struct {
	config DeepSeekConfig
	client *http.Client
}

// NewDeepSeek creates a new DeepSeek provider
func NewDeepSeek(config *DeepSeekConfig) *DeepSeek {
	if config.APIKey == "" {
		config.APIKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	if config.Model == "" {
		config.Model = "deepseek-chat"
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}
	if config.Temperature == 0 {
		config.Temperature = 0.7
	}
	if config.BaseURL == "" {
		config.BaseURL = deepseekAPIURL
	}

	return &DeepSeek{
		config: *config,
		client: &http.Client{Timeout: 120 * time.Second},
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
	model := d.config.Model
	maxTokens := d.config.MaxTokens
	temperature := d.config.Temperature

	if req.MaxTokens > 0 {
		maxTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		temperature = req.Temperature
	}

	// Convert messages to DeepSeek format (OpenAI-compatible)
	dsMessages := make([]map[string]string, len(req.Messages))
	for i, msg := range req.Messages {
		dsMessages[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    dsMessages,
		"max_tokens":  maxTokens,
		"temperature": temperature,
		"stream":      false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", d.config.BaseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.config.APIKey)

	start := time.Now()
	resp, err := d.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("DeepSeek API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("DeepSeek API error: %d - %s", resp.StatusCode, string(body))
	}

	var dsResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &dsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(dsResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from DeepSeek")
	}

	return &Response{
		Provider:     "deepseek",
		Model:        dsResp.Model,
		Content:      dsResp.Choices[0].Message.Content,
		InputTokens:  dsResp.Usage.PromptTokens,
		OutputTokens: dsResp.Usage.CompletionTokens,
		Latency:      time.Since(start),
	}, nil
}

// Models returns available DeepSeek models
func (d *DeepSeek) Models() []string {
	return []string{
		"deepseek-chat",       // General chat model
		"deepseek-coder",      // Code-focused model  
		"deepseek-reasoner",   // Reasoning model (R1)
	}
}
