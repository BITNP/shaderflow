package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shaderflow/internal/game"
	"strings"
	"testing"
)

func TestProductionConfigRequiresCanonicalPublicURLAndMatchingRedirect(t *testing.T) {
	base := Config{Environment: "production", PublicURL: "https://shaderflow.example", AdminPasswordHash: "$2a$10$012345678901234567890uK7i14ZkmO.Bm65rNlf.ZXIvFKRm", AdminPasswordHashRef: SecretRef{Env: "ADMIN_HASH"}, TLSCert: "cert", TLSKey: "key", OIDC: OIDC{Issuer: "https://id.example", ClientID: "client", ClientSecret: "secret", ClientSecretRef: SecretRef{Env: "OIDC_SECRET"}, RedirectURL: "https://shaderflow.example/auth/callback"}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := base
	bad.PublicURL = "http://shaderflow.example"
	if err := bad.Validate(); err == nil {
		t.Fatal("HTTP public URL accepted")
	}
	bad = base
	bad.OIDC.RedirectURL = "https://other.example/auth/callback"
	if err := bad.Validate(); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
}

func TestProductionConfigRequiresHTTPSOIDCAndResolvedSecret(t *testing.T) {
	base := Config{Environment: "production", PublicURL: "https://app.example", AdminPasswordHash: "$2a$10$012345678901234567890uK7i14ZkmO.Bm65rNlf.ZXIvFKRm", AdminPasswordHashRef: SecretRef{Env: "ADMIN_HASH"}, TLSCert: "cert", TLSKey: "key", OIDC: OIDC{Issuer: "https://id.example", ClientID: "client", ClientSecret: "secret", ClientSecretRef: SecretRef{Env: "OIDC_SECRET"}, RedirectURL: "https://app.example/auth/callback"}}
	bad := base
	bad.OIDC.Issuer = "http://id.example"
	if err := bad.Validate(); err == nil {
		t.Fatal("HTTP issuer accepted")
	}
	bad = base
	bad.OIDC.RedirectURL = "http://app.example/auth/callback"
	if err := bad.Validate(); err == nil {
		t.Fatal("HTTP redirect accepted")
	}
	bad = base
	bad.OIDC.ClientSecret = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("missing OIDC client secret accepted")
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProductionConfigRejectsWeakDeployment(t *testing.T) {
	base := Config{Environment: "production", PublicURL: "https://app.example", AdminPasswordHash: "$2a$10$012345678901234567890uK7i14ZkmO.Bm65rNlf.ZXIvFKRm", AdminPasswordHashRef: SecretRef{Env: "ADMIN_HASH"}, TLSCert: "cert", TLSKey: "key", OIDC: OIDC{Issuer: "https://id.example", ClientID: "client", ClientSecret: "secret", ClientSecretRef: SecretRef{Env: "OIDC_SECRET"}, RedirectURL: "https://app.example/auth/callback"}}
	base.EnableTestPlayerLogin = true
	if err := base.Validate(); err == nil {
		t.Fatal("production test login accepted")
	}
	base.EnableTestPlayerLogin = false
	base.TLSCert = ""
	base.TLSKey = ""
	if err := base.Validate(); err == nil {
		t.Fatal("production plain HTTP accepted")
	}
	base.TLSCert = "cert"
	base.TLSKey = "key"
	base.AdminPassword = "plain"
	if err := base.Validate(); err == nil {
		t.Fatal("production plaintext password accepted")
	}
}

func TestCookieSessionLoginLogoutAndOriginProtection(t *testing.T) {
	s := New(Config{AdminPassword: "secret", DataDir: t.TempDir()})
	login := httptest.NewRequest(http.MethodPost, "https://example.test/api/admin/login", strings.NewReader(`{"password":"secret"}`))
	login.Host = "example.test"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, login)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["token"]; ok {
		t.Fatal("login exposed bearer token")
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("weak cookie: %#v", cookies)
	}
	mutation := httptest.NewRequest(http.MethodPost, "https://example.test/api/admin/round", strings.NewReader(`{}`))
	mutation.Host = "example.test"
	mutation.AddCookie(cookies[0])
	mutation.Header.Set("Origin", "https://evil.test")
	denied := httptest.NewRecorder()
	s.Handler().ServeHTTP(denied, mutation)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("cross origin mutation=%d", denied.Code)
	}
	logout := httptest.NewRequest(http.MethodPost, "https://example.test/api/auth/logout", nil)
	logout.Host = "example.test"
	logout.AddCookie(cookies[0])
	logout.Header.Set("Origin", "https://example.test")
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, logout)
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout=%d %s", out.Code, out.Body.String())
	}
	me := httptest.NewRequest(http.MethodGet, "https://example.test/api/auth/me", nil)
	me.AddCookie(cookies[0])
	result := httptest.NewRecorder()
	s.Handler().ServeHTTP(result, me)
	if result.Code != http.StatusUnauthorized {
		t.Fatalf("logged out session accepted: %d", result.Code)
	}
}

func TestAdminLoginRateLimit(t *testing.T) {
	s := New(Config{AdminPassword: "secret", DataDir: t.TempDir()})
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"password":"wrong"}`))
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("attempt %d=%d", i, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"password":"secret"}`))
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("limit status=%d", w.Code)
	}
}

func TestAdminAuthenticationAndRound(t *testing.T) {
	s := New(Config{AdminPassword: "secret", DataDir: t.TempDir()})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"password":"secret"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var auth struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &auth); err != nil {
		t.Fatal(err)
	}
	if auth.Role != "admin" {
		t.Fatalf("unexpected auth: %#v", auth)
	}
	cookie := w.Result().Cookies()[0]
	cfg, _ := json.Marshal(game.DefaultSettings())
	req = httptest.NewRequest(http.MethodPost, "/api/admin/round", strings.NewReader(string(cfg)))
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("round: %d %s", w.Code, w.Body.String())
	}
	var state game.State
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Phase != "registration" {
		t.Fatalf("phase %s", state.Phase)
	}
}
func TestAuthCapabilitiesExposeTestLoginFlag(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		want    bool
	}{{false, false}, {true, true}} {
		s := New(Config{AdminPassword: "secret", DataDir: t.TempDir(), EnableTestPlayerLogin: tc.enabled})
		req := httptest.NewRequest(http.MethodGet, "/api/auth/capabilities", nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("capabilities status=%d", w.Code)
		}
		var got struct {
			TestPlayerLogin bool `json:"testPlayerLogin"`
			OIDC            bool `json:"oidc"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.TestPlayerLogin != tc.want {
			t.Fatalf("enabled=%v got=%v", tc.enabled, got.TestPlayerLogin)
		}
	}
}

func TestTestPlayerLoginRequiresExplicitEnablement(t *testing.T) {
	login := func(s *Server) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/test/player-login", strings.NewReader(`{"name":"测试玩家"}`))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		return w
	}
	disabled := New(Config{AdminPassword: "secret", DataDir: t.TempDir()})
	if got := login(disabled).Code; got != http.StatusNotFound {
		t.Fatalf("disabled test login status = %d, want 404", got)
	}
	enabled := New(Config{AdminPassword: "secret", DataDir: t.TempDir(), EnableTestPlayerLogin: true})
	w := login(enabled)
	if w.Code != http.StatusOK {
		t.Fatalf("enabled test login: %d %s", w.Code, w.Body.String())
	}
	var auth struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &auth); err != nil {
		t.Fatal(err)
	}
	if auth.Role != "player" {
		t.Fatalf("unexpected auth response: %#v", auth)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(w.Result().Cookies()[0])
	me := httptest.NewRecorder()
	enabled.Handler().ServeHTTP(me, req)
	if me.Code != http.StatusOK {
		t.Fatalf("test player token rejected: %d %s", me.Code, me.Body.String())
	}
}

func TestReplayAndPlayerRedaction(t *testing.T) {
	g := game.New(nil)
	cfg := game.DefaultSettings()
	cfg.Players = 1
	if err := g.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if err := g.Register("u", "Player"); err != nil {
		t.Fatal(err)
	}
	ch, initial, cancel := g.Subscribe("u", false, 0)
	defer cancel()
	_ = ch
	if len(initial) != 1 || initial[0].Type != "snapshot" || initial[0].State.Phase != "registration" {
		t.Fatalf("initial %#v", initial)
	}
	_, events, stop := g.Subscribe("u", false, 1)
	defer stop()
	if len(events) != 1 || events[0].Type != "snapshot" || events[0].State.Registrations[0].Sub != "u" {
		t.Fatalf("replay %#v", events)
	}
}
