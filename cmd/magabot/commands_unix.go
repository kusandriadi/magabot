//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func startDaemonProcess() (int, error) {
	exePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("cannot determine executable path: %w", err)
	}

	cmd := exec.Command(exePath, "daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func stopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func tailLogFile(logFile string) {
	cmd := exec.Command("tail", "-f", "--", logFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func readLastLines(filename string, n int) (string, error) {
	if n <= 0 {
		n = 10
	}
	cmd := exec.Command("tail", fmt.Sprintf("-%d", n), "--", filename)
	output, err := cmd.Output()
	return string(output), err
}
