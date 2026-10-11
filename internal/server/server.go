package server

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"shaderflow/internal/game"
)

type Config struct {
	Address           string   `json:"address"`
	DataDir           string   `json:"dataDir"`
	StaticDir         string   `json:"staticDir"`
	AdminPassword     string   `json:"adminPassword"`
	TrustedProxyCIDRs []string `json:"trustedProxyCIDRs"`
	CookieSecure      bool     `json:"cookieSecure"`
	TLSCert           string   `json:"tlsCert"`
	TLSKey            string   `json:"tlsKey"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web"
	}
	if len(cfg.TrustedProxyCIDRs) == 0 {
		cfg.TrustedProxyCIDRs = []string{"127.0.0.0/8", "::1/128"}
	}
	return cfg, cfg.Validate()
}
func (cfg Config) Validate() error {
	if cfg.AdminPassword == "" {
		return errors.New("adminPassword is required")
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return errors.New("TLS certificate and key must both be configured")
	}
	if len(cfg.TrustedProxyCIDRs) == 0 {
		return errors.New("trustedProxyCIDRs is required")
	}
	for _, cidr := range cfg.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", cidr)
		}
	}
	return nil
}

type Server struct {
	cfg          Config
	Game         *game.Game
	mux          *http.ServeMux
	sessions     *sessionStore
	loginLimiter *loginLimiter
	trusted      []*net.IPNet
}

func New(cfg Config) *Server {
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web"
	}
	if len(cfg.TrustedProxyCIDRs) == 0 {
		cfg.TrustedProxyCIDRs = []string{"127.0.0.0/8", "::1/128"}
	}
	trusted := make([]*net.IPNet, 0, len(cfg.TrustedProxyCIDRs))
	for _, cidr := range cfg.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			trusted = append(trusted, network)
		}
	}
	sessions, err := newFileSessionStore(filepath.Join(cfg.DataDir, "sessions.json"))
	if err != nil {
		sessions = newSessionStore()
	}
	s := &Server{cfg: cfg, Game: game.New(), mux: http.NewServeMux(), sessions: sessions, loginLimiter: newLoginLimiter(), trusted: trusted}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && !sameOrigin(r) {
			fail(w, http.StatusForbidden, errors.New("cross-origin mutation denied"))
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}
func (s *Server) ListenAndServe() error {
	srv := &http.Server{Addr: s.cfg.Address, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
	if s.cfg.TLSCert != "" {
		return srv.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
	}
	return srv.ListenAndServe()
}

func (s *Server) trustedRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range s.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
func (s *Server) identity(r *http.Request) (sub, name, role string) {
	if cookie, err := r.Cookie("shaderflow_admin"); err == nil {
		if value, ok := s.sessions.get(cookie.Value); ok && value.Role == "admin" {
			return value.Sub, value.Name, "admin"
		}
	}
	if s.trustedRequest(r) {
		user := strings.TrimSpace(r.Header.Get("X-Forwarded-User"))
		if user != "" {
			return user, user, "player"
		}
	}
	return "", "", ""
}
func (s *Server) cookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{Name: "shaderflow_admin", Value: value, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
}
func (s *Server) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "shaderflow_admin", Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
}
func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) { http.Error(w, err.Error(), status) }
func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
func (s *Server) require(role string, fn func(http.ResponseWriter, *http.Request, string, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, name, got := s.identity(r)
		if got != role {
			fail(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
		fn(w, r, sub, name)
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { reply(w, map[string]bool{"ok": true}) })
	s.mux.HandleFunc("POST /api/admin/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
		}
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		if !s.loginLimiter.allow(r.RemoteAddr) {
			fail(w, 429, errors.New("too many login attempts"))
			return
		}
		if !hmac.Equal([]byte(req.Password), []byte(s.cfg.AdminPassword)) {
			s.loginLimiter.fail(r.RemoteAddr)
			fail(w, 401, errors.New("invalid password"))
			return
		}
		s.loginLimiter.success(r.RemoteAddr)
		id, err := s.sessions.create("admin", "Administrator", "admin")
		if err != nil {
			fail(w, 500, err)
			return
		}
		s.cookie(w, r, id)
		reply(w, map[string]string{"role": "admin"})
	})
	s.mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("shaderflow_admin"); err == nil {
			s.sessions.delete(c.Value)
		}
		s.clearCookie(w, r)
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		sub, name, role := s.identity(r)
		if role == "" {
			fail(w, 401, errors.New("authentication required"))
			return
		}
		reply(w, map[string]string{"sub": sub, "name": name, "role": role})
	})
	s.mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		sub, _, role := s.identity(r)
		if role == "" {
			fail(w, 401, errors.New("authentication required"))
			return
		}
		reply(w, s.Game.Snapshot(sub, role == "admin"))
	})
	s.mux.HandleFunc("POST /api/candidate", s.require("player", func(w http.ResponseWriter, r *http.Request, sub, name string) {
		var req struct {
			Version int64  `json:"version"`
			Source  string `json:"source"`
		}
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.SubmitCandidate(sub, name, req.Version, req.Source); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot(sub, false))
	}))
	s.mux.HandleFunc("PUT /api/admin/settings", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var settings game.Settings
		if err := decode(r, &settings); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.UpdateSettings(settings); err != nil {
			fail(w, 400, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("POST /api/admin/candidates/{sub}/promote", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var req struct {
			Version int64 `json:"version"`
		}
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.Promote(r.PathValue("sub"), req.Version); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("GET /api/ws", s.ws)
	if _, err := os.Stat(s.cfg.StaticDir); err == nil {
		s.mux.Handle("/", http.FileServer(http.Dir(s.cfg.StaticDir)))
	}
}
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	sub, _, role := s.identity(r)
	if role == "" {
		fail(w, 401, errors.New("authentication required"))
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && !sameOrigin(r) {
		fail(w, 403, errors.New("cross-origin websocket denied"))
		return
	}
	var since uint64
	fmt.Sscan(r.URL.Query().Get("since"), &since)
	conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch, initial, cancel := s.Game.Subscribe(sub, role == "admin", since)
	defer cancel()
	for _, event := range initial {
		if err := conn.WriteJSON(event); err != nil {
			return
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	conn.SetPongHandler(func(string) error { _ = conn.SetReadDeadline(time.Now().Add(65 * time.Second)); return nil })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(s.Game.Visible(event, sub, role == "admin")); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}
