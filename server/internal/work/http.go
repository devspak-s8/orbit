package work

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
)

// Handlers wires Work Management onto a mux that receives workspace paths.
// Every route requires the module to be installed and enabled; otherwise
// these paths behave as if they do not exist.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces/{id}/projects", s.handleListProjects)
	api.HandleFunc("POST /v1/workspaces/{id}/projects", s.handleCreateProject)
	api.HandleFunc("GET /v1/workspaces/{id}/projects/{pid}", s.handleGetProject)
	api.HandleFunc("GET /v1/workspaces/{id}/projects/{pid}/tasks", s.handleListTasks)
	api.HandleFunc("POST /v1/workspaces/{id}/projects/{pid}/tasks", s.handleCreateTask)
	api.HandleFunc("GET /v1/workspaces/{id}/projects/{pid}/labels", s.handleListLabels)
	api.HandleFunc("POST /v1/workspaces/{id}/projects/{pid}/labels", s.handleCreateLabel)
	api.HandleFunc("GET /v1/workspaces/{id}/tasks/{tid}", s.handleGetTask)
	api.HandleFunc("PATCH /v1/workspaces/{id}/tasks/{tid}", s.handleUpdateTask)
	api.HandleFunc("POST /v1/workspaces/{id}/tasks/{tid}/status", s.handleSetStatus)
	api.HandleFunc("PUT /v1/workspaces/{id}/tasks/{tid}/labels", s.handleSetTaskLabels)
	api.HandleFunc("GET /v1/workspaces/{id}/tasks/{tid}/comments", s.handleListComments)
	api.HandleFunc("POST /v1/workspaces/{id}/tasks/{tid}/comments", s.handleAddComment)
	api.HandleFunc("DELETE /v1/workspaces/{id}/tasks/{tid}", s.handleDeleteTask)
}

// gate checks the session and the module install state. Returns the caller
// id, workspace id, and whether to continue.
func (s *Service) gate(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return "", "", false
	}
	wsID := r.PathValue("id")
	if !s.Enabled(r.Context(), wsID) {
		apijson.WriteError(w, http.StatusNotFound, "not_found", ErrModuleDisabled.Error())
		return "", "", false
	}
	return user.ID, wsID, true
}

func (s *Service) handleListProjects(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.ListProjects(r.Context(), userID, wsID)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"projects": items})
}

func (s *Service) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	p, err := s.CreateProject(r.Context(), userID, wsID, req.Name, req.Description)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"project": p})
}

func (s *Service) handleGetProject(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	project, err := s.GetProject(r.Context(), userID, wsID, r.PathValue("pid"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	tasks, err := s.ListTasks(r.Context(), userID, wsID, project.ID, "")
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"project": project, "tasks": tasks})
}

func (s *Service) handleListTasks(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.ListTasks(r.Context(), userID, wsID, r.PathValue("pid"), r.URL.Query().Get("status"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"tasks": items})
}

func (s *Service) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
		AssigneeID  string `json:"assigneeId"`
		DueDate     string `json:"dueDate"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	t, err := s.CreateTask(r.Context(), userID, wsID, r.PathValue("pid"), req.Title, TaskPatch{
		Description: &req.Description,
		Priority:    &req.Priority,
		AssigneeID:  &req.AssigneeID,
		DueDate:     &req.DueDate,
	})
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"task": t})
}

func (s *Service) handleGetTask(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	t, err := s.GetTask(r.Context(), userID, wsID, r.PathValue("tid"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"task": t})
}

func (s *Service) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var patch TaskPatch
	if !decodeBody(w, r, &patch) {
		return
	}
	t, err := s.UpdateTask(r.Context(), userID, wsID, r.PathValue("tid"), patch)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"task": t})
}

func (s *Service) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	t, err := s.SetTaskStatus(r.Context(), userID, wsID, r.PathValue("tid"), req.Status)
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"task": t})
}

func (s *Service) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	if err := s.DeleteTask(r.Context(), userID, wsID, r.PathValue("tid")); err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Service) handleListLabels(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.ListLabels(r.Context(), userID, wsID, r.PathValue("pid"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if items == nil {
		items = []Label{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"labels": items})
}

func (s *Service) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	l, err := s.CreateLabel(r.Context(), userID, wsID, r.PathValue("pid"), req.Name, req.Color)
	if err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"label": l})
}

func (s *Service) handleSetTaskLabels(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		LabelIDs []string `json:"labelIds"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.LabelIDs == nil {
		req.LabelIDs = []string{}
	}
	if err := s.SetTaskLabels(r.Context(), userID, wsID, r.PathValue("tid"), req.LabelIDs); err != nil {
		writeWorkError(w, err)
		return
	}
	t, err := s.GetTask(r.Context(), userID, wsID, r.PathValue("tid"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"task": t})
}

func (s *Service) handleListComments(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.ListComments(r.Context(), userID, wsID, r.PathValue("tid"))
	if err != nil {
		writeWorkError(w, err)
		return
	}
	if items == nil {
		items = []Comment{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"comments": items})
}

func (s *Service) handleAddComment(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	c, err := s.AddComment(r.Context(), userID, wsID, r.PathValue("tid"), req.Body)
	if err != nil {
		if err.Error() == "comment must be 1-4000 characters" || strings.Contains(err.Error(), "label") {
			apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeWorkError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"comment": c})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return false
	}
	return true
}

func writeWorkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrForbidden):
		apijson.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrModuleDisabled):
		apijson.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidTitle), errors.Is(err, ErrInvalidName),
		errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidPriority), errors.Is(err, ErrInvalidDate):
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
	}
}
