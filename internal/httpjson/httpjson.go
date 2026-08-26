// Package httpjson provides the JSON transport shared by model adapters.
package httpjson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Error describes a non-2xx provider response.
type Error struct {
	StatusCode int
	Body       string
}

// Error returns a compact provider failure.
// @param e provider error.
// @return formatted status and response body.
func (e *Error) Error() string {
	return fmt.Sprintf("provider returned HTTP %d: %s", e.StatusCode, e.Body)
}

// Post sends and receives one JSON document.
// @param ctx request cancellation context.
// @param client HTTP client; nil uses http.DefaultClient.
// @param url absolute endpoint URL.
// @param headers request headers.
// @param input request body.
// @param output response destination.
// @return transport, status, encoding, or decoding error.
func Post(ctx context.Context, client *http.Client, url string, headers map[string]string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Endpoint joins a base URL and API path.
// @param baseURL provider or gateway base URL.
// @param path endpoint path.
// @return absolute endpoint URL.
func Endpoint(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}
