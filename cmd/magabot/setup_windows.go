//go:build windows

package main

// signalReload on Windows - no SIGHUP, return false to trigger full restart
func signalReload(pid int) bool {
	// Windows doesn't support SIGHUP, always do full restart
	return false
}

// processExists is defined in commands_windows.go
