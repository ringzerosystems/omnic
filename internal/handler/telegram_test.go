package handler_test

import (
	"bytes"
	"encoding/json"
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

func setupTelegramTestServer() (*echo.Echo, *handler.Handler, *service.TelegramService, *memory.Store) {
	e := echo.New()
	store := memory.NewStore()
	broker := service.NewMemoryEventBroker()
	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	dispatcher := service.NewWebhookDispatcher(store, "token")

	cfg := service.TelegramConfig{
		BotToken:      "123456789:mock_token_dev",
		BotUsername:   "omnic_bot",
		BotName:       "OmnicBot",
		DeliveryDelay: 10 * time.Millisecond,
	}
	tgSvc := service.NewTelegramService(store, broker, cfg)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, dispatcher)
	h.SetBroker(broker)
	h.SetTelegram(tgSvc)
	h.RegisterRoutes(e)

	return e, h, tgSvc, store
}

func TestTelegramGetMe(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupTelegramTestServer()

	// 1. Success with valid token
	req := httptest.NewRequest(http.MethodGet, "/bot123456789:mock_token_dev/getMe", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var res service.TelegramResponse[service.TelegramUser]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Ok {
		t.Fatalf("unexpected getMe response: %+v, err=%v", res, err)
	}
	if res.Result.Username != "omnic_bot" {
		t.Errorf("expected omnic_bot, got %s", res.Result.Username)
	}

	// 2. Unauthorized with invalid token
	reqBad := httptest.NewRequest(http.MethodGet, "/botinvalid_token/getMe", nil)
	recBad := httptest.NewRecorder()
	e.ServeHTTP(recBad, reqBad)

	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recBad.Code)
	}
}

func TestTelegramSendMessage(t *testing.T) {
	t.Parallel()
	e, _, _, store := setupTelegramTestServer()

	payload := `{
		"chat_id": 14155551111,
		"text": "Hello Telegram User",
		"reply_markup": {
			"inline_keyboard": [
				[{"text": "Option 1", "callback_data": "opt_1"}]
			]
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendMessage", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res service.TelegramResponse[service.TelegramMessage]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Ok {
		t.Fatalf("failed to decode message response: %v", err)
	}
	if res.Result.Text != "Hello Telegram User" {
		t.Errorf("unexpected message text: %s", res.Result.Text)
	}

	// Verify persistence in store
	list, total, err := store.ListMessages(req.Context(), "telegram", domain.MessageFilter{Engine: domain.EngineTelegram})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 telegram message in store, got %d (err=%v)", total, err)
	}
	if list[0].InteractiveContent == nil || len(list[0].InteractiveContent.Buttons) != 1 {
		t.Errorf("expected interactive button stored: %+v", list[0].InteractiveContent)
	}
}

func TestTelegramSendVoice(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupTelegramTestServer()

	// JSON payload sendVoice
	payload := `{
		"chat_id": 14155551111,
		"voice": "http://localhost:8080/file/bot_tok/sample.oga",
		"caption": "Voice note memo",
		"duration": 4
	}`
	req := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendVoice", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res service.TelegramResponse[service.TelegramMessage]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Ok {
		t.Fatalf("failed to decode voice response: %v", err)
	}
	if res.Result.Voice == nil || res.Result.Voice.Duration != 4 {
		t.Errorf("unexpected voice info: %+v", res.Result.Voice)
	}
}

func TestTelegramSendVoiceMultipart(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupTelegramTestServer()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("chat_id", "14155551111")
	_ = writer.WriteField("caption", "Multipart voice")
	_ = writer.WriteField("duration", "6")
	part, err := writer.CreateFormFile("voice", "test_memo.oga")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	_, _ = part.Write([]byte("fake-ogg-opus-bytes"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendVoice", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for multipart sendVoice, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTelegramSendAudioAndPhoto(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupTelegramTestServer()

	// sendAudio
	audioJSON := `{"chat_id": 14155551111, "audio": "http://example.com/a.mp3", "title": "Track"}`
	reqAudio := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendAudio", strings.NewReader(audioJSON))
	reqAudio.Header.Set("Content-Type", "application/json")
	recAudio := httptest.NewRecorder()
	e.ServeHTTP(recAudio, reqAudio)
	if recAudio.Code != http.StatusOK {
		t.Fatalf("expected 200 for sendAudio, got %d: %s", recAudio.Code, recAudio.Body.String())
	}

	// sendPhoto
	photoJSON := `{"chat_id": 14155551111, "photo": "http://example.com/p.jpg", "caption": "Pic"}`
	reqPhoto := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendPhoto", strings.NewReader(photoJSON))
	reqPhoto.Header.Set("Content-Type", "application/json")
	recPhoto := httptest.NewRecorder()
	e.ServeHTTP(recPhoto, reqPhoto)
	if recPhoto.Code != http.StatusOK {
		t.Fatalf("expected 200 for sendPhoto, got %d: %s", recPhoto.Code, recPhoto.Body.String())
	}
}

func TestTelegramWebhooksAndCommands(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupTelegramTestServer()

	// setWebhook
	setWh := `{"url": "http://agent.local/hook", "secret_token": "sec_tok"}`
	req := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/setWebhook", strings.NewReader(setWh))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setWebhook failed: %d", rec.Code)
	}

	// getWebhookInfo
	reqInfo := httptest.NewRequest(http.MethodGet, "/bot123456789:mock_token_dev/getWebhookInfo", nil)
	recInfo := httptest.NewRecorder()
	e.ServeHTTP(recInfo, reqInfo)
	if recInfo.Code != http.StatusOK {
		t.Fatalf("getWebhookInfo failed: %d", recInfo.Code)
	}

	// setMyCommands
	cmdPayload := `{"commands": [{"command": "start", "description": "Start"}]}`
	reqCmd := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/setMyCommands", strings.NewReader(cmdPayload))
	reqCmd.Header.Set("Content-Type", "application/json")
	recCmd := httptest.NewRecorder()
	e.ServeHTTP(recCmd, reqCmd)
	if recCmd.Code != http.StatusOK {
		t.Fatalf("setMyCommands failed: %d", recCmd.Code)
	}

	// getMyCommands
	reqGetCmd := httptest.NewRequest(http.MethodGet, "/bot123456789:mock_token_dev/getMyCommands", nil)
	recGetCmd := httptest.NewRecorder()
	e.ServeHTTP(recGetCmd, reqGetCmd)
	if recGetCmd.Code != http.StatusOK {
		t.Fatalf("getMyCommands failed: %d", recGetCmd.Code)
	}

	// setChatMenuButton
	menuPayload := `{"menu_button": {"type": "commands"}}`
	reqMenu := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/setChatMenuButton", strings.NewReader(menuPayload))
	reqMenu.Header.Set("Content-Type", "application/json")
	recMenu := httptest.NewRecorder()
	e.ServeHTTP(recMenu, reqMenu)
	if recMenu.Code != http.StatusOK {
		t.Fatalf("setChatMenuButton failed: %d", recMenu.Code)
	}

	// getChatMenuButton
	reqGetMenu := httptest.NewRequest(http.MethodGet, "/bot123456789:mock_token_dev/getChatMenuButton", nil)
	recGetMenu := httptest.NewRecorder()
	e.ServeHTTP(recGetMenu, reqGetMenu)
	if recGetMenu.Code != http.StatusOK {
		t.Fatalf("getChatMenuButton failed: %d", recGetMenu.Code)
	}
}

func TestTelegramGetFileAndDownload(t *testing.T) {
	t.Parallel()
	e, _, tgSvc, _ := setupTelegramTestServer()

	ctx := t.Context()
	fileID, err := tgSvc.UploadMedia(ctx, "audio/ogg", "sample_voice.oga", []byte("sample-audio-bytes"))
	if err != nil {
		t.Fatalf("failed to upload media: %v", err)
	}

	// getFile
	reqFile := httptest.NewRequest(http.MethodGet, "/bot123456789:mock_token_dev/getFile?file_id="+fileID, nil)
	recFile := httptest.NewRecorder()
	e.ServeHTTP(recFile, reqFile)
	if recFile.Code != http.StatusOK {
		t.Fatalf("getFile failed: %d", recFile.Code)
	}

	// File download
	reqDown := httptest.NewRequest(http.MethodGet, "/file/bot123456789:mock_token_dev/"+fileID, nil)
	recDown := httptest.NewRecorder()
	e.ServeHTTP(recDown, reqDown)
	if recDown.Code != http.StatusOK {
		t.Fatalf("file download failed: %d", recDown.Code)
	}
	if recDown.Body.String() != "sample-audio-bytes" {
		t.Errorf("unexpected file content: %s", recDown.Body.String())
	}
}

func TestTelegramInboundControlPlane(t *testing.T) {
	t.Parallel()
	e, _, _, store := setupTelegramTestServer()

	// Inbound message through /emu/inbound/telegram
	inboundJSON := `{
		"from_id": 14155551111,
		"chat_id": 14155551111,
		"username": "tester",
		"first_name": "Tester",
		"text": "Hello from handset"
	}`

	req := httptest.NewRequest(http.MethodPost, "/emu/inbound/telegram", strings.NewReader(inboundJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	list, total, err := store.ListMessages(req.Context(), "telegram", domain.MessageFilter{Engine: domain.EngineTelegram})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("expected inbound message in store, total=%d", total)
	}
	if list[0].Body != "Hello from handset" {
		t.Errorf("unexpected message body: %s", list[0].Body)
	}
}

func TestTelegramSendChatAction(t *testing.T) {
	t.Parallel()
	e, _, _, _ := setupTelegramTestServer()

	// 1. JSON POST
	bodyJSON := `{"chat_id": 14155551111, "action": "typing"}`
	req := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendChatAction", strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res service.TelegramResponse[bool]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Ok || !res.Result {
		t.Fatalf("unexpected sendChatAction response: %+v", res)
	}

	// 2. Form POST
	reqForm := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendChatAction", strings.NewReader("chat_id=14155551111&action=upload_photo"))
	reqForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recForm := httptest.NewRecorder()
	e.ServeHTTP(recForm, reqForm)

	if recForm.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recForm.Code, recForm.Body.String())
	}

	// 3. GET
	reqGet := httptest.NewRequest(http.MethodGet, "/bot123456789:mock_token_dev/sendChatAction?chat_id=14155551111&action=record_voice", nil)
	recGet := httptest.NewRecorder()
	e.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recGet.Code, recGet.Body.String())
	}

	// 4. Missing chat_id returns error
	badReq := httptest.NewRequest(http.MethodPost, "/bot123456789:mock_token_dev/sendChatAction", strings.NewReader(`{"action":"typing"}`))
	badReq.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	e.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing chat_id, got %d", badRec.Code)
	}
}
