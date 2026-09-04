package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/platform/config"
	"gamegen/backend/internal/server"
	"gamegen/backend/internal/sharing"

	"github.com/go-chi/chi/v5"
)

type apiAnalyticsRepository struct{}

func (*apiAnalyticsRepository) RecordEvent(context.Context, analytics.RecordInput) (analytics.RecordResult, error) {
	return analytics.RecordResult{}, nil
}

func (*apiAnalyticsRepository) ListAdminEvents(context.Context, analytics.ListFilter) (analytics.EventPage, error) {
	return analytics.EventPage{}, nil
}

func TestAssembleSurfaceMountsUsesFinalAnalyticsRouteMatrix(t *testing.T) {
	const (
		creatorRoute = "POST /api/v1/analytics/events"
		adminRoute   = "GET /api/v1/admin/behavior-events"
		publicRoute  = "POST /api/v1/public/play-sessions/current/events"
	)
	tests := []struct {
		surface string
		want    map[string]int
	}{
		{"app", map[string]int{creatorRoute: 1, adminRoute: 1}},
		{"play", map[string]int{publicRoute: 1}},
		{"all", map[string]int{creatorRoute: 1, adminRoute: 1, publicRoute: 1}},
	}
	for _, test := range tests {
		t.Run(test.surface, func(t *testing.T) {
			appFactory, playFactory := actualAnalyticsMountFactories()
			mounts, err := assembleSurfaceMounts(test.surface, appFactory, playFactory)
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Route("/api/v1", func(router chi.Router) {
				for _, mount := range mounts {
					mount(router)
				}
			})
			got := make(map[string]int)
			if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
				key := method + " " + route
				if key == creatorRoute || key == adminRoute || key == publicRoute {
					got[key]++
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("surface %q routes = %#v, want %#v", test.surface, got, test.want)
			}
			for route, count := range test.want {
				if got[route] != count {
					t.Errorf("surface %q route %q count = %d, want %d", test.surface, route, got[route], count)
				}
			}
		})
	}
}

func TestPlayAssemblyNeverCallsPrivateDependencyFactory(t *testing.T) {
	privateCalls := 0
	publicCalls := 0
	mounts, err := assembleSurfaceMounts("play", func(bool) ([]server.Mount, error) {
		privateCalls++
		return nil, errors.New("private auth/cipher factory must not run")
	}, func() (server.Mount, error) {
		publicCalls++
		return func(chi.Router) {}, nil
	})
	if err != nil || len(mounts) != 1 {
		t.Fatalf("play assembly = (%d mounts, %v)", len(mounts), err)
	}
	if privateCalls != 0 || publicCalls != 1 {
		t.Fatalf("factory calls: private=%d public=%d", privateCalls, publicCalls)
	}
}

func TestAssembleSurfaceMountsRejectsUnknownSurface(t *testing.T) {
	_, err := assembleSurfaceMounts("unexpected", func(bool) ([]server.Mount, error) { return nil, nil }, func() (server.Mount, error) { return nil, nil })
	if err == nil {
		t.Fatal("unexpected surface was accepted")
	}
}

func TestAnalyticsRecorderInjectionFollowsSurfaceAndNeverDuplicates(t *testing.T) {
	recorder := &apiAnalyticsRepository{}
	tests := []struct {
		surface     string
		wantPrivate bool
		wantPublic  bool
	}{
		{"app", true, false},
		{"play", false, true},
		{"all", true, true},
	}
	for _, test := range tests {
		t.Run(test.surface, func(t *testing.T) {
			private, public, err := analyticsRecordersForSurface(test.surface, recorder)
			if err != nil {
				t.Fatal(err)
			}
			if (private == recorder) != test.wantPrivate || (public == recorder) != test.wantPublic {
				t.Fatalf("private=%T public=%T", private, public)
			}
			if test.surface == "all" && private != public {
				t.Fatal("all surface did not reuse exactly one recorder")
			}
		})
	}
	if _, _, err := analyticsRecordersForSurface("invalid", recorder); err == nil {
		t.Fatal("invalid surface accepted recorder injection")
	}
}

func actualAnalyticsMountFactories() (
	func(bool) ([]server.Mount, error),
	func() (server.Mount, error),
) {
	cfg := config.Config{App: config.AppConfig{
		Environment: "test", AppBaseURL: "https://app.example", PlayBaseURL: "https://play.example",
	}, Sharing: config.SharingConfig{MaxLinkLifetimeDays: 90, PlaySessionMinutes: 30}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repository := &apiAnalyticsRepository{}
	analyticsHandler := analytics.NewHandler(repository, cfg, logger)
	publicHandler := sharing.NewHandler(nil, nil, nil, nil, repository, cfg, logger)
	allow := func(next http.Handler) http.Handler { return next }
	appFactory := func(includePublic bool) ([]server.Mount, error) {
		mountAnalytics := func(router chi.Router) {
			analyticsHandler.MountApp(router, allow, allow, allow, func(*http.Request) (string, string) { return "", "" })
		}
		mounts := []server.Mount{mountAnalytics}
		if includePublic {
			mounts = append(mounts, publicHandler.MountPublic)
		}
		return mounts, nil
	}
	playFactory := func() (server.Mount, error) { return publicHandler.MountPublic, nil }
	return appFactory, playFactory
}
