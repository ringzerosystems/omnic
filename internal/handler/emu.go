package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/service"
)

// InboundRequest represents the JSON payload to simulate an inbound SMS.
type InboundRequest struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Body      string   `json:"body"`
	MediaURLs []string `json:"media_urls"`
}

// WhatsAppInboundRequest represents the JSON payload to simulate an inbound WhatsApp message.
type WhatsAppInboundRequest struct {
	From            string  `json:"from"`
	To              string  `json:"to"`
	Type            string  `json:"type"`
	Body            string  `json:"body"`
	MediaID         string  `json:"media_id"`
	MediaURL        string  `json:"media_url"`
	InteractiveType string  `json:"interactive_type"`
	SelectedID      string  `json:"selected_id"`
	SelectedTitle   string  `json:"selected_title"`
	SelectedDesc    string  `json:"selected_description"`
	DurationSeconds float64 `json:"duration_seconds"`
	Voice           bool    `json:"voice"`
}

// TelegramInboundRequest represents the JSON payload to simulate an inbound Telegram event.
type TelegramInboundRequest struct {
	Username        string  `json:"username"`
	FirstName       string  `json:"first_name"`
	Text            string  `json:"text"`
	Type            string  `json:"type"`
	MediaID         string  `json:"media_id"`
	CallbackQueryID string  `json:"callback_query_id"`
	CallbackData    string  `json:"callback_data"`
	FromID          int64   `json:"from_id"`
	ChatID          int64   `json:"chat_id"`
	DurationSeconds float64 `json:"duration_seconds"`
	MessageID       int     `json:"message_id"`
	Voice           bool    `json:"voice"`
}

func (h *Handler) registerEmuRoutes(e *echo.Echo) {
	e.POST("/emu/inbound/sms", h.handleEmuInboundSMS)
	e.POST("/emu/inbound/whatsapp", h.handleEmuInboundWhatsApp)
	e.POST("/emu/inbound/telegram", h.handleEmuInboundTelegram)
	e.GET("/emu/messages", h.handleEmuMessages)
	e.GET("/emu/events", h.handleEmuEvents)
	e.POST("/emu/numbers/seed", h.handleEmuSeedNumbers)
	e.GET("/emu/simulate/rules", h.handleGetSimulateRules)
	e.POST("/emu/simulate/rules", h.handleSetSimulateRules)
	e.DELETE("/emu/simulate/rules", h.handleClearSimulateRules)
}

func (h *Handler) handleEmuInboundSMS(c echo.Context) error {
	var req InboundRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid json body"})
	}

	reply, err := h.dispatcher.DispatchInbound(c.Request().Context(), service.InboundSMSParams{
		From:      req.From,
		To:        req.To,
		Body:      req.Body,
		MediaURLs: req.MediaURLs,
	})
	if err != nil {
		return handleError(c, err)
	}

	res := map[string]any{"status": "delivered_to_webhook"}
	if reply != nil {
		res["reply"] = NewMessageView(reply)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Handler) handleEmuInboundWhatsApp(c echo.Context) error {
	if h.waService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "whatsapp service unavailable"})
	}

	var req WhatsAppInboundRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid json body"})
	}

	reply, err := h.waService.DispatchInbound(c.Request().Context(), service.InboundWhatsAppParams{
		From:            req.From,
		To:              req.To,
		Type:            req.Type,
		Body:            req.Body,
		MediaID:         req.MediaID,
		MediaURL:        req.MediaURL,
		Voice:           req.Voice,
		DurationSeconds: req.DurationSeconds,
		InteractiveType: domain.InteractiveType(req.InteractiveType),
		SelectedID:      req.SelectedID,
		SelectedTitle:   req.SelectedTitle,
		SelectedDesc:    req.SelectedDesc,
	})
	if err != nil {
		return handleError(c, err)
	}

	res := map[string]any{"status": "delivered_to_webhook"}
	if reply != nil {
		res["reply"] = NewMessageView(reply)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Handler) handleEmuInboundTelegram(c echo.Context) error {
	if h.tgService == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "telegram service unavailable"})
	}

	var req TelegramInboundRequest
	_ = c.Bind(&req)

	if req.FromID == 0 {
		if val := c.FormValue("from_id"); val != "" {
			req.FromID, _ = strconv.ParseInt(val, 10, 64)
		}
	}
	if req.ChatID == 0 {
		if val := c.FormValue("chat_id"); val != "" {
			req.ChatID, _ = strconv.ParseInt(val, 10, 64)
		}
	}
	if req.Text == "" {
		req.Text = c.FormValue("text")
	}
	if req.Username == "" {
		req.Username = c.FormValue("username")
	}
	if req.FirstName == "" {
		req.FirstName = c.FormValue("first_name")
	}
	if req.CallbackData == "" {
		req.CallbackData = c.FormValue("callback_data")
	}
	if req.CallbackQueryID == "" {
		req.CallbackQueryID = c.FormValue("callback_query_id")
	}
	if !req.Voice && c.FormValue("voice") == "true" {
		req.Voice = true
	}
	if req.DurationSeconds == 0 {
		if dur, err := strconv.ParseFloat(c.FormValue("duration_seconds"), 64); err == nil {
			req.DurationSeconds = dur
		}
	}

	if file, err := c.FormFile("voice"); err == nil && file != nil {
		src, err := file.Open()
		if err == nil {
			defer func() { _ = src.Close() }()
			data, _ := io.ReadAll(src)
			mediaID, err := h.tgService.UploadMedia(c.Request().Context(), "audio/ogg", file.Filename, data)
			if err == nil {
				req.MediaID = mediaID
				req.Voice = true
			}
		}
	}

	msg, err := h.tgService.DispatchInbound(c.Request().Context(), service.InboundTelegramParams{
		FromID:          req.FromID,
		ChatID:          req.ChatID,
		Username:        req.Username,
		FirstName:       req.FirstName,
		Text:            req.Text,
		Type:            req.Type,
		Voice:           req.Voice,
		DurationSeconds: req.DurationSeconds,
		MediaID:         req.MediaID,
		CallbackQueryID: req.CallbackQueryID,
		CallbackData:    req.CallbackData,
		MessageID:       req.MessageID,
	})
	if err != nil {
		return handleError(c, err)
	}

	res := map[string]any{"status": "delivered_to_webhook"}
	if msg != nil {
		res["reply"] = NewMessageView(msg)
	}
	return c.JSON(http.StatusOK, res)
}

func (h *Handler) handleEmuMessages(c echo.Context) error {
	accountSID := c.QueryParam("account_sid")
	filterEngine := domain.Engine(c.QueryParam("engine"))
	messages, total, err := h.store.ListMessages(c.Request().Context(), accountSID, domain.MessageFilter{
		Engine: filterEngine,
		Limit:  100,
	})
	if err != nil {
		return handleError(c, err)
	}

	formatted := make([]MessageView, 0, len(messages))
	for _, m := range messages {
		formatted = append(formatted, NewMessageView(m))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"messages": formatted,
		"total":    total,
	})
}

func (h *Handler) handleEmuSeedNumbers(c echo.Context) error {
	accountSID := c.QueryParam("account_sid")
	if accountSID == "" {
		accountSID = "AC001"
	}
	smsURL := c.QueryParam("sms_url")
	if smsURL == "" {
		smsURL = "http://localhost:3000/sms"
	}

	numbers := []string{"+14155550100", "+14155550199", "+12125550100"}
	seeded := make([]PhoneNumberView, 0, len(numbers))
	for _, num := range numbers {
		pn, err := h.phoneService.Provision(c.Request().Context(), accountSID, service.ProvisionParams{
			PhoneNumber:  num,
			FriendlyName: "Demo " + num,
			SmsURL:       smsURL,
			SmsMethod:    "POST",
		})
		if err == nil {
			seeded = append(seeded, NewPhoneNumberView(pn))
		}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"seeded": seeded,
	})
}

func (h *Handler) handleEmuEvents(c echo.Context) error {
	if h.broker == nil {
		return c.String(http.StatusServiceUnavailable, "Event broker unavailable\n")
	}

	filterNumber := c.QueryParam("number")
	if strings.HasPrefix(filterNumber, " ") {
		filterNumber = "+" + strings.TrimSpace(filterNumber)
	}
	filterAccount := c.QueryParam("account_sid")
	filterEngine := c.QueryParam("engine")

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	w.Flush()

	ctx := c.Request().Context()
	events, unsub := h.broker.Subscribe(ctx, filterNumber, filterAccount, filterEngine)
	defer unsub()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return nil
			}
			w.Flush()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if err := writeSSEEvent(w, ev); err != nil {
				return nil
			}
			w.Flush()
		}
	}
}

func writeSSEEvent(w io.Writer, ev service.Event) error {
	var payloadData any = ev.Payload
	if ev.Payload != nil {
		payloadData = NewMessageView(ev.Payload)
	}
	b, err := json.Marshal(payloadData)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\nid: %s\ndata: %s\n\n", ev.Type, ev.ID, string(b))
	return err
}

func (h *Handler) handleGetSimulateRules(c echo.Context) error {
	if h.simulation == nil {
		return c.JSON(http.StatusOK, map[string]any{"rules": []service.FailRule{}})
	}
	return c.JSON(http.StatusOK, map[string]any{"rules": h.simulation.GetRules()})
}

func (h *Handler) handleSetSimulateRules(c echo.Context) error {
	if h.simulation == nil {
		return c.String(http.StatusServiceUnavailable, "Simulation service unavailable\n")
	}
	var req struct {
		Rules []service.FailRule `json:"rules"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid json body"})
	}
	h.simulation.SetRules(req.Rules)
	return c.JSON(http.StatusOK, map[string]any{"rules": h.simulation.GetRules()})
}

func (h *Handler) handleClearSimulateRules(c echo.Context) error {
	if h.simulation != nil {
		h.simulation.ClearRules()
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "rules cleared"})
}
