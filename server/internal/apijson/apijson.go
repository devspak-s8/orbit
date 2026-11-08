// Package apijson holds the shared JSON response conventions for the API:
// the unified error envelope and writers used by every domain.
package apijson

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Error is the unified error format for every API endpoint (PRD 8.7).
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details carries optional structured context, e.g. validation failures.
	Details map[string]any `json:"details,omitempty"`
}

// WriteJSON encodes body as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("encoding response", "error", err)
	}
}

// WriteError writes the unified error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, Error{Code: code, Message: message})
}