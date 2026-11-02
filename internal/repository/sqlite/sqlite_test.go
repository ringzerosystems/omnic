package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/repository/sqlite"
)

func TestSQLiteRepositories(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Test Phone Number Repository
	pn := &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		FriendlyName: "Support",
		Status:       "in-use",
		SmsURL:       "https://example.com/sms",
		SmsMethod:    "POST",
		DateCreated:  time.Now().UTC(),
		DateUpdated:  time.Now().UTC(),
	}
	if err := db.CreatePhoneNumber(ctx, pn); err != nil {
		t.Fatalf("Create phone number failed: %v", err)
	}

	gotPN, err := db.GetPhoneNumberBySID(ctx, "AC001", "PN001")
	if err != nil || gotPN.PhoneNumber != "+14155550199" {
		t.Fatalf("GetBySID phone number failed: got=%v err=%v", gotPN, err)
	}

	// Test Message Repository
	msg := &domain.Message{
		SID:         "SM001",
		AccountSID:  "AC001",
		To:          "+14155551111",
		From:        "+14155550199",
		Body:        "Persistent message",
		Status:      domain.StatusQueued,
		Direction:   domain.DirectionOutboundAPI,
		NumSegments: 1,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}
	if err := db.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("Create message failed: %v", err)
	}

	gotMsg, err := db.GetMessageBySID(ctx, "AC001", "SM001")
	if err != nil || gotMsg.Body != "Persistent message" {
		t.Fatalf("GetBySID message failed: got=%v err=%v", gotMsg, err)
	}

	// Test OptOut Repository
	if err := db.SetOptOut(ctx, "AC001", "+14155551111", true); err != nil {
		t.Fatalf("SetOptOut failed: %v", err)
	}
	opted, err := db.IsOptedOut(ctx, "AC001", "+14155551111")
	if err != nil || !opted {
		t.Fatalf("expected opted out, got %v (err=%v)", opted, err)
	}

	// Test Media Repository
	media := &domain.Media{
		SID:         "ME001",
		AccountSID:  "AC001",
		ParentSID:   "SM001",
		ContentType: "image/png",
		Data:        []byte("rawbytes"),
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}
	if err := db.CreateMedia(ctx, media); err != nil {
		t.Fatalf("Create media failed: %v", err)
	}
	gotMedia, err := db.GetMediaBySID(ctx, "AC001", "SM001", "ME001")
	if err != nil || string(gotMedia.Data) != "rawbytes" {
		t.Fatalf("GetBySID media failed: got=%v err=%v", gotMedia, err)
	}

	// Test Messaging Service Repository
	svc := &domain.MessagingService{
		SID:          "MG001",
		AccountSID:   "AC001",
		FriendlyName: "Pool",
		DateCreated:  time.Now().UTC(),
		DateUpdated:  time.Now().UTC(),
	}
	if err := db.CreateMessagingService(ctx, svc); err != nil {
		t.Fatalf("Create messaging service failed: %v", err)
	}
	gotSvc, err := db.GetMessagingServiceBySID(ctx, "AC001", "MG001")
	if err != nil || gotSvc.FriendlyName != "Pool" {
		t.Fatalf("GetBySID service failed: got=%v err=%v", gotSvc, err)
	}

	// Delete test
	if err := db.DeleteMessage(ctx, "AC001", "SM001"); err != nil {
		t.Fatalf("Delete message failed: %v", err)
	}
	_, err = db.GetMessageBySID(ctx, "AC001", "SM001")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWhatsAppSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer func() { _ = db.Close() }()

	msg := &domain.Message{
		SID:         "wamid.sq123",
		AccountSID:  "AC001",
		To:          "+14155551111",
		From:        "+14155552222",
		Body:        "WhatsApp SQL message",
		Status:      domain.StatusQueued,
		Direction:   domain.DirectionOutboundAPI,
		Engine:      domain.EngineWhatsApp,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
		InteractiveContent: &domain.InteractiveContent{
			Type: domain.InteractiveTypeButton,
			Body: "Choose an option:",
			Buttons: []domain.ButtonItem{
				{ID: "btn_1", Title: "Accept"},
			},
		},
		VoiceMetadata: &domain.VoiceMetadata{
			DurationSeconds: 8.2,
			IsVoiceNote:     true,
			Waveform:        []int{5, 15, 25},
		},
	}

	if err := db.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("Create WhatsApp message failed: %v", err)
	}

	got, err := db.GetMessageByEngineID(ctx, domain.EngineWhatsApp, "wamid.sq123")
	if err != nil {
		t.Fatalf("GetByEngineID failed: %v", err)
	}
	if got.Engine != domain.EngineWhatsApp {
		t.Errorf("expected engine %s, got %s", domain.EngineWhatsApp, got.Engine)
	}
	if got.InteractiveContent == nil || len(got.InteractiveContent.Buttons) != 1 {
		t.Errorf("interactive content not restored: %+v", got.InteractiveContent)
	}
	if got.VoiceMetadata == nil || !got.VoiceMetadata.IsVoiceNote || len(got.VoiceMetadata.Waveform) != 3 {
		t.Errorf("voice metadata not restored: %+v", got.VoiceMetadata)
	}

	// Filter
	waList, total, err := db.ListMessages(ctx, "AC001", repository.MessageFilter{Engine: domain.EngineWhatsApp})
	if err != nil || total != 1 || len(waList) != 1 {
		t.Fatalf("List by engine failed: total=%d, len=%d, err=%v", total, len(waList), err)
	}

	twList, twTotal, _ := db.ListMessages(ctx, "AC001", repository.MessageFilter{Engine: domain.EngineTwilio})
	if twTotal != 0 || len(twList) != 0 {
		t.Errorf("expected 0 twilio messages, got %d", twTotal)
	}

	// Test Telegram Message Persistence
	tgMsg := &domain.Message{
		SID:        "tg_msg_999",
		AccountSID: "telegram",
		To:         "14155551111",
		From:       "omnic_bot",
		Body:       "Telegram sqlite test",
		Status:     domain.StatusSent,
		Direction:  domain.DirectionOutboundAPI,
		Engine:     domain.EngineTelegram,
		TelegramMetadata: &domain.TelegramMetadata{
			ChatID:    14155551111,
			MessageID: 999,
			InlineKeyboard: [][]domain.ButtonItem{
				{{ID: "btn_tg", Title: "TG Button"}},
			},
			CallbackData: "tg_data",
		},
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}
	if err := db.CreateMessage(ctx, tgMsg); err != nil {
		t.Fatalf("Create Telegram message failed: %v", err)
	}
	gotTG, err := db.GetMessageByEngineID(ctx, domain.EngineTelegram, "tg_msg_999")
	if err != nil {
		t.Fatalf("GetByEngineID for Telegram failed: %v", err)
	}
	if gotTG.TelegramMetadata == nil || gotTG.TelegramMetadata.ChatID != 14155551111 || gotTG.TelegramMetadata.CallbackData != "tg_data" {
		t.Fatalf("TelegramMetadata not restored: %+v", gotTG.TelegramMetadata)
	}
}
