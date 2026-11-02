package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// TelegramConfig holds service credentials and configuration for the Telegram engine.
type TelegramConfig struct {
	BotToken      string
	BotUsername   string
	BotName       string
	UpstreamURL   string
	SecretToken   string
	DeliveryDelay time.Duration
}

// TelegramUser represents a Telegram user or bot.
type TelegramUser struct {
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
}

// TelegramChat represents a Telegram chat.
type TelegramChat struct {
	Type      string `json:"type"`
	FirstName string `json:"first_name,omitempty"`
	Username  string `json:"username,omitempty"`
	ID        int64  `json:"id"`
}

// TelegramFile represents file metadata.
type TelegramFile struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FilePath     string `json:"file_path,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// TelegramVoice represents a voice note.
type TelegramVoice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	Duration     int    `json:"duration"`
}

// TelegramAudio represents an audio file.
type TelegramAudio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Performer    string `json:"performer,omitempty"`
	Title        string `json:"title,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	Duration     int    `json:"duration"`
}

// TelegramPhotoSize represents one size of a photo.
type TelegramPhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

// TelegramInlineKeyboardButton represents a button in an inline keyboard.
type TelegramInlineKeyboardButton struct {
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
}

// TelegramInlineKeyboardMarkup represents an inline keyboard.
type TelegramInlineKeyboardMarkup struct {
	InlineKeyboard [][]TelegramInlineKeyboardButton `json:"inline_keyboard"`
}

// TelegramKeyboardButton represents a button in a custom reply keyboard.
type TelegramKeyboardButton struct {
	Text string `json:"text"`
}

// TelegramReplyKeyboardMarkup represents a custom keyboard with reply options.
type TelegramReplyKeyboardMarkup struct {
	InputFieldPlaceholder string                     `json:"input_field_placeholder,omitempty"`
	Keyboard              [][]TelegramKeyboardButton `json:"keyboard"`
	ResizeKeyboard        bool                       `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard       bool                       `json:"one_time_keyboard,omitempty"`
	IsPersistent          bool                       `json:"is_persistent,omitempty"`
}

// TelegramReplyKeyboardRemove removes custom keyboard.
type TelegramReplyKeyboardRemove struct {
	RemoveKeyboard bool `json:"remove_keyboard"`
}

// TelegramForceReply displays a reply interface to the user.
type TelegramForceReply struct {
	InputFieldPlaceholder string `json:"input_field_placeholder,omitempty"`
	ForceReply            bool   `json:"force_reply"`
}

// TelegramBotCommand represents a bot command.
type TelegramBotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// TelegramMenuButton represents the chat menu button.
type TelegramMenuButton struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TelegramMessage represents a Telegram message.
type TelegramMessage struct {
	ReplyMarkup any                 `json:"reply_markup,omitempty"`
	From        *TelegramUser       `json:"from,omitempty"`
	Voice       *TelegramVoice      `json:"voice,omitempty"`
	Audio       *TelegramAudio      `json:"audio,omitempty"`
	Chat        TelegramChat        `json:"chat"`
	Text        string              `json:"text,omitempty"`
	Caption     string              `json:"caption,omitempty"`
	Photo       []TelegramPhotoSize `json:"photo,omitempty"`
	Date        int64               `json:"date"`
	MessageID   int                 `json:"message_id"`
}

// TelegramCallbackQuery represents an incoming callback query from an inline keyboard.
type TelegramCallbackQuery struct {
	Message      *TelegramMessage `json:"message,omitempty"`
	ID           string           `json:"id"`
	ChatInstance string           `json:"chat_instance,omitempty"`
	Data         string           `json:"data,omitempty"`
	From         TelegramUser     `json:"from"`
}

// TelegramUpdate represents an incoming update.
type TelegramUpdate struct {
	Message       *TelegramMessage       `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
	UpdateID      int                    `json:"update_id"`
}

// TelegramResponse represents the standard Bot API JSON envelope.
type TelegramResponse[T any] struct {
	Result      T      `json:"result,omitempty"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Ok          bool   `json:"ok"`
}

// TelegramWebhookInfo describes the current status of a webhook.
type TelegramWebhookInfo struct {
	URL                  string   `json:"url"`
	AllowedUpdates       []string `json:"allowed_updates,omitempty"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	MaxConnections       int      `json:"max_connections,omitempty"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
}

// SendMessageRequest models POST /bot<token>/sendMessage
type SendMessageRequest struct {
	ChatID      any             `json:"chat_id"`
	Text        string          `json:"text"`
	ReplyMarkup json.RawMessage `json:"reply_markup,omitempty"`
}

// SendVoiceRequest models POST /bot<token>/sendVoice
type SendVoiceRequest struct {
	ChatID   any    `json:"chat_id"`
	Voice    string `json:"voice"`
	Caption  string `json:"caption,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

// SendAudioRequest models POST /bot<token>/sendAudio
type SendAudioRequest struct {
	ChatID    any    `json:"chat_id"`
	Audio     string `json:"audio"`
	Caption   string `json:"caption,omitempty"`
	Performer string `json:"performer,omitempty"`
	Title     string `json:"title,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

// SendPhotoRequest models POST /bot<token>/sendPhoto
type SendPhotoRequest struct {
	ChatID  any    `json:"chat_id"`
	Photo   string `json:"photo"`
	Caption string `json:"caption,omitempty"`
}

// SendChatActionRequest models POST /bot<token>/sendChatAction
type SendChatActionRequest struct {
	ChatID               any    `json:"chat_id"`
	Action               string `json:"action"`
	BusinessConnectionID string `json:"business_connection_id,omitempty"`
	MessageThreadID      int    `json:"message_thread_id,omitempty"`
}

// AnswerCallbackQueryRequest models POST /bot<token>/answerCallbackQuery
type AnswerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

// SetWebhookRequest models POST /bot<token>/setWebhook
type SetWebhookRequest struct {
	URL            string   `json:"url"`
	SecretToken    string   `json:"secret_token,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
	MaxConnections int      `json:"max_connections,omitempty"`
}

// SetMyCommandsRequest models POST /bot<token>/setMyCommands
type SetMyCommandsRequest struct {
	Commands []TelegramBotCommand `json:"commands"`
}

// SetChatMenuButtonRequest models POST /bot<token>/setChatMenuButton
type SetChatMenuButtonRequest struct {
	ChatID     any                 `json:"chat_id,omitempty"`
	MenuButton *TelegramMenuButton `json:"menu_button,omitempty"`
}

// InboundTelegramParams defines parameters for simulating an inbound Telegram event.
type InboundTelegramParams struct {
	CallbackQueryID string
	Username        string
	FirstName       string
	Text            string
	Type            string
	MediaID         string
	CallbackData    string
	Waveform        []int
	FromID          int64
	ChatID          int64
	DurationSeconds float64
	MessageID       int
	Voice           bool
}

// TelegramService implements Telegram Bot API emulation.
type TelegramService struct {
	store        repository.Store
	broker       *EventBroker
	dispatcher   *TelegramDispatcher
	menuButton   *TelegramMenuButton
	cfg          TelegramConfig
	commands     []TelegramBotCommand
	botID        int64
	nextMsgID    atomic.Int64
	nextUpdateID atomic.Int64
	mu           sync.RWMutex
}

// NewTelegramService constructs a new TelegramService.
func NewTelegramService(store repository.Store, broker *EventBroker, cfg TelegramConfig) *TelegramService {
	dispatcher := NewTelegramDispatcher(cfg.UpstreamURL, cfg.SecretToken)
	botID := int64(123456789)
	if parts := strings.Split(cfg.BotToken, ":"); len(parts) > 0 {
		if id, err := strconv.ParseInt(parts[0], 10, 64); err == nil && id > 0 {
			botID = id
		}
	}

	svc := &TelegramService{
		store:      store,
		broker:     broker,
		dispatcher: dispatcher,
		cfg:        cfg,
		botID:      botID,
		commands:   make([]TelegramBotCommand, 0),
	}
	svc.nextMsgID.Store(400)
	svc.nextUpdateID.Store(800000)
	return svc
}

func parseChatIDHelper(raw any) int64 {
	switch v := raw.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		clean := strings.TrimPrefix(v, "+")
		if n, err := strconv.ParseInt(clean, 10, 64); err == nil {
			return n
		}
	}
	return 14155551111
}

// SendMessage handles outbound text and interactive keyboard messages.
func (s *TelegramService) SendMessage(ctx context.Context, req SendMessageRequest) (*TelegramMessage, error) {
	if strings.TrimSpace(req.Text) == "" {
		return nil, errors.New("bad request: message text is empty")
	}

	chatID := parseChatIDHelper(req.ChatID)
	msgID := int(s.nextMsgID.Add(1))
	now := time.Now().UTC()

	meta, interactive, markupAny := parseReplyMarkup(req.ReplyMarkup, chatID, msgID)

	msg := &domain.Message{
		SID:                fmt.Sprintf("tg_msg_%d", msgID),
		AccountSID:         "telegram",
		From:               s.cfg.BotUsername,
		To:                 strconv.FormatInt(chatID, 10),
		Body:               req.Text,
		Status:             domain.StatusSent,
		Direction:          domain.DirectionOutboundAPI,
		Engine:             domain.EngineTelegram,
		TelegramMetadata:   meta,
		InteractiveContent: interactive,
		DateCreated:        now,
		DateUpdated:        now,
		DateSent:           &now,
	}

	if err := s.store.CreateMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("storing telegram message: %w", err)
	}

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageOutbound,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}

	return &TelegramMessage{
		MessageID:   msgID,
		From:        &TelegramUser{ID: s.botID, IsBot: true, FirstName: s.cfg.BotName, Username: s.cfg.BotUsername},
		Chat:        TelegramChat{ID: chatID, Type: "private", FirstName: "Handset User"},
		Date:        now.Unix(),
		Text:        req.Text,
		ReplyMarkup: markupAny,
	}, nil
}

// SendVoice stores and broadcasts an outbound voice note.
func (s *TelegramService) SendVoice(ctx context.Context, req SendVoiceRequest) (*TelegramMessage, error) {
	chatID := parseChatIDHelper(req.ChatID)
	msgID := int(s.nextMsgID.Add(1))
	now := time.Now().UTC()

	fileID := fmt.Sprintf("tg_file_voice_%d", msgID)
	if strings.HasPrefix(req.Voice, "tg_file_") {
		fileID = req.Voice
	}
	body := req.Caption
	if body == "" {
		body = "[Voice Note]"
	}

	meta := &domain.TelegramMetadata{
		ChatID:    chatID,
		MessageID: msgID,
	}

	msg := &domain.Message{
		SID:              fmt.Sprintf("tg_msg_%d", msgID),
		AccountSID:       "telegram",
		From:             s.cfg.BotUsername,
		To:               strconv.FormatInt(chatID, 10),
		Body:             body,
		Status:           domain.StatusSent,
		Direction:        domain.DirectionOutboundAPI,
		Engine:           domain.EngineTelegram,
		TelegramMetadata: meta,
		VoiceMetadata: &domain.VoiceMetadata{
			DurationSeconds: float64(req.Duration),
			IsVoiceNote:     true,
		},
		MediaURLs:   []string{req.Voice},
		DateCreated: now,
		DateUpdated: now,
		DateSent:    &now,
	}

	if err := s.store.CreateMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("storing voice message: %w", err)
	}

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageOutbound,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}

	return &TelegramMessage{
		MessageID: msgID,
		From:      &TelegramUser{ID: s.botID, IsBot: true, FirstName: s.cfg.BotName, Username: s.cfg.BotUsername},
		Chat:      TelegramChat{ID: chatID, Type: "private", FirstName: "Handset User"},
		Date:      now.Unix(),
		Caption:   req.Caption,
		Voice: &TelegramVoice{
			FileID:       fileID,
			FileUniqueID: fmt.Sprintf("uniq_v%d", msgID),
			Duration:     req.Duration,
			MimeType:     "audio/ogg",
			FileSize:     15420,
		},
	}, nil
}

// SendAudio stores and broadcasts an outbound audio track.
func (s *TelegramService) SendAudio(ctx context.Context, req SendAudioRequest) (*TelegramMessage, error) {
	chatID := parseChatIDHelper(req.ChatID)
	msgID := int(s.nextMsgID.Add(1))
	now := time.Now().UTC()
	fileID := fmt.Sprintf("tg_file_audio_%d", msgID)
	if strings.HasPrefix(req.Audio, "tg_file_") {
		fileID = req.Audio
	}

	msg := &domain.Message{
		SID:        fmt.Sprintf("tg_msg_%d", msgID),
		AccountSID: "telegram",
		From:       s.cfg.BotUsername,
		To:         strconv.FormatInt(chatID, 10),
		Body:       "[Audio Attachment]",
		Status:     domain.StatusSent,
		Direction:  domain.DirectionOutboundAPI,
		Engine:     domain.EngineTelegram,
		MediaURLs:  []string{req.Audio},
		VoiceMetadata: &domain.VoiceMetadata{
			DurationSeconds: float64(req.Duration),
			IsVoiceNote:     false,
		},
		TelegramMetadata: &domain.TelegramMetadata{ChatID: chatID, MessageID: msgID},
		DateCreated:      now,
		DateUpdated:      now,
		DateSent:         &now,
	}
	_ = s.store.CreateMessage(ctx, msg)

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageOutbound,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}

	return &TelegramMessage{
		MessageID: msgID,
		From:      &TelegramUser{ID: s.botID, IsBot: true, FirstName: s.cfg.BotName, Username: s.cfg.BotUsername},
		Chat:      TelegramChat{ID: chatID, Type: "private", FirstName: "Handset User"},
		Date:      now.Unix(),
		Caption:   req.Caption,
		Audio: &TelegramAudio{
			FileID:       fileID,
			FileUniqueID: fmt.Sprintf("uniq_a%d", msgID),
			Duration:     req.Duration,
			Performer:    req.Performer,
			Title:        req.Title,
			MimeType:     "audio/mpeg",
			FileSize:     32000,
		},
	}, nil
}

// SendPhoto stores and broadcasts an outbound photo message.
func (s *TelegramService) SendPhoto(ctx context.Context, req SendPhotoRequest) (*TelegramMessage, error) {
	chatID := parseChatIDHelper(req.ChatID)
	msgID := int(s.nextMsgID.Add(1))
	now := time.Now().UTC()
	fileID := fmt.Sprintf("tg_file_photo_%d", msgID)
	if strings.HasPrefix(req.Photo, "tg_file_") {
		fileID = req.Photo
	}

	msg := &domain.Message{
		SID:              fmt.Sprintf("tg_msg_%d", msgID),
		AccountSID:       "telegram",
		From:             s.cfg.BotUsername,
		To:               strconv.FormatInt(chatID, 10),
		Body:             "[Photo Attachment]",
		Status:           domain.StatusSent,
		Direction:        domain.DirectionOutboundAPI,
		Engine:           domain.EngineTelegram,
		MediaURLs:        []string{req.Photo},
		TelegramMetadata: &domain.TelegramMetadata{ChatID: chatID, MessageID: msgID},
		DateCreated:      now,
		DateUpdated:      now,
		DateSent:         &now,
	}
	_ = s.store.CreateMessage(ctx, msg)

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageOutbound,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}

	return &TelegramMessage{
		MessageID: msgID,
		From:      &TelegramUser{ID: s.botID, IsBot: true, FirstName: s.cfg.BotName, Username: s.cfg.BotUsername},
		Chat:      TelegramChat{ID: chatID, Type: "private", FirstName: "Handset User"},
		Date:      now.Unix(),
		Caption:   req.Caption,
		Photo: []TelegramPhotoSize{
			{
				FileID:       fileID,
				FileUniqueID: fmt.Sprintf("uniq_p%d", msgID),
				Width:        800,
				Height:       600,
				FileSize:     45000,
			},
		},
	}, nil
}

// SendChatAction handles POST /bot<token>/sendChatAction to broadcast chat status.
func (s *TelegramService) SendChatAction(_ context.Context, req SendChatActionRequest) (bool, error) {
	if req.ChatID == nil || req.ChatID == "" {
		return false, errors.New("bad request: chat_id is empty")
	}
	if strings.TrimSpace(req.Action) == "" {
		return false, errors.New("bad request: chat action is empty")
	}

	chatID := parseChatIDHelper(req.ChatID)
	if s.broker != nil {
		now := time.Now().UTC()
		msg := &domain.Message{
			SID:        fmt.Sprintf("tg_act_%d_%d", chatID, now.UnixNano()),
			AccountSID: "telegram",
			Body:       req.Action,
			Status:     domain.StatusDelivered,
			Engine:     domain.EngineTelegram,
			TelegramMetadata: &domain.TelegramMetadata{
				ChatID: chatID,
			},
			DateCreated: now,
			DateUpdated: now,
		}
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageStatus,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}
	return true, nil
}

// AnswerCallbackQuery acknowledges a button click and clears loading states.
func (s *TelegramService) AnswerCallbackQuery(_ context.Context, req AnswerCallbackQueryRequest) (bool, error) {
	if s.broker != nil {
		now := time.Now().UTC()
		msg := &domain.Message{
			SID:        req.CallbackQueryID,
			AccountSID: "telegram",
			Body:       req.Text,
			Status:     domain.StatusDelivered,
			Engine:     domain.EngineTelegram,
			TelegramMetadata: &domain.TelegramMetadata{
				CallbackQueryID: req.CallbackQueryID,
			},
			DateCreated: now,
			DateUpdated: now,
		}
		s.broker.Publish(Event{
			ID:        req.CallbackQueryID,
			Type:      EventMessageStatus,
			Engine:    domain.EngineTelegram,
			Payload:   msg,
			Timestamp: now,
		})
	}
	return true, nil
}

// GetFile retrieves metadata and download URL for an uploaded file ID.
func (s *TelegramService) GetFile(ctx context.Context, fileID string) (*TelegramFile, error) {
	media, err := s.store.GetMediaBySID(ctx, "telegram", "telegram", fileID)
	if err == nil && media != nil {
		ext := filepath.Ext(media.Filename)
		if ext == "" {
			ext = ".oga"
		}
		return &TelegramFile{
			FileID:       fileID,
			FileUniqueID: "uniq_" + fileID,
			FilePath:     "voice/" + fileID + ext,
			FileSize:     int64(len(media.Data)),
		}, nil
	}

	return &TelegramFile{
		FileID:       fileID,
		FileUniqueID: "uniq_" + fileID,
		FilePath:     "voice/" + fileID + ".oga",
		FileSize:     15420,
	}, nil
}

// UploadMedia stores a binary asset and returns its file_id.
func (s *TelegramService) UploadMedia(ctx context.Context, contentType, filename string, data []byte) (string, error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	fileID := "tg_file_" + hex.EncodeToString(b)
	now := time.Now().UTC()

	media := &domain.Media{
		SID:         fileID,
		AccountSID:  "telegram",
		ParentSID:   "telegram",
		ContentType: contentType,
		Filename:    filename,
		Data:        data,
		DateCreated: now,
		DateUpdated: now,
	}
	if err := s.store.CreateMedia(ctx, media); err != nil {
		return "", err
	}
	return fileID, nil
}

// GetMedia retrieves binary file content by file ID.
func (s *TelegramService) GetMedia(ctx context.Context, fileID string) (*domain.Media, error) {
	return s.store.GetMediaBySID(ctx, "telegram", "telegram", fileID)
}

func parseReplyMarkup(raw []byte, chatID int64, msgID int) (*domain.TelegramMetadata, *domain.InteractiveContent, any) {
	if len(raw) == 0 {
		return &domain.TelegramMetadata{ChatID: chatID, MessageID: msgID}, nil, nil
	}

	var inlineMarkup TelegramInlineKeyboardMarkup
	if err := json.Unmarshal(raw, &inlineMarkup); err == nil && len(inlineMarkup.InlineKeyboard) > 0 {
		meta := &domain.TelegramMetadata{
			ChatID:    chatID,
			MessageID: msgID,
		}
		var domainButtons []domain.ButtonItem
		for _, row := range inlineMarkup.InlineKeyboard {
			var metaRow []domain.ButtonItem
			for _, btn := range row {
				bi := domain.ButtonItem{ID: btn.CallbackData, Title: btn.Text}
				if btn.URL != "" {
					bi.ID = btn.URL
				}
				metaRow = append(metaRow, bi)
				domainButtons = append(domainButtons, bi)
			}
			meta.InlineKeyboard = append(meta.InlineKeyboard, metaRow)
		}
		ic := &domain.InteractiveContent{
			Type:    domain.InteractiveTypeTelegramInline,
			Buttons: domainButtons,
		}
		return meta, ic, inlineMarkup
	}

	var replyMarkup TelegramReplyKeyboardMarkup
	if err := json.Unmarshal(raw, &replyMarkup); err == nil && len(replyMarkup.Keyboard) > 0 {
		meta := &domain.TelegramMetadata{
			ChatID:    chatID,
			MessageID: msgID,
		}
		var domainButtons []domain.ButtonItem
		for _, row := range replyMarkup.Keyboard {
			var metaRow []domain.ButtonItem
			for _, btn := range row {
				bi := domain.ButtonItem{ID: btn.Text, Title: btn.Text}
				metaRow = append(metaRow, bi)
				domainButtons = append(domainButtons, bi)
			}
			meta.ReplyKeyboard = append(meta.ReplyKeyboard, metaRow)
		}
		ic := &domain.InteractiveContent{
			Type:    domain.InteractiveTypeTelegramReply,
			Buttons: domainButtons,
		}
		return meta, ic, replyMarkup
	}

	var removeMarkup TelegramReplyKeyboardRemove
	if err := json.Unmarshal(raw, &removeMarkup); err == nil && removeMarkup.RemoveKeyboard {
		return &domain.TelegramMetadata{ChatID: chatID, MessageID: msgID, RemoveKeyboard: true}, nil, removeMarkup
	}

	var forceReply TelegramForceReply
	if err := json.Unmarshal(raw, &forceReply); err == nil && forceReply.ForceReply {
		return &domain.TelegramMetadata{ChatID: chatID, MessageID: msgID, ForceReply: true}, nil, forceReply
	}

	return &domain.TelegramMetadata{ChatID: chatID, MessageID: msgID}, nil, nil
}
