package httpapi

import (
	"net/http"

	"github.com/orbit-org/orbit/server/internal/apijson"
)

// writeJSON and writeError are thin aliases kept for the httpapi package's
// own callers; the canonical helpers live in internal/apijson.
func writeJSON(w http.ResponseWriter, status int, body any) {
	apijson.WriteJSON(w, status, body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	apijson.WriteError(w, status, code, message)
}