package service_test

import (
	"testing"

	"github.com/ringzerosystems/omnic/internal/service"
)

func TestSimulationRuleParsing(t *testing.T) {
	sim := service.NewSimulationService()

	r1, err := sim.ParseRuleString("send=429:20429@after 5")
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if r1.Operation != "send" || r1.StatusCode != 429 || r1.ErrorCode != 20429 || r1.AfterN != 5 {
		t.Errorf("unexpected r1: %+v", r1)
	}

	r2, err := sim.ParseRuleString("send=400:30007@to +15550009999")
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if r2.Operation != "send" || r2.StatusCode != 400 || r2.ErrorCode != 30007 || r2.MatchTo != "+15550009999" {
		t.Errorf("unexpected r2: %+v", r2)
	}

	r3, err := sim.ParseRuleString("send=400:30007@every 2")
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if r3.Operation != "send" || r3.EveryN != 2 {
		t.Errorf("unexpected r3: %+v", r3)
	}

	r4, err := sim.ParseRuleString("webhook=timeout")
	if err != nil {
		t.Fatalf("failed to parse rule: %v", err)
	}
	if r4.Operation != "webhook" || !r4.Timeout {
		t.Errorf("unexpected r4: %+v", r4)
	}
}

func TestSimulationEvaluationMatchTo(t *testing.T) {
	sim := service.NewSimulationService()
	sim.AddRule(service.FailRule{
		Operation:  "send",
		StatusCode: 400,
		ErrorCode:  30007,
		Message:    "Carrier content filtering violation",
		MatchTo:    "+15550009999",
	})

	err := sim.Evaluate("send", "+15550009999", "+14155550199")
	if err == nil {
		t.Fatal("expected simulation error, got nil")
	}
	if err.Status != 400 || err.Code != 30007 {
		t.Errorf("unexpected error: %+v", err)
	}

	// Other number should not trigger
	errOther := sim.Evaluate("send", "+15551234567", "+14155550199")
	if errOther != nil {
		t.Errorf("expected no error for other number, got: %v", errOther)
	}
}

func TestSimulationEvaluationAfterN(t *testing.T) {
	sim := service.NewSimulationService()
	sim.AddRule(service.FailRule{
		Operation:  "send",
		StatusCode: 429,
		ErrorCode:  20429,
		AfterN:     2,
	})

	// First two calls should succeed
	if err := sim.Evaluate("send", "+15551234567", ""); err != nil {
		t.Fatalf("expected call 1 to succeed, got %v", err)
	}
	if err := sim.Evaluate("send", "+15551234567", ""); err != nil {
		t.Fatalf("expected call 2 to succeed, got %v", err)
	}

	// Call 3 should fail
	err3 := sim.Evaluate("send", "+15551234567", "")
	if err3 == nil || err3.Code != 20429 {
		t.Fatalf("expected call 3 to fail with 20429, got %v", err3)
	}
}

func TestSimulationEvaluationEveryN(t *testing.T) {
	sim := service.NewSimulationService()
	sim.AddRule(service.FailRule{
		Operation:  "send",
		StatusCode: 400,
		ErrorCode:  30003,
		EveryN:     2,
	})

	if err := sim.Evaluate("send", "+15551234567", ""); err != nil {
		t.Fatalf("call 1 should pass, got %v", err)
	}
	if err := sim.Evaluate("send", "+15551234567", ""); err == nil || err.Code != 30003 {
		t.Fatalf("call 2 should fail with 30003, got %v", err)
	}
	if err := sim.Evaluate("send", "+15551234567", ""); err != nil {
		t.Fatalf("call 3 should pass, got %v", err)
	}
	if err := sim.Evaluate("send", "+15551234567", ""); err == nil || err.Code != 30003 {
		t.Fatalf("call 4 should fail with 30003, got %v", err)
	}
}

func TestSimulationWebhookTimeout(t *testing.T) {
	sim := service.NewSimulationService()
	sim.AddRule(service.FailRule{
		Operation: "webhook",
		Timeout:   true,
	})

	if !sim.ShouldTimeout("webhook", "+14155550199", "+15551234567") {
		t.Errorf("expected ShouldTimeout to return true")
	}
	if sim.ShouldTimeout("send", "+14155550199", "+15551234567") {
		t.Errorf("expected ShouldTimeout for send to return false")
	}
}
