// Magabot - Lightweight, secure multi-platform chatbot with LLM integration
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kusa/magabot/internal/version"
)

// Default paths (cross-platform)
var (
	configDir  string
	configFile string
	dataDir    string
	logDir     string
	logFile    string
	pidFile    string
)

func init() {
	// Get home directory (works on Windows, Linux, macOS)
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
		if home == "" {
			home = os.Getenv("USERPROFILE") // Windows fallback
		}
	}

	configDir = filepath.Join(home, ".magabot")
	configFile = filepath.Join(configDir, "config.yaml")
	dataDir = filepath.Join(configDir, "data")
	logDir = filepath.Join(configDir, "logs")
	logFile = filepath.Join(logDir, "magabot.log")
	pidFile = filepath.Join(configDir, "magabot.pid")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "restart":
		cmdRestart()
	case "status":
		cmdStatus()
	case "log", "logs":
		cmdLog()
	case "setup":
		cmdSetup()
	case "reset":
		cmdReset()
	case "uninstall":
		cmdUninstall()
	case "version", "-v", "--version":
		fmt.Println(version.Info())
	case "help", "-h", "--help":
		printUsage()
	case "genkey":
		cmdGenKey()
	case "skill", "skills":
		cmdSkill()
	case "cron":
		cmdCron()
	case "config":
		cmdConfig()
	case "update":
		cmdUpdate()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`Magabot - Lightweight, secure multi-platform chatbot

Usage: magabot <command>

Commands:
  start       Start magabot daemon
  stop        Stop magabot daemon
  restart     Restart magabot daemon
  status      Show magabot status
  log         View logs (tail -f)
  setup       First-time setup wizard
  reset       Reset config (keep platform connections)
  uninstall   Completely uninstall magabot
  genkey      Generate encryption key
  skill       Manage skills (list, create, enable, disable)
  cron        Manage scheduled jobs
  config      Show/edit config (admins, allowlist, platforms)
  update      Check and apply updates
  version     Show version
  help        Show this help

Update Commands:
  update check          Check for new version
  update apply          Download and install update
  update rollback       Restore previous version

Config Commands:
  config show                   Show current configuration
  config edit                   Edit config.yaml
  config admin add <id>         Add global admin
  config admin remove <id>      Remove global admin

Cron Commands:
  cron list           List all cron jobs
  cron add            Add new scheduled job
  cron edit <id>      Edit a job
  cron delete <id>    Delete a job
  cron enable <id>    Enable a job
  cron disable <id>   Disable a job
  cron run <id>       Run job immediately
  cron show <id>      Show job details

Skill Commands:
  skill list          List installed skills
  skill info <name>   Show skill details
  skill create <name> Create new skill template
  skill builtin       List built-in skills
  skill reload        Reload all skills

Config: %s
Data:   %s
Logs:   %s
Skills: %s/skills

`, configFile, dataDir, logFile, configDir)
}
