package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func setupTestServerWithBroker() (*echo.Echo, *handler.Handler, *service.EventBroker) {
	store := memory.NewStore()

	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	dispatcher := service.NewWebhookDispatcher(store, "test-token")

	broker := service.NewMemoryEventBroker()
	msgSvc.SetBroker(broker)
	dispatcher.SetBroker(broker)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, dispatcher)
	h.SetBroker(broker)

	e := echo.New()
	h.RegisterRoutes(e)
	return e, h, broker
}

func TestEmuEventsSSE(t *testing.T) {
	e, _, broker := setupTestServerWithBroker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/emu/events?number=+15551234567", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.ServeHTTP(rec, req)
	}()

	// Wait briefly for handler to establish subscription
	time.Sleep(50 * time.Millisecond)

	// Publish an event via broker
	broker.Publish(service.Event{
		ID:        "SM_SSE_1",
		Type:      service.EventMessageOutbound,
		Timestamp: time.Now().UTC(),
		Payload: &domain.Message{
			SID:        "SM_SSE_1",
			AccountSID: "AC001",
			To:         "+15551234567",
			From:       "+14155550199",
			Body:       "SSE Test Message",
			Status:     domain.StatusQueued,
		},
	})

	// Cancel context to close SSE stream
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: message.outbound") {
		t.Errorf("expected event: message.outbound in SSE output, got:\n%s", body)
	}
	if !strings.Contains(body, "id: SM_SSE_1") {
		t.Errorf("expected id: SM_SSE_1 in SSE output, got:\n%s", body)
	}
	if !strings.Contains(body, "SSE Test Message") {
		t.Errorf("expected payload in SSE output, got:\n%s", body)
	}
}

func TestEmuSimulateRulesREST(t *testing.T) {
	e, h, _ := setupTestServerWithBroker()
	sim := service.NewSimulationService()
	h.SetSimulation(sim)

	// 1. Set rules via POST
	payload := `{"rules":[{"operation":"send","status_code":400,"error_code":30007,"match_to":"+15550009999"}]}`
	req := httptest.NewRequest(http.MethodPost, "/emu/simulate/rules", strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /emu/simulate/rules failed: %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Get rules via GET
	req = httptest.NewRequest(http.MethodGet, "/emu/simulate/rules", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /emu/simulate/rules failed: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "30007") {
		t.Errorf("expected 30007 in response, got: %s", rec.Body.String())
	}

	// 3. Clear rules via DELETE
	req = httptest.NewRequest(http.MethodDelete, "/emu/simulate/rules", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /emu/simulate/rules failed: %d", rec.Code)
	}
	if len(sim.GetRules()) != 0 {
		t.Errorf("expected rules to be cleared, got %d", len(sim.GetRules()))
	}
}
