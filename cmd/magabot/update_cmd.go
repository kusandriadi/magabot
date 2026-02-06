package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/updater"
	"github.com/kusa/magabot/internal/version"
)

const (
	repoOwner = "kusa"
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
		fmt.Println("Cancelled.")
		return
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
	fmt.Println("\n💡 Run 'magabot start' to start the new version")
	fmt.Println("💡 Run 'magabot update rollback' if you encounter issues")
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

	// Uses platform-specific stopProcess from commands_unix.go / commands_windows.go
	if err := stopProcess(pid); err != nil {
		return
	}

	time.Sleep(2 * time.Second)
}

func truncateNotes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
