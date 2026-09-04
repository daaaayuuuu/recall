package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestRequestIDUsesDatabaseCompatibleIdentifier(t *testing.T) {
	handler := requestID(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		id := middleware.GetReqID(request.Context())
		if len(id) != 26 {
			t.Fatalf("unexpected request id length: %q", id)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if len(recorder.Header().Get("X-Request-ID")) != 26 {
		t.Fatalf("unexpected response request id: %q", recorder.Header().Get("X-Request-ID"))
	}
}

func TestStaticHandlerServesSPAAndAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<main>game-gen</main>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newStaticHandler(root, "all")
	if err != nil {
		t.Fatal(err)
	}

	for _, requestPath := range []string{"/", "/app/games", "/play/public-id"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != "<main>game-gen</main>" {
			t.Fatalf("expected SPA index for %s, got %d %q", requestPath, recorder.Code, recorder.Body.String())
		}
		if recorder.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("expected index not to be cached for %s", requestPath)
		}
	}

	assetRecorder := httptest.NewRecorder()
	handler.ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if assetRecorder.Code != http.StatusOK || assetRecorder.Body.String() != "console.log('ok')" {
		t.Fatalf("unexpected asset response: %d %q", assetRecorder.Code, assetRecorder.Body.String())
	}
	if assetRecorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatal("expected versioned assets to be cached")
	}
}

func TestStaticHandlerDoesNotMaskMissingAPIOrAsset(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := newStaticHandler(root, "all")
	if err != nil {
		t.Fatal(err)
	}

	for _, requestPath := range []string{"/api/v1/unknown", "/assets/missing.js", "/health/unknown"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", requestPath, recorder.Code)
		}
	}
}

func TestStaticHandlerRestrictsApplicationSurface(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		surface string
		allowed []string
		denied  []string
	}{
		{surface: "app", allowed: []string{"/", "/auth/login", "/app/games", "/admin"}, denied: []string{"/play/public-id", "/unknown"}},
		{surface: "play", allowed: []string{"/play/public-id"}, denied: []string{"/", "/auth/login", "/app/games", "/admin"}},
	}

	for _, test := range tests {
		handler, err := newStaticHandler(root, test.surface)
		if err != nil {
			t.Fatal(err)
		}
		for _, requestPath := range test.allowed {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected %s surface to allow %s, got %d", test.surface, requestPath, recorder.Code)
			}
		}
		for _, requestPath := range test.denied {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("expected %s surface to deny %s, got %d", test.surface, requestPath, recorder.Code)
			}
		}
		assetRecorder := httptest.NewRecorder()
		handler.ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
		if assetRecorder.Code != http.StatusOK {
			t.Fatalf("expected %s surface to serve built assets, got %d", test.surface, assetRecorder.Code)
		}
	}
}
