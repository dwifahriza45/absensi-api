package shared

import (
	"sync"
	"time"
)

// AttemptLimiter reserves attempts atomically, including concurrent logins.
// It is process-local; multiple replicas need a shared limiter.
type AttemptLimiter struct {
	mu          sync.Mutex
	max         int
	window      time.Duration
	entries     map[string]attempt
	nextCleanup time.Time
}
type attempt struct {
	count int
	until time.Time
}

const maxLimiterEntries = 10000

func NewAttemptLimiter(max int, window time.Duration) *AttemptLimiter {
	return &AttemptLimiter{max: max, window: window, entries: make(map[string]attempt)}
}
func (l *AttemptLimiter) cleanup(now time.Time) {
	if now.Before(l.nextCleanup) {
		return
	}
	for k, e := range l.entries {
		if !now.Before(e.until) {
			delete(l.entries, k)
		}
	}
	l.nextCleanup = now.Add(time.Minute)
}
func (l *AttemptLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.cleanup(now)
	e, ok := l.entries[key]
	if ok && !now.Before(e.until) {
		delete(l.entries, key)
		ok = false
	}
	if !ok {
		if len(l.entries) >= maxLimiterEntries {
			return false
		}
		e = attempt{until: now.Add(l.window)}
	}
	if e.count >= l.max {
		return false
	}
	e.count++
	l.entries[key] = e
	return true
}
func (l *AttemptLimiter) Reset(key string) { l.mu.Lock(); defer l.mu.Unlock(); delete(l.entries, key) }
