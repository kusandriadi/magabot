// Models listing for LLM providers
package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// ListModels lists available models from a provider
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
		return listGLMModels(ctx, p)
	case *Anthropic:
		return listAnthropicModels(), nil
	case *DeepSeek:
		return listDeepSeekModels(p), nil
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

// Anthropic models (hardcoded)
func listAnthropicModels() []ModelInfo {
	return []ModelInfo{
		{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Provider: "anthropic", MaxTokens: 16384},
		{ID: "claude-sonnet-4-5-20250929", Name: "Claude Sonnet 4.5", Provider: "anthropic", MaxTokens: 16384},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Provider: "anthropic", MaxTokens: 8192},
	}
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

// GLM models (hardcoded)
func listGLMModels(ctx context.Context, g *GLM) ([]ModelInfo, error) {
	return []ModelInfo{
		{ID: "glm-4.7", Name: "GLM-4.7", Provider: "glm", Description: "Latest, most capable"},
		{ID: "glm-4.6", Name: "GLM-4.6", Provider: "glm", Description: "Previous generation"},
		{ID: "glm-4.5", Name: "GLM-4.5", Provider: "glm", Description: "Stable"},
		{ID: "glm-4-plus", Name: "GLM-4 Plus", Provider: "glm", Description: "Most capable (legacy)"},
		{ID: "glm-4-flash", Name: "GLM-4 Flash", Provider: "glm", Description: "Fast, free tier"},
		{ID: "glm-4-long", Name: "GLM-4 Long", Provider: "glm", Description: "Long context"},
	}, nil
}

// DeepSeek models (hardcoded)
func listDeepSeekModels(d *DeepSeek) []ModelInfo {
	var models []ModelInfo
	for _, id := range d.Models() {
		models = append(models, ModelInfo{
			ID:       id,
			Name:     id,
			Provider: "deepseek",
		})
	}
	return models
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
