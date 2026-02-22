package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kusandriadi/allm-go"
	"github.com/kusandriadi/allm-go/allmtest"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-3-opus", "anthropic"},
		{"gpt-4", "openai"},
		{"gpt-3.5-turbo", "openai"},
		{"o1-preview", "openai"},
		{"gemini-pro", "gemini"},
		{"glm-4", "glm"},
		{"deepseek-chat", "deepseek"},
		{"llama3", "local"},
		{"mistral", "local"},
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
		{"generic error", errors.New("something"), "error occurred"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.err)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("FormatError(%v) = %q, want to contain %q", tt.err, result, tt.contains)
			}
		})
	}
}

func TestImageFromBytes(t *testing.T) {
	data := []byte("test image data")
	img := ImageFromBytes("image/jpeg", data)

	if img.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q, want %q", img.MimeType, "image/jpeg")
	}
	if string(img.Data) != string(data) {
		t.Errorf("Data mismatch")
	}
}

func TestRouter_Complete(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{
			Content:  "Hello!",
			Provider: "test",
			Model:    "test-model",
		}),
	)

	router := NewRouter(&Config{
		Main:     "test",
		MaxInput: 1000,
	})
	router.Register("test", allm.New(mock))

	ctx := context.Background()
	resp, err := router.Complete(ctx, "user1", "Hi there")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if resp.Content != "Hello!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello!")
	}

	// Verify request was captured
	if mock.CallCount() != 1 {
		t.Errorf("CallCount = %d, want 1", mock.CallCount())
	}
}

func TestRouter_Chat(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "Response"}),
	)

	router := NewRouter(&Config{Main: "test"})
	router.Register("test", allm.New(mock))

	messages := []Message{
		{Role: "user", Content: "First message"},
		{Role: "assistant", Content: "First response"},
		{Role: "user", Content: "Second message"},
	}

	ctx := context.Background()
	resp, err := router.Chat(ctx, "user1", messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Content != "Response" {
		t.Errorf("Content = %q, want %q", resp.Content, "Response")
	}
}

func TestRouter_RateLimit(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{
		Main:      "test",
		RateLimit: 2, // 2 requests per minute
	})
	router.Register("test", allm.New(mock))

	ctx := context.Background()
	userID := "user1"

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		_, err := router.Complete(ctx, userID, "test")
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
	}

	// Third request should be rate limited
	_, err := router.Complete(ctx, userID, "test")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("Expected ErrRateLimited, got %v", err)
	}
}

func TestRouter_InputTooLong(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{
		Main:     "test",
		MaxInput: 10,
	})
	router.Register("test", allm.New(mock))

	ctx := context.Background()
	_, err := router.Complete(ctx, "user1", "This is a very long message that exceeds the limit")
	if !errors.Is(err, ErrInputTooLong) {
		t.Errorf("Expected ErrInputTooLong, got %v", err)
	}
}

func TestRouter_SystemPrompt(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{
		Main:         "test",
		SystemPrompt: "You are a helpful assistant.",
	})
	router.Register("test", allm.New(mock))

	ctx := context.Background()
	_, err := router.Complete(ctx, "user1", "Hello")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Verify system prompt was added
	req := mock.LastRequest()
	if req == nil {
		t.Fatal("No request captured")
	}

	if len(req.Messages) != 2 {
		t.Fatalf("Expected 2 messages (system + user), got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" {
		t.Errorf("First message role = %q, want %q", req.Messages[0].Role, "system")
	}
	if req.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("System prompt = %q, want %q", req.Messages[0].Content, "You are a helpful assistant.")
	}
}

func TestRouter_NoProvider(t *testing.T) {
	router := NewRouter(&Config{Main: "missing"})

	ctx := context.Background()
	_, err := router.Complete(ctx, "user1", "Hello")
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("Expected ErrNoProvider, got %v", err)
	}
}

func TestRouter_ProviderFails(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithError(errors.New("API error")),
	)

	router := NewRouter(&Config{Main: "test"})
	router.Register("test", allm.New(mock))

	ctx := context.Background()
	_, err := router.Complete(ctx, "user1", "Hello")
	if !errors.Is(err, ErrProviderFailed) {
		t.Errorf("Expected ErrProviderFailed, got %v", err)
	}
}

func TestRouter_Stats(t *testing.T) {
	mock1 := allmtest.NewMockProvider("provider1",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)
	mock2 := allmtest.NewMockProvider("provider2",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{Main: "provider1"})
	router.Register("provider1", allm.New(mock1))
	router.Register("provider2", allm.New(mock2))

	stats := router.Stats()

	if stats["main"] != "provider1" {
		t.Errorf("main = %v, want %q", stats["main"], "provider1")
	}
	if stats["providers"] != 2 {
		t.Errorf("providers = %v, want 2", stats["providers"])
	}

	providers := router.Providers()
	if len(providers) != 2 {
		t.Errorf("Providers count = %d, want 2", len(providers))
	}
}

func TestRouter_ChatWithImages(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "I see the image"}),
	)

	router := NewRouter(&Config{Main: "test", MaxInput: 10000})
	router.Register("test", allm.New(mock))

	messages := []Message{
		{
			Role:    "user",
			Content: "What's in this image?",
			Images: []Image{
				{MimeType: "image/jpeg", Data: []byte("fake image data")},
			},
		},
	}

	ctx := context.Background()
	resp, err := router.Chat(ctx, "user1", messages)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Content != "I see the image" {
		t.Errorf("Content = %q, want %q", resp.Content, "I see the image")
	}

	// Verify images were passed through
	req := mock.LastRequest()
	if req == nil {
		t.Fatal("No request captured")
	}

	// Should have system prompt + user message = 2 messages (if system prompt empty, just 1)
	// Since we didn't set system prompt, should be 1 message
	if len(req.Messages) == 0 {
		t.Fatal("No messages in request")
	}

	lastMsg := req.Messages[len(req.Messages)-1]
	if len(lastMsg.Images) != 1 {
		t.Errorf("Images count = %d, want 1", len(lastMsg.Images))
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := newRateLimiter(5)

	// Add old requests for multiple users (simulate by adding past timestamps)
	rl.mu.Lock()
	for i := 0; i < 10; i++ {
		userID := string(rune('A' + i))
		// Add timestamps from 2 minutes ago (outside the 1-minute window)
		oldTime := time.Now().Add(-2 * time.Minute)
		rl.requests[userID] = []time.Time{oldTime, oldTime, oldTime}
	}
	rl.mu.Unlock()

	stats := rl.Stats()
	if stats["tracked_users"].(int) != 10 {
		t.Errorf("Expected 10 users tracked, got %d", stats["tracked_users"])
	}

	// Trigger cleanup by making requests
	for i := 0; i < 200; i++ {
		rl.allow("cleanup-trigger")
	}

	// Old users should be cleaned up
	stats = rl.Stats()
	tracked := stats["tracked_users"].(int)
	if tracked > 2 { // Only cleanup-trigger and maybe one other should remain
		t.Errorf("After cleanup: %d users tracked (expected <= 2)", tracked)
	}
}

func TestRateLimiter_DifferentUsers(t *testing.T) {
	rl := newRateLimiter(2)

	// User1 uses their quota
	if !rl.allow("user1") {
		t.Fatal("First request for user1 should be allowed")
	}
	if !rl.allow("user1") {
		t.Fatal("Second request for user1 should be allowed")
	}
	if rl.allow("user1") {
		t.Fatal("Third request for user1 should be blocked")
	}

	// User2 should have independent quota
	if !rl.allow("user2") {
		t.Fatal("First request for user2 should be allowed")
	}
	if !rl.allow("user2") {
		t.Fatal("Second request for user2 should be allowed")
	}
	if rl.allow("user2") {
		t.Fatal("Third request for user2 should be blocked")
	}
}

func TestRouter_Concurrent(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{Main: "test", RateLimit: 100})
	router.Register("test", allm.New(mock))

	ctx := context.Background()
	done := make(chan error, 10)

	// Run 10 concurrent requests
	for i := 0; i < 10; i++ {
		go func(id int) {
			userID := string(rune('A' + id))
			_, err := router.Complete(ctx, userID, "test")
			done <- err
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent request %d failed: %v", i, err)
		}
	}
}

func TestFormatModelList(t *testing.T) {
	tests := []struct {
		name   string
		models map[string][]ModelInfo
		want   []string // Strings that should appear in output
	}{
		{
			name:   "empty",
			models: map[string][]ModelInfo{},
			want:   []string{},
		},
		{
			name: "single provider few models",
			models: map[string][]ModelInfo{
				"anthropic": {
					{ID: "claude-3-opus", Provider: "anthropic"},
					{ID: "claude-3-sonnet", Provider: "anthropic"},
				},
			},
			want: []string{"ANTHROPIC", "claude-3-opus", "claude-3-sonnet", "2 models"},
		},
		{
			name: "multiple providers",
			models: map[string][]ModelInfo{
				"anthropic": {{ID: "claude-3-opus", Provider: "anthropic"}},
				"openai":    {{ID: "gpt-4", Provider: "openai"}},
			},
			want: []string{"ANTHROPIC", "OPENAI", "claude-3-opus", "gpt-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatModelList(tt.models)
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("FormatModelList() result should contain %q\nGot: %s", want, result)
				}
			}
		})
	}
}

func TestImageFromBase64(t *testing.T) {
	tests := []struct {
		name      string
		mimeType  string
		base64    string
		wantError bool
	}{
		{
			name:      "valid jpeg",
			mimeType:  "image/jpeg",
			base64:    "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			wantError: false,
		},
		{
			name:      "invalid base64",
			mimeType:  "image/png",
			base64:    "not-valid-base64!!!",
			wantError: true,
		},
		{
			name:      "empty data",
			mimeType:  "image/jpeg",
			base64:    "",
			wantError: false,
		},
		{
			name:      "unsupported mime type",
			mimeType:  "text/html",
			base64:    "SGVsbG8=",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := ImageFromBase64(tt.mimeType, tt.base64)
			if tt.wantError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if img.MimeType != tt.mimeType {
				t.Errorf("MimeType = %q, want %q", img.MimeType, tt.mimeType)
			}
		})
	}
}

func TestRouter_SetSystemPrompt(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{Main: "test"})
	router.Register("test", allm.New(mock))

	// Set system prompt
	router.SetSystemPrompt("You are a helpful bot.")

	ctx := context.Background()
	_, err := router.Complete(ctx, "user1", "Hello")
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	req := mock.LastRequest()
	if req == nil {
		t.Fatal("No request captured")
	}

	// Should have system + user message
	if len(req.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" {
		t.Errorf("First message role = %q, want system", req.Messages[0].Role)
	}
	if req.Messages[0].Content != "You are a helpful bot." {
		t.Errorf("System prompt = %q, want %q", req.Messages[0].Content, "You are a helpful bot.")
	}
}

func TestRouter_ImageTooLarge(t *testing.T) {
	mock := allmtest.NewMockProvider("test",
		allmtest.WithResponse(&allm.Response{Content: "OK"}),
	)

	router := NewRouter(&Config{Main: "test", MaxInput: 10000})
	router.Register("test", allm.New(mock))

	// Create a very large image (> 10MB)
	largeData := make([]byte, 11*1024*1024)

	messages := []Message{
		{
			Role:    "user",
			Content: "test",
			Images: []Image{
				{MimeType: "image/jpeg", Data: largeData},
			},
		},
	}

	ctx := context.Background()
	_, err := router.Chat(ctx, "user1", messages)
	if err == nil {
		t.Fatal("Expected error for too-large image")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("Error should mention 'too large', got: %v", err)
	}
}

func TestRateLimiter_MaxUsers(t *testing.T) {
	rl := newRateLimiter(10)
	rl.maxUsers = 5 // Set low limit for testing

	// Fill up to max
	for i := 0; i < 5; i++ {
		userID := string(rune('A' + i))
		if !rl.allow(userID) {
			t.Fatalf("Request for user %s should be allowed (slot %d)", userID, i)
		}
	}

	// Next new user should be rejected due to capacity
	if rl.allow("new-user") {
		t.Fatal("Expected rate limit for new user when at capacity")
	}
}
