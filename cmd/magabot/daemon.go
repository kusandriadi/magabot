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

	"github.com/kusa/magabot/internal/agent"
	"github.com/kusa/magabot/internal/backup"
	"github.com/kusa/magabot/internal/bot"
	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/hooks"
	"github.com/kusa/magabot/internal/llm"
	"github.com/kusa/magabot/internal/platform/slack"
	"github.com/kusa/magabot/internal/platform/telegram"
	"github.com/kusa/magabot/internal/platform/webhook"
	"github.com/kusa/magabot/internal/platform/whatsapp"
	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/secrets"
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

	// Load secrets from backend and overlay onto config
	secretsMgr := loadSecrets(cfg, logger)
	if secretsMgr != nil {
		defer secretsMgr.Stop()
	}

	// Ensure all configured directories exist before initializing subsystems
	if err := cfg.EnsureDirectories(); err != nil {
		logger.Error("ensure directories failed", "error", err)
		os.Exit(1)
	}

	// Initialize vault
	vault, err := security.NewVault(cfg.Security.EncryptionKey)
	if err != nil {
		logger.Error("init vault failed", "error", err)
		os.Exit(1)
	}

	// Initialize storage
	store, err := storage.New(cfg.GetDatabasePath())
	if err != nil {
		logger.Error("init storage failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize backup manager
	backupMgr := backup.New(cfg.GetBackupDir(), cfg.Storage.Backup.KeepCount)

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
		Default:      cfg.LLM.Default,
		SystemPrompt: cfg.LLM.SystemPrompt,
		MaxInput:     cfg.LLM.MaxInputLength,
		Timeout:      time.Duration(cfg.LLM.Timeout) * time.Second,
		RateLimit:    cfg.LLM.RateLimit,
		Logger:       logger.With("component", "llm"),
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

	if cfg.LLM.Local.Enabled {
		llmRouter.Register(llm.NewLocal(&llm.LocalConfig{
			Enabled:     true,
			BaseURL:     cfg.LLM.Local.BaseURL,
			Model:       cfg.LLM.Local.Model,
			APIKey:      cfg.LLM.Local.APIKey,
			MaxTokens:   cfg.LLM.Local.MaxTokens,
			Temperature: cfg.LLM.Local.Temperature,
		}))
	}

	// Initialize bot handlers
	adminHandler := bot.NewAdminHandler(cfg, configDir)
	memoryHandler := bot.NewMemoryHandler(cfg.Paths.MemoryDir)

	// Initialize session manager
	maxHistory := cfg.Session.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 50
	}

	// Initialize message router
	rtr := router.NewRouter(store, vault, cfg, authorizer, rateLimiter, logger)

	// Initialize audit logger
	auditLogger, err := security.NewAuditLogger(filepath.Dir(cfg.GetSecurityLogPath()))
	if err != nil {
		logger.Warn("init audit logger failed, continuing without it", "error", err)
	} else {
		rtr.SetAuditLogger(auditLogger)
		defer auditLogger.Close()
	}

	// Initialize hooks manager — load from config-hooks.yml, merge with inline config hooks
	hooksMgr := hooks.NewManager(mergeHooksConfig(cfg, logger), logger.With("component", "hooks"))
	rtr.SetHooks(hooksMgr)

	// Initialize agent session manager
	agentMgr := agent.NewManager(agent.Config{
		Default: cfg.Agents.Default,
		Timeout: cfg.Agents.Timeout,
	}, logger.With("component", "agent"))

	// Create session manager with router's send function for background task notifications
	sessionMgr := session.NewManager(func(platform, chatID, message string) error {
		return rtr.Send(platform, chatID, message)
	}, maxHistory, logger)
	sessionHandler := bot.NewSessionHandler(sessionMgr)

	// Set message handler with LLM integration
	rtr.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		logger.Info("received message",
			"platform", msg.Platform,
			"user", security.HashUserID(msg.Platform, msg.UserID),
		)

		// Handle bot commands
		if strings.HasPrefix(msg.Text, "/") {
			return handleCommand(msg, llmRouter, store, cfg, adminHandler, memoryHandler, sessionHandler)
		}

		// Handle agent session commands (:new, :send, :quit, :status)
		if strings.HasPrefix(msg.Text, ":") {
			return handleAgentCommand(msg, agentMgr, cfg)
		}

		// Route to active agent session if one exists
		if agentMgr.HasSession(msg.Platform, msg.ChatID) {
			return routeToAgent(ctx, msg, agentMgr)
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
			userMsg.Blocks = buildContentBlocks(msg.Text, msg.Media, cfg.Paths.DownloadsDir, logger)
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

	// Fire on_start hooks
	hooksMgr.FireAsync(hooks.OnStart, &hooks.EventData{
		Version:   version.Short(),
		Platforms: rtr.Platforms(),
	})

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

	// Fire on_stop hooks (synchronous, give hooks a chance to run)
	hooksMgr.Fire(hooks.OnStop, &hooks.EventData{
		Version:   version.Short(),
		Platforms: rtr.Platforms(),
	})

	done := make(chan struct{})
	go func() {
		rtr.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		logger.Warn("shutdown timed out after 10s")
	}

	if cfg.Storage.Backup.Enabled {
		if info, err := backupMgr.Create(dataDir, rtr.Platforms()); err == nil {
			logger.Info("shutdown backup created", "file", info.Filename)
		}
	}

	logger.Info("magabot stopped")
}

// handleCommand handles bot commands
func handleCommand(msg *router.Message, llmRouter *llm.Router, store *storage.Store, cfg *config.Config, adminH *bot.AdminHandler, memoryH *bot.MemoryHandler, sessionH *bot.SessionHandler) (string, error) {
	parts := strings.Fields(msg.Text)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/start":
		return "👋 Halo! Saya Magabot.\n\nKirim pesan apapun dan saya akan menjawab menggunakan AI.\n\nCommands:\n• /status - Status bot\n• /models - List models\n• /config - Admin config\n• /memory - Memory management\n• /task - Background tasks\n• /help - Bantuan", nil

	case "/help":
		return `📖 *Magabot Help*

Kirim pesan apapun dan saya akan menjawab menggunakan AI.

*Commands:*
• /start - Mulai
• /status - Status bot
• /models - Available models
• /providers - LLM providers
• /config - Admin configuration
• /memory - Memory management
• /task - Background tasks
• /help - Bantuan ini

*Agent Sessions:*
• :new [agent] <dir> - Start coding agent (claude/codex/gemini)
• :send <message> - Send message to active agent
• :quit - Close agent session
• :status - Show agent session info`, nil

	case "/status":
		stats, err := store.Stats()
		if err != nil {
			return fmt.Sprintf("📊 *Status*\n\n⚠️ Error getting stats: %v", err), nil
		}
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
		if !cfg.IsPlatformAdmin(msg.Platform, msg.UserID) {
			return "🔒 Admin access required.", nil
		}
		resp, needRestart, err := adminH.HandleCommand(msg.Platform, msg.UserID, msg.ChatID, args)
		if err != nil {
			return fmt.Sprintf("❌ Error: %v", err), nil
		}
		if needRestart {
			adminH.ScheduleRestart(3, nil)
		}
		return resp, nil

	case "/memory":
		return memoryH.HandleCommand(msg.UserID, msg.Platform, args)

	case "/task":
		return sessionH.HandleCommand(msg.UserID, msg.Platform, msg.ChatID, args)

	default:
		return "❓ Unknown command. Try /help", nil
	}
}

// loadSecrets loads secrets from the configured backend and overlays them onto
// the config. Config values (from YAML/env vars) take precedence — secrets are
// only applied when the config field is empty. Returns the Manager so the
// caller can defer Stop() for background token renewal cleanup.
func loadSecrets(cfg *config.Config, logger *slog.Logger) *secrets.Manager {
	if cfg.Secrets.Backend == "" {
		return nil
	}

	// Convert config types to secrets package types
	secretsCfg := &secrets.Config{
		Backend: cfg.Secrets.Backend,
	}
	if cfg.Secrets.Vault != nil {
		secretsCfg.VaultConfig = &secrets.VaultConfig{
			Address:       cfg.Secrets.Vault.Address,
			MountPath:     cfg.Secrets.Vault.MountPath,
			SecretPath:    cfg.Secrets.Vault.SecretPath,
			TLSCACert:     cfg.Secrets.Vault.TLSCACert,
			TLSClientCert: cfg.Secrets.Vault.TLSClientCert,
			TLSClientKey:  cfg.Secrets.Vault.TLSClientKey,
			TLSSkipVerify: cfg.Secrets.Vault.TLSSkipVerify,
			Logger:        logger.With("component", "vault"),
		}
	}
	if cfg.Secrets.Local != nil {
		secretsCfg.LocalConfig = &secrets.LocalConfig{
			Path: cfg.Secrets.Local.Path,
		}
	}

	mgr, err := secrets.NewFromConfig(secretsCfg)
	if err != nil {
		logger.Warn("secrets backend init failed, skipping", "backend", cfg.Secrets.Backend, "error", err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("loading secrets", "backend", mgr.Backend())

	// Map of secret key -> pointer to config field
	type secretMapping struct {
		key  string
		dest *string
		name string
	}

	mappings := []secretMapping{
		{secrets.KeyEncryptionKey, &cfg.Security.EncryptionKey, "encryption_key"},
		{secrets.KeyAnthropicAPIKey, &cfg.LLM.Anthropic.APIKey, "anthropic_api_key"},
		{secrets.KeyOpenAIAPIKey, &cfg.LLM.OpenAI.APIKey, "openai_api_key"},
		{secrets.KeyGeminiAPIKey, &cfg.LLM.Gemini.APIKey, "gemini_api_key"},
		{secrets.KeyGLMAPIKey, &cfg.LLM.GLM.APIKey, "glm_api_key"},
		{secrets.KeyDeepSeekAPIKey, &cfg.LLM.DeepSeek.APIKey, "deepseek_api_key"},
	}

	// Platform secrets need nil-safe handling
	if cfg.Platforms.Telegram != nil {
		mappings = append(mappings, secretMapping{secrets.KeyTelegramToken, &cfg.Platforms.Telegram.BotToken, "telegram_bot_token"})
	}
	if cfg.Platforms.Slack != nil {
		mappings = append(mappings,
			secretMapping{secrets.KeySlackBotToken, &cfg.Platforms.Slack.BotToken, "slack_bot_token"},
			secretMapping{secrets.KeySlackAppToken, &cfg.Platforms.Slack.AppToken, "slack_app_token"},
		)
	}

	var loaded int
	for _, m := range mappings {
		if *m.dest != "" {
			continue // config value already set, skip
		}
		val, err := mgr.Get(ctx, m.key)
		if err != nil {
			continue // not found or error, skip silently
		}
		*m.dest = val
		loaded++
		logger.Debug("loaded secret", "key", m.name)
	}

	if loaded > 0 {
		logger.Info("secrets loaded", "count", loaded, "backend", mgr.Backend())
	}

	return mgr
}

// buildContentBlocks creates multi-modal content blocks from text and media paths.
// allowedDir restricts which directory files may be read from (path traversal protection).
func buildContentBlocks(text string, mediaPaths []string, allowedDir string, logger *slog.Logger) []llm.ContentBlock {
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
		// Validate path is within allowed directory to prevent directory traversal
		absPath, err := filepath.Abs(path)
		if err != nil {
			logger.Warn("invalid media path", "path", path, "error", err)
			continue
		}
		absDir, err := filepath.Abs(allowedDir)
		if err != nil || !strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
			logger.Warn("media path outside allowed directory", "path", path, "allowed", allowedDir)
			continue
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			logger.Warn("read media file failed", "path", absPath, "error", err)
			continue
		}

		mimeType := mime.TypeByExtension(filepath.Ext(absPath))
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

// handleAgentCommand processes colon-prefixed agent session commands.
// Only platform admins can use agent sessions (they execute code on the server).
func handleAgentCommand(msg *router.Message, agentMgr *agent.Manager, cfg *config.Config) (string, error) {
	// Agent sessions execute code on the server — restrict to admins
	if !cfg.IsPlatformAdmin(msg.Platform, msg.UserID) {
		return "Agent sessions require admin access.", nil
	}

	parts := strings.Fields(msg.Text)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case ":new":
		if agentMgr.HasSession(msg.Platform, msg.ChatID) {
			return "Agent session already active. Use :quit first.", nil
		}

		agentType := ""
		dir := ""

		switch len(parts) {
		case 1:
			return "Usage: :new [agent] <directory>\nAgents: claude, codex, gemini", nil
		case 2:
			dir = parts[1]
		default:
			if agent.ValidAgent(parts[1]) {
				agentType = parts[1]
				dir = parts[2]
			} else {
				dir = parts[1]
			}
		}

		sess, err := agentMgr.NewSession(msg.Platform, msg.ChatID, msg.UserID, agentType, dir)
		if err != nil {
			return fmt.Sprintf("Failed to start agent session: %v", err), nil
		}

		return fmt.Sprintf("Agent session started: %s in %s\nSend messages to interact. Use :quit to end.", sess.Agent, sess.Dir), nil

	case ":send":
		if !agentMgr.HasSession(msg.Platform, msg.ChatID) {
			return "No active agent session. Use :new to start one.", nil
		}
		if len(parts) < 2 {
			return "Usage: :send <message>", nil
		}
		message := strings.TrimSpace(strings.TrimPrefix(msg.Text, parts[0]))
		sess := agentMgr.GetSession(msg.Platform, msg.ChatID)
		ctx := context.Background()
		output, err := agentMgr.Execute(ctx, sess, message)
		if err != nil {
			return fmt.Sprintf("Agent error: %v", err), nil
		}
		if output == "" {
			return "(no output)", nil
		}
		return output, nil

	case ":quit", ":exit", ":close":
		if !agentMgr.HasSession(msg.Platform, msg.ChatID) {
			return "No active agent session.", nil
		}
		agentMgr.CloseSession(msg.Platform, msg.ChatID)
		return "Agent session closed.", nil

	case ":status":
		sess := agentMgr.GetSession(msg.Platform, msg.ChatID)
		if sess == nil {
			return "No active agent session.", nil
		}
		return fmt.Sprintf("Agent: %s\nDirectory: %s\nMessages: %d", sess.Agent, sess.Dir, sess.GetMsgCount()), nil

	default:
		return fmt.Sprintf("Unknown agent command: %s\nAvailable: :new, :send, :quit, :status", cmd), nil
	}
}

// routeToAgent sends a regular message to the active agent session.
func routeToAgent(ctx context.Context, msg *router.Message, agentMgr *agent.Manager) (string, error) {
	sess := agentMgr.GetSession(msg.Platform, msg.ChatID)
	if sess == nil {
		return "", nil
	}

	output, err := agentMgr.Execute(ctx, sess, msg.Text)
	if err != nil {
		return fmt.Sprintf("Agent error: %v", err), nil
	}
	if output == "" {
		return "(no output)", nil
	}
	return output, nil
}

// mergeHooksConfig loads hooks from config-hooks.yml and merges with inline config hooks.
// File hooks take precedence over inline hooks with the same name.
func mergeHooksConfig(cfg *config.Config, logger *slog.Logger) []config.HookConfig {
	hooksFile := filepath.Join(configDir, "config-hooks.yml")
	fileHooks, err := config.LoadHooksFile(hooksFile)
	if err != nil {
		logger.Warn("failed to load hooks file", "path", hooksFile, "error", err)
	}

	if len(fileHooks) == 0 {
		return cfg.Hooks
	}

	if len(cfg.Hooks) == 0 {
		return fileHooks
	}

	// File hooks take precedence by name
	seen := make(map[string]bool)
	var merged []config.HookConfig
	for _, h := range fileHooks {
		seen[h.Name] = true
		merged = append(merged, h)
	}
	for _, h := range cfg.Hooks {
		if !seen[h.Name] {
			merged = append(merged, h)
		}
	}

	logger.Info("hooks loaded", "file", len(fileHooks), "inline", len(cfg.Hooks), "merged", len(merged))
	return merged
}
