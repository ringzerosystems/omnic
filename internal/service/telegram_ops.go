package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
)

// GetMe returns bot profile information.
func (s *TelegramService) GetMe(_ context.Context) (*TelegramUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &TelegramUser{
		ID:        s.botID,
		IsBot:     true,
		FirstName: s.cfg.BotName,
		Username:  s.cfg.BotUsername,
	}, nil
}

// SetWebhook registers an upstream webhook URL and secret token.
func (s *TelegramService) SetWebhook(_ context.Context, req SetWebhookRequest) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.UpstreamURL = req.URL
	s.cfg.SecretToken = req.SecretToken
	if s.dispatcher != nil {
		s.dispatcher.SetUpstreamURL(req.URL)
		s.dispatcher.SetSecretToken(req.SecretToken)
	}
	return true, nil
}

// DeleteWebhook removes any active webhook configuration.
func (s *TelegramService) DeleteWebhook(_ context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.UpstreamURL = ""
	if s.dispatcher != nil {
		s.dispatcher.SetUpstreamURL("")
	}
	return true, nil
}

// GetWebhookInfo retrieves current webhook registration details.
func (s *TelegramService) GetWebhookInfo(_ context.Context) (*TelegramWebhookInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &TelegramWebhookInfo{
		URL:                  s.cfg.UpstreamURL,
		HasCustomCertificate: false,
		PendingUpdateCount:   0,
		MaxConnections:       40,
	}, nil
}

// SetMyCommands saves registered slash bot commands.
func (s *TelegramService) SetMyCommands(_ context.Context, req SetMyCommandsRequest) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = make([]TelegramBotCommand, len(req.Commands))
	copy(s.commands, req.Commands)
	return true, nil
}

// GetMyCommands returns all registered slash bot commands.
func (s *TelegramService) GetMyCommands(_ context.Context) ([]TelegramBotCommand, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]TelegramBotCommand, len(s.commands))
	copy(res, s.commands)
	return res, nil
}

// SetChatMenuButton configures the chat menu button style.
func (s *TelegramService) SetChatMenuButton(_ context.Context, req SetChatMenuButtonRequest) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.menuButton = req.MenuButton
	return true, nil
}

// GetChatMenuButton returns current chat menu button configuration.
func (s *TelegramService) GetChatMenuButton(_ context.Context) (*TelegramMenuButton, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.menuButton == nil {
		return &TelegramMenuButton{Type: "default"}, nil
	}
	return s.menuButton, nil
}

// GetConfig returns the active Telegram configuration.
func (s *TelegramService) GetConfig() TelegramConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// DispatchInbound injects a message or callback into the emulator and forwards to bot webhook.
func (s *TelegramService) DispatchInbound(ctx context.Context, params InboundTelegramParams) (*domain.Message, error) {
	msgID := int(s.nextMsgID.Add(1))
	updateID := int(s.nextUpdateID.Add(1))
	now := time.Now().UTC()

	meta := &domain.TelegramMetadata{
		ChatID:    params.ChatID,
		MessageID: msgID,
	}

	body := params.Text
	var voiceMeta *domain.VoiceMetadata
	var mediaURLs []string
	var interactive *domain.InteractiveContent

	if params.Voice {
		body = "[Voice Note]"
		voiceMeta = &domain.VoiceMetadata{
			DurationSeconds: params.DurationSeconds,
			IsVoiceNote:     true,
			Waveform:        params.Waveform,
		}
		if params.MediaID != "" {
			mediaURLs = []string{params.MediaID}
		}
	} else if params.Type == "callback" || params.CallbackData != "" {
		body = params.CallbackData
		meta.CallbackQueryID = params.CallbackQueryID
		meta.CallbackData = params.CallbackData
		interactive = &domain.InteractiveContent{
			Type:       domain.InteractiveTypeTelegramCallback,
			Body:       params.CallbackData,
			SelectedID: params.CallbackData,
		}
	}

	msg := &domain.Message{
		SID:                fmt.Sprintf("tg_msg_%d", msgID),
		AccountSID:         "telegram",
		From:               strconv.FormatInt(params.FromID, 10),
		To:                 s.cfg.BotUsername,
		Body:               body,
		Status:             domain.StatusReceived,
		Direction:          domain.DirectionInbound,
		Engine:             domain.EngineTelegram,
		TelegramMetadata:   meta,
		VoiceMetadata:      voiceMeta,
		InteractiveContent: interactive,
		MediaURLs:          mediaURLs,
		DateCreated:        now,
		DateUpdated:        now,
	}

	if err := s.store.CreateMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("storing inbound telegram message: %w", err)
	}

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageInbound,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}

	update := buildTelegramUpdate(updateID, msgID, now, params)
	if s.dispatcher != nil {
		_ = s.dispatcher.DispatchUpdate(ctx, update)
	}

	return msg, nil
}

func buildTelegramUpdate(updateID, msgID int, now time.Time, p InboundTelegramParams) *TelegramUpdate {
	update := &TelegramUpdate{UpdateID: updateID}
	u := TelegramUser{ID: p.FromID, FirstName: p.FirstName, Username: p.Username}
	ch := TelegramChat{ID: p.ChatID, Type: "private", FirstName: p.FirstName, Username: p.Username}

	if p.Type == "callback" || p.CallbackData != "" {
		cbqID := p.CallbackQueryID
		if cbqID == "" {
			cbqID = fmt.Sprintf("cbq_%d", updateID)
		}
		update.CallbackQuery = &TelegramCallbackQuery{
			ID:           cbqID,
			From:         u,
			ChatInstance: fmt.Sprintf("ci_%d", p.ChatID),
			Data:         p.CallbackData,
			Message: &TelegramMessage{
				MessageID: p.MessageID,
				Chat:      ch,
				Date:      now.Unix(),
				Text:      p.Text,
			},
		}
		return update
	}

	if p.Voice {
		mediaID := p.MediaID
		if mediaID == "" {
			mediaID = fmt.Sprintf("tg_file_voice_%d", msgID)
		}
		update.Message = &TelegramMessage{
			MessageID: msgID,
			From:      &u,
			Chat:      ch,
			Date:      now.Unix(),
			Voice: &TelegramVoice{
				FileID:       mediaID,
				FileUniqueID: "uniq_" + mediaID,
				Duration:     int(p.DurationSeconds),
				MimeType:     "audio/ogg",
				FileSize:     12800,
			},
		}
		return update
	}

	update.Message = &TelegramMessage{
		MessageID: msgID,
		From:      &u,
		Chat:      ch,
		Date:      now.Unix(),
		Text:      p.Text,
	}
	return update
}
