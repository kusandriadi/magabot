package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/storage"
	"github.com/kusa/magabot/internal/version"
	"gopkg.in/yaml.v3"
)

// cmdStart starts the magabot daemon
func cmdStart() {
	// Check if already running
	if isRunning() {
		fmt.Println("⚠️  Magabot is already running")
		return
	}

	// Check if config exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		fmt.Println("❌ Config not found. Run 'magabot setup' first.")
		os.Exit(1)
	}

	// Ensure directories
	ensureDirs()

	// Start daemon (platform-specific)
	pid, err := startDaemonProcess()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start: %v\n", err)
		os.Exit(1)
	}

	// Save PID
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write PID file: %v\n", err)
	}

	fmt.Printf("✅ Magabot started (PID: %d)\n", pid)
	fmt.Printf("   Logs: %s\n", logFile)
	fmt.Println()
	fmt.Println("   Run 'magabot status' to check status")
	fmt.Println("   Run 'magabot logs' to view logs")
}

// cmdStop stops the magabot daemon
func cmdStop() {
	pid := getPID()
	if pid == 0 {
		fmt.Println("⚠️  Magabot is not running")
		return
	}

	if err := stopProcess(pid); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to stop: %v\n", err)
		os.Exit(1)
	}

	_ = os.Remove(pidFile)
	fmt.Println("✅ Magabot stopped")
}

// cmdRestart restarts the daemon
func cmdRestart() {
	pid := getPID()
	if pid == 0 {
		fmt.Println("⚠️  Magabot is not running, starting...")
		cmdStart()
		return
	}

	if err := stopProcess(pid); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to stop: %v\n", err)
		os.Exit(1)
	}

	// Wait for process to actually exit (max 10 seconds)
	for i := 0; i < 100; i++ {
		if !processExists(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if processExists(pid) {
		fmt.Fprintf(os.Stderr, "❌ Process %d did not exit in time\n", pid)
		os.Exit(1)
	}

	_ = os.Remove(pidFile)
	fmt.Println("✅ Magabot stopped")
	cmdStart()
}

// cmdStatus shows the daemon status
func cmdStatus() {
	pid := getPID()

	if pid == 0 || !processExists(pid) {
		fmt.Println("🔴 Magabot is NOT running")

		// Check for recent errors in log
		if content, err := readLastLines(logFile, 5); err == nil && content != "" {
			fmt.Println("\n📋 Recent log entries:")
			fmt.Println(content)
		}
		return
	}

	fmt.Printf("🟢 Magabot is running (PID: %d)\n", pid)
	fmt.Printf("   Config: %s\n", configFile)
	fmt.Printf("   Logs:   %s\n", logFile)

	// DB size
	dbPath := filepath.Join(dataDir, "magabot.db")
	if info, err := os.Stat(dbPath); err == nil {
		fmt.Printf("   DB:     %.2f KB\n", float64(info.Size())/1024)
	}

	// Load config for detailed status
	cfg, err := config.Load(configFile)
	if err != nil {
		return
	}

	// System
	fmt.Printf("\n   System:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("   Version:   v%s\n", version.Short())
	fmt.Printf("   Go:        %s\n", runtime.Version())

	// LLM
	fmt.Println("\n   LLM:")
	fmt.Printf("     Provider: %s\n", cfg.LLM.Main)
	if cfg.LLM.Anthropic.Enabled {
		fmt.Printf("     Model:    %s\n", cfg.LLM.Anthropic.Model)
		if cfg.LLM.Anthropic.Mode == "cli" {
			fmt.Printf("     Auth:     Claude CLI\n")
		}
	}
	if cfg.LLM.OpenAI.Enabled {
		fmt.Printf("     OpenAI:   %s\n", cfg.LLM.OpenAI.Model)
	}
	if cfg.LLM.Gemini.Enabled {
		fmt.Printf("     Gemini:   %s\n", cfg.LLM.Gemini.Model)
	}

	// Platforms
	fmt.Println("\n   Platforms:")
	if cfg.Platforms.Telegram != nil && cfg.Platforms.Telegram.Enabled {
		fmt.Printf("     telegram: enabled\n")
	}
	if cfg.Platforms.Discord != nil && cfg.Platforms.Discord.Enabled {
		fmt.Printf("     discord:  enabled\n")
	}
	if cfg.Platforms.Slack != nil && cfg.Platforms.Slack.Enabled {
		fmt.Printf("     slack:    enabled\n")
	}
	if cfg.Platforms.WhatsApp != nil && cfg.Platforms.WhatsApp.Enabled {
		fmt.Printf("     whatsapp: enabled\n")
	}

	// User stats from DB
	store, err := storage.New(cfg.GetDatabasePath())
	if err == nil {
		defer func() { _ = store.Close() }()
		if stats, err := store.Stats(); err == nil {
			if userCounts, ok := stats["users"].(map[string]int64); ok && len(userCounts) > 0 {
				fmt.Println("\n   Users:")
				for platform, count := range userCounts {
					fmt.Printf("     %s: %d\n", platform, count)
				}
			}
		}
	}
}

// cmdLog shows logs (platform-specific: tail -f on Unix, manual read on Windows)
func cmdLog() {
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		fmt.Println("❌ No log file found")
		return
	}

	tailLogFile(logFile)
}

// cmdSetup is defined in setup_cmd.go

// cmdReset resets config to default (keeps platform connections)
func cmdReset() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("⚠️  This will reset your config, sessions, and database. Continue? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("Reset canceled.")
		return
	}

	// Stop if running
	if isRunning() {
		fmt.Println("🛑 Stopping magabot...")
		cmdStop()
	}

	// Backup current config
	if _, err := os.Stat(configFile); err == nil {
		backupFile := configFile + ".backup"
		if err := os.Rename(configFile, backupFile); err != nil {
			fmt.Printf("Warning: failed to backup config: %v\n", err)
		} else {
			fmt.Printf("📦 Config backed up to: %s\n", backupFile)
		}
	}

	// Remove sessions
	sessionsDir := filepath.Join(dataDir, "sessions")
	if err := os.RemoveAll(sessionsDir); err != nil {
		fmt.Printf("Warning: failed to remove sessions: %v\n", err)
	} else {
		fmt.Println("🗑️  Sessions cleared")
	}

	// Remove database
	dbFile := filepath.Join(dataDir, "db", "magabot.db")
	if err := os.Remove(dbFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to remove database: %v\n", err)
	} else {
		fmt.Println("🗑️  Database cleared")
	}

	// Remove secrets
	secretsFile := filepath.Join(configDir, "secrets.json")
	if err := os.Remove(secretsFile); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to remove secrets: %v\n", err)
	} else {
		fmt.Println("🗑️  Secrets cleared")
	}

	// Clear log file
	if err := os.Truncate(logFile, 0); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to clear log file: %v\n", err)
	} else {
		fmt.Println("🗑️  Log cleared")
	}

	// Recreate directories
	ensureDirs()

	// Generate fresh config.yaml with defaults
	freshCfg := &config.Config{}
	data, err := yaml.Marshal(freshCfg)
	if err != nil {
		fmt.Printf("Warning: failed to generate fresh config: %v\n", err)
	} else {
		header := "# Magabot configuration (generated by magabot reset)\n# Run 'magabot setup' to configure platforms and LLM providers.\n\n"
		if err := os.WriteFile(configFile, append([]byte(header), data...), 0600); err != nil {
			fmt.Printf("Warning: failed to write config: %v\n", err)
		} else {
			fmt.Printf("📄 Fresh config created: %s\n", configFile)
		}
	}

	fmt.Println("✅ Reset complete. Run 'magabot setup' to reconfigure.")
}

// cmdUninstall completely removes magabot
func cmdUninstall() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("⚠️  This will completely remove magabot and all data. Continue? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("Uninstall canceled.")
		return
	}

	// Stop if running
	cmdStop()

	// Remove config directory
	if err := os.RemoveAll(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to remove config: %v\n", err)
	}

	fmt.Println("✅ Magabot uninstalled")
	fmt.Println("   To reinstall, run: magabot setup")
}

// cmdGenKey generates an encryption key
func cmdGenKey() {
	key := security.GenerateKey()
	fmt.Printf("Generated key: %s\n", key)
	fmt.Println("Add this to your config.yaml under security.encryption_key")
}

// Helper functions

func ensureDirs() {
	dirs := []string{
		configDir,
		dataDir,
		logDir,
		filepath.Join(dataDir, "sessions"),
		filepath.Join(dataDir, "backups"),
		filepath.Join(dataDir, "db"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create directory %s: %v\n", dir, err)
		}
	}
}

func getPID() int {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func isRunning() bool {
	pid := getPID()
	return pid != 0 && processExists(pid)
}

// processExists, readLastLines, startDaemonProcess, stopProcess, tailLogFile
// are defined in commands_unix.go and commands_windows.go
