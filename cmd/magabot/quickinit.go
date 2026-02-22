package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kusa/magabot/internal/security"
)

// cmdInit runs the zero-config quick init.
// Auto-detects API keys and tokens from environment variables,
// generates encryption key, and writes a minimal working config.
// Falls back to 1-2 prompts if nothing is detected.
func cmdInit() {
	fmt.Println()
	fmt.Println("  magabot init")
	fmt.Println("  ────────────")
	fmt.Println()

	// Check for existing config
	if _, err := os.Stat(configFile); err == nil {
		fmt.Println("  Config already exists:", configFile)
		fmt.Println("  Run 'magabot setup' to reconfigure, or delete the file first.")
		return
	}

	// Auto-detect everything from environment variables
	detected := detectEnvConfig()

	// If no LLM provider found, ask for one
	if !detected.hasLLM() {
		fmt.Println("  No LLM API keys found in environment.")
		fmt.Println()
		fmt.Println("  Set one of these env vars and re-run, or enter a key now:")
		fmt.Println("    ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN (Claude Pro/Max),")
		fmt.Println("    OPENAI_API_KEY, GEMINI_API_KEY, DEEPSEEK_API_KEY, LOCAL_LLM_URL")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("  API key (or 'local' for self-hosted LLM): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			fmt.Println("\n  No API key provided. Exiting.")
			return
		}

		if input == "local" {
			detected.LocalEnabled = true
			detected.LocalURL = "http://localhost:11434/v1"
		} else if strings.HasPrefix(input, "sk-ant-") {
			detected.AnthropicKey = input
		} else if strings.HasPrefix(input, "sk-") {
			detected.OpenAIKey = input
		} else {
			// Best guess: try as Anthropic key
			detected.AnthropicKey = input
		}
	}

	// Optionally detect Telegram
	if detected.TelegramToken == "" {
		// Quick optional prompt — one line
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("  Telegram bot token (Enter to skip): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			detected.TelegramToken = input
		}
	}

	// Generate encryption key
	detected.EncryptionKey = security.GenerateKey()

	// Ensure directories
	ensureDirs()

	// Write config
	configContent := buildInitConfig(detected)
	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		fmt.Printf("  Failed to write config: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	fmt.Println()
	fmt.Println("  Config written to:", configFile)
	fmt.Println()

	fmt.Println("  Detected:")
	if detected.AnthropicKey != "" || detected.AnthropicAuthToken != "" {
		fmt.Println("    LLM: anthropic (Claude)")
	}
	if detected.OpenAIKey != "" {
		fmt.Println("    LLM: openai (GPT)")
	}
	if detected.GeminiKey != "" {
		fmt.Println("    LLM: gemini")
	}
	if detected.DeepSeekKey != "" {
		fmt.Println("    LLM: deepseek")
	}
	if detected.LocalEnabled {
		fmt.Printf("    LLM: local (%s)\n", detected.LocalURL)
	}
	if detected.TelegramToken != "" {
		fmt.Println("    Platform: telegram")
	}
	if detected.SlackBotToken != "" {
		fmt.Println("    Platform: slack")
	}
	fmt.Println("    Encryption key: generated")

	fmt.Println()
	fmt.Println("  Run 'magabot start' to launch.")
	fmt.Println("  Run 'magabot setup' for detailed configuration.")
	fmt.Println()
}

// envConfig holds auto-detected configuration from environment variables.
type envConfig struct {
	// LLM providers
	AnthropicKey       string
	AnthropicAuthToken string
	OpenAIKey          string
	GeminiKey          string
	DeepSeekKey        string
	GLMKey             string
	LocalEnabled       bool
	LocalURL           string
	LocalModel         string

	// Platforms
	TelegramToken string
	SlackBotToken string
	SlackAppToken string

	// Security
	EncryptionKey string
}

func (e *envConfig) hasLLM() bool {
	return e.AnthropicKey != "" || e.AnthropicAuthToken != "" || e.OpenAIKey != "" || e.GeminiKey != "" ||
		e.DeepSeekKey != "" || e.GLMKey != "" || e.LocalEnabled
}

func (e *envConfig) defaultProvider() string {
	switch {
	case e.AnthropicKey != "" || e.AnthropicAuthToken != "":
		return "anthropic"
	case e.OpenAIKey != "":
		return "openai"
	case e.GeminiKey != "":
		return "gemini"
	case e.DeepSeekKey != "":
		return "deepseek"
	case e.GLMKey != "":
		return "glm"
	case e.LocalEnabled:
		return "local"
	default:
		return ""
	}
}

// detectEnvConfig reads configuration from well-known environment variables.
func detectEnvConfig() *envConfig {
	cfg := &envConfig{
		AnthropicKey:       os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicAuthToken: os.Getenv("ANTHROPIC_AUTH_TOKEN"),
		OpenAIKey:          os.Getenv("OPENAI_API_KEY"),
		GeminiKey:          firstEnv("GEMINI_API_KEY", "GOOGLE_API_KEY"),
		DeepSeekKey:        os.Getenv("DEEPSEEK_API_KEY"),
		GLMKey:             os.Getenv("GLM_API_KEY"),
		TelegramToken:      firstEnv("TELEGRAM_BOT_TOKEN", "TELEGRAM_TOKEN"),
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:      os.Getenv("SLACK_APP_TOKEN"),
	}

	// Detect local LLM from env
	localURL := os.Getenv("LOCAL_LLM_URL")
	if localURL != "" {
		cfg.LocalEnabled = true
		cfg.LocalURL = localURL
	}
	cfg.LocalModel = os.Getenv("LOCAL_LLM_MODEL")

	return cfg
}

// firstEnv returns the first non-empty environment variable value.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// buildInitConfig generates a minimal YAML config from detected settings.
func buildInitConfig(cfg *envConfig) string {
	var b strings.Builder

	b.WriteString("# Magabot configuration (generated by magabot init)\n\n")

	// Security
	b.WriteString("security:\n")
	fmt.Fprintf(&b, "  encryption_key: \"%s\"\n", cfg.EncryptionKey)
	b.WriteString("  rate_limit:\n")
	b.WriteString("    messages_per_minute: 30\n")
	b.WriteString("    commands_per_minute: 10\n\n")

	// LLM
	b.WriteString("llm:\n")
	fmt.Fprintf(&b, "  default: \"%s\"\n", cfg.defaultProvider())
	b.WriteString("  system_prompt: \"You are a helpful AI assistant. Be concise and friendly.\"\n")
	b.WriteString("  max_input_length: 10000\n")
	b.WriteString("  timeout: 60\n")
	b.WriteString("  rate_limit: 10\n\n")

	if cfg.AnthropicKey != "" || cfg.AnthropicAuthToken != "" {
		b.WriteString("  anthropic:\n")
		b.WriteString("    enabled: true\n")
		if cfg.AnthropicAuthToken != "" {
			fmt.Fprintf(&b, "    auth_token: \"%s\"\n", cfg.AnthropicAuthToken)
		} else {
			fmt.Fprintf(&b, "    api_key: \"%s\"\n", cfg.AnthropicKey)
		}
		b.WriteString("    model: \"claude-sonnet-4-20250514\"\n")
		b.WriteString("    max_tokens: 4096\n\n")
	}

	if cfg.OpenAIKey != "" {
		b.WriteString("  openai:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", cfg.OpenAIKey)
		b.WriteString("    model: \"gpt-4o\"\n")
		b.WriteString("    max_tokens: 4096\n\n")
	}

	if cfg.GeminiKey != "" {
		b.WriteString("  gemini:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", cfg.GeminiKey)
		b.WriteString("    model: \"gemini-2.0-flash\"\n")
		b.WriteString("    max_tokens: 4096\n\n")
	}

	if cfg.DeepSeekKey != "" {
		b.WriteString("  deepseek:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", cfg.DeepSeekKey)
		b.WriteString("    model: \"deepseek-chat\"\n")
		b.WriteString("    max_tokens: 4096\n\n")
	}

	if cfg.GLMKey != "" {
		b.WriteString("  glm:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    api_key: \"%s\"\n", cfg.GLMKey)
		b.WriteString("    model: \"glm-4\"\n")
		b.WriteString("    max_tokens: 4096\n\n")
	}

	if cfg.LocalEnabled {
		b.WriteString("  local:\n")
		b.WriteString("    enabled: true\n")
		url := cfg.LocalURL
		if url == "" {
			url = "http://localhost:11434/v1"
		}
		fmt.Fprintf(&b, "    base_url: \"%s\"\n", url)
		model := cfg.LocalModel
		if model == "" {
			model = "llama3"
		}
		fmt.Fprintf(&b, "    model: \"%s\"\n", model)
		b.WriteString("    max_tokens: 4096\n\n")
	}

	// Platforms
	b.WriteString("platforms:\n")

	if cfg.TelegramToken != "" {
		b.WriteString("  telegram:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    bot_token: \"%s\"\n", cfg.TelegramToken)
		b.WriteString("    allow_dms: true\n")
		b.WriteString("    allow_groups: true\n\n")
	}

	if cfg.SlackBotToken != "" {
		b.WriteString("  slack:\n")
		b.WriteString("    enabled: true\n")
		fmt.Fprintf(&b, "    bot_token: \"%s\"\n", cfg.SlackBotToken)
		if cfg.SlackAppToken != "" {
			fmt.Fprintf(&b, "    app_token: \"%s\"\n", cfg.SlackAppToken)
		}
		b.WriteString("    allow_dms: true\n")
		b.WriteString("    allow_groups: true\n\n")
	}

	// Access control (open by default for init — user can tighten later)
	b.WriteString("access:\n")
	b.WriteString("  mode: \"open\"\n\n")

	// Storage
	b.WriteString("storage:\n")
	fmt.Fprintf(&b, "  database: \"%s/magabot.db\"\n\n", dataDir)

	// Logging
	b.WriteString("logging:\n")
	b.WriteString("  level: \"info\"\n")

	return b.String()
}
