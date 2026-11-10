package workspaces

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires workspace endpoints onto the versioned API mux. Callers
// must wrap these routes with identity.RequireUser so the user is present
// in the request context.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces", s.handleList)
	api.HandleFunc("POST /v1/workspaces", s.handleCreate)
	api.HandleFunc("GET /v1/workspaces/{id}", s.handleGet)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	items, err := s.List(r.Context(), userID.ID)
	if err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	if items == nil {
		items = []Workspace{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workspaces": items})
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	ws, err := s.Create(r.Context(), user.ID, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidName):
			apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"workspace": ws})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	ws, err := s.Get(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apijson.WriteError(w, http.StatusNotFound, "not_found", "workspace not found")
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workspace": ws})
}
