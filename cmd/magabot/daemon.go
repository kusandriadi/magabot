package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	"github.com/kusa/magabot/internal/updater"
	"github.com/kusa/magabot/internal/util"
	"github.com/kusa/magabot/internal/version"
	"github.com/kusandriadi/allm-go"
	"github.com/kusandriadi/allm-go/provider"
)

// defaultCLITools is the default set of tools allowed for Claude CLI mode.
// These are safe for a bot context — read-only tools plus web search.
var defaultCLITools = []string{
	"Read", "Glob", "Grep", "WebSearch", "WebFetch",
}

// runDaemon runs the main bot daemon
func runDaemon() {
	// Change to home directory so CLI tools (e.g. claude) don't pick up
	// project-specific config (CLAUDE.md, memory) from the working directory.
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Chdir(home)
	}

	// Load config
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Prune disabled providers/platforms from config file
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not prune config: %v\n", err)
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
		defer func() { _ = logFileHandle.Close() }()
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

	// Initialize vault (optional — if no encryption key, messages are stored in plaintext)
	var vault *security.Vault
	if cfg.Security.EncryptionKey != "" {
		var err error
		vault, err = security.NewVault(cfg.Security.EncryptionKey)
		if err != nil {
			logger.Error("init vault failed", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Warn("no encryption key configured, message logging will store plaintext")
	}

	// Initialize storage
	store, err := storage.New(cfg.GetDatabasePath())
	if err != nil {
		logger.Error("init storage failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

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
	llmCfg := &llm.Config{
		Main:            cfg.LLM.Main,
		SystemPrompt:    cfg.LLM.SystemPrompt,
		MaxInput:        cfg.LLM.MaxInputLength,
		MaxContextChars: cfg.LLM.MaxContextChars,
		Timeout:         cfg.LLM.Timeout.Duration(),
		RateLimit:       cfg.LLM.RateLimit,
		Logger:          logger.With("component", "llm"),
	}
	llmRouter := llm.NewRouter(llmCfg)

	// Enable prompt caching if configured
	if cfg.LLM.PromptCaching {
		llmRouter.EnablePromptCaching()
	}

	// Register LLM providers using allm-go (with URL validation - A10 SSRF protection)
	if cfg.LLM.Anthropic.Enabled {
		if err := registerAnthropicProvider(llmRouter, cfg); err != nil {
			logger.Error("register anthropic provider failed", "error", err)
		}
	}

	if cfg.LLM.OpenAI.Enabled {
		if err := registerOpenAIProvider(llmRouter, cfg); err != nil {
			logger.Error("register openai provider failed", "error", err)
		}
	}

	if cfg.LLM.GLM.Enabled {
		if err := registerAnthropicCompatProvider(llmRouter, "glm", cfg.LLM.GLM, cfg); err != nil {
			logger.Error("register glm provider failed", "error", err)
		}
	}

	if cfg.LLM.Local.Enabled {
		if err := registerCompatProvider(llmRouter, compatProviderConfig{
			name: "local", model: cfg.LLM.Local.Model,
			maxTokens: cfg.LLM.Local.MaxTokens, temperature: cfg.LLM.Local.Temperature,
			baseURL: cfg.LLM.Local.BaseURL, isLocal: true, maxRetries: cfg.LLM.Local.MaxRetries,
		}, cfg); err != nil {
			logger.Error("register local provider failed", "error", err)
		}
	}

	if cfg.LLM.Kimi.Enabled {
		if err := registerAnthropicCompatProvider(llmRouter, "kimi", cfg.LLM.Kimi, cfg); err != nil {
			logger.Error("register kimi provider failed", "error", err)
		}
	}

	if cfg.LLM.MiniMax.Enabled {
		if err := registerAnthropicCompatProvider(llmRouter, "minimax", cfg.LLM.MiniMax, cfg); err != nil {
			logger.Error("register minimax provider failed", "error", err)
		}
	}

	// Restore persisted LLM settings (effort, fallback) from config
	restoreLLMSettings(llmRouter, cfg, logger)

	// Initialize bot handlers
	adminHandler := bot.NewAdminHandler(cfg, configDir)
	memoryHandler := bot.NewMemoryHandler(cfg.Paths.MemoryDir)
	confirmMgr := bot.NewConfirmationManager()

	// Initialize session manager
	maxHistory := cfg.Session.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 200
	}

	// Initialize message router
	rtr := router.NewRouter(store, vault, cfg, authorizer, rateLimiter, logger)

	// Initialize audit logger
	auditLogger, err := security.NewAuditLogger(filepath.Dir(cfg.GetSecurityLogPath()))
	if err != nil {
		logger.Warn("init audit logger failed, continuing without it", "error", err)
	} else {
		rtr.SetAuditLogger(auditLogger)
		defer func() { _ = auditLogger.Close() }()
	}

	// Initialize hooks manager — load from config-hooks.yml, merge with inline config hooks
	hooksMgr := hooks.NewManager(mergeHooksConfig(cfg, logger), logger.With("component", "hooks"))
	rtr.SetHooks(hooksMgr)

	// Initialize agent session manager
	agentMgr := agent.NewManager(agent.Config{
		Timeout:        cfg.Agent.Timeout.Seconds(),
		MaxRetries:     cfg.Agent.MaxRetries,
		SessionTimeout: cfg.Agent.SessionTimeout.Seconds(),
		Shortcuts:      cfg.Agent.Shortcuts,
		DiscoverDepth:  cfg.Agent.DiscoverDepth,
		PlanDelegate:   cfg.Agent.PlanDelegate != nil && *cfg.Agent.PlanDelegate,
		CLIPath:        cfg.LLM.Anthropic.CLIPath,
		PlanModel:      cfg.LLM.Anthropic.PlanModel,
		ImplModel:      cfg.LLM.Anthropic.ImplModel,
		OnSessionClose: func(platform, chatID, message string) {
			_ = rtr.Send(platform, chatID, message)
		},
		GetCLISettings: func() string {
			if cli := llmRouter.CLIProvider(); cli != nil {
				return cli.Effort()
			}
			return ""
		},
	}, logger.With("component", "agent"))

	// Create session manager with router's send function for background task notifications
	sessionMgr := session.NewManager(func(platform, chatID, message string) error {
		return rtr.Send(platform, chatID, message)
	}, maxHistory, logger)
	sessionHandler := bot.NewSessionHandler(sessionMgr)

	// Preload conversation history from DB into session memory
	if keys, err := store.ListConversationSessions(); err != nil {
		logger.Warn("failed to list conversation sessions", "error", err)
	} else {
		restored := 0
		for _, key := range keys {
			// key format: "platform:chatID"
			parts := strings.SplitN(key, ":", 2)
			if len(parts) != 2 {
				continue
			}
			dbHistory, err := store.GetConversationHistory(key, maxHistory)
			if err != nil || len(dbHistory) == 0 {
				continue
			}
			sess := sessionMgr.GetOrCreate(parts[0], parts[1], "")
			for _, h := range dbHistory {
				sessionMgr.AddMessage(sess, h.Role, h.Content)
			}
			restored++
		}
		if restored > 0 {
			logger.Info("preloaded conversation sessions", "count", restored)
		}
	}

	// Set message handler with LLM integration
	rtr.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		logArgs := []any{"platform", msg.Platform, "user", security.HashUserID(msg.Platform, msg.UserID)}
		if msg.ReplyTo != nil {
			logArgs = append(logArgs, "reply_to_user", msg.ReplyTo.Username, "reply_to_text", util.Truncate(msg.ReplyTo.Text, 80))
		}
		logger.Info("received message", logArgs...)

		// Handle bot commands
		if strings.HasPrefix(msg.Text, "/") {
			return handleCommand(msg, llmRouter, store, cfg, adminHandler, memoryHandler, sessionHandler, sessionMgr, confirmMgr, logger)
		}

		// Handle pending confirmation (y/n)
		if confirmMgr.HasPending(msg.Platform, msg.ChatID) {
			lower := strings.ToLower(strings.TrimSpace(msg.Text))
			if lower == "y" || lower == "yes" {
				resp, _ := confirmMgr.Confirm(msg.Platform, msg.ChatID, msg.UserID)
				return resp, nil
			}
			if lower == "n" || lower == "no" {
				resp, _ := confirmMgr.Cancel(msg.Platform, msg.ChatID, msg.UserID)
				return resp, nil
			}
		}

		// Handle agent session commands (:new, :quit, :status)
		if strings.HasPrefix(msg.Text, ":") {
			return handleAgentCommand(msg, agentMgr, cfg)
		}

		// Route to active agent session if one exists
		if agentMgr.HasSession(msg.Platform, msg.ChatID) {
			return routeToAgent(ctx, msg, agentMgr, func(text string) {
				_ = rtr.Send(msg.Platform, msg.ChatID, text)
			}, llmRouter)
		}

		// If the message looks like a coding implementation task, short-circuit before
		// calling the LLM — agent mode is required for file edits and the LLM cannot do them.
		isAdmin := cfg.IsPlatformAdmin(msg.Platform, msg.UserID)
		if looksLikeAgentTask(msg.Text) {
			if isAdmin {
				return "💡 This task requires direct file editing. Start an agent session with `:new <directory>`.", nil
			}
			return "⚠️ This request requires agent mode which requires admin access.", nil
		}

		// Check if first-time user
		isFirst, err := store.IsFirstMessage(msg.Platform, security.HashUserID(msg.Platform, msg.UserID))
		if err != nil {
			// Log but don't block — non-critical
			logger.Warn("first message check failed", "error", err)
		}

		// Auto-promote first user as platform admin if no admins exist yet
		if isFirst && cfg.PromoteFirstAdmin(msg.Platform, msg.UserID) {
			logger.Info("first user promoted to admin",
				"platform", msg.Platform, "user_id", msg.UserID,
			)
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

		// Add current user message (with reply context and media file paths if present)
		content := msg.Text
		isVoiceMsg := false // set to true only if transcription actually succeeds

		// Prepend reply context so the LLM knows what message is being replied to
		if msg.ReplyTo != nil && msg.ReplyTo.Text != "" {
			sender := msg.ReplyTo.Username
			if sender == "" {
				if msg.ReplyTo.IsBot {
					sender = "assistant"
				} else {
					sender = "someone"
				}
			}
			content = fmt.Sprintf("[Replying to %s: %s]\n\n%s", sender, msg.ReplyTo.Text, content)
		}

		if len(msg.Media) > 0 {
			var otherMedia []string
			var transcripts []string
			for _, path := range msg.Media {
				if isAudioFile(path) {
					transcript, err := transcribeAudioFile(path)
					if err != nil {
						logger.Warn("voice transcription failed", "path", path, "error", err)
						otherMedia = append(otherMedia, path)
					} else if transcript != "" {
						transcripts = append(transcripts, transcript)
					}
				} else {
					otherMedia = append(otherMedia, path)
				}
			}

			if len(transcripts) > 0 {
				transcribed := strings.Join(transcripts, " ")
				// Replace the voice placeholder with the actual transcription
				if content == "[Voice Message]" || content == "[Audio Message]" {
					content = transcribed
				} else {
					content = transcribed + "\n\n" + content
				}
				msg.Media = otherMedia
				isVoiceMsg = true // transcription succeeded — reply with voice
			}

			if len(otherMedia) > 0 {
				var parts []string
				parts = append(parts, "User sent files (use Read tool to view them):")
				for _, path := range otherMedia {
					parts = append(parts, "  "+path)
				}
				if content != "" {
					parts = append(parts, "", content)
				}
				content = strings.Join(parts, "\n")
			}
		}
		userMsg := llm.Message{
			Role:    "user",
			Content: content,
		}
		messages = append(messages, userMsg)

		// Prepend welcome message for first-time users
		welcomePrefix := ""
		if isFirst {
			welcomePrefix = "👋 *Welcome!* This is our first conversation.\nType /help to see all features.\n\n"
		}

		// Build system prompt from active persona + platform formatting rules
		var systemPromptOverride string
		if len(cfg.Personas.List) > 0 {
			personaName, _ := sessionMgr.GetContext(sess, "persona").(string)
			persona := cfg.GetPersona(personaName)
			if persona == nil {
				persona = cfg.GetDefaultPersona()
			}
			if persona != nil {
				systemPromptOverride = llm.BuildSystemPrompt(persona.SystemPrompt, msg.Platform)
			}
		}
		if systemPromptOverride == "" {
			// No personas configured — use llm.system_prompt with platform-aware formatting
			systemPromptOverride = llm.BuildSystemPrompt(cfg.LLM.SystemPrompt, msg.Platform)
		}

		// For voice input: suppress text streaming — we'll send a voice reply at the end
		if isVoiceMsg {
			msg.StreamCallback = func(string) {}
		}

		// Send to LLM (streaming)
		ch, err := llmRouter.StreamChat(ctx, msg.UserID, messages, systemPromptOverride)
		if err != nil {
			return llm.FormatError(err), nil
		}

		var respContent string
		{
			var content strings.Builder
			var thinking bool
			for chunk := range ch {
				if chunk.Error != nil {
					if content.Len() == 0 {
						return llm.FormatError(chunk.Error), nil
					}
					break // keep partial response
				}
				// Show thinking indicator while AI reasons
				if chunk.Thinking != "" && content.Len() == 0 {
					if !thinking {
						thinking = true
						msg.StreamCallback(welcomePrefix + "🤔 _Thinking..._")
					}
					continue
				}
				content.WriteString(chunk.Content)
				if content.Len() > 0 {
					msg.StreamCallback(welcomePrefix + content.String())
				}
			}
			respContent = content.String()
		}

		if respContent == "" {
			return "", nil
		}

		// Record messages in session (use resolved content, which includes transcription if voice)
		sessionMgr.AddMessage(sess, "user", userMsg.Content)
		sessionMgr.AddMessage(sess, "assistant", respContent)

		// Persist conversation to database
		sessionKey := fmt.Sprintf("%s:%s", msg.Platform, msg.ChatID)
		now := time.Now()
		if err := store.SaveConversationMessage(sessionKey, "user", userMsg.Content, now); err != nil {
			logger.Warn("save conversation message failed", "error", err, "role", "user")
		}
		if err := store.SaveConversationMessage(sessionKey, "assistant", respContent, now); err != nil {
			logger.Warn("save conversation message failed", "error", err, "role", "assistant")
		}

		// For voice messages: reply with TTS audio. Fall back to text if TTS is unavailable.
		if isVoiceMsg {
			ttsText := welcomePrefix + respContent
			audio, err := llmRouter.Speak(ctx, ttsText)
			if err != nil {
				// No OpenAI TTS — try local tts-speak script
				audio, err = speakText(ttsText)
			}
			if err == nil {
				if voiceErr := rtr.SendVoice(msg.Platform, msg.ChatID, audio); voiceErr != nil {
					logger.Warn("send voice failed, falling back to text", "error", voiceErr)
				} else {
					return "", nil // voice sent — suppress text
				}
			} else {
				logger.Warn("TTS unavailable, falling back to text", "error", err)
			}
		}

		return welcomePrefix + respContent, nil
	})

	// Register platforms
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.Platforms.Telegram != nil && cfg.Platforms.Telegram.Enabled {
		tg, err := telegram.New(&telegram.Config{
			Token:        cfg.Platforms.Telegram.BotToken,
			DownloadsDir: filepath.Join(cfg.GetPlatformDir("telegram"), "downloads"),
			Logger:       logger.With("platform", "telegram"),
		})
		if err != nil {
			logger.Error("init telegram failed", "error", err)
		} else {
			rtr.Register(tg)
		}
	}

	if cfg.Platforms.Slack != nil && cfg.Platforms.Slack.Enabled {
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

	if cfg.Platforms.WhatsApp != nil && cfg.Platforms.WhatsApp.Enabled {
		wa, err := whatsapp.New(&whatsapp.Config{
			DataDir:      cfg.GetPlatformDir("whatsapp"),
			DownloadsDir: filepath.Join(cfg.GetPlatformDir("whatsapp"), "downloads"),
			Logger:       logger.With("platform", "whatsapp"),
			OnPairFailure: func() {
				cfg.Platforms.WhatsApp.Enabled = false
				if err := cfg.Save(); err != nil {
					logger.Error("failed to disable whatsapp in config", "error", err)
				} else {
					logger.Warn("whatsapp disabled in config — run 'magabot setup platform' to re-enable")
				}
			},
		})
		if err != nil {
			logger.Error("init whatsapp failed", "error", err)
		} else {
			rtr.Register(wa)
		}
	}

	if cfg.Platforms.Webhook != nil && cfg.Platforms.Webhook.Enabled {
		wh, err := webhook.New(&webhook.Config{
			Port:         cfg.Platforms.Webhook.Port,
			Path:         cfg.Platforms.Webhook.Path,
			Bind:         cfg.Platforms.Webhook.Bind,
			AuthMethod:   cfg.Platforms.Webhook.AuthMethod,
			BearerToken:  cfg.Platforms.Webhook.BearerToken,
			BearerTokens: cfg.Platforms.Webhook.BearerTokens,
			HMACSecret:   cfg.Platforms.Webhook.HMACSecret,
			HMACUsers:    cfg.Platforms.Webhook.HMACUsers,
			AllowedIPs:   cfg.Platforms.Webhook.AllowedIPs,
			AllowedUsers: cfg.Platforms.Webhook.AllowedUsers,
			Logger:       logger.With("platform", "webhook"),
		})
		if err != nil {
			logger.Error("init webhook failed", "error", err)
		} else {
			rtr.Register(wh)
		}
	}

	// Start download cleanup goroutine
	if cfg.Media.RetentionDays > 0 {
		downloadDirs := []string{
			filepath.Join(cfg.GetPlatformDir("telegram"), "downloads"),
			filepath.Join(cfg.GetPlatformDir("whatsapp"), "downloads"),
		}
		retention := time.Duration(cfg.Media.RetentionDays) * 24 * time.Hour
		go func() {
			cleanOldDownloads(downloadDirs, retention, logger)
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cleanOldDownloads(downloadDirs, retention, logger)
				}
			}
		}()
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

	// Send post-restart notification if pending
	sendRestartNotify(rtr, logger)

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
	agentMgr.Stop()

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

// cleanOldDownloads deletes files in dirs that are older than maxAge.
func cleanOldDownloads(dirs []string, maxAge time.Duration, logger *slog.Logger) {
	cutoff := time.Now().Add(-maxAge)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // dir may not exist yet
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				path := filepath.Join(dir, entry.Name())
				if err := os.Remove(path); err == nil {
					logger.Debug("removed old download", "path", path)
				}
			}
		}
	}
}

// localScriptPath returns the full path to a script in ~/.local/bin/.
func localScriptPath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", name)
}

// speakText runs the local tts-speak script and returns OGG Opus audio bytes.
func speakText(text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, localScriptPath("tts-speak"))
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tts-speak: %w", err)
	}
	return out, nil
}

// transcribeAudioFile runs the local whisper script on an audio file.
func transcribeAudioFile(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, localScriptPath("transcribe-voice"), path).Output()
	if err != nil {
		return "", fmt.Errorf("transcribe-voice: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// looksLikeAgentTask returns true if the message appears to be a request
// to implement, create, edit, or otherwise modify code/files on the system.
// These tasks require agent mode since the LLM has no file-editing tools.
func looksLikeAgentTask(text string) bool {
	lower := strings.ToLower(text)

	// Action verbs that imply modifying something
	actions := []string{
		"implement", "implementasikan",
		"buat fitur", "bikin fitur", "tambah fitur", "add feature", "create feature",
		"buat fungsi", "bikin fungsi", "tambah fungsi", "add function", "create function",
		"buat method", "add method", "create method",
		"buat class", "add class", "create class",
		"edit file", "ubah file", "modify file", "update file", "change file",
		"edit kode", "ubah kode", "modify code", "update code", "change code", "edit code",
		"fix bug", "perbaiki bug", "fix the bug", "debug dan fix",
		"refactor", "restructure", "reorganize",
		"hapus file", "delete file", "remove file",
		"hapus fungsi", "delete function", "remove function",
		"rename file", "move file", "pindah file",
		"tulis kode", "write the code", "write code for",
		"buat script", "bikin script", "create script", "write script",
	}
	for _, a := range actions {
		if strings.Contains(lower, a) {
			return true
		}
	}
	return false
}

// isAudioFile returns true if the file extension is a known audio format.
func isAudioFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ogg", ".oga", ".mp3", ".m4a", ".wav":
		return true
	}
	return false
}

// handleCommand handles bot commands
func handleCommand(msg *router.Message, llmRouter *llm.Router, store *storage.Store, cfg *config.Config, adminH *bot.AdminHandler, memoryH *bot.MemoryHandler, sessionH *bot.SessionHandler, sessionMgr *session.Manager, confirmMgr *bot.ConfirmationManager, logger *slog.Logger) (string, error) {
	parts := strings.Fields(msg.Text)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(parts[0])
	// Strip @botname suffix from Telegram commands (e.g., /models@mybot → /models)
	if i := strings.Index(cmd, "@"); i > 0 {
		cmd = cmd[:i]
	}
	args := parts[1:]

	switch cmd {
	case "/yes", "/confirm":
		if resp, handled := confirmMgr.Confirm(msg.Platform, msg.ChatID, msg.UserID); handled {
			return resp, nil
		}
		return "No pending action to confirm.", nil

	case "/no", "/cancel":
		if resp, handled := confirmMgr.Cancel(msg.Platform, msg.ChatID, msg.UserID); handled {
			return resp, nil
		}
		return "No pending action to cancel.", nil

	case "/start":
		welcome := `👋 *Hi! I'm Magabot* — your personal AI chatbot.

💬 Send any message and I'll reply using AI.

🎯 What I can do:
1. 💬 Chat — ask anything, multi-turn conversation
2. 📷 Image — send a photo, I'll analyze it (vision)
3. 🎤 Voice — send a voice message, I'll transcribe & reply
4. 📄 Document — send a PDF/file, I'll read & analyze it
5. 🎨 Generate — ask me to create an image (DALL-E)
6. 🔊 TTS — I can reply with voice messages
7. 💭 Thinking — deep reasoning for complex questions

⚡ /help — full help
📊 /status — bot & provider status
🔧 /config — bot configuration
🧠 /memory — memory management`
		return welcome, nil

	case "/help":
		return `📖 *Magabot Help*

Send any message and I'll reply using AI.

💬 Commands:
 1. /start — Welcome message
 2. /status — Bot status
 3. /model — Current model & switch
 4. /effort — Set effort level (low/medium/high/max)
 5. /prompt — Custom system prompt
 6. /persona — Switch AI persona
 7. /fallback — Set fallback model
 8. /budget — Budget limit per request
 9. /clear — Clear conversation history
10. /help — This help

🔧 Admin:
11. /restart — Restart bot
12. /config — Configuration
13. /memory — Memory management
14. /task — Background tasks

🤖 Agent Sessions:
• :new [agent] <dir> — Start coding agent
• :quit — Close session
• :status — Session info`, nil

	case "/status":
		stats, err := store.Stats()
		if err != nil {
			return fmt.Sprintf("📊 *Status*\n\n⚠️ Error getting stats: %v", err), nil
		}
		llmStats := llmRouter.Stats()

		var sb strings.Builder
		sb.WriteString("📊 *Magabot Status*\n\n")
		sb.WriteString("🖥️ System:\n")
		sb.WriteString(fmt.Sprintf("  • OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))
		sb.WriteString(fmt.Sprintf("  • Magabot: v%s\n", version.Short()))
		sb.WriteString(fmt.Sprintf("  • Go: %s\n", runtime.Version()))
		sb.WriteString(fmt.Sprintf("  • PID: %d (PPID: %d)\n", os.Getpid(), os.Getppid()))

		srv := util.GetServerStats()
		sb.WriteString("\n💻 Server:\n")
		sb.WriteString(fmt.Sprintf("  • CPU: load %.2f / %.2f / %.2f (1/5/15m)\n", srv.LoadAvg1, srv.LoadAvg5, srv.LoadAvg15))
		if srv.MemTotal > 0 {
			memPct := float64(srv.MemUsed) / float64(srv.MemTotal) * 100
			sb.WriteString(fmt.Sprintf("  • Memory: %s / %s (%.0f%%)\n", util.FormatBytes(srv.MemUsed), util.FormatBytes(srv.MemTotal), memPct))
		}
		if srv.DiskTotal > 0 {
			diskPct := float64(srv.DiskUsed) / float64(srv.DiskTotal) * 100
			sb.WriteString(fmt.Sprintf("  • Disk: %s / %s (%.0f%%)\n", util.FormatBytes(srv.DiskUsed), util.FormatBytes(srv.DiskTotal), diskPct))
		}
		if srv.HasGPU {
			gpuMemPct := float64(srv.GPUMemUsed) / float64(srv.GPUMemTotal) * 100
			sb.WriteString(fmt.Sprintf("  • GPU: %s — %s / %s (%.0f%% mem, %d%% util)\n",
				srv.GPUName, util.FormatBytes(srv.GPUMemUsed), util.FormatBytes(srv.GPUMemTotal), gpuMemPct, srv.GPUUtil))
		}

		sb.WriteString("\n🤖 LLM:\n")
		sb.WriteString(fmt.Sprintf("  • Provider: %s\n", llmStats["main"]))
		// Show plan/impl model for the active provider
		var activeCfg *config.LLMProviderConfig
		switch cfg.LLM.Main {
		case "anthropic":
			activeCfg = &cfg.LLM.Anthropic
		case "openai":
			activeCfg = &cfg.LLM.OpenAI
		case "glm":
			activeCfg = &cfg.LLM.GLM
		case "kimi":
			activeCfg = &cfg.LLM.Kimi
		case "minimax":
			activeCfg = &cfg.LLM.MiniMax
		}
		if activeCfg != nil {
			if activeCfg.PlanModel != "" {
				sb.WriteString(fmt.Sprintf("  • Plan Model: %s\n", activeCfg.PlanModel))
			}
			if activeCfg.ImplModel != "" {
				sb.WriteString(fmt.Sprintf("  • Impl Model: %s\n", activeCfg.ImplModel))
			}
			if activeCfg.Effort != "" {
				sb.WriteString(fmt.Sprintf("  • Effort: %s\n", activeCfg.Effort))
			}
		}

		if cli := llmRouter.CLIProvider(); cli != nil {
			if fb := cli.FallbackModel(); fb != "" {
				sb.WriteString(fmt.Sprintf("  • Fallback: %s\n", fb))
			}
			if budget := cli.MaxBudget(); budget > 0 {
				sb.WriteString(fmt.Sprintf("  • Budget: $%.2f/req\n", budget))
			}
		}

		sb.WriteString("\n📡 Platforms:\n")
		userCounts, _ := stats["users"].(map[string]int64)
		if len(userCounts) > 0 {
			for platform, users := range userCounts {
				sb.WriteString(fmt.Sprintf("  • %s — %d users\n", platform, users))
			}
		} else {
			sb.WriteString("  • _no activity yet_\n")
		}

		return sb.String(), nil

	case "/model":
		allModels := llmRouter.ListAllModels(context.Background())
		if len(allModels) == 0 {
			return "❌ No models available", nil
		}

		// Flatten models into numbered list
		type flatModel struct {
			provider string
			model    llm.ModelInfo
		}
		var flat []flatModel
		for provider, models := range allModels {
			for _, m := range models {
				flat = append(flat, flatModel{provider, m})
			}
		}

		// No args: show current model + numbered list
		if len(args) == 0 {
			stats := llmRouter.Stats()
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🤖 *Current:* `%s`", stats["main"]))
			if cli := llmRouter.CLIProvider(); cli != nil {
				if e := cli.Effort(); e != "" {
					sb.WriteString(fmt.Sprintf(" | effort: %s", e))
				}
				if fb := cli.FallbackModel(); fb != "" {
					sb.WriteString(fmt.Sprintf(" | fallback: %s", fb))
				}
			}
			sb.WriteString("\n\n📋 Available models:\n")
			for i, fm := range flat {
				sb.WriteString(fmt.Sprintf("`%d.` `%s`", i+1, fm.model.ID))
				if fm.model.Name != "" && fm.model.Name != fm.model.ID {
					sb.WriteString(fmt.Sprintf(" — %s", fm.model.Name))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n_Switch: /model <number> or /model <name>_")
			return sb.String(), nil
		}

		// With args: switch model by number or name
		selection := strings.Join(args, " ")

		var selectedID string
		// Try as number first
		var idx int
		if n, err := fmt.Sscanf(selection, "%d", &idx); n == 1 && err == nil {
			if idx < 1 || idx > len(flat) {
				return fmt.Sprintf("❌ Invalid number. Choose 1-%d", len(flat)), nil
			}
			selectedID = flat[idx-1].model.ID
		} else {
			// Try as model name/ID
			for _, fm := range flat {
				if strings.EqualFold(fm.model.ID, selection) || strings.EqualFold(fm.model.Name, selection) {
					selectedID = fm.model.ID
					break
				}
			}
			if selectedID == "" {
				return fmt.Sprintf("❌ Model '%s' not found. Use /model to see available models.", selection), nil
			}
		}

		llmRouter.SetModel(selectedID)
		// Persist model to config YAML
		if provider := llmRouter.MainProvider(); provider != "" {
			if err := cfg.PatchYAMLField("llm."+provider+".model", selectedID); err != nil {
				logger.Warn("persist model failed", "error", err)
			}
		}
		return fmt.Sprintf("✅ Model switched to `%s`", selectedID), nil

	case "/effort":
		cli := llmRouter.CLIProvider()
		if cli == nil {
			return "❌ Effort only available for Claude CLI mode", nil
		}
		if len(args) == 0 {
			current := cli.Effort()
			if current == "" {
				current = "default"
			}
			return fmt.Sprintf("⚡ *Effort:* `%s`\n\n"+
				"1. *low* — fast, short answers\n"+
				"2. *medium* — balanced (default)\n"+
				"3. *high* — detailed, slower\n"+
				"4. *max* — maximum (Opus only)\n\n"+
				"_Set: /effort <level> or /effort <number>_\n"+
				"_Reset: /effort reset_", current), nil
		}
		// Support number selection
		switch args[0] {
		case "1":
			args[0] = "low"
		case "2":
			args[0] = "medium"
		case "3":
			args[0] = "high"
		case "4":
			args[0] = "max"
		}
		level := strings.ToLower(args[0])
		switch level {
		case "low", "medium", "high", "max":
			cli.SetEffort(level)
			if provider := llmRouter.MainProvider(); provider != "" {
				if err := cfg.PatchYAMLField("llm."+provider+".effort", level); err != nil {
					logger.Warn("persist effort failed", "error", err)
				}
			}
			return fmt.Sprintf("✅ Effort set to `%s`", level), nil
		case "default", "off", "reset":
			cli.SetEffort("")
			if provider := llmRouter.MainProvider(); provider != "" {
				if err := cfg.PatchYAMLField("llm."+provider+".effort", ""); err != nil {
					logger.Warn("persist effort reset failed", "error", err)
				}
			}
			return "✅ Effort reset to default", nil
		default:
			return "❌ Invalid effort. Options: `low` | `medium` | `high` | `max`", nil
		}

	case "/prompt":
		cli := llmRouter.CLIProvider()
		if cli == nil {
			return "❌ Prompt customization only available for Claude CLI mode", nil
		}
		if len(args) == 0 {
			current := cli.AppendPrompt()
			if current == "" {
				return "📝 *Custom prompt:* _none_\n\n_Set: /prompt <instructions>_\n_Clear: /prompt reset_", nil
			}
			return fmt.Sprintf("📝 *Custom prompt:*\n%s\n\n_Clear: /prompt reset_", current), nil
		}
		if args[0] == "reset" || args[0] == "off" || args[0] == "clear" {
			cli.SetAppendPrompt("")
			return "✅ Custom prompt cleared", nil
		}
		prompt := strings.Join(args, " ")
		cli.SetAppendPrompt(prompt)
		return fmt.Sprintf("✅ Custom prompt set:\n_%s_", prompt), nil

	case "/fallback":
		cli := llmRouter.CLIProvider()
		if cli == nil {
			return "❌ Fallback only available for Claude CLI mode", nil
		}
		if len(args) == 0 {
			current := cli.FallbackModel()
			if current == "" {
				return "🔄 *Fallback model:* _none_\n\n_Set: /fallback <model>_\n_Example: /fallback claude-sonnet-4-6_", nil
			}
			return fmt.Sprintf("🔄 *Fallback model:* `%s`\n\n_Clear: /fallback off_", current), nil
		}
		if args[0] == "off" || args[0] == "reset" || args[0] == "none" {
			cli.SetFallbackModel("")
			if provider := llmRouter.MainProvider(); provider != "" {
				if err := cfg.PatchYAMLField("llm."+provider+".fallback_model", ""); err != nil {
					logger.Warn("persist fallback reset failed", "error", err)
				}
			}
			return "✅ Fallback model disabled", nil
		}
		model := args[0]
		cli.SetFallbackModel(model)
		if provider := llmRouter.MainProvider(); provider != "" {
			if err := cfg.PatchYAMLField("llm."+provider+".fallback_model", model); err != nil {
				logger.Warn("persist fallback failed", "error", err)
			}
		}
		return fmt.Sprintf("✅ Fallback model set to `%s`", model), nil

	case "/budget":
		cli := llmRouter.CLIProvider()
		if cli == nil {
			return "❌ Budget only available for Claude CLI mode", nil
		}
		if len(args) == 0 {
			current := cli.MaxBudget()
			if current <= 0 {
				return "💰 *Budget:* _unlimited_\n\n_Set: /budget <amount>_ (e.g. /budget 5.00)\n_Clear: /budget off_", nil
			}
			return fmt.Sprintf("💰 *Budget:* $%.2f per request\n\n_Clear: /budget off_", current), nil
		}
		if args[0] == "off" || args[0] == "reset" || args[0] == "unlimited" {
			cli.SetMaxBudget(0)
			return "✅ Budget limit removed", nil
		}
		var amount float64
		if _, err := fmt.Sscanf(args[0], "%f", &amount); err != nil || amount <= 0 {
			return "❌ Invalid amount. Example: `/budget 5.00`", nil
		}
		cli.SetMaxBudget(amount)
		return fmt.Sprintf("✅ Budget set to $%.2f per request", amount), nil

	case "/clear":
		sess := sessionMgr.GetOrCreate(msg.Platform, msg.ChatID, msg.UserID)
		sessionMgr.ClearMessages(sess)
		sessionKey := fmt.Sprintf("%s:%s", msg.Platform, msg.ChatID)
		if err := store.ClearConversationHistory(sessionKey); err != nil {
			return fmt.Sprintf("⚠️ History cleared from memory but DB error: %v", err), nil
		}
		return "🗑 Conversation history cleared.", nil

	case "/persona":
		if len(cfg.Personas.List) == 0 {
			return "No personas configured. Add a `personas` section to config.yaml.", nil
		}
		sess := sessionMgr.GetOrCreate(msg.Platform, msg.ChatID, msg.UserID)

		if len(args) == 0 {
			// Show current persona and list available
			currentName, _ := sessionMgr.GetContext(sess, "persona").(string)
			if currentName == "" {
				if p := cfg.GetDefaultPersona(); p != nil {
					currentName = p.Name
				}
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🎭 Active persona: %s\n\n", currentName))
			sb.WriteString("Available personas:\n")
			for i, p := range cfg.Personas.List {
				marker := "  "
				if p.Name == currentName {
					marker = "▸ "
				}
				personality := ""
				if p.Personality != "" {
					personality = fmt.Sprintf("\n     Personality: %s", p.Personality)
				}
				sb.WriteString(fmt.Sprintf("%s%d. %s — %s%s\n", marker, i+1, p.Name, p.Description, personality))
			}
			sb.WriteString("\nSwitch: /persona <name or number>")
			return sb.String(), nil
		}

		// Switch persona by name or number
		var persona *config.Persona
		if num, err := strconv.Atoi(args[0]); err == nil {
			if num >= 1 && num <= len(cfg.Personas.List) {
				persona = &cfg.Personas.List[num-1]
			}
		} else {
			persona = cfg.GetPersona(strings.ToLower(args[0]))
		}
		if persona == nil {
			var names []string
			for i, p := range cfg.Personas.List {
				names = append(names, fmt.Sprintf("%d:%s", i+1, p.Name))
			}
			return fmt.Sprintf("Unknown persona %q. Available: %s", args[0], strings.Join(names, ", ")), nil
		}

		sessionMgr.SetContext(sess, "persona", persona.Name)

		// Clear conversation history when switching persona
		sessionMgr.ClearMessages(sess)
		sessionKey := fmt.Sprintf("%s:%s", msg.Platform, msg.ChatID)
		_ = store.ClearConversationHistory(sessionKey)

		if persona.FirstMessage != "" {
			return fmt.Sprintf("🎭 Switched to %s\n\n%s", persona.Name, persona.FirstMessage), nil
		}
		return fmt.Sprintf("🎭 Switched to %s", persona.Name), nil

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

	case "/restart":
		if !cfg.IsPlatformAdmin(msg.Platform, msg.UserID) {
			return "🔒 Admin access required.", nil
		}
		prompt := confirmMgr.Request(
			msg.Platform, msg.ChatID, msg.UserID,
			"🔄 *Restart Magabot?*\nBot will restart and be briefly offline.",
			2*time.Minute,
			func() (string, error) {
				saveRestartNotify(msg.Platform, msg.ChatID, "restart")
				adminH.ScheduleRestart(3, nil)
				return "✅ Restarting in 3 seconds...", nil
			},
		)
		return prompt, nil

	case "/update":
		if !cfg.IsPlatformAdmin(msg.Platform, msg.UserID) {
			return "🔒 Admin access required.", nil
		}

		u := updater.New(updater.Config{
			RepoOwner:      repoOwner,
			RepoName:       repoName,
			CurrentVersion: version.Short(),
			BinaryName:     "magabot",
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		release, hasUpdate, err := u.CheckUpdate(ctx)
		if err != nil {
			return fmt.Sprintf("❌ Update check failed: %v", err), nil
		}
		if !hasUpdate {
			return fmt.Sprintf("✅ Already up to date! (v%s)", version.Short()), nil
		}

		prompt := confirmMgr.Request(
			msg.Platform, msg.ChatID, msg.UserID,
			fmt.Sprintf("🔄 *Update Available*\n\n📦 %s → %s\n\n📝 %s",
				version.Short(), release.TagName,
				truncateNotes(release.Body, 200)),
			5*time.Minute,
			func() (string, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				if err := u.Update(ctx, release); err != nil {
					return "", fmt.Errorf("update failed: %w", err)
				}
				saveRestartNotify(msg.Platform, msg.ChatID, "update")
				adminH.ScheduleRestart(3, nil)
				return fmt.Sprintf("✅ Updated to %s! Restarting in 3s...", release.TagName), nil
			},
		)
		return prompt, nil

	default:
		return "❓ Unknown command. Try /help", nil
	}
}

// restartNotify holds info for post-restart notification.
type restartNotify struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
	Reason   string `json:"reason"` // "restart" or "update"
}

// restartNotifyPath returns the path to the restart notification file.
// Cannot be a package-level var because dataDir is set in init().
func restartNotifyPath() string {
	return filepath.Join(dataDir, "restart-notify.json")
}

// saveRestartNotify saves notification info to be sent after restart.
func saveRestartNotify(platform, chatID, reason string) {
	n := restartNotify{Platform: platform, ChatID: chatID, Reason: reason}
	data, err := json.Marshal(n)
	if err != nil {
		slog.Warn("marshal restart notify failed", "error", err)
		return
	}
	if err := os.WriteFile(restartNotifyPath(), data, 0600); err != nil {
		slog.Warn("write restart notify failed", "file", restartNotifyPath(), "error", err)
	}
}

// sendRestartNotify sends a post-restart notification if one was saved, then removes the file.
func sendRestartNotify(rtr *router.Router, logger *slog.Logger) {
	data, err := os.ReadFile(restartNotifyPath())
	if err != nil {
		logger.Debug("no restart notify file", "file", restartNotifyPath())
		return
	}
	_ = os.Remove(restartNotifyPath())

	var n restartNotify
	if err := json.Unmarshal(data, &n); err != nil {
		logger.Warn("unmarshal restart notify failed", "error", err, "data", string(data))
		return
	}

	logger.Info("sending post-restart notification", "platform", n.Platform, "chat_id", n.ChatID, "reason", n.Reason)

	msg := fmt.Sprintf("✅ Magabot is back online! (v%s)", version.Short())
	if n.Reason == "update" {
		msg = fmt.Sprintf("✅ Update complete! Magabot v%s is now running.", version.Short())
	}

	if err := rtr.Send(n.Platform, n.ChatID, msg); err != nil {
		logger.Warn("failed to send restart notification", "error", err)
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
		{secrets.KeyClaudeCodeAuthToken, &cfg.LLM.Anthropic.AuthToken, "claude_code_auth_token"},
		{secrets.KeyOpenAIAPIKey, &cfg.LLM.OpenAI.APIKey, "openai_api_key"},
		{secrets.KeyGLMAPIKey, &cfg.LLM.GLM.APIKey, "glm_api_key"},
		{secrets.KeyKimiAPIKey, &cfg.LLM.Kimi.APIKey, "kimi_api_key"},
		{secrets.KeyMiniMaxAPIKey, &cfg.LLM.MiniMax.APIKey, "minimax_api_key"},
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

// Provider registration helpers with URL validation (SSRF protection - A10)

// compatProviderConfig holds config for OpenAI-compatible providers (GLM, Local)
type compatProviderConfig struct {
	name        string
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	baseURL     string
	isLocal     bool // local providers allow localhost/private IPs
	maxRetries  int
	constructor func(apiKey string, opts ...provider.CompatOption) *provider.OpenAICompatibleProvider
}

// registerCompatProvider registers an OpenAI-compatible provider with shared validation logic.
func registerCompatProvider(llmRouter *llm.Router, cfg compatProviderConfig, llmCfg *config.Config) error {
	if cfg.baseURL != "" {
		var err error
		if cfg.isLocal {
			err = util.ValidateLocalBaseURL(cfg.baseURL)
		} else {
			err = util.ValidateBaseURL(cfg.baseURL)
		}
		if err != nil {
			return fmt.Errorf("invalid base URL for %s: %w", cfg.name, err)
		}
	}

	opts := []provider.CompatOption{
		provider.WithDefaultModel(cfg.model),
	}
	if cfg.temperature > 0 {
		opts = append(opts, provider.WithTemperature(cfg.temperature))
	}
	if cfg.maxTokens > 0 {
		opts = append(opts, provider.WithMaxTokens(cfg.maxTokens))
	}
	if cfg.baseURL != "" && !cfg.isLocal {
		opts = append(opts, provider.WithBaseURL(cfg.baseURL))
	}

	var p allm.Provider
	if cfg.isLocal {
		p = provider.Local(cfg.baseURL, opts...)
	} else {
		p = cfg.constructor(cfg.apiKey, opts...)
	}

	// Create client with retry, context window, and input validation options
	clientOpts := []allm.Option{}
	if cfg.model != "" {
		clientOpts = append(clientOpts, allm.WithModel(cfg.model))
	}
	if cfg.maxRetries > 0 {
		clientOpts = append(clientOpts, allm.WithMaxRetries(cfg.maxRetries), allm.WithRetryBaseDelay(1*time.Second))
	}
	if llmCfg.LLM.MaxContextTokens > 0 {
		clientOpts = append(clientOpts, allm.WithMaxContextTokens(llmCfg.LLM.MaxContextTokens))
	}
	if llmCfg.LLM.TruncationStrategy != "" {
		clientOpts = append(clientOpts, allm.WithTruncationStrategy(llmCfg.LLM.TruncationStrategy))
	}
	if llmCfg.LLM.MaxInputLength > 0 {
		clientOpts = append(clientOpts, allm.WithMaxInputLen(llmCfg.LLM.MaxInputLength))
	}

	llmRouter.Register(cfg.name, allm.New(p, clientOpts...))
	return nil
}

func registerAnthropicProvider(llmRouter *llm.Router, cfg *config.Config) error {
	return registerAnthropicCompatProvider(llmRouter, "anthropic", cfg.LLM.Anthropic, cfg)
}

// registerAnthropicCompatProvider registers any provider that uses the Anthropic-compatible API.
// Used by Anthropic, GLM, Kimi, and MiniMax.
func registerAnthropicCompatProvider(llmRouter *llm.Router, name string, ac config.LLMProviderConfig, cfg *config.Config) error {
	// Create client with retry, context window, and input validation options
	clientOpts := []allm.Option{}
	if ac.Model != "" {
		clientOpts = append(clientOpts, allm.WithModel(ac.Model))
	}
	if ac.MaxRetries > 0 {
		clientOpts = append(clientOpts, allm.WithMaxRetries(ac.MaxRetries), allm.WithRetryBaseDelay(1*time.Second))
	}
	if cfg.LLM.MaxContextTokens > 0 {
		clientOpts = append(clientOpts, allm.WithMaxContextTokens(cfg.LLM.MaxContextTokens))
	}
	if cfg.LLM.TruncationStrategy != "" {
		clientOpts = append(clientOpts, allm.WithTruncationStrategy(cfg.LLM.TruncationStrategy))
	}
	if cfg.LLM.MaxInputLength > 0 {
		clientOpts = append(clientOpts, allm.WithMaxInputLen(cfg.LLM.MaxInputLength))
	}

	// CLI mode: use claude command, no API key needed
	// Auto-switch to CLI mode when auth_token is set (backward compat)
	if ac.Mode == "cli" || (ac.AuthToken != "" && ac.APIKey == "") {
		var cliOpts []provider.CLIOption
		if ac.Model != "" {
			cliOpts = append(cliOpts, provider.WithCLIModel(ac.Model))
		}
		if ac.CLIPath != "" {
			cliOpts = append(cliOpts, provider.WithCLIPath(ac.CLIPath))
		}
		allowedTools := ac.AllowedTools
		if len(allowedTools) == 0 {
			allowedTools = defaultCLITools
		}
		cliOpts = append(cliOpts, provider.WithCLIAllowedTools(allowedTools))
		llmRouter.Register(name, allm.New(provider.ClaudeCLI(cliOpts...), clientOpts...))
		return nil
	}

	// API mode (default)
	if ac.BaseURL != "" {
		if err := util.ValidateBaseURL(ac.BaseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
	}

	model := ac.Model
	if model == "" {
		model = ac.ImplModel
	}
	opts := []provider.AnthropicOption{
		provider.WithAnthropicModel(model),
		provider.WithAnthropicMaxTokens(ac.MaxTokens),
		provider.WithAnthropicTemperature(ac.Temperature),
	}
	if ac.BaseURL != "" {
		opts = append(opts, provider.WithAnthropicBaseURL(ac.BaseURL))
	}

	var p allm.Provider
	switch name {
	case "glm":
		p = provider.GLM(ac.APIKey, opts...)
	case "kimi":
		p = provider.Kimi(ac.APIKey, opts...)
	case "minimax":
		p = provider.MiniMax(ac.APIKey, opts...)
	default:
		p = provider.Anthropic(ac.APIKey, opts...)
	}
	llmRouter.Register(name, allm.New(p, clientOpts...))
	return nil
}

func registerOpenAIProvider(llmRouter *llm.Router, cfg *config.Config) error {
	if cfg.LLM.OpenAI.BaseURL != "" {
		if err := util.ValidateBaseURL(cfg.LLM.OpenAI.BaseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
	}

	openaiModel := cfg.LLM.OpenAI.Model
	if openaiModel == "" {
		openaiModel = cfg.LLM.OpenAI.ImplModel
	}
	opts := []provider.OpenAIOption{
		provider.WithOpenAIModel(openaiModel),
		provider.WithOpenAIMaxTokens(cfg.LLM.OpenAI.MaxTokens),
		provider.WithOpenAITemperature(cfg.LLM.OpenAI.Temperature),
	}
	if cfg.LLM.OpenAI.BaseURL != "" {
		opts = append(opts, provider.WithOpenAIBaseURL(cfg.LLM.OpenAI.BaseURL))
	}

	// Create client with retry, context window, and input validation options
	clientOpts := []allm.Option{}
	if cfg.LLM.OpenAI.Model != "" {
		clientOpts = append(clientOpts, allm.WithModel(cfg.LLM.OpenAI.Model))
	}
	if cfg.LLM.OpenAI.MaxRetries > 0 {
		clientOpts = append(clientOpts, allm.WithMaxRetries(cfg.LLM.OpenAI.MaxRetries), allm.WithRetryBaseDelay(1*time.Second))
	}
	if cfg.LLM.MaxContextTokens > 0 {
		clientOpts = append(clientOpts, allm.WithMaxContextTokens(cfg.LLM.MaxContextTokens))
	}
	if cfg.LLM.TruncationStrategy != "" {
		clientOpts = append(clientOpts, allm.WithTruncationStrategy(cfg.LLM.TruncationStrategy))
	}
	if cfg.LLM.MaxInputLength > 0 {
		clientOpts = append(clientOpts, allm.WithMaxInputLen(cfg.LLM.MaxInputLength))
	}

	llmRouter.Register("openai", allm.New(provider.OpenAI(cfg.LLM.OpenAI.APIKey, opts...), clientOpts...))
	return nil
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
			dir = "~"
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

		resolved, resolveErr := agentMgr.ResolveDir(dir)
		if resolveErr != nil {
			return resolveErr.Error(), nil
		}

		sess, err := agentMgr.NewSession(msg.Platform, msg.ChatID, msg.UserID, agentType, resolved)
		if err != nil {
			return fmt.Sprintf("Failed to start agent session: %v", err), nil
		}

		return fmt.Sprintf("Agent session started: %s in %s\nSend messages to interact. Use :quit to end.", sess.Agent, sess.Dir), nil

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
		duration := time.Since(sess.GetStartTime()).Truncate(time.Second)
		idle := time.Since(sess.GetLastActivity()).Truncate(time.Second)
		timeoutInfo := "disabled"
		if !cfg.Agent.SessionTimeout.IsZero() {
			remaining := cfg.Agent.SessionTimeout.Duration() - idle
			if remaining < 0 {
				remaining = 0
			}
			timeoutInfo = fmt.Sprintf("%s (closes in %s)", cfg.Agent.SessionTimeout.Duration(), remaining.Truncate(time.Second))
		}
		return fmt.Sprintf("Agent: %s\nDirectory: %s\nMessages: %d\nDuration: %s\nIdle: %s\nIdle timeout: %s",
			sess.Agent, sess.Dir, sess.GetMsgCount(), duration, idle, timeoutInfo), nil

	default:
		return fmt.Sprintf("Unknown agent command: %s\nAvailable: :new, :quit, :status", cmd), nil
	}
}

// routeToAgent sends a regular message to the active agent session.
func routeToAgent(ctx context.Context, msg *router.Message, agentMgr *agent.Manager, notify func(string), llmRouter *llm.Router) (string, error) {
	sess := agentMgr.GetSession(msg.Platform, msg.ChatID)
	if sess == nil {
		return "", nil
	}

	templates := agent.DefaultTemplates

	// Wrap notify to track last send time so ticker can coordinate.
	var progressMu sync.Mutex
	var lastProgressAt time.Time

	wrappedNotify := func(text string) {
		progressMu.Lock()
		lastProgressAt = time.Now()
		progressMu.Unlock()
		notify(text)
	}

	// Send periodic status updates so user knows agent is still working.
	// Only fires if no tool-use notification was sent recently.
	statusDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-statusDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				progressMu.Lock()
				recentNotify := time.Since(lastProgressAt) < 90*time.Second
				progressMu.Unlock()
				if recentNotify {
					continue // tool notification was sent recently, skip
				}
				elapsed := time.Since(start).Truncate(time.Second)
				t := templates["still_working"]
				if t == "" {
					t = agent.DefaultTemplates["still_working"]
				}
				notify(strings.ReplaceAll(t, "{elapsed}", elapsed.String()))
			}
		}
	}()

	output, err := agentMgr.Execute(ctx, sess, msg.Text, msg.Media, wrappedNotify)
	close(statusDone)

	if err != nil {
		if output != "" {
			return fmt.Sprintf("%s\n\n⚠️ %v", output, err), nil
		}
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

// restoreLLMSettings applies persisted effort and fallback settings from config.
func restoreLLMSettings(llmRouter *llm.Router, cfg *config.Config, logger *slog.Logger) {
	cli := llmRouter.CLIProvider()
	if cli == nil {
		return
	}

	// Determine which provider config to read from
	providerName := llmRouter.MainProvider()
	var pc *config.LLMProviderConfig
	switch providerName {
	case "anthropic":
		pc = &cfg.LLM.Anthropic
	case "glm":
		pc = &cfg.LLM.GLM
	default:
		return // effort/fallback only apply to CLI providers
	}

	if pc.Effort != "" {
		cli.SetEffort(pc.Effort)
		logger.Info("restored effort from config", "effort", pc.Effort)
	}
	if pc.FallbackModel != "" {
		cli.SetFallbackModel(pc.FallbackModel)
		logger.Info("restored fallback model from config", "fallback", pc.FallbackModel)
	}
}
