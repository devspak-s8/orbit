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
	api.HandleFunc("PATCH /v1/workspaces/{id}", s.handleRename)
	api.HandleFunc("DELETE /v1/workspaces/{id}", s.handleDeleteWorkspace)
	api.HandleFunc("GET /v1/workspaces/{id}/members", s.handleListMembers)
	api.HandleFunc("POST /v1/workspaces/{id}/invites", s.handleCreateInvite)
}

// HandlersUser wires user-scoped routes (not under /v1/workspaces). Callers
// mount these behind identity.RequireUser on the top-level API mux.
func (s *Service) HandlersUser(api *http.ServeMux) {
	api.HandleFunc("POST /v1/invites/{code}/redeem", s.handleRedeemInvite)
}

func (s *Service) handleRename(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	ws, err := s.UpdateWorkspace(r.Context(), user.ID, r.PathValue("id"), req.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			apijson.WriteError(w, http.StatusForbidden, "forbidden", "only owners can rename")
		case errors.Is(err, ErrNotFound):
			apijson.WriteError(w, http.StatusNotFound, "not_found", "workspace not found")
		case errors.Is(err, ErrInvalidName):
			apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workspace": ws})
}

func (s *Service) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req struct {
		ConfirmName string `json:"confirmName"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if err := s.DeleteWorkspace(r.Context(), user.ID, r.PathValue("id"), req.ConfirmName); err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			apijson.WriteError(w, http.StatusForbidden, "forbidden", "only owners can delete")
		case errors.Is(err, ErrNotFound):
			apijson.WriteError(w, http.StatusNotFound, "not_found", "workspace not found")
		default:
			apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return false
	}
	return true
}

func (s *Service) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	invite, err := s.CreateInvite(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			apijson.WriteError(w, http.StatusForbidden, "forbidden", "only owners can create invites")
		case errors.Is(err, ErrNotFound):
			apijson.WriteError(w, http.StatusNotFound, "not_found", "workspace not found")
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"invite": invite})
}

func (s *Service) handleRedeemInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	ws, err := s.RedeemInvite(r.Context(), user.ID, r.PathValue("code"))
	if err != nil {
		switch {
		case errors.Is(err, ErrInviteInvalid):
			apijson.WriteError(w, http.StatusNotFound, "not_found", "invite is invalid or has expired")
		case errors.Is(err, ErrAlreadyMember):
			apijson.WriteError(w, http.StatusConflict, "already_member", err.Error())
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workspace": ws})
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

func (s *Service) handleListMembers(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	items, err := s.ListMembers(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apijson.WriteError(w, http.StatusNotFound, "not_found", "workspace not found")
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	if items == nil {
		items = []Member{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"members": items})
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
