package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/ratelimit"
)

// hookLimiter throttles webhook deliveries per source IP. Webhooks are
// unauthenticated by design (the secret is the URL), so this is the front
// line against scraping and replay abuse.
var hookLimiter = ratelimit.New(60, time.Minute)

// HookHandlers wires the top-level (unauthenticated) webhook routes onto
// the root mux. Tokens carry the secret; every delivery is rate-limited
// and audited.
func (s *Service) HookHandlers(root *http.ServeMux) {
	root.HandleFunc("POST /hooks/{token}", s.handleWebhook)
}

// CatalogHandlers wires the connector-preset catalog onto a mux mounted
// behind identity.RequireUser. The catalog is static (ships with the
// binary), so it is user-scoped, not workspace-scoped.
func (s *Service) CatalogHandlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/connectors", func(w http.ResponseWriter, r *http.Request) {
		apijson.WriteJSON(w, http.StatusOK, map[string]any{"connectors": WireCatalog()})
	})
}

func (s *Service) handleWebhook(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !hookLimiter.Allow(ip) {
		w.Header().Set("Retry-After", "60")
		apijson.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many deliveries, try again later")
		return
	}

	token := r.PathValue("token")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var parsed any
	if err := json.NewDecoder(r.Body).Decode(&parsed); err != nil {
		// An empty body is a valid webhook: deliver an empty payload.
		parsed = map[string]any{}
	}
	payload, ok := parsed.(map[string]any)
	if !ok {
		// Non-object bodies (strings, arrays, numbers) are wrapped so
		// substitution still has something to read.
		payload = map[string]any{"value": parsed}
	}

	run, wf, err := s.EnqueueWebhook(r.Context(), token, payload)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			// Unknown token, non-ready workflow, or a trigger that is not a
			// webhook: identical response, no existence leak.
			apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	// Delivery accepted; execution is asynchronous like every trigger.
	apijson.WriteJSON(w, http.StatusAccepted, map[string]any{"runId": run.ID, "workflow": wf.ID})
}

// Handlers wires workflow endpoints onto a mux that receives workspace
// paths. All routes sit behind identity.RequireUser; membership is checked
// in the service layer.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("POST /v1/workspaces/{id}/workflows", s.handleCreate)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows", s.handleList)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows/{wid}", s.handleGet)
	api.HandleFunc("PATCH /v1/workspaces/{id}/workflows/{wid}", s.handleUpdate)
	api.HandleFunc("POST /v1/workspaces/{id}/workflows/{wid}/validate", s.handleValidate)
	api.HandleFunc("POST /v1/workspaces/{id}/workflows/{wid}/run", s.handleRun)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows/{wid}/runs", s.handleListRuns)
	api.HandleFunc("GET /v1/workspaces/{id}/workflows/{wid}/runs/{rid}", s.handleGetRun)
	api.HandleFunc("POST /v1/workspaces/{id}/workflows/{wid}/runs/{rid}/cancel", s.handleCancel)
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

func (s *Service) handleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req struct {
		Name       *string     `json:"name"`
		Definition *Definition `json:"definition"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	wf, err := s.UpdateWorkflow(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"), req.Name, req.Definition)
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

func (s *Service) handleCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		apijson.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if err := s.CancelRun(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"), r.PathValue("rid")); err != nil {
		writeAutomationError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
	items, err := s.ListRuns(r.Context(), user.ID, r.PathValue("id"), r.PathValue("wid"),
		intFromQuery(r, "limit", 50), r.URL.Query().Get("before"))
	if err != nil {
		writeAutomationError(w, err)
		return
	}
	if items == nil {
		items = []Run{}
	}
	var nextCursor string
	if len(items) > 0 {
		nextCursor = items[len(items)-1].ID
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"runs": items, "nextCursor": nextCursor})
}

func intFromQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
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
