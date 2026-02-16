package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/session"
	"github.com/kusa/magabot/internal/util"
)

// SessionHandler handles session-related commands
type SessionHandler struct {
	manager *session.Manager
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(manager *session.Manager) *SessionHandler {
	return &SessionHandler{
		manager: manager,
	}
}

// HandleCommand processes session commands
func (h *SessionHandler) HandleCommand(userID, platform, chatID string, args []string) (string, error) {
	if len(args) == 0 {
		return h.showHelp(), nil
	}

	cmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch cmd {
	case "spawn", "run", "bg":
		return h.spawnTask(userID, platform, chatID, subArgs)
	case "list", "ls":
		return h.listSessions(userID)
	case "status":
		return h.sessionStatus(subArgs)
	case "cancel", "stop":
		return h.cancelSession(subArgs)
	case "clear":
		return h.clearSessions()
	case "help":
		return h.showHelp(), nil
	default:
		return fmt.Sprintf("❌ Unknown command: %s\nUse /task help for available commands.", cmd), nil
	}
}

// spawnTask creates a new background task
func (h *SessionHandler) spawnTask(userID, platform, chatID string, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /task spawn <task description>", nil
	}

	task := strings.Join(args, " ")

	// Get or create main session
	mainSession := h.manager.GetOrCreate(platform, chatID, userID)

	// Spawn sub-session
	subSession, err := h.manager.Spawn(mainSession, task)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("🚀 *Task Spawned*\n\n📋 %s\n🔑 ID: %s\n\nI'll notify you when it's done!",
		util.Truncate(task, 100), subSession.ID[:16]), nil
}

// listSessions lists active sessions
func (h *SessionHandler) listSessions(userID string) (string, error) {
	sessions := h.manager.List(userID, false)

	if len(sessions) == 0 {
		return "📋 No active sessions.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 *Active Sessions* (%d)\n\n", len(sessions)))

	for _, s := range sessions {
		icon := "🔄"
		switch s.Status {
		case session.StatusComplete:
			icon = "✅"
		case session.StatusFailed:
			icon = "❌"
		case session.StatusPending:
			icon = "⏳"
		case session.StatusCanceled:
			icon = "🚫"
		}

		sb.WriteString(fmt.Sprintf("%s `%s` [%s]\n", icon, s.ID[:12], s.Type))
		if s.Task != "" {
			sb.WriteString(fmt.Sprintf("   📋 %s\n", util.Truncate(s.Task, 50)))
		}
	}

	return sb.String(), nil
}

// sessionStatus shows status of a specific session
func (h *SessionHandler) sessionStatus(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /task status <session_id>", nil
	}

	sessionID := args[0]

	// Find session by partial ID
	sessions := h.manager.List("", true)
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, sessionID) {
			return h.formatSessionStatus(s), nil
		}
	}

	return fmt.Sprintf("❌ Session not found: %s", sessionID), nil
}

// formatSessionStatus formats session details
func (h *SessionHandler) formatSessionStatus(s *session.Session) string {
	var sb strings.Builder

	icon := "🔄"
	switch s.Status {
	case session.StatusComplete:
		icon = "✅"
	case session.StatusFailed:
		icon = "❌"
	case session.StatusPending:
		icon = "⏳"
	case session.StatusCanceled:
		icon = "🚫"
	}

	sb.WriteString(fmt.Sprintf("%s *Session Status*\n\n", icon))
	sb.WriteString(fmt.Sprintf("ID: `%s`\n", s.ID))
	sb.WriteString(fmt.Sprintf("Type: %s\n", s.Type))
	sb.WriteString(fmt.Sprintf("Status: %s\n", s.Status))

	if s.Task != "" {
		sb.WriteString(fmt.Sprintf("\n📋 *Task:*\n%s\n", s.Task))
	}

	if s.Result != "" {
		sb.WriteString(fmt.Sprintf("\n✅ *Result:*\n%s\n", util.Truncate(s.Result, 500)))
	}

	if s.Error != "" {
		sb.WriteString(fmt.Sprintf("\n❌ *Error:*\n%s\n", s.Error))
	}

	sb.WriteString(fmt.Sprintf("\n📅 Created: %s", s.CreatedAt.Format("15:04:05")))
	if s.CompletedAt != nil {
		sb.WriteString(fmt.Sprintf("\n⏱️ Completed: %s", s.CompletedAt.Format("15:04:05")))
	}

	return sb.String()
}

// cancelSession cancels a running session
func (h *SessionHandler) cancelSession(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /task cancel <session_id>", nil
	}

	sessionID := args[0]

	// Find session by partial ID
	sessions := h.manager.List("", false)
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, sessionID) {
			if err := h.manager.Cancel(s.ID); err != nil {
				return fmt.Sprintf("❌ Failed to cancel: %v", err), nil
			}
			return fmt.Sprintf("🚫 Canceled session: %s", s.ID[:12]), nil
		}
	}

	return fmt.Sprintf("❌ Session not found: %s", sessionID), nil
}

// clearSessions clears completed sessions
func (h *SessionHandler) clearSessions() (string, error) {
	count := h.manager.Clear(time.Hour)
	return fmt.Sprintf("🗑️ Cleared %d completed sessions.", count), nil
}

// showHelp shows help text
func (h *SessionHandler) showHelp() string {
	return `🔄 *Session/Task Commands*

/task spawn <task>     Run a task in background
/task list             List active sessions
/task status <id>      Show session status
/task cancel <id>      Cancel a running task
/task clear            Clear completed tasks
/task help             Show this help

*Examples:*
• /task spawn Research AI trends and summarize
• /task spawn Analyze BBNI stock performance
• /task list
• /task cancel abc123

💡 Background tasks run independently and notify you when done.`
}
