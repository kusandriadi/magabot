// Models listing for LLM providers
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
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

// Anthropic models (hardcoded - no list API)
func listAnthropicModels() []ModelInfo {
	return []ModelInfo{
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Provider: "anthropic", MaxTokens: 8192},
		{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", Provider: "anthropic", MaxTokens: 4096},
		{ID: "claude-3-sonnet-20240229", Name: "Claude 3 Sonnet", Provider: "anthropic", MaxTokens: 4096},
		{ID: "claude-3-haiku-20240307", Name: "Claude 3 Haiku", Provider: "anthropic", MaxTokens: 4096},
	}
}

// OpenAI models via API
func listOpenAIModels(ctx context.Context, o *OpenAI) ([]ModelInfo, error) {
	url := strings.TrimSuffix(o.baseURL, "/chat/completions") + "/models"
	if !strings.Contains(url, "/models") {
		url = "https://api.openai.com/v1/models"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// Filter chat models only
	var models []ModelInfo
	chatModels := []string{"gpt-4", "gpt-3.5", "o1", "o3"}
	for _, m := range result.Data {
		for _, prefix := range chatModels {
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

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID > models[j].ID // Newest first
	})

	return models, nil
}

// Gemini models via API
func listGeminiModels(ctx context.Context, g *Gemini) ([]ModelInfo, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", g.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			Description                string   `json:"description"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var models []ModelInfo
	for _, m := range result.Models {
		// Only include models that support generateContent
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				// Extract model ID from name (e.g., "models/gemini-pro" -> "gemini-pro")
				id := strings.TrimPrefix(m.Name, "models/")
				models = append(models, ModelInfo{
					ID:          id,
					Name:        m.DisplayName,
					Provider:    "gemini",
					Description: m.Description,
				})
				break
			}
		}
	}

	return models, nil
}

// GLM models via API
func listGLMModels(ctx context.Context, g *GLM) ([]ModelInfo, error) {
	// GLM doesn't have a public models endpoint, return known models
	return []ModelInfo{
		{ID: "glm-4-plus", Name: "GLM-4 Plus", Provider: "glm", Description: "Most capable"},
		{ID: "glm-4", Name: "GLM-4", Provider: "glm", Description: "Balanced"},
		{ID: "glm-4-air", Name: "GLM-4 Air", Provider: "glm", Description: "Fast"},
		{ID: "glm-4-airx", Name: "GLM-4 AirX", Provider: "glm", Description: "Fastest"},
		{ID: "glm-4-flash", Name: "GLM-4 Flash", Provider: "glm", Description: "Free tier"},
		{ID: "glm-4-long", Name: "GLM-4 Long", Provider: "glm", Description: "Long context"},
		{ID: "glm-4v", Name: "GLM-4V", Provider: "glm", Description: "Vision"},
		{ID: "glm-4v-plus", Name: "GLM-4V Plus", Provider: "glm", Description: "Vision+"},
	}, nil
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
