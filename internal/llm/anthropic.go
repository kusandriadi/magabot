// Anthropic Claude provider
package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic provider
type Anthropic struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
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

	// Try to load from environment
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
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
		baseURL:     cfg.BaseURL,
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

	// Build SDK client options
	var clientOpts []option.RequestOption
	if a.baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(a.baseURL))
	}
	clientOpts = append(clientOpts, option.WithAPIKey(a.apiKey))

	client := anthropic.NewClient(clientOpts...)

	// Convert messages to Anthropic SDK format
	var systemBlocks []anthropic.TextBlockParam
	var messages []anthropic.MessageParam

	for _, m := range req.Messages {
		if m.Role == "system" {
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: m.Content})
			continue
		}
		if m.HasBlocks() {
			var parts []anthropic.ContentBlockParamUnion
			for _, b := range m.Blocks {
				switch b.Type {
				case "text":
					parts = append(parts, anthropic.NewTextBlock(b.Text))
				case "image":
					parts = append(parts, anthropic.NewImageBlockBase64(b.MimeType, b.ImageData))
				}
			}
			switch m.Role {
			case "user":
				messages = append(messages, anthropic.NewUserMessage(parts...))
			case "assistant":
				messages = append(messages, anthropic.NewAssistantMessage(parts...))
			}
		} else {
			switch m.Role {
			case "user":
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			case "assistant":
				messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
			}
		}
	}

	// Build request params
	maxTokens := int64(a.maxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}

	temperature := a.temperature
	if req.Temperature > 0 {
		temperature = req.Temperature
	}
	if temperature > 0 {
		params.Temperature = anthropic.Float(temperature)
	}

	// Send request
	message, err := client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("API error: %v", err)
	}

	// Extract response text
	content := ""
	if len(message.Content) > 0 {
		content = message.Content[0].Text
	}

	return &Response{
		Content:      content,
		Provider:     "anthropic",
		Model:        a.model,
		InputTokens:  int(message.Usage.InputTokens),
		OutputTokens: int(message.Usage.OutputTokens),
		Latency:      time.Since(start),
	}, nil
}
