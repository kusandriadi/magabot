package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// AdminAction represents a config change action
type AdminAction struct {
	Platform  string
	UserID    string
	Action    string
	Target    string
	TargetID  string
	Success   bool
	Message   string
	NeedRestart bool
}

// AddGlobalAdmin adds a global admin (requires global admin)
func (c *Config) AddGlobalAdmin(requesterID, newAdminID string) AdminAction {
	result := AdminAction{
		Action:   "add_global_admin",
		Target:   "global",
		TargetID: newAdminID,
	}

	if !c.IsGlobalAdmin(requesterID) {
		result.Message = "❌ Only global admins can add global admins"
		return result
	}

	c.mu.Lock()
	c.Access.GlobalAdmins = addUnique(c.Access.GlobalAdmins, newAdminID)
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Added global admin: %s", newAdminID)
	return result
}

// RemoveGlobalAdmin removes a global admin
func (c *Config) RemoveGlobalAdmin(requesterID, adminID string) AdminAction {
	result := AdminAction{
		Action:   "remove_global_admin",
		Target:   "global",
		TargetID: adminID,
	}

	if !c.IsGlobalAdmin(requesterID) {
		result.Message = "❌ Only global admins can remove global admins"
		return result
	}

	// Prevent removing self if last admin
	c.mu.RLock()
	if len(c.Access.GlobalAdmins) == 1 && c.Access.GlobalAdmins[0] == adminID {
		c.mu.RUnlock()
		result.Message = "❌ Cannot remove the last global admin"
		return result
	}
	c.mu.RUnlock()

	c.mu.Lock()
	c.Access.GlobalAdmins = remove(c.Access.GlobalAdmins, adminID)
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Removed global admin: %s", adminID)
	return result
}

// AddPlatformAdmin adds a platform admin (requires platform admin or global admin)
func (c *Config) AddPlatformAdmin(platform, requesterID, newAdminID string) AdminAction {
	result := AdminAction{
		Platform: platform,
		Action:   "add_platform_admin",
		Target:   platform,
		TargetID: newAdminID,
	}

	if !c.IsPlatformAdmin(platform, requesterID) {
		result.Message = "❌ Only platform admins can add platform admins"
		return result
	}

	// New admin must be in allowlist first
	if !c.IsAllowed(platform, newAdminID, "", false) {
		result.Message = "❌ User must be in allowlist first. Use /allow user " + newAdminID
		return result
	}

	c.mu.Lock()
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			c.Platforms.Telegram.Admins = addUnique(c.Platforms.Telegram.Admins, newAdminID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			c.Platforms.Discord.Admins = addUnique(c.Platforms.Discord.Admins, newAdminID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			c.Platforms.Slack.Admins = addUnique(c.Platforms.Slack.Admins, newAdminID)
		}
	case "lark":
		if c.Platforms.Lark != nil {
			c.Platforms.Lark.Admins = addUnique(c.Platforms.Lark.Admins, newAdminID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			c.Platforms.WhatsApp.Admins = addUnique(c.Platforms.WhatsApp.Admins, newAdminID)
		}
	default:
		c.mu.Unlock()
		result.Message = fmt.Sprintf("❌ Unknown platform: %s", platform)
		return result
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Added %s admin: %s", platform, newAdminID)
	return result
}

// RemovePlatformAdmin removes a platform admin
func (c *Config) RemovePlatformAdmin(platform, requesterID, adminID string) AdminAction {
	result := AdminAction{
		Platform: platform,
		Action:   "remove_platform_admin",
		Target:   platform,
		TargetID: adminID,
	}

	if !c.IsPlatformAdmin(platform, requesterID) {
		result.Message = "❌ Only platform admins can remove platform admins"
		return result
	}

	c.mu.Lock()
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			c.Platforms.Telegram.Admins = remove(c.Platforms.Telegram.Admins, adminID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			c.Platforms.Discord.Admins = remove(c.Platforms.Discord.Admins, adminID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			c.Platforms.Slack.Admins = remove(c.Platforms.Slack.Admins, adminID)
		}
	case "lark":
		if c.Platforms.Lark != nil {
			c.Platforms.Lark.Admins = remove(c.Platforms.Lark.Admins, adminID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			c.Platforms.WhatsApp.Admins = remove(c.Platforms.WhatsApp.Admins, adminID)
		}
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Removed %s admin: %s", platform, adminID)
	return result
}

// AllowUser adds a user to the platform allowlist
func (c *Config) AllowUser(platform, requesterID, userID string) AdminAction {
	result := AdminAction{
		Platform: platform,
		Action:   "allow_user",
		Target:   "user",
		TargetID: userID,
	}

	if !c.IsPlatformAdmin(platform, requesterID) {
		result.Message = "❌ Only platform admins can modify allowlist"
		return result
	}

	c.mu.Lock()
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			c.Platforms.Telegram.AllowedUsers = addUnique(c.Platforms.Telegram.AllowedUsers, userID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			c.Platforms.Discord.AllowedUsers = addUnique(c.Platforms.Discord.AllowedUsers, userID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			c.Platforms.Slack.AllowedUsers = addUnique(c.Platforms.Slack.AllowedUsers, userID)
		}
	case "lark":
		if c.Platforms.Lark != nil {
			c.Platforms.Lark.AllowedUsers = addUnique(c.Platforms.Lark.AllowedUsers, userID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			c.Platforms.WhatsApp.AllowedUsers = addUnique(c.Platforms.WhatsApp.AllowedUsers, userID)
		}
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Allowed user: %s", userID)
	return result
}

// RemoveUser removes a user from the allowlist
func (c *Config) RemoveUser(platform, requesterID, userID string) AdminAction {
	result := AdminAction{
		Platform: platform,
		Action:   "remove_user",
		Target:   "user",
		TargetID: userID,
	}

	if !c.IsPlatformAdmin(platform, requesterID) {
		result.Message = "❌ Only platform admins can modify allowlist"
		return result
	}

	// Cannot remove platform admin from allowlist
	if c.IsPlatformAdmin(platform, userID) && userID != requesterID {
		result.Message = "❌ Cannot remove a platform admin. Remove admin status first."
		return result
	}

	c.mu.Lock()
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			c.Platforms.Telegram.AllowedUsers = remove(c.Platforms.Telegram.AllowedUsers, userID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			c.Platforms.Discord.AllowedUsers = remove(c.Platforms.Discord.AllowedUsers, userID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			c.Platforms.Slack.AllowedUsers = remove(c.Platforms.Slack.AllowedUsers, userID)
		}
	case "lark":
		if c.Platforms.Lark != nil {
			c.Platforms.Lark.AllowedUsers = remove(c.Platforms.Lark.AllowedUsers, userID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			c.Platforms.WhatsApp.AllowedUsers = remove(c.Platforms.WhatsApp.AllowedUsers, userID)
		}
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Removed user: %s", userID)
	return result
}

// AllowChat adds a chat/group to the allowlist
func (c *Config) AllowChat(platform, requesterID, chatID string) AdminAction {
	result := AdminAction{
		Platform: platform,
		Action:   "allow_chat",
		Target:   "chat",
		TargetID: chatID,
	}

	if !c.IsPlatformAdmin(platform, requesterID) {
		result.Message = "❌ Only platform admins can modify allowlist"
		return result
	}

	c.mu.Lock()
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			c.Platforms.Telegram.AllowedChats = addUnique(c.Platforms.Telegram.AllowedChats, chatID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			c.Platforms.Discord.AllowedChats = addUnique(c.Platforms.Discord.AllowedChats, chatID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			c.Platforms.Slack.AllowedChats = addUnique(c.Platforms.Slack.AllowedChats, chatID)
		}
	case "lark":
		if c.Platforms.Lark != nil {
			c.Platforms.Lark.AllowedChats = addUnique(c.Platforms.Lark.AllowedChats, chatID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			c.Platforms.WhatsApp.AllowedChats = addUnique(c.Platforms.WhatsApp.AllowedChats, chatID)
		}
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Allowed chat: %s", chatID)
	return result
}

// RemoveChat removes a chat from the allowlist
func (c *Config) RemoveChat(platform, requesterID, chatID string) AdminAction {
	result := AdminAction{
		Platform: platform,
		Action:   "remove_chat",
		Target:   "chat",
		TargetID: chatID,
	}

	if !c.IsPlatformAdmin(platform, requesterID) {
		result.Message = "❌ Only platform admins can modify allowlist"
		return result
	}

	c.mu.Lock()
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			c.Platforms.Telegram.AllowedChats = remove(c.Platforms.Telegram.AllowedChats, chatID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			c.Platforms.Discord.AllowedChats = remove(c.Platforms.Discord.AllowedChats, chatID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			c.Platforms.Slack.AllowedChats = remove(c.Platforms.Slack.AllowedChats, chatID)
		}
	case "lark":
		if c.Platforms.Lark != nil {
			c.Platforms.Lark.AllowedChats = remove(c.Platforms.Lark.AllowedChats, chatID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			c.Platforms.WhatsApp.AllowedChats = remove(c.Platforms.WhatsApp.AllowedChats, chatID)
		}
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Removed chat: %s", chatID)
	return result
}

// SetAccessMode sets the global access mode
func (c *Config) SetAccessMode(requesterID, mode string) AdminAction {
	result := AdminAction{
		Action:   "set_mode",
		Target:   "access",
		TargetID: mode,
	}

	if !c.IsGlobalAdmin(requesterID) {
		result.Message = "❌ Only global admins can change access mode"
		return result
	}

	mode = strings.ToLower(mode)
	if mode != "allowlist" && mode != "denylist" && mode != "open" {
		result.Message = "❌ Invalid mode. Use: allowlist, denylist, or open"
		return result
	}

	c.mu.Lock()
	c.Access.Mode = mode
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("❌ Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("✅ Access mode set to: %s", mode)
	return result
}

// RestartBot restarts the magabot process
func RestartBot(pidFile string) error {
	// Read PID
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID: %w", err)
	}

	// Send SIGHUP to trigger graceful restart
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send restart signal: %w", err)
	}

	return nil
}

// RestartBotAsync restarts in background
func RestartBotAsync(executable string) error {
	cmd := exec.Command(executable, "restart")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
