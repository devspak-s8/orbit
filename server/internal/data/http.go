package data

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
	"github.com/orbit-org/orbit/server/internal/identity"
	"github.com/orbit-org/orbit/server/internal/ratelimit"
)

// formLimiter throttles public form submissions per source IP — the same
// posture as webhook deliveries.
var formLimiter = ratelimit.New(60, 5)

// Handlers wires the Data module's member routes onto a mux that
// receives workspace paths; every route requires the module to be
// installed and enabled.
func (s *Service) Handlers(api *http.ServeMux) {
	api.HandleFunc("GET /v1/workspaces/{id}/tables", s.handleListTables)
	api.HandleFunc("POST /v1/workspaces/{id}/tables", s.handleCreateTable)
	api.HandleFunc("GET /v1/workspaces/{id}/tables/{tid}", s.handleGetTable)
	api.HandleFunc("POST /v1/workspaces/{id}/tables/{tid}/columns", s.handleAddColumn)
	api.HandleFunc("PATCH /v1/workspaces/{id}/columns/{cid}", s.handleRenameColumn)
	api.HandleFunc("DELETE /v1/workspaces/{id}/columns/{cid}", s.handleDeleteColumn)
	api.HandleFunc("GET /v1/workspaces/{id}/tables/{tid}/records", s.handleListRecords)
	api.HandleFunc("POST /v1/workspaces/{id}/tables/{tid}/records", s.handleCreateRecord)
	api.HandleFunc("POST /v1/workspaces/{id}/tables/{tid}/import", s.handleImportCSV)
	api.HandleFunc("GET /v1/workspaces/{id}/tables/{tid}/export", s.handleExportCSV)
	api.HandleFunc("POST /v1/workspaces/{id}/tables/{tid}/form", s.handleEnableForm)
	api.HandleFunc("PATCH /v1/workspaces/{id}/records/{rid}", s.handleUpdateRecord)
	api.HandleFunc("DELETE /v1/workspaces/{id}/records/{rid}", s.handleDeleteRecord)
}

// FormHandlers wires the unauthenticated public routes onto the root mux:
// submissions and the shape spec a public form renders from.
func (s *Service) FormHandlers(root *http.ServeMux) {
	root.HandleFunc("POST /forms/{token}", s.handleFormSubmit)
	root.HandleFunc("GET /forms/{token}/shape", s.handleFormShape)
}

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

func (s *Service) handleListTables(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	items, err := s.ListTables(r.Context(), userID, wsID)
	if err != nil {
		writeDataError(w, err)
		return
	}
	if items == nil {
		items = []Table{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"tables": items})
}

func (s *Service) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Name    string   `json:"name"`
		Columns []Column `json:"columns"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Columns == nil {
		req.Columns = []Column{}
	}
	t, err := s.CreateTable(r.Context(), userID, wsID, req.Name, req.Columns)
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"table": t})
}

func (s *Service) handleGetTable(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	t, err := s.GetTable(r.Context(), userID, wsID, r.PathValue("tid"))
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"table": t})
}

func (s *Service) handleAddColumn(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var c Column
	if !decodeBody(w, r, &c) {
		return
	}
	col, err := s.AddColumn(r.Context(), userID, wsID, r.PathValue("tid"), c)
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"column": col})
}

func (s *Service) handleListRecords(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	items, err := s.ListRecords(r.Context(), userID, wsID, r.PathValue("tid"),
		q.Get("filterColumn"), q.Get("filterValue"))
	if err != nil {
		writeDataError(w, err)
		return
	}
	if items == nil {
		items = []Record{}
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"records": items})
}

func (s *Service) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Values map[string]any `json:"values"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	rec, err := s.CreateRecord(r.Context(), userID, wsID, r.PathValue("tid"), req.Values, "ui")
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"record": rec})
}

func (s *Service) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		CSV string `json:"csv"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	count, err := s.ImportCSV(r.Context(), userID, wsID, r.PathValue("tid"), req.CSV)
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"imported": count})
}

func (s *Service) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	csvText, err := s.ExportCSV(r.Context(), userID, wsID, r.PathValue("tid"))
	if err != nil {
		writeDataError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="orbit-export.csv"`)
	_, _ = w.Write([]byte(csvText))
}

func (s *Service) handleEnableForm(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	t, err := s.EnableForm(r.Context(), userID, wsID, r.PathValue("tid"))
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"table": t})
}

func (s *Service) handleUpdateRecord(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Values map[string]any `json:"values"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	rec, err := s.UpdateRecord(r.Context(), userID, wsID, r.PathValue("rid"), req.Values)
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"record": rec})
}

func (s *Service) handleDeleteRecord(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	if err := s.DeleteRecord(r.Context(), userID, wsID, r.PathValue("rid")); err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Service) handleRenameColumn(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	col, err := s.RenameColumn(r.Context(), userID, wsID, r.PathValue("cid"), req.Name)
	if err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"column": col})
}

func (s *Service) handleDeleteColumn(w http.ResponseWriter, r *http.Request) {
	userID, wsID, ok := s.gate(w, r)
	if !ok {
		return
	}
	if err := s.DeleteColumn(r.Context(), userID, wsID, r.PathValue("cid")); err != nil {
		writeDataError(w, err)
		return
	}
	apijson.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleFormShape serves the public column spec (GET, no auth — the
// token is the authorization, exactly like the submit path).
func (s *Service) handleFormShape(w http.ResponseWriter, r *http.Request) {
	shape, err := s.FormShape(r.Context(), r.PathValue("token"))
	if err != nil {
		apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	apijson.WriteJSON(w, http.StatusOK, shape)
}

// handleFormSubmit is the public path: any JSON object becomes a record.
func (s *Service) handleFormSubmit(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !formLimiter.Allow(ip) {
		w.Header().Set("Retry-After", "60")
		apijson.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many submissions, try again later")
		return
	}
	var values map[string]any
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil || values == nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "expected a JSON object")
		return
	}
	rec, err := s.SubmitForm(r.Context(), r.PathValue("token"), values)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if errors.Is(err, ErrInvalidValue) {
			apijson.WriteError(w, http.StatusBadRequest, "invalid_values", err.Error())
			return
		}
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
		return
	}
	apijson.WriteJSON(w, http.StatusCreated, map[string]any{"record": rec})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return false
	}
	return true
}

func writeDataError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apijson.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrForbidden):
		apijson.WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrModuleDisabled):
		apijson.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidColumn), errors.Is(err, ErrInvalidValue):
		apijson.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		apijson.WriteError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
	}
}
