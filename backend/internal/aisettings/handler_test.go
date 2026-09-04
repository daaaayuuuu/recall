package aisettings

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamegen/backend/internal/platform/config"

	"github.com/go-chi/chi/v5"
)

func TestAdminReadNeverReturnsPlaintextAPIKeys(t *testing.T) {
	base := testBaseAIConfig()
	base.Text.APIKey = "super-secret-value"
	manager, err := NewManager(nil, base, "test", config.DynamicAIConfig{})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(manager, func(*http.Request) (string, string) { return "session", "admin" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	allow := func(next http.Handler) http.Handler { return next }
	router := chi.NewRouter()
	handler.Mount(router, allow, allow)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/ai-settings", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, "super-secret-value") || !strings.Contains(body, "••••alue") {
		t.Fatalf("response did not preserve the secret boundary: %s", body)
	}
}
