// Package llm provides unified interface for multiple LLM providers using allm-go
package llm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/util"
	"github.com/kusandriadi/allm-go"
	"github.com/kusandriadi/allm-go/provider"
)

// Re-export allm types for convenience
type (
	Message            = allm.Message
	Response           = allm.Response
	Image              = allm.Image
	ResponseFormat     = allm.ResponseFormat
	ThinkingConfig     = allm.ThinkingConfig
	StreamChunk        = allm.StreamChunk
	CacheControl       = allm.CacheControl
	TokenCount         = allm.TokenCount
	ImageResponse      = allm.ImageResponse
	ImageOption        = allm.ImageOption
	Document           = allm.Document
	Citation           = allm.Citation
	StreamUsage        = allm.StreamUsage
	SpeechRequest      = allm.SpeechRequest
	SpeechResponse     = allm.SpeechResponse
	TranscribeRequest  = allm.TranscribeRequest
	TranscribeResponse = allm.TranscribeResponse
	LogProb            = allm.LogProb
	TokenLogProb       = allm.TokenLogProb
	SearchResult       = allm.SearchResult
	HealthStatus       = allm.HealthStatus
)

var (
	WithImageSize    = allm.WithImageSize
	WithImageQuality = allm.WithImageQuality
	WithImageCount   = allm.WithImageCount
	WithImageModel   = allm.WithImageModel
)

const (
	ResponseFormatJSON       = allm.ResponseFormatJSON
	ResponseFormatJSONSchema = allm.ResponseFormatJSONSchema
)

var (
	ErrNoProvider     = allm.ErrNoProvider
	ErrProviderFailed = allm.ErrProvider
	ErrRateLimited    = allm.ErrRateLimited
	ErrInputTooLong   = allm.ErrInputTooLong
	ErrTimeout        = allm.ErrTimeout
)

// Router manages LLM clients
type Router struct {
	clients       map[string]*allm.Client
	mainName      string
	systemPrompt  string
	maxInput      int
	timeout       time.Duration
	rateLimiter   *rateLimiter
	logger        *slog.Logger
	mu            sync.RWMutex
	promptCaching bool
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

// DetectProvider detects the provider name from a model name.
// Delegates to allm.DetectProvider.
func DetectProvider(model string) string {
	return string(allm.DetectProvider(model))
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

// EnablePromptCaching enables prompt caching on system prompts
func (r *Router) EnablePromptCaching() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.promptCaching = true
}

// Complete sends a simple text request to the LLM
func (r *Router) Complete(ctx context.Context, userID, text string) (*Response, error) {
	// Rate limit check
	if !r.rateLimiter.allow(userID) {
		r.logger.Warn("rate limit exceeded", "user", util.MaskSecret(userID))
		return nil, ErrRateLimited
	}

	// Sanitize input to remove control characters (injection protection)
	text = allm.SanitizeInput(text)

	// Build messages
	messages := []Message{
		{Role: "user", Content: text},
	}

	allmMessages := r.buildMessages(messages)

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.chat(ctx, allmMessages)
}

// Chat sends a multi-turn conversation
func (r *Router) Chat(ctx context.Context, userID string, messages []Message) (*Response, error) {
	if !r.rateLimiter.allow(userID) {
		r.logger.Warn("rate limit exceeded", "user", util.MaskSecret(userID))
		return nil, ErrRateLimited
	}

	// Copy messages to avoid mutating caller's slice during sanitization
	sanitized := make([]Message, len(messages))
	copy(sanitized, messages)
	for i := range sanitized {
		sanitized[i].Content = allm.SanitizeInput(sanitized[i].Content)
	}

	allmMessages := r.buildMessages(sanitized)

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.chat(ctx, allmMessages)
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

	if resp.RequestID != "" {
		r.logger.Debug("llm response", "provider", r.mainName, "request_id", resp.RequestID, "tokens_in", resp.InputTokens, "tokens_out", resp.OutputTokens)
	}

	return resp, nil
}

// buildMessages converts user messages to allm messages with system prompt
func (r *Router) buildMessages(messages []Message) []allm.Message {
	r.mu.RLock()
	systemPrompt := r.systemPrompt
	promptCaching := r.promptCaching
	r.mu.RUnlock()

	result := make([]allm.Message, 0, len(messages)+1)

	// Add system prompt if set
	if systemPrompt != "" {
		sysMsg := allm.Message{Role: "system", Content: systemPrompt}
		if promptCaching {
			sysMsg.CacheControl = &allm.CacheControl{Type: "ephemeral"}
		}
		result = append(result, sysMsg)
	}

	// Append user messages
	result = append(result, messages...)

	return result
}

// StreamChat streams a chat response
func (r *Router) StreamChat(ctx context.Context, userID string, messages []Message) (<-chan StreamChunk, error) {
	// Rate limit check
	if !r.rateLimiter.allow(userID) {
		r.logger.Warn("rate limit exceeded", "user", util.MaskSecret(userID))
		return nil, ErrRateLimited
	}

	// Copy messages to avoid mutating caller's slice during sanitization
	sanitized := make([]Message, len(messages))
	copy(sanitized, messages)
	for i := range sanitized {
		sanitized[i].Content = allm.SanitizeInput(sanitized[i].Content)
	}

	// Get main client
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: provider %q not registered", ErrNoProvider, r.mainName)
	}

	// Build allm messages
	allmMessages := r.buildMessages(sanitized)

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	go func() {
		<-ctx.Done()
		cancel()
	}()

	// Return stream channel
	return client.Stream(ctx, allmMessages), nil
}

// CountTokens counts tokens in a set of messages
func (r *Router) CountTokens(ctx context.Context, messages []Message) (*TokenCount, error) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()

	if !ok {
		return nil, ErrNoProvider
	}

	allmMessages := r.buildMessages(messages)
	return client.CountTokens(ctx, allmMessages)
}

// GenerateImage generates an image from a text prompt
func (r *Router) GenerateImage(ctx context.Context, userID, prompt string, opts ...ImageOption) (*ImageResponse, error) {
	// Rate limit check
	if r.rateLimiter != nil && !r.rateLimiter.allow(userID) {
		return nil, ErrRateLimited
	}

	// Need OpenAI client specifically (DALL-E)
	r.mu.RLock()
	client, ok := r.clients["openai"]
	if !ok {
		// Fall back to main client
		client, ok = r.clients[r.mainName]
	}
	r.mu.RUnlock()

	if !ok {
		return nil, ErrNoProvider
	}

	return client.GenerateImage(ctx, prompt, opts...)
}

// Speak converts text to speech using the configured provider
func (r *Router) Speak(ctx context.Context, req *SpeechRequest) (*SpeechResponse, error) {
	r.mu.RLock()
	// Prefer OpenAI for TTS (has the API)
	client, ok := r.clients["openai"]
	if !ok {
		client, ok = r.clients[r.mainName]
	}
	r.mu.RUnlock()
	if !ok {
		return nil, ErrNoProvider
	}
	return client.Speak(ctx, req)
}

// Transcribe converts audio to text using the configured provider
func (r *Router) Transcribe(ctx context.Context, req *TranscribeRequest) (*TranscribeResponse, error) {
	r.mu.RLock()
	client, ok := r.clients["openai"]
	if !ok {
		client, ok = r.clients[r.mainName]
	}
	r.mu.RUnlock()
	if !ok {
		return nil, ErrNoProvider
	}
	return client.Transcribe(ctx, req)
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

// FormatError formats error for user display with sanitization.
// Delegates to allm.FormatError.
func FormatError(err error) string {
	return allm.FormatError(err)
}

// ImageFromBase64 creates an Image from base64-encoded data and mime type.
// Delegates to allm.ImageFromBase64.
func ImageFromBase64(mimeType, base64Data string) (Image, error) {
	return allm.ImageFromBase64(mimeType, base64Data)
}

// ImageFromBytes creates an Image from raw bytes and mime type.
// Delegates to allm.ImageFromBytes.
func ImageFromBytes(mimeType string, data []byte) Image {
	return allm.ImageFromBytes(mimeType, data)
}

// HealthCheck returns health status of all registered providers
func (r *Router) HealthCheck(ctx context.Context) map[string]*allm.HealthStatus {
	r.mu.RLock()
	clients := make(map[string]*allm.Client)
	for k, v := range r.clients {
		clients[k] = v
	}
	r.mu.RUnlock()

	results := make(map[string]*allm.HealthStatus)
	for name, client := range clients {
		results[name] = client.Ping(ctx)
	}
	return results
}

// MainProvider returns the name of the main/active LLM provider.
func (r *Router) MainProvider() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mainName
}

// SetModel sets the model on the main client
func (r *Router) SetModel(model string) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()
	if ok {
		client.SetModel(model)
	}
}

// GetModel returns the active model ID from the main client.
func (r *Router) GetModel() string {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()
	if ok {
		if m := client.Model(); m != "" {
			return m
		}
	}
	return r.mainName
}

// Usage returns cumulative usage stats aggregated from all clients.
func (r *Router) Usage() allm.UsageStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var total allm.UsageStats
	for _, c := range r.clients {
		u := c.Usage()
		total.Requests += u.Requests
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
	}
	return total
}

// CLIProvider returns the underlying ClaudeCLIProvider if the main provider is CLI-based.
func (r *Router) CLIProvider() *provider.ClaudeCLIProvider {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	cli, _ := client.Provider().(*provider.ClaudeCLIProvider)
	return cli
}

// SetResponseFormat sets the response format on the main client
func (r *Router) SetResponseFormat(format *allm.ResponseFormat) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()
	if ok {
		client.SetResponseFormat(format)
	}
}

// SetThinking sets the thinking configuration on the main client
func (r *Router) SetThinking(thinking *allm.ThinkingConfig) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()
	if ok {
		client.SetThinking(thinking)
	}
}
