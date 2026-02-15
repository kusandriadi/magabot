package main

import (
	"fmt"
	"os"
	"os/exec"

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
	fmt.Printf("  Global Admins: %v\n", cfg.Access.GlobalAdmins)
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
	fmt.Printf("  Default: %s\n", cfg.LLM.MainProvider)
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
		for _, e := range []string{"nano", "vim", "vi"} {
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
		os.MkdirAll(configDir, 0700)
		os.WriteFile(configFile, data, 0600)
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
		fmt.Println("Usage: magabot config admin <add|remove|list> [user_id]")
		os.Exit(1)
	}

	action := os.Args[3]

	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	switch action {
	case "list", "ls":
		fmt.Println("Global Admins:")
		if len(cfg.Access.GlobalAdmins) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, admin := range cfg.Access.GlobalAdmins {
				fmt.Printf("  • %s\n", admin)
			}
		}

	case "add":
		if len(os.Args) < 5 {
			fmt.Println("Usage: magabot config admin add <user_id>")
			os.Exit(1)
		}
		userID := os.Args[4]

		// Add to global admins
		found := false
		for _, admin := range cfg.Access.GlobalAdmins {
			if admin == userID {
				found = true
				break
			}
		}
		if !found {
			cfg.Access.GlobalAdmins = append(cfg.Access.GlobalAdmins, userID)
			cfg.UpdatedBy = "cli"
			if err := cfg.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Added global admin: %s\n", userID)
			fmt.Println("   Run 'magabot restart' to apply changes.")
		} else {
			fmt.Printf("User %s is already a global admin.\n", userID)
		}

	case "remove", "rm":
		if len(os.Args) < 5 {
			fmt.Println("Usage: magabot config admin remove <user_id>")
			os.Exit(1)
		}
		userID := os.Args[4]

		// Remove from global admins
		newAdmins := make([]string, 0, len(cfg.Access.GlobalAdmins))
		found := false
		for _, admin := range cfg.Access.GlobalAdmins {
			if admin == userID {
				found = true
			} else {
				newAdmins = append(newAdmins, admin)
			}
		}

		if found {
			cfg.Access.GlobalAdmins = newAdmins
			cfg.UpdatedBy = "cli"
			if err := cfg.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Removed global admin: %s\n", userID)
			fmt.Println("   Run 'magabot restart' to apply changes.")
		} else {
			fmt.Printf("User %s is not a global admin.\n", userID)
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
  admin <cmd>   Manage global admins
  path          Print config file path
  help          Show this help

Admin Commands:
  admin list              List global admins
  admin add <user_id>     Add a global admin
  admin remove <user_id>  Remove a global admin

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
    global_admins:  Users who can manage all platforms
  cron:             Scheduled jobs

Chat Commands (for platform admins):
  /config status              Show config status
  /config allow user <id>     Allow a user
  /config allow chat <id>     Allow a chat
  /config admin add <id>      Add platform admin
  /config mode <mode>         Set access mode (global admin only)

Note: Config changes via chat trigger auto-restart.

File: ` + configFile)
}

