package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kusa/magabot/internal/config"
	"gopkg.in/yaml.v3"
)

func cmdConfig() {
	if len(os.Args) < 3 {
		cmdConfigShow()
		return
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "show", "status":
		cmdConfigShow()
	case "edit":
		cmdConfigEdit()
	case "admin":
		cmdConfigAdmin()
	case "path":
		fmt.Println(configFile)
	case "help":
		cmdConfigHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown config command: %s\n", subCmd)
		cmdConfigHelp()
		os.Exit(1)
	}
}

func cmdConfigShow() {
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("📁 Config File:", configFile)
	fmt.Println()

	// Show summary
	fmt.Println("🤖 Bot:")
	fmt.Printf("  Name: %s\n", cfg.Bot.Name)
	fmt.Printf("  Prefix: %s\n", cfg.Bot.Prefix)
	fmt.Println()

	fmt.Println("🔐 Access Control:")
	fmt.Printf("  Mode: %s\n", cfg.Access.Mode)
	fmt.Println()

	fmt.Println("📱 Platforms:")
	if cfg.Platforms.Telegram != nil && cfg.Platforms.Telegram.Enabled {
		fmt.Printf("  Telegram: ✅ (admins: %d, users: %d, chats: %d)\n",
			len(cfg.Platforms.Telegram.Admins),
			len(cfg.Platforms.Telegram.AllowedUsers),
			len(cfg.Platforms.Telegram.AllowedChats))
	} else {
		fmt.Println("  Telegram: ❌")
	}
	if cfg.Platforms.Discord != nil && cfg.Platforms.Discord.Enabled {
		fmt.Printf("  Discord: ✅ (admins: %d, users: %d, chats: %d)\n",
			len(cfg.Platforms.Discord.Admins),
			len(cfg.Platforms.Discord.AllowedUsers),
			len(cfg.Platforms.Discord.AllowedChats))
	} else {
		fmt.Println("  Discord: ❌")
	}
	if cfg.Platforms.Slack != nil && cfg.Platforms.Slack.Enabled {
		fmt.Printf("  Slack: ✅ (admins: %d, users: %d, chats: %d)\n",
			len(cfg.Platforms.Slack.Admins),
			len(cfg.Platforms.Slack.AllowedUsers),
			len(cfg.Platforms.Slack.AllowedChats))
	} else {
		fmt.Println("  Slack: ❌")
	}
	if cfg.Platforms.WhatsApp != nil && cfg.Platforms.WhatsApp.Enabled {
		fmt.Printf("  WhatsApp: ✅ (admins: %d, users: %d, chats: %d)\n",
			len(cfg.Platforms.WhatsApp.Admins),
			len(cfg.Platforms.WhatsApp.AllowedUsers),
			len(cfg.Platforms.WhatsApp.AllowedChats))
	} else {
		fmt.Println("  WhatsApp: ❌")
	}
	fmt.Println()

	fmt.Println("🤖 LLM:")
	fmt.Printf("  Default: %s\n", cfg.LLM.Main)
	fmt.Println()

	if len(cfg.Cron.Jobs) > 0 {
		fmt.Printf("⏰ Cron Jobs: %d\n", len(cfg.Cron.Jobs))
	}

	fmt.Printf("\n📅 Last Updated: %s\n", cfg.LastUpdated.Format("2006-01-02 15:04:05"))
	if cfg.UpdatedBy != "" {
		fmt.Printf("   Updated By: %s\n", cfg.UpdatedBy)
	}
}

func cmdConfigEdit() {
	// Find editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		// Try common editors
		for _, e := range []string{"nano", "vim", "vi", "notepad"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		fmt.Println("No editor found. Set EDITOR environment variable.")
		fmt.Printf("Config file: %s\n", configFile)
		os.Exit(1)
	}

	// Check if config exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Create default config
		cfg := &config.Config{}
		data, _ := yaml.Marshal(cfg)
		_ = os.MkdirAll(configDir, 0700)
		_ = os.WriteFile(configFile, data, 0600)
	}

	cmd := exec.Command(editor, configFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Editor error: %v\n", err)
		os.Exit(1)
	}

	// Validate config after edit
	if _, err := config.Load(configFile); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Config may have errors: %v\n", err)
	} else {
		fmt.Println("✅ Config saved and validated.")
		fmt.Println("   Run 'magabot restart' to apply changes.")
	}
}

func cmdConfigAdmin() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: magabot config admin <platform> <add|remove|list> [user_id]")
		fmt.Println("Platforms: telegram, discord, slack, whatsapp")
		os.Exit(1)
	}

	platform := strings.ToLower(os.Args[3])

	if len(os.Args) < 5 {
		fmt.Printf("Usage: magabot config admin %s <add|remove|list> [user_id]\n", platform)
		os.Exit(1)
	}

	action := os.Args[4]

	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	getAdmins := func() []string {
		switch platform {
		case "telegram":
			if cfg.Platforms.Telegram != nil {
				return cfg.Platforms.Telegram.Admins
			}
		case "discord":
			if cfg.Platforms.Discord != nil {
				return cfg.Platforms.Discord.Admins
			}
		case "slack":
			if cfg.Platforms.Slack != nil {
				return cfg.Platforms.Slack.Admins
			}
		case "whatsapp":
			if cfg.Platforms.WhatsApp != nil {
				return cfg.Platforms.WhatsApp.Admins
			}
		}
		return nil
	}

	setAdmins := func(admins []string) {
		switch platform {
		case "telegram":
			if cfg.Platforms.Telegram != nil {
				cfg.Platforms.Telegram.Admins = admins
			}
		case "discord":
			if cfg.Platforms.Discord != nil {
				cfg.Platforms.Discord.Admins = admins
			}
		case "slack":
			if cfg.Platforms.Slack != nil {
				cfg.Platforms.Slack.Admins = admins
			}
		case "whatsapp":
			if cfg.Platforms.WhatsApp != nil {
				cfg.Platforms.WhatsApp.Admins = admins
			}
		}
	}

	switch action {
	case "list", "ls":
		admins := getAdmins()
		fmt.Printf("%s Admins:\n", strings.ToUpper(platform[:1])+platform[1:])
		if len(admins) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, admin := range admins {
				fmt.Printf("  • %s\n", admin)
			}
		}

	case "add":
		if len(os.Args) < 6 {
			fmt.Printf("Usage: magabot config admin %s add <user_id>\n", platform)
			os.Exit(1)
		}
		userID := os.Args[5]
		admins := getAdmins()
		for _, admin := range admins {
			if admin == userID {
				fmt.Printf("User %s is already a %s admin.\n", userID, platform)
				return
			}
		}
		setAdmins(append(admins, userID))
		cfg.UpdatedBy = "cli"
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Added %s admin: %s\n", platform, userID)
		fmt.Println("   Run 'magabot restart' to apply changes.")

	case "remove", "rm":
		if len(os.Args) < 6 {
			fmt.Printf("Usage: magabot config admin %s remove <user_id>\n", platform)
			os.Exit(1)
		}
		userID := os.Args[5]
		admins := getAdmins()
		newAdmins := make([]string, 0, len(admins))
		found := false
		for _, admin := range admins {
			if admin == userID {
				found = true
			} else {
				newAdmins = append(newAdmins, admin)
			}
		}
		if found {
			setAdmins(newAdmins)
			cfg.UpdatedBy = "cli"
			if err := cfg.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Removed %s admin: %s\n", platform, userID)
			fmt.Println("   Run 'magabot restart' to apply changes.")
		} else {
			fmt.Printf("User %s is not a %s admin.\n", userID, platform)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown action: %s\n", action)
		os.Exit(1)
	}
}

func cmdConfigHelp() {
	fmt.Println(`Configuration Management

Usage: magabot config <command>

Commands:
  show          Show current configuration summary
  edit          Edit config.yaml in $EDITOR
  admin <cmd>   Manage platform admins
  path          Print config file path
  help          Show this help

Admin Commands:
  admin <platform> list              List platform admins
  admin <platform> add <user_id>     Add a platform admin
  admin <platform> remove <user_id>  Remove a platform admin

Config File Structure:
  bot:              Bot name, prefix
  platforms:        Platform configs (telegram, discord, etc.)
    <platform>:
      enabled:      true/false
      token:        API token
      admins:       Platform admins (can manage config via chat)
      allowed_users: Allowed user IDs
      allowed_chats: Allowed group/channel IDs
  llm:              LLM provider settings
  access:           Global access settings
    mode:           allowlist/denylist/open
  cron:             Scheduled jobs

Chat Commands (for platform admins):
  /config status              Show config status
  /config allow user <id>     Allow a user
  /config allow chat <id>     Allow a chat
  /config admin add <id>      Add platform admin
  /config mode <mode>         Set access mode

Note: Config changes via chat trigger auto-restart.

File: ` + configFile)
}
