// Package handler contains HTTP handlers for the server.
package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// HealthResponse represents the health probe status when JSON is requested.
type HealthResponse struct {
	Status string `json:"status"`
}

func healthOK(c echo.Context) error {
	if strings.Contains(c.Request().Header.Get("Accept"), "application/json") || c.QueryParam("format") == "json" {
		return c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
	}
	return c.String(http.StatusOK, "ok")
}

// Healthz responds to generic health check probes.
func Healthz(c echo.Context) error {
	return healthOK(c)
}

// Livez responds to Kubernetes liveness probes.
func Livez(c echo.Context) error {
	return healthOK(c)
}

// Readyz responds to Kubernetes readiness probes.
func Readyz(c echo.Context) error {
	return healthOK(c)
}

// Startupz responds to Kubernetes startup probes.
func Startupz(c echo.Context) error {
	return healthOK(c)
}

// RegisterHealthRoutes registers Kubernetes health check endpoints on the router.
func RegisterHealthRoutes(e *echo.Echo) {
	e.GET("/healthz", Healthz)
	e.GET("/livez", Livez)
	e.GET("/readyz", Readyz)
	e.GET("/startupz", Startupz)
}
