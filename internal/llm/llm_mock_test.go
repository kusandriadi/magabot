package llm

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

// Additional mock-based tests for high coverage

// --- Router Edge Cases ---

func TestRouterConfigDefaults(t *testing.T) {
	t.Run("ZeroMaxInput", func(t *testing.T) {
		r := NewRouter(&Config{MaxInput: 0})
		if r.maxInput != 10000 {
			t.Errorf("expected main maxInput 10000, got %d", r.maxInput)
		}
	})

	t.Run("ZeroTimeout", func(t *testing.T) {
		r := NewRouter(&Config{Timeout: 0})
		if r.timeout != 60*time.Second {
			t.Errorf("expected main timeout 60s, got %v", r.timeout)
		}
	})

	t.Run("ZeroRateLimit", func(t *testing.T) {
		r := NewRouter(&Config{RateLimit: 0})
		if r.rateLimiter.limit != 10 {
			t.Errorf("expected main rateLimit 10, got %d", r.rateLimiter.limit)
		}
	})

	t.Run("NilLogger", func(t *testing.T) {
		r := NewRouter(&Config{Logger: nil})
		if r.logger == nil {
			t.Error("logger should default to slog.Default()")
		}
	})

	t.Run("CustomConfig", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		r := NewRouter(&Config{
			Main:      "custom",
			SystemPrompt: "Be helpful",
			MaxInput:     5000,
			Timeout:      30 * time.Second,
			RateLimit:    20,
			Logger:       logger,
		})

		if r.mainName != "custom" {
			t.Errorf("expected main 'custom', got %q", r.mainName)
		}
		if r.systemPrompt != "Be helpful" {
			t.Errorf("expected systemPrompt 'Be helpful', got %q", r.systemPrompt)
		}
		if r.maxInput != 5000 {
			t.Errorf("expected maxInput 5000, got %d", r.maxInput)
		}
		if r.timeout != 30*time.Second {
			t.Errorf("expected timeout 30s, got %v", r.timeout)
		}
	})
}

func TestRouterRegisterAutoDefault(t *testing.T) {
	r := NewRouter(&Config{}) // No default set
	mock := &mockProvider{
		name:      "auto",
		available: true,
		response:  &Response{Content: "ok"},
	}
	r.Register(mock)

	// Should auto-select as default
	if r.mainName != "auto" {
		t.Errorf("expected auto-selected main 'auto', got %q", r.mainName)
	}
}

func TestRouterRegisterUnavailable(t *testing.T) {
	r := NewRouter(&Config{})
	mock := &mockProvider{
		name:      "unavailable",
		available: false,
	}
	r.Register(mock)

	// Should not auto-select unavailable provider as default
	if r.mainName == "unavailable" {
		t.Error("should not auto-select unavailable provider")
	}
}

func TestRouterCompleteWithSystemPrompt(t *testing.T) {
	r := NewRouter(&Config{
		Main:      "mock",
		SystemPrompt: "You are a test assistant",
		RateLimit:    100,
		Logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	var capturedReq *Request
	mock := &mockProvider{
		name:      "mock",
		available: true,
		response:  &Response{Content: "ok"},
	}
	r.providers["mock"] = &requestCapture{mock, &capturedReq}

	_, _ = r.Complete(context.Background(), "user1", "hello")

	if capturedReq == nil {
		t.Fatal("request not captured")
	}
	if len(capturedReq.Messages) != 2 {
		t.Errorf("expected 2 messages (system + user), got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("expected system message first, got %q", capturedReq.Messages[0].Role)
	}
}

type requestCapture struct {
	*mockProvider
	captured **Request
}

func (c *requestCapture) Complete(ctx context.Context, req *Request) (*Response, error) {
	*c.captured = req
	return c.mockProvider.Complete(ctx, req)
}

func TestRouterChatRateLimit(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "mock",
		RateLimit: 1,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	mock := &mockProvider{name: "mock", available: true, response: &Response{Content: "ok"}}
	r.Register(mock)

	messages := []Message{{Role: "user", Content: "hi"}}

	// First should succeed
	_, err := r.Chat(context.Background(), "chatuser", messages)
	if err != nil {
		t.Fatalf("first chat should succeed: %v", err)
	}

	// Second should be rate limited
	_, err = r.Chat(context.Background(), "chatuser", messages)
	if err != ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestRouterChatInputTooLong(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "mock",
		MaxInput:  20,
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	mock := &mockProvider{name: "mock", available: true, response: &Response{Content: "ok"}}
	r.Register(mock)

	// Long content
	messages := []Message{{Role: "user", Content: "This message is way too long for the limit"}}

	_, err := r.Chat(context.Background(), "user1", messages)
	if err != ErrInputTooLong {
		t.Errorf("expected ErrInputTooLong, got %v", err)
	}
}

func TestRouterChatInputTooLongWithBlocks(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "mock",
		MaxInput:  50,
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	mock := &mockProvider{name: "mock", available: true, response: &Response{Content: "ok"}}
	r.Register(mock)

	// Message with blocks that exceed limit
	messages := []Message{{
		Role:    "user",
		Content: "short",
		Blocks: []ContentBlock{
			{Type: "text", Text: "some text"},
			{Type: "image", ImageData: "base64datathatisveryverylongandexceedsthelimit"},
		},
	}}

	_, err := r.Chat(context.Background(), "user1", messages)
	if err != ErrInputTooLong {
		t.Errorf("expected ErrInputTooLong for blocks, got %v", err)
	}
}

func TestRouterChatWithSystemPrompt(t *testing.T) {
	r := NewRouter(&Config{
		Main:      "mock",
		SystemPrompt: "Be helpful",
		RateLimit:    100,
		Logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	var capturedReq *Request
	mock := &mockProvider{name: "mock", available: true, response: &Response{Content: "ok"}}
	r.providers["mock"] = &requestCapture{mock, &capturedReq}

	messages := []Message{{Role: "user", Content: "hi"}}
	_, _ = r.Chat(context.Background(), "user1", messages)

	if capturedReq == nil {
		t.Fatal("request not captured")
	}
	if len(capturedReq.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("expected system first, got %q", capturedReq.Messages[0].Role)
	}
}

func TestRouterProviderFails(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "p1",
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	p1 := &mockProvider{name: "p1", available: true, err: errors.New("p1 failed")}
	r.Register(p1)

	_, err := r.Complete(context.Background(), "user1", "hi")
	if err == nil {
		t.Error("expected error when provider fails")
	}
	if !errors.Is(err, ErrProviderFailed) {
		t.Errorf("expected ErrProviderFailed, got %v", err)
	}
}

func TestRouterProviderNotRegistered(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "nonexistent",
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	_, err := r.Complete(context.Background(), "user1", "hi")
	if err == nil {
		t.Error("expected error when provider not registered")
	}
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("expected ErrNoProvider, got %v", err)
	}
}

func TestRouterDefaultNotAvailable(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "unavailable",
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	unavailable := &mockProvider{name: "unavailable", available: false}
	r.Register(unavailable)

	_, err := r.Complete(context.Background(), "user1", "hi")
	if err == nil {
		t.Error("expected error when default not available")
	}
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("expected ErrNoProvider, got %v", err)
	}
}

// --- Rate Limiter Edge Cases ---

func TestRateLimiterPeriodicCleanup(t *testing.T) {
	rl := newRateLimiter(1000)

	// Add many users
	for i := 0; i < 150; i++ {
		rl.allow("user" + string(rune('A'+i%26)))
	}

	// Should trigger cleanup around call 100
	// Should not panic
	if !rl.allow("newuser") {
		t.Error("should allow new user")
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	rl := newRateLimiter(1000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rl.allow("concurrent" + string(rune('A'+n%10)))
		}(i)
	}
	wg.Wait()
}

// --- Error Formatting ---

func TestExtractAPIMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "JSON message field",
			input:    `{"error": {"message":"Rate limit exceeded"}}`,
			expected: "Rate limit exceeded",
		},
		{
			name:     "No message field",
			input:    `{"error": "something"}`,
			expected: "",
		},
		{
			name:     "Plain text",
			input:    "plain error text",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAPIMessage(errors.New(tt.input))
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatErrorWithAPIMessage(t *testing.T) {
	// Error with embedded JSON message
	err := errors.New(`no LLM provider available: anthropic: {"error":{"message":"API key invalid"}}`)
	result := FormatError(err)
	if result == "" {
		t.Error("should return non-empty error")
	}
}

func TestFormatErrorGeneric(t *testing.T) {
	err := errors.New("something went wrong")
	result := FormatError(err)
	if result == "" {
		t.Error("should return non-empty error")
	}
	if result != "❌ Error: something went wrong" {
		t.Errorf("unexpected format: %q", result)
	}
}

// --- Message Tests ---

func TestMessageHasBlocksEmpty(t *testing.T) {
	m := Message{Role: "user", Content: "hello"}
	if m.HasBlocks() {
		t.Error("message without blocks should return false")
	}
}

func TestMessageHasBlocksNil(t *testing.T) {
	m := Message{Role: "user", Content: "hello", Blocks: nil}
	if m.HasBlocks() {
		t.Error("message with nil blocks should return false")
	}
}

func TestMessageHasBlocksWithContent(t *testing.T) {
	m := Message{
		Role: "user",
		Blocks: []ContentBlock{
			{Type: "text", Text: "hello"},
		},
	}
	if !m.HasBlocks() {
		t.Error("message with blocks should return true")
	}
}

// --- DetectProvider Additional Tests ---

func TestDetectProviderCaseInsensitive(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"CLAUDE-3-OPUS", "anthropic"},
		{"GPT-4O-MINI", "openai"},
		{"Gemini-Pro", "gemini"},
		{"LLAMA-70B", "local"},
		{"Mistral-7B", "local"},
		{"MIXTRAL-8X7B", "local"},
		{"PHI-3", "local"},
		{"QWEN-72B", "local"},
		{"CodeLlama-34B", "local"},
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

// --- Stats Tests ---

func TestRouterStatsEmpty(t *testing.T) {
	r := NewRouter(&Config{Main: "test", RateLimit: 10})
	stats := r.Stats()

	if stats["main"] != "test" {
		t.Errorf("expected main 'test', got %v", stats["main"])
	}
	if stats["providers"] != 0 {
		t.Errorf("expected 0 providers, got %v", stats["providers"])
	}
}

func TestRouterStatsWithProviders(t *testing.T) {
	r := NewRouter(&Config{Main: "p1", RateLimit: 10})
	r.Register(&mockProvider{name: "p1", available: true})
	r.Register(&mockProvider{name: "p2", available: false})
	r.Register(&mockProvider{name: "p3", available: true})

	stats := r.Stats()
	if stats["providers"] != 3 {
		t.Errorf("expected 3 providers, got %v", stats["providers"])
	}

	available := stats["available"].([]string)
	if len(available) != 2 {
		t.Errorf("expected 2 available, got %d", len(available))
	}
}

// --- Providers List ---

func TestRouterProvidersEmpty(t *testing.T) {
	r := NewRouter(&Config{RateLimit: 10})
	providers := r.Providers()
	if len(providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(providers))
	}
}

func TestRouterProvidersList(t *testing.T) {
	r := NewRouter(&Config{RateLimit: 10})
	r.Register(&mockProvider{name: "a"})
	r.Register(&mockProvider{name: "b"})
	r.Register(&mockProvider{name: "c"})

	providers := r.Providers()
	if len(providers) != 3 {
		t.Errorf("expected 3 providers, got %d", len(providers))
	}
}

// --- Concurrency Tests ---

func TestRouterConcurrentRequests(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "mock",
		RateLimit: 1000,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
	mock := &mockProvider{name: "mock", available: true, response: &Response{Content: "ok"}}
	r.Register(mock)

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := r.Complete(context.Background(), "user"+string(rune('A'+n%26)), "hello")
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}

func TestRouterConcurrentRegister(t *testing.T) {
	r := NewRouter(&Config{RateLimit: 10})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mock := &mockProvider{name: "p" + string(rune('A'+n)), available: true}
			r.Register(mock)
		}(i)
	}

	wg.Wait()

	// Should have registered all providers
	providers := r.Providers()
	if len(providers) != 20 {
		t.Errorf("expected 20 providers, got %d", len(providers))
	}
}

// --- Timeout Test ---

func TestRouterCompleteTimeout(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "slow",
		Timeout:   50 * time.Millisecond,
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	slow := &slowProvider{delay: 200 * time.Millisecond}
	r.providers["slow"] = slow

	_, err := r.Complete(context.Background(), "user1", "hi")
	if err == nil {
		t.Error("expected timeout error")
	}
}

type slowProvider struct {
	delay time.Duration
}

func (s *slowProvider) Name() string    { return "slow" }
func (s *slowProvider) Available() bool { return true }
func (s *slowProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	select {
	case <-time.After(s.delay):
		return &Response{Content: "ok"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// --- Error Types ---

func TestErrorTypes(t *testing.T) {
	errors := []error{
		ErrNoProvider,
		ErrProviderFailed,
		ErrRateLimited,
		ErrInputTooLong,
		ErrTimeout,
	}

	for _, err := range errors {
		if err.Error() == "" {
			t.Errorf("error %v should have message", err)
		}
	}
}

// --- Provider Tests ---

func TestGeminiAvailable(t *testing.T) {
	// Clear env vars
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

	t.Run("WithAPIKey", func(t *testing.T) {
		p := NewGemini(&GeminiConfig{APIKey: "test-key"})
		if !p.Available() {
			t.Error("should be available with API key")
		}
	})

	t.Run("WithoutAPIKey", func(t *testing.T) {
		p := NewGemini(&GeminiConfig{})
		if p.Available() {
			t.Error("should not be available without API key")
		}
	})

	t.Run("FromGeminiEnv", func(t *testing.T) {
		os.Setenv("GEMINI_API_KEY", "env-key")
		defer os.Unsetenv("GEMINI_API_KEY")

		p := NewGemini(&GeminiConfig{})
		if !p.Available() {
			t.Error("should be available with GEMINI_API_KEY env")
		}
	})

	t.Run("FromGoogleEnv", func(t *testing.T) {
		os.Setenv("GOOGLE_API_KEY", "google-key")
		defer os.Unsetenv("GOOGLE_API_KEY")

		p := NewGemini(&GeminiConfig{})
		if !p.Available() {
			t.Error("should be available with GOOGLE_API_KEY env")
		}
	})
}

func TestGLMAvailable(t *testing.T) {
	// Clear env vars
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

	t.Run("WithAPIKey", func(t *testing.T) {
		p := NewGLM(&GLMConfig{APIKey: "test-key"})
		if !p.Available() {
			t.Error("should be available with API key")
		}
	})

	t.Run("WithoutAPIKey", func(t *testing.T) {
		p := NewGLM(&GLMConfig{})
		if p.Available() {
			t.Error("should not be available without API key")
		}
	})

	t.Run("FromZAIEnv", func(t *testing.T) {
		os.Setenv("ZAI_API_KEY", "zai-key")
		defer os.Unsetenv("ZAI_API_KEY")

		p := NewGLM(&GLMConfig{})
		if !p.Available() {
			t.Error("should be available with ZAI_API_KEY env")
		}
	})

	t.Run("FromGLMEnv", func(t *testing.T) {
		os.Setenv("GLM_API_KEY", "glm-key")
		defer os.Unsetenv("GLM_API_KEY")

		p := NewGLM(&GLMConfig{})
		if !p.Available() {
			t.Error("should be available with GLM_API_KEY env")
		}
	})
}

func TestNewLocalProvider(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		p := NewLocal(&LocalConfig{})
		if p == nil {
			t.Fatal("Local provider should not be nil")
		}
		if p.Name() != "local" {
			t.Errorf("expected name 'local', got %q", p.Name())
		}
	})

	t.Run("CustomConfig", func(t *testing.T) {
		p := NewLocal(&LocalConfig{
			Model:       "custom-model",
			BaseURL:     "http://localhost:8080",
			MaxTokens:   2048,
			Temperature: 0.5,
		})
		if p.config.Model != "custom-model" {
			t.Errorf("expected model 'custom-model', got %q", p.config.Model)
		}
		if p.config.BaseURL != "http://localhost:8080" {
			t.Errorf("expected baseURL 'http://localhost:8080', got %q", p.config.BaseURL)
		}
	})

	t.Run("Available", func(t *testing.T) {
		p := NewLocal(&LocalConfig{BaseURL: "http://localhost:11434"})
		// Available depends on actual server, so just check it doesn't panic
		_ = p.Available()
	})
}

func TestDeepSeekConfig(t *testing.T) {
	orig := os.Getenv("DEEPSEEK_API_KEY")
	os.Unsetenv("DEEPSEEK_API_KEY")
	defer func() {
		if orig != "" {
			os.Setenv("DEEPSEEK_API_KEY", orig)
		}
	}()

	t.Run("DefaultModel", func(t *testing.T) {
		p := NewDeepSeek(&DeepSeekConfig{APIKey: "test"})
		if p.config.Model != "deepseek-chat" {
			t.Errorf("expected main model 'deepseek-chat', got %q", p.config.Model)
		}
	})

	t.Run("CustomModel", func(t *testing.T) {
		p := NewDeepSeek(&DeepSeekConfig{APIKey: "test", Model: "deepseek-coder"})
		if p.config.Model != "deepseek-coder" {
			t.Errorf("expected model 'deepseek-coder', got %q", p.config.Model)
		}
	})

	t.Run("FromEnv", func(t *testing.T) {
		os.Setenv("DEEPSEEK_API_KEY", "env-key")
		defer os.Unsetenv("DEEPSEEK_API_KEY")

		p := NewDeepSeek(&DeepSeekConfig{})
		if !p.Available() {
			t.Error("should be available with DEEPSEEK_API_KEY env")
		}
	})
}

// --- FormatError Additional Cases ---

func TestFormatErrorAllCases(t *testing.T) {
	t.Run("RateLimited", func(t *testing.T) {
		result := FormatError(ErrRateLimited)
		if result == "" {
			t.Error("should return non-empty")
		}
		if !containsSubstring(result, "Too many") {
			t.Error("should contain 'Too many'")
		}
	})

	t.Run("InputTooLong", func(t *testing.T) {
		result := FormatError(ErrInputTooLong)
		if result == "" {
			t.Error("should return non-empty")
		}
		if !containsSubstring(result, "too long") {
			t.Error("should contain 'too long'")
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		result := FormatError(ErrTimeout)
		if result == "" {
			t.Error("should return non-empty")
		}
		if !containsSubstring(result, "timed out") {
			t.Error("should contain 'timed out'")
		}
	})

	t.Run("NoProviderWithMessage", func(t *testing.T) {
		err := errors.New("no LLM provider available: {\"message\":\"API key invalid\"}")
		result := FormatError(err)
		if result == "" {
			t.Error("should return non-empty")
		}
	})

	t.Run("NoProviderPlain", func(t *testing.T) {
		result := FormatError(ErrNoProvider)
		if result == "" {
			t.Error("should return non-empty")
		}
		if !containsSubstring(result, "No AI provider") {
			t.Error("should contain 'No AI provider'")
		}
	})
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Local Provider HTTP Tests ---

func TestLocalAvailableWithMockServer(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models": []}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("ServerAvailable", func(t *testing.T) {
		p := NewLocal(&LocalConfig{
			Enabled: true,
			BaseURL: server.URL,
		})
		if !p.Available() {
			t.Error("should be available when server responds OK")
		}
	})

	t.Run("ServerDisabled", func(t *testing.T) {
		p := NewLocal(&LocalConfig{
			Enabled: false,
			BaseURL: server.URL,
		})
		if p.Available() {
			t.Error("should not be available when disabled")
		}
	})
}

func TestLocalAvailableServerError(t *testing.T) {
	// Server returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewLocal(&LocalConfig{
		Enabled: true,
		BaseURL: server.URL,
	})
	if p.Available() {
		t.Error("should not be available when server returns 500")
	}
}

func TestLocalAvailableServerUnreachable(t *testing.T) {
	p := NewLocal(&LocalConfig{
		Enabled: true,
		BaseURL: "http://127.0.0.1:59999", // Unlikely port
	})
	if p.Available() {
		t.Error("should not be available when server is unreachable")
	}
}

func TestLocalEnvConfig(t *testing.T) {
	origURL := os.Getenv("LOCAL_LLM_URL")
	origModel := os.Getenv("LOCAL_LLM_MODEL")
	origKey := os.Getenv("LOCAL_LLM_API_KEY")
	defer func() {
		if origURL != "" {
			os.Setenv("LOCAL_LLM_URL", origURL)
		} else {
			os.Unsetenv("LOCAL_LLM_URL")
		}
		if origModel != "" {
			os.Setenv("LOCAL_LLM_MODEL", origModel)
		} else {
			os.Unsetenv("LOCAL_LLM_MODEL")
		}
		if origKey != "" {
			os.Setenv("LOCAL_LLM_API_KEY", origKey)
		} else {
			os.Unsetenv("LOCAL_LLM_API_KEY")
		}
	}()

	os.Setenv("LOCAL_LLM_URL", "http://envserver:8080")
	os.Setenv("LOCAL_LLM_MODEL", "envmodel")
	os.Setenv("LOCAL_LLM_API_KEY", "envkey")

	p := NewLocal(&LocalConfig{})
	if p.config.BaseURL != "http://envserver:8080" {
		t.Errorf("expected URL from env, got %q", p.config.BaseURL)
	}
	if p.config.Model != "envmodel" {
		t.Errorf("expected model from env, got %q", p.config.Model)
	}
	if p.config.APIKey != "envkey" {
		t.Errorf("expected key from env, got %q", p.config.APIKey)
	}
}

// --- Additional Edge Cases ---

func TestTryProvidersContextCancelled(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "slow",
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	slow := &slowProvider{delay: 5 * time.Second}
	r.providers["slow"] = slow

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := r.Complete(ctx, "user1", "hi")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestRateLimiterStaleCleanup(t *testing.T) {
	rl := newRateLimiter(1000)

	// Add old entries
	now := time.Now()
	rl.mu.Lock()
	rl.requests["olduser"] = []time.Time{now.Add(-5 * time.Minute)} // Stale
	rl.requests["recentuser"] = []time.Time{now.Add(-30 * time.Second)} // Recent
	rl.callCount = 99 // Next call will trigger cleanup
	rl.mu.Unlock()

	// Trigger cleanup
	rl.allow("triggeruser")

	rl.mu.Lock()
	_, oldExists := rl.requests["olduser"]
	_, recentExists := rl.requests["recentuser"]
	rl.mu.Unlock()

	if oldExists {
		t.Error("old user should be cleaned up")
	}
	if !recentExists {
		t.Error("recent user should not be cleaned up")
	}
}

func TestFormatErrorNoProviderWithAPIMessage(t *testing.T) {
	// Test the case where ErrNoProvider wraps an error with JSON message
	baseErr := errors.New(`{"error":{"message":"Invalid API key provided"}}`)
	err := errors.Join(ErrNoProvider, baseErr)
	result := FormatError(err)

	if result == "" {
		t.Error("should return non-empty error")
	}
}

func TestRouterStatsWithNoAvailable(t *testing.T) {
	r := NewRouter(&Config{RateLimit: 10})
	r.Register(&mockProvider{name: "p1", available: false})
	r.Register(&mockProvider{name: "p2", available: false})

	stats := r.Stats()
	available := stats["available"].([]string)
	if len(available) != 0 {
		t.Errorf("expected 0 available, got %d", len(available))
	}
}

func TestRouterChatSuccess(t *testing.T) {
	r := NewRouter(&Config{
		Main:   "mock",
		RateLimit: 100,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	mock := &mockProvider{
		name:      "mock",
		available: true,
		response: &Response{
			Content:      "Hello!",
			Provider:     "mock",
			Model:        "mock-1",
			InputTokens:  10,
			OutputTokens: 5,
			Latency:      100 * time.Millisecond,
		},
	}
	r.Register(mock)

	messages := []Message{
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello"},
		{Role: "user", Content: "How are you?"},
	}

	resp, err := r.Chat(context.Background(), "user1", messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.Content)
	}
	if resp.Provider != "mock" {
		t.Errorf("expected provider 'mock', got %q", resp.Provider)
	}
}
