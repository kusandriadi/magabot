package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/updater"
	"github.com/kusa/magabot/internal/version"
)

const (
	repoOwner = "kusandriadi"
	repoName  = "magabot"
)

func cmdUpdate() {
	if len(os.Args) < 3 {
		cmdUpdateCheck()
		return
	}

	subCmd := os.Args[2]

	switch subCmd {
	case "check":
		cmdUpdateCheck()
	case "apply", "install":
		cmdUpdateApply()
	case "rollback":
		cmdUpdateRollback()
	case "help":
		cmdUpdateHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown update command: %s\n", subCmd)
		cmdUpdateHelp()
		os.Exit(1)
	}
}

func cmdUpdateCheck() {
	fmt.Printf("🔍 Checking for updates...\n\n")
	fmt.Printf("Current version: %s\n", version.Short())

	u := updater.New(updater.Config{
		RepoOwner:      repoOwner,
		RepoName:       repoName,
		CurrentVersion: version.Short(),
		BinaryName:     "magabot",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	release, hasUpdate, err := u.CheckUpdate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to check updates: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(updater.FormatReleaseInfo(release, hasUpdate))

	if hasUpdate {
		fmt.Println("\n💡 Run 'magabot update apply' to update")
	}
}

func cmdUpdateApply() {
	fmt.Printf("🔄 Checking for updates...\n")

	u := updater.New(updater.Config{
		RepoOwner:      repoOwner,
		RepoName:       repoName,
		CurrentVersion: version.Short(),
		BinaryName:     "magabot",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	release, hasUpdate, err := u.CheckUpdate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to check updates: %v\n", err)
		os.Exit(1)
	}

	if !hasUpdate {
		fmt.Println("✅ Already up to date!")
		return
	}

	fmt.Printf("\n📦 New version available: %s → %s\n", version.Short(), release.TagName)
	fmt.Printf("\n📝 Release Notes:\n%s\n", truncateNotes(release.Body, 300))

	// Confirm
	fmt.Print("\nDo you want to update? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "y" && confirm != "yes" {
		fmt.Println("Canceled.")
		return
	}

	// Resolve executable path BEFORE update (after update, os.Executable
	// follows the inode which points to the renamed .backup file)
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Cannot determine executable path: %v\n", err)
		os.Exit(1)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Cannot resolve executable path: %v\n", err)
		os.Exit(1)
	}

	// Stop bot if running
	fmt.Println("\n⏳ Stopping bot if running...")
	stopIfRunning()

	// Download and apply update
	fmt.Println("⬇️  Downloading update...")
	if err := u.Update(ctx, release); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Update failed: %v\n", err)
		fmt.Println("💡 Run 'magabot update rollback' to restore previous version")
		os.Exit(1)
	}

	fmt.Println("✅ Update successful!")
	fmt.Printf("📦 Version: %s → %s\n", version.Short(), release.TagName)
	fmt.Println("💡 Run 'magabot update rollback' if you encounter issues")

	// Auto-start the new version using the resolved path (not os.Executable
	// which now points to the .backup file)
	fmt.Println("\n🚀 Starting new version...")
	startUpdatedDaemon(execPath)
}

func cmdUpdateRollback() {
	fmt.Println("🔙 Rolling back to previous version...")

	u := updater.New(updater.Config{
		RepoOwner:      repoOwner,
		RepoName:       repoName,
		CurrentVersion: version.Short(),
		BinaryName:     "magabot",
	})

	if err := u.Rollback(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Rollback failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Rollback successful!")
	fmt.Println("💡 Run 'magabot start' to start the previous version")
}

func cmdUpdateHelp() {
	fmt.Println(`Update Management

Usage: magabot update <command>

Commands:
  check       Check for available updates
  apply       Download and install update
  rollback    Restore previous version
  help        Show this help

Update Process:
  1. 'magabot update check' - Check if update available
  2. 'magabot update apply' - Download and install
  3. 'magabot start' - Start new version

If issues occur:
  'magabot update rollback' - Restore previous version

Notes:
  - Bot will be stopped during update
  - Previous version is kept as backup
  - Rollback available until next update`)
}

func stopIfRunning() {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return
	}

	if !processExists(pid) {
		_ = os.Remove(pidFile)
		return
	}

	// Uses platform-specific stopProcess from commands_unix.go / commands_windows.go
	if err := stopProcess(pid); err != nil {
		return
	}

	// Wait for process to actually exit (max 10 seconds)
	for i := 0; i < 100; i++ {
		if !processExists(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = os.Remove(pidFile)
}

// startUpdatedDaemon starts the daemon using an explicit binary path.
// This is needed after update because os.Executable() would resolve to
// the .backup file (Linux/macOS tracks by inode, not path).
func startUpdatedDaemon(binPath string) {
	pid, err := startDaemonProcessAt(binPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start: %v\n", err)
		fmt.Println("💡 Run 'magabot start' manually")
		return
	}

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write PID file: %v\n", err)
	}

	fmt.Printf("✅ Magabot started (PID: %d)\n", pid)
}

func truncateNotes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
