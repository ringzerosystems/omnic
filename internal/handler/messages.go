package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/service"
)

func (h *Handler) registerMessageRoutes(e *echo.Echo) {
	e.POST("/2010-04-01/Accounts/:account_sid/Messages", h.handleCreateMessage)
	e.POST("/2010-04-01/Accounts/:account_sid/Messages.json", h.handleCreateMessage)

	e.GET("/2010-04-01/Accounts/:account_sid/Messages", h.handleListMessages)
	e.GET("/2010-04-01/Accounts/:account_sid/Messages.json", h.handleListMessages)

	e.GET("/2010-04-01/Accounts/:account_sid/Messages/:sid", h.handleGetMessage)
	e.POST("/2010-04-01/Accounts/:account_sid/Messages/:sid", h.handleUpdateMessage)
	e.DELETE("/2010-04-01/Accounts/:account_sid/Messages/:sid", h.handleDeleteMessage)

	e.POST("/2010-04-01/Accounts/:account_sid/Messages/:sid/Feedback", h.handleFeedback)
	e.POST("/2010-04-01/Accounts/:account_sid/Messages/:sid/Feedback.json", h.handleFeedback)
}

func (h *Handler) handleCreateMessage(c echo.Context) error {
	rc := NewReqContext(c)
	_ = c.Request().ParseForm()

	to := c.FormValue("To")
	from := c.FormValue("From")
	msgSvcSID := c.FormValue("MessagingServiceSid")

	if from == "" && msgSvcSID != "" {
		if selected, err := h.svcService.SelectSender(rc.Context(), msgSvcSID, to); err == nil {
			from = selected
		}
	}

	formParams := make(map[string]any)
	for k, v := range c.Request().Form {
		if len(v) == 1 {
			formParams[k] = v[0]
		} else {
			formParams[k] = v
		}
	}

	mediaURLs := c.Request().Form["MediaUrl"]

	msg, err := h.msgService.Create(rc.Context(), rc.AccountSID(), service.CreateMessageParams{
		To:                  to,
		From:                from,
		MessagingServiceSID: msgSvcSID,
		Body:                c.FormValue("Body"),
		ContentSID:          c.FormValue("ContentSid"),
		MediaURLs:           mediaURLs,
		StatusCallback:      c.FormValue("StatusCallback"),
		ScheduleType:        c.FormValue("ScheduleType"),
		Properties:          formParams,
	})
	if err != nil {
		return rc.Error(err)
	}

	if msg.Status == domain.StatusQueued {
		h.dispatcher.EnqueueOutboundDelivery(msg.AccountSID, msg.SID)
	}
	return c.JSON(http.StatusCreated, NewMessageView(msg))
}

func (h *Handler) handleListMessages(c echo.Context) error {
	rc := NewReqContext(c)
	pageSize := rc.PageSize(50)
	page := rc.Page()
	offset := rc.Offset()

	list, total, err := h.store.ListMessages(rc.Context(), rc.AccountSID(), domain.MessageFilter{
		To:     rc.Query("To"),
		From:   rc.Query("From"),
		Limit:  pageSize,
		Offset: offset,
	})
	if err != nil {
		return rc.Error(err)
	}

	formatted := make([]MessageView, 0, len(list))
	for _, m := range list {
		formatted = append(formatted, NewMessageView(m))
	}

	uri := fmt.Sprintf("/2010-04-01/Accounts/%s/Messages.json?PageSize=%d&Page=%d", rc.AccountSID(), pageSize, page)
	return c.JSON(http.StatusOK, MessageListPageView{
		FirstPageURI: uri,
		End:          len(formatted),
		Messages:     formatted,
		URI:          uri,
		PageSize:     pageSize,
		Start:        offset,
		Page:         page,
		Total:        total,
	})
}

func (h *Handler) handleGetMessage(c echo.Context) error {
	rc := NewReqContext(c)
	msg, err := h.store.GetMessageBySID(rc.Context(), rc.AccountSID(), rc.CleanParam("sid"))
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, NewMessageView(msg))
}

func (h *Handler) handleUpdateMessage(c echo.Context) error {
	rc := NewReqContext(c)
	var status *domain.MessageStatus
	if val := c.FormValue("Status"); val != "" {
		st := domain.MessageStatus(strings.ToLower(val))
		status = &st
	}
	var body *string
	if val := c.FormValue("Body"); val != "" {
		body = &val
	}

	msg, err := h.msgService.Update(rc.Context(), rc.AccountSID(), rc.CleanParam("sid"), service.UpdateMessageParams{
		Status: status,
		Body:   body,
	})
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, NewMessageView(msg))
}

func (h *Handler) handleDeleteMessage(c echo.Context) error {
	rc := NewReqContext(c)
	if err := h.store.DeleteMessage(rc.Context(), rc.AccountSID(), rc.CleanParam("sid")); err != nil {
		return rc.Error(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleFeedback(c echo.Context) error {
	rc := NewReqContext(c)
	return c.JSON(http.StatusOK, map[string]any{
		"message_sid": rc.CleanParam("sid"),
		"outcome":     c.FormValue("Outcome"),
	})
}
