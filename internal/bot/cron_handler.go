package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/cron"
)

// CronHandler handles cron-related bot commands
type CronHandler struct {
	scheduler *cron.Scheduler
}

// NewCronHandler creates a new cron handler
func NewCronHandler(scheduler *cron.Scheduler) *CronHandler {
	return &CronHandler{scheduler: scheduler}
}

// HandleCommand processes cron commands from chat
func (h *CronHandler) HandleCommand(ctx context.Context, cmd string, args []string, chatID string) (string, error) {
	switch cmd {
	case "/cron", "/cron_list", "/jobs":
		return h.listJobs(args)
	case "/cron_add":
		return h.addJob(args, chatID)
	case "/cron_edit":
		return h.editJob(args)
	case "/cron_delete", "/cron_rm":
		return h.deleteJob(args)
	case "/cron_enable":
		return h.enableJob(args)
	case "/cron_disable":
		return h.disableJob(args)
	case "/cron_run":
		return h.runJob(args)
	case "/cron_show":
		return h.showJob(args)
	case "/cron_help":
		return h.helpText(), nil
	default:
		return "", fmt.Errorf("unknown cron command: %s", cmd)
	}
}

// listJobs returns formatted job list
func (h *CronHandler) listJobs(args []string) (string, error) {
	showAll := len(args) > 0 && args[0] == "all"

	jobs := h.scheduler.ListJobs()
	if len(jobs) == 0 {
		return "📋 No cron jobs configured.\n\nUse /cron_add to create one.", nil
	}

	var sb strings.Builder
	sb.WriteString("📋 *Cron Jobs*\n\n")

	count := 0
	for _, job := range jobs {
		if !showAll && !job.Enabled {
			continue
		}
		count++

		status := "✅"
		if !job.Enabled {
			status = "❌"
		}

		channels := make([]string, len(job.Channels))
		for i, ch := range job.Channels {
			channels[i] = ch.Type
		}

		lastRun := "-"
		if job.LastRunAt != nil {
			lastRun = job.LastRunAt.Format("01/02 15:04")
		}

		sb.WriteString(fmt.Sprintf("%s `%s` *%s*\n", status, job.ID, job.Name))
		sb.WriteString(fmt.Sprintf("   📅 `%s`\n", job.Schedule))
		sb.WriteString(fmt.Sprintf("   📨 %s | ⏱️ %s\n\n", strings.Join(channels, ", "), lastRun))
	}

	if count == 0 {
		return "📋 No enabled jobs. Use `/cron all` to see disabled jobs.", nil
	}

	sb.WriteString("---\n")
	sb.WriteString("Commands: /cron\\_show, /cron\\_enable, /cron\\_disable, /cron\\_run")

	return sb.String(), nil
}

// addJob creates a new job
func (h *CronHandler) addJob(args []string, defaultChatID string) (string, error) {
	// Format: /cron_add name | schedule | message | channel:target
	// Example: /cron_add Morning Alert | 0 9 * * 1-5 | Good morning! | telegram:123456

	if len(args) == 0 {
		return `📝 *Add Cron Job*

Format:
/cron_add name | schedule | message | channel:target

Example:
` + "`/cron_add Morning Alert | 0 9 * * 1-5 | Good morning! | telegram:me`" + `

Channels:
• telegram:CHAT_ID or telegram:me (this chat)
• whatsapp:PHONE
• slack:#channel
• discord:WEBHOOK_URL
• webhook:URL

Schedules:
• ` + "`0 9 * * 1-5`" + ` - 9am weekdays
• ` + "`0 */2 * * *`" + ` - every 2 hours
• ` + "`@hourly`" + ` - every hour
• ` + "`@daily`" + ` - daily at midnight`, nil
	}

	// Parse pipe-separated format
	input := strings.Join(args, " ")
	parts := strings.Split(input, "|")

	if len(parts) < 4 {
		return "❌ Invalid format. Use: `/cron_add name | schedule | message | channel:target`", nil
	}

	name := strings.TrimSpace(parts[0])
	schedule := strings.TrimSpace(parts[1])
	message := strings.TrimSpace(parts[2])
	channelStr := strings.TrimSpace(parts[3])

	// Parse channel
	channelParts := strings.SplitN(channelStr, ":", 2)
	if len(channelParts) != 2 {
		return "❌ Invalid channel format. Use: `type:target` (e.g., telegram:123456)", nil
	}

	target := channelParts[1]
	if target == "me" || target == "this" {
		target = defaultChatID
	}

	channel := cron.NotifyChannel{
		Type:   strings.ToLower(channelParts[0]),
		Target: target,
		Name:   target,
	}

	job := &cron.Job{
		Name:     name,
		Schedule: schedule,
		Message:  message,
		Channels: []cron.NotifyChannel{channel},
		Enabled:  true,
	}

	if err := h.scheduler.AddJob(job); err != nil {
		return fmt.Sprintf("❌ Failed to create job: %v", err), nil
	}

	return fmt.Sprintf("✅ Created job `%s` (*%s*)\n\n📅 Schedule: `%s`\n📨 Channel: %s",
		job.ID, job.Name, job.Schedule, channel.Type), nil
}

// editJob modifies an existing job
func (h *CronHandler) editJob(args []string) (string, error) {
	if len(args) < 2 {
		return `📝 *Edit Cron Job*

Format:
/cron_edit JOB_ID field=value

Fields: name, schedule, message, channel

Examples:
` + "`/cron_edit abc123 name=New Name`" + `
` + "`/cron_edit abc123 schedule=0 10 * * *`" + `
` + "`/cron_edit abc123 message=Hello World!`", nil
	}

	jobID := args[0]
	job, err := h.scheduler.GetJob(jobID)
	if err != nil {
		return fmt.Sprintf("❌ Job not found: %s", jobID), nil
	}

	// Parse field=value pairs
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			continue
		}

		field := strings.ToLower(parts[0])
		value := parts[1]

		switch field {
		case "name":
			job.Name = value
		case "schedule":
			job.Schedule = value
		case "message", "msg":
			job.Message = value
		case "channel":
			channelParts := strings.SplitN(value, ":", 2)
			if len(channelParts) == 2 {
				job.Channels = []cron.NotifyChannel{{
					Type:   channelParts[0],
					Target: channelParts[1],
					Name:   channelParts[1],
				}}
			}
		}
	}

	if err := h.scheduler.UpdateJob(job); err != nil {
		return fmt.Sprintf("❌ Failed to update: %v", err), nil
	}

	return fmt.Sprintf("✅ Updated job `%s`", jobID), nil
}

// deleteJob removes a job
func (h *CronHandler) deleteJob(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: `/cron_delete JOB_ID`", nil
	}

	jobID := args[0]
	job, err := h.scheduler.GetJob(jobID)
	if err != nil {
		return fmt.Sprintf("❌ Job not found: %s", jobID), nil
	}

	if err := h.scheduler.DeleteJob(jobID); err != nil {
		return fmt.Sprintf("❌ Failed to delete: %v", err), nil
	}

	return fmt.Sprintf("✅ Deleted job `%s` (*%s*)", jobID, job.Name), nil
}

// enableJob enables a job
func (h *CronHandler) enableJob(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: `/cron_enable JOB_ID`", nil
	}

	jobID := args[0]
	if err := h.scheduler.EnableJob(jobID); err != nil {
		return fmt.Sprintf("❌ Failed: %v", err), nil
	}

	return fmt.Sprintf("✅ Enabled job `%s`", jobID), nil
}

// disableJob disables a job
func (h *CronHandler) disableJob(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: `/cron_disable JOB_ID`", nil
	}

	jobID := args[0]
	if err := h.scheduler.DisableJob(jobID); err != nil {
		return fmt.Sprintf("❌ Failed: %v", err), nil
	}

	return fmt.Sprintf("✅ Disabled job `%s`", jobID), nil
}

// runJob executes a job immediately
func (h *CronHandler) runJob(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: `/cron_run JOB_ID`", nil
	}

	jobID := args[0]
	job, err := h.scheduler.GetJob(jobID)
	if err != nil {
		return fmt.Sprintf("❌ Job not found: %s", jobID), nil
	}

	if err := h.scheduler.RunNow(jobID); err != nil {
		return fmt.Sprintf("⚠️ Job ran with error: %v", err), nil
	}

	return fmt.Sprintf("✅ Executed job `%s` (*%s*)", jobID, job.Name), nil
}

// showJob displays job details
func (h *CronHandler) showJob(args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: `/cron_show JOB_ID`", nil
	}

	jobID := args[0]
	job, err := h.scheduler.GetJob(jobID)
	if err != nil {
		return fmt.Sprintf("❌ Job not found: %s", jobID), nil
	}

	status := "✅ Enabled"
	if !job.Enabled {
		status = "❌ Disabled"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 *Job: %s*\n\n", job.Name))
	sb.WriteString(fmt.Sprintf("ID: `%s`\n", job.ID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", status))
	sb.WriteString(fmt.Sprintf("Schedule: `%s`\n", job.Schedule))
	sb.WriteString(fmt.Sprintf("Message: %s\n\n", job.Message))

	sb.WriteString("*Channels:*\n")
	for _, ch := range job.Channels {
		sb.WriteString(fmt.Sprintf("• %s: `%s`\n", ch.Type, ch.Target))
	}

	sb.WriteString(fmt.Sprintf("\nCreated: %s\n", job.CreatedAt.Format("2006-01-02 15:04")))
	if job.LastRunAt != nil {
		sb.WriteString(fmt.Sprintf("Last Run: %s\n", job.LastRunAt.Format("2006-01-02 15:04")))
	}
	sb.WriteString(fmt.Sprintf("Run Count: %d\n", job.RunCount))

	if job.LastError != "" {
		sb.WriteString(fmt.Sprintf("\n⚠️ Last Error: %s\n", job.LastError))
	}

	return sb.String(), nil
}

// helpText returns help message
func (h *CronHandler) helpText() string {
	return `🕐 *Cron Job Commands*

*List & View*
/cron - List enabled jobs
/cron all - List all jobs
/cron_show ID - Show job details

*Create & Manage*
/cron_add - Add new job
/cron_edit ID field=value - Edit job
/cron_delete ID - Delete job

*Control*
/cron_enable ID - Enable job
/cron_disable ID - Disable job
/cron_run ID - Run job now

*Add Job Format*
` + "`/cron_add name | schedule | message | channel:target`" + `

*Channels*
• telegram:CHAT_ID
• whatsapp:PHONE
• slack:#channel
• discord:WEBHOOK
• webhook:URL

*Schedule Examples*
• ` + "`0 9 * * 1-5`" + ` - 9am weekdays
• ` + "`0 */2 * * *`" + ` - every 2 hours
• ` + "`@hourly`" + ` @daily @weekly`
}

// NextRuns returns next scheduled run times
func (h *CronHandler) NextRuns(jobID string, count int) ([]time.Time, error) {
	// This would require parsing the cron expression
	// For now, return empty
	return nil, nil
}
