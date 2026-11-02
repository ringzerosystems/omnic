package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/service"
)

func (h *Handler) registerTelegramRoutes(e *echo.Echo) {
	g := e.Group("/bot:token", h.requireTelegramAuth)
	g.GET("/getMe", h.handleTelegramGetMe)
	g.POST("/getMe", h.handleTelegramGetMe)
	g.POST("/sendMessage", h.handleTelegramSendMessage)
	g.POST("/sendVoice", h.handleTelegramSendVoice)
	g.POST("/sendAudio", h.handleTelegramSendAudio)
	g.POST("/sendPhoto", h.handleTelegramSendPhoto)
	g.POST("/sendChatAction", h.handleTelegramSendChatAction)
	g.GET("/sendChatAction", h.handleTelegramSendChatAction)
	g.POST("/answerCallbackQuery", h.handleTelegramAnswerCallbackQuery)
	g.POST("/setWebhook", h.handleTelegramSetWebhook)
	g.POST("/deleteWebhook", h.handleTelegramDeleteWebhook)
	g.GET("/getWebhookInfo", h.handleTelegramGetWebhookInfo)
	g.POST("/getWebhookInfo", h.handleTelegramGetWebhookInfo)
	g.POST("/setMyCommands", h.handleTelegramSetMyCommands)
	g.GET("/getMyCommands", h.handleTelegramGetMyCommands)
	g.POST("/getMyCommands", h.handleTelegramGetMyCommands)
	g.POST("/setChatMenuButton", h.handleTelegramSetChatMenuButton)
	g.GET("/getChatMenuButton", h.handleTelegramGetChatMenuButton)
	g.POST("/getChatMenuButton", h.handleTelegramGetChatMenuButton)
	g.GET("/getFile", h.handleTelegramGetFile)
	g.POST("/getFile", h.handleTelegramGetFile)

	e.GET("/file/bot:token/*", h.handleTelegramFileDownload, h.requireTelegramAuth)
}

func (h *Handler) requireTelegramAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !h.authorizeTelegram(c) {
			return sendTelegramError(c, http.StatusUnauthorized, "Unauthorized: invalid bot token")
		}
		return next(c)
	}
}

func (h *Handler) authorizeTelegram(c echo.Context) bool {
	if h.tgService == nil {
		return false
	}
	cfg := h.tgService.GetConfig()
	token := c.Param("token")
	return cfg.BotToken == "" || token == cfg.BotToken
}

func sendTelegramJSON[T any](c echo.Context, result T) error {
	return c.JSON(http.StatusOK, service.TelegramResponse[T]{
		Ok:     true,
		Result: result,
	})
}

func sendTelegramError(c echo.Context, code int, desc string) error {
	return c.JSON(code, service.TelegramResponse[any]{
		Ok:          false,
		ErrorCode:   code,
		Description: desc,
	})
}

func (h *Handler) handleTelegramGetMe(c echo.Context) error {
	me, err := h.tgService.GetMe(c.Request().Context())
	if err != nil {
		return sendTelegramError(c, http.StatusInternalServerError, err.Error())
	}
	return sendTelegramJSON(c, me)
}

func (h *Handler) handleTelegramSendMessage(c echo.Context) error {
	var req service.SendMessageRequest
	if err := c.Bind(&req); err != nil {
		return sendTelegramError(c, http.StatusBadRequest, "Bad Request: invalid json payload")
	}

	msg, err := h.tgService.SendMessage(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, msg)
}

func (h *Handler) handleTelegramSendVoice(c echo.Context) error {
	var req service.SendVoiceRequest
	_ = c.Bind(&req)

	if file, err := c.FormFile("voice"); err == nil && file != nil {
		src, err := file.Open()
		if err == nil {
			defer func() { _ = src.Close() }()
			data, _ := io.ReadAll(src)
			fileID, err := h.tgService.UploadMedia(c.Request().Context(), "audio/ogg", file.Filename, data)
			if err == nil {
				req.Voice = fileID
			}
		}
	}
	if req.ChatID == nil {
		req.ChatID = c.FormValue("chat_id")
	}
	if req.Caption == "" {
		req.Caption = c.FormValue("caption")
	}
	if req.Duration == 0 {
		dur, _ := strconv.Atoi(c.FormValue("duration"))
		req.Duration = dur
	}

	msg, err := h.tgService.SendVoice(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, msg)
}

func (h *Handler) handleTelegramSendAudio(c echo.Context) error {
	var req service.SendAudioRequest
	_ = c.Bind(&req)

	if file, err := c.FormFile("audio"); err == nil && file != nil {
		src, err := file.Open()
		if err == nil {
			defer func() { _ = src.Close() }()
			data, _ := io.ReadAll(src)
			fileID, err := h.tgService.UploadMedia(c.Request().Context(), "audio/mpeg", file.Filename, data)
			if err == nil {
				req.Audio = fileID
			}
		}
	}
	if req.ChatID == nil {
		req.ChatID = c.FormValue("chat_id")
	}

	msg, err := h.tgService.SendAudio(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, msg)
}

func (h *Handler) handleTelegramSendPhoto(c echo.Context) error {
	var req service.SendPhotoRequest
	_ = c.Bind(&req)

	if file, err := c.FormFile("photo"); err == nil && file != nil {
		src, err := file.Open()
		if err == nil {
			defer func() { _ = src.Close() }()
			data, _ := io.ReadAll(src)
			fileID, err := h.tgService.UploadMedia(c.Request().Context(), "image/jpeg", file.Filename, data)
			if err == nil {
				req.Photo = fileID
			}
		}
	}
	if req.ChatID == nil {
		req.ChatID = c.FormValue("chat_id")
	}

	msg, err := h.tgService.SendPhoto(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, msg)
}

func (h *Handler) handleTelegramSendChatAction(c echo.Context) error {
	var req service.SendChatActionRequest
	_ = c.Bind(&req)
	if req.ChatID == nil {
		if cid := c.FormValue("chat_id"); cid != "" {
			req.ChatID = cid
		} else if cid := c.QueryParam("chat_id"); cid != "" {
			req.ChatID = cid
		}
	}
	if req.Action == "" {
		if act := c.FormValue("action"); act != "" {
			req.Action = act
		} else if act := c.QueryParam("action"); act != "" {
			req.Action = act
		}
	}
	if req.MessageThreadID == 0 {
		if tid, err := strconv.Atoi(c.FormValue("message_thread_id")); err == nil {
			req.MessageThreadID = tid
		} else if tid, err := strconv.Atoi(c.QueryParam("message_thread_id")); err == nil {
			req.MessageThreadID = tid
		}
	}
	if req.BusinessConnectionID == "" {
		if bcid := c.FormValue("business_connection_id"); bcid != "" {
			req.BusinessConnectionID = bcid
		} else if bcid := c.QueryParam("business_connection_id"); bcid != "" {
			req.BusinessConnectionID = bcid
		}
	}

	ok, err := h.tgService.SendChatAction(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, ok)
}

func (h *Handler) handleTelegramAnswerCallbackQuery(c echo.Context) error {
	var req service.AnswerCallbackQueryRequest
	if err := c.Bind(&req); err != nil {
		return sendTelegramError(c, http.StatusBadRequest, "Bad Request: invalid json payload")
	}

	ok, err := h.tgService.AnswerCallbackQuery(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, ok)
}

func (h *Handler) handleTelegramSetWebhook(c echo.Context) error {
	var req service.SetWebhookRequest
	if err := c.Bind(&req); err != nil {
		return sendTelegramError(c, http.StatusBadRequest, "Bad Request: invalid json payload")
	}

	ok, err := h.tgService.SetWebhook(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, ok)
}

func (h *Handler) handleTelegramDeleteWebhook(c echo.Context) error {
	ok, err := h.tgService.DeleteWebhook(c.Request().Context())
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, ok)
}

func (h *Handler) handleTelegramGetWebhookInfo(c echo.Context) error {
	info, err := h.tgService.GetWebhookInfo(c.Request().Context())
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, info)
}

func (h *Handler) handleTelegramSetMyCommands(c echo.Context) error {
	var req service.SetMyCommandsRequest
	if err := c.Bind(&req); err != nil {
		return sendTelegramError(c, http.StatusBadRequest, "Bad Request: invalid json payload")
	}

	ok, err := h.tgService.SetMyCommands(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, ok)
}

func (h *Handler) handleTelegramGetMyCommands(c echo.Context) error {
	cmds, err := h.tgService.GetMyCommands(c.Request().Context())
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, cmds)
}

func (h *Handler) handleTelegramSetChatMenuButton(c echo.Context) error {
	var req service.SetChatMenuButtonRequest
	if err := c.Bind(&req); err != nil {
		return sendTelegramError(c, http.StatusBadRequest, "Bad Request: invalid json payload")
	}

	ok, err := h.tgService.SetChatMenuButton(c.Request().Context(), req)
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, ok)
}

func (h *Handler) handleTelegramGetChatMenuButton(c echo.Context) error {
	mb, err := h.tgService.GetChatMenuButton(c.Request().Context())
	if err != nil {
		return sendTelegramError(c, http.StatusBadRequest, err.Error())
	}
	return sendTelegramJSON(c, mb)
}

func (h *Handler) handleTelegramGetFile(c echo.Context) error {
	fileID := c.QueryParam("file_id")
	if fileID == "" {
		var req struct {
			FileID string `json:"file_id"`
		}
		_ = c.Bind(&req)
		fileID = req.FileID
	}
	if fileID == "" {
		return sendTelegramError(c, http.StatusBadRequest, "Bad Request: file_id is required")
	}

	file, err := h.tgService.GetFile(c.Request().Context(), fileID)
	if err != nil {
		return sendTelegramError(c, http.StatusNotFound, "Bad Request: file not found")
	}
	return sendTelegramJSON(c, file)
}

func (h *Handler) handleTelegramFileDownload(c echo.Context) error {
	path := c.Param("*")
	base := filepath.Base(path)
	fileID := strings.TrimSuffix(base, filepath.Ext(base))

	media, err := h.tgService.GetMedia(c.Request().Context(), fileID)
	if err != nil || media == nil {
		media, err = h.tgService.GetMedia(c.Request().Context(), path)
	}
	if err != nil || media == nil {
		return c.String(http.StatusNotFound, "file not found\n")
	}

	ct := media.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return c.Blob(http.StatusOK, ct, media.Data)
}
