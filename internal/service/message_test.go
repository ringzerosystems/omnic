package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func setupMessageService() (*service.MessageService, repository.Store) {
	store := memory.NewStore()
	svc := service.NewMessageService(store)
	return svc, store
}

func TestCreateMessageValidation(t *testing.T) {
	svc, store := setupMessageService()
	ctx := context.Background()

	// Provision a sender phone number
	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155551234",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true, MMS: true},
	})

	// Missing To
	_, err := svc.Create(ctx, "AC001", service.CreateMessageParams{
		From: "+14155551234",
		Body: "Hello",
	})
	var twErr *service.TwilioError
	if !errors.As(err, &twErr) || twErr.Code != 21604 {
		t.Fatalf("expected ErrMissingTo 21604, got %v", err)
	}

	// Missing Body and MediaURL
	_, err = svc.Create(ctx, "AC001", service.CreateMessageParams{
		To:   "+14155559999",
		From: "+14155551234",
	})
	if !errors.As(err, &twErr) || twErr.Code != 21602 {
		t.Fatalf("expected ErrMissingBody 21602, got %v", err)
	}

	// Unprovisioned From
	_, err = svc.Create(ctx, "AC001", service.CreateMessageParams{
		To:   "+14155559999",
		From: "+14155559998",
		Body: "Hello",
	})
	if !errors.As(err, &twErr) || twErr.Code != 21606 {
		t.Fatalf("expected ErrInvalidFrom 21606, got %v", err)
	}

	// Opted-out recipient
	_ = store.SetOptOut(ctx, "AC001", "+14155559999", true)
	_, err = svc.Create(ctx, "AC001", service.CreateMessageParams{
		To:   "+14155559999",
		From: "+14155551234",
		Body: "Hello",
	})
	if !errors.As(err, &twErr) || twErr.Code != 21610 {
		t.Fatalf("expected ErrUnsubscribed 21610, got %v", err)
	}
}

func TestCreateAndFetchSMS(t *testing.T) {
	svc, store := setupMessageService()
	ctx := context.Background()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155551234",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true},
	})

	msg, err := svc.Create(ctx, "AC001", service.CreateMessageParams{
		To:         "+14155559999",
		From:       "+14155551234",
		Body:       "Test SMS body",
		Properties: map[string]any{"CustomParam": "customValue"},
	})
	if err != nil {
		t.Fatalf("Create SMS failed: %v", err)
	}
	if msg.SID[:2] != "SM" {
		t.Errorf("expected SM prefix for SMS, got %s", msg.SID)
	}
	if msg.Status != domain.StatusQueued {
		t.Errorf("expected queued status, got %v", msg.Status)
	}
	if msg.NumSegments != 1 {
		t.Errorf("expected 1 segment, got %d", msg.NumSegments)
	}
	if msg.Properties["CustomParam"] != "customValue" {
		t.Errorf("expected custom parameter preserved in properties")
	}

	got, err := store.GetMessageBySID(ctx, "AC001", msg.SID)
	if err != nil || got.Body != "Test SMS body" {
		t.Fatalf("Get message failed: got=%v err=%v", got, err)
	}
}

func TestCreateMMS(t *testing.T) {
	svc, store := setupMessageService()
	ctx := context.Background()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155551234",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true, MMS: true},
	})

	msg, err := svc.Create(ctx, "AC001", service.CreateMessageParams{
		To:        "+14155559999",
		From:      "+14155551234",
		Body:      "Check this image",
		MediaURLs: []string{"https://example.com/cat.jpg"},
	})
	if err != nil {
		t.Fatalf("Create MMS failed: %v", err)
	}
	if msg.SID[:2] != "MM" {
		t.Errorf("expected MM prefix for MMS, got %s", msg.SID)
	}
	if msg.NumMedia != 1 {
		t.Errorf("expected 1 media attachment, got %d", msg.NumMedia)
	}

	// Verify media was created in repository
	mediaList, err := store.ListMedia(ctx, "AC001", msg.SID)
	if err != nil || len(mediaList) != 1 {
		t.Fatalf("expected 1 media entity, got %d (err=%v)", len(mediaList), err)
	}
	if mediaList[0].SID[:2] != "ME" {
		t.Errorf("expected ME prefix on media SID, got %s", mediaList[0].SID)
	}
}

func TestUpdateAndDeleteMessage(t *testing.T) {
	svc, store := setupMessageService()
	ctx := context.Background()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155551234",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true},
	})

	future := time.Now().Add(1 * time.Hour).UTC()
	msg, err := svc.Create(ctx, "AC001", service.CreateMessageParams{
		To:           "+14155559999",
		From:         "+14155551234",
		Body:         "Scheduled message",
		SendAt:       &future,
		ScheduleType: "fixed",
	})
	if err != nil {
		t.Fatalf("Create scheduled failed: %v", err)
	}
	if msg.Status != domain.StatusScheduled {
		t.Errorf("expected scheduled status, got %v", msg.Status)
	}

	// Cancel scheduled message
	statusCanceled := domain.StatusCanceled
	updated, err := svc.Update(ctx, "AC001", msg.SID, service.UpdateMessageParams{
		Status: &statusCanceled,
	})
	if err != nil || updated.Status != domain.StatusCanceled {
		t.Fatalf("Cancel message failed: updated=%v err=%v", updated, err)
	}

	// Delete message
	if err := store.DeleteMessage(ctx, "AC001", msg.SID); err != nil {
		t.Fatalf("Delete message failed: %v", err)
	}
	_, err = store.GetMessageBySID(ctx, "AC001", msg.SID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMessageServiceBrokerPublish(t *testing.T) {
	svc, store := setupMessageService()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155551234",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true},
	})

	broker := service.NewMemoryEventBroker()
	svc.SetBroker(broker)

	ch, unsub := broker.Subscribe(ctx, "+14155559999", "AC001")
	defer unsub()

	msg, err := svc.Create(ctx, "AC001", service.CreateMessageParams{
		From: "+14155551234",
		To:   "+14155559999",
		Body: "Hello via broker",
	})
	if err != nil {
		t.Fatalf("svc.Create failed: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.ID != msg.SID {
			t.Errorf("expected event ID %s, got %s", msg.SID, ev.ID)
		}
		if ev.Type != service.EventMessageOutbound {
			t.Errorf("expected event type %s, got %s", service.EventMessageOutbound, ev.Type)
		}
		if ev.Payload.Body != "Hello via broker" {
			t.Errorf("expected payload body 'Hello via broker', got %s", ev.Payload.Body)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event on subscriber channel")
	}
}

func TestMessageServiceSimulationFailure(t *testing.T) {
	svc, store := setupMessageService()
	ctx := context.Background()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155551234",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true},
	})

	sim := service.NewSimulationService()
	sim.AddRule(service.FailRule{
		Operation:  "send",
		StatusCode: 429,
		ErrorCode:  20429,
	})
	svc.SetSimulation(sim)

	_, err := svc.Create(ctx, "AC001", service.CreateMessageParams{
		From: "+14155551234",
		To:   "+14155559999",
		Body: "Rate limited message",
	})
	if err == nil {
		t.Fatal("expected simulation error 429, got nil")
	}
	var twErr *service.TwilioError
	if !errors.As(err, &twErr) || twErr.Code != 20429 {
		t.Fatalf("expected 20429 rate limit error, got %v", err)
	}
}
