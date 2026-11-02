package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/domain"
	"github.com/ringzerosystems/omnic/internal/service"
)

// ReqContext provides a shared stack frame for HTTP handlers with domain extraction verbs.
type ReqContext struct {
	echo echo.Context
}

// NewReqContext constructs a ReqContext wrapping the echo context.
func NewReqContext(c echo.Context) *ReqContext {
	return &ReqContext{echo: c}
}

// Context returns the standard request context.
func (rc *ReqContext) Context() context.Context {
	return rc.echo.Request().Context()
}

// CleanParam returns the path parameter with .json and .xml extensions removed.
func (rc *ReqContext) CleanParam(name string) string {
	s := rc.echo.Param(name)
	s = strings.TrimSuffix(s, ".json")
	return strings.TrimSuffix(s, ".xml")
}

// AccountSID extracts the account SID from path parameters, headers, or default.
func (rc *ReqContext) AccountSID() string {
	sid := rc.CleanParam("account_sid")
	if sid != "" {
		return sid
	}
	if hdr := rc.echo.Request().Header.Get("X-Account-Sid"); hdr != "" {
		return hdr
	}
	if q := rc.echo.QueryParam("account_sid"); q != "" {
		return q
	}
	return "AC001"
}

// Query returns the query parameter string value.
func (rc *ReqContext) Query(name string) string {
	return rc.echo.QueryParam(name)
}

// Page returns the requested 0-indexed page number.
func (rc *ReqContext) Page() int {
	p, _ := strconv.Atoi(rc.echo.QueryParam("Page"))
	if p < 0 {
		return 0
	}
	return p
}

// PageSize returns the requested page size with default fallback.
func (rc *ReqContext) PageSize(defaultSize int) int {
	size, _ := strconv.Atoi(rc.echo.QueryParam("PageSize"))
	if size <= 0 {
		return defaultSize
	}
	return size
}

// Offset returns the computed pagination offset (page * pageSize).
func (rc *ReqContext) Offset() int {
	return rc.Page() * rc.PageSize(50)
}

// Error handles centralized error mapping for domain and Twilio errors.
func (rc *ReqContext) Error(err error) error {
	if err == nil {
		return nil
	}
	if twErr, ok := errors.AsType[*service.TwilioError](err); ok {
		return rc.echo.JSON(twErr.Status, twErr)
	}
	if errors.Is(err, domain.ErrNotFound) {
		return rc.echo.JSON(http.StatusNotFound, service.NewTwilioError(20404, http.StatusNotFound, "The requested resource was not found"))
	}
	if errors.Is(err, domain.ErrConflict) {
		return rc.echo.JSON(http.StatusConflict, service.NewTwilioError(21450, http.StatusConflict, "Resource conflict"))
	}
	return rc.echo.JSON(http.StatusInternalServerError, map[string]any{
		"code":    500,
		"message": err.Error(),
		"status":  http.StatusInternalServerError,
	})
}
