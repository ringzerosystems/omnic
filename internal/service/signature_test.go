package service_test

import (
	"testing"

	"github.com/ringzerosystems/omnic/internal/service"
)

func TestCalculateSignature(t *testing.T) {
	authToken := "12345"
	targetURL := "https://mycompany.com/myapp.php?foo=1&bar=2"
	params := map[string]string{
		"CallSid": "CA1234567890ABCDE",
		"Caller":  "+14158675309",
		"Digits":  "1234",
		"From":    "+14158675309",
		"To":      "+18005551212",
	}

	// Calculate signature
	sig := service.CalculateSignature(targetURL, params, authToken)
	if sig == "" {
		t.Fatalf("expected non-empty signature")
	}

	// Validate signature
	if !service.ValidateSignature(targetURL, params, authToken, sig) {
		t.Errorf("expected signature to validate successfully")
	}

	// Validate signature with incorrect token
	if service.ValidateSignature(targetURL, params, "wrongtoken", sig) {
		t.Errorf("expected signature to fail with incorrect token")
	}
}
