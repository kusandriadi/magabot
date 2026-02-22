// Media utilities for image processing
package util

import (
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// ImageData represents an image with mime type and data
type ImageData struct {
	MimeType string
	Data     []byte
}

// BuildImagesFromPaths creates image attachments from media file paths.
// allowedDir restricts which directory files may be read from (path traversal protection - A04, A10).
// Returns a slice of ImageData and any errors encountered.
func BuildImagesFromPaths(mediaPaths []string, allowedDir string, logger *slog.Logger) ([]ImageData, []error) {
	var images []ImageData
	var errors []error

	for _, path := range mediaPaths {
		// Resolve symlinks and get absolute path to prevent symlink traversal (A04, A10)
		absPath, err := filepath.Abs(path)
		if err != nil {
			logger.Warn("invalid media path", "path", path, "error", err)
			errors = append(errors, fmt.Errorf("invalid path %s: %w", path, err))
			continue
		}
		// EvalSymlinks resolves symlinks — prevents symlink-based directory escape
		absPath, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			logger.Warn("resolve symlink failed", "path", path, "error", err)
			errors = append(errors, fmt.Errorf("resolve path %s: %w", path, err))
			continue
		}

		absDir, err := filepath.Abs(allowedDir)
		if err != nil {
			logger.Warn("invalid allowed directory", "dir", allowedDir, "error", err)
			errors = append(errors, fmt.Errorf("invalid allowed directory: %w", err))
			continue
		}
		absDir, _ = filepath.EvalSymlinks(absDir) // resolve dir symlinks too

		// Ensure path is within allowed directory (prevent directory traversal)
		if !strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
			logger.Warn("media path outside allowed directory",
				"path", path,
				"allowed", allowedDir,
				"severity", "security")
			errors = append(errors, fmt.Errorf("path outside allowed directory: %s", path))
			continue
		}

		// Check file size before reading (prevent OOM - A04)
		info, err := os.Stat(absPath)
		if err != nil {
			logger.Warn("stat media file failed", "path", absPath, "error", err)
			errors = append(errors, fmt.Errorf("stat failed %s: %w", path, err))
			continue
		}

		const maxFileSize = 20 * 1024 * 1024 // 20MB max
		if info.Size() > maxFileSize {
			logger.Warn("media file too large",
				"path", absPath,
				"size", info.Size(),
				"max", maxFileSize)
			errors = append(errors, fmt.Errorf("file too large %s: %d bytes", path, info.Size()))
			continue
		}

		// Read file
		data, err := os.ReadFile(absPath) // #nosec G304 -- path is validated above
		if err != nil {
			logger.Warn("read media file failed", "path", absPath, "error", err)
			errors = append(errors, fmt.Errorf("read failed %s: %w", path, err))
			continue
		}

		// Determine MIME type
		mimeType := mime.TypeByExtension(filepath.Ext(absPath))
		if mimeType == "" {
			mimeType = "image/jpeg" // Default fallback
		}

		images = append(images, ImageData{
			MimeType: mimeType,
			Data:     data,
		})
	}

	return images, errors
}
