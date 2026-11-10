package automation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires workflow endpoints onto a mux that receives workspace
// paths. All routes sit behind identity.RequireUser; membership is checked
// in the service layer.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("POST /v1/workspaces/{id}/workflows", s.handleCreate)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows", s.handleList)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows/{wid}", s.handleGet)
	api.HandleFunc("POST /v1/workspaces/{id}/workflows/{wid}/validate", s.handleValidate)
	api.HandleFunc("POST /v1/workspaces/{id}/workflows/{wid}/run", s.handleRun)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows/{wid}/runs", s.handleListRuns)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows/{wid}/runs/{rid}", s.handleGetRun)
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req struct {
		Name       string     `json:"name"`
		Definition Definition `json:"definition"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	wf, err := s.Create(r.Context(), user.ID, r.PathValue("id"), req.Name, req.Definition)
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"workflow": wf})
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	items, err := s.List(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	if items == nil {
		items = []Workflow{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workflows": items})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	wf, err := s.Get(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (s *Service) handleValidate(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	wf, err := s.Validate(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (s *Service) handleRun(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	run, err := s.StartRun(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	// Accepted: the worker executes asynchronously; history shows progress.
	apijson.WriteJSON(w, http.StatusAccepted, map[string]any{"run": run})
}

func (s *Service) handleListRuns(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	items, err := s.ListRuns(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	if items == nil {
		items = []Run{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"runs": items})
}

func (s *Service) handleGetRun(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	run, steps, err := s.GetRun(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"), r.PathValue("rid"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	if steps == nil {
		steps = []Step{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"run": run, "steps": steps})
}

func writeAutomationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrWorkflowNotFound), errors.Is(err, ErrRunNotFound):
		apijson.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrNotReady):
		apijson.WriteError(w, http.StatusConflict, "not_ready", err.Error())
	case errors.Is(err, ErrForbidden):
		apijson.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrInvalidInput):
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrInvalidDefinition):
		apijson.WriteError(w, http.StatusBadRequest, "invalid_definition", err.Error())
	default:
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
	}
}
