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
	"strings"
	"time"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// Anthropic provider
type Anthropic struct {
	apiKey       string
	model        string
	maxTokens    int
	temperature  float64
	baseURL      string
	client       *http.Client
	useOAuth     bool
	oauthManager *OAuthManager
}

// AnthropicConfig for Anthropic provider
type AnthropicConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
	BaseURL     string
	UseOAuth    bool // Use Claude CLI OAuth tokens
}

// NewAnthropic creates a new Anthropic provider
func NewAnthropic(cfg *AnthropicConfig) *Anthropic {
	apiKey := cfg.APIKey
	useOAuth := cfg.UseOAuth

	// Try to load from environment
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	// Auto-detect OAuth token format (sk-ant-oat01-* is OAuth)
	if apiKey != "" && strings.HasPrefix(apiKey, "sk-ant-oat01-") {
		useOAuth = true
	}

	// If no API key, try to load from Claude CLI credentials
	var oauthManager *OAuthManager
	if apiKey == "" || useOAuth {
		oauthManager = GetOAuthManager()
		creds, err := oauthManager.LoadClaudeCliCredentials()
		if err == nil && creds != nil {
			apiKey = creds.AccessToken
			useOAuth = true
		}
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
		apiKey:       apiKey,
		model:        model,
		maxTokens:    maxTokens,
		temperature:  cfg.Temperature,
		baseURL:      baseURL,
		client:       &http.Client{Timeout: 120 * time.Second},
		useOAuth:     useOAuth,
		oauthManager: oauthManager,
	}
}

// Name returns provider name
func (a *Anthropic) Name() string {
	return "anthropic"
}

// Available checks if provider is available
func (a *Anthropic) Available() bool {
	if a.apiKey != "" {
		return true
	}
	// Check if OAuth credentials are available
	if a.oauthManager != nil && a.oauthManager.HasCredentials("anthropic") {
		return true
	}
	return false
}

// getAPIKey returns the current API key, refreshing OAuth token if needed
func (a *Anthropic) getAPIKey() (string, error) {
	if !a.useOAuth {
		return a.apiKey, nil
	}

	if a.oauthManager == nil {
		return a.apiKey, nil
	}

	// Get token, auto-refresh if expired
	token, err := a.oauthManager.GetAccessToken("anthropic")
	if err != nil {
		// Fall back to stored key if refresh fails
		if a.apiKey != "" {
			return a.apiKey, nil
		}
		return "", fmt.Errorf("get OAuth token: %w", err)
	}

	return token, nil
}

// Complete sends a completion request
func (a *Anthropic) Complete(ctx context.Context, req *Request) (*Response, error) {
	start := time.Now()

	// Get API key (with OAuth refresh if needed)
	apiKey, err := a.getAPIKey()
	if err != nil {
		return nil, err
	}

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
	httpReq.Header.Set("x-api-key", apiKey)
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
