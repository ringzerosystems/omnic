package service_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func TestWhatsAppServiceOutbound(t *testing.T) {
	t.Parallel()

	var webhookCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCount.Add(1)
		sig := r.Header.Get("X-Hub-Signature-256")
		if !strings.HasPrefix(sig, "sha256=") {
			t.Errorf("missing or invalid hub signature: %s", sig)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Errorf("empty webhook body")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := memory.NewStore()
	broker := service.NewMemoryEventBroker()
	cfg := service.WhatsAppConfig{
		PhoneNumberID: "100000000000001",
		WABAID:        "200000000000001",
		DisplayNumber: "+14155550199",
		AppSecret:     "test_secret",
		UpstreamURL:   server.URL,
		DeliveryDelay: 10 * time.Millisecond,
	}

	waService := service.NewWhatsAppService(store, broker, cfg)
	ctx := context.Background()

	// 1. Text message
	req := service.OutboundMessageRequest{
		MessagingProduct: "whatsapp",
		To:               "+14155551111",
		Type:             "text",
		Text:             &service.OutboundText{Body: "Hello WhatsApp"},
	}

	res, err := waService.SendOutbound(ctx, cfg.PhoneNumberID, req)
	if err != nil {
		t.Fatalf("SendOutbound text failed: %v", err)
	}
	if len(res.Messages) != 1 || !strings.HasPrefix(res.Messages[0].ID, "wamid.") {
		t.Fatalf("unexpected response: %+v", res)
	}

	// Verify persistence
	wamid := res.Messages[0].ID
	msg, err := store.GetMessageByEngineID(ctx, domain.EngineWhatsApp, wamid)
	if err != nil {
		t.Fatalf("GetMessageByEngineID failed: %v", err)
	}
	if msg.Body != "Hello WhatsApp" || msg.Engine != domain.EngineWhatsApp {
		t.Errorf("unexpected msg: %+v", msg)
	}

	// 2. Interactive Buttons
	btnReq := service.OutboundMessageRequest{
		MessagingProduct: "whatsapp",
		To:               "+14155551111",
		Type:             "interactive",
		Interactive: &service.OutboundInteractive{
			Type: "button",
			Body: service.InteractiveBody{Text: "Please confirm:"},
			Action: service.InteractiveAction{
				Buttons: []service.ButtonWrapper{
					{Type: "reply", Reply: domain.ButtonItem{ID: "btn_yes", Title: "Yes"}},
					{Type: "reply", Reply: domain.ButtonItem{ID: "btn_no", Title: "No"}},
				},
			},
		},
	}
	btnRes, err := waService.SendOutbound(ctx, cfg.PhoneNumberID, btnReq)
	if err != nil {
		t.Fatalf("SendOutbound interactive failed: %v", err)
	}
	if len(btnRes.Messages) != 1 {
		t.Fatalf("unexpected btnRes: %+v", btnRes)
	}

	// 3. Validation error: 4 buttons
	invalidReq := btnReq
	invalidReq.Interactive.Action.Buttons = append(invalidReq.Interactive.Action.Buttons,
		service.ButtonWrapper{Type: "reply", Reply: domain.ButtonItem{ID: "3", Title: "3"}},
		service.ButtonWrapper{Type: "reply", Reply: domain.ButtonItem{ID: "4", Title: "4"}},
	)
	if _, err := waService.SendOutbound(ctx, cfg.PhoneNumberID, invalidReq); err == nil {
		t.Fatalf("expected error on 4 buttons, got nil")
	}

	// 4. Mark As Read
	if err := waService.MarkAsRead(ctx, wamid); err != nil {
		t.Fatalf("MarkAsRead failed: %v", err)
	}
	readMsg, err := store.GetMessageByEngineID(ctx, domain.EngineWhatsApp, wamid)
	if err != nil || readMsg == nil {
		t.Fatalf("GetMessageByEngineID failed: %v", err)
	}
	if readMsg.Status != domain.StatusRead {
		t.Errorf("expected status read, got %s", readMsg.Status)
	}

	// 5. Media Upload
	mediaID, err := waService.UploadMedia(ctx, cfg.PhoneNumberID, "audio/ogg", "note.ogg", []byte("fake_opus_audio"))
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}
	media, err := waService.GetMedia(ctx, mediaID)
	if err != nil || string(media.Data) != "fake_opus_audio" {
		t.Fatalf("GetMedia failed: got %v err %v", media, err)
	}

	// 6. Inbound WhatsApp Message
	inMsg, err := waService.DispatchInbound(ctx, service.InboundWhatsAppParams{
		From:            "+14155551111",
		To:              cfg.DisplayNumber,
		Type:            "interactive",
		InteractiveType: domain.InteractiveTypeButtonReply,
		SelectedID:      "btn_yes",
		SelectedTitle:   "Yes",
	})
	if err != nil {
		t.Fatalf("DispatchInbound failed: %v", err)
	}
	if inMsg.InteractiveContent == nil || inMsg.InteractiveContent.SelectedID != "btn_yes" {
		t.Errorf("unexpected inbound interactive content: %+v", inMsg.InteractiveContent)
	}

	// Wait for async delivery transitions
	time.Sleep(50 * time.Millisecond)
	if webhookCount.Load() == 0 {
		t.Errorf("expected webhooks to be dispatched to upstream server")
	}
}
