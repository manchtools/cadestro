package auth

import (
	"sync"
	"time"
)

const maxRateLimiterKeys = 100_000

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time

	lastSeen map[string]time.Time
	limit    int
	window   time.Duration
	stopCh   chan struct{}
	now      func() time.Time

	maxKeys int
}

type RateLimiterOption func(*RateLimiter)

func WithClock(now func() time.Time) RateLimiterOption {
	return func(rl *RateLimiter) { rl.now = now }
}

func NewRateLimiter(limit int, window time.Duration, opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		attempts: make(map[string][]time.Time),
		lastSeen: make(map[string]time.Time),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
		now:      time.Now,
		maxKeys:  maxRateLimiterKeys,
	}
	for _, opt := range opts {
		opt(rl)
	}
	go rl.cleanup()
	return rl
}

func newRateLimiterWithCap(limit int, window time.Duration, maxKeys int, opts ...RateLimiterOption) *RateLimiter {
	rl := NewRateLimiter(limit, window, opts...)
	rl.mu.Lock()
	rl.maxKeys = maxKeys
	rl.mu.Unlock()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	cutoff := now.Add(-rl.window)

	attempts := rl.attempts[key]
	valid := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.attempts[key] = valid
		rl.lastSeen[key] = now
		return false
	}

	if _, exists := rl.attempts[key]; !exists && len(rl.attempts) >= rl.maxKeys {
		rl.evictEldestLocked()
	}

	rl.attempts[key] = append(valid, now)
	rl.lastSeen[key] = now
	return true
}

func (rl *RateLimiter) Blocked(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := rl.now().Add(-rl.window)
	valid := 0
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			valid++
		}
	}
	return valid >= rl.limit
}

func (rl *RateLimiter) evictEldestLocked() {
	var eldestKey string
	var eldestAt time.Time
	for k, t := range rl.lastSeen {
		if eldestKey == "" || t.Before(eldestAt) {
			eldestKey = k
			eldestAt = t
		}
	}
	if eldestKey != "" {
		delete(rl.attempts, eldestKey)
		delete(rl.lastSeen, eldestKey)
	}
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := rl.now().Add(-rl.window)
			for key, attempts := range rl.attempts {
				valid := attempts[:0]
				for _, t := range attempts {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(rl.attempts, key)
					delete(rl.lastSeen, key)
				} else {
					rl.attempts[key] = valid
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}
