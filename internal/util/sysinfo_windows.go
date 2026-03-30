//go:build windows

package util

// ServerStats holds system resource usage information.
// On Windows all fields are zero — collection is not implemented.
type ServerStats struct {
	DiskTotal    uint64
	DiskUsed     uint64
	DiskFree     uint64
	MemTotal     uint64
	MemUsed      uint64
	MemAvailable uint64
	LoadAvg1     float64
	LoadAvg5     float64
	LoadAvg15    float64
	HasGPU       bool
	GPUName      string
	GPUMemTotal  uint64
	GPUMemUsed   uint64
	GPUUtil      int
}

// GetServerStats returns empty stats on Windows.
func GetServerStats() ServerStats {
	return ServerStats{}
}
