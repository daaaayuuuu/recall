package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gamegen/backend/internal/platform/health"
	"gamegen/backend/internal/platform/security"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

type Mount func(chi.Router)

func New(service, address, staticDir string, logger *slog.Logger, dependencies []health.Dependency, mounts ...func(chi.Router)) (*Server, error) {
	converted := make([]Mount, 0, len(mounts))
	for _, mount := range mounts {
		converted = append(converted, mount)
	}
	return NewForSurface(service, address, staticDir, "all", logger, dependencies, converted...)
}

func NewForSurface(service, address, staticDir, surface string, logger *slog.Logger, dependencies []health.Dependency, mounts ...Mount) (*Server, error) {
	healthHandler := health.NewHandler(service, logger, dependencies)
	router := chi.NewRouter()
	router.Use(requestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)
	router.Get("/api/health/live", healthHandler.Live)
	router.Get("/api/health/ready", healthHandler.Ready)
	if len(mounts) > 0 {
		router.Route("/api/v1", func(apiRouter chi.Router) {
			for _, mount := range mounts {
				mount(apiRouter)
			}
		})
	}
	if staticDir != "" {
		staticHandler, err := newStaticHandler(staticDir, surface)
		if err != nil {
			return nil, err
		}
		router.NotFound(staticHandler.ServeHTTP)
	}

	return &Server{
		logger: logger,
		httpServer: &http.Server{
			Addr:              address,
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}, nil
}

func newStaticHandler(root, surface string) (http.Handler, error) {
	if surface != "all" && surface != "app" && surface != "play" {
		return nil, fmt.Errorf("unsupported static surface %q", surface)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve static directory: %w", err)
	}
	staticFS := os.DirFS(absoluteRoot)
	indexInfo, err := fs.Stat(staticFS, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read static index: %w", err)
	}
	indexContents, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read static index: %w", err)
	}
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.NotFound(w, request)
			return
		}
		if strings.HasPrefix(request.URL.Path, "/api/") || strings.HasPrefix(request.URL.Path, "/health/") {
			http.NotFound(w, request)
			return
		}

		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if name == "." || name == "index.html" {
			if !spaRouteAllowed(surface, request.URL.Path) {
				http.NotFound(w, request)
				return
			}
			serveIndex(w, request, indexInfo.ModTime(), indexContents)
			return
		}
		if fs.ValidPath(name) {
			if info, statErr := fs.Stat(staticFS, name); statErr == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, request)
				return
			}
		}
		if path.Ext(name) != "" {
			http.NotFound(w, request)
			return
		}
		if !spaRouteAllowed(surface, request.URL.Path) {
			http.NotFound(w, request)
			return
		}
		serveIndex(w, request, indexInfo.ModTime(), indexContents)
	}), nil
}

func spaRouteAllowed(surface, requestPath string) bool {
	cleaned := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	switch surface {
	case "all":
		return true
	case "app":
		return cleaned == "/" || cleaned == "/auth" || strings.HasPrefix(cleaned, "/auth/") ||
			cleaned == "/app" || strings.HasPrefix(cleaned, "/app/") ||
			cleaned == "/admin" || strings.HasPrefix(cleaned, "/admin/")
	case "play":
		return cleaned == "/play" || strings.HasPrefix(cleaned, "/play/")
	default:
		return false
	}
}

func serveIndex(w http.ResponseWriter, request *http.Request, modifiedAt time.Time, contents []byte) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, request, "index.html", modifiedAt, bytes.NewReader(contents))
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		id, err := security.NewID()
		if err != nil {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(request.Context(), middleware.RequestIDKey, id)
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

func (server *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.httpServer.Shutdown(shutdownCtx); err != nil {
			server.logger.Warn("http server shutdown incomplete", "error", err)
			return err
		}
		return nil
	}
}
