// Google Gemini provider
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

const geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models"

// Gemini provider
type Gemini struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	client      *http.Client
}

// GeminiConfig for Gemini provider
type GeminiConfig struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float64
}

// NewGemini creates a new Gemini provider
func NewGemini(cfg *GeminiConfig) *Gemini {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}

	model := cfg.Model
	if model == "" {
		model = "gemini-2.0-flash"
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	return &Gemini{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: cfg.Temperature,
		client:      &http.Client{Timeout: 120 * time.Second},
	}
}

// Name returns provider name
func (g *Gemini) Name() string {
	return "gemini"
}

// Available checks if provider is available
func (g *Gemini) Available() bool {
	return g.apiKey != ""
}

// Complete sends a completion request
func (g *Gemini) Complete(ctx context.Context, req *Request) (*Response, error) {
	start := time.Now()

	// Convert messages to Gemini format
	var contents []map[string]interface{}
	var systemInstruction string

	for _, m := range req.Messages {
		if m.Role == "system" {
			systemInstruction = m.Content
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		if m.HasBlocks() {
			// Multi-modal message
			var parts []map[string]interface{}
			for _, b := range m.Blocks {
				switch b.Type {
				case "text":
					parts = append(parts, map[string]interface{}{
						"text": b.Text,
					})
				case "image":
					parts = append(parts, map[string]interface{}{
						"inlineData": map[string]string{
							"mimeType": b.MimeType,
							"data":     b.ImageData,
						},
					})
				}
			}
			contents = append(contents, map[string]interface{}{
				"role":  role,
				"parts": parts,
			})
		} else {
			contents = append(contents, map[string]interface{}{
				"role": role,
				"parts": []map[string]string{
					{"text": m.Content},
				},
			})
		}
	}

	// Build request body
	body := map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": g.maxTokens,
		},
	}

	if systemInstruction != "" {
		body["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]string{
				{"text": systemInstruction},
			},
		}
	}

	if g.temperature > 0 {
		body["generationConfig"].(map[string]interface{})["temperature"] = g.temperature
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Build URL (API key sent via header, not query parameter, to avoid leaking in logs)
	url := fmt.Sprintf("%s/%s:generateContent", geminiAPIURL, g.model)

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey)

	// Send request
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response (limit to 10MB to prevent OOM)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	content := ""
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		content = result.Candidates[0].Content.Parts[0].Text
	}

	return &Response{
		Content:      content,
		Provider:     "gemini",
		Model:        g.model,
		InputTokens:  result.UsageMetadata.PromptTokenCount,
		OutputTokens: result.UsageMetadata.CandidatesTokenCount,
		Latency:      time.Since(start),
	}, nil
}
