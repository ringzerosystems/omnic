package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/handler"
	"github.com/ringzerosystems/omnic/internal/repository/memory"
	"github.com/ringzerosystems/omnic/internal/service"
)

type mockAgentState struct {
	inboundBody     string
	inboundSigValid bool
	statusDelivered bool
	statusSigValid  bool
	mu              sync.Mutex
}

func setupEmuServer(authToken string) (*httptest.Server, *handler.Handler, *service.PhoneNumberService) {
	store := memory.NewStore()

	phoneSvc := service.NewPhoneNumberService(store)
	msgSvc := service.NewMessageService(store)
	svcSvc := service.NewMessagingServiceService(store)
	dispatcher := service.NewWebhookDispatcher(store, authToken)
	dispatcher.SetDeliveryDelay(20 * time.Millisecond)

	broker := service.NewMemoryEventBroker()
	msgSvc.SetBroker(broker)
	dispatcher.SetBroker(broker)

	h := handler.NewHandler(store, phoneSvc, msgSvc, svcSvc, dispatcher)
	h.SetBroker(broker)

	e := echo.New()
	h.RegisterRoutes(e)

	srv := httptest.NewServer(e)
	return srv, h, phoneSvc
}

func setupMockAgent(state *mockAgentState, emuURL, authToken string) *httptest.Server {
	var agentSrv *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/api/sms/inbound", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		params := make(map[string]string)
		for k := range r.Form {
			params[k] = r.Form.Get(k)
		}
		sig := r.Header.Get("X-Twilio-Signature")
		valid := service.ValidateSignature(agentSrv.URL+"/api/sms/inbound", params, authToken, sig)

		state.mu.Lock()
		state.inboundBody = r.FormValue("Body")
		state.inboundSigValid = valid
		userNumber := r.FormValue("From")
		agentNumber := r.FormValue("To")
		statusURL := agentSrv.URL + "/api/sms/status"
		state.mu.Unlock()

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("<Response/>"))

		go func() {
			time.Sleep(30 * time.Millisecond)
			sendOutboundReply(emuURL, agentNumber, userNumber, statusURL)
		}()
	})

	mux.HandleFunc("/api/sms/status", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		params := make(map[string]string)
		for k := range r.Form {
			params[k] = r.Form.Get(k)
		}
		sig := r.Header.Get("X-Twilio-Signature")
		valid := service.ValidateSignature(agentSrv.URL+"/api/sms/status", params, authToken, sig)

		state.mu.Lock()
		if r.FormValue("MessageStatus") == "delivered" {
			state.statusDelivered = true
		}
		state.statusSigValid = valid
		state.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	agentSrv = httptest.NewServer(mux)
	return agentSrv
}

func sendOutboundReply(emuURL, from, to, statusCallback string) {
	form := url.Values{
		"From":           {from},
		"To":             {to},
		"Body":           {"Package #9876 shipped yesterday, arrives Thursday."},
		"StatusCallback": {statusCallback},
	}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/AC001/Messages.json", emuURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return
	}
	_ = resp.Body.Close()
}

func readSSEEvents(resp *http.Response, targetText string, timeout time.Duration) bool {
	if resp == nil || resp.Body == nil {
		return false
	}
	deadline := time.After(timeout)
	scanner := bufio.NewScanner(resp.Body)
	ch := make(chan string, 10)

	go func() {
		for scanner.Scan() {
			ch <- scanner.Text()
		}
	}()

	for {
		select {
		case line := <-ch:
			if strings.Contains(line, targetText) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func TestTripartiteAgentLoop(t *testing.T) {
	authToken := "secret-agent-token"
	emuSrv, _, phoneSvc := setupEmuServer(authToken)
	defer emuSrv.Close()

	var state mockAgentState
	agentSrv := setupMockAgent(&state, emuSrv.URL, authToken)
	defer agentSrv.Close()

	// Provision phone number pointing to mock agent webhook
	ctx := context.Background()
	_, _ = phoneSvc.Provision(ctx, "AC001", service.ProvisionParams{
		PhoneNumber:  "+14155550199",
		FriendlyName: "AI Support Number",
		SmsURL:       agentSrv.URL + "/api/sms/inbound",
		SmsMethod:    "POST",
	})

	// Handset connects to SSE stream
	sseReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, emuSrv.URL+"/emu/events?number=+15551234567", nil)
	client := &http.Client{Timeout: 0}
	sseResp, err := client.Do(sseReq)
	if err != nil || sseResp == nil {
		t.Fatalf("SSE connection failed: %v", err)
	}
	defer func() { _ = sseResp.Body.Close() }()

	// Send inbound SMS from handset to agent
	inboundPayload, _ := json.Marshal(map[string]any{
		"from": "+15551234567",
		"to":   "+14155550199",
		"body": "Where is my package?",
	})
	sendResp, err := http.Post(emuSrv.URL+"/emu/inbound/sms", "application/json", bytes.NewBuffer(inboundPayload))
	if err != nil || sendResp == nil {
		t.Fatalf("inbound SMS send failed: %v", err)
	}
	if sendResp.StatusCode != http.StatusOK {
		_ = sendResp.Body.Close()
		t.Fatalf("inbound SMS send returned non-200: %d", sendResp.StatusCode)
	}
	_ = sendResp.Body.Close()

	// Handset SSE must receive agent's asynchronous reply
	foundReply := readSSEEvents(sseResp, "Package #9876 shipped yesterday", 2*time.Second)
	if !foundReply {
		t.Fatal("Handset did not receive async AI agent reply over SSE within timeout")
	}

	// Verify state in mock agent
	time.Sleep(100 * time.Millisecond)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.inboundBody != "Where is my package?" {
		t.Errorf("expected inbound body 'Where is my package?', got %q", state.inboundBody)
	}
	if !state.inboundSigValid {
		t.Errorf("expected valid HMAC-SHA1 X-Twilio-Signature on inbound webhook")
	}
	if !state.statusDelivered {
		t.Errorf("expected mock agent to receive delivered StatusCallback")
	}
	if !state.statusSigValid {
		t.Errorf("expected valid HMAC-SHA1 X-Twilio-Signature on StatusCallback")
	}
}

func TestEchoServerLoop(t *testing.T) {
	authToken := "secret-agent-token"
	emuSrv, _, phoneSvc := setupEmuServer(authToken)
	defer emuSrv.Close()

	echoMux := http.NewServeMux()
	echoMux.HandleFunc("/api/sms/inbound", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		body := html.EscapeString(r.FormValue("Body"))
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<Response><Message><Body>You said, "%s"</Body></Message></Response>`, body)
	})
	echoSrv := httptest.NewServer(echoMux)
	defer echoSrv.Close()

	ctx := context.Background()
	_, _ = phoneSvc.Provision(ctx, "AC001", service.ProvisionParams{
		PhoneNumber:  "+14155550199",
		FriendlyName: "Echo Server Number",
		SmsURL:       echoSrv.URL + "/api/sms/inbound",
		SmsMethod:    "POST",
	})

	inboundPayload, _ := json.Marshal(map[string]any{
		"from": "+15551234567",
		"to":   "+14155550199",
		"body": "hello, how are you?",
	})
	sendResp, err := http.Post(emuSrv.URL+"/emu/inbound/sms", "application/json", bytes.NewBuffer(inboundPayload))
	if err != nil || sendResp == nil {
		t.Fatalf("inbound SMS send failed: %v", err)
	}
	defer func() { _ = sendResp.Body.Close() }()

	var result struct {
		Reply struct {
			Body string `json:"body"`
		} `json:"reply"`
	}
	if err := json.NewDecoder(sendResp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expected := `You said, "hello, how are you?"`
	if result.Reply.Body != expected {
		t.Errorf("expected echo reply %q, got %q", expected, result.Reply.Body)
	}
}
