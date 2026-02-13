//go:build windows

package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RestartBot restarts the magabot process on Windows by stopping and re-launching.
func RestartBot(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID: %w", err)
	}

	// On Windows, use taskkill /F /PID to stop the running process
	if err := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid)).Run(); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Re-launch the daemon
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	cmd := exec.Command(exePath, "daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart daemon: %w", err)
	}

	return nil
}
