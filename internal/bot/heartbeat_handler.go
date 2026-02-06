package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/heartbeat"
)

// HeartbeatHandler handles heartbeat-related commands
type HeartbeatHandler struct {
	service *heartbeat.Service
}

// NewHeartbeatHandler creates a new heartbeat handler
func NewHeartbeatHandler(service *heartbeat.Service) *HeartbeatHandler {
	return &HeartbeatHandler{
		service: service,
	}
}

// HandleCommand processes heartbeat commands
func (h *HeartbeatHandler) HandleCommand(args []string) (string, error) {
	if len(args) == 0 {
		return h.showStatus(), nil
	}
	
	cmd := strings.ToLower(args[0])
	subArgs := args[1:]
	
	switch cmd {
	case "status":
		return h.showStatus(), nil
	case "run", "now":
		return h.runNow()
	case "enable":
		return h.enableCheck(subArgs)
	case "disable":
		return h.disableCheck(subArgs)
	case "list", "checks":
		return h.listChecks(), nil
	case "help":
		return h.showHelp(), nil
	default:
		return fmt.Sprintf("❌ Unknown command: %s", cmd), nil
	}
}

// showStatus shows heartbeat status
func (h *HeartbeatHandler) showStatus() string {
	checks := h.service.Status()
	
	if len(checks) == 0 {
		return "💓 *Heartbeat Status*\n\nNo checks configured."
	}
	
	var sb strings.Builder
	sb.WriteString("💓 *Heartbeat Status*\n\n")
	
	for name, check := range checks {
		icon := "✅"
		if !check.Enabled {
			icon = "❌"
		} else if check.LastResult == "alert" {
			icon = "🔔"
		} else if check.LastResult == "error" {
			icon = "⚠️"
		}
		
		sb.WriteString(fmt.Sprintf("%s *%s*\n", icon, name))
		sb.WriteString(fmt.Sprintf("   Interval: %v\n", check.Interval))
		
		if !check.LastRun.IsZero() {
			sb.WriteString(fmt.Sprintf("   Last run: %s (%s)\n", 
				check.LastRun.Format("15:04"), 
				check.LastResult))
		}
		
		if check.LastMessage != "" {
			sb.WriteString(fmt.Sprintf("   Message: %s\n", truncateHB(check.LastMessage, 50)))
		}
		
		sb.WriteString(fmt.Sprintf("   Runs: %d | Alerts: %d\n\n", check.RunCount, check.AlertCount))
	}
	
	return sb.String()
}

// runNow triggers all checks immediately
func (h *HeartbeatHandler) runNow() (string, error) {
	results := h.service.RunNow()
	
	if len(results) == 0 {
		return "💓 No checks to run.", nil
	}
	
	var sb strings.Builder
	sb.WriteString("💓 *Heartbeat Results*\n\n")
	
	for _, result := range results {
		sb.WriteString(result + "\n")
	}
	
	return sb.String(), nil
}

// enableCheck enables a check
func (h *HeartbeatHandler) enableCheck(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /heartbeat enable <check_name>", nil
	}
	
	name := args[0]
	h.service.EnableCheck(name)
	
	return fmt.Sprintf("✅ Enabled check: %s", name), nil
}

// disableCheck disables a check
func (h *HeartbeatHandler) disableCheck(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: /heartbeat disable <check_name>", nil
	}
	
	name := args[0]
	h.service.DisableCheck(name)
	
	return fmt.Sprintf("❌ Disabled check: %s", name), nil
}

// listChecks lists all configured checks
func (h *HeartbeatHandler) listChecks() string {
	checks := h.service.Status()
	
	if len(checks) == 0 {
		return "💓 No checks configured."
	}
	
	var sb strings.Builder
	sb.WriteString("💓 *Configured Checks*\n\n")
	
	for name, check := range checks {
		icon := "✅"
		if !check.Enabled {
			icon = "❌"
		}
		
		sb.WriteString(fmt.Sprintf("%s %s - %v\n", icon, name, check.Interval))
		if check.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", check.Description))
		}
	}
	
	return sb.String()
}

// showHelp shows help text
func (h *HeartbeatHandler) showHelp() string {
	return `💓 *Heartbeat Commands*

/heartbeat status     Show check status
/heartbeat run        Run all checks now
/heartbeat enable <n> Enable a check
/heartbeat disable <n> Disable a check
/heartbeat list       List configured checks
/heartbeat help       Show this help

💡 Heartbeat runs checks periodically and alerts you if something needs attention.`
}

// RegisterDefaultChecks adds default checks
func (h *HeartbeatHandler) RegisterDefaultChecks() {
	// Time-based check (example)
	h.service.AddCheck(
		"morning_reminder",
		"Morning reminder at 9 AM",
		time.Hour,
		heartbeat.NewTimeCheck([]int{9}),
	)
	
	// Add more default checks as needed
}

func truncateHB(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
