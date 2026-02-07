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

// processExists checks if a process is running
func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
