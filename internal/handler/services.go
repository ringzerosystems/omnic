package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/service"
)

func (h *Handler) registerServiceRoutes(e *echo.Echo) {
	e.POST("/v1/Services", h.handleCreateService)
	e.GET("/v1/Services", h.handleListServices)
	e.GET("/v1/Services/:service_sid", h.handleGetService)
	e.POST("/v1/Services/:service_sid", h.handleUpdateService)
	e.DELETE("/v1/Services/:service_sid", h.handleDeleteService)

	e.POST("/v1/Services/:service_sid/PhoneNumbers", h.handleAddServicePhoneNumber)
	e.GET("/v1/Services/:service_sid/PhoneNumbers", h.handleListServicePhoneNumbers)
	e.DELETE("/v1/Services/:service_sid/PhoneNumbers/:phone_number_sid", h.handleRemoveServicePhoneNumber)
}

func (h *Handler) handleCreateService(c echo.Context) error {
	rc := NewReqContext(c)
	sticky := c.FormValue("StickySender") == "true"
	geomatch := c.FormValue("Geomatch") == "true"

	svc, err := h.svcService.Create(rc.Context(), rc.AccountSID(), service.CreateServiceParams{
		FriendlyName:      c.FormValue("FriendlyName"),
		InboundRequestURL: c.FormValue("InboundRequestUrl"),
		InboundMethod:     c.FormValue("InboundMethod"),
		FallbackURL:       c.FormValue("FallbackUrl"),
		FallbackMethod:    c.FormValue("FallbackMethod"),
		StatusCallback:    c.FormValue("StatusCallback"),
		StickySender:      sticky,
		Geomatch:          geomatch,
	})
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusCreated, NewServiceView(svc))
}

func (h *Handler) handleListServices(c echo.Context) error {
	rc := NewReqContext(c)
	list, total, err := h.store.ListMessagingServices(rc.Context(), rc.AccountSID(), rc.PageSize(50), rc.Offset())
	if err != nil {
		return rc.Error(err)
	}
	formatted := make([]ServiceView, 0, len(list))
	for _, s := range list {
		formatted = append(formatted, NewServiceView(s))
	}
	return c.JSON(http.StatusOK, ServiceListPageView{
		Services: formatted,
		Total:    total,
	})
}

func (h *Handler) handleGetService(c echo.Context) error {
	rc := NewReqContext(c)
	svc, err := h.store.GetMessagingServiceBySID(rc.Context(), rc.AccountSID(), rc.CleanParam("service_sid"))
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, NewServiceView(svc))
}

func (h *Handler) handleUpdateService(c echo.Context) error {
	rc := NewReqContext(c)
	var friendly *string
	if val := c.FormValue("FriendlyName"); val != "" {
		friendly = &val
	}
	svc, err := h.svcService.Update(rc.Context(), rc.AccountSID(), rc.CleanParam("service_sid"), service.UpdateServiceParams{
		FriendlyName: friendly,
	})
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, NewServiceView(svc))
}

func (h *Handler) handleDeleteService(c echo.Context) error {
	rc := NewReqContext(c)
	if err := h.store.DeleteMessagingService(rc.Context(), rc.AccountSID(), rc.CleanParam("service_sid")); err != nil {
		return rc.Error(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleAddServicePhoneNumber(c echo.Context) error {
	rc := NewReqContext(c)
	spn, err := h.svcService.AddPhoneNumber(rc.Context(), rc.CleanParam("service_sid"), c.FormValue("PhoneNumberSid"))
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusCreated, spn)
}

func (h *Handler) handleListServicePhoneNumbers(c echo.Context) error {
	rc := NewReqContext(c)
	numbers, err := h.store.ListServicePhoneNumbers(rc.Context(), rc.CleanParam("service_sid"))
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"phone_numbers": numbers,
	})
}

func (h *Handler) handleRemoveServicePhoneNumber(c echo.Context) error {
	rc := NewReqContext(c)
	if err := h.store.RemoveServicePhoneNumber(rc.Context(), rc.CleanParam("service_sid"), rc.CleanParam("phone_number_sid")); err != nil {
		return rc.Error(err)
	}
	return c.NoContent(http.StatusNoContent)
}
