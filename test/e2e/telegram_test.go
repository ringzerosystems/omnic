package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

type mockTelegramAgentState struct {
	lastHeaderSecret string
	lastUpdate       map[string]any
	updates          []map[string]any
	mu               sync.Mutex
}

func setupTelegramEmuServer(botToken, secretToken, upstreamURL string) (*httptest.Server, *service.TelegramService) {
	store := memory.NewStore()
	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	twilioDispatcher := service.NewWebhookDispatcher(store, "twilio_auth_token")

	broker := service.NewMemoryEventBroker()
	msgSvc.SetBroker(broker)

	tgConfig := service.TelegramConfig{
		BotToken:      botToken,
		BotUsername:   "omnic_bot",
		BotName:       "OmnicBot",
		UpstreamURL:   upstreamURL,
		SecretToken:   secretToken,
		DeliveryDelay: 10 * time.Millisecond,
	}
	tgSvc := service.NewTelegramService(store, broker, tgConfig)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, twilioDispatcher)
	h.SetBroker(broker)
	h.SetTelegram(tgSvc)

	e := echo.New()
	h.RegisterRoutes(e)
	return httptest.NewServer(e), tgSvc
}

func setupMockTelegramAgent(state *mockTelegramAgentState) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/tg_webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)

		state.mu.Lock()
		state.lastHeaderSecret = r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		state.lastUpdate = payload
		state.updates = append(state.updates, payload)
		state.mu.Unlock()

		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(mux)
}

func TestTelegram_GetMe(t *testing.T) {
	botToken := "test_token_123"
	emu, _ := setupTelegramEmuServer(botToken, "secret_xyz", "")
	defer emu.Close()

	resp, err := http.Get(fmt.Sprintf("%s/bot%s/getMe", emu.URL, botToken))
	if err != nil || resp == nil {
		t.Fatalf("getMe failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res struct {
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
		Ok bool `json:"ok"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if !res.Ok || res.Result.Username != "omnic_bot" {
		t.Errorf("expected username omnic_bot, got %+v", res)
	}
}

func TestTelegram_SendMessage_Outbound(t *testing.T) {
	botToken := "test_token_123"
	emu, _ := setupTelegramEmuServer(botToken, "secret_xyz", "")
	defer emu.Close()

	payload := map[string]any{
		"chat_id": 14155551111,
		"text":    "Choose option:",
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]any{
				{
					{"text": "Option 1", "callback_data": "opt_1"},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	resp, err := http.Post(fmt.Sprintf("%s/bot%s/sendMessage", emu.URL, botToken), "application/json", bytes.NewBuffer(b))
	if err != nil || resp == nil {
		t.Fatalf("sendMessage failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res struct {
		Result struct {
			Text string `json:"text"`
		} `json:"result"`
		Ok bool `json:"ok"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if !res.Ok || res.Result.Text != "Choose option:" {
		t.Errorf("unexpected sendMessage response: %+v", res)
	}
}

func TestTelegram_InboundText_WebhookWithSecretToken(t *testing.T) {
	botToken := "test_token_123"
	secret := "secret_tg_token_999"
	state := &mockTelegramAgentState{}
	agent := setupMockTelegramAgent(state)
	defer agent.Close()

	emu, _ := setupTelegramEmuServer(botToken, secret, agent.URL+"/tg_webhook")
	defer emu.Close()

	inboundPayload := map[string]any{
		"from_id":    14155551111,
		"chat_id":    14155551111,
		"username":   "handset_user",
		"first_name": "Handset User",
		"text":       "Hello AI Bot",
		"type":       "text",
	}
	b, _ := json.Marshal(inboundPayload)
	resp, err := http.Post(emu.URL+"/emu/inbound/telegram", "application/json", bytes.NewBuffer(b))
	if err != nil || resp == nil {
		t.Fatalf("inbound telegram post failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	time.Sleep(50 * time.Millisecond)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.lastHeaderSecret != secret {
		t.Errorf("expected secret header %q, got %q", secret, state.lastHeaderSecret)
	}
	if len(state.updates) == 0 {
		t.Fatalf("expected webhook update received by agent")
	}
	msg, ok := state.lastUpdate["message"].(map[string]any)
	if !ok || msg["text"] != "Hello AI Bot" {
		t.Errorf("unexpected webhook message: %+v", state.lastUpdate)
	}
}

func TestTelegram_InlineButtonCallback(t *testing.T) {
	botToken := "test_token_123"
	secret := "secret_button"
	state := &mockTelegramAgentState{}
	agent := setupMockTelegramAgent(state)
	defer agent.Close()

	emu, _ := setupTelegramEmuServer(botToken, secret, agent.URL+"/tg_webhook")
	defer emu.Close()

	cbPayload := map[string]any{
		"from_id":           14155551111,
		"chat_id":           14155551111,
		"username":          "handset_user",
		"callback_data":     "billing_info",
		"callback_query_id": "cbq_001",
	}
	b, _ := json.Marshal(cbPayload)
	resp, err := http.Post(emu.URL+"/emu/inbound/telegram", "application/json", bytes.NewBuffer(b))
	if err != nil || resp == nil {
		t.Fatalf("callback query inbound failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	time.Sleep(50 * time.Millisecond)

	state.mu.Lock()
	if len(state.updates) == 0 {
		state.mu.Unlock()
		t.Fatalf("expected callback update received by mock agent")
	}
	cb, ok := state.lastUpdate["callback_query"].(map[string]any)
	state.mu.Unlock()

	if !ok || cb["data"] != "billing_info" {
		t.Errorf("unexpected callback query payload: %+v", cb)
	}

	ansPayload := map[string]any{
		"callback_query_id": "cbq_001",
		"text":              "Billing acknowledged",
	}
	ansB, _ := json.Marshal(ansPayload)
	ansResp, err := http.Post(fmt.Sprintf("%s/bot%s/answerCallbackQuery", emu.URL, botToken), "application/json", bytes.NewBuffer(ansB))
	if err != nil || ansResp == nil {
		t.Fatalf("answerCallbackQuery failed: %v", err)
	}
	defer func() { _ = ansResp.Body.Close() }()

	if ansResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for answerCallbackQuery, got %d", ansResp.StatusCode)
	}
}

func TestTelegram_VoiceNote_Roundtrip(t *testing.T) {
	botToken := "test_token_123"
	emu, tgSvc := setupTelegramEmuServer(botToken, "secret_voice", "")
	defer emu.Close()

	ctx := context.Background()
	sampleAudio := []byte("OggS mock opus audio stream data")
	mediaID, err := tgSvc.UploadMedia(ctx, "audio/ogg", "sample.oga", sampleAudio)
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	sendVoicePayload := map[string]any{
		"chat_id":  14155551111,
		"voice":    mediaID,
		"caption":  "Test voice note",
		"duration": 4,
	}
	b, _ := json.Marshal(sendVoicePayload)
	resp, err := http.Post(fmt.Sprintf("%s/bot%s/sendVoice", emu.URL, botToken), "application/json", bytes.NewBuffer(b))
	if err != nil || resp == nil {
		t.Fatalf("sendVoice failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var sendRes struct {
		Result struct {
			Voice struct {
				FileID string `json:"file_id"`
			} `json:"voice"`
		} `json:"result"`
		Ok bool `json:"ok"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sendRes)
	if !sendRes.Ok || sendRes.Result.Voice.FileID == "" {
		t.Fatalf("expected valid file_id, got %+v", sendRes)
	}

	fileID := sendRes.Result.Voice.FileID
	fileResp, err := http.Get(fmt.Sprintf("%s/bot%s/getFile?file_id=%s", emu.URL, botToken, fileID))
	if err != nil || fileResp == nil {
		t.Fatalf("getFile failed: %v", err)
	}
	defer func() { _ = fileResp.Body.Close() }()

	var fileRes struct {
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
		Ok bool `json:"ok"`
	}
	_ = json.NewDecoder(fileResp.Body).Decode(&fileRes)
	if !fileRes.Ok || fileRes.Result.FilePath == "" {
		t.Fatalf("expected valid file_path, got %+v", fileRes)
	}

	dlResp, err := http.Get(fmt.Sprintf("%s/file/bot%s/%s", emu.URL, botToken, fileRes.Result.FilePath))
	if err != nil || dlResp == nil {
		t.Fatalf("download failed: %v", err)
	}
	defer func() { _ = dlResp.Body.Close() }()

	downloaded, _ := io.ReadAll(dlResp.Body)
	if !bytes.Equal(downloaded, sampleAudio) {
		t.Errorf("downloaded audio mismatch: got %q, want %q", downloaded, sampleAudio)
	}
}

func TestTelegram_SetWebhook_Lifecycle(t *testing.T) {
	botToken := "test_token_123"
	emu, _ := setupTelegramEmuServer(botToken, "", "")
	defer emu.Close()

	setPayload := map[string]any{
		"url":          "http://example.com/webhook",
		"secret_token": "token_abc",
	}
	b, _ := json.Marshal(setPayload)
	resp, err := http.Post(fmt.Sprintf("%s/bot%s/setWebhook", emu.URL, botToken), "application/json", bytes.NewBuffer(b))
	if err != nil || resp == nil {
		t.Fatalf("setWebhook failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	infoResp, err := http.Get(fmt.Sprintf("%s/bot%s/getWebhookInfo", emu.URL, botToken))
	if err != nil || infoResp == nil {
		t.Fatalf("getWebhookInfo failed: %v", err)
	}
	defer func() { _ = infoResp.Body.Close() }()

	var infoRes struct {
		Result struct {
			URL string `json:"url"`
		} `json:"result"`
		Ok bool `json:"ok"`
	}
	_ = json.NewDecoder(infoResp.Body).Decode(&infoRes)
	if !infoRes.Ok || infoRes.Result.URL != "http://example.com/webhook" {
		t.Errorf("unexpected webhook info: %+v", infoRes)
	}

	delResp, err := http.Post(fmt.Sprintf("%s/bot%s/deleteWebhook", emu.URL, botToken), "application/json", nil)
	if err != nil || delResp == nil {
		t.Fatalf("deleteWebhook failed: %v", err)
	}
	defer func() { _ = delResp.Body.Close() }()

	if delResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for deleteWebhook, got %d", delResp.StatusCode)
	}
}
