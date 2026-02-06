package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kusa/magabot/internal/cron"
)

func cmdCron() {
	if len(os.Args) < 3 {
		cmdCronList(false)
		return
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "list", "ls":
		showAll := len(os.Args) > 3 && os.Args[3] == "-a"
		cmdCronList(showAll)
	case "add":
		cmdCronAdd()
	case "edit":
		cmdCronEdit()
	case "delete", "rm", "remove":
		cmdCronDelete()
	case "enable":
		cmdCronSetEnabled(true)
	case "disable":
		cmdCronSetEnabled(false)
	case "run":
		cmdCronRun()
	case "show":
		cmdCronShow()
	case "help":
		cmdCronHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown cron command: %s\n", subCmd)
		cmdCronHelp()
		os.Exit(1)
	}
}

func cmdCronList(showAll bool) {
	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	jobs := store.List()
	if len(jobs) == 0 {
		fmt.Println("No cron jobs configured.")
		fmt.Println("\nUse 'magabot cron add' to create one.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSCHEDULE\tSTATUS\tCHANNELS\tLAST RUN")

	for _, job := range jobs {
		if !showAll && !job.Enabled {
			continue
		}

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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			job.ID,
			truncateStr(job.Name, 20),
			job.Schedule,
			status,
			strings.Join(channels, ","),
			lastRun,
		)
	}

	w.Flush()
	fmt.Printf("\nUse 'magabot cron list -a' to show disabled jobs\n")
}

func cmdCronAdd() {
	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("📅 Add Cron Job")
	fmt.Println("---------------")

	// Name
	fmt.Print("Job name: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Println("Error: Name is required")
		os.Exit(1)
	}

	// Description
	fmt.Print("Description (optional): ")
	desc, _ := reader.ReadString('\n')
	desc = strings.TrimSpace(desc)

	// Schedule
	fmt.Println("\nSchedule examples:")
	fmt.Println("  0 9 * * 1-5   - 9am weekdays")
	fmt.Println("  0 */2 * * *   - every 2 hours")
	fmt.Println("  @hourly       - every hour")
	fmt.Println("  @daily        - daily at midnight")
	fmt.Print("\nCron schedule: ")
	schedule, _ := reader.ReadString('\n')
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		fmt.Println("Error: Schedule is required")
		os.Exit(1)
	}

	// Message
	fmt.Print("\nMessage to send: ")
	message, _ := reader.ReadString('\n')
	message = strings.TrimSpace(message)
	if message == "" {
		fmt.Println("Error: Message is required")
		os.Exit(1)
	}

	// Channels
	fmt.Println("\nChannel format: type:target")
	fmt.Println("  telegram:123456789     - Telegram chat ID")
	fmt.Println("  whatsapp:+62812345678  - WhatsApp number")
	fmt.Println("  slack:#channel         - Slack channel")
	fmt.Println("  discord:webhook_url    - Discord webhook")
	fmt.Println("  webhook:https://...    - Custom webhook")
	fmt.Println("\nEnter channels (one per line, empty line to finish):")

	var channels []cron.NotifyChannel
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			fmt.Println("  Invalid format, use type:target")
			continue
		}

		channels = append(channels, cron.NotifyChannel{
			Type:   strings.ToLower(parts[0]),
			Target: parts[1],
			Name:   parts[1],
		})
		fmt.Printf("  Added: %s\n", line)
	}

	if len(channels) == 0 {
		fmt.Println("Error: At least one channel is required")
		os.Exit(1)
	}

	// Enable?
	fmt.Print("\nEnable job now? [Y/n]: ")
	enableStr, _ := reader.ReadString('\n')
	enableStr = strings.TrimSpace(strings.ToLower(enableStr))
	enabled := enableStr == "" || enableStr == "y" || enableStr == "yes"

	job := &cron.Job{
		Name:        name,
		Description: desc,
		Schedule:    schedule,
		Message:     message,
		Channels:    channels,
		Enabled:     enabled,
	}

	if err := store.Create(job); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating job: %v\n", err)
		os.Exit(1)
	}

	status := "enabled"
	if !enabled {
		status = "disabled"
	}

	fmt.Printf("\n✅ Created job %s (%s) - %s\n", job.ID, job.Name, status)
	fmt.Println("\nNote: Restart magabot for changes to take effect.")
}

func cmdCronEdit() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: magabot cron edit <job_id>")
		os.Exit(1)
	}

	jobID := os.Args[3]

	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	job, err := store.Get(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("📝 Edit Job: %s\n", job.Name)
	fmt.Println("(Press Enter to keep current value)")
	fmt.Println("----------------------------------")

	// Name
	fmt.Printf("Name [%s]: ", job.Name)
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		job.Name = name
	}

	// Description
	fmt.Printf("Description [%s]: ", job.Description)
	desc, _ := reader.ReadString('\n')
	desc = strings.TrimSpace(desc)
	if desc != "" {
		job.Description = desc
	}

	// Schedule
	fmt.Printf("Schedule [%s]: ", job.Schedule)
	schedule, _ := reader.ReadString('\n')
	schedule = strings.TrimSpace(schedule)
	if schedule != "" {
		job.Schedule = schedule
	}

	// Message
	fmt.Printf("Message [%s]: ", truncateStr(job.Message, 40))
	message, _ := reader.ReadString('\n')
	message = strings.TrimSpace(message)
	if message != "" {
		job.Message = message
	}

	if err := store.Update(job); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Updated job %s\n", jobID)
	fmt.Println("Note: Restart magabot for changes to take effect.")
}

func cmdCronDelete() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: magabot cron delete <job_id>")
		os.Exit(1)
	}

	jobID := os.Args[3]

	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	job, err := store.Get(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check for force flag
	force := len(os.Args) > 4 && os.Args[4] == "-f"

	if !force {
		fmt.Printf("Delete job %s (%s)? [y/N]: ", job.ID, job.Name)
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := store.Delete(jobID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Deleted job %s\n", jobID)
}

func cmdCronSetEnabled(enabled bool) {
	if len(os.Args) < 4 {
		action := "enable"
		if !enabled {
			action = "disable"
		}
		fmt.Printf("Usage: magabot cron %s <job_id>\n", action)
		os.Exit(1)
	}

	jobID := os.Args[3]

	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := store.SetEnabled(jobID, enabled); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	action := "Enabled"
	if !enabled {
		action = "Disabled"
	}

	fmt.Printf("✅ %s job %s\n", action, jobID)
	fmt.Println("Note: Restart magabot for changes to take effect.")
}

func cmdCronRun() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: magabot cron run <job_id>")
		os.Exit(1)
	}

	jobID := os.Args[3]

	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	job, err := store.Get(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Running job %s (%s)...\n", job.ID, job.Name)
	fmt.Printf("Message: %s\n", job.Message)
	fmt.Print("Channels: ")
	for _, ch := range job.Channels {
		fmt.Printf("%s:%s ", ch.Type, ch.Target)
	}
	fmt.Println()

	fmt.Println("\n⚠️  Manual run requires daemon to be running.")
	fmt.Println("   Use 'magabot start' first, then trigger via chat or API.")
}

func cmdCronShow() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: magabot cron show <job_id>")
		os.Exit(1)
	}

	jobID := os.Args[3]
	jsonOutput := len(os.Args) > 4 && os.Args[4] == "-j"

	store, err := cron.NewJobStore(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	job, err := store.Get(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(job)
		return
	}

	status := "✅ Enabled"
	if !job.Enabled {
		status = "❌ Disabled"
	}

	fmt.Printf("ID:          %s\n", job.ID)
	fmt.Printf("Name:        %s\n", job.Name)
	fmt.Printf("Description: %s\n", job.Description)
	fmt.Printf("Schedule:    %s\n", job.Schedule)
	fmt.Printf("Status:      %s\n", status)
	fmt.Printf("Message:     %s\n", job.Message)
	fmt.Println("Channels:")
	for _, ch := range job.Channels {
		fmt.Printf("  - %s: %s\n", ch.Type, ch.Target)
	}
	fmt.Printf("Created:     %s\n", job.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:     %s\n", job.UpdatedAt.Format(time.RFC3339))
	if job.LastRunAt != nil {
		fmt.Printf("Last Run:    %s\n", job.LastRunAt.Format(time.RFC3339))
	}
	fmt.Printf("Run Count:   %d\n", job.RunCount)
	if job.LastError != "" {
		fmt.Printf("Last Error:  %s\n", job.LastError)
	}
}

func cmdCronHelp() {
	fmt.Println(`Cron Job Management

Usage: magabot cron <command> [options]

Commands:
  list, ls          List all cron jobs (-a for all including disabled)
  add               Add a new job (interactive)
  edit <id>         Edit an existing job
  delete <id>       Delete a job (-f to skip confirmation)
  enable <id>       Enable a job
  disable <id>      Disable a job
  run <id>          Run a job immediately
  show <id>         Show job details (-j for JSON output)
  help              Show this help

Channel Types:
  telegram:CHAT_ID      Telegram chat ID
  whatsapp:PHONE        WhatsApp phone number (+62...)
  slack:#channel        Slack channel name
  discord:CHANNEL_ID    Discord channel ID (or webhook URL)
  lark:CHAT_ID          Lark (Feishu) chat ID
  webhook:URL           Custom webhook URL

Schedule Examples:
  "0 9 * * 1-5"         9am on weekdays
  "0 */2 * * *"         Every 2 hours
  "30 8,12,17 * * *"    8:30am, 12:30pm, 5:30pm daily
  "@hourly"             Every hour
  "@daily"              Every day at midnight
  "@weekly"             Every week

Examples:
  magabot cron add
  magabot cron list
  magabot cron enable abc123
  magabot cron delete abc123 -f`)
}

// truncateStr is defined in skill_cmd.go
