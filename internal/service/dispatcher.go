package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/repository"
)

// InboundSMSParams describes an incoming SMS or MMS message payload.
type InboundSMSParams struct {
	From      string
	To        string
	Body      string
	MediaURLs []string
}

// WebhookDispatcher orchestrates inbound webhooks and outbound status callbacks.
type WebhookDispatcher struct {
	store                 repository.Store
	broker                *EventBroker
	simulation            *SimulationService
	httpClient            *http.Client
	authToken             string
	defaultUpstreamURL    string
	defaultUpstreamMethod string
	deliveryDelay         time.Duration
	upstreamTimeout       time.Duration
	mu                    sync.RWMutex
}

// NewWebhookDispatcher constructs a new WebhookDispatcher.
func NewWebhookDispatcher(
	store repository.Store,
	authToken string,
) *WebhookDispatcher {
	return &WebhookDispatcher{
		store:                 store,
		authToken:             authToken,
		deliveryDelay:         20 * time.Millisecond,
		upstreamTimeout:       15 * time.Second,
		defaultUpstreamMethod: http.MethodPost,
		httpClient:            SharedWebhookHTTPClient,
	}
}

// SetDeliveryDelay configures the simulated async transition interval.
func (d *WebhookDispatcher) SetDeliveryDelay(delay time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deliveryDelay = delay
}

// SetDefaultUpstream configures default upstream webhook destination.
func (d *WebhookDispatcher) SetDefaultUpstream(u, method string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.defaultUpstreamURL = u
	if method == "" {
		method = http.MethodPost
	}
	d.defaultUpstreamMethod = method
}

// SetUpstreamTimeout configures timeout for upstream webhook dispatches.
func (d *WebhookDispatcher) SetUpstreamTimeout(timeout time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.upstreamTimeout = timeout
	d.httpClient.Timeout = timeout
}

// SetBroker binds an EventBroker for real-time notification dispatching.
func (d *WebhookDispatcher) SetBroker(broker *EventBroker) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.broker = broker
}

// SetSimulation binds a SimulationService for fault/chaos injection.
func (d *WebhookDispatcher) SetSimulation(sim *SimulationService) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.simulation = sim
}

func isOptOutKeyword(k string) bool {
	switch k {
	case "STOP", "STOPALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT":
		return true
	}
	return false
}

func isOptInKeyword(k string) bool {
	switch k {
	case "START", "YES", "UNSTOP":
		return true
	}
	return false
}

func (d *WebhookDispatcher) handleComplianceKeywords(ctx context.Context, pn *domain.PhoneNumber, params InboundSMSParams) (*domain.Message, bool, error) {
	upper := strings.TrimSpace(strings.ToUpper(params.Body))
	var replyText string

	switch {
	case isOptOutKeyword(upper):
		_ = d.store.SetOptOut(ctx, pn.AccountSID, params.From, true)
		replyText = "You have successfully been unsubscribed. You will not receive any more messages."
	case isOptInKeyword(upper):
		_ = d.store.SetOptOut(ctx, pn.AccountSID, params.From, false)
		replyText = "You have successfully been re-subscribed to messages."
	case upper == "HELP", upper == "INFO":
		replyText = "Reply STOP to unsubscribe. For support call customer care."
	default:
		return nil, false, nil
	}

	reply := &domain.Message{
		SID:         GenerateSID("SM"),
		AccountSID:  pn.AccountSID,
		To:          params.From,
		From:        params.To,
		Body:        replyText,
		Status:      domain.StatusDelivered,
		Direction:   domain.DirectionOutboundReply,
		NumSegments: 1,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}
	_ = d.store.CreateMessage(ctx, reply)
	return reply, true, nil
}

// DispatchInbound delivers an incoming SMS to the provisioned phone number webhook.
func (d *WebhookDispatcher) DispatchInbound(ctx context.Context, params InboundSMSParams) (*domain.Message, error) {
	pn, err := d.store.FindPhoneNumberByNumber(ctx, params.To)
	if err != nil {
		return nil, ErrNotFound(fmt.Sprintf("Incoming number %s is not provisioned", params.To))
	}

	if reply, handled, err := d.handleComplianceKeywords(ctx, pn, params); handled || err != nil {
		return reply, err
	}

	d.mu.RLock()
	sim := d.simulation
	d.mu.RUnlock()
	if sim != nil {
		if sim.ShouldTimeout("webhook", params.To, params.From) {
			return nil, ErrWebhookFailed("simulated upstream timeout")
		}
		if simErr := sim.Evaluate("webhook", params.To, params.From); simErr != nil {
			return nil, simErr
		}
	}

	inboundSID := GenerateSID("SM")
	isMMS := len(params.MediaURLs) > 0
	if isMMS {
		inboundSID = GenerateSID("MM")
	}
	numSegments, _ := domain.CalculateSegments(params.Body, isMMS)
	now := time.Now().UTC()
	inboundMsg := &domain.Message{
		SID:         inboundSID,
		AccountSID:  pn.AccountSID,
		To:          params.To,
		From:        params.From,
		Body:        params.Body,
		Status:      domain.StatusReceived,
		Direction:   domain.DirectionInbound,
		NumSegments: numSegments,
		NumMedia:    len(params.MediaURLs),
		MediaURLs:   params.MediaURLs,
		DateCreated: now,
		DateUpdated: now,
	}
	_ = d.store.CreateMessage(ctx, inboundMsg)
	d.publishEvent(inboundMsg.SID, EventMessageInbound, inboundMsg.DateCreated, inboundMsg)

	targetURL, targetMethod := d.resolveUpstream(pn)
	if targetURL == "" {
		return nil, nil
	}
	return d.postInboundWebhook(ctx, targetURL, targetMethod, pn, inboundMsg)
}

func (d *WebhookDispatcher) resolveUpstream(pn *domain.PhoneNumber) (string, string) {
	if pn.SmsURL != "" {
		method := pn.SmsMethod
		if method == "" {
			method = http.MethodPost
		}
		return pn.SmsURL, method
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.defaultUpstreamURL != "" {
		method := d.defaultUpstreamMethod
		if method == "" {
			method = http.MethodPost
		}
		return d.defaultUpstreamURL, method
	}
	return "", ""
}

func buildInboundForm(pn *domain.PhoneNumber, msg *domain.Message) (url.Values, map[string]string) {
	formVals := url.Values{
		"MessageSid":    {msg.SID},
		"SmsSid":        {msg.SID},
		"SmsMessageSid": {msg.SID},
		"AccountSid":    {pn.AccountSID},
		"From":          {msg.From},
		"To":            {msg.To},
		"Body":          {msg.Body},
		"NumSegments":   {strconv.Itoa(msg.NumSegments)},
		"NumMedia":      {strconv.Itoa(msg.NumMedia)},
		"SmsStatus":     {"received"},
		"ApiVersion":    {"2010-04-01"},
		"FromCity":      {"San Francisco"},
		"FromState":     {"CA"},
		"FromCountry":   {"US"},
		"ToCity":        {"San Francisco"},
		"ToState":       {"CA"},
		"ToCountry":     {"US"},
	}
	for i, u := range msg.MediaURLs {
		formVals.Set(fmt.Sprintf("MediaUrl%d", i), u)
		formVals.Set(fmt.Sprintf("MediaContentType%d", i), "application/octet-stream")
	}
	paramMap := make(map[string]string, len(formVals))
	for k := range formVals {
		paramMap[k] = formVals.Get(k)
	}
	return formVals, paramMap
}

func createInboundRequest(ctx context.Context, targetURL, targetMethod, sig string, form url.Values) (*http.Request, error) {
	reqURL := targetURL
	var bodyReader io.Reader
	if strings.EqualFold(targetMethod, http.MethodGet) {
		parsed, err := url.Parse(targetURL)
		if err != nil {
			return nil, err
		}
		q := parsed.Query()
		for k, v := range form {
			for _, val := range v {
				q.Add(k, val)
			}
		}
		parsed.RawQuery = q.Encode()
		reqURL = parsed.String()
	} else {
		bodyReader = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, targetMethod, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(targetMethod, http.MethodGet) {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	}
	req.Header.Set("User-Agent", "TwilioProxy/1.1 (omnic)")
	req.Header.Set("X-Twilio-Signature", sig)
	return req, nil
}

func (d *WebhookDispatcher) postInboundWebhook(
	ctx context.Context,
	targetURL, targetMethod string,
	pn *domain.PhoneNumber,
	msg *domain.Message,
) (*domain.Message, error) {
	formVals, paramMap := buildInboundForm(pn, msg)
	sig := CalculateSignature(targetURL, paramMap, d.authToken)

	req, err := createInboundRequest(ctx, targetURL, targetMethod, sig, formVals)
	if err != nil {
		return nil, ErrWebhookFailed(err.Error())
	}

	resp, err := d.httpClient.Do(req)
	if err != nil || resp == nil {
		if err != nil {
			return nil, ErrWebhookFailed(err.Error())
		}
		return nil, ErrWebhookFailed("upstream returned nil response")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, ErrWebhookFailed(fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, string(b)))
	}

	if resp.StatusCode == http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		if twiResp, err := ParseTwiML(respBytes); err == nil && len(twiResp.Messages) > 0 {
			return d.createTwiMLReply(ctx, pn, msg, twiResp.Messages[0])
		}
	}
	return nil, nil
}

func (d *WebhookDispatcher) createTwiMLReply(
	ctx context.Context,
	pn *domain.PhoneNumber,
	msg *domain.Message,
	tMsg TwiMLMessage,
) (*domain.Message, error) {
	reply := &domain.Message{
		SID:         GenerateSID("SM"),
		AccountSID:  pn.AccountSID,
		To:          msg.From,
		From:        msg.To,
		Body:        tMsg.Body,
		Status:      domain.StatusDelivered,
		Direction:   domain.DirectionOutboundReply,
		NumSegments: 1,
		DateCreated: time.Now().UTC(),
		DateUpdated: time.Now().UTC(),
	}
	if err := d.store.CreateMessage(ctx, reply); err != nil {
		return nil, err
	}
	d.publishEvent(reply.SID, EventMessageOutbound, reply.DateCreated, reply)
	return reply, nil
}

func (d *WebhookDispatcher) publishEvent(id string, eventType EventType, t time.Time, msg *domain.Message) {
	d.mu.RLock()
	broker := d.broker
	d.mu.RUnlock()
	if broker != nil {
		broker.Publish(Event{
			ID:        id,
			Type:      eventType,
			Timestamp: t,
			Payload:   msg,
		})
	}
}

// EnqueueOutboundDelivery starts asynchronous delivery transitions and status callbacks.
func (d *WebhookDispatcher) EnqueueOutboundDelivery(accountSID, messageSID string) {
	go func() {
		d.mu.RLock()
		delay := d.deliveryDelay
		d.mu.RUnlock()

		ctx := context.Background()
		time.Sleep(delay)
		msg, err := d.store.GetMessageBySID(ctx, accountSID, messageSID)
		if err != nil || msg == nil {
			return
		}
		msg.Status = domain.StatusSending
		msg.DateUpdated = time.Now().UTC()
		_ = d.store.UpdateMessage(ctx, msg)
		d.publishEvent(msg.SID, EventMessageStatus, msg.DateUpdated, msg)
		d.dispatchStatusCallback(ctx, msg)

		time.Sleep(delay)
		msg, err = d.store.GetMessageBySID(ctx, accountSID, messageSID)
		if err != nil || msg == nil {
			return
		}
		msg.Status = domain.StatusDelivered
		now := time.Now().UTC()
		msg.DateSent = &now
		msg.DateUpdated = now
		_ = d.store.UpdateMessage(ctx, msg)
		d.publishEvent(msg.SID, EventMessageStatus, msg.DateUpdated, msg)
		d.dispatchStatusCallback(ctx, msg)
	}()
}

func (d *WebhookDispatcher) dispatchStatusCallback(ctx context.Context, msg *domain.Message) {
	if msg.StatusCallback == "" {
		return
	}
	form := url.Values{
		"MessageSid":    {msg.SID},
		"MessageStatus": {string(msg.Status)},
		"SmsSid":        {msg.SID},
		"SmsStatus":     {string(msg.Status)},
		"AccountSid":    {msg.AccountSID},
		"From":          {msg.From},
		"To":            {msg.To},
	}
	paramMap := make(map[string]string)
	for k := range form {
		paramMap[k] = form.Get(k)
	}
	sig := CalculateSignature(msg.StatusCallback, paramMap, d.authToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, msg.StatusCallback, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", sig)

	resp, err := d.httpClient.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}
