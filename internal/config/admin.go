package config

import (
	"fmt"
	"strings"

	"github.com/kusa/magabot/internal/util"
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

// completeAction saves config and returns a success result, or a failure result on save error.
func (c *Config) completeAction(result AdminAction, requesterID, msg string) AdminAction {
	if err := c.SaveBy(requesterID); err != nil {
		result.Message = fmt.Sprintf("Failed to save: %v", err)
		return result
	}
	result.Success = true
	result.NeedRestart = true
	result.Message = msg
	return result
}

// AddPlatformAdmin adds a platform admin (requires platform admin)
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
	*admins = util.AddUnique(*admins, newAdminID)
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Added %s admin: %s", platform, newAdminID))
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
		*admins = util.Remove(*admins, adminID)
	}
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Removed %s admin: %s", platform, adminID))
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
		*users = util.AddUnique(*users, userID)
	}
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Allowed user: %s", userID))
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
		*users = util.Remove(*users, userID)
	}
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Removed user: %s", userID))
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
		*chats = util.AddUnique(*chats, chatID)
	}
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Allowed chat: %s", chatID))
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
		*chats = util.Remove(*chats, chatID)
	}
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Removed chat: %s", chatID))
}

// SetAccessMode sets the global access mode
func (c *Config) SetAccessMode(platform, requesterID, mode string) AdminAction {
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
	if !c.isPlatformAdmin(platform, requesterID) {
		c.mu.Unlock()
		result.Message = "Only platform admins can change access mode"
		return result
	}
	c.Access.Mode = mode
	c.mu.Unlock()

	return c.completeAction(result, requesterID, fmt.Sprintf("Access mode set to: %s", mode))
}

// PromoteFirstAdmin promotes a user to platform admin if no admins exist yet.
// Returns true if promoted.
// This is safe to call concurrently — only the very first caller wins.
func (c *Config) PromoteFirstAdmin(platform, userID string) bool {
	c.mu.Lock()

	admins := c.platformAdmins(platform)
	if admins == nil || len(*admins) > 0 {
		c.mu.Unlock()
		return false
	}

	*admins = []string{userID}
	c.mu.Unlock()

	if err := c.SaveBy("auto:first-user"); err != nil {
		return false
	}
	return true
}

// platformAccessPtrs returns pointers to the platform's access slices (caller must hold mu).
func (c *Config) platformAccessPtrs(platform string) (admins, users, chats *[]string) {
	switch platform {
	case "telegram":
		if p := c.Platforms.Telegram; p != nil {
			return &p.Admins, &p.AllowedUsers, &p.AllowedChats
		}
	case "discord":
		if p := c.Platforms.Discord; p != nil {
			return &p.Admins, &p.AllowedUsers, &p.AllowedChats
		}
	case "slack":
		if p := c.Platforms.Slack; p != nil {
			return &p.Admins, &p.AllowedUsers, &p.AllowedChats
		}
	case "whatsapp":
		if p := c.Platforms.WhatsApp; p != nil {
			return &p.Admins, &p.AllowedUsers, &p.AllowedChats
		}
	}
	return nil, nil, nil
}

func (c *Config) platformAdmins(platform string) *[]string {
	admins, _, _ := c.platformAccessPtrs(platform)
	return admins
}

func (c *Config) platformAllowedUsers(platform string) *[]string {
	_, users, _ := c.platformAccessPtrs(platform)
	return users
}

func (c *Config) platformAllowedChats(platform string) *[]string {
	_, _, chats := c.platformAccessPtrs(platform)
	return chats
}
