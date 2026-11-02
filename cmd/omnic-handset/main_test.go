package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ringzerosystems/omnic/internal/domain"
)

func TestSendInboundSMS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emu/inbound/sms" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"status": "delivered_to_webhook",
			"reply": map[string]any{
				"body": "Echo: " + body["body"].(string),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	reply, err := sendInboundSMS(ts.URL, "+14155551111", "+14155550199", "Test Hello")
	if err != nil {
		t.Fatalf("sendInboundSMS failed: %v", err)
	}
	if reply != "Echo: Test Hello" {
		t.Errorf("got reply %q, want %q", reply, "Echo: Test Hello")
	}
}

func TestSendInboundSMSError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "dial tcp 127.0.0.1:8000: connect: connection refused",
			"code":    11200,
			"status":  502,
		})
	}))
	defer ts.Close()

	_, err := sendInboundSMS(ts.URL, "+14155551111", "+14155550199", "Test Hello")
	if err == nil {
		t.Fatalf("expected error from 502 response, got nil")
	}
	if !strings.Contains(err.Error(), "code 11200") || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("unexpected error format: %v", err)
	}

	// Verify handleSendResult updates in-flight message to 'failed'
	m := newModel(ts.URL, "+14155551111", "+14155550199")
	m.messages = append(m.messages, chatMessage{
		From:      "+14155551111",
		To:        "+14155550199",
		Body:      "Test Hello",
		Status:    "sending",
		Direction: "inbound",
	})
	m2, _ := m.handleSendResult(sendResultMsg{err: err})
	mod := m2.(model)
	if !strings.Contains(mod.status, "Send error:") {
		t.Errorf("expected status to have Send error, got %q", mod.status)
	}
	if len(mod.messages) != 1 || mod.messages[0].Status != "failed" {
		t.Errorf("expected in-flight message to become failed, got %+v", mod.messages)
	}
}

func TestPhoneModelTextInputAndClear(t *testing.T) {
	m := newModel("http://localhost:8080", "+14155551111", "+14155550199")

	// Type 'H', 'i'
	m2, _ := m.Update(tea.KeyPressMsg{Text: "H"})
	m3, _ := m2.(model).Update(tea.KeyPressMsg{Text: "i"})
	if m3.(model).input != "Hi" {
		t.Errorf("expected input 'Hi', got %q", m3.(model).input)
	}

	// Backspace
	m4, _ := m3.(model).Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m4.(model).input != "H" {
		t.Errorf("expected input 'H' after backspace, got %q", m4.(model).input)
	}

	// Enter sends and sets thinking status
	m5, cmd := m4.(model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m5.(model).input != "" {
		t.Errorf("expected input to be cleared on Enter, got %q", m5.(model).input)
	}
	if !strings.Contains(m5.(model).status, "thinking") {
		t.Errorf("expected thinking status, got %q", m5.(model).status)
	}
	if cmd == nil {
		t.Errorf("expected tea.Cmd to send SMS on Enter")
	}

	// Ctrl+K clears messages
	m6, _ := m5.(model).Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if len(m6.(model).messages) != 0 {
		t.Errorf("expected messages to be cleared on Ctrl+K, got %d", len(m6.(model).messages))
	}
}

func TestPhoneModelSSEUpdateAndBubbleView(t *testing.T) {
	m := newModel("http://localhost:8080", "+14155551111", "+14155550199")

	// Simulate receiving an outbound agent message via SSE
	evMsg := sseEventMsg{
		EventType: "message.outbound",
		Message: chatMessage{
			Timestamp: time.Now(),
			SID:       "SM_REPLY_1",
			From:      "+14155550199",
			To:        "+14155551111",
			Body:      "Your package has shipped!",
			Status:    "delivered",
			Direction: "outbound-api",
		},
	}

	newM, _ := m.Update(evMsg)
	curM := newM.(model)
	if len(curM.messages) != 1 {
		t.Fatalf("expected 1 message in thread, got %d", len(curM.messages))
	}

	view := curM.View()
	content := view.Content
	if !strings.Contains(content, "Your package has shipped!") {
		t.Errorf("expected message body in view, got:\n%s", content)
	}
	if !strings.Contains(content, "AI Support Agent") {
		t.Errorf("expected AI Support Agent label in view, got:\n%s", content)
	}
	if !strings.Contains(content, "+14155551111") {
		t.Errorf("expected handset number in view header, got:\n%s", content)
	}
}

func TestFetchHistory(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"messages": []map[string]any{
				{
					"sid":          "SM001",
					"from":         "+14155550199",
					"to":           "+14155551111",
					"body":         "Previous conversation item",
					"status":       "delivered",
					"direction":    "outbound-api",
					"date_created": "Wed, 16 Sep 2026 12:00:00 +0000",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	msgs, err := fetchHistory(context.Background(), ts.URL, "+14155551111", "+14155550199")
	if err != nil {
		t.Fatalf("fetchHistory failed: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Body != "Previous conversation item" {
		t.Fatalf("unexpected fetched messages: %+v", msgs)
	}
}

func TestPhoneModelWindowResize(t *testing.T) {
	m := newModel("http://localhost:8080", "+14155551111", "+14155550199")
	if m.width != 80 || m.height != 24 {
		t.Errorf("expected default size 80x24, got %dx%d", m.width, m.height)
	}

	newM, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Errorf("expected nil cmd on WindowSizeMsg, got %v", cmd)
	}
	curM, ok := newM.(model)
	if !ok {
		t.Fatalf("expected model type, got %T", newM)
	}
	if curM.width != 100 || curM.height != 40 {
		t.Errorf("expected updated size 100x40, got %dx%d", curM.width, curM.height)
	}

	view := curM.View()
	if view.Content == "" {
		t.Errorf("expected non-empty view content after resize")
	}
}

func TestPhoneModelMultilineRendering(t *testing.T) {
	m := newModel("http://localhost:8080", "+14155551111", "+14155550199")

	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		From:      "+14155551111",
		To:        "+14155550199",
		Body:      "Line 1: Question about order status\nLine 2: https://example.com/very/long/url/that/would/normally/overflow\nLine 3: Thank you!",
		Status:    "delivered",
		Direction: "inbound",
	})
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		SID:       "SM_MULTI_1",
		From:      "+14155550199",
		To:        "+14155551111",
		Body:      "Hello! Here is your detailed response:\n1. In transit.\n2. Arriving tomorrow.\nHave a great day!",
		Status:    "delivered",
		Direction: "outbound-api",
	})

	widths := []int{0, 48, 80, 140}
	for _, w := range widths {
		m2, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		content := m2.(model).View().Content

		if !strings.Contains(content, "Question about order") || !strings.Contains(content, "Thank you!") {
			t.Errorf("width %d: missing user message text; content:\n%s", w, content)
		}
		if !strings.Contains(content, "Arriving tomorrow") {
			t.Errorf("width %d: missing agent message text", w)
		}
		if !strings.Contains(content, "AI Support Agent") {
			t.Errorf("width %d: missing AI Support Agent header", w)
		}
	}
}

func TestPhoneModelScrolling(t *testing.T) {
	m := newModel("http://localhost:8080", "+14155551111", "+14155550199")
	m.width = 80
	m.height = 24

	for i := 1; i <= 15; i++ {
		m.addOrUpdateMessage(chatMessage{
			Timestamp: time.Now(),
			From:      "+14155551111",
			To:        "+14155550199",
			Body:      "Message item number",
			Status:    "delivered",
			Direction: "inbound",
		})
	}

	content := m.View().Content
	if !strings.Contains(content, "+14155551111") {
		t.Errorf("expected header with handset number in view")
	}

	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	curM := m2.(model)
	if curM.scrollOffset != 1 {
		t.Errorf("expected scrollOffset 1, got %d", curM.scrollOffset)
	}

	m3, _ := curM.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	curM2 := m3.(model)
	if curM2.scrollOffset != 6 {
		t.Errorf("expected scrollOffset 6, got %d", curM2.scrollOffset)
	}

	contentScrolled := curM2.View().Content
	if !strings.Contains(contentScrolled, "+14155551111") {
		t.Errorf("expected header to remain pinned when scrolled")
	}

	m4, _ := curM2.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	curM3 := m4.(model)
	if curM3.scrollOffset != 5 {
		t.Errorf("expected scrollOffset 5, got %d", curM3.scrollOffset)
	}
}

func TestTargetModeSingleEngine(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "twilio",
	})
	if m.target != targetTwilio {
		t.Fatalf("expected initial target %s, got %s", targetTwilio, m.target)
	}

	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m2.(model).target != targetTwilio {
		t.Errorf("expected target %s after Tab in single engine mode, got %s", targetTwilio, m2.(model).target)
	}
}

func TestWhatsAppButtonReplyInteraction(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "whatsapp",
	})
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		SID:       "wamid.123",
		From:      "+14155550199",
		To:        "+14155551111",
		Body:      "Please confirm appointment:",
		Status:    "delivered",
		Direction: "outbound-api",
		Engine:    domain.EngineWhatsApp,
		InteractiveContent: &domain.InteractiveContent{
			Type: domain.InteractiveTypeButton,
			Body: "Please confirm appointment:",
			Buttons: []domain.ButtonItem{
				{ID: "btn_yes", Title: "Yes"},
				{ID: "btn_no", Title: "No"},
			},
		},
	})

	view := m.View().Content
	if !strings.Contains(view, "1: Yes") || !strings.Contains(view, "2: No") {
		t.Errorf("expected button pills in view, got:\n%s", view)
	}

	// Press '1' to select first button
	m2, cmd := m.Update(tea.KeyPressMsg{Code: '1'})
	if cmd == nil {
		t.Errorf("expected command dispatched on button '1'")
	}
	curM := m2.(model)
	if len(curM.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(curM.messages))
	}
	last := curM.messages[1]
	if !strings.Contains(last.Body, "You selected: Yes (btn_yes)") {
		t.Errorf("unexpected body: %s", last.Body)
	}
}

func TestWhatsAppListMenuInteraction(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "whatsapp",
	})
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		SID:       "wamid.list",
		From:      "+14155550199",
		To:        "+14155551111",
		Body:      "Choose an option",
		Status:    "delivered",
		Direction: "outbound-api",
		Engine:    domain.EngineWhatsApp,
		InteractiveContent: &domain.InteractiveContent{
			Type: domain.InteractiveTypeList,
			Body: "Choose an option",
			Sections: []domain.MenuSection{
				{
					Title: "Support",
					Rows: []domain.MenuRow{
						{ID: "row_billing", Title: "Billing", Description: "Invoices"},
						{ID: "row_tech", Title: "Tech Support", Description: "API issues"},
					},
				},
			},
		},
	})

	// Press 'M' to open menu
	m2, _ := m.Update(tea.KeyPressMsg{Code: 'm'})
	curM := m2.(model)
	if !curM.menuOpen || len(curM.menuItems) != 2 {
		t.Fatalf("expected menu to open with 2 items, got open=%v count=%d", curM.menuOpen, len(curM.menuItems))
	}
	if !strings.Contains(curM.View().Content, "Technical Menu") {
		t.Errorf("expected modal menu rendered in view")
	}

	// Move cursor down
	m3, _ := curM.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m3.(model).menuCursor != 1 {
		t.Errorf("expected menuCursor 1, got %d", m3.(model).menuCursor)
	}

	// Press Enter to select
	m4, cmd := m3.(model).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Errorf("expected select command on Enter")
	}
	curM4 := m4.(model)
	if curM4.menuOpen {
		t.Errorf("expected menu to close on select")
	}
	if len(curM4.messages) != 2 || !strings.Contains(curM4.messages[1].Body, "Tech Support") {
		t.Errorf("unexpected selected message: %+v", curM4.messages)
	}
}

func TestVoiceNoteWaveformAndPlaybackToggle(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "whatsapp",
	})
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		SID:       "wamid.audio",
		From:      "+14155550199",
		To:        "+14155551111",
		Body:      "[Voice Note]",
		Status:    "delivered",
		Direction: "outbound-api",
		Engine:    domain.EngineWhatsApp,
		MediaURLs: []string{"http://localhost:8080/audio.ogg"},
		VoiceMetadata: &domain.VoiceMetadata{
			DurationSeconds: 14,
			IsVoiceNote:     true,
		},
	})

	view := m.View().Content
	if !strings.Contains(view, "Voice Note") || !strings.Contains(view, "Space: Play") {
		t.Errorf("expected voice note card in view, got:\n%s", view)
	}

	// Space toggles playback
	m2, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Errorf("expected playAudioCmd on Space")
	}
	if !m2.(model).isPlaying {
		t.Errorf("expected isPlaying to be true")
	}

	// Space again stops playback
	m3, _ := m2.(model).Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if m3.(model).isPlaying {
		t.Errorf("expected isPlaying to be false after second Space")
	}
}

func TestVoiceRecordingHUDInteraction(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "whatsapp",
	})

	// Press 'V' to start recording
	m2, cmd := m.Update(tea.KeyPressMsg{Code: 'v'})
	if cmd == nil {
		t.Errorf("expected tick command on record start")
	}
	curM := m2.(model)
	if !curM.isRecording {
		t.Fatalf("expected isRecording to be true")
	}

	// View shows recording HUD
	view := curM.View().Content
	if !strings.Contains(view, "RECORDING VOICE NOTE") {
		t.Errorf("expected recording HUD in view, got:\n%s", view)
	}

	// Esc cancels recording
	m3, _ := curM.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m3.(model).isRecording {
		t.Errorf("expected isRecording to be false on Esc")
	}
}

func TestSendInboundWhatsApp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emu/inbound/whatsapp" {
			http.NotFound(w, r)
			return
		}
		var body whatsAppInboundPayload
		_ = json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"status": "delivered_to_webhook",
			"reply": map[string]any{
				"body": "WA Echo: " + body.Body,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	reply, err := sendInboundWhatsApp(ts.URL, whatsAppInboundPayload{
		From: "+14155551111",
		To:   "+14155550199",
		Type: "text",
		Body: "Hello WhatsApp",
	})
	if err != nil {
		t.Fatalf("sendInboundWhatsApp failed: %v", err)
	}
	if reply != "WA Echo: Hello WhatsApp" {
		t.Errorf("got reply %q, want %q", reply, "WA Echo: Hello WhatsApp")
	}
}

func TestPrintAudioDevices(t *testing.T) {
	code := printAudioDevices()
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestSendInboundTelegram(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emu/inbound/telegram" {
			http.NotFound(w, r)
			return
		}
		var body telegramInboundPayload
		_ = json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"status": "delivered_to_webhook",
			"reply": map[string]any{
				"body": "TG Echo: " + body.Text,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	reply, err := sendInboundTelegram(ts.URL, telegramInboundPayload{
		FromID:    14155551111,
		ChatID:    14155551111,
		Username:  "test_user",
		FirstName: "Test",
		Text:      "Hello Telegram",
	})
	if err != nil {
		t.Fatalf("sendInboundTelegram failed: %v", err)
	}
	if reply != "TG Echo: Hello Telegram" {
		t.Errorf("got reply %q, want %q", reply, "TG Echo: Hello Telegram")
	}
}

func TestTargetModeCycleTriEngine(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "all",
	})
	if m.target != targetWhatsApp {
		t.Fatalf("expected initial target %s, got %s", targetWhatsApp, m.target)
	}

	m2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m2.(model).target != targetTwilio {
		t.Errorf("expected target %s after Tab, got %s", targetTwilio, m2.(model).target)
	}

	m3, _ := m2.(model).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m3.(model).target != targetTelegram {
		t.Errorf("expected target %s after Tab, got %s", targetTelegram, m3.(model).target)
	}

	m4, _ := m3.(model).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m4.(model).target != targetAll {
		t.Errorf("expected target %s after Tab, got %s", targetAll, m4.(model).target)
	}

	m5, _ := m4.(model).Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m5.(model).target != targetWhatsApp {
		t.Errorf("expected target %s after Tab, got %s", targetWhatsApp, m5.(model).target)
	}
}

func TestTelegramInlineButtonInteraction(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emu/inbound/telegram" {
			http.NotFound(w, r)
			return
		}
		var body telegramInboundPayload
		_ = json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"status": "delivered_to_webhook",
			"reply": map[string]any{
				"body": "Callback processed: " + body.CallbackData,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	m := newPhoneModel(phoneConfig{
		Server:           ts.URL,
		From:             "+14155551111",
		To:               "+14155550199",
		Engine:           "telegram",
		TelegramChatID:   14155551111,
		TelegramUsername: "test_user",
	})
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		From:      "+14155550199",
		To:        "+14155551111",
		Body:      "Please select category:",
		Status:    "delivered",
		Direction: "outbound-api",
		Engine:    domain.EngineTelegram,
		TelegramMetadata: &domain.TelegramMetadata{
			ChatID:    14155551111,
			MessageID: 101,
			InlineKeyboard: [][]domain.ButtonItem{
				{
					{ID: "cat_billing", Title: "Billing & Invoices"},
					{ID: "cat_support", Title: "Technical Support"},
				},
			},
		},
	})

	m2, cmd := m.Update(tea.KeyPressMsg{Code: '1'})
	if cmd == nil {
		t.Fatalf("expected command on inline button press '1', got nil")
	}

	msg := cmd()
	sRes, ok := msg.(sendResultMsg)
	if !ok {
		t.Fatalf("expected sendResultMsg, got %T", msg)
	}
	if sRes.err != nil {
		t.Fatalf("expected nil error, got %v", sRes.err)
	}
	if sRes.reply != "Callback processed: cat_billing" {
		t.Errorf("got reply %q, want %q", sRes.reply, "Callback processed: cat_billing")
	}

	mod := m2.(model)
	if !strings.Contains(mod.status, "Sent inline callback:") {
		t.Errorf("expected status to mention Sent inline callback, got %q", mod.status)
	}
}

func TestTelegramViewRendering(t *testing.T) {
	m := newPhoneModel(phoneConfig{
		Server: "http://localhost:8080",
		From:   "+14155551111",
		To:     "+14155550199",
		Engine: "telegram",
	})
	m.width = 100
	m.height = 30
	m.addOrUpdateMessage(chatMessage{
		Timestamp: time.Now(),
		From:      "+14155550199",
		To:        "+14155551111",
		Body:      "Hello from Telegram Bot",
		Status:    "delivered",
		Direction: "outbound-api",
		Engine:    domain.EngineTelegram,
		TelegramMetadata: &domain.TelegramMetadata{
			InlineKeyboard: [][]domain.ButtonItem{
				{{ID: "btn1", Title: "Option 1"}},
			},
		},
	})

	view := m.View()
	content := view.Content
	if !strings.Contains(content, "Telegram") {
		t.Errorf("expected view content to contain 'Telegram'")
	}
	if !strings.Contains(content, "Option 1") {
		t.Errorf("expected view content to render inline button 'Option 1'")
	}
}

func TestPhoneLoadEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := tmpDir + "/.env"
	content := "TWPHONE_ENGINE=telegram\nTWPHONE_TELEGRAM_USERNAME=custom_tester\nTWPHONE_TELEGRAM_CHAT_ID=987654321\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	loadEnvFile(envPath)

	if got := os.Getenv("TWPHONE_ENGINE"); got != "telegram" {
		t.Errorf("expected TWPHONE_ENGINE=telegram, got %q", got)
	}
	if got := os.Getenv("TWPHONE_TELEGRAM_USERNAME"); got != "custom_tester" {
		t.Errorf("expected TWPHONE_TELEGRAM_USERNAME=custom_tester, got %q", got)
	}
	if got := os.Getenv("TWPHONE_TELEGRAM_CHAT_ID"); got != "987654321" {
		t.Errorf("expected TWPHONE_TELEGRAM_CHAT_ID=987654321, got %q", got)
	}
}

func TestPhoneGetEnvDefaults(t *testing.T) {
	t.Setenv("TWPHONE_FROM", "+15550001111")
	t.Setenv("TWPHONE_TO", "+15550002222")

	if got := getEnvDefault("TWPHONE_FROM", "default"); got != "+15550001111" {
		t.Errorf("expected +15550001111, got %q", got)
	}
	if got := getEnvDefault("TWPHONE_TO", "default"); got != "+15550002222" {
		t.Errorf("expected +15550002222, got %q", got)
	}
}
