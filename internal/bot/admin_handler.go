package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/config"
)

// AdminHandler handles admin commands from chat
type AdminHandler struct {
	cfg        *config.Config
	configDir  string
	executable string
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(cfg *config.Config, configDir string) *AdminHandler {
	executable, _ := os.Executable()
	return &AdminHandler{
		cfg:        cfg,
		configDir:  configDir,
		executable: executable,
	}
}

// HandleCommand processes admin commands
// Returns (response, needRestart, error)
func (h *AdminHandler) HandleCommand(platform, userID, chatID string, args []string) (string, bool, error) {
	if len(args) == 0 {
		return h.showHelp(platform, userID), false, nil
	}

	cmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch cmd {
	case "status", "info":
		return h.showStatus(platform, userID), false, nil

	case "admin":
		return h.handleAdmin(platform, userID, subArgs)

	case "allow":
		return h.handleAllow(platform, userID, chatID, subArgs)

	case "remove", "deny":
		return h.handleRemove(platform, userID, subArgs)

	case "mode":
		return h.handleMode(platform, userID, subArgs)

	case "help":
		return h.showHelp(platform, userID), false, nil

	default:
		return fmt.Sprintf("❌ Unknown command: %s\nUse /config help for available commands.", cmd), false, nil
	}
}

// showStatus shows current config status
func (h *AdminHandler) showStatus(platform, userID string) string {
	isGlobalAdmin := h.cfg.IsGlobalAdmin(userID)
	isPlatformAdmin := h.cfg.IsPlatformAdmin(platform, userID)

	var sb strings.Builder
	sb.WriteString("🔐 *Config Status*\n\n")

	// Access info
	sb.WriteString(fmt.Sprintf("Mode: `%s`\n", h.cfg.Access.Mode))
	sb.WriteString(fmt.Sprintf("Your Role: "))
	if isGlobalAdmin {
		sb.WriteString("🌍 Global Admin\n")
	} else if isPlatformAdmin {
		sb.WriteString(fmt.Sprintf("👤 %s Admin\n", platform))
	} else {
		sb.WriteString("User\n")
	}

	// Global admins (only show to global admins)
	if isGlobalAdmin {
		sb.WriteString(fmt.Sprintf("\n*Global Admins (%d):*\n", len(h.cfg.Access.GlobalAdmins)))
		for _, admin := range h.cfg.Access.GlobalAdmins {
			sb.WriteString(fmt.Sprintf("  • `%s`\n", admin))
		}
	}

	// Platform info
	sb.WriteString(fmt.Sprintf("\n*Platform: %s*\n", platform))

	var admins, users, chats []string
	var allowGroups, allowDMs bool

	switch platform {
	case "telegram":
		if h.cfg.Platforms.Telegram != nil {
			admins = h.cfg.Platforms.Telegram.Admins
			users = h.cfg.Platforms.Telegram.AllowedUsers
			chats = h.cfg.Platforms.Telegram.AllowedChats
			allowGroups = h.cfg.Platforms.Telegram.AllowGroups
			allowDMs = h.cfg.Platforms.Telegram.AllowDMs
		}
	case "discord":
		if h.cfg.Platforms.Discord != nil {
			admins = h.cfg.Platforms.Discord.Admins
			users = h.cfg.Platforms.Discord.AllowedUsers
			chats = h.cfg.Platforms.Discord.AllowedChats
			allowGroups = h.cfg.Platforms.Discord.AllowGroups
			allowDMs = h.cfg.Platforms.Discord.AllowDMs
		}
	case "slack":
		if h.cfg.Platforms.Slack != nil {
			admins = h.cfg.Platforms.Slack.Admins
			users = h.cfg.Platforms.Slack.AllowedUsers
			chats = h.cfg.Platforms.Slack.AllowedChats
			allowGroups = h.cfg.Platforms.Slack.AllowGroups
			allowDMs = h.cfg.Platforms.Slack.AllowDMs
		}
	case "whatsapp":
		if h.cfg.Platforms.WhatsApp != nil {
			admins = h.cfg.Platforms.WhatsApp.Admins
			users = h.cfg.Platforms.WhatsApp.AllowedUsers
			chats = h.cfg.Platforms.WhatsApp.AllowedChats
			allowGroups = h.cfg.Platforms.WhatsApp.AllowGroups
			allowDMs = h.cfg.Platforms.WhatsApp.AllowDMs
		}
	}

	groupIcon := "✅"
	if !allowGroups {
		groupIcon = "❌"
	}
	dmIcon := "✅"
	if !allowDMs {
		dmIcon = "❌"
	}

	sb.WriteString(fmt.Sprintf("Groups: %s | DMs: %s\n", groupIcon, dmIcon))
	sb.WriteString(fmt.Sprintf("Admins: %d | Users: %d | Chats: %d\n", len(admins), len(users), len(chats)))

	if isPlatformAdmin && len(admins) > 0 {
		sb.WriteString("\n*Platform Admins:*\n")
		for _, admin := range admins {
			sb.WriteString(fmt.Sprintf("  • `%s`\n", admin))
		}
	}

	sb.WriteString(fmt.Sprintf("\nLast Updated: %s", h.cfg.LastUpdated.Format("2006-01-02 15:04")))

	return sb.String()
}

// handleAdmin manages admin add/remove
func (h *AdminHandler) handleAdmin(platform, userID string, args []string) (string, bool, error) {
	if len(args) < 2 {
		return `*Admin Management*

/config admin add <user_id>    - Add platform admin
/config admin remove <user_id> - Remove platform admin
/config admin global add <id>  - Add global admin
/config admin global rm <id>   - Remove global admin

Note: New admin must be in allowlist first.`, false, nil
	}

	action := strings.ToLower(args[0])
	
	// Global admin management
	if action == "global" && len(args) >= 3 {
		globalAction := strings.ToLower(args[1])
		targetID := args[2]

		switch globalAction {
		case "add":
			result := h.cfg.AddGlobalAdmin(userID, targetID)
			return result.Message, result.NeedRestart, nil
		case "remove", "rm":
			result := h.cfg.RemoveGlobalAdmin(userID, targetID)
			return result.Message, result.NeedRestart, nil
		}
		return "❌ Use: /config admin global add|remove <user_id>", false, nil
	}

	// Platform admin management
	targetID := args[1]

	switch action {
	case "add":
		result := h.cfg.AddPlatformAdmin(platform, userID, targetID)
		return result.Message, result.NeedRestart, nil
	case "remove", "rm":
		result := h.cfg.RemovePlatformAdmin(platform, userID, targetID)
		return result.Message, result.NeedRestart, nil
	default:
		return "❌ Use: /config admin add|remove <user_id>", false, nil
	}
}

// handleAllow manages allowlist
func (h *AdminHandler) handleAllow(platform, userID, currentChatID string, args []string) (string, bool, error) {
	if len(args) < 1 {
		return `*Allow User/Chat*

/config allow user <user_id>  - Allow a user
/config allow chat <chat_id>  - Allow a group/channel
/config allow chat this       - Allow this chat
/config allow me              - Allow yourself`, false, nil
	}

	targetType := strings.ToLower(args[0])

	// Shortcut: /config allow me
	if targetType == "me" {
		result := h.cfg.AllowUser(platform, userID, userID)
		return result.Message, result.NeedRestart, nil
	}

	if len(args) < 2 {
		return "❌ Please specify the ID to allow", false, nil
	}

	targetID := args[1]

	switch targetType {
	case "user":
		result := h.cfg.AllowUser(platform, userID, targetID)
		return result.Message, result.NeedRestart, nil
	case "chat", "group", "channel":
		if targetID == "this" || targetID == "here" {
			targetID = currentChatID
		}
		result := h.cfg.AllowChat(platform, userID, targetID)
		return result.Message, result.NeedRestart, nil
	default:
		return "❌ Use: /config allow user|chat <id>", false, nil
	}
}

// handleRemove manages removing from allowlist
func (h *AdminHandler) handleRemove(platform, userID string, args []string) (string, bool, error) {
	if len(args) < 2 {
		return `*Remove User/Chat*

/config remove user <user_id>  - Remove a user
/config remove chat <chat_id>  - Remove a chat`, false, nil
	}

	targetType := strings.ToLower(args[0])
	targetID := args[1]

	switch targetType {
	case "user":
		result := h.cfg.RemoveUser(platform, userID, targetID)
		return result.Message, result.NeedRestart, nil
	case "chat", "group", "channel":
		result := h.cfg.RemoveChat(platform, userID, targetID)
		return result.Message, result.NeedRestart, nil
	default:
		return "❌ Use: /config remove user|chat <id>", false, nil
	}
}

// handleMode sets access mode
func (h *AdminHandler) handleMode(platform, userID string, args []string) (string, bool, error) {
	if len(args) < 1 {
		return `*Access Mode*

/config mode allowlist  - Only allowed users (default)
/config mode denylist   - Everyone except denied
/config mode open       - Everyone can use

Current: ` + h.cfg.Access.Mode, false, nil
	}

	mode := args[0]
	result := h.cfg.SetAccessMode(userID, mode)
	return result.Message, result.NeedRestart, nil
}

// showHelp shows available commands
func (h *AdminHandler) showHelp(platform, userID string) string {
	isGlobalAdmin := h.cfg.IsGlobalAdmin(userID)
	isPlatformAdmin := h.cfg.IsPlatformAdmin(platform, userID)

	var sb strings.Builder
	sb.WriteString("🔧 *Config Commands*\n\n")

	sb.WriteString("/config status - Show current config\n")

	if isPlatformAdmin {
		sb.WriteString("\n*Allowlist:*\n")
		sb.WriteString("/config allow user <id>\n")
		sb.WriteString("/config allow chat <id|this>\n")
		sb.WriteString("/config remove user <id>\n")
		sb.WriteString("/config remove chat <id>\n")

		sb.WriteString("\n*Platform Admins:*\n")
		sb.WriteString("/config admin add <id>\n")
		sb.WriteString("/config admin remove <id>\n")
	}

	if isGlobalAdmin {
		sb.WriteString("\n*Global Admin:*\n")
		sb.WriteString("/config admin global add <id>\n")
		sb.WriteString("/config admin global rm <id>\n")
		sb.WriteString("/config mode <allowlist|open>\n")
	}

	if !isPlatformAdmin && !isGlobalAdmin {
		sb.WriteString("\n_You don't have admin access._")
	}

	sb.WriteString("\n\n💡 Changes trigger auto-restart.")

	return sb.String()
}

// TriggerRestart triggers a bot restart
func (h *AdminHandler) TriggerRestart() error {
	pidFile := filepath.Join(h.configDir, "magabot.pid")
	return config.RestartBot(pidFile)
}

// ScheduleRestart schedules a restart after a short delay
func (h *AdminHandler) ScheduleRestart(delaySec int, notifyFunc func(string)) {
	go func() {
		if notifyFunc != nil {
			notifyFunc(fmt.Sprintf("🔄 Restarting in %d seconds...", delaySec))
		}
		time.Sleep(time.Duration(delaySec) * time.Second)
		if err := h.TriggerRestart(); err != nil {
			if notifyFunc != nil {
				notifyFunc(fmt.Sprintf("⚠️ Restart failed: %v\nPlease restart manually.", err))
			}
		}
	}()
}
