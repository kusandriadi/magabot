package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/llm"
	"github.com/kusa/magabot/internal/secrets"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/util"
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

Examples:
  magabot setup            # Full wizard
  magabot setup llm        # Setup LLM providers
  magabot setup platform   # Setup chat platform
  magabot setup webhook    # Setup webhook endpoint`)
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
		cfg.Access.GlobalAdmins = util.AddUnique(cfg.Access.GlobalAdmins, userID)
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
		cfg.Access.GlobalAdmins = util.AddUnique(cfg.Access.GlobalAdmins, userID)
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
	fmt.Println("   QR code will appear on first start")

	askRestart(reader)
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

	// Authentication
	fmt.Println()
	fmt.Println("🔐 Authentication")
	fmt.Println("─────────────────")
	fmt.Println("  1. bearer - Bearer token (recommended)")
	fmt.Println("  2. hmac   - HMAC-SHA256 signature")
	fmt.Println("  3. none   - No authentication")
	fmt.Println()

	authChoice := askString(reader, "Auth method", "1")
	var bearerToken, hmacSecret string
	switch authChoice {
	case "2", "hmac":
		cfg.Platforms.Webhook.AuthMethod = "hmac"
		hmacSecret = askString(reader, "HMAC secret (leave empty to generate)", "")
		if hmacSecret == "" {
			hmacSecret = security.GenerateKey()
		}
		cfg.Platforms.Webhook.HMACSecret = hmacSecret
		saveSecret("webhook/hmac_secret", hmacSecret)
		fmt.Printf("   Secret: %s\n", hmacSecret)
	case "3", "none":
		cfg.Platforms.Webhook.AuthMethod = "none"
		fmt.Println("   ⚠️  No authentication - use IP allowlist for security")
	default:
		cfg.Platforms.Webhook.AuthMethod = "bearer"
		bearerToken = askString(reader, "Bearer token (leave empty to generate)", "")
		if bearerToken == "" {
			bearerToken = security.GenerateKey()[:32]
		}
		cfg.Platforms.Webhook.BearerToken = bearerToken
		saveSecret("webhook/bearer_token", bearerToken)
		fmt.Printf("   Token: %s\n", bearerToken)
	}

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
	hasTelegram := strings.Contains(platforms, "1") || strings.Contains(strings.ToLower(platforms), "telegram")
	hasSlack := strings.Contains(platforms, "2") || strings.Contains(strings.ToLower(platforms), "slack")
	hasGitHub := strings.Contains(platforms, "3") || strings.Contains(strings.ToLower(platforms), "github")

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
			cfg.Platforms.Telegram.Admins = util.AddUnique(cfg.Platforms.Telegram.Admins, userID)
			cfg.Platforms.Telegram.AllowedUsers = util.AddUnique(cfg.Platforms.Telegram.AllowedUsers, userID)
			cfg.Access.GlobalAdmins = util.AddUnique(cfg.Access.GlobalAdmins, userID)
			// Auto-add to webhook allowlist
			allowedUsers = append(allowedUsers, "telegram:"+userID)
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
			cfg.Platforms.Slack.Admins = util.AddUnique(cfg.Platforms.Slack.Admins, slackUserID)
			cfg.Platforms.Slack.AllowedUsers = util.AddUnique(cfg.Platforms.Slack.AllowedUsers, slackUserID)
			// Auto-add to webhook allowlist
			allowedUsers = append(allowedUsers, "slack:"+slackUserID)
		}
	}

	// Configure GitHub
	if hasGitHub {
		fmt.Println()
		fmt.Println("🐙 GitHub Configuration")
		fmt.Println("───────────────────────")
		githubUser := askString(reader, "GitHub username (or * for all)", "*")
		if githubUser != "" {
			allowedUsers = append(allowedUsers, "github:"+githubUser)
		}
	}

	// Additional allowed users
	fmt.Println()
	fmt.Println("👥 Additional Allowed Users (optional)")
	fmt.Println("──────────────────────────────────────")
	fmt.Println("  Format: prefix:id (e.g., grafana, custom:myapp)")
	additionalUsers := askString(reader, "Additional users (comma-separated)", "")
	if additionalUsers != "" {
		for _, u := range strings.Split(additionalUsers, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				allowedUsers = append(allowedUsers, u)
			}
		}
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
	fmt.Printf("   Auth: %s\n", cfg.Platforms.Webhook.AuthMethod)
	if len(allowedUsers) > 0 {
		fmt.Printf("   Allowed: %s\n", strings.Join(allowedUsers, ", "))
	}

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
	fmt.Println("📝 Test with curl:")
	if cfg.Platforms.Webhook.AuthMethod == "bearer" {
		fmt.Printf(`   curl -X POST http://%s:%d%s \
     -H "Authorization: Bearer %s" \
     -H "Content-Type: application/json" \
     -d '{"message": "Hello!", "user_id": "test"}'
`, bind, port, path, bearerToken)
	} else {
		fmt.Printf(`   curl -X POST http://%s:%d%s \
     -H "Content-Type: application/json" \
     -d '{"message": "Hello!", "user_id": "test"}'
`, bind, port, path)
	}

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
	fmt.Println("  2. openai     - GPT-4")
	fmt.Println("  3. gemini     - Google Gemini")
	fmt.Println("  4. deepseek   - DeepSeek")
	fmt.Println("  5. glm        - Zhipu GLM")
	fmt.Println()

	defaultLLM := askString(reader, "Default provider", "anthropic")
	cfg.LLM.Main = defaultLLM

	fmt.Println()
	if askYesNo(reader, "Configure Anthropic (Claude)?", true) {
		key := askString(reader, "Anthropic API Key (sk-ant-...)", "")
		if key != "" {
			saveSecret("llm/anthropic_api_key", key)
			cfg.LLM.Anthropic.Enabled = true
		}
	}

	fmt.Println()
	if askYesNo(reader, "Configure OpenAI (GPT)?", false) {
		key := askString(reader, "OpenAI API Key (sk-...)", "")
		if key != "" {
			saveSecret("llm/openai_api_key", key)
			cfg.LLM.OpenAI.Enabled = true
		}
	}

	fmt.Println()
	if askYesNo(reader, "Configure Google Gemini?", false) {
		key := askString(reader, "Google API Key", "")
		if key != "" {
			saveSecret("llm/gemini_api_key", key)
			cfg.LLM.Gemini.Enabled = true
		}
	}

	fmt.Println()
	if askYesNo(reader, "Configure DeepSeek?", false) {
		key := askString(reader, "DeepSeek API Key", "")
		if key != "" {
			saveSecret("llm/deepseek_api_key", key)
			cfg.LLM.DeepSeek.Enabled = true
		}
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ LLM providers configured!")

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

// askModel fetches available models from the provider API (using the given key),
// shows a numbered list, and lets the user pick. If the API call fails, shows the
// error and asks the user to enter a model name manually.
func askModel(reader *bufio.Reader, provider, apiKey, baseURL, defaultModel string) string {
	fmt.Println()
	fmt.Print("🔄 Fetching available models... ")
	models, err := llm.FetchModels(provider, apiKey, baseURL)
	if err != nil {
		fmt.Println("failed")
		fmt.Printf("❌ Could not fetch models: %v\n", err)
		fmt.Println("   Please verify your API key is valid and has access to list models.")
		fmt.Println()
		return askString(reader, "Model (enter manually)", defaultModel)
	}
	fmt.Println("done")

	fmt.Println()
	fmt.Println("Available models:")
	for i, m := range models {
		desc := ""
		if m.Description != "" {
			desc = " - " + m.Description
		}
		marker := " "
		if m.ID == defaultModel {
			marker = "*"
		}
		fmt.Printf("  %s%d. %s%s\n", marker, i+1, m.ID, desc)
	}
	fmt.Println()

	input := askString(reader, "Model (number or name)", defaultModel)

	// Try numeric selection
	if idx := parseNumericChoice(input, len(models)); idx > 0 {
		return models[idx-1].ID
	}

	return input
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
