// Package httpapi implements the HTTP surface of the platform: routing,
// middleware, and the shared response conventions from the PRD (unified
// error format, request IDs, versioned endpoints).
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/automation"
	"github.com/orbit-org/orbit/server/internal/demo"
	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/modules"
	"github.com/orbit-org/orbit/server/internal/notifications"
	"github.com/orbit-org/orbit/server/internal/work"
	"github.com/orbit-org/orbit/server/internal/workspaces"
)

// Services bundles the domain services the router wires into HTTP routes.
type Services struct {
	Identity      *identity.Service
	Workspaces    *workspaces.Service
	Modules       *modules.Service
	Demo          *demo.Service
	Events        *events.Service
	Automation    *automation.Service
	Work          *work.Service
	Notifications *notifications.Service
}

// NewRouter builds the full API handler with standard middleware applied.
func NewRouter(pool *pgxpool.Pool, logger *slog.Logger, allowedOrigins []string, svcs Services) http.Handler {
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

	// Identity endpoints (some public, some session-guarded internally).
	svcs.Identity.Handlers(api)

	// User-scoped routes: notifications and invite redemption.
	userAPI := http.NewServeMux()
	svcs.Notifications.Handlers(userAPI)
	svcs.Workspaces.HandlersUser(userAPI)
	api.Handle("/v1/notifications", svcs.Identity.RequireUser(userAPI))
	api.Handle("/v1/notifications/", svcs.Identity.RequireUser(userAPI))
	api.Handle("/v1/invites/", svcs.Identity.RequireUser(userAPI))

	// Everything under /v1/workspaces requires an authenticated session.
	// Deeper authorization (membership, owner role, module enabled) is
	// enforced inside the services, which query the database directly.
	wsAPI := http.NewServeMux()
	svcs.Workspaces.Handlers(wsAPI)
	svcs.Modules.Handlers(wsAPI)
	svcs.Demo.Handlers(wsAPI)
	svcs.Events.Handlers(wsAPI)
	svcs.Automation.Handlers(wsAPI)
	svcs.Work.Handlers(wsAPI)
	api.Handle("/v1/workspaces", svcs.Identity.RequireUser(wsAPI))
	api.Handle("/v1/workspaces/", svcs.Identity.RequireUser(wsAPI))

	mux.Handle("/v1/", api)

	// Webhook deliveries are top-level and unauthenticated: the secret is
	// the token in the path. Rate-limited and audited in the handler.
	svcs.Automation.HookHandlers(mux)

	var h http.Handler = mux
	h = withCORS(allowedOrigins, h)
	h = withLogging(logger, h)
	h = withRequestID(h)
	return h
}
