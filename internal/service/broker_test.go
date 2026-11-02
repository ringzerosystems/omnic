package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/service"
)

func TestEventBrokerPublishSubscribe(t *testing.T) {
	broker := service.NewMemoryEventBroker()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, unsub := broker.Subscribe(ctx, "+15551234567", "")
	defer unsub()

	// 1. Publish matching event (To matches)
	msg1 := &domain.Message{
		SID:        "SM111",
		AccountSID: "AC001",
		To:         "+15551234567",
		From:       "+14155550199",
		Body:       "Matched event",
		Status:     domain.StatusQueued,
	}
	broker.Publish(service.Event{
		ID:        "SM111",
		Type:      service.EventMessageOutbound,
		Timestamp: time.Now().UTC(),
		Payload:   msg1,
	})

	select {
	case ev := <-ch:
		if ev.ID != "SM111" || ev.Payload.Body != "Matched event" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for matching event")
	}

	// 2. Publish non-matching event
	msg2 := &domain.Message{
		SID:        "SM222",
		AccountSID: "AC001",
		To:         "+19999999999",
		From:       "+18888888888",
		Body:       "Unmatched event",
		Status:     domain.StatusQueued,
	}
	broker.Publish(service.Event{
		ID:        "SM222",
		Type:      service.EventMessageOutbound,
		Timestamp: time.Now().UTC(),
		Payload:   msg2,
	})

	select {
	case ev := <-ch:
		t.Fatalf("did not expect event, got: %+v", ev)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event received
	}
}

func TestEventBrokerSlowSubscriberNonBlocking(t *testing.T) {
	broker := service.NewMemoryEventBroker()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, unsub := broker.Subscribe(ctx, "", "")
	defer unsub()

	start := time.Now()
	for i := range 200 {
		broker.Publish(service.Event{
			ID:        "SM_SLOW",
			Type:      service.EventMessageInbound,
			Timestamp: time.Now().UTC(),
			Payload: &domain.Message{
				SID:  "SM_SLOW",
				Body: "burst",
			},
		})
		_ = i
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Publish blocked on slow subscriber, took %v", elapsed)
	}
}

func TestEventBrokerContextCancellation(t *testing.T) {
	broker := service.NewMemoryEventBroker()
	ctx, cancel := context.WithCancel(context.Background())

	ch, unsub := broker.Subscribe(ctx, "", "")
	defer unsub()

	cancel() // Trigger cancellation

	// Give a short window for cleanup goroutine to exit
	select {
	case _, ok := <-ch:
		if ok {
			// A drained buffered event is fine, but channel should close
			for range ch {
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected channel to close after context cancellation")
	}
}
