package util

import (
	"io"
	"net/http"
)

// DefaultMaxBodySize is the default max response body size (1MB).
const DefaultMaxBodySize int64 = 1 << 20

// ReadHTTPBody reads the response body with a size limit.
// Caller is responsible for closing resp.Body (typically via defer).
// maxSize defaults to 1MB if <= 0.
func ReadHTTPBody(resp *http.Response, maxSize int64) ([]byte, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxBodySize
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}
