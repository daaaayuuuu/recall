package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadyReportsUnavailableDependency(t *testing.T) {
	handler := NewHandler("api", slog.New(slog.NewTextHandler(io.Discard, nil)), []Dependency{
		{Name: "mysql", Check: func(context.Context) error { return nil }},
		{Name: "minio", Check: func(context.Context) error { return errors.New("offline") }},
	})

	response := httptest.NewRecorder()
	handler.Ready(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
	if body := response.Body.String(); body == "" || body == "{}\n" {
		t.Fatalf("expected readiness response, got %q", body)
	}
}
