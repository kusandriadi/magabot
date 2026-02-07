package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/backup"
	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/llm"
	"github.com/kusa/magabot/internal/platform/slack"
	"github.com/kusa/magabot/internal/platform/telegram"
	"github.com/kusa/magabot/internal/platform/webhook"
	"github.com/kusa/magabot/internal/platform/whatsapp"
	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/session"
	"github.com/kusa/magabot/internal/storage"
	"github.com/kusa/magabot/internal/version"
)

// runDaemon runs the main bot daemon
func runDaemon() {
	// Load config
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	logLevel := slog.LevelInfo
	switch cfg.Logging.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	logOpts := &slog.HandlerOptions{Level: logLevel}
	var logHandler slog.Handler

	if cfg.Logging.File != "" {
		logFileHandle, err := os.OpenFile(cfg.Logging.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open log file: %v\n", err)
			os.Exit(1)
		}
		defer logFileHandle.Close()
		logHandler = slog.NewJSONHandler(logFileHandle, logOpts)
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, logOpts)
	}
	logger := slog.New(logHandler)

	logger.Info("magabot starting", "version", version.Short())

	// Initialize vault
	vault, err := security.NewVault(cfg.Security.EncryptionKey)
	if err != nil {
		logger.Error("init vault failed", "error", err)
		os.Exit(1)
	}

	// Initialize storage
	store, err := storage.New(cfg.Storage.Database)
	if err != nil {
		logger.Error("init storage failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize backup manager
	backupMgr := backup.New(cfg.Storage.Backup.Path, cfg.Storage.Backup.KeepCount)

	// Initialize security components
	authorizer := security.NewAuthorizer()
	for platform, users := range cfg.Security.AllowedUsers {
		authorizer.SetAllowedUsers(platform, users)
	}

	rateLimiter := security.NewRateLimiter(
		cfg.Security.RateLimit.MessagesPerMinute,
		cfg.Security.RateLimit.CommandsPerMinute,
	)

	// Initialize LLM router
	llmRouter := llm.NewRouter(&llm.Config{
		Default:       cfg.LLM.Default,
		FallbackChain: cfg.LLM.FallbackChain,
		SystemPrompt:  cfg.LLM.SystemPrompt,
		MaxInput:      cfg.LLM.MaxInputLength,
		Timeout:       time.Duration(cfg.LLM.Timeout) * time.Second,
		RateLimit:     cfg.LLM.RateLimit,
		Logger:        logger.With("component", "llm"),
	})

	// Register LLM providers
	if cfg.LLM.Anthropic.Enabled {
		llmRouter.Register(llm.NewAnthropic(&llm.AnthropicConfig{
			APIKey:      cfg.LLM.Anthropic.APIKey,
			Model:       cfg.LLM.Anthropic.Model,
			MaxTokens:   cfg.LLM.Anthropic.MaxTokens,
			Temperature: cfg.LLM.Anthropic.Temperature,
			BaseURL:     cfg.LLM.Anthropic.BaseURL,
		}))
	}

	if cfg.LLM.OpenAI.Enabled {
		llmRouter.Register(llm.NewOpenAI(&llm.OpenAIConfig{
			APIKey:      cfg.LLM.OpenAI.APIKey,
			Model:       cfg.LLM.OpenAI.Model,
			MaxTokens:   cfg.LLM.OpenAI.MaxTokens,
			Temperature: cfg.LLM.OpenAI.Temperature,
			BaseURL:     cfg.LLM.OpenAI.BaseURL,
		}))
	}

	if cfg.LLM.Gemini.Enabled {
		llmRouter.Register(llm.NewGemini(&llm.GeminiConfig{
			APIKey:      cfg.LLM.Gemini.APIKey,
			Model:       cfg.LLM.Gemini.Model,
			MaxTokens:   cfg.LLM.Gemini.MaxTokens,
			Temperature: cfg.LLM.Gemini.Temperature,
		}))
	}

	if cfg.LLM.GLM.Enabled {
		llmRouter.Register(llm.NewGLM(&llm.GLMConfig{
			APIKey:      cfg.LLM.GLM.APIKey,
			Model:       cfg.LLM.GLM.Model,
			MaxTokens:   cfg.LLM.GLM.MaxTokens,
			Temperature: cfg.LLM.GLM.Temperature,
			BaseURL:     cfg.LLM.GLM.BaseURL,
		}))
	}

	if cfg.LLM.DeepSeek.Enabled {
		llmRouter.Register(llm.NewDeepSeek(&llm.DeepSeekConfig{
			APIKey:      cfg.LLM.DeepSeek.APIKey,
			Model:       cfg.LLM.DeepSeek.Model,
			MaxTokens:   cfg.LLM.DeepSeek.MaxTokens,
			Temperature: cfg.LLM.DeepSeek.Temperature,
			BaseURL:     cfg.LLM.DeepSeek.BaseURL,
		}))
	}

	// Initialize session manager
	maxHistory := cfg.Session.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 50
	}
	sessionMgr := session.NewManager(nil, maxHistory)

	// Initialize message router
	rtr := router.NewRouter(store, vault, authorizer, rateLimiter, logger)

	// Set session notify function after router is created
	sessionMgr = session.NewManager(func(platform, chatID, message string) error {
		return rtr.Send(platform, chatID, message)
	}, maxHistory)

	// Set message handler with LLM integration
	rtr.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		logger.Info("received message",
			"platform", msg.Platform,
			"user", security.HashUserID(msg.Platform, msg.UserID),
		)

		// Handle commands
		if strings.HasPrefix(msg.Text, "/") {
			return handleCommand(msg, llmRouter, store)
		}

		// Get or create session for this chat
		sess := sessionMgr.GetOrCreate(msg.Platform, msg.ChatID, msg.UserID)

		// Build message list from session history
		history := sessionMgr.GetHistory(sess, maxHistory)
		messages := make([]llm.Message, 0, len(history)+1)
		for _, h := range history {
			messages = append(messages, llm.Message{
				Role:    h.Role,
				Content: h.Content,
			})
		}

		// Add current user message (with media if present)
		userMsg := llm.Message{
			Role:    "user",
			Content: msg.Text,
		}
		if len(msg.Media) > 0 {
			userMsg.Blocks = buildContentBlocks(msg.Text, msg.Media, logger)
		}
		messages = append(messages, userMsg)

		// Send to LLM with full conversation history
		resp, err := llmRouter.Chat(ctx, msg.UserID, messages)
		if err != nil {
			return llm.FormatError(err), nil
		}

		// Record messages in session
		sessionMgr.AddMessage(sess, "user", msg.Text)
		sessionMgr.AddMessage(sess, "assistant", resp.Content)

		logger.Debug("llm response",
			"provider", resp.Provider,
			"model", resp.Model,
			"latency", resp.Latency,
		)

		return resp.Content, nil
	})

	// Register platforms
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.Platforms.Telegram.Enabled {
		tg, err := telegram.New(&telegram.Config{
			Token:        cfg.Platforms.Telegram.BotToken,
			DownloadsDir: cfg.Paths.DownloadsDir,
			Logger:       logger.With("platform", "telegram"),
		})
		if err != nil {
			logger.Error("init telegram failed", "error", err)
		} else {
			rtr.Register(tg)
		}
	}

	if cfg.Platforms.Slack.Enabled {
		sl, err := slack.New(&slack.Config{
			BotToken: cfg.Platforms.Slack.BotToken,
			AppToken: cfg.Platforms.Slack.AppToken,
			Logger:   logger.With("platform", "slack"),
		})
		if err != nil {
			logger.Error("init slack failed", "error", err)
		} else {
			rtr.Register(sl)
		}
	}

	if cfg.Platforms.WhatsApp.Enabled {
		wa, err := whatsapp.New(&whatsapp.Config{
			DBPath: cfg.Platforms.WhatsApp.DBPath,
			Logger: logger.With("platform", "whatsapp"),
		})
		if err != nil {
			logger.Error("init whatsapp failed", "error", err)
		} else {
			rtr.Register(wa)
		}
	}

	if cfg.Platforms.Webhook.Enabled {
		wh, err := webhook.New(&webhook.Config{
			Port:        cfg.Platforms.Webhook.Port,
			Path:        cfg.Platforms.Webhook.Path,
			Bind:        cfg.Platforms.Webhook.Bind,
			AuthMethod:  cfg.Platforms.Webhook.AuthMethod,
			BearerToken: cfg.Platforms.Webhook.BearerToken,
			HMACSecret:  cfg.Platforms.Webhook.HMACSecret,
			AllowedIPs:  cfg.Platforms.Webhook.AllowedIPs,
			Logger:      logger.With("platform", "webhook"),
		})
		if err != nil {
			logger.Error("init webhook failed", "error", err)
		} else {
			rtr.Register(wh)
		}
	}

	// Start router
	if err := rtr.Start(ctx); err != nil {
		logger.Error("start router failed", "error", err)
		os.Exit(1)
	}

	logger.Info("magabot started",
		"version", version.Short(),
		"platforms", rtr.Platforms(),
		"llm_providers", llmRouter.Providers(),
	)

	// Wait for shutdown or restart signal (platform-specific)
	sigCh := make(chan os.Signal, 1)
	registerSignals(sigCh)

	for {
		sig := <-sigCh

		if handleReloadSignal(sig, rtr, logger) {
			continue
		}

		// SIGINT or SIGTERM (or os.Interrupt on Windows) - shutdown
		break
	}

	logger.Info("shutting down...")
	rtr.Stop()

	if cfg.Storage.Backup.Enabled {
		if info, err := backupMgr.Create(dataDir, rtr.Platforms()); err == nil {
			logger.Info("shutdown backup created", "file", info.Filename)
		}
	}

	logger.Info("magabot stopped")
}

// handleCommand handles bot commands
func handleCommand(msg *router.Message, llmRouter *llm.Router, store *storage.Store) (string, error) {
	parts := strings.Fields(msg.Text)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/start":
		return "👋 Halo! Saya Magabot.\n\nKirim pesan apapun dan saya akan menjawab menggunakan AI.\n\nCommands:\n• /status - Status bot\n• /models - List models\n• /help - Bantuan", nil

	case "/help":
		return `📖 *Magabot Help*

Kirim pesan apapun dan saya akan menjawab menggunakan AI.

*Commands:*
• /start - Mulai
• /status - Status bot
• /models - Available models
• /providers - LLM providers
• /help - Bantuan ini`, nil

	case "/status":
		stats, _ := store.Stats()
		llmStats := llmRouter.Stats()
		return fmt.Sprintf("📊 *Status*\n\n• LLM: %s\n• Providers: %v\n• Messages: %v",
			llmStats["default"],
			llmStats["available"],
			stats["messages"],
		), nil

	case "/models":
		models := llmRouter.ListAllModels(context.Background())
		if len(models) == 0 {
			return "❌ No models available", nil
		}
		return "🤖 *Available Models*" + llm.FormatModelList(models), nil

	case "/providers":
		stats := llmRouter.Stats()
		return fmt.Sprintf("🤖 *LLM Providers*\n\n• Default: %s\n• Available: %v",
			stats["default"],
			stats["available"],
		), nil

	case "/config":
		// Config commands are handled by AdminHandler in bot package
		return "🔧 Config commands available.\nUse /config help for more info.", nil

	default:
		return "❓ Unknown command. Try /help", nil
	}
}

// buildContentBlocks creates multi-modal content blocks from text and media paths
func buildContentBlocks(text string, mediaPaths []string, logger *slog.Logger) []llm.ContentBlock {
	var blocks []llm.ContentBlock

	// Add text block if present
	if text != "" {
		blocks = append(blocks, llm.ContentBlock{
			Type: "text",
			Text: text,
		})
	}

	// Add image blocks
	for _, path := range mediaPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("read media file failed", "path", path, "error", err)
			continue
		}

		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if mimeType == "" {
			mimeType = "image/jpeg"
		}

		blocks = append(blocks, llm.ContentBlock{
			Type:      "image",
			MimeType:  mimeType,
			ImageData: base64.StdEncoding.EncodeToString(data),
		})
	}

	return blocks
}
