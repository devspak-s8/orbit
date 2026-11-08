package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMeEndpointRequiresSession guards against the regression where
// /v1/me was registered without the RequireUser middleware and therefore
// always returned 401.
func TestMeEndpointRequiresSession(t *testing.T) {
	svc, email, cleanup := setupService(t)
	defer cleanup()

	user, token, err := svc.Register(context.Background(), email, "Me Test", "password123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	api := http.NewServeMux()
	svc.Handlers(api)

	t.Run("unauthenticated", func(t *testing.T) {
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/me", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			User User `json:"user"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.User.ID != user.ID {
			t.Errorf("user id = %s, want %s", body.User.ID, user.ID)
		}
	})
}