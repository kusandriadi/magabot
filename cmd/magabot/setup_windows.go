//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// signalReload on Windows - no SIGHUP, return false to trigger full restart
func signalReload(pid int) bool {
	// Windows doesn't support SIGHUP, always do full restart
	return false
}

// processExists checks if a process is running on Windows
func processExists(pid int) bool {
	// Use tasklist to check if process exists
	cmd := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// If process exists, output contains the PID
	return strings.Contains(string(output), strconv.Itoa(pid))
}
