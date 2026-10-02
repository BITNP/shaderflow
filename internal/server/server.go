package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/websocket"
	"golang.org/x/oauth2"
	"shaderflow/internal/archive"
	"shaderflow/internal/game"
)

type OIDC struct {
	Issuer          string    `json:"issuer"`
	ClientID        string    `json:"clientId"`
	ClientSecret    string    `json:"clientSecret"`
	ClientSecretRef SecretRef `json:"clientSecretRef"`
	RedirectURL     string    `json:"redirectUrl"`
}
type Config struct {
	Environment           string    `json:"environment"`
	Address               string    `json:"address"`
	PublicURL             string    `json:"publicUrl"`
	AdminPassword         string    `json:"adminPassword"`
	AdminPasswordHash     string    `json:"adminPasswordHash"`
	AdminPasswordHashRef  SecretRef `json:"adminPasswordHashRef"`
	TLSCert               string    `json:"tlsCert"`
	TLSKey                string    `json:"tlsKey"`
	DataDir               string    `json:"dataDir"`
	StaticDir             string    `json:"staticDir"`
	EnableTestPlayerLogin bool      `json:"enableTestPlayerLogin"`
	OIDC                  OIDC      `json:"oidc"`
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
		cfg.Address = ":8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web"
	}
	if err := cfg.ResolveSecrets(); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}
func (cfg Config) Validate() error {
	if cfg.AdminPassword == "" && cfg.AdminPasswordHash == "" {
		return errors.New("adminPassword or adminPasswordHash required")
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return errors.New("TLS certificate and key must both be configured")
	}
	if cfg.Environment == "production" {
		if cfg.EnableTestPlayerLogin {
			return errors.New("test player login forbidden in production")
		}
		if cfg.TLSCert == "" {
			return errors.New("TLS required in production")
		}
		if cfg.AdminPassword != "" {
			return errors.New("plaintext adminPassword forbidden in production")
		}
		if cfg.AdminPasswordHashRef.Env == "" && cfg.AdminPasswordHashRef.File == "" {
			return errors.New("adminPasswordHashRef required in production")
		}
		if cfg.OIDC.ClientSecret != "" && cfg.OIDC.ClientSecretRef.Env == "" && cfg.OIDC.ClientSecretRef.File == "" {
			return errors.New("inline OIDC clientSecret forbidden in production")
		}
		if !strings.HasPrefix(cfg.AdminPasswordHash, "$2") {
			return errors.New("bcrypt adminPasswordHash required in production")
		}
		if cfg.PublicURL == "" {
			return errors.New("publicUrl required in production")
		}
		if cfg.OIDC.Issuer == "" || cfg.OIDC.ClientID == "" || cfg.OIDC.ClientSecret == "" || cfg.OIDC.RedirectURL == "" {
			return errors.New("complete OIDC configuration required in production")
		}
		issuer, err := url.Parse(cfg.OIDC.Issuer)
		if err != nil || issuer.Scheme != "https" || issuer.Host == "" {
			return errors.New("production OIDC issuer must use HTTPS")
		}
		redirect, err := url.Parse(cfg.OIDC.RedirectURL)
		if err != nil || redirect.Scheme != "https" || redirect.Host == "" {
			return errors.New("production OIDC redirectUrl must use HTTPS")
		}
		public, err := url.Parse(cfg.PublicURL)
		if err != nil || public.Scheme != "https" || public.Host == "" {
			return errors.New("production publicUrl must use HTTPS")
		}
		if public.Path != "" && public.Path != "/" {
			return errors.New("production publicUrl must be an origin without path")
		}
		if redirect.Scheme != public.Scheme || redirect.Host != public.Host {
			return errors.New("OIDC redirectUrl must match publicUrl origin")
		}
	}
	return nil
}

type Server struct {
	cfg          Config
	storageErr   error
	Game         *game.Game
	archives     *archive.Filesystem
	mux          *http.ServeMux
	oidcMu       sync.Mutex
	oauth        *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	sessions     *sessionStore
	loginLimiter *loginLimiter
}

func New(cfg Config) *Server {
	store := archive.NewFilesystem(cfg.DataDir)
	sessions, err := newFileSessionStore(filepath.Join(cfg.DataDir, "sessions.json"))
	if err != nil {
		log.Printf("session store unavailable, using memory adapter: %v", err)
		sessions = newSessionStore()
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux(), archives: store, sessions: sessions, loginLimiter: newLoginLimiter()}
	s.storageErr = store.Health()
	s.Game = game.New(store)
	s.routes()
	return s
}
func mustInt64(value string) int64   { parsed, _ := strconv.ParseInt(value, 10, 64); return parsed }
func mustUint64(value string) uint64 { parsed, _ := strconv.ParseUint(value, 10, 64); return parsed }
func parseVersionMap(value string) map[string]int64 {
	if value == "" {
		return nil
	}
	var result map[string]int64
	if json.Unmarshal([]byte(value), &result) != nil {
		return nil
	}
	return result
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
	if s.storageErr != nil {
		return fmt.Errorf("archive storage unavailable: %w", s.storageErr)
	}
	srv := &http.Server{Addr: s.cfg.Address, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
	if s.cfg.TLSCert != "" {
		return srv.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
	}
	log.Print("WARNING: HTTP is only suitable for localhost; admin passwords and WebGPU require HTTPS on LAN")
	return srv.ListenAndServe()
}
func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func (s *Server) identity(r *http.Request) (sub, name, role string) {
	if c, err := r.Cookie("shaderflow_session"); err == nil {
		if value, ok := s.sessions.get(c.Value); ok {
			return value.Sub, value.Name, value.Role
		}
	}
	return "", "", ""
}
func (s *Server) cookie(w http.ResponseWriter, r *http.Request, value string) {
	http.SetCookie(w, &http.Cookie{Name: "shaderflow_session", Value: value, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
}
func clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "shaderflow_session", Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return origin == "http://"+r.Host || origin == "https://"+r.Host
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
			fail(w, 401, errors.New("authentication required"))
			return
		}
		fn(w, r, sub, name)
	}
}
func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		status := s.archives.Status()
		if s.storageErr != nil || !status.Ready {
			message := status.Error
			if s.storageErr != nil {
				message = s.storageErr.Error()
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			reply(w, map[string]any{"ok": false, "storage": "degraded", "error": message})
			return
		}
		reply(w, map[string]any{"ok": true, "storage": "ready"})
	})
	s.mux.HandleFunc("GET /api/auth/capabilities", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]bool{"testPlayerLogin": s.cfg.EnableTestPlayerLogin, "oidc": s.cfg.OIDC.Issuer != "" && s.cfg.OIDC.ClientID != ""})
	})
	s.mux.HandleFunc("POST /api/admin/login", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
		}
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		key := r.RemoteAddr
		if !s.loginLimiter.allow(key) {
			fail(w, http.StatusTooManyRequests, errors.New("too many login attempts"))
			return
		}
		valid := false
		if s.cfg.AdminPasswordHash != "" {
			valid = VerifyPassword(req.Password, s.cfg.AdminPasswordHash)
		} else {
			valid = hmac.Equal([]byte(req.Password), []byte(s.cfg.AdminPassword))
		}
		if !valid {
			s.loginLimiter.fail(key)
			fail(w, 401, errors.New("invalid password"))
			return
		}
		s.loginLimiter.success(key)
		sessionID, err := s.sessions.create("admin", "Administrator", "admin")
		if err != nil {
			fail(w, 500, err)
			return
		}
		s.cookie(w, r, sessionID)
		reply(w, map[string]string{"role": "admin"})
	})
	if s.cfg.EnableTestPlayerLogin {
		s.mux.HandleFunc("POST /api/test/player-login", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Name string `json:"name"`
			}
			if err := decode(r, &req); err != nil {
				fail(w, 400, err)
				return
			}
			name := strings.TrimSpace(req.Name)
			if name == "" || len([]rune(name)) > 40 {
				fail(w, 400, errors.New("test player name must be 1-40 characters"))
				return
			}
			random := make([]byte, 12)
			if _, err := rand.Read(random); err != nil {
				fail(w, 500, err)
				return
			}
			sessionID, err := s.sessions.createExclusive("test-"+b64(random), name, "player")
			if err != nil {
				fail(w, 500, err)
				return
			}
			s.cookie(w, r, sessionID)
			reply(w, map[string]string{"role": "player", "name": name})
		})
	}
	s.mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		sub, _, role := s.identity(r)
		if role == "" {
			fail(w, 401, errors.New("authentication required"))
			return
		}
		reply(w, s.Game.Snapshot(sub, role == "admin"))
	})
	s.mux.HandleFunc("POST /api/register", s.require("player", func(w http.ResponseWriter, r *http.Request, sub, name string) {
		if err := s.Game.Register(sub, name); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot(sub, false))
	}))
	s.mux.HandleFunc("POST /api/code", s.require("player", func(w http.ResponseWriter, r *http.Request, sub, name string) {
		var c struct {
			Version int64  `json:"version"`
			Code    string `json:"code"`
		}
		if err := decode(r, &c); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.Upload(sub, c.Version, c.Code); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, map[string]any{"ok": true, "version": c.Version})
	}))
	s.mux.HandleFunc("POST /api/admin/round", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var cfg game.Settings
		if err := decode(r, &cfg); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.Configure(cfg); err != nil {
			fail(w, 400, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("POST /api/admin/graph/generate", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		if err := s.Game.Generate(); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("PUT /api/admin/graph", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var graph game.Graph
		if err := decode(r, &graph); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.SetGraph(graph); err != nil {
			fail(w, 400, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("POST /api/admin/start", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		if err := s.Game.Start(); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("PUT /api/admin/inputs/{inputId}", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			fail(w, 400, err)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, err)
			return
		}
		defer file.Close()
		if header.Size > 16<<20 {
			fail(w, 413, errors.New("input image too large"))
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, 16<<20+1))
		if err != nil {
			fail(w, 400, err)
			return
		}
		contentType := http.DetectContentType(data)
		if contentType != "image/png" && contentType != "image/jpeg" {
			fail(w, 415, errors.New("only PNG/JPEG supported"))
			return
		}
		ref, err := s.archives.StoreBlob(contentType, data)
		if err != nil {
			fail(w, 500, err)
			return
		}
		version, err := strconv.ParseInt(r.FormValue("version"), 10, 64)
		if err != nil {
			fail(w, 400, err)
			return
		}
		input := game.ExternalInput{ID: r.PathValue("inputId"), RoundID: ref.RoundID, Version: version, SHA256: ref.SHA256, Size: ref.Size, ContentType: ref.ContentType, ResizeMode: r.FormValue("resizeMode")}
		if err := s.Game.PublishExternalInput(input); err != nil {
			_ = s.archives.DeleteBlob(ref)
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("GET /api/admin/archives/{roundId}/blobs/{sha256}", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		ref := archive.BlobRef{RoundID: r.PathValue("roundId"), SHA256: r.PathValue("sha256"), Size: mustInt64(r.URL.Query().Get("size")), ContentType: r.URL.Query().Get("contentType")}
		data, contentType, err := s.archives.ReadBlob(ref)
		if err != nil {
			fail(w, 404, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}))
	s.mux.HandleFunc("POST /api/admin/snapshots", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		if err := r.ParseMultipartForm(40 << 20); err != nil {
			fail(w, 400, err)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, err)
			return
		}
		defer file.Close()
		if header.Size > 32<<20 {
			fail(w, 413, errors.New("snapshot too large"))
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, 32<<20+1))
		if err != nil {
			fail(w, 400, err)
			return
		}
		contentType := http.DetectContentType(data)
		if contentType != "image/png" && contentType != "image/jpeg" {
			fail(w, 415, errors.New("only PNG/JPEG supported"))
			return
		}
		ref, err := s.archives.StoreBlob(contentType, data)
		if err != nil {
			fail(w, 500, err)
			return
		}
		width, widthErr := strconv.Atoi(r.FormValue("width"))
		height, heightErr := strconv.Atoi(r.FormValue("height"))
		timeMS, timeErr := strconv.ParseFloat(r.FormValue("timeMs"), 64)
		if widthErr != nil || heightErr != nil || timeErr != nil {
			fail(w, 400, errors.New("invalid snapshot dimensions or time"))
			return
		}
		snapshot := game.RenderSnapshot{ID: r.FormValue("id"), RoundID: ref.RoundID, TextureID: r.FormValue("textureId"), Format: contentType, Width: width, Height: height, SHA256: ref.SHA256, Size: ref.Size, TimeMS: timeMS, SourceSeq: mustUint64(r.FormValue("sourceSeq")), CodeVersions: parseVersionMap(r.FormValue("codeVersions")), InputVersions: parseVersionMap(r.FormValue("inputVersions")), TrajectoryVersion: mustInt64(r.FormValue("trajectoryVersion")), TimeControlVersion: mustInt64(r.FormValue("timeControlVersion"))}
		if err := s.Game.RecordRenderSnapshot(snapshot); err != nil {
			_ = s.archives.DeleteBlob(ref)
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("PUT /api/admin/time-control", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var control game.TimeControl
		if err := decode(r, &control); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.SetTimeControl(control); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("PUT /api/admin/trajectory", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var trajectory game.Trajectory
		if err := decode(r, &trajectory); err != nil {
			fail(w, 400, err)
			return
		}
		if err := s.Game.PublishTrajectory(trajectory); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("POST /api/admin/terminate", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		if err := s.Game.Terminate(); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("DELETE /api/admin/registrations/{sub}", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		if err := s.Game.Remove(r.PathValue("sub")); err != nil {
			fail(w, 409, err)
			return
		}
		reply(w, s.Game.Snapshot("", true))
	}))
	s.mux.HandleFunc("POST /api/admin/archives/gc", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		var req struct {
			DryRun          bool    `json:"dryRun"`
			MinimumAgeHours float64 `json:"minimumAgeHours"`
		}
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		if req.MinimumAgeHours < 0 {
			fail(w, 400, errors.New("minimumAgeHours must be non-negative"))
			return
		}
		report, err := s.archives.GarbageCollectBlobs(archive.GCOptions{DryRun: req.DryRun, MinimumAge: time.Duration(req.MinimumAgeHours * float64(time.Hour))})
		if err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, report)
	}))
	s.mux.HandleFunc("GET /api/admin/archives", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		items, err := s.archives.List()
		if err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, items)
	}))
	s.mux.HandleFunc("GET /api/admin/archives/{roundId}", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		round, err := s.archives.Read(r.PathValue("roundId"))
		if err != nil {
			fail(w, 404, err)
			return
		}
		reply(w, round)
	}))
	s.mux.HandleFunc("GET /api/admin/archives/{roundId}/state", s.require("admin", func(w http.ResponseWriter, r *http.Request, _, _ string) {
		round, err := s.archives.Read(r.PathValue("roundId"))
		if err != nil {
			fail(w, 404, err)
			return
		}
		through := uint64(^uint64(0))
		if raw := r.URL.Query().Get("through"); raw != "" {
			value, parseErr := strconv.ParseUint(raw, 10, 64)
			if parseErr != nil {
				fail(w, 400, parseErr)
				return
			}
			through = value
		}
		state, replayErr := archive.ReplayFrom(round.Start, round.Events, round.Checkpoints, through)
		if replayErr != nil {
			fail(w, 422, replayErr)
			return
		}
		reply(w, state)
	}))
	s.mux.HandleFunc("GET /api/ws", s.ws)
	s.mux.HandleFunc("GET /auth/login", s.login)
	s.mux.HandleFunc("GET /auth/callback", s.callback)
	s.mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("shaderflow_session"); err == nil {
			s.sessions.delete(c.Value)
		}
		clearCookie(w, r)
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
	if origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
		fail(w, 403, errors.New("cross-origin websocket denied"))
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch, initial, cancel := s.Game.Subscribe(sub, role == "admin", since)
	defer cancel()
	for _, e := range initial {
		if err := conn.WriteJSON(e); err != nil {
			return
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(65 * time.Second)) })
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
		case e, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(s.Game.Visible(e, sub, role == "admin")); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}
func (s *Server) provider(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oauth != nil {
		return s.oauth, s.verifier, nil
	}
	if s.cfg.OIDC.Issuer == "" {
		return nil, nil, errors.New("OIDC not configured")
	}
	provider, err := oidc.NewProvider(ctx, s.cfg.OIDC.Issuer)
	if err != nil {
		return nil, nil, err
	}
	s.verifier = provider.Verifier(&oidc.Config{ClientID: s.cfg.OIDC.ClientID})
	s.oauth = &oauth2.Config{ClientID: s.cfg.OIDC.ClientID, ClientSecret: s.cfg.OIDC.ClientSecret, RedirectURL: s.cfg.OIDC.RedirectURL, Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	return s.oauth, s.verifier, nil
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	oc, _, err := s.provider(ctx)
	if err != nil {
		fail(w, 503, err)
		return
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		fail(w, 500, err)
		return
	}
	state := b64(nonce)
	http.SetCookie(w, &http.Cookie{Name: "shaderflow_oidc_state", Value: state, Path: "/auth", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, oc.AuthCodeURL(state, oidc.Nonce(state)), http.StatusFound)
}
func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("shaderflow_oidc_state")
	if err != nil || state.Value == "" || state.Value != r.URL.Query().Get("state") {
		fail(w, 400, errors.New("OIDC state mismatch"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "shaderflow_oidc_state", Path: "/auth", MaxAge: -1})
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	oc, v, err := s.provider(ctx)
	if err != nil {
		fail(w, 503, err)
		return
	}
	tok, err := oc.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		fail(w, 401, err)
		return
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		fail(w, 401, errors.New("missing id_token"))
		return
	}
	id, err := v.Verify(ctx, raw)
	if err != nil || id.Nonce != state.Value {
		fail(w, 401, errors.New("invalid id_token or nonce"))
		return
	}
	var claims struct {
		Sub       string `json:"sub"`
		Name      string `json:"name"`
		Preferred string `json:"preferred_username"`
	}
	if err := id.Claims(&claims); err != nil || claims.Sub == "" {
		fail(w, 401, errors.New("invalid claims"))
		return
	}
	name := claims.Preferred
	if name == "" {
		name = claims.Name
	}
	if name == "" {
		name = claims.Sub
	}
	sessionID, err := s.sessions.createExclusive(claims.Sub, name, "player")
	if err != nil {
		fail(w, 500, err)
		return
	}
	s.cookie(w, r, sessionID)
	http.Redirect(w, r, "/player/", http.StatusFound)
}
