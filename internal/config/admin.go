package config

import (
	"fmt"
	"strings"
)

// AdminAction represents a config change action
type AdminAction struct {
	Platform    string
	UserID      string
	Action      string
	Target      string
	TargetID    string
	Success     bool
	Message     string
	NeedRestart bool
}

// AddGlobalAdmin adds a global admin (requires global admin)
func (c *Config) AddGlobalAdmin(requesterID, newAdminID string) AdminAction {
	result := AdminAction{
		Action:   "add_global_admin",
		Target:   "global",
		TargetID: newAdminID,
	}

	c.mu.Lock()
	if !c.isGlobalAdmin(requesterID) {
		c.mu.Unlock()
		result.Message = "Only global admins can add global admins"
		return result
	}
	c.Access.GlobalAdmins = addUnique(c.Access.GlobalAdmins, newAdminID)
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Added global admin: %s", newAdminID)
	return result
}

// RemoveGlobalAdmin removes a global admin
func (c *Config) RemoveGlobalAdmin(requesterID, adminID string) AdminAction {
	result := AdminAction{
		Action:   "remove_global_admin",
		Target:   "global",
		TargetID: adminID,
	}

	c.mu.Lock()
	if !c.isGlobalAdmin(requesterID) {
		c.mu.Unlock()
		result.Message = "Only global admins can remove global admins"
		return result
	}
	if len(c.Access.GlobalAdmins) == 1 && c.Access.GlobalAdmins[0] == adminID {
		c.mu.Unlock()
		result.Message = "Cannot remove the last global admin"
		return result
	}
	c.Access.GlobalAdmins = remove(c.Access.GlobalAdmins, adminID)
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Removed global admin: %s", adminID)
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

	c.mu.Lock()
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can add platform admins"
		return result
	}
	c.mu.Unlock()

	// New admin must be in allowlist first (IsAllowed acquires its own lock)
	if !c.IsAllowed(platform, newAdminID, "", false) {
		result.Message = "User must be in allowlist first. Use /allow user " + newAdminID
		return result
	}

	c.mu.Lock()
	admins := c.platformAdmins(platform)
	if admins == nil {
		c.mu.Unlock()
		result.Message = fmt.Sprintf("Unknown platform: %s", platform)
		return result
	}
	*admins = addUnique(*admins, newAdminID)
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Added %s admin: %s", platform, newAdminID)
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

	c.mu.Lock()
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can remove platform admins"
		return result
	}
	if admins := c.platformAdmins(platform); admins != nil {
		*admins = remove(*admins, adminID)
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Removed %s admin: %s", platform, adminID)
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

	c.mu.Lock()
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can modify allowlist"
		return result
	}
	if users := c.platformAllowedUsers(platform); users != nil {
		*users = addUnique(*users, userID)
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Allowed user: %s", userID)
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

	c.mu.Lock()
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can modify allowlist"
		return result
	}
	// Cannot remove platform admin from allowlist
	if c.isPlatformAdmin(platform, userID) && userID != requesterID {
		c.mu.Unlock()
		result.Message = "Cannot remove a platform admin. Remove admin status first."
		return result
	}
	if users := c.platformAllowedUsers(platform); users != nil {
		*users = remove(*users, userID)
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Removed user: %s", userID)
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

	c.mu.Lock()
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can modify allowlist"
		return result
	}
	if chats := c.platformAllowedChats(platform); chats != nil {
		*chats = addUnique(*chats, chatID)
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Allowed chat: %s", chatID)
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

	c.mu.Lock()
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can modify allowlist"
		return result
	}
	if chats := c.platformAllowedChats(platform); chats != nil {
		*chats = remove(*chats, chatID)
	}
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Removed chat: %s", chatID)
	return result
}

// SetAccessMode sets the global access mode
func (c *Config) SetAccessMode(requesterID, mode string) AdminAction {
	result := AdminAction{
		Action:   "set_mode",
		Target:   "access",
		TargetID: mode,
	}

	mode = strings.ToLower(mode)
	if mode != "allowlist" && mode != "denylist" && mode != "open" {
		result.Message = "Invalid mode. Use: allowlist, denylist, or open"
		return result
	}

	c.mu.Lock()
	if !c.isGlobalAdmin(requesterID) {
		c.mu.Unlock()
		result.Message = "Only global admins can change access mode"
		return result
	}
	c.Access.Mode = mode
	c.mu.Unlock()

	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}

	result.Success = true
	result.NeedRestart = true
	result.Message = fmt.Sprintf("Access mode set to: %s", mode)
	return result
}

// platformAdmins returns a pointer to the platform's Admins slice, or nil if unknown
func (c *Config) platformAdmins(platform string) *[]string {
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			return &c.Platforms.Telegram.Admins
		}
	case "discord":
		if c.Platforms.Discord != nil {
			return &c.Platforms.Discord.Admins
		}
	case "slack":
		if c.Platforms.Slack != nil {
			return &c.Platforms.Slack.Admins
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			return &c.Platforms.WhatsApp.Admins
		}
	}
	return nil
}

// platformAllowedUsers returns a pointer to the platform's AllowedUsers slice, or nil
func (c *Config) platformAllowedUsers(platform string) *[]string {
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			return &c.Platforms.Telegram.AllowedUsers
		}
	case "discord":
		if c.Platforms.Discord != nil {
			return &c.Platforms.Discord.AllowedUsers
		}
	case "slack":
		if c.Platforms.Slack != nil {
			return &c.Platforms.Slack.AllowedUsers
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			return &c.Platforms.WhatsApp.AllowedUsers
		}
	}
	return nil
}

// platformAllowedChats returns a pointer to the platform's AllowedChats slice, or nil
func (c *Config) platformAllowedChats(platform string) *[]string {
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			return &c.Platforms.Telegram.AllowedChats
		}
	case "discord":
		if c.Platforms.Discord != nil {
			return &c.Platforms.Discord.AllowedChats
		}
	case "slack":
		if c.Platforms.Slack != nil {
			return &c.Platforms.Slack.AllowedChats
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			return &c.Platforms.WhatsApp.AllowedChats
		}
	}
	return nil
}
