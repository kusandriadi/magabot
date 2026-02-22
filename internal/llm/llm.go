// Package llm provides unified interface for multiple LLM providers using allm-go
package llm

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/util"
	"github.com/kusandriadi/allm-go"
)

// Re-export allm types for convenience
type (
	Message  = allm.Message
	Response = allm.Response
	Image    = allm.Image
)

var (
	ErrNoProvider     = errors.New("no LLM provider available")
	ErrProviderFailed = errors.New("LLM provider failed")
	ErrRateLimited    = errors.New("rate limit exceeded")
	ErrInputTooLong   = errors.New("input too long")
	ErrTimeout        = errors.New("request timeout")
)

// Router manages LLM clients
type Router struct {
	clients      map[string]*allm.Client
	mainName     string
	systemPrompt string
	maxInput     int
	timeout      time.Duration
	rateLimiter  *rateLimiter
	logger       *slog.Logger
	mu           sync.RWMutex
}

// Config for LLM router
type Config struct {
	Main         string
	SystemPrompt string
	MaxInput     int
	Timeout      time.Duration
	RateLimit    int // requests per minute per user
	Logger       *slog.Logger
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
		clients:      make(map[string]*allm.Client),
		mainName:     cfg.Main,
		systemPrompt: cfg.SystemPrompt,
		maxInput:     cfg.MaxInput,
		timeout:      cfg.Timeout,
		rateLimiter:  newRateLimiter(cfg.RateLimit),
		logger:       logger,
	}
}

// providerPrefixes maps model name substrings to provider names (allocated once)
var providerPrefixes = map[string]string{
	"claude":    "anthropic",
	"gpt":       "openai",
	"o1":        "openai",
	"o3":        "openai",
	"gemini":    "gemini",
	"glm":       "glm",
	"deepseek":  "deepseek",
	"llama":     "local",
	"mistral":   "local",
	"mixtral":   "local",
	"phi":       "local",
	"qwen":      "local",
	"codellama": "local",
}

// DetectProvider detects the provider name from a model name
func DetectProvider(model string) string {
	model = strings.ToLower(model)
	for prefix, provider := range providerPrefixes {
		if strings.Contains(model, prefix) {
			return provider
		}
	}
	return ""
}

// Register registers a provider with a name
func (r *Router) Register(name string, client *allm.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[name] = client
	r.logger.Info("registered LLM provider", "name", name)

	// Auto-detect main provider if not explicitly set
	if r.mainName == "" && client.Provider().Available() {
		r.mainName = name
		r.logger.Info("auto-selected main provider", "name", name)
	}
}

// Complete sends a simple text request to the LLM
func (r *Router) Complete(ctx context.Context, userID, text string) (*Response, error) {
	// Rate limit check
	if !r.rateLimiter.allow(userID) {
		r.logger.Warn("rate limit exceeded", "user", util.MaskSecret(userID))
		return nil, ErrRateLimited
	}

	// Sanitize input to remove control characters (injection protection)
	text = util.SanitizeInput(text)

	// Input length check
	if len(text) > r.maxInput {
		r.logger.Warn("input too long", "user", util.MaskSecret(userID), "length", len(text), "max", r.maxInput)
		return nil, ErrInputTooLong
	}

	// Build messages
	messages := []allm.Message{
		{Role: "user", Content: text},
	}

	// Add system prompt if set
	if r.systemPrompt != "" {
		messages = append([]allm.Message{{Role: "system", Content: r.systemPrompt}}, messages...)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.chat(ctx, messages)
}

// Chat sends a multi-turn conversation
func (r *Router) Chat(ctx context.Context, userID string, messages []Message) (*Response, error) {
	if !r.rateLimiter.allow(userID) {
		r.logger.Warn("rate limit exceeded", "user", util.MaskSecret(userID))
		return nil, ErrRateLimited
	}

	// Copy messages to avoid mutating caller's slice during sanitization
	sanitized := make([]allm.Message, len(messages))
	copy(sanitized, messages)
	for i := range sanitized {
		sanitized[i].Content = util.SanitizeInput(sanitized[i].Content)
	}
	messages = sanitized

	// Calculate total input length (with size limit for images)
	totalLen := 0
	for _, m := range messages {
		totalLen += len(m.Content)
		for _, img := range m.Images {
			// Limit image size contribution to prevent bypass via huge images
			imgSize := len(img.Data)
			if imgSize > 10*1024*1024 { // 10MB max per image
				r.logger.Warn("image too large", "user", util.MaskSecret(userID), "size", imgSize)
				return nil, fmt.Errorf("image too large: %d bytes (max 10MB)", imgSize)
			}
			totalLen += imgSize
		}
	}
	if totalLen > r.maxInput*10 { // Allow 10x for images
		r.logger.Warn("total input too large", "user", util.MaskSecret(userID), "length", totalLen)
		return nil, ErrInputTooLong
	}

	// Add system prompt if set
	if r.systemPrompt != "" {
		messages = append([]allm.Message{{Role: "system", Content: r.systemPrompt}}, messages...)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.chat(ctx, messages)
}

// chat is the internal method that calls the main client
func (r *Router) chat(ctx context.Context, messages []allm.Message) (*Response, error) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: provider %q not registered", ErrNoProvider, r.mainName)
	}

	if !client.Provider().Available() {
		return nil, fmt.Errorf("%w: provider %q not available", ErrNoProvider, r.mainName)
	}

	resp, err := client.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrProviderFailed, r.mainName, err)
	}

	return resp, nil
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

	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// Stats returns usage statistics
func (r *Router) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := map[string]interface{}{
		"main":      r.mainName,
		"providers": len(r.clients),
	}

	available := []string{}
	for name, c := range r.clients {
		if c.Provider().Available() {
			available = append(available, name)
		}
	}
	stats["available"] = available

	return stats
}

// Simple rate limiter with bounded memory
type rateLimiter struct {
	requests  map[string][]time.Time
	limit     int
	window    time.Duration
	mu        sync.Mutex
	callCount int // tracks calls for periodic cleanup
	maxUsers  int // maximum number of users to track
}

const (
	defaultMaxUsers    = 10000 // Maximum users to track in rate limiter
	cleanupInterval    = 100   // Cleanup every N calls
	maxEntriesPerClean = 1000  // Max entries to clean in one pass
)

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   time.Minute,
		maxUsers: defaultMaxUsers,
	}
}

func (r *rateLimiter) allow(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Periodic cleanup: every 100 calls, purge stale entries
	r.callCount = (r.callCount + 1) % cleanupInterval
	if r.callCount == 0 {
		r.cleanup(cutoff, userID)
	}

	// If we have too many users, reject new users (DoS protection)
	if _, exists := r.requests[userID]; !exists && len(r.requests) >= r.maxUsers {
		// Try aggressive cleanup first
		r.cleanup(cutoff, userID)
		if len(r.requests) >= r.maxUsers {
			return false // Still at capacity, reject
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

// cleanup removes stale entries from the rate limiter map
// Must be called with lock held
func (r *rateLimiter) cleanup(cutoff time.Time, currentUserID string) {
	cleaned := 0
	for uid, times := range r.requests {
		if cleaned >= maxEntriesPerClean {
			break // Limit cleanup to prevent long lock holds
		}
		if uid == currentUserID {
			continue // Don't clean current user
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
			cleaned++
		}
	}
}

// Stats returns rate limiter statistics
func (r *rateLimiter) Stats() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return map[string]interface{}{
		"tracked_users": len(r.requests),
		"limit":         r.limit,
		"window":        r.window.String(),
		"max_users":     r.maxUsers,
	}
}

// FormatError formats error for user display with sanitization
func FormatError(err error) string {
	switch {
	case errors.Is(err, ErrRateLimited):
		return "⏳ Too many requests. Please wait a moment."
	case errors.Is(err, ErrInputTooLong):
		return "📝 Message too long. Please shorten it."
	case errors.Is(err, ErrTimeout):
		return "⏱️ Request timed out. Please try again."
	case errors.Is(err, ErrNoProvider):
		if msg := util.ExtractAPIMessage(err); msg != "" {
			return fmt.Sprintf("🔌 Provider error: %s", msg)
		}
		return "🔌 No AI provider available."
	case errors.Is(err, ErrProviderFailed):
		// Extract sanitized message from provider error
		if msg := util.ExtractAPIMessage(err); msg != "" {
			return fmt.Sprintf("❌ Provider error: %s", msg)
		}
		return "❌ AI provider failed. Please try again."
	default:
		// Generic error - don't leak internal details
		return "❌ An error occurred. Please try again."
	}
}

// allowedImageMIME is the set of accepted image MIME types
var allowedImageMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// ImageFromBase64 creates an Image from base64-encoded data and mime type.
// Only image/jpeg, image/png, image/gif, image/webp MIME types are accepted.
func ImageFromBase64(mimeType, base64Data string) (Image, error) {
	if !allowedImageMIME[mimeType] {
		return Image{}, fmt.Errorf("unsupported image MIME type: %s", mimeType)
	}
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return Image{}, fmt.Errorf("decode base64: %w", err)
	}
	return Image{
		MimeType: mimeType,
		Data:     data,
	}, nil
}

// ImageFromBytes creates an Image from raw bytes and mime type
func ImageFromBytes(mimeType string, data []byte) Image {
	return Image{
		MimeType: mimeType,
		Data:     data,
	}
}
