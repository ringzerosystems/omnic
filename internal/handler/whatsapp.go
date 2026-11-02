package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/service"
)

func (h *Handler) registerWhatsAppRoutes(e *echo.Echo) {
	wa := e.Group("", h.requireWhatsAppAuth)
	wa.POST("/:version/:phone_number_id/messages", h.handleWhatsAppMessages)
	wa.POST("/:phone_number_id/messages", h.handleWhatsAppMessages)

	wa.POST("/:version/:phone_number_id/media", h.handleWhatsAppMediaUpload)
	wa.POST("/:phone_number_id/media", h.handleWhatsAppMediaUpload)

	wa.GET("/:version/media/:media_id/download", h.handleWhatsAppMediaDownload)
	wa.GET("/:version/:media_id/download", h.handleWhatsAppMediaDownload)
	wa.GET("/:version/:media_id", h.handleWhatsAppMediaMetadata)

	e.GET("/webhook", h.handleWhatsAppWebhookVerification)
	e.POST("/webhook", h.handleWhatsAppWebhookIngress)
}

func (h *Handler) requireWhatsAppAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !h.authorizeWhatsApp(c) {
			return sendWhatsAppError(c, http.StatusUnauthorized, "Invalid OAuth access token", 190)
		}
		return next(c)
	}
}

func (h *Handler) authorizeWhatsApp(c echo.Context) bool {
	if h.waService == nil {
		return false
	}
	cfg := h.waService.GetConfig()
	if cfg.BearerToken == "" {
		return true
	}
	auth := c.Request().Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	return token == cfg.BearerToken
}

func sendWhatsAppError(c echo.Context, status int, message string, code int) error {
	return c.JSON(status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "OAuthException",
			"code":    code,
		},
	})
}

func (h *Handler) handleWhatsAppMessages(c echo.Context) error {
	phoneID := c.Param("phone_number_id")
	var req service.OutboundMessageRequest
	if err := c.Bind(&req); err != nil {
		return sendWhatsAppError(c, http.StatusBadRequest, "invalid request body", 100)
	}

	if req.Status != "" {
		if req.Status != "read" {
			return sendWhatsAppError(c, http.StatusBadRequest, "invalid status", 100)
		}
		if req.MessagingProduct != "whatsapp" {
			return sendWhatsAppError(c, http.StatusBadRequest, "invalid messaging_product, expected whatsapp", 100)
		}
		if req.MessageID == "" {
			return sendWhatsAppError(c, http.StatusBadRequest, "message_id is required", 100)
		}
		if err := h.waService.MarkAsRead(c.Request().Context(), req.MessageID); err != nil {
			return sendWhatsAppError(c, http.StatusBadRequest, err.Error(), 131009)
		}
		return c.JSON(http.StatusOK, map[string]bool{"success": true})
	}

	res, err := h.waService.SendOutbound(c.Request().Context(), phoneID, req)
	if err != nil {
		return sendWhatsAppError(c, http.StatusBadRequest, err.Error(), 100)
	}

	return c.JSON(http.StatusOK, res)
}

func (h *Handler) handleWhatsAppMediaUpload(c echo.Context) error {
	phoneID := c.Param("phone_number_id")
	file, err := c.FormFile("file")
	if err != nil {
		return sendWhatsAppError(c, http.StatusBadRequest, "missing file multipart data", 100)
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(src)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	mimeType := file.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	mediaID, err := h.waService.UploadMedia(c.Request().Context(), phoneID, mimeType, file.Filename, data)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"id": mediaID})
}

func (h *Handler) handleWhatsAppMediaMetadata(c echo.Context) error {
	mediaID := c.Param("media_id")
	media, err := h.waService.GetMedia(c.Request().Context(), mediaID)
	if err != nil {
		return sendWhatsAppError(c, http.StatusNotFound, "media not found", 100)
	}

	version := c.Param("version")
	if version == "" {
		version = "v21.0"
	}

	return c.JSON(http.StatusOK, map[string]any{
		"messaging_product": "whatsapp",
		"url":               fmt.Sprintf("/%s/media/%s/download", version, mediaID),
		"mime_type":         media.ContentType,
		"sha256":            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"file_size":         len(media.Data),
		"id":                mediaID,
	})
}

func (h *Handler) handleWhatsAppMediaDownload(c echo.Context) error {
	mediaID := c.Param("media_id")
	media, err := h.waService.GetMedia(c.Request().Context(), mediaID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "media not found"})
	}

	return c.Blob(http.StatusOK, media.ContentType, media.Data)
}

func (h *Handler) handleWhatsAppWebhookVerification(c echo.Context) error {
	if h.waService == nil {
		return c.String(http.StatusServiceUnavailable, "whatsapp service unavailable")
	}

	mode := c.QueryParam("hub.mode")
	challenge := c.QueryParam("hub.challenge")
	verifyToken := c.QueryParam("hub.verify_token")

	cfg := h.waService.GetConfig()
	if mode == "subscribe" && verifyToken == cfg.VerifyToken {
		return c.String(http.StatusOK, challenge)
	}

	return c.String(http.StatusForbidden, "verification failed")
}

func (h *Handler) handleWhatsAppWebhookIngress(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "received"})
}
