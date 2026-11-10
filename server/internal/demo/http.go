package demo

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires the Demo module's routes onto a mux that receives
// workspace paths. Access requires the module to be installed and enabled;
// otherwise the routes behave as if they do not exist.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces/{id}/modules/demo/records", s.handleList)
	api.HandleFunc("POST /v1/workspaces/{id}/modules/demo/records", s.handleAdd)
}

func (s *Service) gate(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return "", "", false
	}
	wsID := r.PathValue("id")
	if !s.Enabled(r.Context(), wsID) {
		apijson.WriteError(w, http.StatusNotFound, "not_found", "module not enabled in this workspace")
		return "", "", false
	}
	return wsID, user.ID, true
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	wsID, _, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.List(r.Context(), wsID)
	if err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	if items == nil {
		items = []Record{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"records": items})
}

func (s *Service) handleAdd(w http.ResponseWriter, r *http.Request) {
	wsID, userID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	record, err := s.Add(r.Context(), wsID, userID, req.Title)
	if err != nil {
		if errors.Is(err, ErrTitleRequired) {
			apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"record": record})
}
