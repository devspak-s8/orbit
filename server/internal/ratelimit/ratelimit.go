// Package ratelimit is a simple in-memory fixed-window limiter shared by
// unauthenticated or high-abuse-risk endpoints. It is not a substitute for
// a distributed limiter, which arrives with horizontal scaling.
package ratelimit

import (
	"sync"
	"time"
)

type entry struct {
	start time.Time
	count int
}

type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*entry
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, entries: map[string]*entry{}}
}

// Allow reports whether the key may proceed, recording the attempt.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.entries[key]
	if !ok || now.Sub(e.start) >= l.window {
		e = &entry{start: now}
		l.entries[key] = e
	}
	e.count++

	// Opportunistic pruning keeps the map bounded.
	if len(l.entries) > 1024 {
		for k, v := range l.entries {
			if now.Sub(v.start) >= l.window {
				delete(l.entries, k)
			}
		}
	}
	return e.count <= l.limit
}
