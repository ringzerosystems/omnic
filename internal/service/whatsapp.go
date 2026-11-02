package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// WhatsAppConfig holds service credentials and routing metadata.
type WhatsAppConfig struct {
	PhoneNumberID string
	WABAID        string
	DisplayNumber string
	VerifyToken   string
	AppSecret     string
	BearerToken   string
	UpstreamURL   string
	DeliveryDelay time.Duration
}

// OutboundMessageRequest models Meta Cloud API POST /v21.0/{phone_number_id}/messages.
type OutboundMessageRequest struct {
	Text             *OutboundText        `json:"text,omitempty"`
	Audio            *OutboundAudio       `json:"audio,omitempty"`
	Interactive      *OutboundInteractive `json:"interactive,omitempty"`
	MessagingProduct string               `json:"messaging_product"`
	RecipientType    string               `json:"recipient_type"`
	To               string               `json:"to"`
	Type             string               `json:"type"`
	Status           string               `json:"status,omitempty"`
	MessageID        string               `json:"message_id,omitempty"`
}

// OutboundText represents a text body.
type OutboundText struct {
	Body string `json:"body"`
}

// OutboundAudio represents audio or voice note attachments.
type OutboundAudio struct {
	ID    string `json:"id,omitempty"`
	Link  string `json:"link,omitempty"`
	Voice bool   `json:"voice,omitempty"`
}

// OutboundInteractive models WhatsApp buttons or list menus.
type OutboundInteractive struct {
	Header *InteractiveHeader `json:"header,omitempty"`
	Footer *InteractiveFooter `json:"footer,omitempty"`
	Type   string             `json:"type"`
	Body   InteractiveBody    `json:"body"`
	Action InteractiveAction  `json:"action"`
}

// InteractiveHeader models an optional header.
type InteractiveHeader struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// InteractiveBody models the main text body.
type InteractiveBody struct {
	Text string `json:"text"`
}

// InteractiveFooter models the footer text.
type InteractiveFooter struct {
	Text string `json:"text"`
}

// InteractiveAction holds buttons or section rows.
type InteractiveAction struct {
	Button   string               `json:"button,omitempty"`
	Buttons  []ButtonWrapper      `json:"buttons,omitempty"`
	Sections []domain.MenuSection `json:"sections,omitempty"`
}

// ButtonWrapper wraps a quick reply button.
type ButtonWrapper struct {
	Type  string            `json:"type"`
	Reply domain.ButtonItem `json:"reply"`
}

// ContactResponse models a Meta Graph API contact entry.
type ContactResponse struct {
	Input string `json:"input"`
	WAID  string `json:"wa_id"`
}

// WAMIDResponse models a message ID response.
type WAMIDResponse struct {
	ID string `json:"id"`
}

// OutboundMessageResponse models the 200 OK Meta Cloud API response.
type OutboundMessageResponse struct {
	MessagingProduct string            `json:"messaging_product"`
	Contacts         []ContactResponse `json:"contacts"`
	Messages         []WAMIDResponse   `json:"messages"`
}

// InboundWhatsAppParams carries parameters for handset or emu inbound messages.
type InboundWhatsAppParams struct {
	From            string
	To              string
	Type            string
	Body            string
	MediaID         string
	MediaURL        string
	InteractiveType domain.InteractiveType
	SelectedID      string
	SelectedTitle   string
	SelectedDesc    string
	Waveform        []int
	DurationSeconds float64
	Voice           bool
}

// WhatsAppService manages Meta WhatsApp Cloud API emulation.
type WhatsAppService struct {
	store      repository.Store
	broker     *EventBroker
	dispatcher *WhatsAppDispatcher
	cfg        WhatsAppConfig
	mu         sync.RWMutex
}

// NewWhatsAppService constructs a new WhatsAppService.
func NewWhatsAppService(store repository.Store, broker *EventBroker, cfg WhatsAppConfig) *WhatsAppService {
	if cfg.DeliveryDelay <= 0 {
		cfg.DeliveryDelay = 20 * time.Millisecond
	}
	if cfg.DisplayNumber == "" {
		cfg.DisplayNumber = "+14155550199"
	}
	disp := NewWhatsAppDispatcher(cfg.UpstreamURL, cfg.AppSecret)
	return &WhatsAppService{
		store:      store,
		broker:     broker,
		dispatcher: disp,
		cfg:        cfg,
	}
}

func generateWAMID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "wamid.HBgM" + hex.EncodeToString(b)
}

func validateInteractive(interactive *OutboundInteractive) error {
	switch interactive.Type {
	case "button":
		if len(interactive.Action.Buttons) > 3 {
			return errors.New("maximum 3 buttons allowed")
		}
		for _, b := range interactive.Action.Buttons {
			if len(b.Reply.Title) > 20 {
				return errors.New("button title cannot exceed 20 characters")
			}
			if len(b.Reply.ID) > 256 {
				return errors.New("button id cannot exceed 256 characters")
			}
		}
	case "list":
		totalRows := 0
		for _, sec := range interactive.Action.Sections {
			if len(sec.Title) > 24 {
				return errors.New("section title cannot exceed 24 characters")
			}
			totalRows += len(sec.Rows)
			for _, r := range sec.Rows {
				if len(r.Title) > 24 {
					return errors.New("row title cannot exceed 24 characters")
				}
				if len(r.Description) > 72 {
					return errors.New("row description cannot exceed 72 characters")
				}
			}
		}
		if totalRows > 10 {
			return errors.New("maximum 10 rows allowed in list message")
		}
		if len(interactive.Action.Button) > 20 {
			return errors.New("action button label cannot exceed 20 characters")
		}
	}
	return nil
}

func validateOutboundRequest(req *OutboundMessageRequest) error {
	if req.MessagingProduct != "whatsapp" {
		return errors.New("invalid messaging_product, expected whatsapp")
	}
	if req.To == "" {
		return errors.New("recipient 'to' is required")
	}
	if req.Type == "interactive" && req.Interactive != nil {
		return validateInteractive(req.Interactive)
	}
	return nil
}

func (s *WhatsAppService) buildDomainMessage(phoneID string, req *OutboundMessageRequest, wamid string) *domain.Message {
	now := time.Now().UTC()
	msg := &domain.Message{
		SID:         wamid,
		AccountSID:  s.cfg.WABAID,
		From:        phoneID,
		To:          req.To,
		Status:      domain.StatusQueued,
		Direction:   domain.DirectionOutboundAPI,
		Engine:      domain.EngineWhatsApp,
		DateCreated: now,
		DateUpdated: now,
	}

	switch {
	case req.Type == "text" && req.Text != nil:
		msg.Body = req.Text.Body
	case req.Type == "audio" && req.Audio != nil:
		msg.Body = "[Voice Note]"
		if !req.Audio.Voice {
			msg.Body = "[Audio Attachment]"
		}
		msg.VoiceMetadata = &domain.VoiceMetadata{IsVoiceNote: req.Audio.Voice}
		if req.Audio.ID != "" {
			msg.MediaURLs = []string{req.Audio.ID}
		} else if req.Audio.Link != "" {
			msg.MediaURLs = []string{req.Audio.Link}
		}
	case req.Type == "interactive" && req.Interactive != nil:
		msg.Body = req.Interactive.Body.Text
		ic := &domain.InteractiveContent{
			Type:        domain.InteractiveType(req.Interactive.Type),
			Body:        req.Interactive.Body.Text,
			ActionLabel: req.Interactive.Action.Button,
			Sections:    req.Interactive.Action.Sections,
		}
		if req.Interactive.Footer != nil {
			ic.Footer = req.Interactive.Footer.Text
		}
		if req.Interactive.Header != nil {
			ic.Header = req.Interactive.Header.Text
		}
		for _, b := range req.Interactive.Action.Buttons {
			ic.Buttons = append(ic.Buttons, b.Reply)
		}
		msg.InteractiveContent = ic
	}
	return msg
}

func (s *WhatsAppService) simulateDeliveryLifecycle(wamid, to string) {
	go func() {
		delay := s.cfg.DeliveryDelay
		time.Sleep(delay)

		ctx := context.Background()
		msg, err := s.store.GetMessageByEngineID(ctx, domain.EngineWhatsApp, wamid)
		if err != nil {
			return
		}
		msg.Status = domain.StatusSent
		now := time.Now().UTC()
		msg.DateSent = &now
		msg.DateUpdated = now
		_ = s.store.UpdateMessage(ctx, msg)
		if s.broker != nil {
			s.broker.Publish(Event{
				ID:        wamid,
				Type:      EventMessageStatus,
				Engine:    domain.EngineWhatsApp,
				Payload:   msg,
				Timestamp: now,
			})
		}
		if s.dispatcher != nil {
			_ = s.dispatcher.DispatchStatus(ctx, WhatsAppStatusParams{
				WABAID:        s.cfg.WABAID,
				PhoneID:       s.cfg.PhoneNumberID,
				DisplayNumber: s.cfg.DisplayNumber,
				WAMID:         wamid,
				Status:        "sent",
				RecipientID:   to,
			})
		}

		time.Sleep(delay)
		msg.Status = domain.StatusDelivered
		msg.DateUpdated = time.Now().UTC()
		_ = s.store.UpdateMessage(ctx, msg)
		if s.broker != nil {
			s.broker.Publish(Event{
				ID:        wamid,
				Type:      EventMessageStatus,
				Engine:    domain.EngineWhatsApp,
				Payload:   msg,
				Timestamp: msg.DateUpdated,
			})
		}
		if s.dispatcher != nil {
			_ = s.dispatcher.DispatchStatus(ctx, WhatsAppStatusParams{
				WABAID:        s.cfg.WABAID,
				PhoneID:       s.cfg.PhoneNumberID,
				DisplayNumber: s.cfg.DisplayNumber,
				WAMID:         wamid,
				Status:        "delivered",
				RecipientID:   to,
			})
		}
	}()
}

// SendOutbound validates, stores, and dispatches an outbound WhatsApp message.
func (s *WhatsAppService) SendOutbound(ctx context.Context, phoneID string, req OutboundMessageRequest) (*OutboundMessageResponse, error) {
	if err := validateOutboundRequest(&req); err != nil {
		return nil, err
	}
	wamid := generateWAMID()
	msg := s.buildDomainMessage(phoneID, &req, wamid)
	if err := s.store.CreateMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("persisting message: %w", err)
	}

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        wamid,
			Type:      EventMessageOutbound,
			Engine:    domain.EngineWhatsApp,
			Payload:   msg,
			Timestamp: msg.DateCreated,
		})
	}

	s.simulateDeliveryLifecycle(wamid, req.To)

	return &OutboundMessageResponse{
		MessagingProduct: "whatsapp",
		Contacts: []ContactResponse{
			{Input: req.To, WAID: sanitizeWAID(req.To)},
		},
		Messages: []WAMIDResponse{
			{ID: wamid},
		},
	}, nil
}

// UploadMedia stores a binary asset and returns a media ID.
func (s *WhatsAppService) UploadMedia(ctx context.Context, phoneID, contentType, filename string, data []byte) (string, error) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	mediaID := "wa_media_" + hex.EncodeToString(b)
	now := time.Now().UTC()
	media := &domain.Media{
		SID:         mediaID,
		AccountSID:  s.cfg.WABAID,
		ParentSID:   phoneID,
		ContentType: contentType,
		Filename:    filename,
		Data:        data,
		DateCreated: now,
		DateUpdated: now,
	}
	if err := s.store.CreateMedia(ctx, media); err != nil {
		return "", err
	}
	return mediaID, nil
}

// GetMedia retrieves media by ID.
func (s *WhatsAppService) GetMedia(ctx context.Context, mediaID string) (*domain.Media, error) {
	return s.store.GetMediaBySID(ctx, s.cfg.WABAID, s.cfg.PhoneNumberID, mediaID)
}

// DispatchInbound simulates an inbound WhatsApp message from a handset or API.
func (s *WhatsAppService) DispatchInbound(ctx context.Context, params InboundWhatsAppParams) (*domain.Message, error) {
	wamid := generateWAMID()
	now := time.Now().UTC()
	msg := &domain.Message{
		SID:         wamid,
		AccountSID:  s.cfg.WABAID,
		From:        params.From,
		To:          params.To,
		Body:        params.Body,
		Status:      domain.StatusReceived,
		Direction:   domain.DirectionInbound,
		Engine:      domain.EngineWhatsApp,
		DateCreated: now,
		DateUpdated: now,
	}

	switch params.Type {
	case "interactive":
		msg.InteractiveContent = &domain.InteractiveContent{
			Type:       params.InteractiveType,
			Body:       params.SelectedTitle,
			SelectedID: params.SelectedID,
		}
	case "audio":
		msg.VoiceMetadata = &domain.VoiceMetadata{
			DurationSeconds: params.DurationSeconds,
			IsVoiceNote:     params.Voice,
			Waveform:        params.Waveform,
		}
		if params.MediaID != "" {
			msg.MediaURLs = []string{params.MediaID}
		} else if params.MediaURL != "" {
			msg.MediaURLs = []string{params.MediaURL}
		}
	}

	if err := s.store.CreateMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("storing inbound message: %w", err)
	}

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        wamid,
			Type:      EventMessageInbound,
			Engine:    domain.EngineWhatsApp,
			Payload:   msg,
			Timestamp: now,
		})
	}

	if s.dispatcher != nil {
		_ = s.dispatcher.DispatchInbound(ctx, s.cfg.WABAID, s.cfg.PhoneNumberID, s.cfg.DisplayNumber, msg)
	}

	return msg, nil
}

// MarkAsRead transitions a WhatsApp message to read status and dispatches status webhook.
func (s *WhatsAppService) MarkAsRead(ctx context.Context, wamid string) error {
	msg, err := s.store.GetMessageByEngineID(ctx, domain.EngineWhatsApp, wamid)
	if err != nil {
		return err
	}
	msg.Status = domain.StatusRead
	msg.DateUpdated = time.Now().UTC()
	if err := s.store.UpdateMessage(ctx, msg); err != nil {
		return err
	}
	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        wamid,
			Type:      EventMessageStatus,
			Engine:    domain.EngineWhatsApp,
			Payload:   msg,
			Timestamp: msg.DateUpdated,
		})
	}
	if s.dispatcher != nil {
		recipientID := msg.To
		if msg.Direction == domain.DirectionInbound && msg.From != "" {
			recipientID = msg.From
		}
		_ = s.dispatcher.DispatchStatus(ctx, WhatsAppStatusParams{
			WABAID:        s.cfg.WABAID,
			PhoneID:       s.cfg.PhoneNumberID,
			DisplayNumber: s.cfg.DisplayNumber,
			WAMID:         wamid,
			Status:        "read",
			RecipientID:   recipientID,
		})
	}
	return nil
}

// GetConfig returns the active WhatsAppConfig.
func (s *WhatsAppService) GetConfig() WhatsAppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// SetUpstreamURL updates the upstream webhook endpoint.
func (s *WhatsAppService) SetUpstreamURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.UpstreamURL = url
	if s.dispatcher != nil {
		s.dispatcher.SetUpstreamURL(url)
	}
}
