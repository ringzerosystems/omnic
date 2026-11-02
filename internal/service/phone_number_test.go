package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func setupPhoneNumberService() (*service.PhoneNumberService, repository.Store) {
	store := memory.NewStore()
	svc := service.NewPhoneNumberService(store)
	return svc, store
}

func TestAvailablePhoneNumbers(t *testing.T) {
	svc, _ := setupPhoneNumberService()
	ctx := context.Background()

	results, err := svc.SearchAvailable(ctx, "US", "Local", service.AvailableFilter{
		AreaCode:   "415",
		SmsEnabled: true,
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("SearchAvailable failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least 1 available number")
	}
	for _, n := range results {
		if !n.Capabilities.SMS {
			t.Errorf("expected SMS capability on %s", n.PhoneNumber)
		}
	}
}

func TestProvisionAndManagePhoneNumber(t *testing.T) {
	svc, store := setupPhoneNumberService()
	ctx := context.Background()

	// Provision by specific number
	pn, err := svc.Provision(ctx, "AC001", service.ProvisionParams{
		PhoneNumber:  "+14155550199",
		FriendlyName: "Main Line",
		SmsURL:       "https://example.com/sms",
		SmsMethod:    "POST",
	})
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}
	if pn.PhoneNumber != "+14155550199" {
		t.Errorf("got PhoneNumber = %s, want +14155550199", pn.PhoneNumber)
	}
	if pn.SID == "" || pn.SID[:2] != "PN" {
		t.Errorf("invalid SID generated: %s", pn.SID)
	}

	// Fetch by SID
	got, err := store.GetPhoneNumberBySID(ctx, "AC001", pn.SID)
	if err != nil || got.PhoneNumber != pn.PhoneNumber {
		t.Fatalf("Get failed: got=%v err=%v", got, err)
	}

	// Update configuration
	newURL := "https://example.com/new-sms"
	updated, err := svc.Update(ctx, "AC001", pn.SID, service.UpdatePhoneParams{
		SmsURL: &newURL,
	})
	if err != nil || updated.SmsURL != newURL {
		t.Fatalf("Update failed: got=%v err=%v", updated, err)
	}

	// Release (Delete)
	if err := store.DeletePhoneNumber(ctx, "AC001", pn.SID); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	_, err = store.GetPhoneNumberBySID(ctx, "AC001", pn.SID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("expected ErrNotFound on released number, got %v", err)
	}
}
