// Package test contains integration tests for router module
package test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/storage"
)

// MockPlatform implements router.Platform for testing
type MockPlatform struct {
	name      string
	handler   router.MessageHandler
	started   bool
	stopped   bool
	messages  []string
	mu        sync.Mutex
}

func NewMockPlatform(name string) *MockPlatform {
	return &MockPlatform{name: name}
}

func (m *MockPlatform) Name() string { return m.name }

func (m *MockPlatform) Start(ctx context.Context) error {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	return nil
}

func (m *MockPlatform) Stop() error {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	return nil
}

func (m *MockPlatform) Send(chatID, message string) error {
	m.mu.Lock()
	m.messages = append(m.messages, message)
	m.mu.Unlock()
	return nil
}

func (m *MockPlatform) SetHandler(h router.MessageHandler) {
	m.handler = h
}

func (m *MockPlatform) SimulateMessage(ctx context.Context, msg *router.Message) (string, error) {
	if m.handler == nil {
		return "", nil
	}
	return m.handler(ctx, msg)
}

func TestRouterIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Setup storage
	store, err := storage.New(filepath.Join(tmpDir, "router.db"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Setup vault
	key := security.GenerateKey()
	vault, err := security.NewVault(key)
	if err != nil {
		t.Fatalf("Failed to create vault: %v", err)
	}

	// Setup config
	configPath := filepath.Join(tmpDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	cfg.Access.GlobalAdmins = []string{"admin1"}
	cfg.Platforms.Telegram = &config.TelegramConfig{
		Enabled:      true,
		AllowedUsers: []string{"user1", "user2"},
		AllowDMs:     true,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Setup authorizer
	auth := security.NewAuthorizer()
	auth.SetAllowedUsers("telegram", []string{"user1", "user2"})

	// Setup rate limiter
	rateLimiter := security.NewRateLimiter(60, 20)

	t.Run("RegisterPlatform", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)
		platform := NewMockPlatform("telegram")

		r.Register(platform)

		platforms := r.Platforms()
		if len(platforms) != 1 {
			t.Errorf("Expected 1 platform, got %d", len(platforms))
		}

		if platforms[0] != "telegram" {
			t.Errorf("Expected 'telegram', got %s", platforms[0])
		}
	})

	t.Run("StartAndStop", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)
		platform := NewMockPlatform("telegram")
		r.Register(platform)

		ctx := context.Background()
		if err := r.Start(ctx); err != nil {
			t.Fatalf("Failed to start router: %v", err)
		}

		platform.mu.Lock()
		started := platform.started
		platform.mu.Unlock()

		if !started {
			t.Error("Platform should be started")
		}

		r.Stop()

		platform.mu.Lock()
		stopped := platform.stopped
		platform.mu.Unlock()

		if !stopped {
			t.Error("Platform should be stopped")
		}
	})

	t.Run("MessageHandling", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)
		platform := NewMockPlatform("telegram")
		r.Register(platform)

		// Set handler
		var handledMessage string
		r.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
			handledMessage = msg.Text
			return "Response: " + msg.Text, nil
		})

		ctx := context.Background()
		if err := r.Start(ctx); err != nil {
			t.Fatalf("Failed to start router: %v", err)
		}

		// Simulate authorized user message
		msg := &router.Message{
			Platform:  "telegram",
			ChatID:    "chat1",
			UserID:    "user1",
			Text:      "Hello bot!",
			Timestamp: time.Now(),
		}

		response, err := platform.SimulateMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Message handling failed: %v", err)
		}

		if handledMessage != "Hello bot!" {
			t.Errorf("Expected 'Hello bot!', got %s", handledMessage)
		}

		if response != "Response: Hello bot!" {
			t.Errorf("Expected 'Response: Hello bot!', got %s", response)
		}

		r.Stop()
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)
		platform := NewMockPlatform("telegram")
		r.Register(platform)

		r.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
			return "Should not reach here", nil
		})

		ctx := context.Background()
		if err := r.Start(ctx); err != nil {
			t.Fatalf("Failed to start router: %v", err)
		}

		// Unauthorized user
		msg := &router.Message{
			Platform:  "telegram",
			ChatID:    "chat1",
			UserID:    "unauthorized_user",
			Text:      "Hello",
			Timestamp: time.Now(),
		}

		_, err := platform.SimulateMessage(ctx, msg)
		if err == nil {
			t.Error("Should return error for unauthorized user")
		}

		r.Stop()
	})

	t.Run("SendMessage", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)
		platform := NewMockPlatform("telegram")
		r.Register(platform)

		err := r.Send("telegram", "chat123", "Hello from router!")
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		platform.mu.Lock()
		messages := platform.messages
		platform.mu.Unlock()

		if len(messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(messages))
		}

		if messages[0] != "Hello from router!" {
			t.Errorf("Expected 'Hello from router!', got %s", messages[0])
		}
	})

	t.Run("SendToUnknownPlatform", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)

		err := r.Send("unknown", "chat123", "Hello")
		if err == nil {
			t.Error("Should return error for unknown platform")
		}
	})

	t.Run("MultiplePlatforms", func(t *testing.T) {
		r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)

		telegram := NewMockPlatform("telegram")
		whatsapp := NewMockPlatform("whatsapp")
		slack := NewMockPlatform("slack")

		r.Register(telegram)
		r.Register(whatsapp)
		r.Register(slack)

		platforms := r.Platforms()
		if len(platforms) != 3 {
			t.Errorf("Expected 3 platforms, got %d", len(platforms))
		}
	})

	t.Run("RateLimiting", func(t *testing.T) {
		// Create router with strict rate limits
		strictRateLimiter := security.NewRateLimiter(2, 1) // 2 msgs/min, 1 cmd/min

		r := router.NewRouter(store, vault, cfg, auth, strictRateLimiter, logger)
		platform := NewMockPlatform("telegram")
		r.Register(platform)

		r.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
			return "OK", nil
		})

		ctx := context.Background()
		if err := r.Start(ctx); err != nil {
			t.Fatalf("Failed to start router: %v", err)
		}

		// Send messages until rate limited
		var rateLimited bool
		for i := 0; i < 5; i++ {
			msg := &router.Message{
				Platform:  "telegram",
				ChatID:    "chat1",
				UserID:    "user1",
				Text:      "Message " + string(rune('0'+i)),
				Timestamp: time.Now(),
			}

			_, err := platform.SimulateMessage(ctx, msg)
			if err == security.ErrRateLimited {
				rateLimited = true
				break
			}
		}

		if !rateLimited {
			t.Error("Should hit rate limit")
		}

		r.Stop()
	})
}

func TestRouterConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := storage.New(filepath.Join(tmpDir, "concurrent.db"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	key := security.GenerateKey()
	vault, err := security.NewVault(key)
	if err != nil {
		t.Fatalf("Failed to create vault: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	cfg.Platforms.Telegram = &config.TelegramConfig{
		Enabled:      true,
		AllowedUsers: []string{},
		AllowDMs:     true,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	auth := security.NewAuthorizer()
	auth.SetAllowedUsers("telegram", []string{}) // Allow all

	rateLimiter := security.NewRateLimiter(1000, 100)

	r := router.NewRouter(store, vault, cfg, auth, rateLimiter, logger)
	platform := NewMockPlatform("telegram")
	r.Register(platform)

	var counter int
	var mu sync.Mutex
	r.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		mu.Lock()
		counter++
		mu.Unlock()
		return "OK", nil
	})

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Failed to start router: %v", err)
	}
	defer r.Stop()

	// Concurrent message handling
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			msg := &router.Message{
				Platform:  "telegram",
				ChatID:    "chat" + string(rune('0'+n%10)),
				UserID:    "user" + string(rune('0'+n%5)),
				Text:      "Concurrent message",
				Timestamp: time.Now(),
			}
			_, _ = platform.SimulateMessage(ctx, msg)
		}(i)
	}

	wg.Wait()

	mu.Lock()
	finalCount := counter
	mu.Unlock()

	// Should have processed most messages (some may be rate limited for same user)
	if finalCount < 10 {
		t.Errorf("Expected at least 10 processed messages, got %d", finalCount)
	}
}
