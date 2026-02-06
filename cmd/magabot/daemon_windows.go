//go:build windows

package main

import (
	"log/slog"
	"os"
	"os/signal"

	"github.com/kusa/magabot/internal/router"
)

func registerSignals(sigCh chan<- os.Signal) {
	signal.Notify(sigCh, os.Interrupt)
}

func handleReloadSignal(_ os.Signal, _ *router.Router, _ *slog.Logger) bool {
	// Windows does not support SIGHUP; reload is not available
	return false
}
