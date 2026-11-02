package memory_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
)

func TestMessageRepository(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	msg := &domain.Message{
		SID:         "SM123",
		AccountSID:  "AC001",
		To:          "+14155551111",
		From:        "+14155552222",
		Body:        "Test message",
		Status:      domain.StatusQueued,
		Direction:   domain.DirectionOutboundAPI,
		NumSegments: 1,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}

	if err := store.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetMessageBySID(ctx, "AC001", "SM123")
	if err != nil {
		t.Fatalf("GetBySID failed: %v", err)
	}
	if got.Body != msg.Body {
		t.Errorf("got Body = %q, want %q", got.Body, msg.Body)
	}

	// Update status
	msg.Status = domain.StatusDelivered
	if err := store.UpdateMessage(ctx, msg); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err = store.GetMessageBySID(ctx, "AC001", "SM123")
	if err != nil || got == nil {
		t.Fatalf("GetBySID failed: %v", err)
	}
	if got.Status != domain.StatusDelivered {
		t.Errorf("got Status = %v, want delivered", got.Status)
	}

	// List
	list, total, err := store.ListMessages(ctx, "AC001", repository.MessageFilter{Limit: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("List failed: list=%d total=%d err=%v", len(list), total, err)
	}

	// Delete
	if err := store.DeleteMessage(ctx, "AC001", "SM123"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetMessageBySID(ctx, "AC001", "SM123")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestPhoneNumberRepository(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	pn := &domain.PhoneNumber{
		SID:          "PN123",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		FriendlyName: "Support",
		Status:       "in-use",
		SmsURL:       "https://example.com/sms",
		SmsMethod:    "POST",
		Capabilities: domain.Capabilities{SMS: true, MMS: true, Voice: true},
		DateCreated:  time.Now().UTC(),
		DateUpdated:  time.Now().UTC(),
	}

	if err := store.CreatePhoneNumber(ctx, pn); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Duplicate phone number should return conflict
	duplicate := &domain.PhoneNumber{
		SID:         "PN456",
		AccountSID:  "AC001",
		PhoneNumber: "+14155550199",
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}
	if err := store.CreatePhoneNumber(ctx, duplicate); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("expected ErrConflict on duplicate phone number, got %v", err)
	}

	got, err := store.FindPhoneNumberByNumber(ctx, "+14155550199")
	if err != nil || got.SID != "PN123" {
		t.Fatalf("FindByNumber failed: got=%v err=%v", got, err)
	}

	list, total, err := store.ListPhoneNumbers(ctx, "AC001", repository.PhoneNumberFilter{Limit: 10})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("List failed: list=%d total=%d err=%v", len(list), total, err)
	}

	if err := store.DeletePhoneNumber(ctx, "AC001", "PN123"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.GetPhoneNumberBySID(ctx, "AC001", "PN123")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMediaRepository(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	media := &domain.Media{
		SID:         "ME123",
		AccountSID:  "AC001",
		ParentSID:   "MM123",
		ContentType: "image/jpeg",
		Data:        []byte{0xFF, 0xD8, 0xFF},
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}

	if err := store.CreateMedia(ctx, media); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetMediaBySID(ctx, "AC001", "MM123", "ME123")
	if err != nil || got.ContentType != "image/jpeg" {
		t.Fatalf("GetBySID failed: got=%v err=%v", got, err)
	}

	list, err := store.ListMedia(ctx, "AC001", "MM123")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByParent failed: len=%d err=%v", len(list), err)
	}

	if err := store.DeleteMedia(ctx, "AC001", "MM123", "ME123"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = store.GetMediaBySID(ctx, "AC001", "MM123", "ME123")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestOptOutRepository(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	opted, err := store.IsOptedOut(ctx, "AC001", "+14155551111")
	if err != nil || opted {
		t.Fatalf("expected not opted out initially, got opted=%v err=%v", opted, err)
	}

	if err := store.SetOptOut(ctx, "AC001", "+14155551111", true); err != nil {
		t.Fatalf("SetOptOut failed: %v", err)
	}

	opted, err = store.IsOptedOut(ctx, "AC001", "+14155551111")
	if err != nil || !opted {
		t.Fatalf("expected opted out, got opted=%v err=%v", opted, err)
	}

	if err := store.SetOptOut(ctx, "AC001", "+14155551111", false); err != nil {
		t.Fatalf("SetOptOut false failed: %v", err)
	}
	opted, _ = store.IsOptedOut(ctx, "AC001", "+14155551111")
	if opted {
		t.Fatalf("expected not opted out after reset")
	}
}

func TestMessagingServiceRepository(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	svc := &domain.MessagingService{
		SID:          "MG123",
		AccountSID:   "AC001",
		FriendlyName: "My Service",
		StickySender: true,
		DateCreated:  time.Now().UTC(),
		DateUpdated:  time.Now().UTC(),
	}

	if err := store.CreateMessagingService(ctx, svc); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	spn := &domain.ServicePhoneNumber{
		ServiceSID:     "MG123",
		PhoneNumberSID: "PN123",
		PhoneNumber:    "+14155550199",
		DateCreated:    time.Now().UTC(),
	}
	if err := store.AddServicePhoneNumber(ctx, spn); err != nil {
		t.Fatalf("AddPhoneNumber failed: %v", err)
	}

	numbers, err := store.ListServicePhoneNumbers(ctx, "MG123")
	if err != nil || len(numbers) != 1 {
		t.Fatalf("ListPhoneNumbers failed: len=%d err=%v", len(numbers), err)
	}

	if err := store.RemoveServicePhoneNumber(ctx, "MG123", "PN123"); err != nil {
		t.Fatalf("RemovePhoneNumber failed: %v", err)
	}

	numbers, _ = store.ListServicePhoneNumbers(ctx, "MG123")
	if len(numbers) != 0 {
		t.Fatalf("expected 0 numbers, got %d", len(numbers))
	}
}

func TestConcurrentSafety(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sid := "SM" + string(rune('A'+(id%26))) + string(rune('0'+(id%10)))
			msg := &domain.Message{
				SID:         sid,
				AccountSID:  "AC001",
				To:          "+14155550000",
				From:        "+14155550001",
				Body:        "Concurrent test",
				Status:      domain.StatusQueued,
				DateCreated: time.Now().UTC(),
				DateUpdated: time.Now().UTC(),
			}
			_ = store.CreateMessage(ctx, msg)
			_, _ = store.GetMessageBySID(ctx, "AC001", sid)
			_, _, _ = store.ListMessages(ctx, "AC001", repository.MessageFilter{Limit: 10})
		}(i)
	}
	wg.Wait()
}

func TestWhatsAppMessageRepository(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	msg := &domain.Message{
		SID:         "wamid.12345",
		AccountSID:  "AC001",
		To:          "+14155551111",
		From:        "+14155552222",
		Body:        "Interactive message",
		Status:      domain.StatusQueued,
		Direction:   domain.DirectionOutboundAPI,
		Engine:      domain.EngineWhatsApp,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
		InteractiveContent: &domain.InteractiveContent{
			Type: domain.InteractiveTypeButton,
			Body: "Choose:",
			Buttons: []domain.ButtonItem{
				{ID: "btn_1", Title: "Option 1"},
			},
		},
		VoiceMetadata: &domain.VoiceMetadata{
			DurationSeconds: 4.5,
			IsVoiceNote:     true,
		},
	}

	if err := store.CreateMessage(ctx, msg); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetMessageByEngineID(ctx, domain.EngineWhatsApp, "wamid.12345")
	if err != nil {
		t.Fatalf("GetByEngineID failed: %v", err)
	}
	if got.Engine != domain.EngineWhatsApp {
		t.Errorf("expected engine %s, got %s", domain.EngineWhatsApp, got.Engine)
	}
	if got.InteractiveContent == nil || len(got.InteractiveContent.Buttons) != 1 {
		t.Errorf("interactive content not preserved: %+v", got.InteractiveContent)
	}
	if got.VoiceMetadata == nil || !got.VoiceMetadata.IsVoiceNote {
		t.Errorf("voice metadata not preserved: %+v", got.VoiceMetadata)
	}

	// Filter by engine
	list, total, err := store.ListMessages(ctx, "AC001", repository.MessageFilter{Engine: domain.EngineWhatsApp})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("List by engine failed: total=%d, len=%d, err=%v", total, len(list), err)
	}

	twilioList, twilioTotal, _ := store.ListMessages(ctx, "AC001", repository.MessageFilter{Engine: domain.EngineTwilio})
	if twilioTotal != 0 || len(twilioList) != 0 {
		t.Errorf("expected 0 twilio messages, got %d", twilioTotal)
	}

	tgMsg := &domain.Message{
		SID:         "tg_msg_mem",
		AccountSID:  "telegram",
		To:          "14155551111",
		From:        "omnic_bot",
		Body:        "Telegram in-memory test",
		Status:      domain.StatusSent,
		Direction:   domain.DirectionOutboundAPI,
		Engine:      domain.EngineTelegram,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
		TelegramMetadata: &domain.TelegramMetadata{
			ChatID:    14155551111,
			MessageID: 777,
		},
	}
	if err := store.CreateMessage(ctx, tgMsg); err != nil {
		t.Fatalf("Create Telegram message failed: %v", err)
	}
	gotTG, err := store.GetMessageByEngineID(ctx, domain.EngineTelegram, "tg_msg_mem")
	if err != nil {
		t.Fatalf("GetByEngineID failed: %v", err)
	}
	if gotTG.TelegramMetadata == nil || gotTG.TelegramMetadata.ChatID != 14155551111 {
		t.Fatalf("TelegramMetadata not preserved: %+v", gotTG.TelegramMetadata)
	}
}
