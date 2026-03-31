// Package llm provides unified interface for multiple LLM providers using allm-go
package llm

import (
	"context"
	"errors"
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

	responseStyleRules = "\n\nResponse style:\n" +
		"- Write naturally and conversationally, like a knowledgeable friend, not a corporate chatbot.\n" +
		"- Vary your sentence structure. Avoid repetitive patterns or robotic phrasing.\n" +
		"- Be warm but not overly enthusiastic. Skip filler like \"Great question!\" or \"Sure thing!\".\n" +
		"- Use simple, everyday language. Avoid jargon unless the user uses it first."

	defaultFormattingRules = "\n\nFormatting rules (for chat responses only, not for generating file content):\n" +
		"- Do not use markdown tables. Use plain text or lists instead.\n" +
		"- Prefer numbered lists over bullet points. Bullet points are allowed but use them sparingly.\n" +
		"- Emojis are allowed but must be relevant to the text and used neatly, not excessive.\n" +
		"- You may use horizontal lines (————) as section separators."

	telegramFormattingRules = "\n\nFormatting rules (for Telegram chat):\n" +
		"- NEVER use markdown headers (# ## ###). Use bold text (*text*) for section titles instead.\n" +
		"- NEVER use **text** for bold. Telegram uses single asterisks: *text*.\n" +
		"- NEVER use markdown tables. Use numbered lists or bullet points instead.\n" +
		"- Always separate paragraphs and steps with a blank line (two newlines). Never run sentences together without a line break.\n" +
		"- When narrating multiple steps or actions, put each step on its own line with a blank line between them.\n" +
		"- Prefer numbered lists and bullet points for structured information.\n" +
		"- Emojis are encouraged where relevant — use them to make responses more visual and scannable.\n" +
		"- You may use horizontal lines (————) as section separators.\n" +
		"- Keep responses concise and easy to read on mobile screens."

	slackFormattingRules = "\n\nFormatting rules (for Slack chat):\n" +
		"- Use *bold* for emphasis (single asterisks).\n" +
		"- Use _italic_ for subtle emphasis.\n" +
		"- Do not use markdown tables. Use lists instead.\n" +
		"- Prefer numbered lists and bullet points for structured information.\n" +
		"- Emojis are allowed but must be relevant to the text and used neatly.\n" +
		"- You may use horizontal lines (————) as section separators."

	whatsappFormattingRules = "\n\nFormatting rules (for WhatsApp chat):\n" +
		"- Use *bold* for emphasis (single asterisks).\n" +
		"- Use _italic_ for subtle emphasis.\n" +
		"- NEVER use markdown tables. Use numbered lists or bullet points instead.\n" +
		"- Emojis are encouraged where relevant.\n" +
		"- Keep responses concise and easy to read on mobile screens."

	// systemPromptRules is kept for backward compatibility in tests.
	systemPromptRules = responseStyleRules + defaultFormattingRules
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
	clients         map[string]*allm.Client
	mainName        string
	systemPrompt    string
	maxInput        int
	maxContextChars int
	timeout         time.Duration
	rateLimiter     *rateLimiter
	logger          *slog.Logger
	mu              sync.RWMutex
	promptCaching   bool
}

// Config for LLM router
type Config struct {
	Main            string
	SystemPrompt    string
	MaxInput        int
	MaxContextChars int // max total chars sent to LLM; 0 = default 250000
	Timeout         time.Duration
	RateLimit       int // requests per minute per user
	Logger          *slog.Logger
}

// NewRouter creates a new LLM router
func NewRouter(cfg *Config) *Router {
	if cfg.MaxInput == 0 {
		cfg.MaxInput = 10000
	}
	if cfg.MaxContextChars == 0 {
		cfg.MaxContextChars = defaultMaxContextChars
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.RateLimit == 0 {
		cfg.RateLimit = 10
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Router{
		clients:         make(map[string]*allm.Client),
		mainName:        cfg.Main,
		systemPrompt:    cfg.SystemPrompt,
		maxInput:        cfg.MaxInput,
		maxContextChars: cfg.MaxContextChars,
		timeout:         cfg.Timeout,
		rateLimiter:     newRateLimiter(cfg.RateLimit),
		logger:          logger,
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

// QuickChat makes a lightweight LLM call without system prompt or rate limiting.
// Useful for internal tasks like translation or template generation.
func (r *Router) QuickChat(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	messages := []allm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := r.chat(ctx, messages)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
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

// defaultMaxContextChars is the safety limit for total conversation context.
// ~62k tokens — fits comfortably in Claude (200k) and GPT-4o (128k).
const defaultMaxContextChars = 250_000

// PlatformFormattingRules returns formatting rules tailored to the given platform.
func PlatformFormattingRules(platform string) string {
	switch platform {
	case "telegram":
		return telegramFormattingRules
	case "slack":
		return slackFormattingRules
	case "whatsapp":
		return whatsappFormattingRules
	default:
		return defaultFormattingRules
	}
}

// BuildSystemPrompt constructs a full system prompt by combining a base prompt
// with response style rules and platform-specific formatting rules.
func BuildSystemPrompt(basePrompt, platform string) string {
	if basePrompt == "" {
		return ""
	}
	return basePrompt + responseStyleRules + PlatformFormattingRules(platform)
}

// buildMessages converts user messages to allm messages with system prompt.
// If systemPromptOverride is non-empty, it is used as-is instead of the router's
// default system prompt (the caller is responsible for including formatting rules).
func (r *Router) buildMessages(messages []Message, systemPromptOverride string) []allm.Message {
	r.mu.RLock()
	systemPrompt := r.systemPrompt
	promptCaching := r.promptCaching
	r.mu.RUnlock()

	// Use override when provided
	if systemPromptOverride != "" {
		systemPrompt = systemPromptOverride
	} else if systemPrompt != "" {
		// Append default rules only when using the router's own prompt
		systemPrompt += systemPromptRules
	}

	// Trim conversation history to prevent context overflow.
	// Drops oldest messages first, always keeps the last message (current user input).
	origLen := len(messages)
	messages = trimHistory(messages, len(systemPrompt), r.maxContextChars)
	if len(messages) < origLen {
		r.logger.Info("trimmed conversation history",
			"original_messages", origLen,
			"kept_messages", len(messages),
			"dropped", origLen-len(messages),
		)
	}

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

// trimHistory removes oldest messages when total content exceeds maxChars.
// Always preserves the last message (the current user input).
func trimHistory(messages []Message, systemPromptLen, maxChars int) []Message {
	total := systemPromptLen
	for _, m := range messages {
		total += len(m.Content)
	}
	if total <= maxChars {
		return messages
	}

	// Drop oldest messages until under limit; always keep last (current user message)
	for len(messages) > 1 && total > maxChars {
		total -= len(messages[0].Content)
		messages = messages[1:]
	}
	return messages
}

// StreamChat streams a chat response with idle timeout.
// The timeout resets on each received chunk, so long-running responses
// (e.g. extended thinking) won't be killed as long as data keeps flowing.
// An optional systemPromptOverride can be provided; when non-empty it replaces
// the router's default system prompt for this request only.
func (r *Router) StreamChat(ctx context.Context, userID string, messages []Message, systemPromptOverride ...string) (<-chan StreamChunk, error) {
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
	var override string
	if len(systemPromptOverride) > 0 {
		override = systemPromptOverride[0]
	}
	allmMessages := r.buildMessages(sanitized, override)

	// Get raw stream from provider (no hard deadline on context)
	rawCh := client.Stream(ctx, allmMessages)

	// Wrap with idle timeout: cancel only if no chunk arrives within r.timeout
	out := make(chan StreamChunk)
	go func() {
		defer close(out)
		idle := time.NewTimer(r.timeout)
		defer idle.Stop()

		for {
			select {
			case chunk, ok := <-rawCh:
				if !ok {
					return
				}
				// Reset idle timer on each chunk
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(r.timeout)

				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
				if chunk.Done || chunk.Error != nil {
					return
				}
			case <-idle.C:
				out <- StreamChunk{Error: ErrTimeout, Done: true}
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// CountTokens counts tokens in a set of messages
func (r *Router) CountTokens(ctx context.Context, messages []Message) (*TokenCount, error) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()

	if !ok {
		return nil, ErrNoProvider
	}

	allmMessages := r.buildMessages(messages, "")
	return client.CountTokens(ctx, allmMessages)
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

// SetThinking sets the thinking configuration on the main client
func (r *Router) SetThinking(thinking *allm.ThinkingConfig) {
	r.mu.RLock()
	client, ok := r.clients[r.mainName]
	r.mu.RUnlock()
	if ok {
		client.SetThinking(thinking)
	}
}

// Speak converts text to speech using the first registered provider that supports TTS.
// Returns OGG Opus audio bytes. Tries the main client first, then falls back to others.
func (r *Router) Speak(ctx context.Context, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	r.mu.RLock()
	defer r.mu.RUnlock()

	req := &allm.SpeechRequest{
		Input:  text,
		Model:  "tts-1",
		Voice:  "alloy",
		Format: "opus",
	}

	// Try main client first
	if client, ok := r.clients[r.mainName]; ok {
		resp, err := client.Speak(ctx, req)
		if err == nil {
			return resp.Audio, nil
		}
		if !errors.Is(err, allm.ErrNotSupported) && !errors.Is(err, allm.ErrNoProvider) {
			return nil, err
		}
	}

	// Fall back to any other registered client
	for name, client := range r.clients {
		if name == r.mainName {
			continue
		}
		resp, err := client.Speak(ctx, req)
		if err == nil {
			return resp.Audio, nil
		}
	}

	return nil, fmt.Errorf("%w: text-to-speech", allm.ErrNotSupported)
}
