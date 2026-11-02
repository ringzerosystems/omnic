package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// AvailableFilter defines search criteria for available numbers.
type AvailableFilter struct {
	AreaCode     string
	Contains     string
	Limit        int
	SmsEnabled   bool
	MmsEnabled   bool
	VoiceEnabled bool
}

// AvailablePhoneNumber represents a searchable, unprovisioned phone number.
type AvailablePhoneNumber struct {
	FriendlyName string              `json:"friendly_name"`
	PhoneNumber  string              `json:"phone_number"`
	Lata         string              `json:"lata"`
	Locality     string              `json:"locality"`
	RateCenter   string              `json:"rate_center"`
	Latitude     string              `json:"latitude"`
	Longitude    string              `json:"longitude"`
	Region       string              `json:"region"`
	PostalCode   string              `json:"postal_code"`
	IsoCountry   string              `json:"iso_country"`
	AddressReq   string              `json:"address_requirements"`
	Capabilities domain.Capabilities `json:"capabilities"`
	Beta         bool                `json:"beta"`
}

// ProvisionParams contains parameters to provision an incoming number.
type ProvisionParams struct {
	PhoneNumber       string
	AreaCode          string
	FriendlyName      string
	SmsURL            string
	SmsMethod         string
	SmsFallbackURL    string
	SmsFallbackMethod string
	StatusCallback    string
}

// UpdatePhoneParams contains optional update fields.
type UpdatePhoneParams struct {
	FriendlyName      *string
	SmsURL            *string
	SmsMethod         *string
	SmsFallbackURL    *string
	SmsFallbackMethod *string
	StatusCallback    *string
}

// PhoneNumberService manages available and incoming phone numbers.
type PhoneNumberService struct {
	store repository.Store
}

// NewPhoneNumberService creates a new PhoneNumberService.
func NewPhoneNumberService(store repository.Store) *PhoneNumberService {
	return &PhoneNumberService{store: store}
}

// SearchAvailable generates matching candidate phone numbers.
func (s *PhoneNumberService) SearchAvailable(ctx context.Context, countryCode, numberType string, filter AvailableFilter) ([]AvailablePhoneNumber, error) {
	area := filter.AreaCode
	if area == "" {
		if strings.EqualFold(numberType, "TollFree") {
			area = "800"
		} else {
			area = "415"
		}
	}
	limit := filter.Limit
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	results := make([]AvailablePhoneNumber, 0, limit)
	for i := 10; i < 100 && len(results) < limit; i++ {
		num := fmt.Sprintf("+1%s55501%02d", area, i)
		if filter.Contains != "" && !strings.Contains(num, strings.Trim(filter.Contains, "*")) {
			continue
		}
		// Check if already provisioned
		if _, err := s.store.FindPhoneNumberByNumber(ctx, num); err == nil {
			continue // already taken
		}

		results = append(results, AvailablePhoneNumber{
			FriendlyName: fmt.Sprintf("(%s) 555-01%02d", area, i),
			PhoneNumber:  num,
			Lata:         "722",
			Locality:     "San Francisco",
			RateCenter:   "SNFRNSCCOCA",
			Latitude:     "37.7749",
			Longitude:    "-122.4194",
			Region:       "CA",
			PostalCode:   "94107",
			IsoCountry:   countryCode,
			AddressReq:   "none",
			Beta:         false,
			Capabilities: domain.Capabilities{Voice: true, SMS: true, MMS: true},
		})
	}
	return results, nil
}

// Provision registers an incoming phone number.
func (s *PhoneNumberService) Provision(ctx context.Context, accountSID string, params ProvisionParams) (*domain.PhoneNumber, error) {
	phoneNum := params.PhoneNumber
	if phoneNum == "" {
		candidates, err := s.SearchAvailable(ctx, "US", "Local", AvailableFilter{
			AreaCode: params.AreaCode,
			Limit:    1,
		})
		if err != nil || len(candidates) == 0 {
			return nil, NewTwilioError(21408, http.StatusBadRequest, "No available phone numbers in specified area")
		}
		phoneNum = candidates[0].PhoneNumber
	}

	friendly := params.FriendlyName
	if friendly == "" {
		friendly = phoneNum
	}
	smsMethod := params.SmsMethod
	if smsMethod == "" {
		smsMethod = "POST"
	}

	pn := &domain.PhoneNumber{
		SID:               GenerateSID("PN"),
		AccountSID:        accountSID,
		PhoneNumber:       phoneNum,
		FriendlyName:      friendly,
		Status:            "in-use",
		SmsURL:            params.SmsURL,
		SmsMethod:         smsMethod,
		SmsFallbackURL:    params.SmsFallbackURL,
		SmsFallbackMethod: params.SmsFallbackMethod,
		StatusCallback:    params.StatusCallback,
		Capabilities:      domain.Capabilities{Voice: true, SMS: true, MMS: true},
		DateCreated:       time.Now().UTC(),
		DateUpdated:       time.Now().UTC(),
	}

	if err := s.store.CreatePhoneNumber(ctx, pn); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return nil, NewTwilioError(21450, http.StatusConflict, fmt.Sprintf("Phone number %s is already provisioned", phoneNum))
		}
		return nil, err
	}
	return pn, nil
}

// Update modifies phone number webhook and friendly name settings.
func (s *PhoneNumberService) Update(ctx context.Context, accountSID, sid string, params UpdatePhoneParams) (*domain.PhoneNumber, error) {
	pn, err := s.store.GetPhoneNumberBySID(ctx, accountSID, sid)
	if err != nil {
		return nil, err
	}

	if params.FriendlyName != nil {
		pn.FriendlyName = *params.FriendlyName
	}
	if params.SmsURL != nil {
		pn.SmsURL = *params.SmsURL
	}
	if params.SmsMethod != nil {
		pn.SmsMethod = *params.SmsMethod
	}
	if params.SmsFallbackURL != nil {
		pn.SmsFallbackURL = *params.SmsFallbackURL
	}
	if params.SmsFallbackMethod != nil {
		pn.SmsFallbackMethod = *params.SmsFallbackMethod
	}
	if params.StatusCallback != nil {
		pn.StatusCallback = *params.StatusCallback
	}
	pn.DateUpdated = time.Now().UTC()

	if err := s.store.UpdatePhoneNumber(ctx, pn); err != nil {
		return nil, err
	}
	return pn, nil
}
