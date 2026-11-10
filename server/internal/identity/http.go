package identity

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/orbit-org/orbit/server/internal/apijson"
)

const sessionCookieName = "orbit_session"

// Handlers wires identity endpoints onto the versioned API mux.
func (s *Service) Handlers(api *http.ServeMux) {
	limiter := newRateLimiter(10, time.Minute)
	api.HandleFunc("POST /v1/auth/register", s.rateLimit(limiter, s.handleRegister))
	api.HandleFunc("POST /v1/auth/login", s.rateLimit(limiter, s.handleLogin))
	api.HandleFunc("POST /v1/auth/logout", s.handleLogout)
	api.Handle("GET /v1/me", s.RequireUser(http.HandlerFunc(s.handleMe)))
}

// RequireUser guards a handler with session authentication.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		user, err := s.UserBySessionToken(r.Context(), raw.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
	})
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	user, token, err := s.Register(r.Context(), req.Email, req.DisplayName, req.Password)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	setSessionCookie(w, token, s.sessionTTL)
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	user, token, err := s.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	setSessionCookie(w, token, s.sessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.Logout(r.Context(), c.Value)
	}
	// Clear the cookie regardless of whether the session existed.
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

// --- helpers ---

type userKey struct{}

func withUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// UserFromContext returns the authenticated user, if the request passed RequireUser.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

func setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int(ttl.Seconds()),
		// Secure is set in production deployments via the reverse proxy
		// config; local development runs over plain HTTP.
	})
}

func (s *Service) rateLimit(limiter *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return false
	}
	return true
}

func writeIdentityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", err.Error())
	case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrSessionExpired):
		writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
	default:
		slog.Error("identity error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "an internal error occurred")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	apijson.WriteJSON(w, status, body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	apijson.WriteError(w, status, code, message)
}
