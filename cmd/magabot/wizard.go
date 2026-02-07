package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/kusa/magabot/internal/secrets"
	"github.com/kusa/magabot/internal/security"
)

// WizardState holds the wizard configuration state
type WizardState struct {
	// Secrets backend
	SecretsBackend string
	VaultAddress   string
	VaultToken     string

	// Security
	EncryptionKey string

	// Platforms
	TelegramEnabled bool
	TelegramToken   string
	TelegramUserID  string

	SlackEnabled      bool
	SlackBotToken     string
	SlackAppToken     string

	WhatsAppEnabled bool

	WebhookEnabled bool
	WebhookPort    int
	WebhookAuth    string
	WebhookToken   string

	// LLM
	LLMDefault       string
	AnthropicEnabled bool
	AnthropicKey     string
	OpenAIEnabled    bool
	OpenAIKey        string
	GeminiEnabled    bool
	GeminiKey        string
}

// RunWizard runs the interactive setup wizard
func RunWizard() {
	reader := bufio.NewReader(os.Stdin)
	state := &WizardState{}

	clearScreen()
	printBanner()

	// Check existing config
	if _, err := os.Stat(configFile); err == nil {
		fmt.Println("⚠️  Existing configuration found!")
		fmt.Println()
		if !askYesNo(reader, "Do you want to overwrite it?", false) {
			fmt.Println("\n👋 Setup cancelled. Your existing config is preserved.")
			return
		}
		fmt.Println()
	}

	// Ensure directories
	ensureDirs()

	// Step 1: Secrets Backend
	step1SecretsBackend(reader, state)

	// Step 2: Platforms
	step2Platforms(reader, state)

	// Step 3: LLM Providers
	step3LLM(reader, state)

	// Step 4: Security
	step4Security(reader, state)

	// Step 5: Generate config
	step5Generate(state)

	// Done!
	printSuccess()
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printBanner() {
	fmt.Print(`
╔══════════════════════════════════════════════════════════════╗
║                                                              ║
║   🤖  M A G A B O T   S E T U P   W I Z A R D              ║
║                                                              ║
║   Lightweight, secure multi-platform chatbot                 ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝
`)
}

func printStep(num int, title string) {
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  STEP %d: %s\n", num, title)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}

func printSuccess() {
	fmt.Print(`
╔══════════════════════════════════════════════════════════════╗
║                                                              ║
║   ✅  SETUP COMPLETE!                                        ║
║                                                              ║
║   Your configuration has been saved to:                      ║
║   ~/.magabot/config.yaml                                     ║
║                                                              ║
║   Next steps:                                                ║
║   1. magabot start     - Start the bot                       ║
║   2. magabot status    - Check if it's running               ║
║   3. magabot log       - View logs                           ║
║                                                              ║
║   Need help? Run: magabot help                               ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝
`)
}

// Step 1: Secrets Backend
func step1SecretsBackend(reader *bufio.Reader, state *WizardState) {
	printStep(1, "SECRETS STORAGE")

	fmt.Println("Where do you want to store sensitive data (API keys, tokens)?")
	fmt.Println()
	fmt.Println("  [1] 📁 Local file (default, recommended for personal use)")
	fmt.Println("      Stored in ~/.magabot/secrets.json with 0600 permissions")
	fmt.Println()
	fmt.Println("  [2] 🔐 HashiCorp Vault (recommended for teams/production)")
	fmt.Println("      Centralized secrets management with audit logging")
	fmt.Println()

	choice := askChoice(reader, "Select option", []string{"1", "2"}, "1")

	if choice == "2" {
		state.SecretsBackend = "vault"
		fmt.Println()
		fmt.Println("📝 HashiCorp Vault Configuration")
		fmt.Println()
		state.VaultAddress = askString(reader, "Vault address", "http://127.0.0.1:8200")
		state.VaultToken = askPassword(reader, "Vault token")

		// Test connection
		fmt.Print("\n🔄 Testing Vault connection... ")
		vault, err := secrets.NewVault(&secrets.VaultConfig{
			Address: state.VaultAddress,
			Token:   state.VaultToken,
		})
		if err != nil {
			fmt.Println("❌ Error:", err)
			fmt.Println("⚠️  Falling back to local storage")
			state.SecretsBackend = "local"
		} else if err := vault.Ping(context.Background()); err != nil {
			fmt.Println("❌ Connection failed:", err)
			fmt.Println("⚠️  Falling back to local storage")
			state.SecretsBackend = "local"
		} else {
			fmt.Println("✅ Connected!")
		}
	} else {
		state.SecretsBackend = "local"
	}
}

// Step 2: Platforms
func step2Platforms(reader *bufio.Reader, state *WizardState) {
	printStep(2, "CHAT PLATFORMS")

	fmt.Println("Which platforms do you want to enable?")
	fmt.Println()

	// Telegram
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  📱 TELEGRAM                                                │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.TelegramEnabled = askYesNo(reader, "Enable Telegram?", true)
	if state.TelegramEnabled {
		fmt.Println()
		fmt.Println("  To get a bot token:")
		fmt.Println("  1. Open Telegram and message @BotFather")
		fmt.Println("  2. Send /newbot and follow the instructions")
		fmt.Println("  3. Copy the token (format: 123456:ABC-DEF...)")
		fmt.Println()
		state.TelegramToken = askPassword(reader, "Bot token")

		fmt.Println()
		fmt.Println("  To find your Telegram user ID:")
		fmt.Println("  Message @userinfobot or @getmyid_bot")
		fmt.Println()
		state.TelegramUserID = askString(reader, "Your Telegram user ID (for whitelist, or leave empty)", "")
	}
	fmt.Println()

	// Slack
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  💼 SLACK                                                   │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.SlackEnabled = askYesNo(reader, "Enable Slack?", false)
	if state.SlackEnabled {
		fmt.Println()
		fmt.Println("  To get Slack tokens:")
		fmt.Println("  1. Go to api.slack.com/apps and create an app")
		fmt.Println("  2. Enable Socket Mode and get App Token (xapp-...)")
		fmt.Println("  3. Add Bot Token Scopes and install to workspace")
		fmt.Println("  4. Copy Bot Token (xoxb-...)")
		fmt.Println()
		state.SlackBotToken = askPassword(reader, "Bot token (xoxb-...)")
		state.SlackAppToken = askPassword(reader, "App token (xapp-...)")
	}
	fmt.Println()

	// WhatsApp
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  💬 WHATSAPP                                                │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.WhatsAppEnabled = askYesNo(reader, "Enable WhatsApp?", false)
	if state.WhatsAppEnabled {
		fmt.Println()
		fmt.Println("  ℹ️  WhatsApp will require QR code scan after starting the bot")
	}
	fmt.Println()

	// Webhook
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  🌐 WEBHOOK (receive HTTP POST)                             │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.WebhookEnabled = askYesNo(reader, "Enable Webhook?", false)
	if state.WebhookEnabled {
		state.WebhookPort = askInt(reader, "Port", 8080)
		state.WebhookAuth = askChoice(reader, "Authentication", []string{"none", "bearer", "hmac"}, "bearer")
		if state.WebhookAuth == "bearer" {
			state.WebhookToken = askPassword(reader, "Bearer token")
		}
	}
}

// Step 3: LLM Providers
func step3LLM(reader *bufio.Reader, state *WizardState) {
	printStep(3, "AI / LLM PROVIDERS")

	fmt.Println("Which AI providers do you want to use?")
	fmt.Println("(You need at least one API key)")
	fmt.Println()

	// Anthropic
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  🟣 ANTHROPIC (Claude)                                      │")
	fmt.Println("│  Models: Claude 3.5 Sonnet, Claude 4 Opus                   │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.AnthropicEnabled = askYesNo(reader, "Enable Anthropic?", true)
	if state.AnthropicEnabled {
		fmt.Println()
		fmt.Println("  Get API key: https://console.anthropic.com/")
		fmt.Println()
		state.AnthropicKey = askPassword(reader, "API key (sk-ant-...)")
	}
	fmt.Println()

	// OpenAI
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  🟢 OPENAI (GPT)                                            │")
	fmt.Println("│  Models: GPT-4o, GPT-4 Turbo                                │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.OpenAIEnabled = askYesNo(reader, "Enable OpenAI?", false)
	if state.OpenAIEnabled {
		fmt.Println()
		fmt.Println("  Get API key: https://platform.openai.com/api-keys")
		fmt.Println()
		state.OpenAIKey = askPassword(reader, "API key (sk-...)")
	}
	fmt.Println()

	// Gemini
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│  🔵 GOOGLE (Gemini)                                         │")
	fmt.Println("│  Models: Gemini 1.5 Pro, Gemini 1.5 Flash                   │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	state.GeminiEnabled = askYesNo(reader, "Enable Gemini?", false)
	if state.GeminiEnabled {
		fmt.Println()
		fmt.Println("  Get API key: https://makersuite.google.com/app/apikey")
		fmt.Println()
		state.GeminiKey = askPassword(reader, "API key")
	}

	// Set default
	if state.AnthropicEnabled {
		state.LLMDefault = "anthropic"
	} else if state.OpenAIEnabled {
		state.LLMDefault = "openai"
	} else if state.GeminiEnabled {
		state.LLMDefault = "gemini"
	}
}

// Step 4: Security
func step4Security(reader *bufio.Reader, state *WizardState) {
	printStep(4, "SECURITY")

	fmt.Println("🔐 Generating encryption key...")
	state.EncryptionKey = security.GenerateKey()
	fmt.Println("✅ Encryption key generated")
	fmt.Println()
	fmt.Println("⚠️  This key is used to encrypt messages and sessions.")
	fmt.Println("    Keep it safe! If lost, encrypted data cannot be recovered.")
}

// Step 5: Generate config
func step5Generate(state *WizardState) {
	printStep(5, "GENERATING CONFIGURATION")

	fmt.Println("📝 Writing configuration...")

	config := generateWizardConfig(state)

	if err := os.WriteFile(configFile, []byte(config), 0600); err != nil {
		fmt.Printf("❌ Failed to write config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Configuration saved to:", configFile)

	// Store secrets if using Vault
	if state.SecretsBackend == "vault" {
		fmt.Println("\n📝 Storing secrets in Vault...")
		storeSecretsInVault(state)
	}
}

// Helper functions

func askString(reader *bufio.Reader, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("  %s: ", prompt)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}

func askPassword(reader *bufio.Reader, prompt string) string {
	fmt.Printf("  %s: ", prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func askYesNo(reader *bufio.Reader, prompt string, defaultVal bool) bool {
	defaultStr := "y/N"
	if defaultVal {
		defaultStr = "Y/n"
	}

	fmt.Printf("  %s [%s]: ", prompt, defaultStr)
	input, _ := reader.ReadString('\n')
	input = strings.ToLower(strings.TrimSpace(input))

	if input == "" {
		return defaultVal
	}
	return input == "y" || input == "yes"
}

func askChoice(reader *bufio.Reader, prompt string, choices []string, defaultVal string) string {
	fmt.Printf("  %s [%s] (default: %s): ", prompt, strings.Join(choices, "/"), defaultVal)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}

	for _, c := range choices {
		if input == c {
			return c
		}
	}
	return defaultVal
}

func askInt(reader *bufio.Reader, prompt string, defaultVal int) int {
	fmt.Printf("  %s [%d]: ", prompt, defaultVal)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}

	val, err := strconv.Atoi(input)
	if err != nil {
		return defaultVal
	}
	return val
}

// parseNumericChoice converts "1", "2", etc. to an int if within range, else 0
func parseNumericChoice(input string, max int) int {
	val, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || val < 1 || val > max {
		return 0
	}
	return val
}

func generateWizardConfig(state *WizardState) string {
	// Build allowed users
	allowedUsers := "[]"
	if state.TelegramUserID != "" {
		// Validate it's a number
		if matched, _ := regexp.MatchString(`^\d+$`, state.TelegramUserID); matched {
			allowedUsers = fmt.Sprintf(`["%s"]`, state.TelegramUserID)
		}
	}

	webhookToken := state.WebhookToken
	if webhookToken == "" {
		webhookToken = security.GenerateKey()[:32]
	}

	return fmt.Sprintf(`# Magabot Configuration
# Generated by setup wizard

# Secrets backend
secrets:
  backend: "%s"
  vault:
    address: "%s"
    mount_path: "secret"
    secret_path: "magabot"

# Security
security:
  encryption_key: "%s"
  allowed_users:
    telegram: %s
  rate_limit:
    messages_per_minute: 30
    commands_per_minute: 10

# Platforms
platforms:
  telegram:
    enabled: %t
    bot_token: "%s"
  
  slack:
    enabled: %t
    bot_token: "%s"
    app_token: "%s"
  
  whatsapp:
    enabled: %t
  
  webhook:
    enabled: %t
    port: %d
    path: "/webhook"
    bind: "127.0.0.1"
    auth_method: "%s"
    bearer_token: "%s"

# Storage
storage:
  database: "%s/magabot.db"
  history_retention: 90
  backup:
    enabled: true
    path: "%s/backups"
    keep_count: 10

# Logging
logging:
  level: "info"
  file: "%s"
  redact_messages: true

# LLM Providers
llm:
  default: "%s"
  fallback_chain: ["anthropic", "openai", "gemini"]
  
  system_prompt: |
    You are a helpful AI assistant. Be concise and friendly.
  
  max_input_length: 10000
  timeout: 60
  rate_limit: 10
  
  anthropic:
    enabled: %t
    api_key: "%s"
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.7
  
  openai:
    enabled: %t
    api_key: "%s"
    model: "gpt-4o"
    max_tokens: 4096
    temperature: 0.7
  
  gemini:
    enabled: %t
    api_key: "%s"
    model: "gemini-1.5-pro"
    max_tokens: 4096
    temperature: 0.7
  
  glm:
    enabled: false
    api_key: ""
    model: "glm-4"

# Tools (all free, no API keys needed)
tools:
  search:
    enabled: true
  maps:
    enabled: true
  weather:
    enabled: true
  scraper:
    enabled: true
  browser:
    enabled: true
    headless: true
`,
		state.SecretsBackend,
		state.VaultAddress,
		state.EncryptionKey,
		allowedUsers,
		state.TelegramEnabled,
		state.TelegramToken,
		state.SlackEnabled,
		state.SlackBotToken,
		state.SlackAppToken,
		state.WhatsAppEnabled,
		state.WebhookEnabled,
		state.WebhookPort,
		state.WebhookAuth,
		webhookToken,
		dataDir,
		dataDir,
		logFile,
		state.LLMDefault,
		state.AnthropicEnabled,
		state.AnthropicKey,
		state.OpenAIEnabled,
		state.OpenAIKey,
		state.GeminiEnabled,
		state.GeminiKey,
	)
}

func storeSecretsInVault(state *WizardState) {
	vault, err := secrets.NewVault(&secrets.VaultConfig{
		Address: state.VaultAddress,
		Token:   state.VaultToken,
	})
	if err != nil {
		fmt.Println("⚠️  Could not connect to Vault:", err)
		return
	}

	ctx := context.Background()

	// Store secrets
	secretsToStore := map[string]string{
		secrets.KeyEncryptionKey: state.EncryptionKey,
	}

	if state.TelegramToken != "" {
		secretsToStore[secrets.KeyTelegramToken] = state.TelegramToken
	}
	if state.SlackBotToken != "" {
		secretsToStore[secrets.KeySlackBotToken] = state.SlackBotToken
	}
	if state.SlackAppToken != "" {
		secretsToStore[secrets.KeySlackAppToken] = state.SlackAppToken
	}
	if state.AnthropicKey != "" {
		secretsToStore[secrets.KeyAnthropicAPIKey] = state.AnthropicKey
	}
	if state.OpenAIKey != "" {
		secretsToStore[secrets.KeyOpenAIAPIKey] = state.OpenAIKey
	}
	if state.GeminiKey != "" {
		secretsToStore[secrets.KeyGeminiAPIKey] = state.GeminiKey
	}

	for key, value := range secretsToStore {
		if err := vault.Set(ctx, key, value); err != nil {
			fmt.Printf("  ⚠️  Failed to store %s: %v\n", key, err)
		} else {
			fmt.Printf("  ✅ Stored %s\n", key)
		}
	}
}
