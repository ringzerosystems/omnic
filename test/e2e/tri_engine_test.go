package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

type triEngineCluster struct {
	server  *httptest.Server
	broker  *service.EventBroker
	store   *memory.Store
	tgToken string
}

func setupTriEngineCluster(twilioURL, waURL, tgURL string) *triEngineCluster {
	store := memory.NewStore()
	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	twilioDispatcher := service.NewWebhookDispatcher(store, "twilio_auth_token")

	ctx := context.Background()
	_, _ = phoneSvc.Provision(ctx, "AC001", service.ProvisionParams{
		PhoneNumber:  "+14155550199",
		FriendlyName: "Demo Twilio",
		SmsURL:       twilioURL,
		SmsMethod:    "POST",
	})

	broker := service.NewMemoryEventBroker()
	msgSvc.SetBroker(broker)

	//nolint:gosec // mock test credentials
	waConfig := service.WhatsAppConfig{
		PhoneNumberID: "100000000000001",
		WABAID:        "200000000000001",
		DisplayNumber: "+14155550199",
		BearerToken:   "wa_token_dev",
		AppSecret:     "wa_secret_dev",
		UpstreamURL:   waURL,
		DeliveryDelay: 10 * time.Millisecond,
	}
	waSvc := service.NewWhatsAppService(store, broker, waConfig)

	//nolint:gosec // mock test credentials
	tgToken := "tri_bot_token_777"
	tgConfig := service.TelegramConfig{
		BotToken:      tgToken,
		BotUsername:   "tri_bot",
		BotName:       "TriBot",
		UpstreamURL:   tgURL,
		SecretToken:   "tri_secret_token",
		DeliveryDelay: 10 * time.Millisecond,
	}
	tgSvc := service.NewTelegramService(store, broker, tgConfig)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, twilioDispatcher)
	h.SetBroker(broker)
	h.SetWhatsApp(waSvc)
	h.SetTelegram(tgSvc)

	e := echo.New()
	h.RegisterRoutes(e)

	return &triEngineCluster{
		server:  httptest.NewServer(e),
		broker:  broker,
		store:   store,
		tgToken: tgToken,
	}
}

func TestTriEngine_SimultaneousExecution(t *testing.T) {
	cluster := setupTriEngineCluster("", "", "")
	defer cluster.server.Close()

	// 1. Send Twilio outbound SMS
	twilioForm := url.Values{
		"From": {"+14155550199"},
		"To":   {"+14155551111"},
		"Body": {"Twilio outbound test"},
	}
	reqTwilio, err := http.NewRequest(http.MethodPost, cluster.server.URL+"/2010-04-01/Accounts/AC001/Messages.json", strings.NewReader(twilioForm.Encode()))
	if err != nil || reqTwilio == nil {
		t.Fatalf("failed creating Twilio request: %v", err)
	}
	reqTwilio.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqTwilio.SetBasicAuth("AC001", "twilio_auth_token")
	respTwilio, err := http.DefaultClient.Do(reqTwilio)
	if err != nil || respTwilio == nil || respTwilio.StatusCode != http.StatusCreated {
		t.Fatalf("Twilio outbound failed: %v", err)
	}
	_ = respTwilio.Body.Close()

	// 2. Send WhatsApp outbound message
	waPayload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                "+14155551111",
		"type":              "text",
		"text":              map[string]string{"body": "WhatsApp outbound test"},
	}
	waBytes, _ := json.Marshal(waPayload)
	reqWA, err := http.NewRequest(http.MethodPost, cluster.server.URL+"/v21.0/100000000000001/messages", bytes.NewBuffer(waBytes))
	if err != nil || reqWA == nil {
		t.Fatalf("failed creating WhatsApp request: %v", err)
	}
	reqWA.Header.Set("Content-Type", "application/json")
	reqWA.Header.Set("Authorization", "Bearer wa_token_dev")
	respWA, err := http.DefaultClient.Do(reqWA)
	if err != nil || respWA == nil || respWA.StatusCode != http.StatusOK {
		t.Fatalf("WhatsApp outbound failed: %v", err)
	}
	_ = respWA.Body.Close()

	// 3. Send Telegram outbound message
	tgPayload := map[string]any{
		"chat_id": 14155551111,
		"text":    "Telegram outbound test",
	}
	tgBytes, _ := json.Marshal(tgPayload)
	respTG, err := http.Post(fmt.Sprintf("%s/bot%s/sendMessage", cluster.server.URL, cluster.tgToken), "application/json", bytes.NewBuffer(tgBytes))
	if err != nil || respTG == nil || respTG.StatusCode != http.StatusOK {
		t.Fatalf("Telegram outbound failed: %v", err)
	}
	_ = respTG.Body.Close()

	// 4. Verify all 3 appear in unified message store with correct Engine tags
	respList, err := http.Get(cluster.server.URL + "/emu/messages")
	if err != nil || respList == nil || respList.StatusCode != http.StatusOK {
		t.Fatalf("Listing emu messages failed: %v", err)
	}
	defer func() { _ = respList.Body.Close() }()

	var listData struct {
		Messages []struct {
			Engine string `json:"engine"`
			Body   string `json:"body"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(respList.Body).Decode(&listData)

	engineCount := map[string]int{}
	for _, m := range listData.Messages {
		engineCount[m.Engine]++
	}

	if engineCount[string(domain.EngineTwilio)] != 1 {
		t.Errorf("expected 1 Twilio message, got %d", engineCount[string(domain.EngineTwilio)])
	}
	if engineCount[string(domain.EngineWhatsApp)] != 1 {
		t.Errorf("expected 1 WhatsApp message, got %d", engineCount[string(domain.EngineWhatsApp)])
	}
	if engineCount[string(domain.EngineTelegram)] != 1 {
		t.Errorf("expected 1 Telegram message, got %d", engineCount[string(domain.EngineTelegram)])
	}
}

type fanOutRecorder struct {
	twilioBody   string
	waBody       string
	tgBody       string
	tgSecretAuth string
	mu           sync.Mutex
}

func TestTriEngine_BroadcastFanOut(t *testing.T) {
	rec := &fanOutRecorder{}

	mockTwilio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		rec.mu.Lock()
		rec.twilioBody = r.FormValue("Body")
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("<Response/>"))
	}))
	defer mockTwilio.Close()

	mockWA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p struct {
			Entry []struct {
				Changes []struct {
					Value struct {
						Messages []struct {
							Text struct {
								Body string `json:"body"`
							} `json:"text"`
						} `json:"messages"`
					} `json:"value"`
				} `json:"changes"`
			} `json:"entry"`
		}
		_ = json.Unmarshal(body, &p)
		rec.mu.Lock()
		if len(p.Entry) > 0 && len(p.Entry[0].Changes) > 0 && len(p.Entry[0].Changes[0].Value.Messages) > 0 {
			rec.waBody = p.Entry[0].Changes[0].Value.Messages[0].Text.Body
		}
		rec.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockWA.Close()

	mockTG := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p struct {
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
		}
		_ = json.Unmarshal(body, &p)
		rec.mu.Lock()
		rec.tgSecretAuth = r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		rec.tgBody = p.Message.Text
		rec.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockTG.Close()

	cluster := setupTriEngineCluster(mockTwilio.URL, mockWA.URL, mockTG.URL)
	defer cluster.server.Close()

	broadcastMsg := "Urgent System Maintenance at 15:00 UTC"

	// Dispatch fan-out: Twilio, WhatsApp, Telegram
	tPayload := map[string]any{"from": "+14155551111", "to": "+14155550199", "body": broadcastMsg}
	tB, _ := json.Marshal(tPayload)
	_, _ = http.Post(cluster.server.URL+"/emu/inbound/sms", "application/json", bytes.NewBuffer(tB))

	waInPayload := map[string]any{"from": "+14155551111", "to": "+14155550199", "type": "text", "body": broadcastMsg}
	waB, _ := json.Marshal(waInPayload)
	_, _ = http.Post(cluster.server.URL+"/emu/inbound/whatsapp", "application/json", bytes.NewBuffer(waB))

	tgInPayload := map[string]any{"from_id": 14155551111, "chat_id": 14155551111, "username": "handset_user", "text": broadcastMsg, "type": "text"}
	tgB, _ := json.Marshal(tgInPayload)
	_, _ = http.Post(cluster.server.URL+"/emu/inbound/telegram", "application/json", bytes.NewBuffer(tgB))

	time.Sleep(100 * time.Millisecond)

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if rec.twilioBody != broadcastMsg {
		t.Errorf("Twilio upstream received %q, want %q", rec.twilioBody, broadcastMsg)
	}
	if rec.waBody != broadcastMsg {
		t.Errorf("WhatsApp upstream received %q, want %q", rec.waBody, broadcastMsg)
	}
	if rec.tgBody != broadcastMsg {
		t.Errorf("Telegram upstream received %q, want %q", rec.tgBody, broadcastMsg)
	}
	if rec.tgSecretAuth != "tri_secret_token" {
		t.Errorf("Telegram upstream received secret %q, want 'tri_secret_token'", rec.tgSecretAuth)
	}
}
