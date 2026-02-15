// Google Gemini provider
package llm

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"google.golang.org/genai"
)

// Gemini provider
type Gemini struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
}

// GeminiConfig for Gemini provider
type GeminiConfig struct {
	APIKey      string // #nosec G117 -- config field, not serialized to untrusted output
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

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  g.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	// Convert messages to Gemini format
	var contents []*genai.Content
	var systemInstruction *genai.Content

	for _, m := range req.Messages {
		if m.Role == "system" {
			systemInstruction = &genai.Content{
				Parts: []*genai.Part{{Text: m.Content}},
			}
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		if m.HasBlocks() {
			var parts []*genai.Part
			for _, b := range m.Blocks {
				switch b.Type {
				case "text":
					parts = append(parts, &genai.Part{Text: b.Text})
				case "image":
					imageData, decodeErr := base64.StdEncoding.DecodeString(b.ImageData)
					if decodeErr != nil {
						return nil, fmt.Errorf("decode image: %w", decodeErr)
					}
					parts = append(parts, &genai.Part{
						InlineData: &genai.Blob{
							Data:     imageData,
							MIMEType: b.MimeType,
						},
					})
				}
			}
			contents = append(contents, &genai.Content{
				Role:  role,
				Parts: parts,
			})
		} else {
			contents = append(contents, &genai.Content{
				Role:  role,
				Parts: []*genai.Part{{Text: m.Content}},
			})
		}
	}

	// Build generation config
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(g.maxTokens),
	}

	if systemInstruction != nil {
		config.SystemInstruction = systemInstruction
	}

	temperature := g.temperature
	if req.Temperature > 0 {
		temperature = req.Temperature
	}
	if temperature > 0 {
		t := float32(temperature)
		config.Temperature = &t
	}

	// Send request
	result, err := client.Models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("API error: %v", err)
	}

	content := ""
	if len(result.Candidates) > 0 && result.Candidates[0].Content != nil &&
		len(result.Candidates[0].Content.Parts) > 0 {
		content = result.Candidates[0].Content.Parts[0].Text
	}

	var inputTokens, outputTokens int
	if result.UsageMetadata != nil {
		inputTokens = int(result.UsageMetadata.PromptTokenCount)
		outputTokens = int(result.UsageMetadata.CandidatesTokenCount)
	}

	return &Response{
		Content:      content,
		Provider:     "gemini",
		Model:        g.model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Latency:      time.Since(start),
	}, nil
}
