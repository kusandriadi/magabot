package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/secrets"
	"github.com/kusa/magabot/internal/security"
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
	case "telegram":
		setupTelegram()
	case "discord":
		setupDiscord()
	case "slack":
		setupSlack()
	case "whatsapp":
		setupWhatsApp()
	case "lark", "feishu":
		setupLark()
	case "webhook":
		setupWebhook()
	case "llm":
		setupLLM()
	case "anthropic":
		setupAnthropic()
	case "openai":
		setupOpenAI()
	case "gemini":
		setupGemini()
	case "deepseek":
		setupDeepSeek()
	case "glm":
		setupGLM()
	case "admin":
		if len(os.Args) >= 4 {
			setupAdmin(os.Args[3])
		} else {
			fmt.Println("Usage: magabot setup admin <user_id>")
		}
	case "paths":
		setupPaths()
	case "skills":
		setupSkills()
	default:
		fmt.Printf("Unknown setup target: %s\n\n", subCmd)
		printSetupUsage()
	}
}

func printSetupUsage() {
	fmt.Println(`Magabot Setup

Usage: magabot setup [target]

Targets:
  (none)          Run full interactive wizard

  Platforms:
    telegram      Configure Telegram bot
    discord       Configure Discord bot
    slack         Configure Slack bot
    whatsapp      Configure WhatsApp (beta)
    lark          Configure Lark/Feishu bot
    webhook       Configure Webhook endpoint

  LLM Providers:
    llm           Configure all LLM settings
    anthropic     Configure Anthropic (Claude)
    openai        Configure OpenAI (GPT)
    gemini        Configure Google Gemini
    deepseek      Configure DeepSeek
    glm           Configure Zhipu GLM

  Other:
    admin <id>    Add global admin by user ID
    paths         Configure data/skills directories
    skills        Configure skills settings

Examples:
  magabot setup                  # Full wizard
  magabot setup telegram         # Setup Telegram only
  magabot setup llm              # Setup LLM providers
  magabot setup admin 287676843  # Add admin`)
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
		cfg.Platforms.Telegram.Admins = addUnique(cfg.Platforms.Telegram.Admins, userID)
		cfg.Platforms.Telegram.AllowedUsers = addUnique(cfg.Platforms.Telegram.AllowedUsers, userID)
		cfg.Access.GlobalAdmins = addUnique(cfg.Access.GlobalAdmins, userID)
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
		cfg.Platforms.Discord.Admins = addUnique(cfg.Platforms.Discord.Admins, userID)
		cfg.Platforms.Discord.AllowedUsers = addUnique(cfg.Platforms.Discord.AllowedUsers, userID)
		cfg.Access.GlobalAdmins = addUnique(cfg.Access.GlobalAdmins, userID)
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

// setupLark configures Lark/Feishu
func setupLark() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🐦 Lark/Feishu Setup")
	fmt.Println("────────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	if cfg.Platforms.Lark == nil {
		cfg.Platforms.Lark = &config.LarkConfig{}
	}
	cfg.Platforms.Lark.Enabled = true

	fmt.Println("📝 Create a Lark bot:")
	fmt.Println("   1. Go to https://open.larksuite.com/")
	fmt.Println("   2. Create an app → Get App ID and App Secret")
	fmt.Println("   3. Add bot capability")
	fmt.Println()

	appID := askString(reader, "App ID", "")
	if appID != "" {
		saveSecret("lark/app_id", appID)
	}

	appSecret := askString(reader, "App Secret", "")
	if appSecret != "" {
		saveSecret("lark/app_secret", appSecret)
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Lark configured!")

	askRestart(reader)
}

// setupWebhook configures Webhook endpoint
func setupWebhook() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🔗 Webhook Setup")
	fmt.Println("────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	if cfg.Platforms.Webhook == nil {
		cfg.Platforms.Webhook = &config.WebhookConfig{}
	}
	cfg.Platforms.Webhook.Enabled = true

	port := askInt(reader, "Port", 8080)
	cfg.Platforms.Webhook.Port = port
	cfg.Platforms.Webhook.Path = askString(reader, "Path", "/webhook")
	cfg.Platforms.Webhook.Bind = askString(reader, "Bind address", "127.0.0.1")

	fmt.Println()
	fmt.Println("Authentication method:")
	fmt.Println("  1. none   - No authentication")
	fmt.Println("  2. bearer - Bearer token")
	fmt.Println("  3. hmac   - HMAC signature")
	fmt.Println()

	auth := askString(reader, "Auth method (none/bearer/hmac)", "bearer")
	cfg.Platforms.Webhook.AuthMethod = auth

	if auth == "bearer" {
		token := askString(reader, "Bearer token (leave empty to generate)", "")
		if token == "" {
			token = security.GenerateKey()[:32]
		}
		saveSecret("webhook/bearer_token", token)
		fmt.Printf("   Token: %s\n", token)
	} else if auth == "hmac" {
		secret := askString(reader, "HMAC secret (leave empty to generate)", "")
		if secret == "" {
			secret = security.GenerateKey()
		}
		saveSecret("webhook/hmac_secret", secret)
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("✅ Webhook configured at http://%s:%d%s\n",
		cfg.Platforms.Webhook.Bind, cfg.Platforms.Webhook.Port, cfg.Platforms.Webhook.Path)

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
	cfg.LLM.Default = defaultLLM

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

// setupAnthropic configures Anthropic only
func setupAnthropic() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🧠 Anthropic (Claude) Setup")
	fmt.Println("───────────────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("📝 Get your API key from https://console.anthropic.com/")
	fmt.Println()

	key := askString(reader, "API Key (sk-ant-...)", "")
	if key == "" {
		fmt.Println("❌ API key is required")
		return
	}

	saveSecret("llm/anthropic_api_key", key)
	cfg.LLM.Anthropic.Enabled = true
	cfg.LLM.Default = "anthropic"

	model := askString(reader, "Model", "claude-sonnet-4-20250514")
	cfg.LLM.Anthropic.Model = model

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Anthropic configured!")

	askRestart(reader)
}

// setupOpenAI configures OpenAI only
func setupOpenAI() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🤖 OpenAI (GPT) Setup")
	fmt.Println("─────────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("📝 Get your API key from https://platform.openai.com/api-keys")
	fmt.Println()

	key := askString(reader, "API Key (sk-...)", "")
	if key == "" {
		fmt.Println("❌ API key is required")
		return
	}

	saveSecret("llm/openai_api_key", key)
	cfg.LLM.OpenAI.Enabled = true

	if cfg.LLM.Default == "" {
		cfg.LLM.Default = "openai"
	}

	model := askString(reader, "Model", "gpt-4o")
	cfg.LLM.OpenAI.Model = model

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ OpenAI configured!")

	askRestart(reader)
}

// setupGemini configures Google Gemini only
func setupGemini() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n💎 Google Gemini Setup")
	fmt.Println("──────────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("📝 Get your API key from https://makersuite.google.com/app/apikey")
	fmt.Println()

	key := askString(reader, "API Key", "")
	if key == "" {
		fmt.Println("❌ API key is required")
		return
	}

	saveSecret("llm/gemini_api_key", key)
	cfg.LLM.Gemini.Enabled = true

	if cfg.LLM.Default == "" {
		cfg.LLM.Default = "gemini"
	}

	model := askString(reader, "Model", "gemini-1.5-pro")
	cfg.LLM.Gemini.Model = model

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Google Gemini configured!")

	askRestart(reader)
}

// setupDeepSeek configures DeepSeek only
func setupDeepSeek() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🔍 DeepSeek Setup")
	fmt.Println("─────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("📝 Get your API key from https://platform.deepseek.com/")
	fmt.Println()

	key := askString(reader, "API Key", "")
	if key == "" {
		fmt.Println("❌ API key is required")
		return
	}

	saveSecret("llm/deepseek_api_key", key)
	cfg.LLM.DeepSeek.Enabled = true

	if cfg.LLM.Default == "" {
		cfg.LLM.Default = "deepseek"
	}

	model := askString(reader, "Model", "deepseek-chat")
	cfg.LLM.DeepSeek.Model = model

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ DeepSeek configured!")

	askRestart(reader)
}

// setupGLM configures Zhipu GLM only
func setupGLM() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🇨🇳 Zhipu GLM Setup")
	fmt.Println("───────────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("📝 Get your API key from https://open.bigmodel.cn/")
	fmt.Println()

	key := askString(reader, "API Key", "")
	if key == "" {
		fmt.Println("❌ API key is required")
		return
	}

	saveSecret("llm/glm_api_key", key)
	cfg.LLM.GLM.Enabled = true

	if cfg.LLM.Default == "" {
		cfg.LLM.Default = "glm"
	}

	model := askString(reader, "Model", "glm-4")
	cfg.LLM.GLM.Model = model

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Zhipu GLM configured!")

	askRestart(reader)
}

// setupAdmin adds a global admin
func setupAdmin(userID string) {
	cfg := loadOrCreateConfig()

	cfg.Access.GlobalAdmins = addUnique(cfg.Access.GlobalAdmins, userID)

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Printf("✅ Added global admin: %s\n", userID)

	reader := bufio.NewReader(os.Stdin)
	askRestart(reader)
}

// setupPaths configures directory paths
func setupPaths() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n📁 Paths Setup")
	fmt.Println("──────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	fmt.Println("Configure directory paths (press Enter for defaults):")
	fmt.Println()

	cfg.Paths.DataDir = askString(reader, "Data directory", cfg.Paths.DataDir)
	cfg.Paths.LogsDir = askString(reader, "Logs directory", cfg.Paths.LogsDir)
	cfg.Paths.MemoryDir = askString(reader, "Memory directory", cfg.Paths.MemoryDir)
	cfg.Paths.DownloadsDir = askString(reader, "Downloads directory", cfg.Paths.DownloadsDir)

	if err := cfg.EnsureDirectories(); err != nil {
		fmt.Printf("⚠️  Warning: %v\n", err)
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Paths configured!")
	cfg.PrintPaths()

	askRestart(reader)
}

// setupSkills configures skills settings
func setupSkills() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\n🧠 Skills Setup")
	fmt.Println("───────────────")
	fmt.Println()

	cfg := loadOrCreateConfig()

	cfg.Skills.Dir = askString(reader, "Skills directory", cfg.Skills.Dir)
	cfg.Skills.AutoReload = askYesNo(reader, "Enable auto-reload?", true)

	// Create directory
	if err := os.MkdirAll(cfg.Skills.Dir, 0755); err != nil {
		fmt.Printf("⚠️  Warning: %v\n", err)
	}

	if err := cfg.Save(); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("✅ Skills directory: %s\n", cfg.Skills.Dir)
	fmt.Printf("   Auto-reload: %v\n", cfg.Skills.AutoReload)

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
				return
			}
		}
	}

	// Full restart
	cmdStop()
	time.Sleep(1 * time.Second)
	cmdStart()
}

func addUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// isRunning, processExists, getPID are defined in commands.go / commands_unix.go / commands_windows.go
