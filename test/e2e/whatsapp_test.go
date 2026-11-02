package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

type mockWhatsAppAgentState struct {
	inboundType    string
	inboundButton  string
	inboundList    string
	inboundAudioID string
	mu             sync.Mutex
	inboundSigOK   bool
	statusSent     bool
	statusDeliv    bool
	statusRead     bool
}

func setupWhatsAppEmuServer(secret, token, upstreamURL string) (*httptest.Server, *service.WhatsAppService) {
	store := memory.NewStore()
	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	twilioDispatcher := service.NewWebhookDispatcher(store, "twilio_auth_token")

	ctx := context.Background()
	_, _ = phoneSvc.Provision(ctx, "AC001", service.ProvisionParams{
		PhoneNumber:  "+14155550199",
		FriendlyName: "Test Number",
		SmsURL:       upstreamURL,
		SmsMethod:    "POST",
	})

	broker := service.NewMemoryEventBroker()
	msgSvc.SetBroker(broker)

	waConfig := service.WhatsAppConfig{
		PhoneNumberID: "100000000000001",
		WABAID:        "200000000000001",
		DisplayNumber: "+14155550199",
		BearerToken:   token,
		AppSecret:     secret,
		UpstreamURL:   upstreamURL,
		DeliveryDelay: 20 * time.Millisecond,
	}
	waSvc := service.NewWhatsAppService(store, broker, waConfig)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, twilioDispatcher)
	h.SetBroker(broker)
	h.SetWhatsApp(waSvc)

	e := echo.New()
	h.RegisterRoutes(e)
	return httptest.NewServer(e), waSvc
}

func setupMockWhatsAppAgent(state *mockWhatsAppAgentState, secret string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("X-Hub-Signature-256")
		valid := service.ValidateWhatsAppSignature(body, secret, sig)
		parseMockAgentBody(state, body, valid)
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(mux)
}

func parseMockAgentBody(state *mockWhatsAppAgentState, body []byte, valid bool) {
	var payload struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						Type        string `json:"type"`
						Interactive struct {
							Type        string `json:"type"`
							ButtonReply struct {
								ID    string `json:"id"`
								Title string `json:"title"`
							} `json:"button_reply"`
							ListReply struct {
								ID    string `json:"id"`
								Title string `json:"title"`
							} `json:"list_reply"`
						} `json:"interactive"`
						Audio struct {
							ID    string `json:"id"`
							Voice bool   `json:"voice"`
						} `json:"audio"`
					} `json:"messages"`
					Statuses []struct {
						Status string `json:"status"`
					} `json:"statuses"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	_ = json.Unmarshal(body, &payload)

	state.mu.Lock()
	defer state.mu.Unlock()
	state.inboundSigOK = valid
	if len(payload.Entry) == 0 || len(payload.Entry[0].Changes) == 0 {
		return
	}
	val := payload.Entry[0].Changes[0].Value
	if len(val.Messages) > 0 {
		m := val.Messages[0]
		state.inboundType = m.Type
		if m.Interactive.Type == "button_reply" {
			state.inboundButton = m.Interactive.ButtonReply.ID
		}
		if m.Interactive.Type == "list_reply" {
			state.inboundList = m.Interactive.ListReply.ID
		}
		if m.Audio.ID != "" {
			state.inboundAudioID = m.Audio.ID
		}
	}
	for _, st := range val.Statuses {
		switch st.Status {
		case "sent":
			state.statusSent = true
		case "delivered":
			state.statusDeliv = true
		case "read":
			state.statusRead = true
		}
	}
}

func TestE2EWhatsAppInteractiveButtons(t *testing.T) {
	secret := "test_wa_secret"
	token := "wa_token_dev" //nolint:gosec // test fixture token
	agentState := &mockWhatsAppAgentState{}
	agentSrv := setupMockWhatsAppAgent(agentState, secret)
	defer agentSrv.Close()

	emuSrv, _ := setupWhatsAppEmuServer(secret, token, agentSrv.URL+"/webhook")
	defer emuSrv.Close()

	outboundPayload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                "+14155551111",
		"type":              "interactive",
		"interactive": map[string]any{
			"type": "button",
			"body": map[string]string{"text": "Confirm appointment?"},
			"action": map[string]any{
				"buttons": []map[string]any{
					{"type": "reply", "reply": map[string]string{"id": "btn_yes", "title": "Yes"}},
					{"type": "reply", "reply": map[string]string{"id": "btn_no", "title": "No"}},
				},
			},
		},
	}
	b, _ := json.Marshal(outboundPayload)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		emuSrv.URL+"/v26.0/100000000000001/messages", bytes.NewBuffer(b))
	if err != nil || req == nil {
		t.Fatalf("failed creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("outbound message failed: %v", err)
	}
	_ = resp.Body.Close()

	time.Sleep(100 * time.Millisecond)

	agentState.mu.Lock()
	if !agentState.statusDeliv {
		t.Errorf("expected delivered status webhook, got sent=%v delivered=%v",
			agentState.statusSent, agentState.statusDeliv)
	}
	agentState.mu.Unlock()

	inboundPayload := map[string]any{
		"from":             "+14155551111",
		"to":               "+14155550199",
		"type":             "interactive",
		"interactive_type": "button_reply",
		"selected_id":      "btn_yes",
		"selected_title":   "Yes",
	}
	inBytes, _ := json.Marshal(inboundPayload)
	inResp, err := http.Post(emuSrv.URL+"/emu/inbound/whatsapp", "application/json", bytes.NewBuffer(inBytes))
	if err != nil || inResp == nil || inResp.StatusCode != http.StatusOK {
		t.Fatalf("inbound button reply failed: %v", err)
	}
	_ = inResp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	agentState.mu.Lock()
	defer agentState.mu.Unlock()
	if !agentState.inboundSigOK {
		t.Errorf("expected valid HMAC signature on inbound webhook")
	}
	if agentState.inboundButton != "btn_yes" {
		t.Errorf("expected inbound button 'btn_yes', got %q", agentState.inboundButton)
	}
}

func uploadTestMedia(t *testing.T, emuURL, token string) string {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "test_voice.ogg")
	if err != nil || part == nil {
		t.Fatalf("failed creating form file: %v", err)
	}
	_, _ = part.Write([]byte("OGG_OPUS_SAMPLE_AUDIO_DATA"))
	_ = w.WriteField("messaging_product", "whatsapp")
	_ = w.WriteField("type", "audio/ogg")
	_ = w.Close()

	uploadReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		emuURL+"/v26.0/100000000000001/media", &body)
	if err != nil || uploadReq == nil {
		t.Fatalf("failed creating upload request: %v", err)
	}
	uploadReq.Header.Set("Content-Type", w.FormDataContentType())
	uploadReq.Header.Set("Authorization", "Bearer "+token)

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil || uploadResp == nil || uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("media upload failed: %v", err)
	}
	var uploadRes struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadRes)
	_ = uploadResp.Body.Close()
	return uploadRes.ID
}

func verifyDownloadedMedia(t *testing.T, emuURL, token, mediaID string) {
	dlReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("%s/v26.0/media/%s/download", emuURL, mediaID), nil)
	if err != nil || dlReq == nil {
		t.Fatalf("failed creating download request: %v", err)
	}
	dlReq.Header.Set("Authorization", "Bearer "+token)
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil || dlResp == nil || dlResp.StatusCode != http.StatusOK {
		t.Fatalf("media download failed: %v", err)
	}
	downloadedData, _ := io.ReadAll(dlResp.Body)
	_ = dlResp.Body.Close()
	if string(downloadedData) != "OGG_OPUS_SAMPLE_AUDIO_DATA" {
		t.Errorf("downloaded data mismatch: got %q", string(downloadedData))
	}
}

func TestE2EWhatsAppVoiceNotes(t *testing.T) {
	secret := "test_wa_secret"
	token := "wa_token_dev" //nolint:gosec // test fixture token
	agentState := &mockWhatsAppAgentState{}
	agentSrv := setupMockWhatsAppAgent(agentState, secret)
	defer agentSrv.Close()

	emuSrv, _ := setupWhatsAppEmuServer(secret, token, agentSrv.URL+"/webhook")
	defer emuSrv.Close()

	mediaID := uploadTestMedia(t, emuSrv.URL, token)
	if mediaID == "" {
		t.Fatalf("expected non-empty media ID")
	}

	verifyDownloadedMedia(t, emuSrv.URL, token, mediaID)

	inboundAudio := map[string]any{
		"from":             "+14155551111",
		"to":               "+14155550199",
		"type":             "audio",
		"voice":            true,
		"media_id":         mediaID,
		"duration_seconds": 3.5,
	}
	inBytes, _ := json.Marshal(inboundAudio)
	inResp, err := http.Post(emuSrv.URL+"/emu/inbound/whatsapp", "application/json", bytes.NewBuffer(inBytes))
	if err != nil || inResp == nil || inResp.StatusCode != http.StatusOK {
		t.Fatalf("inbound audio dispatch failed: %v", err)
	}
	_ = inResp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	agentState.mu.Lock()
	defer agentState.mu.Unlock()
	if agentState.inboundAudioID != mediaID {
		t.Errorf("expected agent inbound audio ID %q, got %q", mediaID, agentState.inboundAudioID)
	}
}

func TestE2EWhatsAppListMenu(t *testing.T) {
	secret := "test_wa_secret"
	token := "wa_token_dev" //nolint:gosec // test fixture token
	agentState := &mockWhatsAppAgentState{}
	agentSrv := setupMockWhatsAppAgent(agentState, secret)
	defer agentSrv.Close()

	emuSrv, _ := setupWhatsAppEmuServer(secret, token, agentSrv.URL+"/webhook")
	defer emuSrv.Close()

	inboundPayload := map[string]any{
		"from":                 "+14155551111",
		"to":                   "+14155550199",
		"type":                 "interactive",
		"interactive_type":     "list_reply",
		"selected_id":          "row_tech_support",
		"selected_title":       "Technical Support",
		"selected_description": "API issues",
	}
	inBytes, _ := json.Marshal(inboundPayload)
	inResp, err := http.Post(emuSrv.URL+"/emu/inbound/whatsapp", "application/json", bytes.NewBuffer(inBytes))
	if err != nil || inResp == nil || inResp.StatusCode != http.StatusOK {
		t.Fatalf("inbound list reply failed: %v", err)
	}
	_ = inResp.Body.Close()

	time.Sleep(50 * time.Millisecond)

	agentState.mu.Lock()
	defer agentState.mu.Unlock()
	if agentState.inboundList != "row_tech_support" {
		t.Errorf("expected inbound list row 'row_tech_support', got %q", agentState.inboundList)
	}
}

func TestE2EDualEngineBroadcast(t *testing.T) {
	waSecret := "wa_secret_123"
	waToken := "wa_token_123" //nolint:gosec // test fixture token
	agentState := &mockWhatsAppAgentState{}
	waAgentSrv := setupMockWhatsAppAgent(agentState, waSecret)
	defer waAgentSrv.Close()

	emuSrv, _ := setupWhatsAppEmuServer(waSecret, waToken, waAgentSrv.URL+"/webhook")
	defer emuSrv.Close()

	smsPayload := map[string]any{
		"from": "+14155551111",
		"to":   "+14155550199",
		"body": "Broadcast to Twilio",
	}
	smsBytes, _ := json.Marshal(smsPayload)
	smsResp, err := http.Post(emuSrv.URL+"/emu/inbound/sms", "application/json", bytes.NewBuffer(smsBytes))
	if err != nil || smsResp == nil || smsResp.StatusCode != http.StatusOK {
		t.Fatalf("inbound SMS failed: %v", err)
	}
	_ = smsResp.Body.Close()

	waPayload := map[string]any{
		"from": "+14155551111",
		"to":   "+14155550199",
		"type": "text",
		"body": "Broadcast to WhatsApp",
	}
	waBytes, _ := json.Marshal(waPayload)
	waResp, err := http.Post(emuSrv.URL+"/emu/inbound/whatsapp", "application/json", bytes.NewBuffer(waBytes))
	if err != nil || waResp == nil || waResp.StatusCode != http.StatusOK {
		t.Fatalf("inbound WhatsApp failed: %v", err)
	}
	_ = waResp.Body.Close()

	msgReq, err := http.Get(emuSrv.URL + "/emu/messages")
	if err != nil || msgReq == nil {
		t.Fatalf("get messages failed: %v", err)
	}
	var data struct {
		Messages []domain.Message `json:"messages"`
	}
	_ = json.NewDecoder(msgReq.Body).Decode(&data)
	_ = msgReq.Body.Close()

	foundTwilio := false
	foundWA := false
	for _, m := range data.Messages {
		if m.Engine == domain.EngineTwilio && m.Body == "Broadcast to Twilio" {
			foundTwilio = true
		}
		if m.Engine == domain.EngineWhatsApp && m.Body == "Broadcast to WhatsApp" {
			foundWA = true
		}
	}
	if !foundTwilio || !foundWA {
		t.Errorf("expected both engine messages saved, foundTwilio=%v, foundWA=%v", foundTwilio, foundWA)
	}
}
