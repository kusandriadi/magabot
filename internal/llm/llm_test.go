package llm

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// --- Unit tests (no API keys needed) ---

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-sonnet-4-20250514", "anthropic"},
		{"Claude-3-Opus", "anthropic"},
		{"gpt-4o", "openai"},
		{"o1-preview", "openai"},
		{"o3-mini", "openai"},
		{"gemini-2.0-flash", "gemini"},
		{"glm-4.7", "glm"},
		{"deepseek-chat", "deepseek"},
		{"unknown-model", ""},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := DetectProvider(tt.model)
			if result != tt.expected {
				t.Errorf("DetectProvider(%q) = %q, want %q", tt.model, result, tt.expected)
			}
		})
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(3)

	// First 3 should be allowed
	for i := 0; i < 3; i++ {
		if !rl.allow("user1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th should be rejected
	if rl.allow("user1") {
		t.Error("4th request should be rate limited")
	}

	// Different user should still be allowed
	if !rl.allow("user2") {
		t.Error("different user should not be rate limited")
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"rate limited", ErrRateLimited, "Too many requests"},
		{"input too long", ErrInputTooLong, "too long"},
		{"timeout", ErrTimeout, "timed out"},
		{"no provider", ErrNoProvider, "No AI provider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.err)
			if result == "" {
				t.Error("FormatError should return non-empty string")
			}
		})
	}
}

func TestMessageHasBlocks(t *testing.T) {
	plain := Message{Role: "user", Content: "hello"}
	if plain.HasBlocks() {
		t.Error("plain message should not have blocks")
	}

	multi := Message{
		Role: "user",
		Blocks: []ContentBlock{
			{Type: "text", Text: "hello"},
			{Type: "image", MimeType: "image/png", ImageData: "base64data"},
		},
	}
	if !multi.HasBlocks() {
		t.Error("multi-modal message should have blocks")
	}
}

func TestResolveParams(t *testing.T) {
	// Provider defaults only
	req := &Request{}
	p := resolveParams(4096, 0.7, req)
	if p.maxTokens != 4096 {
		t.Errorf("expected maxTokens=4096, got %d", p.maxTokens)
	}
	if p.temperature != 0.7 {
		t.Errorf("expected temperature=0.7, got %f", p.temperature)
	}

	// Request overrides
	req = &Request{MaxTokens: 2048, Temperature: 0.3}
	p = resolveParams(4096, 0.7, req)
	if p.maxTokens != 2048 {
		t.Errorf("expected maxTokens=2048, got %d", p.maxTokens)
	}
	if p.temperature != 0.3 {
		t.Errorf("expected temperature=0.3, got %f", p.temperature)
	}
}

// --- Provider constructor tests ---

func TestNewAnthropicDefaults(t *testing.T) {
	// Clear env to test defaults
	orig := os.Getenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("ANTHROPIC_API_KEY", orig)
		}
	}()

	p := NewAnthropic(&AnthropicConfig{APIKey: "sk-ant-api-test"})
	if p.Name() != "anthropic" {
		t.Errorf("expected name 'anthropic', got %q", p.Name())
	}
	if !p.Available() {
		t.Error("should be available with API key")
	}
	if p.model != "claude-sonnet-4-20250514" {
		t.Errorf("unexpected default model: %s", p.model)
	}
	if p.maxTokens != 4096 {
		t.Errorf("unexpected default maxTokens: %d", p.maxTokens)
	}
	if p.useOAuth {
		t.Error("sk-ant-api key should not trigger OAuth")
	}
}

func TestNewAnthropicOAuthDetection(t *testing.T) {
	orig := os.Getenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("ANTHROPIC_API_KEY", orig)
		}
	}()

	// Non-standard key triggers OAuth
	p := NewAnthropic(&AnthropicConfig{APIKey: "eyJhbGciOi..."})
	if !p.useOAuth {
		t.Error("non-sk-ant-api key should trigger OAuth")
	}
}

func TestNewOpenAIDefaults(t *testing.T) {
	orig := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("OPENAI_API_KEY", orig)
		}
	}()

	p := NewOpenAI(&OpenAIConfig{APIKey: "sk-test"})
	if p.Name() != "openai" {
		t.Errorf("expected name 'openai', got %q", p.Name())
	}
	if !p.Available() {
		t.Error("should be available with API key")
	}
	if p.model != "gpt-4o" {
		t.Errorf("unexpected default model: %s", p.model)
	}
}

func TestNewOpenAIUnavailable(t *testing.T) {
	orig := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("OPENAI_API_KEY", orig)
		}
	}()

	p := NewOpenAI(&OpenAIConfig{})
	if p.Available() {
		t.Error("should not be available without API key")
	}
}

func TestNewDeepSeekDefaults(t *testing.T) {
	orig := os.Getenv("DEEPSEEK_API_KEY")
	os.Unsetenv("DEEPSEEK_API_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("DEEPSEEK_API_KEY", orig)
		}
	}()

	p := NewDeepSeek(&DeepSeekConfig{APIKey: "sk-test"})
	if p.Name() != "deepseek" {
		t.Errorf("expected name 'deepseek', got %q", p.Name())
	}
	if p.config.Model != "deepseek-chat" {
		t.Errorf("unexpected default model: %s", p.config.Model)
	}
	if p.config.Temperature != 0 {
		t.Errorf("expected default temperature=0 (let API decide), got %f", p.config.Temperature)
	}
	models := p.Models()
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}
}

func TestNewGeminiDefaults(t *testing.T) {
	origGemini := os.Getenv("GEMINI_API_KEY")
	origGoogle := os.Getenv("GOOGLE_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	defer func() {
		if origGemini != "" {
			os.Setenv("GEMINI_API_KEY", origGemini)
		}
		if origGoogle != "" {
			os.Setenv("GOOGLE_API_KEY", origGoogle)
		}
	}()

	p := NewGemini(&GeminiConfig{APIKey: "test-key"})
	if p.Name() != "gemini" {
		t.Errorf("expected name 'gemini', got %q", p.Name())
	}
	if p.model != "gemini-2.0-flash" {
		t.Errorf("unexpected default model: %s", p.model)
	}
}

func TestNewGLMDefaults(t *testing.T) {
	origZAI := os.Getenv("ZAI_API_KEY")
	origGLM := os.Getenv("GLM_API_KEY")
	os.Unsetenv("ZAI_API_KEY")
	os.Unsetenv("GLM_API_KEY")
	defer func() {
		if origZAI != "" {
			os.Setenv("ZAI_API_KEY", origZAI)
		}
		if origGLM != "" {
			os.Setenv("GLM_API_KEY", origGLM)
		}
	}()

	p := NewGLM(&GLMConfig{APIKey: "test-key"})
	if p.Name() != "glm" {
		t.Errorf("expected name 'glm', got %q", p.Name())
	}
	if p.model != "glm-4.7" {
		t.Errorf("unexpected default model: %s", p.model)
	}
	if p.baseURL != "https://api.z.ai/api/paas/v4" {
		t.Errorf("unexpected default base URL: %s", p.baseURL)
	}
}

func TestNewGLMCustomBaseURL(t *testing.T) {
	p := NewGLM(&GLMConfig{APIKey: "test-key", BaseURL: "https://custom.endpoint/v4"})
	if p.baseURL != "https://custom.endpoint/v4" {
		t.Errorf("custom base URL not respected: %s", p.baseURL)
	}
}

// --- Router tests ---

type mockProvider struct {
	name      string
	available bool
	response  *Response
	err       error
}

func (m *mockProvider) Name() string                                          { return m.name }
func (m *mockProvider) Available() bool                                       { return m.available }
func (m *mockProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	return m.response, m.err
}

func newTestRouter() *Router {
	return NewRouter(&Config{
		Default:   "mock",
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

func TestRouterRegisterAndComplete(t *testing.T) {
	r := newTestRouter()
	mock := &mockProvider{
		name:      "mock",
		available: true,
		response:  &Response{Content: "hello", Provider: "mock", Model: "mock-1"},
	}
	r.Register(mock)

	resp, err := r.Complete(context.Background(), "user1", "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got %q", resp.Content)
	}
}

func TestRouterFallback(t *testing.T) {
	r := NewRouter(&Config{
		Default:       "primary",
		FallbackChain: []string{"fallback"},
		RateLimit:     100,
		Logger:        slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	primary := &mockProvider{name: "primary", available: true, err: ErrProviderFailed}
	fallback := &mockProvider{
		name:      "fallback",
		available: true,
		response:  &Response{Content: "from fallback", Provider: "fallback"},
	}
	r.Register(primary)
	r.Register(fallback)

	resp, err := r.Complete(context.Background(), "user1", "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "from fallback" {
		t.Errorf("expected fallback response, got %q", resp.Content)
	}
}

func TestRouterRateLimit(t *testing.T) {
	r := NewRouter(&Config{
		Default:   "mock",
		RateLimit: 1,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	mock := &mockProvider{
		name:      "mock",
		available: true,
		response:  &Response{Content: "ok"},
	}
	r.Register(mock)

	// First should succeed
	_, err := r.Complete(context.Background(), "user1", "hi")
	if err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}

	// Second should be rate limited
	_, err = r.Complete(context.Background(), "user1", "hi again")
	if err != ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestRouterInputTooLong(t *testing.T) {
	r := NewRouter(&Config{
		Default:   "mock",
		MaxInput:  10,
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	mock := &mockProvider{
		name:      "mock",
		available: true,
		response:  &Response{Content: "ok"},
	}
	r.Register(mock)

	_, err := r.Complete(context.Background(), "user1", "this message is way too long for the limit")
	if err != ErrInputTooLong {
		t.Errorf("expected ErrInputTooLong, got %v", err)
	}
}

func TestRouterSystemPrompt(t *testing.T) {
	r := newTestRouter()

	var capturedReq *Request
	mock := &mockProvider{
		name:      "mock",
		available: true,
		response:  &Response{Content: "ok"},
	}
	// Use a wrapper to capture the request
	wrapper := &captureProvider{Provider: mock, captured: &capturedReq}
	r.Register(wrapper)

	r.SetSystemPrompt("You are helpful.")
	_, _ = r.Complete(context.Background(), "user1", "hi")

	if capturedReq == nil {
		t.Fatal("request was not captured")
	}
	if len(capturedReq.Messages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("first message should be system, got %q", capturedReq.Messages[0].Role)
	}
	if capturedReq.Messages[0].Content != "You are helpful." {
		t.Errorf("system prompt mismatch: %q", capturedReq.Messages[0].Content)
	}
}

type captureProvider struct {
	Provider
	captured **Request
}

func (c *captureProvider) Name() string    { return "mock" }
func (c *captureProvider) Available() bool { return true }
func (c *captureProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	*c.captured = req
	return &Response{Content: "ok"}, nil
}

func TestRouterChat(t *testing.T) {
	r := newTestRouter()
	mock := &mockProvider{
		name:      "mock",
		available: true,
		response:  &Response{Content: "response"},
	}
	r.Register(mock)

	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "how are you"},
	}

	resp, err := r.Chat(context.Background(), "user1", messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "response" {
		t.Errorf("expected 'response', got %q", resp.Content)
	}
}

func TestRouterNoProvider(t *testing.T) {
	r := newTestRouter()
	_, err := r.Complete(context.Background(), "user1", "hi")
	if err == nil {
		t.Fatal("expected error with no providers")
	}
}

func TestRouterProviders(t *testing.T) {
	r := newTestRouter()
	mock1 := &mockProvider{name: "a", available: true, response: &Response{}}
	mock2 := &mockProvider{name: "b", available: false, response: &Response{}}
	r.Register(mock1)
	r.Register(mock2)

	providers := r.Providers()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}

	stats := r.Stats()
	available := stats["available"].([]string)
	if len(available) != 1 {
		t.Errorf("expected 1 available, got %d", len(available))
	}
}

// --- Model listing tests ---

func TestListAnthropicModels(t *testing.T) {
	models := listAnthropicModels()
	if len(models) == 0 {
		t.Fatal("expected at least one Anthropic model")
	}
	for _, m := range models {
		if m.Provider != "anthropic" {
			t.Errorf("model %s has wrong provider: %s", m.ID, m.Provider)
		}
		if m.ID == "" || m.Name == "" {
			t.Errorf("model has empty ID or Name: %+v", m)
		}
	}
}

func TestListGLMModels(t *testing.T) {
	models, err := listGLMModels(context.Background(), &GLM{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one GLM model")
	}
	for _, m := range models {
		if m.Provider != "glm" {
			t.Errorf("model %s has wrong provider: %s", m.ID, m.Provider)
		}
	}
}

func TestListDeepSeekModels(t *testing.T) {
	d := NewDeepSeek(&DeepSeekConfig{APIKey: "test"})
	models := listDeepSeekModels(d)
	if len(models) != 3 {
		t.Errorf("expected 3 DeepSeek models, got %d", len(models))
	}
}

func TestFormatModelList(t *testing.T) {
	models := map[string][]ModelInfo{
		"test": {
			{ID: "model-1", Name: "Model 1", Provider: "test", Description: "A test model"},
		},
	}
	result := FormatModelList(models)
	if result == "" {
		t.Error("FormatModelList should return non-empty string")
	}
}

// --- Integration tests (skipped when API keys not set) ---

func skipIfNoKey(t *testing.T, envVars ...string) {
	t.Helper()
	for _, env := range envVars {
		if os.Getenv(env) != "" {
			return
		}
	}
	t.Skipf("skipping: none of %v set", envVars)
}

func TestIntegrationAnthropic(t *testing.T) {
	skipIfNoKey(t, "ANTHROPIC_API_KEY")

	p := NewAnthropic(&AnthropicConfig{})
	if !p.Available() {
		t.Fatal("Anthropic should be available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{{Role: "user", Content: "Reply with exactly: PONG"}},
	})
	if err != nil {
		t.Fatalf("Anthropic Complete failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	if resp.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", resp.Provider)
	}
	if resp.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
	if resp.Latency == 0 {
		t.Error("expected non-zero latency")
	}
	t.Logf("Anthropic response: %q (in=%d out=%d latency=%s)", resp.Content, resp.InputTokens, resp.OutputTokens, resp.Latency)
}

func TestIntegrationAnthropicSystemPrompt(t *testing.T) {
	skipIfNoKey(t, "ANTHROPIC_API_KEY")

	p := NewAnthropic(&AnthropicConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{
			{Role: "system", Content: "You always reply with exactly one word: PONG"},
			{Role: "user", Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("Anthropic system prompt test failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("Anthropic system prompt response: %q", resp.Content)
}

func TestIntegrationOpenAI(t *testing.T) {
	skipIfNoKey(t, "OPENAI_API_KEY")

	p := NewOpenAI(&OpenAIConfig{})
	if !p.Available() {
		t.Fatal("OpenAI should be available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{{Role: "user", Content: "Reply with exactly: PONG"}},
	})
	if err != nil {
		t.Fatalf("OpenAI Complete failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	if resp.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", resp.Provider)
	}
	t.Logf("OpenAI response: %q (in=%d out=%d latency=%s)", resp.Content, resp.InputTokens, resp.OutputTokens, resp.Latency)
}

func TestIntegrationDeepSeek(t *testing.T) {
	skipIfNoKey(t, "DEEPSEEK_API_KEY")

	p := NewDeepSeek(&DeepSeekConfig{})
	if !p.Available() {
		t.Fatal("DeepSeek should be available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{{Role: "user", Content: "Reply with exactly: PONG"}},
	})
	if err != nil {
		t.Fatalf("DeepSeek Complete failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	if resp.Provider != "deepseek" {
		t.Errorf("expected provider 'deepseek', got %q", resp.Provider)
	}
	t.Logf("DeepSeek response: %q (in=%d out=%d latency=%s)", resp.Content, resp.InputTokens, resp.OutputTokens, resp.Latency)
}

func TestIntegrationGemini(t *testing.T) {
	skipIfNoKey(t, "GEMINI_API_KEY", "GOOGLE_API_KEY")

	p := NewGemini(&GeminiConfig{})
	if !p.Available() {
		t.Fatal("Gemini should be available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{{Role: "user", Content: "Reply with exactly: PONG"}},
	})
	if err != nil {
		t.Fatalf("Gemini Complete failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	if resp.Provider != "gemini" {
		t.Errorf("expected provider 'gemini', got %q", resp.Provider)
	}
	t.Logf("Gemini response: %q (in=%d out=%d latency=%s)", resp.Content, resp.InputTokens, resp.OutputTokens, resp.Latency)
}

func TestIntegrationGLM(t *testing.T) {
	skipIfNoKey(t, "ZAI_API_KEY", "GLM_API_KEY")

	p := NewGLM(&GLMConfig{})
	if !p.Available() {
		t.Fatal("GLM should be available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{{Role: "user", Content: "Reply with exactly: PONG"}},
	})
	if err != nil {
		t.Fatalf("GLM Complete failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	if resp.Provider != "glm" {
		t.Errorf("expected provider 'glm', got %q", resp.Provider)
	}
	t.Logf("GLM response: %q (in=%d out=%d latency=%s)", resp.Content, resp.InputTokens, resp.OutputTokens, resp.Latency)
}

func TestIntegrationMultiTurn(t *testing.T) {
	skipIfNoKey(t, "ANTHROPIC_API_KEY")

	p := NewAnthropic(&AnthropicConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Complete(ctx, &Request{
		Messages: []Message{
			{Role: "user", Content: "My name is TestBot."},
			{Role: "assistant", Content: "Hello TestBot!"},
			{Role: "user", Content: "What is my name? Reply with just the name."},
		},
	})
	if err != nil {
		t.Fatalf("multi-turn test failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("Multi-turn response: %q", resp.Content)
}

func TestIntegrationOpenAIModelList(t *testing.T) {
	skipIfNoKey(t, "OPENAI_API_KEY")

	p := NewOpenAI(&OpenAIConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, err := listOpenAIModels(ctx, p)
	if err != nil {
		t.Fatalf("listOpenAIModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Error("expected at least one OpenAI model")
	}
	t.Logf("Found %d OpenAI chat models", len(models))
}

func TestIntegrationGeminiModelList(t *testing.T) {
	skipIfNoKey(t, "GEMINI_API_KEY", "GOOGLE_API_KEY")

	p := NewGemini(&GeminiConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, err := listGeminiModels(ctx, p)
	if err != nil {
		t.Fatalf("listGeminiModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Error("expected at least one Gemini model")
	}
	t.Logf("Found %d Gemini models", len(models))
}
