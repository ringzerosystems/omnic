package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/ringzerosystems/omnic/internal/handler"
)

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "healthz", path: "/healthz"},
		{name: "livez", path: "/livez"},
		{name: "readyz", path: "/readyz"},
		{name: "startupz", path: "/startupz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			handler.RegisterHealthRoutes(e)

			// Default plain text response
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", tt.path, rec.Code, http.StatusOK)
			}
			if got, want := rec.Body.String(), "ok"; got != want {
				t.Fatalf("GET %s body = %q, want %q", tt.path, got, want)
			}

			// JSON response via Accept header
			reqJSON := httptest.NewRequest(http.MethodGet, tt.path, nil)
			reqJSON.Header.Set("Accept", "application/json")
			recJSON := httptest.NewRecorder()
			e.ServeHTTP(recJSON, reqJSON)

			if recJSON.Code != http.StatusOK {
				t.Fatalf("GET %s with Accept: application/json status = %d, want %d", tt.path, recJSON.Code, http.StatusOK)
			}
			var resp handler.HealthResponse
			if err := json.Unmarshal(recJSON.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode JSON response: %v", err)
			}
			if resp.Status != "ok" {
				t.Fatalf("got status %q, want %q", resp.Status, "ok")
			}
		})
	}
}
