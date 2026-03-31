package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/llm"
	"github.com/kusa/magabot/internal/secrets"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/util"
	"github.com/kusandriadi/allm-go/provider"
	"github.com/mdp/qrterminal/v3"
)

// cmdSetup handles setup commands
func cmdSetup() {
	if len(os.Args) < 3 {
		// No subcommand - run full wizard
		RunWizard()
		return
	}

	subCmd := strings.ToLower(os.Args[2])

	switch subCmd {
	case "llm":
		setupLLM()
	case "platform":
		setupPlatform()
	case "webhook":
		setupWebhook()
	case "voice":
		setupVoice()
	default:
		fmt.Printf("Unknown setup target: %s\n\n", subCmd)
		printSetupUsage()
	}
}

func printSetupUsage() {
	fmt.Println(`Magabot Setup

Usage: magabot setup [target]

Targets:
  (none)      Run full interactive wizard
  llm         Configure LLM providers
  platform    Configure chat platform (Telegram/Discord/Slack/WhatsApp)
  webhook     Configure webhook endpoint
  voice       Install voice dependencies (faster-whisper, edge-tts, ffmpeg)

Examples:
  magabot setup            # Full wizard
  magabot setup llm        # Setup LLM providers
  magabot setup platform   # Setup chat platform
  magabot setup webhook    # Setup webhook endpoint
  magabot setup voice      # Install voice support`)
}

// setupPlatform asks which platform to configure
func setupPlatform() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n📱 Platform Setup")
	fmt.Println("─────────────────")
	fmt.Println()
	fmt.Println("Which platform do you want to configure?")
	fmt.Println("  1. telegram   - Telegram Bot")
	fmt.Println("  2. discord    - Discord Bot")
	fmt.Println("  3. slack      - Slack App")
	fmt.Println("  4. whatsapp   - WhatsApp (beta)")
	fmt.Println()

	platform := askString(reader, "Platform", "telegram")

	switch strings.ToLower(platform) {
	case "1", "telegram":
		setupTelegram()
	case "2", "discord":
		setupDiscord()
	case "3", "slack":
		setupSlack()
	case "4", "whatsapp":
		setupWhatsApp()
	default:
		fmt.Printf("Unknown platform: %s\n", platform)
	}
}

// setupTelegram configures Telegram
func setupTelegram() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🤖 Telegram Setup")
	fmt.Println("─────────────────")
	fmt.Println()

	// Load existing config
	cfg := loadOrCreateConfig()

	// Enable Telegram
	if cfg.Platforms.Telegram == nil {
		cfg.Platforms.Telegram = &config.TelegramConfig{}
	}
	cfg.Platforms.Telegram.Enabled = true

	// Get token
	fmt.Println("📝 Get your bot token from @BotFather on Telegram")
	fmt.Println("   1. Open Telegram, search for @BotFather")
	fmt.Println("   2. Send /newbot and follow the prompts")
	fmt.Println("   3. Copy the token (format: 123456789:ABCdef...)")
	fmt.Println()

	token := askString(reader, "Bot Token", "")
	if token == "" {
		fmt.Println("❌ Token is required")
		return
	}

	// Save to secrets
	saveSecret("telegram/bot_token", token)

	// Get admin user ID
	fmt.Println()
	fmt.Println("📝 Get your Telegram user ID:")
	fmt.Println("   1. Open Telegram, search for @userinfobot")
	fmt.Println("   2. Send /start - it will show your ID")
	fmt.Println()

	userID := askString(reader, "Your Telegram User ID", "")
	if userID != "" {
		cfg.Platforms.Telegram.Admins = util.AddUnique(cfg.Platforms.Telegram.Admins, userID)
		cfg.Platforms.Telegram.AllowedUsers = util.AddUnique(cfg.Platforms.Telegram.AllowedUsers, userID)
	}

	// Settings
	cfg.Platforms.Telegram.AllowGroups = askYesNo(reader, "Allow group chats?", true)
	cfg.Platforms.Telegram.AllowDMs = askYesNo(reader, "Allow direct messages?", true)

	// Save config
	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Telegram configured!")

	askRestart(reader)
}

// setupDiscord configures Discord
func setupDiscord() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🎮 Discord Setup")
	fmt.Println("────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	if cfg.Platforms.Discord == nil {
		cfg.Platforms.Discord = &config.DiscordConfig{}
	}
	cfg.Platforms.Discord.Enabled = true

	fmt.Println("📝 Create a Discord bot:")
	fmt.Println("   1. Go to https://discord.com/developers/applications")
	fmt.Println("   2. Create New Application → Bot → Reset Token")
	fmt.Println("   3. Enable MESSAGE CONTENT INTENT in Bot settings")
	fmt.Println("   4. Copy the bot token")
	fmt.Println()

	token := askString(reader, "Bot Token", "")
	if token == "" {
		fmt.Println("❌ Token is required")
		return
	}

	saveSecret("discord/bot_token", token)

	fmt.Println()
	fmt.Println("📝 Get your Discord User ID:")
	fmt.Println("   1. Enable Developer Mode in Discord settings")
	fmt.Println("   2. Right-click your name → Copy User ID")
	fmt.Println()

	userID := askString(reader, "Your Discord User ID", "")
	if userID != "" {
		cfg.Platforms.Discord.Admins = util.AddUnique(cfg.Platforms.Discord.Admins, userID)
		cfg.Platforms.Discord.AllowedUsers = util.AddUnique(cfg.Platforms.Discord.AllowedUsers, userID)
	}

	cfg.Platforms.Discord.AllowGroups = askYesNo(reader, "Allow server channels?", true)
	cfg.Platforms.Discord.AllowDMs = askYesNo(reader, "Allow direct messages?", true)
	cfg.Platforms.Discord.Prefix = askString(reader, "Command prefix", "!")

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Discord configured!")
	fmt.Println()
	fmt.Println("📝 Invite your bot to a server:")
	fmt.Println("   Go to OAuth2 → URL Generator → Select 'bot' + 'applications.commands'")
	fmt.Println("   Select permissions: Send Messages, Read Message History, etc.")

	askRestart(reader)
}

// setupSlack configures Slack
func setupSlack() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n💬 Slack Setup")
	fmt.Println("──────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	if cfg.Platforms.Slack == nil {
		cfg.Platforms.Slack = &config.SlackConfig{}
	}
	cfg.Platforms.Slack.Enabled = true

	fmt.Println("📝 Create a Slack app:")
	fmt.Println("   1. Go to https://api.slack.com/apps")
	fmt.Println("   2. Create New App → From scratch")
	fmt.Println("   3. Enable Socket Mode (get App Token xapp-...)")
	fmt.Println("   4. Add Bot Token Scopes: chat:write, app_mentions:read, etc.")
	fmt.Println("   5. Install to Workspace (get Bot Token xoxb-...)")
	fmt.Println()

	botToken := askString(reader, "Bot Token (xoxb-...)", "")
	if botToken == "" {
		fmt.Println("❌ Bot token is required")
		return
	}
	saveSecret("slack/bot_token", botToken)

	appToken := askString(reader, "App Token (xapp-...)", "")
	if appToken == "" {
		fmt.Println("❌ App token is required for Socket Mode")
		return
	}
	saveSecret("slack/app_token", appToken)

	cfg.Platforms.Slack.AllowGroups = askYesNo(reader, "Allow channel messages?", true)
	cfg.Platforms.Slack.AllowDMs = askYesNo(reader, "Allow direct messages?", true)

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Slack configured!")

	askRestart(reader)
}

// setupWhatsApp configures WhatsApp
func setupWhatsApp() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n📱 WhatsApp Setup (Beta)")
	fmt.Println("────────────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	if cfg.Platforms.WhatsApp == nil {
		cfg.Platforms.WhatsApp = &config.WhatsAppConfig{}
	}

	fmt.Println("⚠️  WhatsApp uses whatsmeow (Web client protocol)")
	fmt.Println("   - No official API - may break with updates")
	fmt.Println("   - Requires QR code scan on first run")
	fmt.Println("   - Session stored locally")
	fmt.Println()

	if !askYesNo(reader, "Enable WhatsApp?", false) {
		return
	}

	cfg.Platforms.WhatsApp.Enabled = true
	cfg.Platforms.WhatsApp.AllowGroups = askYesNo(reader, "Allow group chats?", false)
	cfg.Platforms.WhatsApp.AllowDMs = askYesNo(reader, "Allow direct messages?", true)

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ WhatsApp enabled!")

	// Remove stale QR file before start/restart
	qrFile := filepath.Join(cfg.GetPlatformDir("whatsapp"), "qr.txt")
	_ = os.Remove(qrFile)

	// Auto start/restart daemon
	fmt.Println()
	if isRunning() {
		restartDaemon()
	} else {
		cmdStart()
	}

	// Display QR inline for pairing
	waitAndShowQR(cfg)
}

// setupWebhook configures Webhook endpoint
func setupWebhook() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🔗 Webhook Setup")
	fmt.Println("────────────────")
	fmt.Println()
	fmt.Println("  This creates an HTTP endpoint to receive messages from:")
	fmt.Println("  - Telegram (webhook mode instead of polling)")
	fmt.Println("  - Slack (Events API instead of Socket Mode)")
	fmt.Println("  - External services (GitHub, Grafana, scripts, etc.)")
	fmt.Println()

	cfg := loadOrCreateConfig()

	if cfg.Platforms.Webhook == nil {
		cfg.Platforms.Webhook = &config.WebhookConfig{}
	}
	cfg.Platforms.Webhook.Enabled = true

	// Server config
	fmt.Println("📡 Server Configuration")
	fmt.Println("───────────────────────")
	port := askInt(reader, "Port", 8080)
	cfg.Platforms.Webhook.Port = port

	path := askString(reader, "Path", "/webhook")
	cfg.Platforms.Webhook.Path = path

	bind := askString(reader, "Bind address", "127.0.0.1")
	cfg.Platforms.Webhook.Bind = bind

	// Platform integrations
	fmt.Println()
	fmt.Println("📱 Platform Integrations")
	fmt.Println("────────────────────────")
	fmt.Println("  Which platforms will send webhooks? (comma-separated)")
	fmt.Println("  1. telegram - Telegram Bot API")
	fmt.Println("  2. slack    - Slack Events API")
	fmt.Println("  3. github   - GitHub Webhooks")
	fmt.Println("  4. custom   - Other services")
	fmt.Println()

	platforms := askString(reader, "Platforms (e.g., 1,2 or telegram,slack)", "1")

	var allowedUsers []string
	bearerTokens := make(map[string]string) // token -> user_id
	hasTelegram := strings.Contains(platforms, "1") || strings.Contains(strings.ToLower(platforms), "telegram")
	hasSlack := strings.Contains(platforms, "2") || strings.Contains(strings.ToLower(platforms), "slack")
	hasGitHub := strings.Contains(platforms, "3") || strings.Contains(strings.ToLower(platforms), "github")
	hasCustom := strings.Contains(platforms, "4") || strings.Contains(strings.ToLower(platforms), "custom")

	// Configure Telegram webhook
	if hasTelegram {
		fmt.Println()
		fmt.Println("📱 Telegram Configuration")
		fmt.Println("─────────────────────────")

		if cfg.Platforms.Telegram == nil {
			cfg.Platforms.Telegram = &config.TelegramConfig{}
		}
		cfg.Platforms.Telegram.Enabled = true
		cfg.Platforms.Telegram.UseWebhook = true
		cfg.Platforms.Telegram.WebhookPort = port
		cfg.Platforms.Telegram.WebhookPath = "/telegram"
		cfg.Platforms.Telegram.WebhookSecret = security.GenerateKey()[:32]

		token := askString(reader, "Bot Token (from @BotFather)", "")
		if token != "" {
			saveSecret("telegram/bot_token", token)
		}

		userID := askString(reader, "Your Telegram User ID", "")
		if userID != "" {
			tgUser := "telegram:" + userID
			cfg.Platforms.Telegram.Admins = util.AddUnique(cfg.Platforms.Telegram.Admins, userID)
			cfg.Platforms.Telegram.AllowedUsers = util.AddUnique(cfg.Platforms.Telegram.AllowedUsers, userID)
			// Generate unique token for this user
			userToken := security.GenerateKey()[:32]
			bearerTokens[userToken] = tgUser
			allowedUsers = append(allowedUsers, tgUser)
			fmt.Printf("   🔑 Token for %s: %s\n", tgUser, userToken)
		}

		webhookURL := askString(reader, "Public HTTPS URL (e.g., https://yourdomain.com)", "")
		cfg.Platforms.Telegram.WebhookURL = webhookURL
	}

	// Configure Slack webhook
	if hasSlack {
		fmt.Println()
		fmt.Println("💼 Slack Configuration")
		fmt.Println("──────────────────────")

		if cfg.Platforms.Slack == nil {
			cfg.Platforms.Slack = &config.SlackConfig{}
		}
		cfg.Platforms.Slack.Enabled = true
		cfg.Platforms.Slack.UseWebhook = true
		cfg.Platforms.Slack.WebhookPort = port
		cfg.Platforms.Slack.WebhookPath = "/slack/events"

		signingSecret := askString(reader, "Signing Secret (from Basic Information)", "")
		if signingSecret != "" {
			saveSecret("slack/signing_secret", signingSecret)
			cfg.Platforms.Slack.SigningSecret = signingSecret
		}

		botToken := askString(reader, "Bot Token (xoxb-...)", "")
		if botToken != "" {
			saveSecret("slack/bot_token", botToken)
		}

		slackUserID := askString(reader, "Your Slack User ID (e.g., U1234567)", "")
		if slackUserID != "" {
			slackUser := "slack:" + slackUserID
			cfg.Platforms.Slack.Admins = util.AddUnique(cfg.Platforms.Slack.Admins, slackUserID)
			cfg.Platforms.Slack.AllowedUsers = util.AddUnique(cfg.Platforms.Slack.AllowedUsers, slackUserID)
			// Generate unique token for this user
			userToken := security.GenerateKey()[:32]
			bearerTokens[userToken] = slackUser
			allowedUsers = append(allowedUsers, slackUser)
			fmt.Printf("   🔑 Token for %s: %s\n", slackUser, userToken)
		}
	}

	// Configure GitHub
	if hasGitHub {
		fmt.Println()
		fmt.Println("🐙 GitHub Configuration")
		fmt.Println("───────────────────────")
		fmt.Println("  Note: GitHub webhooks use HMAC signing, not bearer tokens")
		githubUser := askString(reader, "GitHub username", "")
		if githubUser != "" {
			ghUser := "github:" + githubUser
			// Generate HMAC secret for GitHub
			ghSecret := security.GenerateKey()[:40]
			if cfg.Platforms.Webhook.HMACUsers == nil {
				cfg.Platforms.Webhook.HMACUsers = make(map[string]string)
			}
			cfg.Platforms.Webhook.HMACUsers[ghSecret] = ghUser
			allowedUsers = append(allowedUsers, ghUser)
			fmt.Printf("   🔑 HMAC Secret for %s: %s\n", ghUser, ghSecret)
			fmt.Println("   Use this secret in GitHub webhook settings")
		}
	}

	// Configure custom services
	if hasCustom {
		fmt.Println()
		fmt.Println("🔧 Custom Service Configuration")
		fmt.Println("────────────────────────────────")
		for {
			customUser := askString(reader, "Service name (e.g., grafana, myapp) or Enter to finish", "")
			if customUser == "" {
				break
			}
			// Generate unique token for this service
			userToken := security.GenerateKey()[:32]
			bearerTokens[userToken] = customUser
			allowedUsers = append(allowedUsers, customUser)
			fmt.Printf("   🔑 Token for %s: %s\n", customUser, userToken)
		}
	}

	// Validate allowlist is not empty
	if len(allowedUsers) == 0 {
		fmt.Println()
		fmt.Println("❌ Error: At least one user/service must be configured")
		fmt.Println("   Webhook requires user allowlist for security")
		return
	}

	// Set auth method and tokens
	if len(bearerTokens) > 0 {
		cfg.Platforms.Webhook.AuthMethod = "bearer"
		cfg.Platforms.Webhook.BearerTokens = bearerTokens
	}
	cfg.Platforms.Webhook.AllowedUsers = allowedUsers

	// Save config
	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	// Summary
	fmt.Println()
	fmt.Println("✅ Webhook configured!")
	fmt.Println("═══════════════════════")
	fmt.Printf("   Endpoint: http://%s:%d%s\n", bind, port, path)
	fmt.Printf("   Auth: bearer (token-per-user)\n")
	fmt.Printf("   Allowed: %s\n", strings.Join(allowedUsers, ", "))

	if hasTelegram {
		fmt.Println()
		fmt.Println("📱 Telegram webhook:")
		fmt.Printf("   Path: /telegram\n")
		fmt.Printf("   After start: magabot telegram set-webhook\n")
	}

	if hasSlack {
		fmt.Println()
		fmt.Println("💼 Slack webhook:")
		fmt.Printf("   Path: /slack/events\n")
		fmt.Printf("   Set Request URL in Slack app settings\n")
	}

	fmt.Println()
	fmt.Println("📝 Usage - each user/service uses their unique token:")
	fmt.Printf(`   curl -X POST http://%s:%d%s \
     -H "Authorization: Bearer <your-token>" \
     -H "Content-Type: application/json" \
     -d '{"message": "Hello!"}'
`, bind, port, path)
	fmt.Println()
	fmt.Println("⚠️  Security notes:")
	fmt.Println("   - Each token maps to one user (token IS the identity)")
	fmt.Println("   - User ID in payload is ignored (can't be spoofed)")
	fmt.Println("   - Store tokens securely, don't share between services")

	askRestart(reader)
}

// setupLLM configures all LLM providers
func setupLLM() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🤖 LLM Providers Setup")
	fmt.Println("──────────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("Which LLM provider do you want as default?")
	fmt.Println("  1. anthropic  - Claude (recommended)")
	fmt.Println("  2. openai     - GPT-5")
	fmt.Println("  3. glm        - Zhipu GLM")
	fmt.Println("  4. kimi       - Moonshot Kimi")
	fmt.Println("  5. minimax    - MiniMax")
	fmt.Println("  6. local      - Self-hosted (Ollama/vLLM)")
	fmt.Println()

	defaultLLM := askString(reader, "Default provider", "anthropic")

	// Map choice to provider name
	providerMap := map[string]string{
		"1": "anthropic", "anthropic": "anthropic",
		"2": "openai", "openai": "openai",
		"3": "glm", "glm": "glm",
		"4": "kimi", "kimi": "kimi",
		"5": "minimax", "minimax": "minimax",
		"6": "local", "local": "local",
	}
	llmProvider := providerMap[strings.ToLower(defaultLLM)]
	if llmProvider == "" {
		llmProvider = "anthropic"
	}
	cfg.LLM.Main = llmProvider

	// Reset all provider configs so old settings don't leak through
	cfg.LLM.Anthropic = config.LLMProviderConfig{}
	cfg.LLM.OpenAI = config.LLMProviderConfig{}
	cfg.LLM.GLM = config.LLMProviderConfig{}
	cfg.LLM.Local = config.LLMProviderConfig{}
	cfg.LLM.Kimi = config.LLMProviderConfig{}
	cfg.LLM.MiniMax = config.LLMProviderConfig{}

	fmt.Println()
	switch llmProvider {
	case "anthropic":
		fmt.Println("  Authentication method:")
		fmt.Println("    1. API Key (sk-ant-api03-...)")
		fmt.Println("    2. Claude Pro/Max subscription (via Claude Code)")
		fmt.Println()
		authMethod := askString(reader, "Choose [1/2]", "1")

		if authMethod == "2" {
			cfg.LLM.Anthropic.Enabled = true
			cfg.LLM.Anthropic.Mode = "cli"
			fmt.Println("  ✅ Claude CLI mode enabled (uses your claude login session)")
		} else {
			key := askString(reader, "Anthropic API Key (sk-ant-...)", "")
			if key != "" {
				saveSecret("llm/anthropic_api_key", key)
				cfg.LLM.Anthropic.Enabled = true
			}
		}

		fmt.Println()
		fmt.Println("  Plan model (used for planning phase):")
		fmt.Printf("    1. %s (recommended)\n", provider.AnthropicOpus)
		fmt.Printf("    2. %s\n", provider.AnthropicSonnet)
		fmt.Printf("    3. %s\n", provider.AnthropicHaiku)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			cfg.LLM.Anthropic.PlanModel = provider.AnthropicSonnet
		case "3":
			cfg.LLM.Anthropic.PlanModel = provider.AnthropicHaiku
		default:
			cfg.LLM.Anthropic.PlanModel = provider.AnthropicOpus
		}

		fmt.Println()
		fmt.Println("  Implementation model (used for coding):")
		fmt.Printf("    1. %s (recommended)\n", provider.AnthropicSonnet)
		fmt.Printf("    2. %s\n", provider.AnthropicOpus)
		fmt.Printf("    3. %s\n", provider.AnthropicHaiku)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			cfg.LLM.Anthropic.ImplModel = provider.AnthropicOpus
		case "3":
			cfg.LLM.Anthropic.ImplModel = provider.AnthropicHaiku
		default:
			cfg.LLM.Anthropic.ImplModel = provider.AnthropicSonnet
		}

		fmt.Println()
		fmt.Println("  Effort level:")
		fmt.Println("    1. high (recommended)")
		fmt.Println("    2. medium")
		fmt.Println("    3. low")
		fmt.Println("    4. max")
		fmt.Println()
		effortChoice := askString(reader, "Effort", "1")
		switch effortChoice {
		case "2":
			cfg.LLM.Anthropic.Effort = "medium"
		case "3":
			cfg.LLM.Anthropic.Effort = "low"
		case "4":
			cfg.LLM.Anthropic.Effort = "max"
		default:
			cfg.LLM.Anthropic.Effort = "high"
		}

	case "openai":
		key := askString(reader, "OpenAI API Key (sk-...)", "")
		if key != "" {
			saveSecret("llm/openai_api_key", key)
			cfg.LLM.OpenAI.Enabled = true
		}

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.OpenAIGPT5)
		fmt.Printf("    2. %s\n", provider.OpenAIGPT5Mini)
		fmt.Printf("    3. %s\n", provider.OpenAIO3)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			cfg.LLM.OpenAI.PlanModel = provider.OpenAIGPT5Mini
		case "3":
			cfg.LLM.OpenAI.PlanModel = provider.OpenAIO3
		default:
			cfg.LLM.OpenAI.PlanModel = provider.OpenAIGPT5
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.OpenAIGPT5)
		fmt.Printf("    2. %s\n", provider.OpenAIGPT5Mini)
		fmt.Printf("    3. %s\n", provider.OpenAIGPT4_1)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			cfg.LLM.OpenAI.ImplModel = provider.OpenAIGPT5Mini
		case "3":
			cfg.LLM.OpenAI.ImplModel = provider.OpenAIGPT4_1
		default:
			cfg.LLM.OpenAI.ImplModel = provider.OpenAIGPT5
		}

	case "glm":
		key := askString(reader, "GLM API Key", "")
		if key == "" {
			fmt.Println("  ❌ API key is required for GLM")
			return
		}
		baseURL := askString(reader, "Base URL (Anthropic-compatible endpoint)", "")
		if baseURL == "" {
			fmt.Println("  ❌ Base URL is required for GLM")
			return
		}
		fmt.Println()
		fmt.Println("  Available models:")
		fmt.Printf("    1. %s\n", provider.GLM5Dot1)
		fmt.Printf("    2. %s\n", provider.GLM5)
		fmt.Printf("    3. %s\n", provider.GLM5Turbo)
		fmt.Printf("    4. %s\n", provider.GLM4Dot7)
		fmt.Printf("    5. %s\n", provider.GLM4Dot6)
		fmt.Println()
		modelChoice := askString(reader, "Choose model [1/2/3/4/5]", "1")
		modelMap := map[string]string{
			"1": provider.GLM5Dot1, "2": provider.GLM5, "3": provider.GLM5Turbo,
			"4": provider.GLM4Dot7, "5": provider.GLM4Dot6,
		}
		model := modelMap[modelChoice]
		if model == "" {
			model = provider.GLM5Dot1
		}
		saveSecret("llm/glm_api_key", key)
		cfg.LLM.GLM.Enabled = true
		cfg.LLM.GLM.BaseURL = baseURL
		cfg.LLM.GLM.Model = model

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.GLM5Dot1)
		fmt.Printf("    2. %s\n", provider.GLM5)
		fmt.Printf("    3. %s\n", provider.GLM4Dot7)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			cfg.LLM.GLM.PlanModel = provider.GLM5
		case "3":
			cfg.LLM.GLM.PlanModel = provider.GLM4Dot7
		default:
			cfg.LLM.GLM.PlanModel = provider.GLM5Dot1
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.GLM5Turbo)
		fmt.Printf("    2. %s\n", provider.GLM5)
		fmt.Printf("    3. %s\n", provider.GLM4Dot7)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			cfg.LLM.GLM.ImplModel = provider.GLM5
		case "3":
			cfg.LLM.GLM.ImplModel = provider.GLM4Dot7
		default:
			cfg.LLM.GLM.ImplModel = provider.GLM5Turbo
		}

	case "kimi":
		key := askString(reader, "Kimi API Key", "")
		if key != "" {
			saveSecret("llm/kimi_api_key", key)
			cfg.LLM.Kimi.Enabled = true
		}

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.KimiK2_5)
		fmt.Printf("    2. %s\n", provider.KimiK2Thinking)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			cfg.LLM.Kimi.PlanModel = provider.KimiK2Thinking
		default:
			cfg.LLM.Kimi.PlanModel = provider.KimiK2_5
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.KimiK2_5)
		fmt.Printf("    2. %s\n", provider.KimiK2TurboPreview)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			cfg.LLM.Kimi.ImplModel = provider.KimiK2TurboPreview
		default:
			cfg.LLM.Kimi.ImplModel = provider.KimiK2_5
		}

	case "minimax":
		key := askString(reader, "MiniMax API Key", "")
		if key != "" {
			saveSecret("llm/minimax_api_key", key)
			cfg.LLM.MiniMax.Enabled = true
		}

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.MiniMaxM2_7)
		fmt.Printf("    2. %s\n", provider.MiniMaxM2_5)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			cfg.LLM.MiniMax.PlanModel = provider.MiniMaxM2_5
		default:
			cfg.LLM.MiniMax.PlanModel = provider.MiniMaxM2_7
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.MiniMaxM2_7)
		fmt.Printf("    2. %s\n", provider.MiniMaxM2_7HighSpeed)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			cfg.LLM.MiniMax.ImplModel = provider.MiniMaxM2_7HighSpeed
		default:
			cfg.LLM.MiniMax.ImplModel = provider.MiniMaxM2_7
		}

	case "local":
		baseURL := askString(reader, "Base URL", "http://localhost:11434/v1")
		model := askString(reader, "Model name", "llama3")
		cfg.LLM.Local.Enabled = true
		cfg.LLM.Local.BaseURL = baseURL
		cfg.LLM.Local.Model = model
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ LLM providers configured!")

	// Test connection to default provider
	fmt.Println()
	fmt.Println("🔄 Testing connection to", cfg.LLM.Main, "...")

	if cfg.LLM.Main == "anthropic" && cfg.LLM.Anthropic.Mode == "cli" {
		fmt.Println("⏭️  Skipping connection test (CLI mode — uses claude login session)")
	} else {
		var apiKey string
		switch cfg.LLM.Main {
		case "anthropic":
			apiKey = cfg.LLM.Anthropic.APIKey
		case "openai":
			apiKey = cfg.LLM.OpenAI.APIKey
		case "glm":
			apiKey = cfg.LLM.GLM.APIKey
		case "kimi":
			apiKey = cfg.LLM.Kimi.APIKey
		case "minimax":
			apiKey = cfg.LLM.MiniMax.APIKey
		}

		if apiKey != "" {
			models, err := llm.FetchModels(cfg.LLM.Main, apiKey, "")
			if err != nil {
				fmt.Printf("❌ Connection failed: %v\n", err)
				fmt.Println("   Check your API key and try again with: magabot setup llm")
			} else {
				fmt.Printf("✅ Connected to %s! (%d models available)\n", cfg.LLM.Main, len(models))
			}
		}
	}

	askRestart(reader)
}

// Helper functions

func loadOrCreateConfig() *config.Config {
	cfg, err := config.Load(configFile)
	if err != nil {
		cfg = &config.Config{}
	}
	return cfg
}

func saveSecret(key, value string) {
	ctx := context.Background()
	mgr, err := secrets.NewFromConfig(nil)
	if err != nil {
		fmt.Printf("⚠️  Warning: could not save secret: %v\n", err)
		return
	}

	fullKey := "magabot/" + key
	if err := mgr.Set(ctx, fullKey, value); err != nil {
		fmt.Printf("⚠️  Warning: could not save secret: %v\n", err)
	}
}

// waitAndShowQR polls for the WhatsApp QR file, displays it inline,
// and watches for pairing success.
func waitAndShowQR(cfg *config.Config) {
	qrFile := filepath.Join(cfg.GetPlatformDir("whatsapp"), "qr.txt")

	fmt.Println()
	fmt.Println("⏳ Waiting for QR code...")

	// Poll for QR file (daemon needs time to start + connect to WhatsApp)
	var code string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(qrFile)
		if err == nil {
			c := strings.TrimSpace(string(data))
			if c != "" {
				code = c
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if code == "" {
		fmt.Println("   No QR code needed — WhatsApp may already be paired.")
		fmt.Println("   Run 'magabot status' to check.")
		return
	}

	showQR(code)

	// Watch for pairing success or QR refresh
	fmt.Println()
	fmt.Print("⏳ Waiting for pairing...")

	pairDeadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(pairDeadline) {
		time.Sleep(time.Second)

		data, err := os.ReadFile(qrFile)
		if err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			// File deleted — could be pairing success or retry gap.
			time.Sleep(5 * time.Second)
			if _, err2 := os.Stat(qrFile); os.IsNotExist(err2) {
				// Still gone — check if platform was disabled (all retries failed)
				cfg2, _ := config.Load(configFile)
				if cfg2 != nil && cfg2.Platforms.WhatsApp != nil && !cfg2.Platforms.WhatsApp.Enabled {
					fmt.Println()
					fmt.Println("❌ QR pairing failed after multiple attempts.")
					fmt.Println("   WhatsApp has been disabled. Run 'magabot setup platform' to try again.")
					return
				}
				fmt.Println()
				fmt.Println("✅ WhatsApp paired successfully!")
				return
			}
			// File reappeared — QR was refreshed
			data, _ = os.ReadFile(qrFile)
		}

		if data != nil {
			newCode := strings.TrimSpace(string(data))
			if newCode != "" && newCode != code {
				code = newCode
				fmt.Println()
				fmt.Println("🔄 QR code expired, scan the new one:")
				showQR(code)
				fmt.Println()
				fmt.Print("⏳ Waiting for pairing...")
			}
		}
	}

	fmt.Println()
	fmt.Println("⏱️  Timed out. Run 'magabot qr' if you still need to pair.")
}

func showQR(code string) {
	fmt.Println()
	fmt.Println("📱 Scan this QR code with WhatsApp:")
	fmt.Println("   Open WhatsApp → Settings → Linked Devices → Link a Device")
	fmt.Println()
	qrterminal.GenerateHalfBlock(code, qrterminal.L, os.Stdout)
}

// askString, askInt, askYesNo are defined in wizard.go

func askRestart(reader *bufio.Reader) {
	fmt.Println()
	if !isRunning() {
		if askYesNo(reader, "Start magabot now?", true) {
			cmdStart()
		}
		return
	}

	if askYesNo(reader, "Restart magabot now?", true) {
		restartDaemon()
	}
}

func restartDaemon() {
	fmt.Println("🔄 Restarting magabot...")

	pid := getPID()
	if pid > 0 {
		// Try graceful reload first (platform-specific)
		if signalReload(pid) {
			time.Sleep(2 * time.Second)
			if processExists(pid) {
				fmt.Println("✅ Configuration reloaded")
				fmt.Println()
				fmt.Println("   Run 'magabot status' to check status")
				fmt.Println("   Run 'magabot logs' to view logs")
				return
			}
		}
	}

	// Full restart
	cmdStop()
	time.Sleep(1 * time.Second)
	cmdStart()
}

// isRunning, processExists, getPID are defined in commands.go / commands_unix.go / commands_windows.go
