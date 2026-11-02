package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
)

// WhatsAppDispatcher dispatches inbound webhooks and delivery status updates to an upstream server.
type WhatsAppDispatcher struct {
	client      *WebhookClient
	appSecret   string
	upstreamURL string
	mu          sync.RWMutex
}

// NewWhatsAppDispatcher creates a new WhatsApp webhook dispatcher.
func NewWhatsAppDispatcher(upstreamURL, appSecret string) *WhatsAppDispatcher {
	return &WhatsAppDispatcher{
		upstreamURL: upstreamURL,
		appSecret:   appSecret,
		client:      NewWebhookClient(),
	}
}

// SetUpstreamURL dynamically updates the webhook destination URL.
func (d *WhatsAppDispatcher) SetUpstreamURL(url string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upstreamURL = url
}

func computeHubSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// ValidateWhatsAppSignature verifies the X-Hub-Signature-256 header against the request body.
func ValidateWhatsAppSignature(body []byte, secret, expectedSig string) bool {
	expected := computeHubSignature(body, secret)
	return hmac.Equal([]byte(expected), []byte(expectedSig))
}

func sanitizeWAID(number string) string {
	cleaned := strings.TrimPrefix(number, "+")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	return cleaned
}

func buildInboundMessageMap(msg *domain.Message) map[string]any {
	fromWAID := sanitizeWAID(msg.From)
	m := map[string]any{
		"from":      fromWAID,
		"id":        msg.SID,
		"timestamp": strconv.FormatInt(msg.DateCreated.Unix(), 10),
	}

	if msg.InteractiveContent != nil {
		m["type"] = "interactive"
		ic := msg.InteractiveContent
		switch ic.Type {
		case domain.InteractiveTypeButtonReply:
			m["interactive"] = map[string]any{
				"type": "button_reply",
				"button_reply": map[string]string{
					"id":    ic.SelectedID,
					"title": ic.Body,
				},
			}
		case domain.InteractiveTypeListReply:
			m["interactive"] = map[string]any{
				"type": "list_reply",
				"list_reply": map[string]string{
					"id":    ic.SelectedID,
					"title": ic.Body,
				},
			}
		}
		return m
	}

	if msg.VoiceMetadata != nil && msg.VoiceMetadata.IsVoiceNote {
		m["type"] = "audio"
		mediaID := "wa_media_" + msg.SID
		if len(msg.MediaURLs) > 0 && msg.MediaURLs[0] != "" {
			mediaID = msg.MediaURLs[0]
		}
		m["audio"] = map[string]any{
			"mime_type": "audio/ogg; codecs=opus",
			"sha256":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"id":        mediaID,
			"voice":     true,
		}
		return m
	}

	m["type"] = "text"
	m["text"] = map[string]string{
		"body": msg.Body,
	}
	return m
}

func (d *WhatsAppDispatcher) buildInboundPayload(wabaID, phoneID, displayNumber string, msg *domain.Message) ([]byte, error) {
	fromWAID := sanitizeWAID(msg.From)
	payload := map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{
			map[string]any{
				"id": wabaID,
				"changes": []any{
					map[string]any{
						"value": map[string]any{
							"messaging_product": "whatsapp",
							"metadata": map[string]string{
								"display_phone_number": displayNumber,
								"phone_number_id":      phoneID,
							},
							"contacts": []any{
								map[string]any{
									"profile": map[string]string{"name": "Handset User"},
									"wa_id":   fromWAID,
								},
							},
							"messages": []any{
								buildInboundMessageMap(msg),
							},
						},
						"field": "messages",
					},
				},
			},
		},
	}
	return json.Marshal(payload)
}

func (d *WhatsAppDispatcher) postToUpstream(ctx context.Context, body []byte) error {
	d.mu.RLock()
	targetURL := d.upstreamURL
	secret := d.appSecret
	d.mu.RUnlock()

	if targetURL == "" {
		return nil
	}

	headers := map[string]string{
		"User-Agent": "Meta-WhatsApp-Cloud-API/v21.0",
	}
	if secret != "" {
		headers["X-Hub-Signature-256"] = computeHubSignature(body, secret)
	}

	_, _, err := d.client.Post(ctx, targetURL, "application/json", body, headers)
	return err
}

// DispatchInbound sends an inbound message payload to the upstream webhook.
func (d *WhatsAppDispatcher) DispatchInbound(ctx context.Context, wabaID, phoneID, displayNumber string, msg *domain.Message) error {
	body, err := d.buildInboundPayload(wabaID, phoneID, displayNumber, msg)
	if err != nil {
		return err
	}
	return d.postToUpstream(ctx, body)
}

// WhatsAppStatusParams carries delivery status parameters for webhook dispatching.
type WhatsAppStatusParams struct {
	WABAID        string
	PhoneID       string
	DisplayNumber string
	WAMID         string
	Status        string
	RecipientID   string
}

// DispatchStatus sends a delivery status update to the upstream webhook.
func (d *WhatsAppDispatcher) DispatchStatus(ctx context.Context, params WhatsAppStatusParams) error {
	payload := map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{
			map[string]any{
				"id": params.WABAID,
				"changes": []any{
					map[string]any{
						"value": map[string]any{
							"messaging_product": "whatsapp",
							"metadata": map[string]string{
								"display_phone_number": params.DisplayNumber,
								"phone_number_id":      params.PhoneID,
							},
							"statuses": []any{
								map[string]string{
									"id":           params.WAMID,
									"status":       params.Status,
									"timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
									"recipient_id": sanitizeWAID(params.RecipientID),
								},
							},
						},
						"field": "messages",
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return d.postToUpstream(ctx, body)
}
