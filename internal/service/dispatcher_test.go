package service_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func TestInboundSMSWebhook(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	var receivedBody string
	var receivedSig string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		receivedSig = r.Header.Get("X-Twilio-Signature")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<Response><Message><Body>Echo reply</Body></Message></Response>`))
	}))
	defer server.Close()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       server.URL,
		SmsMethod:    "POST",
		Capabilities: domain.Capabilities{SMS: true},
	})

	dispatcher := service.NewWebhookDispatcher(store, "secret-token")
	reply, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+14155551111",
		To:   "+14155550199",
		Body: "Hello from handset",
	})
	if err != nil {
		t.Fatalf("DispatchInbound failed: %v", err)
	}

	mu.Lock()
	if !strings.Contains(receivedBody, "Body=Hello+from+handset") && !strings.Contains(receivedBody, "Body=Hello%20from%20handset") {
		t.Errorf("expected body in webhook request, got: %s", receivedBody)
	}
	if receivedSig == "" {
		t.Errorf("expected X-Twilio-Signature header")
	}
	mu.Unlock()

	if reply == nil || reply.Body != "Echo reply" {
		t.Fatalf("expected echo reply from TwiML, got: %v", reply)
	}
	if reply.Direction != domain.DirectionOutboundReply {
		t.Errorf("expected outbound-reply direction, got %v", reply.Direction)
	}
}

func TestInboundSMSComplianceKeywords(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN001",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       "http://invalid.url", // Should not be called for STOP
		Capabilities: domain.Capabilities{SMS: true},
	})

	dispatcher := service.NewWebhookDispatcher(store, "token")

	// Send STOP
	reply, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+14155551111",
		To:   "+14155550199",
		Body: "STOP",
	})
	if err != nil || reply == nil {
		t.Fatalf("DispatchInbound STOP failed: %v", err)
	}
	if !strings.Contains(reply.Body, "unsubscribed") {
		t.Errorf("expected unsubscribe notice in reply, got %s", reply.Body)
	}

	opted, _ := store.IsOptedOut(ctx, "AC001", "+14155551111")
	if !opted {
		t.Errorf("expected number to be marked opted-out")
	}

	// Send START
	reply, err = dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+14155551111",
		To:   "+14155550199",
		Body: "START",
	})
	if err != nil || reply == nil {
		t.Fatalf("DispatchInbound START failed: %v", err)
	}
	if !strings.Contains(reply.Body, "re-subscribed") && !strings.Contains(reply.Body, "subscribed") {
		t.Errorf("expected re-subscribed notice in reply, got %s", reply.Body)
	}

	opted, _ = store.IsOptedOut(ctx, "AC001", "+14155551111")
	if opted {
		t.Errorf("expected number to no longer be opted-out")
	}
}

func TestOutboundStatusCallback(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	var statuses []string
	var mu sync.Mutex
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = r.ParseForm()
		statuses = append(statuses, r.FormValue("MessageStatus"))
		w.WriteHeader(http.StatusOK)
	}))
	defer statusServer.Close()

	dispatcher := service.NewWebhookDispatcher(store, "token")
	dispatcher.SetDeliveryDelay(10 * time.Millisecond)

	msg := &domain.Message{
		SID:            "SM999",
		AccountSID:     "AC001",
		To:             "+14155551111",
		From:           "+14155550199",
		Body:           "Outbound message",
		Status:         domain.StatusQueued,
		StatusCallback: statusServer.URL,
		DateCreated:    time.Now().UTC(),
		DateUpdated:    time.Now().UTC(),
	}
	_ = store.CreateMessage(ctx, msg)

	dispatcher.EnqueueOutboundDelivery(msg.AccountSID, msg.SID)

	// Wait for async state transitions (queued -> sending -> delivered)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(statuses) < 2 {
		t.Fatalf("expected at least 2 status callbacks, got %v", statuses)
	}
	lastStatus := statuses[len(statuses)-1]
	if lastStatus != "delivered" {
		t.Errorf("expected final status delivered, got %s", lastStatus)
	}
}

func TestInboundSMSUpstreamFallbackAndHeaders(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	var receivedHeaders http.Header
	var receivedBody string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedHeaders = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<Response/>`))
	}))
	defer server.Close()

	// Provision number with EMPTY SmsURL to trigger fallback to default upstream
	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN002",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       "",
		Capabilities: domain.Capabilities{SMS: true},
	})

	dispatcher := service.NewWebhookDispatcher(store, "secret-token")
	dispatcher.SetDefaultUpstream(server.URL, "POST")

	_, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+15551234567",
		To:   "+14155550199",
		Body: "Testing upstream fallback with multi segment text that should compute segments accurately",
	})
	if err != nil {
		t.Fatalf("DispatchInbound failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if ua := receivedHeaders.Get("User-Agent"); ua != "TwilioProxy/1.1 (omnic)" {
		t.Errorf("expected User-Agent TwilioProxy/1.1 (omnic), got %q", ua)
	}
	if sig := receivedHeaders.Get("X-Twilio-Signature"); sig == "" {
		t.Errorf("expected X-Twilio-Signature header")
	}
	if !strings.Contains(receivedBody, "SmsMessageSid=") {
		t.Errorf("expected SmsMessageSid in body: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, "NumSegments=") {
		t.Errorf("expected NumSegments in body: %s", receivedBody)
	}
	if !strings.Contains(receivedBody, "ApiVersion=2010-04-01") {
		t.Errorf("expected ApiVersion=2010-04-01 in body: %s", receivedBody)
	}
}

func TestInboundSMSUpstreamFailure(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN003",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       server.URL,
		SmsMethod:    "POST",
		Capabilities: domain.Capabilities{SMS: true},
	})

	dispatcher := service.NewWebhookDispatcher(store, "secret-token")
	_, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+15551234567",
		To:   "+14155550199",
		Body: "Fail upstream",
	})
	if err == nil {
		t.Fatalf("expected error on upstream 502, got nil")
	}

	var twErr *service.TwilioError
	if !errors.As(err, &twErr) {
		t.Fatalf("expected *service.TwilioError, got %T: %v", err, err)
	}
	if twErr.Code != 11200 {
		t.Errorf("expected error code 11200, got %d", twErr.Code)
	}
}

func TestInboundSMSNoUpstream(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN004",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       "",
		Capabilities: domain.Capabilities{SMS: true},
	})

	dispatcher := service.NewWebhookDispatcher(store, "secret-token")
	reply, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+15551234567",
		To:   "+14155550199",
		Body: "No upstream configured",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != nil {
		t.Fatalf("expected nil reply, got %v", reply)
	}

	msgs, total, _ := store.ListMessages(ctx, "AC001", domain.MessageFilter{To: "+14155550199"})
	if total != 1 || len(msgs) != 1 {
		t.Fatalf("expected message to be stored in repository, got %d", total)
	}
	if msgs[0].Status != domain.StatusReceived {
		t.Errorf("expected status received, got %v", msgs[0].Status)
	}
}

func TestDispatcherBrokerPublish(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<Response><Message><Body>Agent Reply</Body></Message></Response>`))
	}))
	defer server.Close()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN005",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       server.URL,
		SmsMethod:    "POST",
		Capabilities: domain.Capabilities{SMS: true},
	})

	broker := service.NewMemoryEventBroker()
	dispatcher := service.NewWebhookDispatcher(store, "secret-token")
	dispatcher.SetBroker(broker)

	ch, unsub := broker.Subscribe(ctx, "+15551234567", "AC001")
	defer unsub()

	reply, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+15551234567",
		To:   "+14155550199",
		Body: "User Query",
	})
	if err != nil {
		t.Fatalf("DispatchInbound failed: %v", err)
	}
	if reply == nil || reply.Body != "Agent Reply" {
		t.Fatalf("unexpected reply: %v", reply)
	}

	// We should receive 2 events: message.inbound, and message.outbound (reply)
	receivedEvents := make([]service.Event, 0, 2)
	timeout := time.After(500 * time.Millisecond)
	for len(receivedEvents) < 2 {
		select {
		case ev := <-ch:
			receivedEvents = append(receivedEvents, ev)
		case <-timeout:
			t.Fatalf("timed out waiting for events, received %d", len(receivedEvents))
		}
	}

	if receivedEvents[0].Type != service.EventMessageInbound {
		t.Errorf("expected first event message.inbound, got %s", receivedEvents[0].Type)
	}
	if receivedEvents[1].Type != service.EventMessageOutbound {
		t.Errorf("expected second event message.outbound, got %s", receivedEvents[1].Type)
	}
}

func TestDispatcherSimulationTimeout(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.CreatePhoneNumber(ctx, &domain.PhoneNumber{
		SID:          "PN006",
		AccountSID:   "AC001",
		PhoneNumber:  "+14155550199",
		Status:       "in-use",
		SmsURL:       "http://localhost:9999/sms",
		Capabilities: domain.Capabilities{SMS: true},
	})

	sim := service.NewSimulationService()
	sim.AddRule(service.FailRule{
		Operation: "webhook",
		Timeout:   true,
	})

	dispatcher := service.NewWebhookDispatcher(store, "token")
	dispatcher.SetSimulation(sim)

	_, err := dispatcher.DispatchInbound(ctx, service.InboundSMSParams{
		From: "+15551234567",
		To:   "+14155550199",
		Body: "Timeout query",
	})
	if err == nil {
		t.Fatal("expected simulation timeout error, got nil")
	}
	var twErr *service.TwilioError
	if !errors.As(err, &twErr) || twErr.Code != 11200 {
		t.Fatalf("expected 11200 timeout error, got %v", err)
	}
}
