package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestTwilioContractWireParity(t *testing.T) {
	e, _ := setupTestServer()

	// 1. Provision sender number
	form := url.Values{
		"PhoneNumber":  {"+14155550199"},
		"FriendlyName": {"Contract Test Sender"},
		"SmsUrl":       {"https://example.com/sms"},
	}
	req := httptest.NewRequest(http.MethodPost, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers.json", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Provision failed: %d: %s", rec.Code, rec.Body.String())
	}

	var pnResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &pnResp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify IncomingPhoneNumber wire contract
	requiredPNFields := []string{"sid", "account_sid", "phone_number", "friendly_name", "status", "sms_url", "sms_method", "capabilities", "date_created", "date_updated", "uri"}
	for _, f := range requiredPNFields {
		if _, ok := pnResp[f]; !ok {
			t.Errorf("missing required wire field in phone number: %s", f)
		}
	}

	// 2. Send SMS and verify Message instance schema
	msgForm := url.Values{
		"To":   {"+14155552671"},
		"From": {"+14155550199"},
		"Body": {"Hello from contract test!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/2010-04-01/Accounts/AC001/Messages.json", strings.NewReader(msgForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Send SMS failed: %d: %s", rec.Code, rec.Body.String())
	}

	var msgResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &msgResp); err != nil {
		t.Fatalf("Unmarshal message failed: %v", err)
	}

	// Verify Message instance schema per Section 2.2.2
	requiredMsgFields := []string{
		"sid", "account_sid", "messaging_service_sid", "to", "from", "body",
		"status", "num_segments", "num_media", "direction", "api_version",
		"price", "price_unit", "error_code", "error_message", "uri",
		"subresource_uris", "date_created", "date_updated", "date_sent",
	}
	for _, f := range requiredMsgFields {
		if _, ok := msgResp[f]; !ok {
			t.Errorf("missing required wire field in message: %s", f)
		}
	}

	// Verify RFC 2822 / RFC 1123 date format (e.g. "Wed, 16 Sep 2026 06:45:00 +0000")
	createdStr := msgResp["date_created"].(string)
	if _, err := time.Parse(time.RFC1123Z, createdStr); err != nil {
		t.Errorf("date_created does not conform to RFC 2822/1123Z: %s (err: %v)", createdStr, err)
	}

	// Verify subresource URIs structure
	subresources, ok := msgResp["subresource_uris"].(map[string]any)
	if !ok {
		t.Fatalf("subresource_uris missing or invalid type")
	}
	if _, ok := subresources["media"]; !ok {
		t.Errorf("subresource_uris missing 'media'")
	}
	if _, ok := subresources["feedback"]; !ok {
		t.Errorf("subresource_uris missing 'feedback'")
	}
}
