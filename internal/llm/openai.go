// OpenAI GPT provider
package llm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// OpenAI provider
type OpenAI struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
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
		baseURL:     cfg.BaseURL,
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

// newClient creates an OpenAI SDK client with the provider's configuration
func (o *OpenAI) newClient() openai.Client {
	opts := []option.RequestOption{
		option.WithAPIKey(o.apiKey),
	}
	if o.baseURL != "" {
		opts = append(opts, option.WithBaseURL(o.baseURL))
	}
	return openai.NewClient(opts...)
}

// Complete sends a completion request
func (o *OpenAI) Complete(ctx context.Context, req *Request) (*Response, error) {
	client := o.newClient()
	return completeOpenAICompatible(ctx, client, "openai", o.model, o.maxTokens, o.temperature, req)
}

// openaiCompatibleParams holds the resolved parameters for an OpenAI-compatible request.
type openaiCompatibleParams struct {
	maxTokens   int64
	temperature float64
}

// resolveParams merges provider defaults with request-level overrides.
func resolveParams(providerMaxTokens int, providerTemp float64, req *Request) openaiCompatibleParams {
	maxTokens := int64(providerMaxTokens)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}
	temperature := providerTemp
	if req.Temperature > 0 {
		temperature = req.Temperature
	}
	return openaiCompatibleParams{maxTokens: maxTokens, temperature: temperature}
}

// completeOpenAICompatible is the shared completion logic for OpenAI-compatible providers
// (OpenAI, DeepSeek, GLM). It converts messages, builds params, calls the SDK, and
// extracts the response.
func completeOpenAICompatible(
	ctx context.Context,
	client openai.Client,
	providerName, model string,
	maxTokens int,
	temperature float64,
	req *Request,
) (*Response, error) {
	start := time.Now()

	messages := convertToOpenAIMessages(req.Messages)

	p := resolveParams(maxTokens, temperature, req)

	params := openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		Messages:  messages,
		MaxTokens: openai.Int(p.maxTokens),
	}

	if p.temperature > 0 {
		params.Temperature = openai.Float(p.temperature)
	}

	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("API error: %v", err)
	}

	content := ""
	if len(completion.Choices) > 0 {
		content = completion.Choices[0].Message.Content
	}

	return &Response{
		Content:      content,
		Provider:     providerName,
		Model:        model,
		InputTokens:  int(completion.Usage.PromptTokens),
		OutputTokens: int(completion.Usage.CompletionTokens),
		Latency:      time.Since(start),
	}, nil
}

// convertToOpenAIMessages converts internal messages to OpenAI SDK format.
// Shared by all OpenAI-compatible providers (OpenAI, DeepSeek, GLM).
func convertToOpenAIMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion
	for _, m := range msgs {
		if m.HasBlocks() {
			var parts []openai.ChatCompletionContentPartUnionParam
			for _, b := range m.Blocks {
				switch b.Type {
				case "text":
					parts = append(parts, openai.TextContentPart(b.Text))
				case "image":
					parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
						URL: "data:" + b.MimeType + ";base64," + b.ImageData,
					}))
				}
			}
			switch m.Role {
			case "system":
				messages = append(messages, openai.SystemMessage(m.Content))
			case "user":
				messages = append(messages, openai.UserMessage(parts))
			case "assistant":
				messages = append(messages, openai.AssistantMessage(m.Content))
			}
		} else {
			switch m.Role {
			case "system":
				messages = append(messages, openai.SystemMessage(m.Content))
			case "user":
				messages = append(messages, openai.UserMessage(m.Content))
			case "assistant":
				messages = append(messages, openai.AssistantMessage(m.Content))
			}
		}
	}
	return messages
}
