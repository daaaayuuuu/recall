package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type Dependency struct {
	Name  string
	Check func(context.Context) error
}

type Handler struct {
	service      string
	logger       *slog.Logger
	dependencies []Dependency
}

type response struct {
	Status       string            `json:"status"`
	Service      string            `json:"service"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

func NewHandler(service string, logger *slog.Logger, dependencies []Dependency) *Handler {
	return &Handler{service: service, logger: logger, dependencies: dependencies}
}

func (handler *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, response{Status: "ok", Service: handler.service})
}

func (handler *Handler) Ready(w http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
	defer cancel()

	status := http.StatusOK
	result := response{
		Status:       "ok",
		Service:      handler.service,
		Dependencies: make(map[string]string, len(handler.dependencies)),
	}
	for _, dependency := range handler.dependencies {
		if err := dependency.Check(ctx); err != nil {
			status = http.StatusServiceUnavailable
			result.Status = "unavailable"
			result.Dependencies[dependency.Name] = "unavailable"
			handler.logger.Warn("dependency unavailable", "dependency", dependency.Name, "error", err)
			continue
		}
		result.Dependencies[dependency.Name] = "ok"
	}
	writeJSON(w, status, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
