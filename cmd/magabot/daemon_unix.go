//go:build !windows

package main

import (
	"log/slog"
	"os"
	"os/signal"
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

		executable, _ := os.Executable()
		args := []string{executable, "daemon"}
		env := os.Environ()

		if err := syscall.Exec(executable, args, env); err != nil {
			logger.Error("restart failed", "error", err)
			os.Exit(1)
		}
		return true
	}
	return false
}
