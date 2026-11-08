package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// apiError is the unified error format for every API endpoint (PRD 8.7).
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details carries optional structured context, e.g. validation failures.
	Details map[string]any `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encoding response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Code: code, Message: message})
}