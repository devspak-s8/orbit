package identity

import (
	"sync"
	"time"
)

// rateLimiter is a simple in-memory fixed-window limiter, sufficient for
// throttling authentication endpoints on a single instance. It is not a
// substitute for a distributed limiter, which arrives with horizontal
// scaling (PLAN.md D-works).
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*windowState
}

type windowState struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*windowState),
	}
}

// allow reports whether the key may proceed, recording the attempt.
func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	st, ok := r.entries[key]
	if !ok || now.Sub(st.start) >= r.window {
		st = &windowState{start: now}
		r.entries[key] = st
	}
	st.count++

	// Opportunistic pruning to keep the map bounded.
	if len(r.entries) > 1024 {
		for k, v := range r.entries {
			if now.Sub(v.start) >= r.window {
				delete(r.entries, k)
			}
		}
	}
	return st.count <= r.limit
}