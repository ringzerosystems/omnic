package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// CreateMessageParams defines parameters for creating an outbound message.
type CreateMessageParams struct {
	SendAt              *time.Time
	Properties          map[string]any
	To                  string
	From                string
	MessagingServiceSID string
	Body                string
	ContentSID          string
	ScheduleType        string
	StatusCallback      string
	MediaURLs           []string
	ValidityPeriod      int
	SendAsMMS           bool
}

// UpdateMessageParams defines fields for updating or canceling a message.
type UpdateMessageParams struct {
	Body   *string
	Status *domain.MessageStatus
}

// MessageService handles message lifecycle and delivery validation.
type MessageService struct {
	store      repository.Store
	broker     *EventBroker
	simulation *SimulationService
}

// NewMessageService constructs a new MessageService.
func NewMessageService(store repository.Store) *MessageService {
	return &MessageService{store: store}
}

// SetBroker binds an EventBroker for real-time notification dispatching.
func (s *MessageService) SetBroker(broker *EventBroker) {
	s.broker = broker
}

// SetSimulation sets the chaos/fault injection simulation service.
func (s *MessageService) SetSimulation(sim *SimulationService) {
	s.simulation = sim
}

func (s *MessageService) validateMessageParams(ctx context.Context, accountSID string, params *CreateMessageParams) error {
	if params.To == "" {
		return ErrMissingTo()
	}
	if params.Body == "" && len(params.MediaURLs) == 0 && params.ContentSID == "" {
		return ErrMissingBody()
	}
	if len(params.Body) > 1600 {
		return ErrMaxBodyExceeded()
	}
	if params.From != "" && strings.HasPrefix(params.From, "+") {
		if _, err := s.store.GetPhoneNumberByNumber(ctx, accountSID, params.From); err != nil {
			return ErrInvalidFrom(params.From)
		}
	}
	if opted, _ := s.store.IsOptedOut(ctx, accountSID, params.To); opted {
		return ErrUnsubscribed()
	}
	return nil
}

// Create validates and enqueues a new SMS or MMS message.
func (s *MessageService) Create(ctx context.Context, accountSID string, params CreateMessageParams) (*domain.Message, error) {
	if err := s.validateMessageParams(ctx, accountSID, &params); err != nil {
		return nil, err
	}

	if s.simulation != nil {
		if simErr := s.simulation.Evaluate("send", params.To, params.From); simErr != nil {
			return nil, simErr
		}
	}

	isMMS := len(params.MediaURLs) > 0 || params.SendAsMMS
	prefix := "SM"
	if isMMS {
		prefix = "MM"
	}
	sid := GenerateSID(prefix)

	status := domain.StatusQueued
	if params.SendAt != nil {
		status = domain.StatusScheduled
	}

	numSegments, _ := domain.CalculateSegments(params.Body, isMMS)
	now := time.Now().UTC()
	priceUnit := "USD"

	msg := &domain.Message{
		SID:                 sid,
		AccountSID:          accountSID,
		MessagingServiceSID: params.MessagingServiceSID,
		To:                  params.To,
		From:                params.From,
		Body:                params.Body,
		Status:              status,
		NumSegments:         numSegments,
		NumMedia:            len(params.MediaURLs),
		Direction:           domain.DirectionOutboundAPI,
		PriceUnit:           priceUnit,
		DateCreated:         now,
		DateUpdated:         now,
		ValidityPeriod:      params.ValidityPeriod,
		ScheduleType:        params.ScheduleType,
		SendAt:              params.SendAt,
		MediaURLs:           params.MediaURLs,
		StatusCallback:      params.StatusCallback,
		Properties:          params.Properties,
	}

	if err := s.store.CreateMessage(ctx, msg); err != nil {
		return nil, err
	}

	for _, u := range params.MediaURLs {
		_ = s.store.CreateMedia(ctx, &domain.Media{
			SID:         GenerateSID("ME"),
			AccountSID:  accountSID,
			ParentSID:   sid,
			ContentType: "application/octet-stream",
			URI:         u,
			DateCreated: now,
			DateUpdated: now,
		})
	}

	if s.broker != nil {
		s.broker.Publish(Event{
			ID:        msg.SID,
			Type:      EventMessageOutbound,
			Timestamp: msg.DateCreated,
			Payload:   msg,
		})
	}

	return msg, nil
}

// Update cancels a scheduled message or redacts message body.
func (s *MessageService) Update(ctx context.Context, accountSID, sid string, params UpdateMessageParams) (*domain.Message, error) {
	msg, err := s.store.GetMessageBySID(ctx, accountSID, sid)
	if err != nil {
		return nil, err
	}

	if params.Status != nil && *params.Status == domain.StatusCanceled {
		if msg.Status != domain.StatusScheduled {
			return nil, NewTwilioError(20004, http.StatusBadRequest, "Only scheduled messages can be canceled")
		}
		msg.Status = domain.StatusCanceled
	}
	if params.Body != nil && *params.Body == "" {
		msg.Body = ""
	}
	msg.DateUpdated = time.Now().UTC()

	if err := s.store.UpdateMessage(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
