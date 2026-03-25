//go:build !windows

package config

import (
	"fmt"
	"os"
	"syscall"

	"github.com/kusa/magabot/internal/util"
)

// RestartBot restarts the magabot process by sending SIGHUP.
func RestartBot(pidFile string) error {
	pid, err := util.ReadPID(pidFile)
	if err != nil {
		return err
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
