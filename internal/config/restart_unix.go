//go:build !windows

package config

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// RestartBot restarts the magabot process by sending SIGHUP.
func RestartBot(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %w", err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send restart signal: %w", err)
	}

	return nil
}
