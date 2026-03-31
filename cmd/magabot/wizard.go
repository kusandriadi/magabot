package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/kusa/magabot/internal/llm"
	"github.com/kusa/magabot/internal/secrets"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusandriadi/allm-go/provider"
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
	AnthropicMode    string // "api" or "cli"
	OpenAIEnabled    bool
	OpenAIKey        string
	GLMEnabled       bool
	GLMKey           string
	KimiEnabled      bool
	KimiKey          string
	MiniMaxEnabled   bool
	MiniMaxKey       string
	LocalEnabled     bool
	LocalBaseURL     string
	LocalModel       string

	PlanModel string
	ImplModel string
	Effort    string
}

// testLLMConnection tests the LLM connection with the configured provider
func testLLMConnection(state *WizardState) {
	fmt.Println("\n🔄 Testing LLM connection...")

	// Determine which provider to test
	providerName := state.LLMDefault
	var apiKey string

	switch providerName {
	case "anthropic":
		if state.AnthropicMode == "cli" {
			fmt.Println("⏭️  Skipping connection test (CLI mode — uses claude login session)")
			return
		}
		apiKey = state.AnthropicKey
	case "openai":
		apiKey = state.OpenAIKey
	case "glm":
		apiKey = state.GLMKey
	case "kimi":
		apiKey = state.KimiKey
	case "minimax":
		apiKey = state.MiniMaxKey
	case "local":
		fmt.Println("⏭️  Skipping connection test for local provider")
		return
	}

	if apiKey == "" {
		fmt.Println("⚠️  No API key provided, skipping connection test")
		return
	}

	// Use FetchModels from internal/llm to test
	models, err := llm.FetchModels(providerName, apiKey, "")
	if err != nil {
		fmt.Printf("❌ Connection failed: %v\n", err)
		fmt.Println("   Check your API key and try again with: magabot setup llm")
		return
	}

	fmt.Printf("✅ Connected to %s! (%d models available)\n", providerName, len(models))
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
			fmt.Println("\n👋 Setup canceled. Your existing config is preserved.")
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

	// Test LLM connection
	testLLMConnection(state)

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
	fmt.Println("  2. openai     - GPT-5")
	fmt.Println("  3. glm        - Zhipu GLM")
	fmt.Println("  4. kimi       - Moonshot Kimi")
	fmt.Println("  5. minimax    - MiniMax")
	fmt.Println("  6. local      - Self-hosted (Ollama/vLLM)")
	fmt.Println()

	mainChoice := askString(reader, "Main provider", "1")

	// Map choice to provider name
	providerMap := map[string]string{
		"1": "anthropic", "anthropic": "anthropic",
		"2": "openai", "openai": "openai",
		"3": "glm", "glm": "glm",
		"4": "kimi", "kimi": "kimi",
		"5": "minimax", "minimax": "minimax",
		"6": "local", "local": "local",
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
		fmt.Println()
		fmt.Println("  Authentication method:")
		fmt.Println("    1. API Key (sk-ant-api03-...)")
		fmt.Println("    2. Claude Pro/Max subscription (via Claude Code)")
		fmt.Println()
		authMethod := askString(reader, "Choose [1/2]", "1")

		if authMethod == "2" {
			state.AnthropicMode = "cli"
			fmt.Println("  ✅ Claude CLI mode enabled (uses your claude login session)")
		} else {
			state.AnthropicMode = "api"
			fmt.Println("  Get API key: https://console.anthropic.com/")
			fmt.Println()
			state.AnthropicKey = askPassword(reader, "API key (sk-ant-...)")
		}

		fmt.Println()
		fmt.Println("  Model selection:")
		fmt.Println("    Plan model (used for planning phase):")
		fmt.Printf("    1. %s (recommended)\n", provider.AnthropicOpus)
		fmt.Printf("    2. %s\n", provider.AnthropicSonnet)
		fmt.Printf("    3. %s\n", provider.AnthropicHaiku)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			state.PlanModel = provider.AnthropicSonnet
		case "3":
			state.PlanModel = provider.AnthropicHaiku
		default:
			state.PlanModel = provider.AnthropicOpus
		}

		fmt.Println()
		fmt.Println("    Implementation model (used for coding):")
		fmt.Printf("    1. %s (recommended)\n", provider.AnthropicSonnet)
		fmt.Printf("    2. %s\n", provider.AnthropicOpus)
		fmt.Printf("    3. %s\n", provider.AnthropicHaiku)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			state.ImplModel = provider.AnthropicOpus
		case "3":
			state.ImplModel = provider.AnthropicHaiku
		default:
			state.ImplModel = provider.AnthropicSonnet
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
			state.Effort = "medium"
		case "3":
			state.Effort = "low"
		case "4":
			state.Effort = "max"
		default:
			state.Effort = "high"
		}

	case "openai":
		state.OpenAIEnabled = true
		fmt.Println("🟢 OpenAI Configuration")
		fmt.Println("───────────────────────")
		fmt.Println("  Get API key: https://platform.openai.com/api-keys")
		fmt.Println()
		state.OpenAIKey = askPassword(reader, "API key (sk-...)")

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.OpenAIGPT5)
		fmt.Printf("    2. %s\n", provider.OpenAIGPT5Mini)
		fmt.Printf("    3. %s\n", provider.OpenAIO3)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			state.PlanModel = provider.OpenAIGPT5Mini
		case "3":
			state.PlanModel = provider.OpenAIO3
		default:
			state.PlanModel = provider.OpenAIGPT5
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
			state.ImplModel = provider.OpenAIGPT5Mini
		case "3":
			state.ImplModel = provider.OpenAIGPT4_1
		default:
			state.ImplModel = provider.OpenAIGPT5
		}

	case "glm":
		state.GLMEnabled = true
		fmt.Println("🇨🇳 Zhipu GLM Configuration")
		fmt.Println("───────────────────────────")
		fmt.Println("  Get API key: https://open.bigmodel.cn/")
		fmt.Println()
		state.GLMKey = askPassword(reader, "API key")

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.GLM5Dot1)
		fmt.Printf("    2. %s\n", provider.GLM5)
		fmt.Printf("    3. %s\n", provider.GLM4Dot7)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			state.PlanModel = provider.GLM5
		case "3":
			state.PlanModel = provider.GLM4Dot7
		default:
			state.PlanModel = provider.GLM5Dot1
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.GLM5Dot1)
		fmt.Printf("    2. %s\n", provider.GLM5Turbo)
		fmt.Printf("    3. %s\n", provider.GLM5)
		fmt.Printf("    4. %s\n", provider.GLM4Dot7)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			state.ImplModel = provider.GLM5Turbo
		case "3":
			state.ImplModel = provider.GLM5
		case "4":
			state.ImplModel = provider.GLM4Dot7
		default:
			state.ImplModel = provider.GLM5Dot1
		}

	case "kimi":
		state.KimiEnabled = true
		fmt.Println("🌙 Moonshot Kimi Configuration")
		fmt.Println("──────────────────────────────")
		fmt.Println("  Get API key: https://platform.moonshot.cn/")
		fmt.Println()
		state.KimiKey = askPassword(reader, "API key")

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.KimiK2_5)
		fmt.Printf("    2. %s\n", provider.KimiK2Thinking)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			state.PlanModel = provider.KimiK2Thinking
		default:
			state.PlanModel = provider.KimiK2_5
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.KimiK2_5)
		fmt.Printf("    2. %s\n", provider.KimiK2TurboPreview)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			state.ImplModel = provider.KimiK2TurboPreview
		default:
			state.ImplModel = provider.KimiK2_5
		}

	case "minimax":
		state.MiniMaxEnabled = true
		fmt.Println("⚡ MiniMax Configuration")
		fmt.Println("────────────────────────")
		fmt.Println("  Get API key: https://platform.minimaxi.com/")
		fmt.Println()
		state.MiniMaxKey = askPassword(reader, "API key")

		fmt.Println()
		fmt.Println("  Plan model:")
		fmt.Printf("    1. %s (recommended)\n", provider.MiniMaxM2_7)
		fmt.Printf("    2. %s\n", provider.MiniMaxM2_5)
		fmt.Println()
		planChoice := askString(reader, "Plan model", "1")
		switch planChoice {
		case "2":
			state.PlanModel = provider.MiniMaxM2_5
		default:
			state.PlanModel = provider.MiniMaxM2_7
		}

		fmt.Println()
		fmt.Println("  Implementation model:")
		fmt.Printf("    1. %s (recommended)\n", provider.MiniMaxM2_7)
		fmt.Printf("    2. %s\n", provider.MiniMaxM2_7HighSpeed)
		fmt.Println()
		implChoice := askString(reader, "Impl model", "1")
		switch implChoice {
		case "2":
			state.ImplModel = provider.MiniMaxM2_7HighSpeed
		default:
			state.ImplModel = provider.MiniMaxM2_7
		}

	case "local":
		state.LocalEnabled = true
		fmt.Println("🖥️  Local LLM Configuration")
		fmt.Println("───────────────────────────")
		fmt.Println("  Supports Ollama, vLLM, llama.cpp, etc.")
		fmt.Println()
		state.LocalBaseURL = askString(reader, "Base URL", "http://localhost:11434/v1")
		state.LocalModel = askString(reader, "Model name", "llama3")
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

func generateWizardConfig(state *WizardState) string {
	var b strings.Builder

	b.WriteString("# Magabot Configuration\n")
	b.WriteString("# Generated by setup wizard\n\n")

	// Secrets backend
	b.WriteString("secrets:\n")
	fmt.Fprintf(&b, "  backend: \"%s\"\n", state.SecretsBackend)
	if state.SecretsBackend == "vault" {
		b.WriteString("  vault:\n")
		fmt.Fprintf(&b, "    address: \"%s\"\n", state.VaultAddress)
		b.WriteString("    mount_path: \"secret\"\n")
		b.WriteString("    secret_path: \"magabot\"\n")
	}
	b.WriteString("\n")

	// Security
	b.WriteString("security:\n")
	fmt.Fprintf(&b, "  encryption_key: \"%s\"\n", state.EncryptionKey)
	if state.TelegramUserID != "" {
		if matched, _ := regexp.MatchString(`^\d+$`, state.TelegramUserID); matched {
			b.WriteString("  allowed_users:\n")
			fmt.Fprintf(&b, "    telegram: [\"%s\"]\n", state.TelegramUserID)
		}
	}
	b.WriteString("  rate_limit:\n")
	b.WriteString("    messages_per_minute: 30\n")
	b.WriteString("    commands_per_minute: 10\n\n")

	// Platforms — only enabled ones
	b.WriteString("platforms:\n")

	if state.TelegramEnabled {
		b.WriteString("  telegram:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    bot_token: \"%s\"\n", state.TelegramToken)
		if state.TelegramMode == "webhook" || state.TelegramMode == "both" {
			b.WriteString("    use_webhook: true\n")
			fmt.Fprintf(&b, "    webhook_url: \"%s\"\n", state.TelegramWebhookURL)
			fmt.Fprintf(&b, "    webhook_port: %d\n", state.TelegramWebhookPort)
			fmt.Fprintf(&b, "    webhook_path: \"%s\"\n", state.TelegramWebhookPath)
			fmt.Fprintf(&b, "    webhook_secret: \"%s\"\n", state.TelegramWebhookSecret)
		}
		b.WriteString("\n")
	}

	if state.DiscordEnabled {
		b.WriteString("  discord:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    token: \"%s\"\n", state.DiscordToken)
		b.WriteString("\n")
	}

	if state.SlackEnabled {
		b.WriteString("  slack:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    bot_token: \"%s\"\n", state.SlackBotToken)
		if state.SlackAppToken != "" {
			fmt.Fprintf(&b, "    app_token: \"%s\"\n", state.SlackAppToken)
		}
		if state.SlackMode == "webhook" || state.SlackMode == "both" {
			b.WriteString("    use_webhook: true\n")
			fmt.Fprintf(&b, "    webhook_port: %d\n", state.SlackWebhookPort)
			fmt.Fprintf(&b, "    webhook_path: \"%s\"\n", state.SlackWebhookPath)
			fmt.Fprintf(&b, "    signing_secret: \"%s\"\n", state.SlackSigningSecret)
		}
		b.WriteString("\n")
	}

	if state.WhatsAppEnabled {
		b.WriteString("  whatsapp:\n")
		b.WriteString("    enabled: true\n\n")
	}

	b.WriteString("\n")

	// Storage
	b.WriteString("storage:\n")
	fmt.Fprintf(&b, "  database: \"%s/magabot.db\"\n", dataDir)
	b.WriteString("  history_retention: 90\n")
	b.WriteString("  backup:\n")
	b.WriteString("    enabled: true\n")
	fmt.Fprintf(&b, "    path: \"%s/backups\"\n", dataDir)
	b.WriteString("    keep_count: 10\n\n")

	// Logging
	b.WriteString("logging:\n")
	b.WriteString("  level: \"info\"\n")
	fmt.Fprintf(&b, "  file: \"%s\"\n", logFile)
	b.WriteString("  redact_messages: true\n\n")

	// Media
	b.WriteString("media:\n")
	b.WriteString("  retention_days: 60\n\n")

	// Session
	b.WriteString("session:\n")
	b.WriteString("  max_history: 200\n\n")

	// LLM — only enabled providers
	b.WriteString("llm:\n")
	fmt.Fprintf(&b, "  default: \"%s\"\n", state.LLMDefault)
	b.WriteString("  system_prompt: |\n")
	b.WriteString("    You are a helpful AI assistant. Be concise and friendly.\n")
	b.WriteString("  max_input_length: 10000\n")
	b.WriteString("  timeout: 2m\n")
	b.WriteString("  max_context_chars: 250000\n")
	b.WriteString("  rate_limit: 10\n\n")

	if state.AnthropicEnabled {
		b.WriteString("  anthropic:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    mode: \"%s\"\n", state.AnthropicMode)
		if state.AnthropicKey != "" {
			fmt.Fprintf(&b, "    api_key: \"%s\"\n", state.AnthropicKey)
		}
		fmt.Fprintf(&b, "    model: \"%s\"\n", provider.AnthropicSonnet)
		fmt.Fprintf(&b, "    plan_model: \"%s\"\n", state.PlanModel)
		fmt.Fprintf(&b, "    impl_model: \"%s\"\n", state.ImplModel)
		if state.Effort != "" {
			fmt.Fprintf(&b, "    effort: \"%s\"\n", state.Effort)
		}
		b.WriteString("    max_tokens: 200000\n")
		b.WriteString("    max_retries: 3\n\n")
	}

	if state.OpenAIEnabled {
		b.WriteString("  openai:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", state.OpenAIKey)
		fmt.Fprintf(&b, "    model: \"%s\"\n", provider.OpenAIGPT5)
		fmt.Fprintf(&b, "    plan_model: \"%s\"\n", state.PlanModel)
		fmt.Fprintf(&b, "    impl_model: \"%s\"\n", state.ImplModel)
		b.WriteString("    max_tokens: 200000\n")
		b.WriteString("    max_retries: 3\n\n")
	}

	if state.GLMEnabled {
		b.WriteString("  glm:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", state.GLMKey)
		fmt.Fprintf(&b, "    model: \"%s\"\n", provider.GLM5Turbo)
		fmt.Fprintf(&b, "    plan_model: \"%s\"\n", state.PlanModel)
		fmt.Fprintf(&b, "    impl_model: \"%s\"\n", state.ImplModel)
		b.WriteString("    max_tokens: 200000\n")
		b.WriteString("    max_retries: 3\n\n")
	}

	if state.KimiEnabled {
		b.WriteString("  kimi:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", state.KimiKey)
		fmt.Fprintf(&b, "    model: \"%s\"\n", provider.KimiK2_5)
		fmt.Fprintf(&b, "    plan_model: \"%s\"\n", state.PlanModel)
		fmt.Fprintf(&b, "    impl_model: \"%s\"\n", state.ImplModel)
		b.WriteString("    max_tokens: 200000\n")
		b.WriteString("    max_retries: 3\n\n")
	}

	if state.MiniMaxEnabled {
		b.WriteString("  minimax:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", state.MiniMaxKey)
		fmt.Fprintf(&b, "    model: \"%s\"\n", provider.MiniMaxM2_7)
		fmt.Fprintf(&b, "    plan_model: \"%s\"\n", state.PlanModel)
		fmt.Fprintf(&b, "    impl_model: \"%s\"\n", state.ImplModel)
		b.WriteString("    max_tokens: 200000\n")
		b.WriteString("    max_retries: 3\n\n")
	}

	if state.LocalEnabled {
		b.WriteString("  local:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    base_url: \"%s\"\n", state.LocalBaseURL)
		fmt.Fprintf(&b, "    model: \"%s\"\n", state.LocalModel)
		b.WriteString("    max_tokens: 200000\n")
		b.WriteString("    max_retries: 3\n\n")
	}

	// Tools
	b.WriteString("# Tools (all free, no API keys needed)\n")
	b.WriteString("tools:\n")
	b.WriteString("  search:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("  maps:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("  weather:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("  scraper:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("  browser:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("    headless: true\n\n")

	// Agent
	b.WriteString("agent:\n")
	b.WriteString("  plan_delegate: true\n")
	b.WriteString("  timeout: 5m\n")
	b.WriteString("  max_retries: 2\n")
	b.WriteString("  session_timeout: 6h\n")
	b.WriteString("  discover_depth: 3\n")

	return b.String()
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
	for key, value := range secretsToStore {
		if err := vault.Set(ctx, key, value); err != nil {
			fmt.Printf("  ⚠️  Failed to store %s: %v\n", key, err)
		} else {
			fmt.Printf("  ✅ Stored %s\n", key)
		}
	}
}
