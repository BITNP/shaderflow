package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shaderflow/internal/game"
)

func testServer(t *testing.T) *Server {
	return New(Config{AdminPassword: "secret", DataDir: t.TempDir(), TrustedProxyCIDRs: []string{"192.0.2.0/24"}})
}
func adminCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "https://wall.test/api/admin/login", strings.NewReader(`{"password":"secret"}`))
	req.RemoteAddr = "192.0.2.1:1000"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login=%d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("bad cookie %#v", cookies)
	}
	return cookies[0]
}
func playerRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, "https://wall.test"+path, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.10:4000"
	req.Header.Set("X-Forwarded-User", "alice")
	return req
}

func TestOnlyTrustedProxyHeaderAuthenticatesPlayer(t *testing.T) {
	s := testServer(t)
	req := playerRequest(http.MethodGet, "/api/auth/me", "")
	req.RemoteAddr = "198.51.100.5:4000"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed header status=%d", w.Code)
	}
	req = playerRequest(http.MethodGet, "/api/auth/me", "")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("trusted proxy status=%d %s", w.Code, w.Body.String())
	}
}
func TestCandidateNeedsAdminPromotion(t *testing.T) {
	s := testServer(t)
	req := playerRequest(http.MethodPost, "/api/candidate", `{"version":1,"source":"fn userfn(x:f32,y:f32,t:f32)->f32{return x;}"}`)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("candidate=%d %s", w.Code, w.Body.String())
	}
	admin := adminCookie(t, s)
	stateReq := httptest.NewRequest(http.MethodGet, "https://wall.test/api/state", nil)
	stateReq.AddCookie(admin)
	state := httptest.NewRecorder()
	s.Handler().ServeHTTP(state, stateReq)
	var before game.State
	if err := json.Unmarshal(state.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	if len(before.Players) != 0 || len(before.Candidates) != 1 {
		t.Fatalf("candidate became accepted: %#v", before)
	}
	promote := httptest.NewRequest(http.MethodPost, "https://wall.test/api/admin/candidates/alice/promote", strings.NewReader(`{"version":1}`))
	promote.AddCookie(admin)
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, promote)
	if out.Code != http.StatusOK {
		t.Fatalf("promote=%d %s", out.Code, out.Body.String())
	}
	var after game.State
	if err := json.Unmarshal(out.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Players) != 1 || len(after.Candidates) != 0 {
		t.Fatalf("promotion missing: %#v", after)
	}
	kick := httptest.NewRequest(http.MethodPost, "https://wall.test/api/admin/players/alice/kick", nil)
	kick.AddCookie(admin)
	kicked := httptest.NewRecorder()
	s.Handler().ServeHTTP(kicked, kick)
	if kicked.Code != http.StatusOK {
		t.Fatalf("kick=%d %s", kicked.Code, kicked.Body.String())
	}
	var kickedState game.State
	if err := json.Unmarshal(kicked.Body.Bytes(), &kickedState); err != nil {
		t.Fatal(err)
	}
	if len(kickedState.Players) != 0 {
		t.Fatalf("kicked player remained visible: %#v", kickedState)
	}
	newCode := playerRequest(http.MethodPost, "/api/candidate", `{"version":2,"source":"fn userfn(x:f32,y:f32,t:f32)->f32{return y;}"}`)
	newCodeResult := httptest.NewRecorder()
	s.Handler().ServeHTTP(newCodeResult, newCode)
	if newCodeResult.Code != http.StatusOK {
		t.Fatalf("higher candidate=%d %s", newCodeResult.Code, newCodeResult.Body.String())
	}
}
func TestAdminMutationsRejectCrossOrigin(t *testing.T) {
	s := testServer(t)
	cookie := adminCookie(t, s)
	req := httptest.NewRequest(http.MethodPut, "https://wall.test/api/admin/viewport", strings.NewReader(`{"width":1000,"height":800}`))
	req.AddCookie(cookie)
	req.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d", w.Code)
	}
}
