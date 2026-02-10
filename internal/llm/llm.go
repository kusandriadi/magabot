// Package llm provides unified interface for multiple LLM providers
package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var (
	ErrNoProvider     = errors.New("no LLM provider available")
	ErrProviderFailed = errors.New("LLM provider failed")
	ErrRateLimited    = errors.New("rate limit exceeded")
	ErrInputTooLong   = errors.New("input too long")
	ErrTimeout        = errors.New("request timeout")
)

// ContentBlock represents a content part (text or image)
type ContentBlock struct {
	Type      string `json:"type"`                 // "text" or "image"
	Text      string `json:"text,omitempty"`        // For text blocks
	MimeType  string `json:"mime_type,omitempty"`   // For image blocks (e.g. "image/jpeg")
	ImageData string `json:"image_data,omitempty"`  // Base64-encoded image data
}

// Message represents a chat message
type Message struct {
	Role    string         `json:"role"`    // "system", "user", "assistant"
	Content string         `json:"content"`
	Blocks  []ContentBlock `json:"blocks,omitempty"` // Multi-modal content blocks
}

// HasBlocks returns true if the message has multi-modal content blocks
func (m *Message) HasBlocks() bool {
	return len(m.Blocks) > 0
}

// Request represents an LLM request
type Request struct {
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

// Response represents an LLM response
type Response struct {
	Content      string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
}

// Provider interface for LLM providers
type Provider interface {
	Name() string
	Complete(ctx context.Context, req *Request) (*Response, error)
	Available() bool
}

// Router manages multiple LLM providers with fallback
type Router struct {
	providers     map[string]Provider
	defaultName   string
	fallbackChain []string
	systemPrompt  string
	maxInput      int
	timeout       time.Duration
	rateLimiter   *rateLimiter
	logger        *slog.Logger
	mu            sync.RWMutex
}

// Config for LLM router
type Config struct {
	Default       string
	FallbackChain []string
	SystemPrompt  string
	MaxInput      int
	Timeout       time.Duration
	RateLimit     int // requests per minute per user
	Logger        *slog.Logger
}

// NewRouter creates a new LLM router
func NewRouter(cfg *Config) *Router {
	if cfg.MaxInput == 0 {
		cfg.MaxInput = 10000
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Router{
		providers:     make(map[string]Provider),
		defaultName:   cfg.Default,
		fallbackChain: cfg.FallbackChain,
		systemPrompt:  cfg.SystemPrompt,
		maxInput:      cfg.MaxInput,
		timeout:       cfg.Timeout,
		rateLimiter:   newRateLimiter(cfg.RateLimit),
		logger:        logger,
	}
}

// DetectProvider detects the provider name from a model name
func DetectProvider(model string) string {
	model = strings.ToLower(model)
	prefixes := map[string]string{
		"claude":   "anthropic",
		"gpt":     "openai",
		"o1":      "openai",
		"o3":      "openai",
		"gemini":  "gemini",
		"glm":    "glm",
		"deepseek": "deepseek",
	}
	for prefix, provider := range prefixes {
		if strings.Contains(model, prefix) {
			return provider
		}
	}
	return ""
}

// Register registers a provider
func (r *Router) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
	r.logger.Info("registered LLM provider", "name", p.Name())

	// Auto-detect default provider if not explicitly set
	if r.defaultName == "" && p.Available() {
		r.defaultName = p.Name()
		r.logger.Info("auto-selected default provider", "name", p.Name())
	}
}

// Complete sends a request to the LLM
func (r *Router) Complete(ctx context.Context, userID, text string) (*Response, error) {
	// Rate limit check
	if !r.rateLimiter.allow(userID) {
		return nil, ErrRateLimited
	}

	// Input length check
	if len(text) > r.maxInput {
		return nil, ErrInputTooLong
	}

	// Build request
	req := &Request{
		Messages: []Message{
			{Role: "user", Content: text},
		},
	}

	// Add system prompt if set
	if r.systemPrompt != "" {
		req.Messages = append([]Message{{Role: "system", Content: r.systemPrompt}}, req.Messages...)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	resp, err := r.tryProviders(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Chat sends a multi-turn conversation
func (r *Router) Chat(ctx context.Context, userID string, messages []Message) (*Response, error) {
	if !r.rateLimiter.allow(userID) {
		return nil, ErrRateLimited
	}

	// Calculate total input length (including multi-modal block data)
	totalLen := 0
	for _, m := range messages {
		totalLen += len(m.Content)
		for _, b := range m.Blocks {
			totalLen += len(b.Text) + len(b.ImageData)
		}
	}
	if totalLen > r.maxInput {
		return nil, ErrInputTooLong
	}

	req := &Request{Messages: messages}

	if r.systemPrompt != "" {
		req.Messages = append([]Message{{Role: "system", Content: r.systemPrompt}}, req.Messages...)
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.tryProviders(ctx, req)
}

// tryProviders attempts all providers in order: default, fallback chain, any available
func (r *Router) tryProviders(ctx context.Context, req *Request) (*Response, error) {
	var lastErr error

	// Try default provider first
	r.mu.RLock()
	defaultProvider, ok := r.providers[r.defaultName]
	r.mu.RUnlock()

	if ok && defaultProvider.Available() {
		resp, err := defaultProvider.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = fmt.Errorf("%s: %w", r.defaultName, err)
		r.logger.Warn("default provider failed", "provider", r.defaultName, "error", err)
	}

	// Try fallback chain
	for _, name := range r.fallbackChain {
		if name == r.defaultName {
			continue
		}

		r.mu.RLock()
		provider, ok := r.providers[name]
		r.mu.RUnlock()

		if !ok || !provider.Available() {
			continue
		}

		resp, err := provider.Complete(ctx, req)
		if err == nil {
			r.logger.Info("used fallback provider", "provider", name)
			return resp, nil
		}
		lastErr = fmt.Errorf("%s: %w", name, err)
		r.logger.Warn("fallback provider failed", "provider", name, "error", err)
	}

	// Last resort: try any available provider
	r.mu.RLock()
	type candidate struct {
		name     string
		provider Provider
	}
	var candidates []candidate
	for name, provider := range r.providers {
		if name != r.defaultName && provider.Available() {
			candidates = append(candidates, candidate{name, provider})
		}
	}
	r.mu.RUnlock()

	for _, c := range candidates {
		resp, err := c.provider.Complete(ctx, req)
		if err == nil {
			r.logger.Info("used available provider", "provider", c.name)
			return resp, nil
		}
		lastErr = fmt.Errorf("%s: %w", c.name, err)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoProvider, lastErr)
	}
	return nil, ErrNoProvider
}

// SetSystemPrompt updates the system prompt
func (r *Router) SetSystemPrompt(prompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.systemPrompt = prompt
}

// Providers returns list of registered providers
func (r *Router) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Stats returns usage statistics
func (r *Router) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := map[string]interface{}{
		"default":   r.defaultName,
		"providers": len(r.providers),
	}

	available := []string{}
	for name, p := range r.providers {
		if p.Available() {
			available = append(available, name)
		}
	}
	stats["available"] = available

	return stats
}

// Simple rate limiter
type rateLimiter struct {
	requests  map[string][]time.Time
	limit     int
	window    time.Duration
	mu        sync.Mutex
	callCount int // tracks calls for periodic cleanup
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   time.Minute,
	}
}

func (r *rateLimiter) allow(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Periodic cleanup: every 100 calls, purge stale entries
	r.callCount++
	if r.callCount%100 == 0 {
		for uid, times := range r.requests {
			if uid == userID {
				continue
			}
			hasRecent := false
			for _, t := range times {
				if t.After(cutoff) {
					hasRecent = true
					break
				}
			}
			if !hasRecent {
				delete(r.requests, uid)
			}
		}
	}

	// Clean old entries for current user
	var fresh []time.Time
	for _, t := range r.requests[userID] {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}

	if len(fresh) >= r.limit {
		r.requests[userID] = fresh
		return false
	}

	fresh = append(fresh, now)
	r.requests[userID] = fresh
	return true
}

// extractAPIMessage extracts a human-readable message from API error strings
func extractAPIMessage(err error) string {
	s := err.Error()
	// Look for "message":"..." in JSON error responses
	if idx := strings.Index(s, "\"message\":\""); idx != -1 {
		start := idx + len("\"message\":\"")
		end := strings.Index(s[start:], "\"")
		if end != -1 {
			return s[start : start+end]
		}
	}
	return ""
}

// Helper to format error for user
func FormatError(err error) string {
	switch {
	case errors.Is(err, ErrRateLimited):
		return "⏳ Too many requests. Please wait a moment."
	case errors.Is(err, ErrInputTooLong):
		return "📝 Message too long. Please shorten it."
	case errors.Is(err, ErrTimeout):
		return "⏱️ Request timed out. Please try again."
	case errors.Is(err, ErrNoProvider):
		if msg := extractAPIMessage(err); msg != "" {
			return fmt.Sprintf("🔌 Provider error: %s", msg)
		}
		return "🔌 No AI provider available."
	default:
		return fmt.Sprintf("❌ Error: %v", err)
	}
}
