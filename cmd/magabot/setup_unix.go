//go:build !windows

package main

import (
	"os"
	"syscall"
)

// signalReload sends SIGHUP to the process for config reload
func signalReload(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.SIGHUP)
	return err == nil
}

// processExists is defined in commands_unix.go
