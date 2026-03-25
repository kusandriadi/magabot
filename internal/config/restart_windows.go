//go:build windows

package config

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kusa/magabot/internal/util"
)

// RestartBot restarts the magabot process on Windows by stopping and re-launching.
func RestartBot(pidFile string) error {
	pid, err := util.ReadPID(pidFile)
	if err != nil {
		return err
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
