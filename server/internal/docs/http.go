package docs

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires the Docs module onto a mux receiving workspace paths.
// Every route requires the module to be installed and enabled.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces/{id}/docs", s.handleList)
	api.HandleFunc("POST /v1/workspaces/{id}/docs", s.handleCreate)
	api.HandleFunc("GET /v1/workspaces/{id}/docs/{did}", s.handleGet)
	api.HandleFunc("PATCH /v1/workspaces/{id}/docs/{did}", s.handleUpdate)
	api.HandleFunc("DELETE /v1/workspaces/{id}/docs/{did}", s.handleDelete)
}

func (s *Service) gate(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return "", "", false
	}
	wsID := r.PathValue("id")
	// SECURITY: membership before module status so strangers cannot probe
	// whether a workspace exists (security audit finding #4).
	if err := s.requireMember(r.Context(), user.ID, wsID); err != nil {
		apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return "", "", false
	}
	if !s.Enabled(r.Context(), wsID) {
		apijson.WriteError(w, http.StatusNotFound, "not_found", ErrModuleOff.Error())
		return "", "", false
	}
	return user.ID, wsID, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return false
	}
	return true
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.ListPages(r.Context(), userID, wsID, r.URL.Query().Get("projectId"))
	if err != nil {
		writeDocsError(w, err)
		return
	}
	if items == nil {
		items = []Page{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"pages": items})
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		ProjectID string `json:"projectId"`
		Title     string `json:"title"`
		Body      string `json:"body"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	p, err := s.CreatePage(r.Context(), userID, wsID, req.ProjectID, req.Title, req.Body)
	if err != nil {
		writeDocsError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"page": p})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	p, err := s.GetPage(r.Context(), userID, wsID, r.PathValue("did"))
	if err != nil {
		writeDocsError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"page": p})
}

func (s *Service) handleUpdate(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	p, err := s.UpdatePage(r.Context(), userID, wsID, r.PathValue("did"), req.Title, req.Body)
	if err != nil {
		writeDocsError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"page": p})
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	if err := s.DeletePage(r.Context(), userID, wsID, r.PathValue("did")); err != nil {
		writeDocsError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeDocsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrForbidden):
		apijson.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrModuleOff):
		apijson.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidTitle):
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
	}
}
