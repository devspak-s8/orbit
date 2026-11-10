package modules

import (
	"errors"
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires module endpoints onto a mux that receives workspace paths.
// All routes sit behind identity.RequireUser in the router; owner checks
// happen in the service layer.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces/{id}/modules", s.handleAvailable)
	api.HandleFunc("POST /v1/workspaces/{id}/modules/{mid}/install", s.handleInstall)
	api.HandleFunc("POST /v1/workspaces/{id}/modules/{mid}/enable", s.handleEnable)
	api.HandleFunc("POST /v1/workspaces/{id}/modules/{mid}/disable", s.handleDisable)
	api.HandleFunc("DELETE /v1/workspaces/{id}/modules/{mid}", s.handleUninstall)
}

func (s *Service) handleAvailable(w http.ResponseWriter, r *http.Request) {
	if _, ok := identity.UserFromContext(r.Context()); !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	items, err := s.Available(r.Context(), r.PathValue("id"))
	if err != nil {
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	if items == nil {
		items = []Item{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"modules": items})
}

func (s *Service) handleInstall(w http.ResponseWriter, r *http.Request) {
	s.handleLifecycle(w, r, func(userID, wsID, mid string) (any, error) {
		item, err := s.Install(r.Context(), userID, wsID, mid)
		return item, err
	}, http.StatusCreated)
}

func (s *Service) handleEnable(w http.ResponseWriter, r *http.Request) {
	s.handleLifecycle(w, r, func(userID, wsID, mid string) (any, error) {
		item, err := s.Enable(r.Context(), userID, wsID, mid)
		return item, err
	}, http.StatusOK)
}

func (s *Service) handleDisable(w http.ResponseWriter, r *http.Request) {
	s.handleLifecycle(w, r, func(userID, wsID, mid string) (any, error) {
		item, err := s.Disable(r.Context(), userID, wsID, mid)
		return item, err
	}, http.StatusOK)
}

func (s *Service) handleUninstall(w http.ResponseWriter, r *http.Request) {
	s.handleLifecycle(w, r, func(userID, wsID, mid string) (any, error) {
		return map[string]any{"status": "ok"}, s.Uninstall(r.Context(), userID, wsID, mid)
	}, http.StatusOK)
}

type lifecycleFn func(userID, wsID, mid string) (any, error)

func (s *Service) handleLifecycle(w http.ResponseWriter, r *http.Request, fn lifecycleFn, okStatus int) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	result, err := fn(user.ID, r.PathValue("id"), r.PathValue("mid"))
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			apijson.WriteError(w, http.StatusNotFound, "not_found", err.Error())
		case errors.Is(err, ErrNotInstalled):
			apijson.WriteError(w, http.StatusConflict, "not_installed", err.Error())
		case errors.Is(err, ErrForbidden):
			apijson.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
		default:
			apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		}
		return
	}
	body := map[string]any{}
	if result != nil {
		body["module"] = result
	}
	apijson.WriteJSON(w, okStatus, body)
}