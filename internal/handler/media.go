package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func (h *Handler) registerMediaRoutes(e *echo.Echo) {
	e.GET("/2010-04-01/Accounts/:account_sid/Messages/:message_sid/Media", h.handleListMedia)
	e.GET("/2010-04-01/Accounts/:account_sid/Messages/:message_sid/Media.json", h.handleListMedia)

	e.GET("/2010-04-01/Accounts/:account_sid/Messages/:message_sid/Media/:media_sid", h.handleGetMedia)
	e.DELETE("/2010-04-01/Accounts/:account_sid/Messages/:message_sid/Media/:media_sid", h.handleDeleteMedia)
}

func (h *Handler) handleListMedia(c echo.Context) error {
	rc := NewReqContext(c)
	accountSID := rc.AccountSID()
	messageSID := rc.CleanParam("message_sid")

	list, err := h.store.ListMedia(rc.Context(), accountSID, messageSID)
	if err != nil {
		return rc.Error(err)
	}

	formatted := make([]MediaView, 0, len(list))
	for _, m := range list {
		formatted = append(formatted, NewMediaView(m))
	}

	uri := fmt.Sprintf("/2010-04-01/Accounts/%s/Messages/%s/Media.json", accountSID, messageSID)
	return c.JSON(http.StatusOK, MediaListPageView{
		MediaList: formatted,
		URI:       uri,
	})
}

func (h *Handler) handleGetMedia(c echo.Context) error {
	rc := NewReqContext(c)
	rawSID := c.Param("media_sid")
	mediaSID := rc.CleanParam("media_sid")

	media, err := h.store.GetMediaBySID(rc.Context(), rc.AccountSID(), rc.CleanParam("message_sid"), mediaSID)
	if err != nil {
		return rc.Error(err)
	}

	if strings.HasSuffix(rawSID, ".json") {
		return c.JSON(http.StatusOK, NewMediaView(media))
	}

	contentType := media.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return c.Blob(http.StatusOK, contentType, media.Data)
}

func (h *Handler) handleDeleteMedia(c echo.Context) error {
	rc := NewReqContext(c)
	if err := h.store.DeleteMedia(rc.Context(), rc.AccountSID(), rc.CleanParam("message_sid"), rc.CleanParam("media_sid")); err != nil {
		return rc.Error(err)
	}
	return c.NoContent(http.StatusNoContent)
}
