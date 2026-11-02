package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

func setupTestServer() (*echo.Echo, *handler.Handler) {
	store := memory.NewStore()

	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	dispatcher := service.NewWebhookDispatcher(store, "test-token")

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, dispatcher)
	e := echo.New()
	h.RegisterRoutes(e)
	return e, h
}

func TestAvailablePhoneNumbersHTTP(t *testing.T) {
	e, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/2010-04-01/Accounts/AC001/AvailablePhoneNumbers/US/Local.json?AreaCode=415", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	numbers, ok := resp["available_phone_numbers"].([]any)
	if !ok || len(numbers) == 0 {
		t.Fatalf("expected available_phone_numbers array, got: %v", resp)
	}
}

func TestIncomingPhoneNumbersHTTP(t *testing.T) {
	e, _ := setupTestServer()

	// Provision number
	form := url.Values{
		"PhoneNumber":  {"+14155550199"},
		"FriendlyName": {"Support Line"},
		"SmsUrl":       {"https://example.com/sms"},
	}
	req := httptest.NewRequest(http.MethodPost, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers.json", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	sid, _ := created["sid"].(string)
	if sid == "" || !strings.HasPrefix(sid, "PN") {
		t.Fatalf("invalid phone number SID: %v", sid)
	}

	// Fetch provisioned number
	req = httptest.NewRequest(http.MethodGet, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers/"+sid+".json", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching number, got %d: %s", rec.Code, rec.Body.String())
	}

	// List provisioned numbers
	req = httptest.NewRequest(http.MethodGet, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers.json", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK listing numbers, got %d", rec.Code)
	}

	// Release number
	req = httptest.NewRequest(http.MethodDelete, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers/"+sid+".json", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", rec.Code)
	}
}

func TestMessagesHTTP(t *testing.T) {
	e, _ := setupTestServer()

	// Provision sender number first
	form := url.Values{"PhoneNumber": {"+14155550199"}}
	req := httptest.NewRequest(http.MethodPost, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers.json", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Send SMS
	msgForm := url.Values{
		"To":   {"+14155559999"},
		"From": {"+14155550199"},
		"Body": {"Hello from automated test!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/2010-04-01/Accounts/AC001/Messages.json", strings.NewReader(msgForm.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on Send SMS, got %d: %s", rec.Code, rec.Body.String())
	}

	var msgResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &msgResp)
	sid, _ := msgResp["sid"].(string)
	if sid == "" || !strings.HasPrefix(sid, "SM") {
		t.Fatalf("expected SM SID prefix, got %v", sid)
	}
	if msgResp["status"] != "queued" {
		t.Errorf("expected status queued, got %v", msgResp["status"])
	}
	if msgResp["num_segments"] != "1" {
		t.Errorf("expected num_segments string '1', got %v", msgResp["num_segments"])
	}

	// Fetch message
	req = httptest.NewRequest(http.MethodGet, "/2010-04-01/Accounts/AC001/Messages/"+sid+".json", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching message, got %d", rec.Code)
	}

	// List messages
	req = httptest.NewRequest(http.MethodGet, "/2010-04-01/Accounts/AC001/Messages.json", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK listing messages, got %d", rec.Code)
	}
}

func TestDeveloperControlHTTP(t *testing.T) {
	e, _ := setupTestServer()

	// Provision number
	form := url.Values{"PhoneNumber": {"+14155550199"}}
	req := httptest.NewRequest(http.MethodPost, "/2010-04-01/Accounts/AC001/IncomingPhoneNumbers.json", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Simulate inbound SMS
	inboundJSON := `{"from":"+14155558888","to":"+14155550199","body":"STOP"}`
	req = httptest.NewRequest(http.MethodPost, "/emu/inbound/sms", strings.NewReader(inboundJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for emu inbound SMS, got %d: %s", rec.Code, rec.Body.String())
	}

	// Inspect messages stream
	req = httptest.NewRequest(http.MethodGet, "/emu/messages?account_sid=AC001", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for emu messages, got %d", rec.Code)
	}
}
