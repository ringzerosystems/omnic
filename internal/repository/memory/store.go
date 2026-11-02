package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// Store is a unified in-memory thread-safe store for all domain entities.
type Store struct {
	messages     map[string]*domain.Message
	byEngineID   map[string]*domain.Message
	numbers      map[string]*domain.PhoneNumber
	media        map[string]*domain.Media
	optOut       map[string]bool
	services     map[string]*domain.MessagingService
	servicePools map[string][]*domain.ServicePhoneNumber
	mu           sync.RWMutex
}

// NewStore constructs a new unified in-memory Store.
func NewStore() *Store {
	return &Store{
		messages:     make(map[string]*domain.Message),
		byEngineID:   make(map[string]*domain.Message),
		numbers:      make(map[string]*domain.PhoneNumber),
		media:        make(map[string]*domain.Media),
		optOut:       make(map[string]bool),
		services:     make(map[string]*domain.MessagingService),
		servicePools: make(map[string][]*domain.ServicePhoneNumber),
	}
}

func entityKey(accountSID, sid string) string {
	return accountSID + ":" + sid
}

func engineKey(engine domain.Engine, engineID string) string {
	return string(engine) + ":" + engineID
}

// --- Messages ---

// CreateMessage inserts a message into the store.
func (s *Store) CreateMessage(_ context.Context, msg *domain.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if msg.Engine == "" {
		msg.Engine = domain.EngineTwilio
	}
	key := entityKey(msg.AccountSID, msg.SID)
	if _, exists := s.messages[key]; exists {
		return repository.ErrConflict
	}
	cpy := *msg
	s.messages[key] = &cpy
	s.byEngineID[engineKey(msg.Engine, msg.SID)] = &cpy
	return nil
}

// GetMessageBySID retrieves a message by account and SID.
func (s *Store) GetMessageBySID(_ context.Context, accountSID, sid string) (*domain.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg, exists := s.messages[entityKey(accountSID, sid)]
	if !exists {
		return nil, repository.ErrNotFound
	}
	cpy := *msg
	return &cpy, nil
}

// GetMessageByEngineID retrieves a message by engine and engine-specific ID.
func (s *Store) GetMessageByEngineID(_ context.Context, engine domain.Engine, engineID string) (*domain.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if msg, exists := s.byEngineID[engineKey(engine, engineID)]; exists {
		cpy := *msg
		return &cpy, nil
	}
	// Fallback scan
	for _, m := range s.messages {
		if m.SID == engineID && (m.Engine == engine || (engine == domain.EngineTwilio && m.Engine == "")) {
			cpy := *m
			return &cpy, nil
		}
	}
	return nil, repository.ErrNotFound
}

// ListMessages returns messages matching filter criteria.
func (s *Store) ListMessages(_ context.Context, accountSID string, filter domain.MessageFilter) ([]*domain.Message, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*domain.Message, 0, len(s.messages))
	for _, m := range s.messages {
		if accountSID != "" && m.AccountSID != accountSID {
			continue
		}
		if filter.Engine != "" && m.Engine != filter.Engine {
			continue
		}
		if filter.To != "" && m.To != filter.To {
			continue
		}
		if filter.From != "" && m.From != filter.From {
			continue
		}
		if filter.DateSent != nil {
			if m.DateSent == nil || !m.DateSent.Equal(*filter.DateSent) {
				continue
			}
		}
		cpy := *m
		matched = append(matched, &cpy)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].DateCreated.After(matched[j].DateCreated)
	})

	total := len(matched)
	if filter.Offset >= total {
		return []*domain.Message{}, total, nil
	}
	matched = matched[filter.Offset:]
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}
	return matched, total, nil
}

// UpdateMessage updates an existing message.
func (s *Store) UpdateMessage(_ context.Context, msg *domain.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(msg.AccountSID, msg.SID)
	if _, exists := s.messages[key]; !exists {
		return repository.ErrNotFound
	}
	if msg.Engine == "" {
		msg.Engine = domain.EngineTwilio
	}
	cpy := *msg
	s.messages[key] = &cpy
	s.byEngineID[engineKey(msg.Engine, msg.SID)] = &cpy
	return nil
}

// DeleteMessage removes a message.
func (s *Store) DeleteMessage(_ context.Context, accountSID, sid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(accountSID, sid)
	msg, exists := s.messages[key]
	if !exists {
		return repository.ErrNotFound
	}
	delete(s.byEngineID, engineKey(msg.Engine, sid))
	delete(s.messages, key)
	return nil
}

// --- Phone Numbers ---

// CreatePhoneNumber provisions a phone number.
func (s *Store) CreatePhoneNumber(_ context.Context, pn *domain.PhoneNumber) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.numbers {
		if p.PhoneNumber == pn.PhoneNumber {
			return repository.ErrConflict
		}
	}
	cpy := *pn
	s.numbers[entityKey(pn.AccountSID, pn.SID)] = &cpy
	return nil
}

// GetPhoneNumberBySID retrieves a number by account and SID.
func (s *Store) GetPhoneNumberBySID(_ context.Context, accountSID, sid string) (*domain.PhoneNumber, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pn, exists := s.numbers[entityKey(accountSID, sid)]
	if !exists {
		return nil, repository.ErrNotFound
	}
	cpy := *pn
	return &cpy, nil
}

// GetPhoneNumberByNumber retrieves a number by account and phone number string.
func (s *Store) GetPhoneNumberByNumber(_ context.Context, accountSID, number string) (*domain.PhoneNumber, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.numbers {
		if p.AccountSID == accountSID && p.PhoneNumber == number {
			cpy := *p
			return &cpy, nil
		}
	}
	return nil, repository.ErrNotFound
}

// FindPhoneNumberByNumber retrieves a number by phone number string across any account.
func (s *Store) FindPhoneNumberByNumber(_ context.Context, number string) (*domain.PhoneNumber, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.numbers {
		if p.PhoneNumber == number {
			cpy := *p
			return &cpy, nil
		}
	}
	return nil, repository.ErrNotFound
}

// ListPhoneNumbers returns numbers matching filter criteria.
func (s *Store) ListPhoneNumbers(_ context.Context, accountSID string, filter domain.PhoneNumberFilter) ([]*domain.PhoneNumber, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*domain.PhoneNumber, 0, len(s.numbers))
	for _, p := range s.numbers {
		if p.AccountSID != accountSID {
			continue
		}
		if filter.PhoneNumber != "" && p.PhoneNumber != filter.PhoneNumber {
			continue
		}
		if filter.FriendlyName != "" && p.FriendlyName != filter.FriendlyName {
			continue
		}
		cpy := *p
		matched = append(matched, &cpy)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].DateCreated.After(matched[j].DateCreated)
	})

	total := len(matched)
	if filter.Offset >= total {
		return []*domain.PhoneNumber{}, total, nil
	}
	matched = matched[filter.Offset:]
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}
	return matched, total, nil
}

// UpdatePhoneNumber updates an existing phone number.
func (s *Store) UpdatePhoneNumber(_ context.Context, pn *domain.PhoneNumber) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(pn.AccountSID, pn.SID)
	if _, exists := s.numbers[key]; !exists {
		return repository.ErrNotFound
	}
	cpy := *pn
	s.numbers[key] = &cpy
	return nil
}

// DeletePhoneNumber releases a phone number.
func (s *Store) DeletePhoneNumber(_ context.Context, accountSID, sid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(accountSID, sid)
	if _, exists := s.numbers[key]; !exists {
		return repository.ErrNotFound
	}
	delete(s.numbers, key)
	return nil
}

// --- Media ---

// CreateMedia stores a media attachment.
func (s *Store) CreateMedia(_ context.Context, media *domain.Media) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(media.AccountSID, media.SID)
	if _, exists := s.media[key]; exists {
		return repository.ErrConflict
	}
	cpy := *media
	s.media[key] = &cpy
	return nil
}

// GetMediaBySID retrieves media by SID and parent message SID.
func (s *Store) GetMediaBySID(_ context.Context, accountSID, parentSID, sid string) (*domain.Media, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, exists := s.media[entityKey(accountSID, sid)]
	if !exists || m.ParentSID != parentSID {
		return nil, repository.ErrNotFound
	}
	cpy := *m
	return &cpy, nil
}

// ListMedia returns all media attachments for a parent message.
func (s *Store) ListMedia(_ context.Context, accountSID, parentSID string) ([]*domain.Media, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*domain.Media, 0, len(s.media))
	for _, m := range s.media {
		if m.AccountSID == accountSID && m.ParentSID == parentSID {
			cpy := *m
			matched = append(matched, &cpy)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].DateCreated.Before(matched[j].DateCreated)
	})
	return matched, nil
}

// DeleteMedia removes a media attachment.
func (s *Store) DeleteMedia(_ context.Context, accountSID, parentSID, sid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(accountSID, sid)
	m, exists := s.media[key]
	if !exists || m.ParentSID != parentSID {
		return repository.ErrNotFound
	}
	delete(s.media, key)
	return nil
}

// --- OptOut ---

// SetOptOut records the opt-out status for a phone number.
func (s *Store) SetOptOut(_ context.Context, accountSID, phoneNumber string, optedOut bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.optOut[accountSID+":"+phoneNumber] = optedOut
	return nil
}

// IsOptedOut checks if a recipient number is opted out.
func (s *Store) IsOptedOut(_ context.Context, accountSID, phoneNumber string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.optOut[accountSID+":"+phoneNumber], nil
}

// --- Messaging Services ---

// CreateMessagingService inserts a messaging service.
func (s *Store) CreateMessagingService(_ context.Context, svc *domain.MessagingService) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(svc.AccountSID, svc.SID)
	if _, exists := s.services[key]; exists {
		return repository.ErrConflict
	}
	cpy := *svc
	s.services[key] = &cpy
	return nil
}

// GetMessagingServiceBySID retrieves a service by SID.
func (s *Store) GetMessagingServiceBySID(_ context.Context, accountSID, sid string) (*domain.MessagingService, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	svc, exists := s.services[entityKey(accountSID, sid)]
	if !exists {
		return nil, repository.ErrNotFound
	}
	cpy := *svc
	return &cpy, nil
}

// ListMessagingServices returns messaging services for an account.
func (s *Store) ListMessagingServices(_ context.Context, accountSID string, limit, offset int) ([]*domain.MessagingService, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*domain.MessagingService, 0, len(s.services))
	for _, svc := range s.services {
		if svc.AccountSID == accountSID {
			cpy := *svc
			matched = append(matched, &cpy)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].DateCreated.After(matched[j].DateCreated)
	})

	total := len(matched)
	if offset >= total {
		return []*domain.MessagingService{}, total, nil
	}
	matched = matched[offset:]
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, total, nil
}

// UpdateMessagingService updates an existing messaging service.
func (s *Store) UpdateMessagingService(_ context.Context, svc *domain.MessagingService) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(svc.AccountSID, svc.SID)
	if _, exists := s.services[key]; !exists {
		return repository.ErrNotFound
	}
	cpy := *svc
	s.services[key] = &cpy
	return nil
}

// DeleteMessagingService removes a messaging service and its sender pool.
func (s *Store) DeleteMessagingService(_ context.Context, accountSID, sid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := entityKey(accountSID, sid)
	if _, exists := s.services[key]; !exists {
		return repository.ErrNotFound
	}
	delete(s.services, key)
	delete(s.servicePools, sid)
	return nil
}

// AddServicePhoneNumber binds a phone number to a service pool.
func (s *Store) AddServicePhoneNumber(_ context.Context, spn *domain.ServicePhoneNumber) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pool := s.servicePools[spn.ServiceSID]
	for _, existing := range pool {
		if existing.PhoneNumberSID == spn.PhoneNumberSID {
			return repository.ErrConflict
		}
	}
	cpy := *spn
	s.servicePools[spn.ServiceSID] = append(pool, &cpy)
	return nil
}

// ListServicePhoneNumbers returns all numbers in a service sender pool.
func (s *Store) ListServicePhoneNumbers(_ context.Context, serviceSID string) ([]*domain.ServicePhoneNumber, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pool, exists := s.servicePools[serviceSID]
	if !exists {
		return []*domain.ServicePhoneNumber{}, nil
	}
	matched := make([]*domain.ServicePhoneNumber, len(pool))
	for i, spn := range pool {
		cpy := *spn
		matched[i] = &cpy
	}
	return matched, nil
}

// RemoveServicePhoneNumber unbinds a phone number from a service pool.
func (s *Store) RemoveServicePhoneNumber(_ context.Context, serviceSID, phoneNumberSID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pool, exists := s.servicePools[serviceSID]
	if !exists {
		return repository.ErrNotFound
	}
	newPool := make([]*domain.ServicePhoneNumber, 0, len(pool))
	found := false
	for _, spn := range pool {
		if spn.PhoneNumberSID == phoneNumberSID {
			found = true
			continue
		}
		newPool = append(newPool, spn)
	}
	if !found {
		return repository.ErrNotFound
	}
	s.servicePools[serviceSID] = newPool
	return nil
}
