package domain_test

import (
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
)

func TestDomainEntities(t *testing.T) {
	t.Parallel()

	buttons := []domain.ButtonItem{
		{ID: "btn_1", Title: "Yes"},
		{ID: "btn_2", Title: "No"},
	}

	interactive := &domain.InteractiveContent{
		Type:    domain.InteractiveTypeButton,
		Header:  "Header",
		Body:    "Please choose:",
		Footer:  "Footer",
		Buttons: buttons,
	}

	voice := &domain.VoiceMetadata{
		DurationSeconds: 12.5,
		IsVoiceNote:     true,
		Waveform:        []int{10, 20, 30, 40},
	}

	now := time.Now()
	msg := &domain.Message{
		SID:                "wamid.test1234",
		AccountSID:         "AC001",
		To:                 "+14155550100",
		From:               "+14155550199",
		Body:               "Voice note or buttons",
		Status:             domain.StatusSent,
		Direction:          domain.DirectionOutboundAPI,
		Engine:             domain.EngineWhatsApp,
		InteractiveContent: interactive,
		VoiceMetadata:      voice,
		DateCreated:        now,
		DateUpdated:        now,
	}

	if msg.Engine != domain.EngineWhatsApp {
		t.Errorf("expected engine %s, got %s", domain.EngineWhatsApp, msg.Engine)
	}
	if msg.InteractiveContent == nil || len(msg.InteractiveContent.Buttons) != 2 {
		t.Errorf("expected 2 interactive buttons, got %+v", msg.InteractiveContent)
	}
	if msg.VoiceMetadata == nil || !msg.VoiceMetadata.IsVoiceNote {
		t.Errorf("expected voice note metadata, got %+v", msg.VoiceMetadata)
	}

	filter := domain.MessageFilter{
		Engine: domain.EngineWhatsApp,
		Limit:  10,
	}
	if filter.Engine != domain.EngineWhatsApp {
		t.Errorf("expected filter engine %s, got %s", domain.EngineWhatsApp, filter.Engine)
	}

	tgMetadata := &domain.TelegramMetadata{
		ChatID:    14155551111,
		MessageID: 101,
		InlineKeyboard: [][]domain.ButtonItem{
			{{ID: "btn_opt1", Title: "Option 1"}},
		},
		CallbackData: "opt1",
	}

	tgMsg := &domain.Message{
		SID:              "tg_msg_101",
		AccountSID:       "telegram",
		To:               "14155551111",
		From:             "omnic_bot",
		Body:             "Telegram message",
		Status:           domain.StatusSent,
		Direction:        domain.DirectionOutboundAPI,
		Engine:           domain.EngineTelegram,
		TelegramMetadata: tgMetadata,
		DateCreated:      now,
		DateUpdated:      now,
	}

	if tgMsg.Engine != domain.EngineTelegram {
		t.Errorf("expected engine %s, got %s", domain.EngineTelegram, tgMsg.Engine)
	}
	if tgMsg.TelegramMetadata == nil || tgMsg.TelegramMetadata.ChatID != 14155551111 {
		t.Errorf("expected TelegramMetadata with chat_id 14155551111, got %+v", tgMsg.TelegramMetadata)
	}
}
