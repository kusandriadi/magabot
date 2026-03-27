package util

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultMaxBodySize is the default max response body size (1MB).
const DefaultMaxBodySize int64 = 1 << 20

// DefaultUserAgent is the default User-Agent header for outgoing requests.
const DefaultUserAgent = "Magabot/1.0"

// DoGET creates and executes an HTTP GET request with the given headers.
// Caller is responsible for closing resp.Body.
func DoGET(ctx context.Context, client *http.Client, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return client.Do(req)
}

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
		return nil, fmt.Errorf("%s error %d: %s", context, resp.StatusCode, SanitizeErrorMessage(string(body)))
	}
	return body, nil
}

// DoPostJSON marshals payload as JSON, sends a POST request, and checks for
// a non-success status code. Optional headers are applied to the request.
// Returns the response body on success.
func DoPostJSON(ctx context.Context, client *http.Client, url string, payload interface{}, headers map[string]string) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := ReadHTTPBody(resp, 0)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, SanitizeErrorMessage(string(body)))
	}

	return body, nil
}
