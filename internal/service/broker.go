package service

import (
	"context"
	"sync"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
)

// EventType represents the category of real-time events.
type EventType string

const (
	EventMessageInbound     EventType = "message.inbound"
	EventMessageOutbound    EventType = "message.outbound"
	EventMessageStatus      EventType = "message.status"
	EventConversationUpdate EventType = "conversation.update"
)

// Event represents a pub/sub domain event for handsets and clients.
type Event struct {
	Timestamp time.Time       `json:"timestamp"`
	Payload   *domain.Message `json:"payload"`
	ID        string          `json:"id"`
	Type      EventType       `json:"type"`
	Engine    domain.Engine   `json:"engine"`
}

type subscriber struct {
	ch            chan Event
	done          chan struct{}
	filterNumber  string
	filterAccount string
	filterEngine  string
	id            uint64
	once          sync.Once
}

func (s *subscriber) close(cleanup func(uint64)) {
	s.once.Do(func() {
		close(s.done)
		cleanup(s.id)
		close(s.ch)
	})
}

// EventBroker provides a high-throughput, non-blocking in-memory event bus.
type EventBroker struct {
	subscribers map[uint64]*subscriber
	nextID      uint64
	mu          sync.RWMutex
}

// NewEventBroker constructs a new EventBroker.
func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: make(map[uint64]*subscriber),
	}
}

// NewMemoryEventBroker is an alias for NewEventBroker.
func NewMemoryEventBroker() *EventBroker {
	return NewEventBroker()
}

// Publish distributes an event to all matching subscribers without blocking.
func (b *EventBroker) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers {
		if !matchesFilter(sub, &event) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// Non-blocking drop on slow or overwhelmed subscribers
		}
	}
}

// Subscribe registers a subscriber with optional number, account, and engine filters.
func (b *EventBroker) Subscribe(ctx context.Context, filterNumber, filterAccount string, filterEngine ...string) (<-chan Event, func()) {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	fe := ""
	if len(filterEngine) > 0 {
		fe = filterEngine[0]
	}
	sub := &subscriber{
		id:            id,
		ch:            make(chan Event, 64),
		done:          make(chan struct{}),
		filterNumber:  filterNumber,
		filterAccount: filterAccount,
		filterEngine:  fe,
	}
	b.subscribers[id] = sub
	b.mu.Unlock()

	unsub := func() {
		sub.close(b.removeSubscriber)
	}

	go func() {
		select {
		case <-ctx.Done():
			sub.close(b.removeSubscriber)
		case <-sub.done:
		}
	}()

	return sub.ch, unsub
}

func (b *EventBroker) removeSubscriber(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, id)
}

func matchesFilter(sub *subscriber, ev *Event) bool {
	if sub.filterEngine != "" {
		evEng := ev.Engine
		if evEng == "" && ev.Payload != nil {
			evEng = ev.Payload.Engine
		}
		if evEng != "" && string(evEng) != sub.filterEngine {
			return false
		}
	}
	if sub.filterAccount != "" && (ev.Payload == nil || ev.Payload.AccountSID != sub.filterAccount) {
		return false
	}
	if sub.filterNumber != "" {
		if ev.Payload == nil {
			return false
		}
		if ev.Payload.To != sub.filterNumber && ev.Payload.From != sub.filterNumber {
			return false
		}
	}
	return true
}
