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
	TelegramEnabled       bool
	TelegramToken         string
	TelegramUserID        string
	TelegramMode          string // "bot", "webhook", "both"
	TelegramWebhookURL    string
	TelegramWebhookPort   int
	TelegramWebhookPath   string
	TelegramWebhookSecret string

	DiscordEnabled bool
	DiscordToken   string
	DiscordUserID  string

	SlackEnabled       bool
	SlackBotToken      string
	SlackAppToken      string
	SlackMode          string // "bot", "webhook", "both"
	SlackWebhookPort   int
	SlackWebhookPath   string
	SlackSigningSecret string

	WhatsAppEnabled bool

	// LLM
	LLMDefault       string
	AnthropicEnabled bool
	AnthropicKey     string
	OpenAIEnabled    bool
	OpenAIKey        string
	GeminiEnabled    bool
	GeminiKey        string
	DeepSeekEnabled  bool
	DeepSeekKey      string
	GLMEnabled       bool
	GLMKey           string
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
	fmt.Println("(Enter numbers separated by comma, e.g., 1,3)")
	fmt.Println()
	fmt.Println("  1. telegram   - Telegram Bot")
	fmt.Println("  2. discord    - Discord Bot")
	fmt.Println("  3. slack      - Slack App")
	fmt.Println("  4. whatsapp   - WhatsApp (beta)")
	fmt.Println()

	choices := askString(reader, "Platforms to enable", "1")
	selected := parseNumberList(choices)

	// Configure selected platforms
	for _, num := range selected {
		switch num {
		case 1:
			state.TelegramEnabled = true
			fmt.Println()
			fmt.Println("📱 Telegram Configuration")
			fmt.Println("─────────────────────────")
			fmt.Println()
			fmt.Println("  Connection mode:")
			fmt.Println("  1. bot     - Long polling (simple, no server needed)")
			fmt.Println("  2. webhook - HTTP webhook (requires public HTTPS URL)")
			fmt.Println("  3. both    - Enable both modes")
			fmt.Println()
			modeChoice := askString(reader, "Mode", "1")
			switch modeChoice {
			case "2", "webhook":
				state.TelegramMode = "webhook"
			case "3", "both":
				state.TelegramMode = "both"
			default:
				state.TelegramMode = "bot"
			}

			fmt.Println()
			fmt.Println("  To get a bot token:")
			fmt.Println("  1. Open Telegram and message @BotFather")
			fmt.Println("  2. Send /newbot and follow the instructions")
			fmt.Println("  3. Copy the token (format: 123456:ABC-DEF...)")
			fmt.Println()
			state.TelegramToken = askPassword(reader, "Bot token")

			// Webhook configuration
			if state.TelegramMode == "webhook" || state.TelegramMode == "both" {
				fmt.Println()
				fmt.Println("  Webhook configuration:")
				fmt.Println("  Your server must be accessible via HTTPS")
				fmt.Println()
				state.TelegramWebhookURL = askString(reader, "Webhook URL (e.g., https://yourdomain.com)", "")
				state.TelegramWebhookPort = askInt(reader, "Local port", 8443)
				state.TelegramWebhookPath = askString(reader, "Path", "/telegram")
				state.TelegramWebhookSecret = security.GenerateKey()[:32]
				fmt.Printf("  Secret: %s\n", state.TelegramWebhookSecret)
			}

			fmt.Println()
			fmt.Println("  To find your Telegram user ID:")
			fmt.Println("  Message @userinfobot or @getmyid_bot")
			fmt.Println()
			state.TelegramUserID = askString(reader, "Your Telegram user ID (optional)", "")

		case 2:
			state.DiscordEnabled = true
			fmt.Println()
			fmt.Println("🎮 Discord Configuration")
			fmt.Println("────────────────────────")
			fmt.Println("  To get a bot token:")
			fmt.Println("  1. Go to discord.com/developers/applications")
			fmt.Println("  2. Create New Application → Bot → Reset Token")
			fmt.Println("  3. Enable MESSAGE CONTENT INTENT")
			fmt.Println()
			state.DiscordToken = askPassword(reader, "Bot token")
			fmt.Println()
			state.DiscordUserID = askString(reader, "Your Discord user ID (optional)", "")

		case 3:
			state.SlackEnabled = true
			fmt.Println()
			fmt.Println("💼 Slack Configuration")
			fmt.Println("──────────────────────")
			fmt.Println()
			fmt.Println("  Connection mode:")
			fmt.Println("  1. bot     - Socket Mode (simple, no server needed)")
			fmt.Println("  2. webhook - Events API (requires public URL)")
			fmt.Println("  3. both    - Enable both modes")
			fmt.Println()
			modeChoice := askString(reader, "Mode", "1")
			switch modeChoice {
			case "2", "webhook":
				state.SlackMode = "webhook"
			case "3", "both":
				state.SlackMode = "both"
			default:
				state.SlackMode = "bot"
			}

			fmt.Println()
			fmt.Println("  To get Slack tokens:")
			fmt.Println("  1. Go to api.slack.com/apps and create an app")
			fmt.Println("  2. Add Bot Token Scopes and install to workspace")
			fmt.Println("  3. Copy Bot Token (xoxb-...)")
			fmt.Println()
			state.SlackBotToken = askPassword(reader, "Bot token (xoxb-...)")

			// Socket Mode (bot mode)
			if state.SlackMode == "bot" || state.SlackMode == "both" {
				fmt.Println()
				fmt.Println("  For Socket Mode, you need an App Token:")
				fmt.Println("  Go to Basic Information → App-Level Tokens → Generate")
				fmt.Println()
				state.SlackAppToken = askPassword(reader, "App token (xapp-...)")
			}

			// Webhook configuration
			if state.SlackMode == "webhook" || state.SlackMode == "both" {
				fmt.Println()
				fmt.Println("  Webhook configuration:")
				fmt.Println("  Go to Basic Information → Copy Signing Secret")
				fmt.Println()
				state.SlackSigningSecret = askPassword(reader, "Signing Secret")
				state.SlackWebhookPort = askInt(reader, "Local port", 3000)
				state.SlackWebhookPath = askString(reader, "Path", "/slack/events")
			}

		case 4:
			state.WhatsAppEnabled = true
			fmt.Println()
			fmt.Println("💬 WhatsApp Configuration")
			fmt.Println("─────────────────────────")
			fmt.Println("  ℹ️  WhatsApp will require QR code scan after starting the bot")

		}
	}
}

// parseNumberList parses "1,2,3" into []int{1,2,3}
func parseNumberList(s string) []int {
	var result []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if num, err := strconv.Atoi(part); err == nil {
			result = append(result, num)
		}
	}
	return result
}

// Step 3: LLM Providers
func step3LLM(reader *bufio.Reader, state *WizardState) {
	printStep(3, "AI / LLM PROVIDERS")

	fmt.Println("Which LLM provider do you want as your main/default?")
	fmt.Println()
	fmt.Println("  1. anthropic  - Claude (recommended)")
	fmt.Println("  2. openai     - GPT-4")
	fmt.Println("  3. gemini     - Google Gemini")
	fmt.Println("  4. deepseek   - DeepSeek")
	fmt.Println("  5. glm        - Zhipu GLM")
	fmt.Println()

	mainChoice := askString(reader, "Main provider", "1")

	// Map choice to provider name
	providerMap := map[string]string{
		"1": "anthropic", "anthropic": "anthropic",
		"2": "openai", "openai": "openai",
		"3": "gemini", "gemini": "gemini",
		"4": "deepseek", "deepseek": "deepseek",
		"5": "glm", "glm": "glm",
	}
	state.LLMDefault = providerMap[strings.ToLower(mainChoice)]
	if state.LLMDefault == "" {
		state.LLMDefault = "anthropic"
	}

	// Configure main provider
	fmt.Println()
	switch state.LLMDefault {
	case "anthropic":
		state.AnthropicEnabled = true
		fmt.Println("🟣 Anthropic Configuration")
		fmt.Println("──────────────────────────")
		fmt.Println("  Get API key: https://console.anthropic.com/")
		fmt.Println()
		state.AnthropicKey = askPassword(reader, "API key (sk-ant-...)")

	case "openai":
		state.OpenAIEnabled = true
		fmt.Println("🟢 OpenAI Configuration")
		fmt.Println("───────────────────────")
		fmt.Println("  Get API key: https://platform.openai.com/api-keys")
		fmt.Println()
		state.OpenAIKey = askPassword(reader, "API key (sk-...)")

	case "gemini":
		state.GeminiEnabled = true
		fmt.Println("🔵 Google Gemini Configuration")
		fmt.Println("──────────────────────────────")
		fmt.Println("  Get API key: https://makersuite.google.com/app/apikey")
		fmt.Println()
		state.GeminiKey = askPassword(reader, "API key")

	case "deepseek":
		state.DeepSeekEnabled = true
		fmt.Println("🔍 DeepSeek Configuration")
		fmt.Println("─────────────────────────")
		fmt.Println("  Get API key: https://platform.deepseek.com/")
		fmt.Println()
		state.DeepSeekKey = askPassword(reader, "API key")

	case "glm":
		state.GLMEnabled = true
		fmt.Println("🇨🇳 Zhipu GLM Configuration")
		fmt.Println("───────────────────────────")
		fmt.Println("  Get API key: https://open.bigmodel.cn/")
		fmt.Println()
		state.GLMKey = askPassword(reader, "API key")
	}

	// Ask about additional providers
	fmt.Println()
	if askYesNo(reader, "Configure additional LLM providers?", false) {
		fmt.Println()
		fmt.Println("Which additional providers? (comma-separated, e.g., 2,3)")
		fmt.Println()
		for name, num := range map[string]string{"anthropic": "1", "openai": "2", "gemini": "3", "deepseek": "4", "glm": "5"} {
			if name != state.LLMDefault {
				fmt.Printf("  %s. %s\n", num, name)
			}
		}
		fmt.Println()
		additional := askString(reader, "Additional providers", "")
		for _, num := range parseNumberList(additional) {
			switch num {
			case 1:
				if !state.AnthropicEnabled {
					state.AnthropicEnabled = true
					fmt.Println()
					fmt.Println("  Get API key: https://console.anthropic.com/")
					state.AnthropicKey = askPassword(reader, "Anthropic API key (sk-ant-...)")
				}
			case 2:
				if !state.OpenAIEnabled {
					state.OpenAIEnabled = true
					fmt.Println()
					fmt.Println("  Get API key: https://platform.openai.com/api-keys")
					state.OpenAIKey = askPassword(reader, "OpenAI API key (sk-...)")
				}
			case 3:
				if !state.GeminiEnabled {
					state.GeminiEnabled = true
					fmt.Println()
					fmt.Println("  Get API key: https://makersuite.google.com/app/apikey")
					state.GeminiKey = askPassword(reader, "Gemini API key")
				}
			case 4:
				if !state.DeepSeekEnabled {
					state.DeepSeekEnabled = true
					fmt.Println()
					fmt.Println("  Get API key: https://platform.deepseek.com/")
					state.DeepSeekKey = askPassword(reader, "DeepSeek API key")
				}
			case 5:
				if !state.GLMEnabled {
					state.GLMEnabled = true
					fmt.Println()
					fmt.Println("  Get API key: https://open.bigmodel.cn/")
					state.GLMKey = askPassword(reader, "GLM API key")
				}
			}
		}
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
    use_webhook: %t
    webhook_url: "%s"
    webhook_port: %d
    webhook_path: "%s"
    webhook_secret: "%s"
  
  discord:
    enabled: %t
    bot_token: "%s"
  
  slack:
    enabled: %t
    bot_token: "%s"
    app_token: "%s"
    use_webhook: %t
    webhook_port: %d
    webhook_path: "%s"
    signing_secret: "%s"
  
  whatsapp:
    enabled: %t

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
    model: "gemini-2.0-flash"
    max_tokens: 4096
    temperature: 0.7
  
  deepseek:
    enabled: %t
    api_key: "%s"
    model: "deepseek-chat"
    max_tokens: 4096
    temperature: 0.7
  
  glm:
    enabled: %t
    api_key: "%s"
    model: "glm-4"
    max_tokens: 4096
    temperature: 0.7

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
		// Telegram
		state.TelegramEnabled,
		state.TelegramToken,
		state.TelegramMode == "webhook" || state.TelegramMode == "both",
		state.TelegramWebhookURL,
		state.TelegramWebhookPort,
		state.TelegramWebhookPath,
		state.TelegramWebhookSecret,
		// Discord
		state.DiscordEnabled,
		state.DiscordToken,
		// Slack
		state.SlackEnabled,
		state.SlackBotToken,
		state.SlackAppToken,
		state.SlackMode == "webhook" || state.SlackMode == "both",
		state.SlackWebhookPort,
		state.SlackWebhookPath,
		state.SlackSigningSecret,
		// WhatsApp
		state.WhatsAppEnabled,
		// Storage/Logging
		dataDir,
		dataDir,
		logFile,
		// LLM
		state.LLMDefault,
		state.AnthropicEnabled,
		state.AnthropicKey,
		state.OpenAIEnabled,
		state.OpenAIKey,
		state.GeminiEnabled,
		state.GeminiKey,
		state.DeepSeekEnabled,
		state.DeepSeekKey,
		state.GLMEnabled,
		state.GLMKey,
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
