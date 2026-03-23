//go:build !windows

package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kusa/magabot/internal/router"
)

func registerSignals(sigCh chan<- os.Signal) {
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
}

func handleReloadSignal(sig os.Signal, rtr *router.Router, logger *slog.Logger) bool {
	if sig == syscall.SIGHUP {
		logger.Info("SIGHUP received, restarting...")
		rtr.Stop()

		// os.Executable() reads /proc/self/exe which follows the inode,
		// not the path. After an update renames the binary to .backup,
		// /proc/self/exe points to the .backup file instead of the new
		// binary. Strip .backup suffixes to exec the updated binary.
		executable, _ := os.Executable()
		canonical := executable
		for strings.HasSuffix(canonical, ".backup") {
			canonical = strings.TrimSuffix(canonical, ".backup")
		}
		if canonical != executable {
			logger.Info("resolved executable path", "from", executable, "to", canonical)
		}

		args := []string{canonical, "daemon"}
		env := os.Environ()

		if err := syscall.Exec(canonical, args, env); err != nil {
			logger.Error("restart failed", "error", err)
			os.Exit(1)
		}
		return true
	}
	return false
}
