package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/kusa/magabot/internal/cron"
)

// CronCommands returns cron management commands
func CronCommands(dataDir string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage scheduled jobs",
		Long:  "Create, edit, delete, and manage scheduled cron jobs",
	}
	
	cmd.AddCommand(
		cronListCmd(dataDir),
		cronAddCmd(dataDir),
		cronEditCmd(dataDir),
		cronDeleteCmd(dataDir),
		cronEnableCmd(dataDir),
		cronDisableCmd(dataDir),
		cronRunCmd(dataDir),
		cronShowCmd(dataDir),
	)
	
	return cmd
}

func cronListCmd(dataDir string) *cobra.Command {
	var showAll bool
	
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all cron jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			jobs := store.List()
			if len(jobs) == 0 {
				fmt.Println("No cron jobs configured.")
				return nil
			}
			
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSCHEDULE\tSTATUS\tCHANNELS\tLAST RUN")
			
			for _, job := range jobs {
				if !showAll && !job.Enabled {
					continue
				}
				
				status := "✅ Enabled"
				if !job.Enabled {
					status = "❌ Disabled"
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
					truncate(job.Name, 20),
					job.Schedule,
					status,
					strings.Join(channels, ","),
					lastRun,
				)
			}
			
			w.Flush()
			return nil
		},
	}
	
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all jobs including disabled")
	
	return cmd
}

func cronAddCmd(dataDir string) *cobra.Command {
	var (
		name        string
		description string
		schedule    string
		message     string
		channels    []string
		enabled     bool
	)
	
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new cron job",
		Long: `Add a new cron job with schedule and notification channels.

Examples:
  magabot cron add -n "Morning Alert" -s "0 9 * * 1-5" -m "Good morning!" -c telegram:123456
  magabot cron add -n "Hourly Check" -s "@hourly" -m "Status OK" -c slack:#general -c webhook:https://...

Channel format: type:target
  telegram:CHAT_ID      - Telegram chat ID
  whatsapp:PHONE        - WhatsApp phone number
  slack:CHANNEL         - Slack channel (e.g., #general)
  discord:WEBHOOK_URL   - Discord webhook URL
  webhook:URL           - Custom webhook URL

Schedule formats:
  "* * * * *"           - minute hour day month weekday
  "0 9 * * 1-5"         - 9am on weekdays
  "@hourly"             - every hour
  "@daily"              - every day at midnight
  "@weekly"             - every week
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || schedule == "" || message == "" || len(channels) == 0 {
				return fmt.Errorf("name, schedule, message, and at least one channel are required")
			}
			
			// Parse channels
			notifyChannels := make([]cron.NotifyChannel, 0, len(channels))
			for _, ch := range channels {
				parts := strings.SplitN(ch, ":", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid channel format: %s (use type:target)", ch)
				}
				
				notifyChannels = append(notifyChannels, cron.NotifyChannel{
					Type:   strings.ToLower(parts[0]),
					Target: parts[1],
					Name:   parts[1],
				})
			}
			
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			job := &cron.Job{
				Name:        name,
				Description: description,
				Schedule:    schedule,
				Message:     message,
				Channels:    notifyChannels,
				Enabled:     enabled,
			}
			
			if err := store.Create(job); err != nil {
				return err
			}
			
			status := "enabled"
			if !enabled {
				status = "disabled"
			}
			
			fmt.Printf("✅ Created job %s (%s) - %s\n", job.ID, job.Name, status)
			return nil
		},
	}
	
	cmd.Flags().StringVarP(&name, "name", "n", "", "Job name (required)")
	cmd.Flags().StringVarP(&description, "desc", "d", "", "Job description")
	cmd.Flags().StringVarP(&schedule, "schedule", "s", "", "Cron schedule (required)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Message to send (required)")
	cmd.Flags().StringArrayVarP(&channels, "channel", "c", nil, "Notification channels (type:target)")
	cmd.Flags().BoolVarP(&enabled, "enabled", "e", true, "Enable job immediately")
	
	return cmd
}

func cronEditCmd(dataDir string) *cobra.Command {
	var (
		name        string
		description string
		schedule    string
		message     string
		channels    []string
	)
	
	cmd := &cobra.Command{
		Use:   "edit JOB_ID",
		Short: "Edit an existing cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			job, err := store.Get(jobID)
			if err != nil {
				return err
			}
			
			// Update fields if provided
			if name != "" {
				job.Name = name
			}
			if description != "" {
				job.Description = description
			}
			if schedule != "" {
				job.Schedule = schedule
			}
			if message != "" {
				job.Message = message
			}
			if len(channels) > 0 {
				notifyChannels := make([]cron.NotifyChannel, 0, len(channels))
				for _, ch := range channels {
					parts := strings.SplitN(ch, ":", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid channel format: %s", ch)
					}
					notifyChannels = append(notifyChannels, cron.NotifyChannel{
						Type:   strings.ToLower(parts[0]),
						Target: parts[1],
						Name:   parts[1],
					})
				}
				job.Channels = notifyChannels
			}
			
			if err := store.Update(job); err != nil {
				return err
			}
			
			fmt.Printf("✅ Updated job %s\n", job.ID)
			return nil
		},
	}
	
	cmd.Flags().StringVarP(&name, "name", "n", "", "New job name")
	cmd.Flags().StringVarP(&description, "desc", "d", "", "New description")
	cmd.Flags().StringVarP(&schedule, "schedule", "s", "", "New schedule")
	cmd.Flags().StringVarP(&message, "message", "m", "", "New message")
	cmd.Flags().StringArrayVarP(&channels, "channel", "c", nil, "Replace channels")
	
	return cmd
}

func cronDeleteCmd(dataDir string) *cobra.Command {
	var force bool
	
	cmd := &cobra.Command{
		Use:     "delete JOB_ID",
		Aliases: []string{"rm", "remove"},
		Short:   "Delete a cron job",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			job, err := store.Get(jobID)
			if err != nil {
				return err
			}
			
			if !force {
				fmt.Printf("Delete job %s (%s)? [y/N]: ", job.ID, job.Name)
				var confirm string
				fmt.Scanln(&confirm)
				if strings.ToLower(confirm) != "y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}
			
			if err := store.Delete(jobID); err != nil {
				return err
			}
			
			fmt.Printf("✅ Deleted job %s\n", jobID)
			return nil
		},
	}
	
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	
	return cmd
}

func cronEnableCmd(dataDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "enable JOB_ID",
		Short: "Enable a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			if err := store.SetEnabled(args[0], true); err != nil {
				return err
			}
			
			fmt.Printf("✅ Enabled job %s\n", args[0])
			return nil
		},
	}
}

func cronDisableCmd(dataDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "disable JOB_ID",
		Short: "Disable a cron job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			if err := store.SetEnabled(args[0], false); err != nil {
				return err
			}
			
			fmt.Printf("✅ Disabled job %s\n", args[0])
			return nil
		},
	}
}

func cronRunCmd(dataDir string) *cobra.Command {
	return &cobra.Command{
		Use:   "run JOB_ID",
		Short: "Run a job immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Note: This only works if daemon is running
			// For standalone run, we need to load notifier config
			
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			job, err := store.Get(args[0])
			if err != nil {
				return err
			}
			
			fmt.Printf("Running job %s (%s)...\n", job.ID, job.Name)
			fmt.Printf("Message: %s\n", job.Message)
			fmt.Printf("Channels: ")
			for _, ch := range job.Channels {
				fmt.Printf("%s:%s ", ch.Type, ch.Target)
			}
			fmt.Println()
			
			// TODO: Load notifier config and actually send
			fmt.Println("⚠️  Manual run requires daemon to be running. Use 'magabot start' first.")
			
			return nil
		},
	}
}

func cronShowCmd(dataDir string) *cobra.Command {
	var jsonOutput bool
	
	cmd := &cobra.Command{
		Use:   "show JOB_ID",
		Short: "Show job details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := cron.NewJobStore(dataDir)
			if err != nil {
				return err
			}
			
			job, err := store.Get(args[0])
			if err != nil {
				return err
			}
			
			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(job)
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
				fmt.Printf("  - %s: %s (%s)\n", ch.Type, ch.Target, ch.Name)
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
			
			return nil
		},
	}
	
	cmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output as JSON")
	
	return cmd
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
