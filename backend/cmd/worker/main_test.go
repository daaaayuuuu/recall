package main

import (
	"testing"

	"gamegen/backend/internal/analytics"
	"gamegen/backend/internal/platform/database"
)

func TestWorkerCreatesAnIndependentPersistentAnalyticsRecorder(t *testing.T) {
	db := &database.DB{}
	first := newWorkerAnalyticsRecorder(db)
	second := newWorkerAnalyticsRecorder(db)
	if first == nil || second == nil || first == second {
		t.Fatalf("worker recorder instances: first=%T second=%T equal=%v", first, second, first == second)
	}
	if _, ok := first.(*analytics.Repository); !ok {
		t.Fatalf("worker recorder type=%T, want persistent analytics repository", first)
	}
}
