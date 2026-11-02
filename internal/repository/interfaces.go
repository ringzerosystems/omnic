package repository

import (
	"context"

	"github.com/ringzerosystems/omnic/internal/domain"
)

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = domain.ErrNotFound

	// ErrConflict is returned when an entity violates uniqueness.
	ErrConflict = domain.ErrConflict
)

// MessageFilter defines criteria for listing messages.
type MessageFilter = domain.MessageFilter

// PhoneNumberFilter defines criteria for listing phone numbers.
type PhoneNumberFilter = domain.PhoneNumberFilter

// Store unifies persistence operations across all domain entities with cohesive domain verbs.
type Store interface {
	// Messages
	CreateMessage(ctx context.Context, msg *domain.Message) error
	GetMessageBySID(ctx context.Context, accountSID, sid string) (*domain.Message, error)
	GetMessageByEngineID(ctx context.Context, engine domain.Engine, engineID string) (*domain.Message, error)
	ListMessages(ctx context.Context, accountSID string, filter MessageFilter) ([]*domain.Message, int, error)
	UpdateMessage(ctx context.Context, msg *domain.Message) error
	DeleteMessage(ctx context.Context, accountSID, sid string) error

	// Phone numbers
	CreatePhoneNumber(ctx context.Context, pn *domain.PhoneNumber) error
	GetPhoneNumberBySID(ctx context.Context, accountSID, sid string) (*domain.PhoneNumber, error)
	GetPhoneNumberByNumber(ctx context.Context, accountSID, number string) (*domain.PhoneNumber, error)
	FindPhoneNumberByNumber(ctx context.Context, number string) (*domain.PhoneNumber, error)
	ListPhoneNumbers(ctx context.Context, accountSID string, filter PhoneNumberFilter) ([]*domain.PhoneNumber, int, error)
	UpdatePhoneNumber(ctx context.Context, pn *domain.PhoneNumber) error
	DeletePhoneNumber(ctx context.Context, accountSID, sid string) error

	// Media
	CreateMedia(ctx context.Context, media *domain.Media) error
	GetMediaBySID(ctx context.Context, accountSID, parentSID, sid string) (*domain.Media, error)
	ListMedia(ctx context.Context, accountSID, parentSID string) ([]*domain.Media, error)
	DeleteMedia(ctx context.Context, accountSID, parentSID, sid string) error

	// OptOut
	SetOptOut(ctx context.Context, accountSID, phoneNumber string, optedOut bool) error
	IsOptedOut(ctx context.Context, accountSID, phoneNumber string) (bool, error)

	// Messaging Services
	CreateMessagingService(ctx context.Context, svc *domain.MessagingService) error
	GetMessagingServiceBySID(ctx context.Context, accountSID, sid string) (*domain.MessagingService, error)
	ListMessagingServices(ctx context.Context, accountSID string, limit, offset int) ([]*domain.MessagingService, int, error)
	UpdateMessagingService(ctx context.Context, svc *domain.MessagingService) error
	DeleteMessagingService(ctx context.Context, accountSID, sid string) error

	// Service Pool
	AddServicePhoneNumber(ctx context.Context, spn *domain.ServicePhoneNumber) error
	ListServicePhoneNumbers(ctx context.Context, serviceSID string) ([]*domain.ServicePhoneNumber, error)
	RemoveServicePhoneNumber(ctx context.Context, serviceSID, phoneNumberSID string) error
}
