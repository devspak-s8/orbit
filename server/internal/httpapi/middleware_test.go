package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestIDFromContext(r.Context()) == "" {
			t.Error("request id missing from context")
		}
		w.WriteHeader(http.StatusOK)
	})

	h := withRequestID(next)
	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if id := rec.Header().Get("X-Request-ID"); id == "" {
		t.Error("response missing X-Request-ID header")
	}
}

func TestRequestIDReusesInboundHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromContext(r.Context()); got != "abc-123" {
			t.Errorf("context request id = %q, want abc-123", got)
		}
	})

	h := withRequestID(next)
	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Errorf("response request id = %q, want abc-123", got)
	}
}

func TestCORSMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	})
	h := withCORS([]string{"http://localhost:5173"}, next)

	t.Run("allowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Errorf("allow-origin = %q, want http://localhost:5173", got)
		}
	})

	t.Run("disallowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.Header.Set("Origin", "http://evil.example")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("allow-origin = %q, want empty", got)
		}
	})

	t.Run("preflight short-circuits", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/v1/ping", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rec.Code)
		}
	})
}

// Ensure the log middleware never panics on normal requests.
func TestLoggingMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := withRequestID(withLogging(logger, next))

	req := httptest.NewRequest(http.MethodPost, "/v1/ping", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}