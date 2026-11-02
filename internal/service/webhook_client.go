package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SharedWebhookHTTPClient provides a shared HTTP connection pool across all webhook dispatchers.
var SharedWebhookHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
}

// WebhookClient provides unified procedural HTTP execution for upstream webhook deliveries.
type WebhookClient struct {
	client *http.Client
}

// NewWebhookClient constructs a WebhookClient wrapping an HTTP client.
func NewWebhookClient(client ...*http.Client) *WebhookClient {
	c := SharedWebhookHTTPClient
	if len(client) > 0 && client[0] != nil {
		c = client[0]
	}
	return &WebhookClient{client: c}
}

// Post sends an HTTP POST request with headers and drains the response body.
func (w *WebhookClient) Post(ctx context.Context, targetURL, contentType string, body []byte, headers map[string]string) (*http.Response, []byte, error) {
	if targetURL == "" {
		return nil, nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("creating webhook request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("dispatching webhook: %w", err)
	}
	if resp == nil {
		return nil, nil, errors.New("empty http response from upstream webhook")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return resp, respBody, fmt.Errorf("upstream webhook returned status %d", resp.StatusCode)
	}
	return resp, respBody, nil
}
