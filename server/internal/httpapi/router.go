// Package httpapi implements the HTTP surface of the platform: routing,
// middleware, and the shared response conventions from the PRD (unified
// error format, request IDs, versioned endpoints).
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter builds the full API handler with standard middleware applied.
// The pool is threaded through here so future domain handlers can be
// registered against real infrastructure.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()

	// Liveness/readiness, unauthenticated.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Versioned API namespace.
	api := http.NewServeMux()
	api.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "pong"})
	})
	mux.Handle("/v1/", api)

	var h http.Handler = mux
	h = withCORS(allowedOrigins, h)
	h = withRequestID(h)
	h = withLogging(logger, h)
	return h
}