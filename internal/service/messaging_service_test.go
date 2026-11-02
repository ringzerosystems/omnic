package service_test

import (
	"context"
	"testing"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func setupMessagingService() (*service.MessagingServiceService, repository.Store) {
	store := memory.NewStore()
	svc := service.NewMessagingServiceService(store)
	return svc, store
}

func TestMessagingServiceCRUD(t *testing.T) {
	ctx := context.Background()
	svc, store := setupMessagingService()

	created, err := svc.Create(ctx, "AC001", service.CreateServiceParams{
		FriendlyName: "Marketing Pool",
		StickySender: true,
		Geomatch:     true,
	})
	if err != nil {
		t.Fatalf("Create service failed: %v", err)
	}
	if created.SID[:2] != "MG" {
		t.Errorf("expected MG prefix, got %s", created.SID)
	}

	got, err := store.GetMessagingServiceBySID(ctx, "AC001", created.SID)
	if err != nil || got.FriendlyName != "Marketing Pool" {
		t.Fatalf("Get service failed: got=%v err=%v", got, err)
	}

	newName := "Updated Marketing Pool"
	updated, err := svc.Update(ctx, "AC001", created.SID, service.UpdateServiceParams{
		FriendlyName: &newName,
	})
	if err != nil || updated.FriendlyName != newName {
		t.Fatalf("Update service failed: updated=%v err=%v", updated, err)
	}
}

func TestSenderPoolAndGeomatch(t *testing.T) {
	ctx := context.Background()
	svc, store := setupMessagingService()

	// Provision phone numbers in different area codes
	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN415",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550100",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true},
	})
	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN212",
		AccountSID:   "AC001",
		PhoneNumber:  "+12125550200",
		Status:       "in-use",
		Capabilities: domain.Capabilities{SMS: true},
	})

	ms, err := svc.Create(ctx, "AC001", service.CreateServiceParams{
		FriendlyName: "Multi-Region Pool",
		Geomatch:     true,
	})
	if err != nil || ms == nil {
		t.Fatalf("Create service failed: %v", err)
	}

	_, err = svc.AddPhoneNumber(ctx, ms.SID, "PN415")
	if err != nil {
		t.Fatalf("AddPhoneNumber PN415 failed: %v", err)
	}
	_, err = svc.AddPhoneNumber(ctx, ms.SID, "PN212")
	if err != nil {
		t.Fatalf("AddPhoneNumber PN212 failed: %v", err)
	}

	// Geomatch for NYC area code 212
	selected, err := svc.SelectSender(ctx, ms.SID, "+12125559999")
	if err != nil {
		t.Fatalf("SelectSender failed: %v", err)
	}
	if selected != "+12125550200" {
		t.Errorf("expected Geomatch to pick +12125550200, got %s", selected)
	}

	// Geomatch for SF area code 415
	selected, err = svc.SelectSender(ctx, ms.SID, "+14155559999")
	if err != nil {
		t.Fatalf("SelectSender failed: %v", err)
	}
	if selected != "+14155550100" {
		t.Errorf("expected Geomatch to pick +14155550100, got %s", selected)
	}
}
