package httpapi_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

// TestSecurityAudit is a live end-to-end security pass against the running
// API (localhost:8080). It skips when the API is not running, so the offline
// test suite is unaffected. Coverage:
//
//  1. auth bypass        — every protected route without credentials
//  2. tenant isolation   — the FULL workspace route inventory, cross-tenant
//  3. injection          — SQL/XSS payloads in write endpoints
//  4. token forgery      — malformed/forged Bearer tokens
//  5. MCP auth           — JSON-RPC without and with forged credentials
//  6. webhook no-leak    — uniform 404 for unknown hook tokens
//  7. payload limits     — oversized bodies rejected
//  8. delete protection  — wrong confirmation, non-owner
//  9. bearer lifecycle   — valid token works, never crosses tenants
// 10. cookie flags       — HttpOnly + SameSite on session cookies
// 11. member vs owner    — invited member reads OK, owner actions 403
//
// Sections run in order; section 11 mutates membership, so it stays last.

func TestSecurityAudit(t *testing.T) {
	base := "http://localhost:8080"

	// Skip when the API is not running (offline suite runs).
	resp, err := http.Get(base + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Skipf("API not running at %s — start with `go run ./cmd/api`: %v", base, err)
	}
	resp.Body.Close()

	stamp := time.Now().UnixNano()
	emailA := fmt.Sprintf("sec-a-%d@orbit.test", stamp)
	emailB := fmt.Sprintf("sec-b-%d@orbit.test", stamp)
	password := "password123"
	fakeID := "00000000-0000-0000-0000-000000000000"

	jarA, _ := cookiejar.New(nil)
	jarB, _ := cookiejar.New(nil)
	clientA := &http.Client{Jar: jarA}
	clientB := &http.Client{Jar: jarB}

	userA := registerUser(t, clientA, base, emailA, "Sec A", password)
	registerUser(t, clientB, base, emailB, "Sec B", password)

	wsA := createWorkspace(t, clientA, base, fmt.Sprintf("SecAuditA-%d", stamp))
	wsB := createWorkspace(t, clientB, base, fmt.Sprintf("SecAuditB-%d", stamp))

	// Enable all modules in workspace A (A is the owner).
	for _, m := range []string{"orbit.work", "orbit.data", "orbit.docs", "orbit.demo"} {
		mustStatus(t, clientA, "POST", base+fmt.Sprintf("/v1/workspaces/%s/modules/%s/install", wsA, m), "{}", 201)
		mustStatus(t, clientA, "POST", base+fmt.Sprintf("/v1/workspaces/%s/modules/%s/enable", wsA, m), "{}", 200)
	}

	// Seed one project in wsA so cross-tenant reads have data behind them.
	mustStatus(t, clientA, "POST", base+fmt.Sprintf("/v1/workspaces/%s/projects", wsA),
		`{"name":"Audit Project"}`, 201)
	seed := reqJSON(t, clientA, "GET", base+fmt.Sprintf("/v1/workspaces/%s/projects", wsA), "")
	var projList struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(seed, &projList); err != nil || len(projList.Projects) == 0 {
		t.Fatalf("seed project missing: %v %s", err, seed)
	}
	projA := projList.Projects[0].ID

	// Mint an API token for user A.
	var tok struct {
		Secret string `json:"secret"`
	}
	raw := reqJSON(t, clientA, "POST", base+"/v1/tokens", `{"name":"sec-audit"}`)
	if err := json.Unmarshal(raw, &tok); err != nil || tok.Secret == "" {
		t.Fatalf("token create failed: %v %s", err, raw)
	}

	t.Run("1_auth_bypass", func(t *testing.T) {
		noAuthRoutes := []struct{ method, path string }{
			{"GET", "/v1/me"},
			{"GET", "/v1/workspaces"},
			{"POST", "/v1/workspaces"},
			{"GET", "/v1/notifications"},
			{"POST", "/v1/notifications/read-all"},
			{"GET", "/v1/tokens"},
			{"POST", "/v1/tokens"},
			{"GET", fmt.Sprintf("/v1/workspaces/%s", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/members", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/projects", wsA)},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/projects", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/tables", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/docs", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/workflows", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/modules", wsA)},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/events", wsA)},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.work/install", wsA)},
			{"GET", "/v1/connectors"},
			{"GET", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/evals", wsA, fakeID)},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/evals", wsA, fakeID)},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/evals/%s/run", wsA, fakeID, fakeID)},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/runs/%s/retry", wsA, fakeID, fakeID)},
		}
		for _, r := range noAuthRoutes {
			req, _ := http.NewRequest(r.method, base+r.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("%s %s: request error: %v", r.method, r.path, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Errorf("SECURITY: %s %s without auth = %d, want 401", r.method, r.path, resp.StatusCode)
			}
		}
	})

	t.Run("2_tenant_isolation", func(t *testing.T) {
		// Every workspace-scoped route, hit by an authenticated stranger.
		reads := []string{
			fmt.Sprintf("/v1/workspaces/%s", wsA),
			fmt.Sprintf("/v1/workspaces/%s/members", wsA),
			fmt.Sprintf("/v1/workspaces/%s/modules", wsA),
			fmt.Sprintf("/v1/workspaces/%s/events", wsA),
			fmt.Sprintf("/v1/workspaces/%s/modules/demo/records", wsA),
			fmt.Sprintf("/v1/workspaces/%s/projects", wsA),
			fmt.Sprintf("/v1/workspaces/%s/projects/%s", wsA, projA),
			fmt.Sprintf("/v1/workspaces/%s/tables", wsA),
			fmt.Sprintf("/v1/workspaces/%s/docs", wsA),
			fmt.Sprintf("/v1/workspaces/%s/workflows", wsA),
		}
		for _, path := range reads {
			resp, err := clientB.Get(base + path)
			if err != nil {
				t.Errorf("GET %s: %v", path, err)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode >= 500 {
				t.Errorf("SECURITY: stranger read %s = %d! body: %s", path, resp.StatusCode, body)
			}
		}

		writes := []struct{ method, path, body string }{
			{"POST", fmt.Sprintf("/v1/workspaces/%s/projects", wsA), `{"name":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/tables", wsA), `{"name":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/docs", wsA), `{"title":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows", wsA), `{"name":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/demo/records", wsA), `{"title":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.work/install", wsA), `{}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.work/enable", wsA), `{}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.work/disable", wsA), `{}`},
			{"DELETE", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.work", wsA), ""},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/invites", wsA), `{}`},
			{"PATCH", fmt.Sprintf("/v1/workspaces/%s", wsA), `{"name":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/evals", wsA, fakeID), `{"name":"Evil"}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/evals/%s/run", wsA, fakeID, fakeID), `{}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/workflows/%s/runs/%s/retry", wsA, fakeID, fakeID), `{}`},
		}
		for _, w := range writes {
			req, _ := http.NewRequest(w.method, base+w.path, strings.NewReader(w.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := clientB.Do(req)
			if err != nil {
				t.Errorf("%s %s: %v", w.method, w.path, err)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 400 || resp.StatusCode >= 500 {
				t.Errorf("SECURITY: stranger %s %s = %d! body: %s", w.method, w.path, resp.StatusCode, body)
			}
		}

		// Workspace lists must never leak the stranger's target.
		resp, _ := clientB.Get(base + "/v1/workspaces")
		var list struct {
			Workspaces []struct {
				ID string `json:"id"`
			} `json:"workspaces"`
		}
		json.NewDecoder(resp.Body).Decode(&list)
		resp.Body.Close()
		for _, ws := range list.Workspaces {
			if ws.ID == wsA {
				t.Error("SECURITY: stranger sees the target workspace in their list")
			}
		}
	})

	t.Run("3_injection", func(t *testing.T) {
		injections := []string{
			`'; DROP TABLE users; --`,
			`' OR '1'='1`,
			`1; DELETE FROM workspaces WHERE 1=1; --`,
			`{{constructor.constructor('return 1')()}}`,
			`<script>alert(1)</script>`,
		}
		for _, inj := range injections {
			body, _ := json.Marshal(map[string]any{"name": inj})
			resp, err := clientA.Post(base+"/v1/workspaces", "application/json", strings.NewReader(string(body)))
			if err != nil {
				continue
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				t.Errorf("SECURITY: injection %q caused %d: %s", inj, resp.StatusCode, respBody)
			}
			// The database must stay alive after every attempt.
			hc, err := http.Get(base + "/healthz")
			if err != nil {
				t.Fatalf("injection %q broke the database: %v", inj, err)
			}
			hc.Body.Close()
			if hc.StatusCode != 200 {
				t.Errorf("SECURITY: injection %q broke the database (healthz %d)", inj, hc.StatusCode)
			}
		}
	})

	t.Run("4_token_forgery", func(t *testing.T) {
		forged := []string{
			"ob_fake",
			"ob_" + strings.Repeat("A", 43),
			"' OR '1'='1",
			"ob_00000000000000000000000000000000000000000000",
			strings.Repeat("A", 200),
		}
		for _, tok := range forged {
			req, _ := http.NewRequest("GET", base+"/v1/me", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Errorf("SECURITY: forged token %q authenticated!", tok)
			}
		}
	})

	t.Run("5_mcp_auth", func(t *testing.T) {
		mcpBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

		resp, err := http.Post(base+"/mcp", "application/json", strings.NewReader(mcpBody))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Errorf("SECURITY: MCP without auth = %d, want 401", resp.StatusCode)
			}
		}

		req, _ := http.NewRequest("POST", base+"/mcp", strings.NewReader(mcpBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer ob_forged")
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Error("SECURITY: MCP accepted forged Bearer token")
			}
		}
	})

	t.Run("6_webhook_no_leak", func(t *testing.T) {
		r1 := postRaw(t, base+"/hooks/definitely-not-real", `{"x":1}`)
		r2 := postRaw(t, base+"/hooks/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", `{"x":1}`)
		if r1 != r2 {
			t.Errorf("SECURITY: webhook 404 responses differ:\n  %s\n  %s", r1, r2)
		}
	})

	t.Run("7_payload_limits", func(t *testing.T) {
		big := strings.Repeat("a", 2<<20)
		body := fmt.Sprintf(`{"email":"%s@t.co","password":"x12345678"}`, big)
		resp, err := http.Post(base+"/v1/auth/register", "application/json", strings.NewReader(body))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 201 {
				t.Error("SECURITY: 2MB registration body accepted")
			}
		}
	})

	t.Run("8_workspace_delete_protection", func(t *testing.T) {
		// Wrong confirmation name must fail.
		resp, err := doJSON(t, clientA, "DELETE", base+fmt.Sprintf("/v1/workspaces/%s", wsA), `{"confirmName":"WRONG"}`)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Error("SECURITY: workspace deleted with wrong confirmation name!")
			}
		}
		// Non-owner must not delete.
		resp, err = doJSON(t, clientB, "DELETE", base+fmt.Sprintf("/v1/workspaces/%s", wsA), `{"confirmName":"anything"}`)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Error("SECURITY: non-owner deleted another user's workspace!")
			}
		}
		// Correct name from the real owner must succeed — use wsB (B owns it).
		wsBName := fmt.Sprintf("SecAuditB-%d", stamp)
		resp, err = doJSON(t, clientB, "DELETE", base+fmt.Sprintf("/v1/workspaces/%s", wsB), fmt.Sprintf(`{"confirmName":%q}`, wsBName))
		if err != nil {
			t.Fatalf("owner delete: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("owner delete with correct name = %d, want 200 (control check)", resp.StatusCode)
		}
	})

	t.Run("9_bearer_lifecycle", func(t *testing.T) {
		// Valid token identifies user A.
		req, _ := http.NewRequest("GET", base+"/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+tok.Secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("bearer /me: %v", err)
		}
		var me struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		json.NewDecoder(resp.Body).Decode(&me)
		resp.Body.Close()
		if resp.StatusCode != 200 || me.User.ID != userA {
			t.Errorf("SECURITY: valid Bearer token got %d (user %q, want %q)", resp.StatusCode, me.User.ID, userA)
		}

		// Token must not reach workspaces A has no membership in. wsB was
		// deleted in section 8, so use a fresh stranger workspace.
		strangerWS := createWorkspace(t, clientB, base, fmt.Sprintf("SecAuditB2-%d", stamp))
		req, _ = http.NewRequest("GET", base+fmt.Sprintf("/v1/workspaces/%s/projects", strangerWS), nil)
		req.Header.Set("Authorization", "Bearer "+tok.Secret)
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Error("SECURITY: user A's token accessed a stranger's workspace!")
			}
		}
	})

	t.Run("10_cookie_flags", func(t *testing.T) {
		// Raw client (no jar) so we can inspect Set-Cookie.
		body := fmt.Sprintf(`{"email":"sec-c-%d@orbit.test","password":"%s","displayName":"Sec C"}`, stamp, password)
		resp, err := http.Post(base+"/v1/auth/register", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		resp.Body.Close()
		var cookie string
		for _, c := range resp.Header.Values("Set-Cookie") {
			if strings.HasPrefix(c, "orbit_session=") {
				cookie = c
			}
		}
		if cookie == "" {
			t.Fatal("no session cookie on register response")
		}
		if !strings.Contains(cookie, "HttpOnly") {
			t.Errorf("SECURITY: session cookie missing HttpOnly: %s", cookie)
		}
		if !strings.Contains(cookie, "SameSite=Lax") {
			t.Errorf("SECURITY: session cookie missing SameSite=Lax: %s", cookie)
		}
	})

	t.Run("11_member_vs_owner", func(t *testing.T) {
		// Owner mints an invite; B redeems and becomes a member.
		raw := reqJSON(t, clientA, "POST", base+fmt.Sprintf("/v1/workspaces/%s/invites", wsA), `{}`)
		var inv struct {
			Invite struct {
				Code string `json:"code"`
			} `json:"invite"`
		}
		if err := json.Unmarshal(raw, &inv); err != nil || inv.Invite.Code == "" {
			t.Fatalf("create invite: %v %s", err, raw)
		}
		mustStatus(t, clientB, "POST", base+fmt.Sprintf("/v1/invites/%s/redeem", inv.Invite.Code), "{}", 200)

		// Member reads must now succeed.
		memberReads := []string{
			fmt.Sprintf("/v1/workspaces/%s", wsA),
			fmt.Sprintf("/v1/workspaces/%s/members", wsA),
			fmt.Sprintf("/v1/workspaces/%s/modules", wsA),
			fmt.Sprintf("/v1/workspaces/%s/events", wsA),
			fmt.Sprintf("/v1/workspaces/%s/modules/demo/records", wsA),
			fmt.Sprintf("/v1/workspaces/%s/projects", wsA),
		}
		for _, path := range memberReads {
			resp, err := clientB.Get(base + path)
			if err != nil {
				t.Errorf("member GET %s: %v", path, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Errorf("member GET %s = %d, want 200 (fix over-blocks members)", path, resp.StatusCode)
			}
		}

		// Owner-only actions must still be forbidden for a plain member.
		ownerOnly := []struct{ method, path, body string }{
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.data/install", wsA), `{}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.data/enable", wsA), `{}`},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.work/disable", wsA), `{}`},
			{"DELETE", fmt.Sprintf("/v1/workspaces/%s/modules/orbit.docs", wsA), ""},
			{"POST", fmt.Sprintf("/v1/workspaces/%s/invites", wsA), `{}`},
			{"PATCH", fmt.Sprintf("/v1/workspaces/%s", wsA), `{"name":"Member Takeover"}`},
			{"DELETE", fmt.Sprintf("/v1/workspaces/%s", wsA), `{"confirmName":"whatever"}`},
		}
		for _, o := range ownerOnly {
			req, _ := http.NewRequest(o.method, base+o.path, strings.NewReader(o.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := clientB.Do(req)
			if err != nil {
				t.Errorf("member %s %s: %v", o.method, o.path, err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != 403 {
				t.Errorf("SECURITY: member %s %s = %d, want 403 (owner-only action)", o.method, o.path, resp.StatusCode)
			}
		}
	})
}

// --- helpers ---

func registerUser(t *testing.T, client *http.Client, base, email, name, password string) string {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"password":%q,"displayName":%q}`, email, password, name)
	resp, err := client.Post(base+"/v1/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register %s: %d %s", email, resp.StatusCode, b)
	}
	var out struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("register decode: %v", err)
	}
	return out.User.ID
}

func createWorkspace(t *testing.T, client *http.Client, base, name string) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q}`, name)
	resp, err := client.Post(base+"/v1/workspaces", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create workspace: %d %s", resp.StatusCode, b)
	}
	var out struct {
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("workspace decode: %v", err)
	}
	return out.Workspace.ID
}

func doJSON(t *testing.T, client *http.Client, method, url, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

func reqJSON(t *testing.T, client *http.Client, method, url, body string) []byte {
	t.Helper()
	resp, err := doJSON(t, client, method, url, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return b
}

func mustStatus(t *testing.T, client *http.Client, method, url, body string, want int) {
	t.Helper()
	resp, err := doJSON(t, client, method, url, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s = %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
}

func postRaw(t *testing.T, url, body string) string {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
