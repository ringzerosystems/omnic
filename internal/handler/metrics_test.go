package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	e := echo.New()
	e.Use(Metrics(reg))
	e.GET("/healthz", Healthz)
	e.GET("/users/:id", func(echo.Context) error { return echo.ErrNotFound })
	e.GET("/boom", func(echo.Context) error { return errors.New("boom") })

	for _, p := range []string{"/healthz", "/healthz", "/users/42", "/boom", "/nope"} {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	for _, want := range []string{
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="200",http_route="/healthz"} 2`,
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="404",http_route="/users/:id"} 1`,
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="500",http_route="/boom"} 1`,
		`http_server_request_duration_seconds_count{http_request_method="GET",http_response_status_code="404",http_route="unmatched"} 1`,
		`http_server_active_requests{http_request_method="GET"} 0`,
		`http_server_response_body_size_bytes_sum{http_request_method="GET",http_response_status_code="200",http_route="/healthz"} 4`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q\n%s", want, body)
		}
	}
}
