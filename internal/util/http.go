package util

import (
	"fmt"
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

// ReadHTTPResponse reads the body and checks for a non-OK status code.
// Returns the body bytes on success, or an error describing the API failure.
func ReadHTTPResponse(resp *http.Response, context string) ([]byte, error) {
	body, err := ReadHTTPBody(resp, 0)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", context, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s error %d: %s", context, resp.StatusCode, string(body))
	}
	return body, nil
}
