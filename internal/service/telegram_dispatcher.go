package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// TelegramDispatcher forwards webhook updates to an upstream bot server.
type TelegramDispatcher struct {
	client      *WebhookClient
	upstreamURL string
	secretToken string
	mu          sync.RWMutex
}

// NewTelegramDispatcher creates a new Telegram webhook dispatcher.
func NewTelegramDispatcher(upstreamURL, secretToken string) *TelegramDispatcher {
	return &TelegramDispatcher{
		upstreamURL: upstreamURL,
		secretToken: secretToken,
		client:      NewWebhookClient(),
	}
}

// SetUpstreamURL dynamically updates the webhook URL.
func (d *TelegramDispatcher) SetUpstreamURL(url string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upstreamURL = url
}

// SetSecretToken dynamically updates the secret token.
func (d *TelegramDispatcher) SetSecretToken(token string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.secretToken = token
}

// DispatchUpdate sends an update payload to the configured upstream bot webhook.
func (d *TelegramDispatcher) DispatchUpdate(ctx context.Context, update *TelegramUpdate) error {
	d.mu.RLock()
	targetURL := d.upstreamURL
	secret := d.secretToken
	d.mu.RUnlock()

	if targetURL == "" {
		return nil
	}

	bodyBytes, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("marshaling telegram update: %w", err)
	}

	headers := map[string]string{
		"User-Agent": "Telegram-Bot-Api-Emulator/1.0",
	}
	if secret != "" {
		headers["X-Telegram-Bot-Api-Secret-Token"] = secret
	}

	_, _, err = d.client.Post(ctx, targetURL, "application/json", bodyBytes, headers)
	return err
}
