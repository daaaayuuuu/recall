package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterRefillsTokens(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	limiter := New(2, time.Minute)
	limiter.now = func() time.Time { return now }

	if !limiter.Allow("client") || !limiter.Allow("client") {
		t.Fatal("expected initial tokens to be allowed")
	}
	if limiter.Allow("client") {
		t.Fatal("expected exhausted bucket to be denied")
	}

	now = now.Add(30 * time.Second)
	if !limiter.Allow("client") {
		t.Fatal("expected one token to be refilled")
	}
	if limiter.Allow("client") {
		t.Fatal("expected refilled token to be consumed")
	}
}

func TestLimiterSeparatesKeys(t *testing.T) {
	limiter := New(1, time.Hour)
	if !limiter.Allow("first") || !limiter.Allow("second") {
		t.Fatal("expected independent keys to have independent buckets")
	}
}
