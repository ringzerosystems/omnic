package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func newTestTelegramService(upstreamURL, secretToken string) (*service.TelegramService, *service.EventBroker, *memory.Store) {
	store := memory.NewStore()
	broker := service.NewMemoryEventBroker()
	cfg := service.TelegramConfig{
		BotToken:      "123456789:test_token",
		BotUsername:   "test_bot",
		BotName:       "Test Bot",
		UpstreamURL:   upstreamURL,
		SecretToken:   secretToken,
		DeliveryDelay: 10 * time.Millisecond,
	}
	svc := service.NewTelegramService(store, broker, cfg)
	return svc, broker, store
}

func TestTelegramGetMe(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestTelegramService("", "")
	me, err := svc.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if me.ID != 123456789 || me.Username != "test_bot" || !me.IsBot {
		t.Errorf("unexpected user info: %+v", me)
	}
}

func TestTelegramSendMessage(t *testing.T) {
	t.Parallel()
	svc, broker, store := newTestTelegramService("", "")
	ctx := context.Background()

	// Empty text validation
	_, err := svc.SendMessage(ctx, service.SendMessageRequest{
		ChatID: int64(14155551111),
		Text:   "",
	})
	if err == nil {
		t.Errorf("expected error for empty text, got nil")
	}

	// Successful send
	msg, err := svc.SendMessage(ctx, service.SendMessageRequest{
		ChatID: int64(14155551111),
		Text:   "Hello from bot",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if msg.Text != "Hello from bot" || msg.MessageID == 0 {
		t.Errorf("unexpected message: %+v", msg)
	}

	// Check store
	stored, err := store.GetMessageByEngineID(ctx, domain.EngineTelegram, "tg_msg_401")
	if err != nil {
		t.Fatalf("failed to retrieve stored message: %v", err)
	}
	if stored.Body != "Hello from bot" || stored.Engine != domain.EngineTelegram {
		t.Errorf("unexpected stored message: %+v", stored)
	}
	_ = broker
}

func TestTelegramSendInlineKeyboard(t *testing.T) {
	t.Parallel()
	svc, _, store := newTestTelegramService("", "")
	ctx := context.Background()

	markupJSON := []byte(`{
		"inline_keyboard": [
			[{"text": "Order", "callback_data": "order_123"}, {"text": "Docs", "url": "https://example.com"}]
		]
	}`)

	msg, err := svc.SendMessage(ctx, service.SendMessageRequest{
		ChatID:      int64(14155551111),
		Text:        "Choose an option:",
		ReplyMarkup: markupJSON,
	})
	if err != nil {
		t.Fatalf("SendMessage with inline keyboard failed: %v", err)
	}
	if msg.ReplyMarkup == nil {
		t.Errorf("expected non-nil reply markup in response")
	}

	stored, err := store.GetMessageByEngineID(ctx, domain.EngineTelegram, "tg_msg_401")
	if err != nil {
		t.Fatalf("message not stored: %v", err)
	}
	if stored.InteractiveContent == nil || len(stored.InteractiveContent.Buttons) != 2 {
		t.Errorf("expected 2 interactive buttons: %+v", stored.InteractiveContent)
	}
	if stored.TelegramMetadata == nil || len(stored.TelegramMetadata.InlineKeyboard) != 1 {
		t.Errorf("expected telegram metadata with inline keyboard: %+v", stored.TelegramMetadata)
	}
}

func TestTelegramSendReplyKeyboard(t *testing.T) {
	t.Parallel()
	svc, _, store := newTestTelegramService("", "")
	ctx := context.Background()

	markupJSON := []byte(`{
		"keyboard": [
			[{"text": "Yes"}, {"text": "No"}]
		],
		"resize_keyboard": true,
		"one_time_keyboard": true
	}`)

	_, err := svc.SendMessage(ctx, service.SendMessageRequest{
		ChatID:      int64(14155551111),
		Text:        "Confirm:",
		ReplyMarkup: markupJSON,
	})
	if err != nil {
		t.Fatalf("SendMessage with reply keyboard failed: %v", err)
	}

	stored, err := store.GetMessageByEngineID(ctx, domain.EngineTelegram, "tg_msg_401")
	if err != nil {
		t.Fatalf("message not stored: %v", err)
	}
	if stored.InteractiveContent == nil || stored.InteractiveContent.Type != domain.InteractiveTypeTelegramReply {
		t.Errorf("expected reply interactive content: %+v", stored.InteractiveContent)
	}
	if stored.TelegramMetadata == nil || len(stored.TelegramMetadata.ReplyKeyboard) != 1 {
		t.Errorf("expected reply keyboard metadata: %+v", stored.TelegramMetadata)
	}
}

func TestTelegramSendVoice(t *testing.T) {
	t.Parallel()
	svc, _, store := newTestTelegramService("", "")
	ctx := context.Background()

	msg, err := svc.SendVoice(ctx, service.SendVoiceRequest{
		ChatID:   int64(14155551111),
		Voice:    "http://example.com/voice.oga",
		Caption:  "Voice note",
		Duration: 5,
	})
	if err != nil {
		t.Fatalf("SendVoice failed: %v", err)
	}
	if msg.Voice == nil || msg.Voice.Duration != 5 {
		t.Fatalf("unexpected voice message: %+v", msg)
	}

	stored, err := store.GetMessageByEngineID(ctx, domain.EngineTelegram, "tg_msg_401")
	if err != nil {
		t.Fatalf("voice message not stored: %v", err)
	}
	if stored.VoiceMetadata == nil || !stored.VoiceMetadata.IsVoiceNote || stored.VoiceMetadata.DurationSeconds != 5 {
		t.Errorf("unexpected voice metadata in store: %+v", stored.VoiceMetadata)
	}
}

func TestTelegramSendAudioAndPhoto(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestTelegramService("", "")
	ctx := context.Background()

	audioMsg, err := svc.SendAudio(ctx, service.SendAudioRequest{
		ChatID:    int64(14155551111),
		Audio:     "http://example.com/audio.mp3",
		Performer: "Artist",
		Title:     "Song",
		Duration:  180,
	})
	if err != nil || audioMsg.Audio == nil || audioMsg.Audio.Title != "Song" {
		t.Fatalf("SendAudio failed: %v, audio=%+v", err, audioMsg)
	}

	photoMsg, err := svc.SendPhoto(ctx, service.SendPhotoRequest{
		ChatID:  int64(14155551111),
		Photo:   "http://example.com/pic.jpg",
		Caption: "Look at this",
	})
	if err != nil || len(photoMsg.Photo) == 0 {
		t.Fatalf("SendPhoto failed: %v, photo=%+v", err, photoMsg)
	}
}

func TestTelegramWebhooksAndCommands(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestTelegramService("", "")
	ctx := context.Background()

	// Webhook
	ok, err := svc.SetWebhook(ctx, service.SetWebhookRequest{
		URL:         "http://test.local/tg_hook",
		SecretToken: "sec123",
	})
	if err != nil || !ok {
		t.Fatalf("SetWebhook failed: %v", err)
	}

	info, err := svc.GetWebhookInfo(ctx)
	if err != nil || info.URL != "http://test.local/tg_hook" {
		t.Fatalf("GetWebhookInfo failed: %v, info=%+v", err, info)
	}

	ok, err = svc.DeleteWebhook(ctx)
	if err != nil || !ok {
		t.Fatalf("DeleteWebhook failed: %v", err)
	}
	info, err = svc.GetWebhookInfo(ctx)
	if err != nil || info == nil || info.URL != "" {
		t.Errorf("expected empty URL after delete, got %+v", info)
	}

	// Commands
	cmds := []service.TelegramBotCommand{
		{Command: "start", Description: "Start bot"},
		{Command: "help", Description: "Get help"},
	}
	ok, err = svc.SetMyCommands(ctx, service.SetMyCommandsRequest{Commands: cmds})
	if err != nil || !ok {
		t.Fatalf("SetMyCommands failed: %v", err)
	}
	gotCmds, err := svc.GetMyCommands(ctx)
	if err != nil || len(gotCmds) != 2 || gotCmds[0].Command != "start" {
		t.Fatalf("GetMyCommands failed: %v, cmds=%+v", err, gotCmds)
	}

	// Chat menu button
	ok, err = svc.SetChatMenuButton(ctx, service.SetChatMenuButtonRequest{
		MenuButton: &service.TelegramMenuButton{Type: "commands"},
	})
	if err != nil || !ok {
		t.Fatalf("SetChatMenuButton failed: %v", err)
	}
	mb, err := svc.GetChatMenuButton(ctx)
	if err != nil || mb.Type != "commands" {
		t.Fatalf("GetChatMenuButton failed: %v, mb=%+v", err, mb)
	}
}

func TestTelegramMediaAndFile(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestTelegramService("", "")
	ctx := context.Background()

	fileID, err := svc.UploadMedia(ctx, "audio/ogg", "voice_memo.oga", []byte("audio-bytes"))
	if err != nil || fileID == "" {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	media, err := svc.GetMedia(ctx, fileID)
	if err != nil || string(media.Data) != "audio-bytes" {
		t.Fatalf("GetMedia failed: %v, media=%+v", err, media)
	}

	tgFile, err := svc.GetFile(ctx, fileID)
	if err != nil || tgFile.FileID != fileID || !strings.Contains(tgFile.FilePath, fileID) {
		t.Fatalf("GetFile failed: %v, tgFile=%+v", err, tgFile)
	}
}

func TestTelegramDispatchInboundToWebhook(t *testing.T) {
	t.Parallel()

	receivedHeader := make(chan string, 1)
	receivedBody := make(chan []byte, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader <- r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		body, _ := io.ReadAll(r.Body)
		receivedBody <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc, _, _ := newTestTelegramService(server.URL, "secret_header_token")
	ctx := context.Background()

	// Inbound text
	_, err := svc.DispatchInbound(ctx, service.InboundTelegramParams{
		FromID:    14155551111,
		ChatID:    14155551111,
		Username:  "user1",
		FirstName: "User",
		Text:      "Inbound hello",
	})
	if err != nil {
		t.Fatalf("DispatchInbound text failed: %v", err)
	}

	select {
	case h := <-receivedHeader:
		if h != "secret_header_token" {
			t.Errorf("expected secret header token, got %s", h)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook header")
	}

	select {
	case b := <-receivedBody:
		var update service.TelegramUpdate
		if err := json.Unmarshal(b, &update); err != nil {
			t.Fatalf("failed to unmarshal update: %v", err)
		}
		if update.Message == nil || update.Message.Text != "Inbound hello" {
			t.Errorf("unexpected update payload: %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook body")
	}

	// Inbound callback query
	_, err = svc.DispatchInbound(ctx, service.InboundTelegramParams{
		FromID:          14155551111,
		ChatID:          14155551111,
		Username:        "user1",
		FirstName:       "User",
		Type:            "callback",
		CallbackQueryID: "cbq_123",
		CallbackData:    "order_123",
		MessageID:       401,
		Text:            "Original message text",
	})
	if err != nil {
		t.Fatalf("DispatchInbound callback failed: %v", err)
	}

	select {
	case b := <-receivedBody:
		var update service.TelegramUpdate
		if err := json.Unmarshal(b, &update); err != nil {
			t.Fatalf("failed to unmarshal update: %v", err)
		}
		if update.CallbackQuery == nil || update.CallbackQuery.Data != "order_123" {
			t.Errorf("unexpected callback update: %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback webhook")
	}
	<-receivedHeader
}
