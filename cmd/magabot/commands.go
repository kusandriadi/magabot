package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kusa/magabot/internal/security"
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
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0600)

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

	os.Remove(pidFile)
	fmt.Println("✅ Magabot stopped")
}

// cmdRestart restarts the daemon
func cmdRestart() {
	cmdStop()
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

	// Show some stats
	if info, err := os.Stat(filepath.Join(dataDir, "magabot.db")); err == nil {
		fmt.Printf("   DB:     %.2f KB\n", float64(info.Size())/1024)
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
	fmt.Print("⚠️  This will reset your config but keep platform sessions. Continue? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("Reset cancelled.")
		return
	}

	// Backup current config
	if _, err := os.Stat(configFile); err == nil {
		backupFile := configFile + ".backup"
		if err := os.Rename(configFile, backupFile); err != nil {
			fmt.Printf("Warning: failed to backup config: %v\n", err)
		} else {
			fmt.Printf("Config backed up to: %s\n", backupFile)
		}
	}

	// Run setup
	cmdSetup()
}

// cmdUninstall completely removes magabot
func cmdUninstall() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("⚠️  This will completely remove magabot and all data. Continue? [y/N]: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("Uninstall cancelled.")
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
	os.MkdirAll(configDir, 0700)
	os.MkdirAll(dataDir, 0700)
	os.MkdirAll(logDir, 0700)
	os.MkdirAll(filepath.Join(dataDir, "sessions"), 0700)
	os.MkdirAll(filepath.Join(dataDir, "backups"), 0700)
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
