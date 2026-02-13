// Models listing for LLM providers
package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"
)

// ModelInfo represents a model
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Description string `json:"description,omitempty"`
	MaxTokens   int    `json:"max_tokens,omitempty"`
}

// ListModels lists available models from a provider via API
func (r *Router) ListModels(ctx context.Context, providerName string) ([]ModelInfo, error) {
	r.mu.RLock()
	provider, ok := r.providers[providerName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	if !provider.Available() {
		return nil, fmt.Errorf("provider not available: %s", providerName)
	}

	switch p := provider.(type) {
	case *OpenAI:
		return listOpenAIModels(ctx, p)
	case *Gemini:
		return listGeminiModels(ctx, p)
	case *GLM:
		return listGLMModelsAPI(ctx, p)
	case *Anthropic:
		return listAnthropicModelsAPI(ctx, p)
	case *DeepSeek:
		return listDeepSeekModelsAPI(ctx, p)
	default:
		return nil, fmt.Errorf("list models not supported for: %s", providerName)
	}
}

// ListAllModels lists models from all available providers
func (r *Router) ListAllModels(ctx context.Context) map[string][]ModelInfo {
	result := make(map[string][]ModelInfo)

	r.mu.RLock()
	providers := make(map[string]Provider)
	for k, v := range r.providers {
		providers[k] = v
	}
	r.mu.RUnlock()

	for name, provider := range providers {
		if !provider.Available() {
			continue
		}
		models, err := r.ListModels(ctx, name)
		if err == nil {
			result[name] = models
		}
	}

	return result
}

// listAnthropicModelsAPI lists models from the Anthropic API using the SDK
func listAnthropicModelsAPI(ctx context.Context, a *Anthropic) ([]ModelInfo, error) {
	var opts []anthropicOption.RequestOption
	if a.baseURL != "" {
		opts = append(opts, anthropicOption.WithBaseURL(a.baseURL))
	}
	opts = append(opts, anthropicOption.WithAPIKey(a.apiKey))

	client := anthropic.NewClient(opts...)
	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})

	var models []ModelInfo
	for pager.Next() {
		m := pager.Current()
		models = append(models, ModelInfo{
			ID:       m.ID,
			Name:     m.DisplayName,
			Provider: "anthropic",
		})
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	return models, nil
}

// OpenAI models via SDK
func listOpenAIModels(ctx context.Context, o *OpenAI) ([]ModelInfo, error) {
	client := o.newClient()

	// Use auto-paging to list all models
	pager := client.Models.ListAutoPaging(ctx)

	// Filter chat models only
	chatPrefixes := []string{"gpt-4", "gpt-3.5", "o1", "o3"}
	var models []ModelInfo

	for pager.Next() {
		m := pager.Current()
		for _, prefix := range chatPrefixes {
			if strings.HasPrefix(m.ID, prefix) {
				models = append(models, ModelInfo{
					ID:       m.ID,
					Name:     m.ID,
					Provider: "openai",
				})
				break
			}
		}
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID > models[j].ID // Newest first
	})

	return models, nil
}

// Gemini models via SDK
func listGeminiModels(ctx context.Context, g *Gemini) ([]ModelInfo, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  g.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	var models []ModelInfo
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		// Only include models that support generateContent
		for _, action := range model.SupportedActions {
			if action == "generateContent" {
				id := strings.TrimPrefix(model.Name, "models/")
				models = append(models, ModelInfo{
					ID:          id,
					Name:        model.DisplayName,
					Provider:    "gemini",
					Description: model.Description,
				})
				break
			}
		}
	}

	return models, nil
}

// listGLMModelsAPI lists models from the GLM/Z.AI API (OpenAI-compatible)
func listGLMModelsAPI(ctx context.Context, g *GLM) ([]ModelInfo, error) {
	return fetchOpenAICompatibleModels(ctx, g.apiKey, g.baseURL,
		[]string{"glm", "chatglm"}, "glm")
}

// listDeepSeekModelsAPI lists models from the DeepSeek API (OpenAI-compatible)
func listDeepSeekModelsAPI(ctx context.Context, d *DeepSeek) ([]ModelInfo, error) {
	return fetchOpenAICompatibleModels(ctx, d.config.APIKey, d.config.BaseURL,
		[]string{"deepseek"}, "deepseek")
}

// FetchModels fetches available models from a provider's API using the given key.
// Returns an error if the API call fails — callers should show the error to the user.
func FetchModels(provider, apiKey, baseURL string) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch provider {
	case "anthropic":
		return fetchAnthropicModels(ctx, apiKey, baseURL)
	case "openai":
		return fetchOpenAICompatibleModels(ctx, apiKey, baseURL,
			[]string{"gpt-4", "gpt-3.5", "o1", "o3", "chatgpt"}, "openai")
	case "gemini":
		return fetchGeminiModels(ctx, apiKey)
	case "deepseek":
		if baseURL == "" {
			baseURL = deepseekAPIURL
		}
		return fetchOpenAICompatibleModels(ctx, apiKey, baseURL,
			[]string{"deepseek"}, "deepseek")
	case "glm":
		if baseURL == "" {
			baseURL = glmAPIURL
		}
		return fetchOpenAICompatibleModels(ctx, apiKey, baseURL,
			[]string{"glm", "chatglm"}, "glm")
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// fetchAnthropicModels lists models from the Anthropic API
func fetchAnthropicModels(ctx context.Context, apiKey, baseURL string) ([]ModelInfo, error) {
	var opts []anthropicOption.RequestOption
	if baseURL != "" {
		opts = append(opts, anthropicOption.WithBaseURL(baseURL))
	}
	opts = append(opts, anthropicOption.WithAPIKey(apiKey))

	client := anthropic.NewClient(opts...)
	pager := client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})

	var models []ModelInfo
	for pager.Next() {
		m := pager.Current()
		models = append(models, ModelInfo{
			ID:       m.ID,
			Name:     m.DisplayName,
			Provider: "anthropic",
		})
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned — check your API key")
	}

	return models, nil
}

// fetchOpenAICompatibleModels lists models from an OpenAI-compatible API
func fetchOpenAICompatibleModels(ctx context.Context, apiKey, baseURL string, prefixes []string, providerName string) ([]ModelInfo, error) {
	opts := []openaiOption.RequestOption{
		openaiOption.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, openaiOption.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	pager := client.Models.ListAutoPaging(ctx)

	var models []ModelInfo
	for pager.Next() {
		m := pager.Current()
		for _, prefix := range prefixes {
			if strings.HasPrefix(strings.ToLower(m.ID), prefix) {
				models = append(models, ModelInfo{
					ID:       m.ID,
					Name:     m.ID,
					Provider: providerName,
				})
				break
			}
		}
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned — check your API key")
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID > models[j].ID
	})
	return models, nil
}

// fetchGeminiModels lists models from the Gemini API
func fetchGeminiModels(ctx context.Context, apiKey string) ([]ModelInfo, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	var models []ModelInfo
	for model, err := range client.Models.All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		for _, action := range model.SupportedActions {
			if action == "generateContent" {
				id := strings.TrimPrefix(model.Name, "models/")
				models = append(models, ModelInfo{
					ID:          id,
					Name:        model.DisplayName,
					Provider:    "gemini",
					Description: model.Description,
				})
				break
			}
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned — check your API key")
	}

	return models, nil
}

// FormatModelList formats models for display
func FormatModelList(models map[string][]ModelInfo) string {
	var sb strings.Builder

	providers := make([]string, 0, len(models))
	for p := range models {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	for _, provider := range providers {
		modelList := models[provider]
		sb.WriteString(fmt.Sprintf("\n**%s** (%d models):\n", strings.ToUpper(provider), len(modelList)))
		for _, m := range modelList {
			if len(modelList) > 10 {
				// Compact for many models
				sb.WriteString(fmt.Sprintf("• `%s`\n", m.ID))
			} else {
				if m.Description != "" {
					sb.WriteString(fmt.Sprintf("• `%s` - %s\n", m.ID, m.Description))
				} else {
					sb.WriteString(fmt.Sprintf("• `%s`\n", m.ID))
				}
			}
		}
	}

	return sb.String()
}
