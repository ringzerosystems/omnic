package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func setupWhatsAppTestServer() (*echo.Echo, *handler.Handler, *service.WhatsAppService, *memory.Store) {
	e := echo.New()
	store := memory.NewStore()
	broker := service.NewMemoryEventBroker()
	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	dispatcher := service.NewWebhookDispatcher(store, "token")

	//nolint:gosec // G101: test credentials
	cfg := service.WhatsAppConfig{
		PhoneNumberID: "100000000000001",
		WABAID:        "200000000000001",
		VerifyToken:   "my_verify_token",
		BearerToken:   "wa_token_dev",
		DeliveryDelay: 10 * time.Millisecond,
	}
	waSvc := service.NewWhatsAppService(store, broker, cfg)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, dispatcher)
	h.SetBroker(broker)
	h.SetWhatsApp(waSvc)
	h.RegisterRoutes(e)

	return e, h, waSvc, store
}

func TestWhatsAppWebhookVerification(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupWhatsAppTestServer()

	// 1. Success verification
	req := httptest.NewRequest(http.MethodGet, "/webhook?hub.mode=subscribe&hub.challenge=test_challenge_123&hub.verify_token=my_verify_token", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "test_challenge_123" {
		t.Fatalf("expected test_challenge_123, got %s", rec.Body.String())
	}

	// 2. Failed verification
	reqFail := httptest.NewRequest(http.MethodGet, "/webhook?hub.mode=subscribe&hub.challenge=test_challenge_123&hub.verify_token=wrong_token", nil)
	recFail := httptest.NewRecorder()
	e.ServeHTTP(recFail, reqFail)

	if recFail.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recFail.Code)
	}
}

func TestWhatsAppOutboundMessages(t *testing.T) {
	t.Parallel()
	e, _, _, store := setupWhatsAppTestServer()

	// Outbound text message
	payload := `{
		"messaging_product": "whatsapp",
		"recipient_type": "individual",
		"to": "+14155551111",
		"type": "text",
		"text": {"body": "Hello from Cloud API"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v26.0/100000000000001/messages", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wa_token_dev")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		MessagingProduct string `json:"messaging_product"`
		Messages         []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(res.Messages) != 1 || !strings.HasPrefix(res.Messages[0].ID, "wamid.") {
		t.Fatalf("unexpected messages in response: %+v", res)
	}

	// Verify persistence in store
	wamid := res.Messages[0].ID
	msg, err := store.GetMessageByEngineID(context.Background(), domain.EngineWhatsApp, wamid)
	if err != nil || msg.Body != "Hello from Cloud API" {
		t.Fatalf("message not found in store: %+v", msg)
	}
}

func TestWhatsAppMediaUploadAndDownload(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupWhatsAppTestServer()

	// 1. Upload media
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "audio.ogg")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	_, _ = io.WriteString(part, "dummy_ogg_data")
	_ = writer.WriteField("messaging_product", "whatsapp")
	_ = writer.WriteField("type", "audio/ogg")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v21.0/100000000000001/media", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer wa_token_dev")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var uploadRes struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &uploadRes)
	if uploadRes.ID == "" {
		t.Fatalf("missing media ID in upload response: %s", rec.Body.String())
	}

	// 2. Download media
	reqDl := httptest.NewRequest(http.MethodGet, "/v21.0/media/"+uploadRes.ID+"/download", nil)
	reqDl.Header.Set("Authorization", "Bearer wa_token_dev")
	recDl := httptest.NewRecorder()
	e.ServeHTTP(recDl, reqDl)

	if recDl.Code != http.StatusOK {
		t.Fatalf("expected 200 download, got %d", recDl.Code)
	}
	if recDl.Body.String() != "dummy_ogg_data" {
		t.Fatalf("unexpected media content: %s", recDl.Body.String())
	}
}

func TestEmuInboundWhatsApp(t *testing.T) {
	t.Parallel()
	e, _, _, store := setupWhatsAppTestServer()

	inboundPayload := `{
		"from": "+14155551111",
		"to": "+14155550199",
		"type": "interactive",
		"interactive_type": "button_reply",
		"selected_id": "btn_confirm",
		"selected_title": "Confirm"
	}`
	req := httptest.NewRequest(http.MethodPost, "/emu/inbound/whatsapp", strings.NewReader(inboundPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	list, total, err := store.ListMessages(context.Background(), "200000000000001", domain.MessageFilter{Engine: domain.EngineWhatsApp})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 inbound message, got total=%d err=%v", total, err)
	}
	if list[0].InteractiveContent == nil || list[0].InteractiveContent.SelectedID != "btn_confirm" {
		t.Fatalf("unexpected interactive content: %+v", list[0].InteractiveContent)
	}
}

func TestWhatsAppMarkAsRead(t *testing.T) {
	t.Parallel()
	e, _, waSvc, store := setupWhatsAppTestServer()

	// 1. Ingest an inbound message
	inboundMsg, err := waSvc.DispatchInbound(context.Background(), service.InboundWhatsAppParams{
		From: "+14155551111",
		To:   "+14155550199",
		Body: "Incoming query",
	})
	if err != nil {
		t.Fatalf("failed to dispatch inbound: %v", err)
	}

	// 2. Mark as read without 'to' field
	payload := `{"messaging_product":"whatsapp","status":"read","message_id":"` + inboundMsg.SID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v26.0/100000000000001/messages", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wa_token_dev")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res["success"] {
		t.Fatalf("expected {\"success\": true}, got: %s", rec.Body.String())
	}

	// 3. Verify message status updated to read
	updated, err := store.GetMessageByEngineID(context.Background(), domain.EngineWhatsApp, inboundMsg.SID)
	if err != nil {
		t.Fatalf("failed to get updated message: %v", err)
	}
	if updated.Status != domain.StatusRead {
		t.Fatalf("expected status %s, got %s", domain.StatusRead, updated.Status)
	}

	// 4. Missing message_id fails
	badPayload := `{"messaging_product":"whatsapp","status":"read"}`
	badReq := httptest.NewRequest(http.MethodPost, "/v26.0/100000000000001/messages", strings.NewReader(badPayload))
	badReq.Header.Set("Content-Type", "application/json")
	badReq.Header.Set("Authorization", "Bearer wa_token_dev")
	badRec := httptest.NewRecorder()
	e.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing message_id, got %d", badRec.Code)
	}
}
