package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens     float64
	updatedAt  time.Time
	lastAccess time.Time
}

type Limiter struct {
	mu         sync.Mutex
	buckets    map[string]bucket
	capacity   float64
	refillRate float64
	now        func() time.Time
	requests   uint64
}

// New creates a process-local token bucket with capacity tokens refilled over window.
func New(capacity int, window time.Duration) *Limiter {
	if capacity < 1 {
		capacity = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{
		buckets:    make(map[string]bucket),
		capacity:   float64(capacity),
		refillRate: float64(capacity) / window.Seconds(),
		now:        time.Now,
	}
}

func (limiter *Limiter) Allow(key string) bool {
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry, exists := limiter.buckets[key]
	if !exists {
		entry = bucket{tokens: limiter.capacity, updatedAt: now}
	}
	elapsed := now.Sub(entry.updatedAt).Seconds()
	if elapsed > 0 {
		entry.tokens = min(limiter.capacity, entry.tokens+elapsed*limiter.refillRate)
	}
	entry.updatedAt = now
	entry.lastAccess = now

	allowed := entry.tokens >= 1
	if allowed {
		entry.tokens--
	}
	limiter.buckets[key] = entry

	limiter.requests++
	if limiter.requests%256 == 0 {
		limiter.removeIdle(now)
	}
	return allowed
}

func (limiter *Limiter) removeIdle(now time.Time) {
	idleAfter := time.Duration(limiter.capacity/limiter.refillRate*2) * time.Second
	for key, entry := range limiter.buckets {
		if now.Sub(entry.lastAccess) > idleAfter {
			delete(limiter.buckets, key)
		}
	}
}
