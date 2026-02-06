// Package version provides version information
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version (set via ldflags)
	Version = "0.1.0"

	// GitCommit is the git commit hash (set via ldflags)
	GitCommit = "unknown"

	// BuildTime is the build timestamp (set via ldflags)
	BuildTime = "unknown"

	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
)

// Info returns formatted version info
func Info() string {
	return fmt.Sprintf("magabot %s (commit: %s, built: %s, %s)",
		Version, GitCommit, BuildTime, GoVersion)
}

// Short returns just the version number
func Short() string {
	return Version
}

// Full returns detailed version info as map
func Full() map[string]string {
	return map[string]string{
		"version":    Version,
		"git_commit": GitCommit,
		"build_time": BuildTime,
		"go_version": GoVersion,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}
}
