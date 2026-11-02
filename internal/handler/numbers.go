package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/service"
)

func (h *Handler) registerNumberRoutes(e *echo.Echo) {
	e.GET("/2010-04-01/Accounts/:account_sid/AvailablePhoneNumbers/:country_code/:type", h.handleAvailableNumbers)

	e.POST("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers", h.handleProvisionNumber)
	e.POST("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers.json", h.handleProvisionNumber)

	e.GET("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers", h.handleListIncomingNumbers)
	e.GET("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers.json", h.handleListIncomingNumbers)

	e.GET("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers/:sid", h.handleGetIncomingNumber)
	e.POST("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers/:sid", h.handleUpdateIncomingNumber)
	e.DELETE("/2010-04-01/Accounts/:account_sid/IncomingPhoneNumbers/:sid", h.handleDeleteIncomingNumber)
}

func (h *Handler) handleAvailableNumbers(c echo.Context) error {
	rc := NewReqContext(c)
	countryCode := c.Param("country_code")
	numType := rc.CleanParam("type")

	results, err := h.phoneService.SearchAvailable(rc.Context(), countryCode, numType, service.AvailableFilter{
		AreaCode: rc.Query("AreaCode"),
		Contains: rc.Query("Contains"),
		Limit:    rc.PageSize(0),
	})
	if err != nil {
		return rc.Error(err)
	}

	uri := fmt.Sprintf("/2010-04-01/Accounts/%s/AvailablePhoneNumbers/%s/%s.json", rc.AccountSID(), countryCode, numType)
	return c.JSON(http.StatusOK, map[string]any{
		"uri":                     uri,
		"available_phone_numbers": results,
	})
}

func (h *Handler) handleProvisionNumber(c echo.Context) error {
	rc := NewReqContext(c)
	pn, err := h.phoneService.Provision(rc.Context(), rc.AccountSID(), service.ProvisionParams{
		PhoneNumber:       c.FormValue("PhoneNumber"),
		AreaCode:          c.FormValue("AreaCode"),
		FriendlyName:      c.FormValue("FriendlyName"),
		SmsURL:            c.FormValue("SmsUrl"),
		SmsMethod:         c.FormValue("SmsMethod"),
		SmsFallbackURL:    c.FormValue("SmsFallbackUrl"),
		SmsFallbackMethod: c.FormValue("SmsFallbackMethod"),
		StatusCallback:    c.FormValue("StatusCallback"),
	})
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusCreated, NewPhoneNumberView(pn))
}

func (h *Handler) handleListIncomingNumbers(c echo.Context) error {
	rc := NewReqContext(c)
	list, total, err := h.store.ListPhoneNumbers(rc.Context(), rc.AccountSID(), domain.PhoneNumberFilter{
		PhoneNumber:  rc.Query("PhoneNumber"),
		FriendlyName: rc.Query("FriendlyName"),
		Limit:        rc.PageSize(50),
		Offset:       rc.Offset(),
	})
	if err != nil {
		return rc.Error(err)
	}

	formatted := make([]PhoneNumberView, 0, len(list))
	for _, pn := range list {
		formatted = append(formatted, NewPhoneNumberView(pn))
	}

	uri := fmt.Sprintf("/2010-04-01/Accounts/%s/IncomingPhoneNumbers.json?PageSize=%d&Page=%d", rc.AccountSID(), rc.PageSize(50), rc.Page())
	return c.JSON(http.StatusOK, PhoneNumberListPageView{
		FirstPageURI:         uri,
		End:                  len(formatted),
		IncomingPhoneNumbers: formatted,
		URI:                  uri,
		PageSize:             rc.PageSize(50),
		Start:                rc.Offset(),
		Page:                 rc.Page(),
		Total:                total,
	})
}

func (h *Handler) handleGetIncomingNumber(c echo.Context) error {
	rc := NewReqContext(c)
	pn, err := h.store.GetPhoneNumberBySID(rc.Context(), rc.AccountSID(), rc.CleanParam("sid"))
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, NewPhoneNumberView(pn))
}

func (h *Handler) handleUpdateIncomingNumber(c echo.Context) error {
	rc := NewReqContext(c)
	var friendly *string
	if val := c.FormValue("FriendlyName"); val != "" {
		friendly = &val
	}
	var smsURL *string
	if val := c.FormValue("SmsUrl"); val != "" {
		smsURL = &val
	}

	pn, err := h.phoneService.Update(rc.Context(), rc.AccountSID(), rc.CleanParam("sid"), service.UpdatePhoneParams{
		FriendlyName: friendly,
		SmsURL:       smsURL,
	})
	if err != nil {
		return rc.Error(err)
	}
	return c.JSON(http.StatusOK, NewPhoneNumberView(pn))
}

func (h *Handler) handleDeleteIncomingNumber(c echo.Context) error {
	rc := NewReqContext(c)
	if err := h.store.DeletePhoneNumber(rc.Context(), rc.AccountSID(), rc.CleanParam("sid")); err != nil {
		return rc.Error(err)
	}
	return c.NoContent(http.StatusNoContent)
}
