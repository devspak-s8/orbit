package notifications

import (
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires the user-scoped notification endpoints. Callers mount
// these behind identity.RequireUser.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/notifications", s.handleList)
	api.HandleFunc("POST /v1/notifications/read-all", s.handleReadAll)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	items, unread, err := s.List(r.Context(), user.ID, 20)
	if err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"notifications": items, "unread": unread})
}

func (s *Service) handleReadAll(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if err := s.MarkAllRead(r.Context(), user.ID); err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
