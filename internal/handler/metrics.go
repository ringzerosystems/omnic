package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics returns middleware that records RED metrics per route using the
// OpenTelemetry HTTP server semantic conventions:
//
//	http_server_request_duration_seconds{http_request_method,http_route,http_response_status_code}
//	http_server_active_requests{http_request_method}
//	http_server_response_body_size_bytes{http_request_method,http_route,http_response_status_code}
//
// http_route is the registered route pattern (never the raw URI), so
// cardinality is bounded by the number of routes.
func Metrics(reg prometheus.Registerer) echo.MiddlewareFunc {
	labels := []string{"http_request_method", "http_route", "http_response_status_code"}
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_server_request_duration_seconds",
		Help: "Duration of HTTP server requests.",
		// OTel semconv recommended buckets for http.server.request.duration.
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10},
	}, labels)
	active := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_server_active_requests",
		Help: "Number of active HTTP server requests.",
	}, labels[:1])
	respSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_response_body_size_bytes",
		Help:    "Size of HTTP server response bodies.",
		Buckets: prometheus.ExponentialBuckets(64, 4, 10), // 64B .. 16MiB
	}, labels)
	reg.MustRegister(duration, active, respSize)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method
			active.WithLabelValues(method).Inc()
			start := time.Now()
			err := next(c)
			active.WithLabelValues(method).Dec()

			// Handler errors are rendered by Echo's HTTPErrorHandler after this
			// middleware returns, so derive the status the client will see.
			res := c.Response()
			status := res.Status
			if err != nil && !res.Committed {
				status = http.StatusInternalServerError
				if he, ok := errors.AsType[*echo.HTTPError](err); ok {
					status = he.Code
				}
			}
			route := c.Path()
			if route == "" {
				route = "unmatched"
			}
			code := strconv.Itoa(status)
			duration.WithLabelValues(method, route, code).Observe(time.Since(start).Seconds())
			respSize.WithLabelValues(method, route, code).Observe(float64(res.Size))
			return err
		}
	}
}
