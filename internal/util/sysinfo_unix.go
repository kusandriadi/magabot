//go:build !windows

package util

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ServerStats holds system resource usage information.
type ServerStats struct {
	// Disk usage (bytes)
	DiskTotal uint64
	DiskUsed  uint64
	DiskFree  uint64

	// Memory (bytes)
	MemTotal     uint64
	MemUsed      uint64
	MemAvailable uint64

	// CPU — load averages (1, 5, 15 minute)
	LoadAvg1  float64
	LoadAvg5  float64
	LoadAvg15 float64

	// GPU (populated only when nvidia-smi is available)
	HasGPU      bool
	GPUName     string
	GPUMemTotal uint64 // bytes
	GPUMemUsed  uint64 // bytes
	GPUUtil     int    // percentage
}

// GetServerStats collects disk, memory, CPU load, and GPU usage.
func GetServerStats() ServerStats {
	var s ServerStats
	s.readDisk()
	s.readMemory()
	s.readLoadAvg()
	s.readGPU()
	return s
}

func (s *ServerStats) readDisk() {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err == nil {
		bs := uint64(st.Bsize)
		s.DiskTotal = st.Blocks * bs
		s.DiskFree = st.Bfree * bs
		s.DiskUsed = s.DiskTotal - s.DiskFree
	}
}

func (s *ServerStats) readMemory() {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	vals := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				vals[key] = v * 1024 // kB → bytes
			}
		}
	}

	s.MemTotal = vals["MemTotal"]
	s.MemAvailable = vals["MemAvailable"]
	if s.MemTotal >= s.MemAvailable {
		s.MemUsed = s.MemTotal - s.MemAvailable
	}
}

func (s *ServerStats) readLoadAvg() {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		s.LoadAvg1, _ = strconv.ParseFloat(fields[0], 64)
		s.LoadAvg5, _ = strconv.ParseFloat(fields[1], 64)
		s.LoadAvg15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

func (s *ServerStats) readGPU() {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total,memory.used,utilization.gpu",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i] // first GPU only
	}
	parts := strings.SplitN(line, ",", 4)
	if len(parts) < 4 {
		return
	}
	name := strings.TrimSpace(parts[0])
	memTotalMiB, err1 := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
	memUsedMiB, err2 := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
	util, err3 := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}
	s.HasGPU = true
	s.GPUName = name
	s.GPUMemTotal = memTotalMiB * 1024 * 1024
	s.GPUMemUsed = memUsedMiB * 1024 * 1024
	s.GPUUtil = util
}
