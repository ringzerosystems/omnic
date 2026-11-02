package handler

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/repository"
	"github.com/ringzerosystems/omnic/internal/service"
)

// Handler serves Twilio REST API endpoints and emulator control routes.
type Handler struct {
	store        repository.Store
	phoneService *service.PhoneNumberService
	msgService   *service.MessageService
	svcService   *service.MessagingServiceService
	dispatcher   *service.WebhookDispatcher
	broker       *service.EventBroker
	simulation   *service.SimulationService
	waService    *service.WhatsAppService
	tgService    *service.TelegramService
}

// NewHandler constructs a new Handler.
func NewHandler(
	store repository.Store,
	phoneService *service.PhoneNumberService,
	msgService *service.MessageService,
	svcService *service.MessagingServiceService,
	dispatcher *service.WebhookDispatcher,
) *Handler {
	return &Handler{
		store:        store,
		phoneService: phoneService,
		msgService:   msgService,
		svcService:   svcService,
		dispatcher:   dispatcher,
	}
}

// SetBroker sets the event broker for real-time SSE event publishing.
func (h *Handler) SetBroker(broker *service.EventBroker) {
	h.broker = broker
}

// SetSimulation sets the chaos/fault injection simulation service.
func (h *Handler) SetSimulation(sim *service.SimulationService) {
	h.simulation = sim
}

// SetWhatsApp sets the WhatsApp Cloud API service.
func (h *Handler) SetWhatsApp(wa *service.WhatsAppService) {
	h.waService = wa
}

// SetTelegram sets the Telegram Bot API service.
func (h *Handler) SetTelegram(tg *service.TelegramService) {
	h.tgService = tg
}

// RegisterTwilioRoutes registers Twilio v2010 REST endpoints.
func (h *Handler) RegisterTwilioRoutes(e *echo.Echo) {
	h.registerNumberRoutes(e)
	h.registerMessageRoutes(e)
	h.registerMediaRoutes(e)
	h.registerServiceRoutes(e)
}

// RegisterWhatsAppRoutes registers WhatsApp Cloud API endpoints.
func (h *Handler) RegisterWhatsAppRoutes(e *echo.Echo) {
	if h.waService != nil {
		h.registerWhatsAppRoutes(e)
	}
}

// RegisterTelegramRoutes registers Telegram Bot API endpoints.
func (h *Handler) RegisterTelegramRoutes(e *echo.Echo) {
	if h.tgService != nil {
		h.registerTelegramRoutes(e)
	}
}

// RegisterEmuRoutes registers /emu emulator endpoints.
func (h *Handler) RegisterEmuRoutes(e *echo.Echo) {
	h.registerEmuRoutes(e)
}

// RegisterRoutes registers all Twilio protocol, WhatsApp, Telegram, and /emu routes on the Echo router.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	h.RegisterTwilioRoutes(e)
	h.RegisterWhatsAppRoutes(e)
	h.RegisterTelegramRoutes(e)
	h.RegisterEmuRoutes(e)
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC1123Z)
	return &s
}

func handleError(c echo.Context, err error) error {
	return NewReqContext(c).Error(err)
}
