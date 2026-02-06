//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func startDaemonProcess() (int, error) {
	cmd := exec.Command(os.Args[0], "daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func stopProcess(pid int) error {
	// Windows does not support SIGTERM; use taskkill for graceful stop
	cmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid))
	return cmd.Run()
}

func processExists(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), fmt.Sprintf("%d", pid))
}

func tailLogFile(logFile string) {
	// Windows: use PowerShell Get-Content -Wait (equivalent of tail -f)
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Get-Content -Path '%s' -Tail 20 -Wait", logFile))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func readLastLines(filename string, n int) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), scanner.Err()
}
