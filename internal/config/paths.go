// Package config - Path management utilities
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureDirectories creates all configured directories with proper permissions
func (c *Config) EnsureDirectories() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{c.Paths.DataDir, 0700},
		{c.Paths.LogsDir, 0700},
		{c.Paths.MemoryDir, 0700},
		{c.Paths.CacheDir, 0700},
		{c.Paths.ExportsDir, 0700},
		{c.Paths.DownloadsDir, 0700},
		{c.Skills.Dir, 0755}, // Skills can be shared
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return fmt.Errorf("create %s: %w", d.path, err)
		}
	}

	return nil
}

// GetDatabasePath returns the full path to the SQLite database
func (c *Config) GetDatabasePath() string {
	if c.Storage.Database != "" {
		return expandPath(c.Storage.Database)
	}
	return filepath.Join(c.Paths.DataDir, "db", "magabot.db")
}

// GetSecurityLogPath returns the path to the security log
func (c *Config) GetSecurityLogPath() string {
	return filepath.Join(c.Paths.LogsDir, "security.log")
}

// GetMainLogPath returns the path to the main log
func (c *Config) GetMainLogPath() string {
	return filepath.Join(c.Paths.LogsDir, "magabot.log")
}

// GetBackupDir returns the backup directory path
func (c *Config) GetBackupDir() string {
	if c.Storage.Backup.Path != "" {
		return expandPath(c.Storage.Backup.Path)
	}
	return filepath.Join(c.Paths.DataDir, "backups")
}

// PrintPaths prints all configured paths (for debugging)
func (c *Config) PrintPaths() {
	fmt.Println("📁 Configured Paths:")
	fmt.Printf("   Data:      %s\n", c.Paths.DataDir)
	fmt.Printf("   Logs:      %s\n", c.Paths.LogsDir)
	fmt.Printf("   Memory:    %s\n", c.Paths.MemoryDir)
	fmt.Printf("   Cache:     %s\n", c.Paths.CacheDir)
	fmt.Printf("   Exports:   %s\n", c.Paths.ExportsDir)
	fmt.Printf("   Downloads: %s\n", c.Paths.DownloadsDir)
	fmt.Printf("   Skills:    %s\n", c.Skills.Dir)
	fmt.Printf("   Database:  %s\n", c.GetDatabasePath())
}
