//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func startDaemonProcess() (int, error) {
	exePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("cannot determine executable path: %w", err)
	}

	cmd := exec.Command(exePath, "daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func stopProcess(pid int) error {
	// Use /F for force kill — console processes don't respond to WM_CLOSE
	return exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid)).Run()
}

func processExists(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// CSV format: "process.exe","PID","Session Name","Session#","Mem Usage"
	// Check for exact PID match in CSV field
	pidStr := strconv.Itoa(pid)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			// Strip quotes from PID field
			fieldPID := strings.Trim(fields[1], "\" ")
			if fieldPID == pidStr {
				return true
			}
		}
	}
	return false
}

func tailLogFile(logFile string) {
	// Use PowerShell with properly escaped path argument
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"Get-Content", "-Path", logFile, "-Tail", "20", "-Wait")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func readLastLines(filename string, n int) (string, error) {
	if n <= 0 {
		n = 10
	}

	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		// Keep only the last n+buffer lines to limit memory usage
		if len(lines) > n*2 {
			lines = lines[len(lines)-n:]
		}
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), scanner.Err()
}
