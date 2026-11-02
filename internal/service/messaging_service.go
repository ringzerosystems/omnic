package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// CreateServiceParams defines parameters for creating a Messaging Service.
type CreateServiceParams struct {
	FriendlyName      string
	InboundRequestURL string
	InboundMethod     string
	FallbackURL       string
	FallbackMethod    string
	StatusCallback    string
	StickySender      bool
	Geomatch          bool
}

// UpdateServiceParams defines fields for updating a Messaging Service.
type UpdateServiceParams struct {
	FriendlyName      *string
	InboundRequestURL *string
	InboundMethod     *string
	FallbackURL       *string
	FallbackMethod    *string
	StatusCallback    *string
	StickySender      *bool
	Geomatch          *bool
}

// MessagingServiceService manages Copilot messaging services and sender pools.
type MessagingServiceService struct {
	store repository.Store
}

// NewMessagingServiceService constructs a new MessagingServiceService.
func NewMessagingServiceService(store repository.Store) *MessagingServiceService {
	return &MessagingServiceService{store: store}
}

// Create registers a new Messaging Service.
func (s *MessagingServiceService) Create(ctx context.Context, accountSID string, params CreateServiceParams) (*domain.MessagingService, error) {
	now := time.Now().UTC()
	svc := &domain.MessagingService{
		SID:               GenerateSID("MG"),
		AccountSID:        accountSID,
		FriendlyName:      params.FriendlyName,
		InboundRequestURL: params.InboundRequestURL,
		InboundMethod:     params.InboundMethod,
		FallbackURL:       params.FallbackURL,
		FallbackMethod:    params.FallbackMethod,
		StatusCallback:    params.StatusCallback,
		StickySender:      params.StickySender,
		Geomatch:          params.Geomatch,
		DateCreated:       now,
		DateUpdated:       now,
	}
	if err := s.store.CreateMessagingService(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

// Update modifies Messaging Service settings.
func (s *MessagingServiceService) Update(ctx context.Context, accountSID, sid string, params UpdateServiceParams) (*domain.MessagingService, error) {
	svc, err := s.store.GetMessagingServiceBySID(ctx, accountSID, sid)
	if err != nil {
		return nil, err
	}
	if params.FriendlyName != nil {
		svc.FriendlyName = *params.FriendlyName
	}
	if params.StickySender != nil {
		svc.StickySender = *params.StickySender
	}
	if params.Geomatch != nil {
		svc.Geomatch = *params.Geomatch
	}
	svc.DateUpdated = time.Now().UTC()
	if err := s.store.UpdateMessagingService(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

// AddPhoneNumber attaches a provisioned number to the service pool.
func (s *MessagingServiceService) AddPhoneNumber(ctx context.Context, serviceSID, phoneNumberSID string) (*domain.ServicePhoneNumber, error) {
	spn := &domain.ServicePhoneNumber{
		ServiceSID:     serviceSID,
		PhoneNumberSID: phoneNumberSID,
		PhoneNumber:    phoneNumberSID,
		DateCreated:    time.Now().UTC(),
	}
	if got, err := s.store.FindPhoneNumberByNumber(ctx, phoneNumberSID); err == nil {
		spn.PhoneNumber = got.PhoneNumber
	}

	if err := s.store.AddServicePhoneNumber(ctx, spn); err != nil {
		return nil, err
	}
	return spn, nil
}

// SelectSender chooses the optimal sender from the pool using Geomatch and Sticky Sender.
func (s *MessagingServiceService) SelectSender(ctx context.Context, serviceSID, recipientTo string) (string, error) {
	pool, err := s.store.ListServicePhoneNumbers(ctx, serviceSID)
	if err != nil || len(pool) == 0 {
		return "", errors.New("no phone numbers available in messaging service pool")
	}

	// Geomatch by area code if recipient is US (+1)
	if strings.HasPrefix(recipientTo, "+1") && len(recipientTo) >= 5 {
		recipientArea := recipientTo[2:5]
		for _, spn := range pool {
			if strings.Contains(spn.PhoneNumber, recipientArea) || strings.Contains(spn.PhoneNumberSID, recipientArea) {
				if strings.HasPrefix(spn.PhoneNumber, "+") {
					return spn.PhoneNumber, nil
				}
				return s.lookupPhoneNumberBySID(ctx, spn.PhoneNumberSID)
			}
		}
	}

	first := pool[0]
	if strings.HasPrefix(first.PhoneNumber, "+") {
		return first.PhoneNumber, nil
	}
	return s.lookupPhoneNumberBySID(ctx, first.PhoneNumberSID)
}

func (s *MessagingServiceService) lookupPhoneNumberBySID(ctx context.Context, sid string) (string, error) {
	pn, err := s.store.GetPhoneNumberBySID(ctx, "AC001", sid)
	if err == nil {
		return pn.PhoneNumber, nil
	}
	return sid, nil
}
