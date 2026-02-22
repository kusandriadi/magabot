// Models listing for LLM providers using allm-go
package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/util"
	"github.com/kusandriadi/allm-go"
	"github.com/kusandriadi/allm-go/provider"
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
	client, ok := r.clients[providerName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	if !client.Provider().Available() {
		return nil, fmt.Errorf("provider not available: %s", providerName)
	}

	models, err := client.Models(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	// Convert allm.Model to ModelInfo
	result := make([]ModelInfo, len(models))
	for i, m := range models {
		result[i] = ModelInfo{
			ID:       m.ID,
			Name:     m.Name,
			Provider: m.Provider,
		}
	}

	return result, nil
}

// ListAllModels lists models from all available providers
func (r *Router) ListAllModels(ctx context.Context) map[string][]ModelInfo {
	result := make(map[string][]ModelInfo)

	r.mu.RLock()
	clients := make(map[string]*allm.Client)
	for k, v := range r.clients {
		clients[k] = v
	}
	r.mu.RUnlock()

	for name, client := range clients {
		if !client.Provider().Available() {
			continue
		}
		models, err := r.ListModels(ctx, name)
		if err == nil && len(models) > 0 {
			result[name] = models
		}
	}

	return result
}

// FetchModels fetches available models from a provider's API using the given key.
// Returns an error if the API call fails — callers should show the error to the user.
func FetchModels(providerName, apiKey, baseURL string) ([]ModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Create a temporary client for the provider
	// URL validation is per-provider: cloud providers block localhost, local allows it
	var p allm.Provider

	switch providerName {
	case "anthropic":
		if err := validateCloudBaseURL(baseURL); err != nil {
			return nil, err
		}
		opts := []provider.AnthropicOption{}
		if baseURL != "" {
			opts = append(opts, provider.WithAnthropicBaseURL(baseURL))
		}
		p = provider.Anthropic(apiKey, opts...)

	case "openai":
		if err := validateCloudBaseURL(baseURL); err != nil {
			return nil, err
		}
		opts := []provider.OpenAIOption{}
		if baseURL != "" {
			opts = append(opts, provider.WithOpenAIBaseURL(baseURL))
		}
		p = provider.OpenAI(apiKey, opts...)

	case "gemini":
		p = provider.Gemini(apiKey)

	case "deepseek":
		if err := validateCloudBaseURL(baseURL); err != nil {
			return nil, err
		}
		opts := []provider.CompatOption{}
		if baseURL != "" {
			opts = append(opts, provider.WithBaseURL(baseURL))
		}
		p = provider.DeepSeek(apiKey, opts...)

	case "glm":
		if err := validateCloudBaseURL(baseURL); err != nil {
			return nil, err
		}
		opts := []provider.CompatOption{}
		if baseURL != "" {
			opts = append(opts, provider.WithBaseURL(baseURL))
		}
		p = provider.GLM(apiKey, opts...)

	case "local":
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1" // Ollama default
		}
		if err := util.ValidateLocalBaseURL(baseURL); err != nil {
			return nil, fmt.Errorf("invalid local base URL: %w", err)
		}
		p = provider.Local(baseURL)

	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerName)
	}

	// Create client and fetch models
	client := allm.New(p)
	models, err := client.Models(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned — check your API key")
	}

	// Convert to ModelInfo
	result := make([]ModelInfo, len(models))
	for i, m := range models {
		result[i] = ModelInfo{
			ID:       m.ID,
			Name:     m.Name,
			Provider: m.Provider,
		}
	}

	// Sort by ID (newest first for OpenAI-like providers)
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID > result[j].ID
	})

	return result, nil
}

// validateCloudBaseURL validates a base URL for cloud providers (blocks localhost/private IPs)
func validateCloudBaseURL(baseURL string) error {
	if baseURL == "" {
		return nil
	}
	return util.ValidateBaseURL(baseURL)
}

// FormatModelList formats models for display
func FormatModelList(models map[string][]ModelInfo) string {
	var sb strings.Builder

	providers := make([]string, 0, len(models))
	for p := range models {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	for _, providerName := range providers {
		modelList := models[providerName]
		sb.WriteString(fmt.Sprintf("\n**%s** (%d models):\n", strings.ToUpper(providerName), len(modelList)))
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
