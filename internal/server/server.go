// Package server wires Nurtur's HTTP surface: the dashboard, feed mutations,
// HTMX fragments, health checks, and static assets.
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"runtime/debug"
	"strings"
	"time"

	"nurtur/internal/clock"
	"nurtur/internal/store"
	"nurtur/internal/view"
)

// Config is everything the HTTP layer needs.
type Config struct {
	Store      *store.Store
	Clock      clock.Clock
	Location   *time.Location
	BabyName   string
	Version    string
	Assets     fs.FS
	Migrations fs.FS
	Logger     *slog.Logger
	SyncWorker *store.SyncWorker
	LocalOnly  bool
	NightStart time.Duration
	NightEnd   time.Duration

	Passcode      string
	SignupKey     string
	SessionSecret string
	SecureCookie  bool
}

// Server holds the router and shared dependencies.
type Server struct {
	store      *store.Store
	clock      clock.Clock
	loc        *time.Location
	version    string
	log        *slog.Logger
	renderer   *view.Renderer
	static     http.Handler
	worker     *store.SyncWorker
	local      bool
	nightStart time.Duration
	nightEnd   time.Duration
	migrations fs.FS
	assets     fs.FS
	auth       *auth

	defaultBabyName string
}

// New validates dependencies and parses the embedded templates.
func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("server: store is required")
	}
	if cfg.Clock == nil {
		return nil, fmt.Errorf("server: clock is required")
	}
	if cfg.Assets == nil {
		return nil, fmt.Errorf("server: assets are required")
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.NightStart <= 0 && cfg.NightEnd <= 0 {
		cfg.NightStart = 19 * time.Hour
		cfg.NightEnd = 7 * time.Hour
	}

	templateFS, err := fs.Sub(cfg.Assets, "web/templates")
	if err != nil {
		return nil, fmt.Errorf("server: templates: %w", err)
	}

	staticFS, err := fs.Sub(cfg.Assets, "web/static")
	if err != nil {
		return nil, fmt.Errorf("server: static assets: %w", err)
	}
	assetVersion, err := staticFingerprint(staticFS)
	if err != nil {
		return nil, fmt.Errorf("server: fingerprint static assets: %w", err)
	}

	renderer, err := view.NewRenderer(templateFS, assetVersion)
	if err != nil {
		return nil, err
	}

	accountCount, err := cfg.Store.CountAccounts(context.Background())
	if err != nil {
		return nil, fmt.Errorf("server: count accounts: %w", err)
	}

	return &Server{
		store:           cfg.Store,
		clock:           cfg.Clock,
		loc:             cfg.Location,
		defaultBabyName: cfg.BabyName,
		version:         cfg.Version,
		log:             cfg.Logger,
		renderer:        renderer,
		static:          newStaticHandler(staticFS),
		worker:          cfg.SyncWorker,
		local:           cfg.LocalOnly,
		nightStart:      cfg.NightStart,
		nightEnd:        cfg.NightEnd,
		migrations:      cfg.Migrations,
		assets:          cfg.Assets,
		auth:            newAuth(cfg.Passcode, cfg.SignupKey, cfg.SessionSecret, cfg.SecureCookie, accountCount > 0),
	}, nil
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleDashboard)
	mux.HandleFunc("GET /partials/feed/timer", s.handleTimerFragment)

	mux.HandleFunc("POST /feeds", s.handleStartFeed)
	mux.HandleFunc("GET /feeds/sheet", s.handleNursingSheet)
	mux.HandleFunc("POST /feeds/{id}/switch", s.handleSwitchFeed)
	mux.HandleFunc("POST /feeds/{id}/stop", s.handleStopFeed)
	mux.HandleFunc("GET /feeds/{id}/edit", s.handleEditPage)
	mux.HandleFunc("PATCH /feeds/{id}", s.handleEditFeed)
	mux.HandleFunc("POST /feeds/{id}", s.handleEditFeed) // no-JS fallback
	mux.HandleFunc("DELETE /feeds/{id}", s.handleDeleteFeed)
	mux.HandleFunc("POST /feeds/{id}/delete", s.handleDeleteFeed) // no-JS fallback
	mux.HandleFunc("POST /feeds/{id}/restore", s.handleRestoreFeed)

	mux.HandleFunc("GET /diapers/sheet", s.handleDiaperSheet)
	mux.HandleFunc("POST /diapers", s.handleCreateDiaper)
	mux.HandleFunc("GET /diapers/{id}/sheet", s.handleDiaperEditPage)
	mux.HandleFunc("PATCH /diapers/{id}", s.handleUpdateDiaper)
	mux.HandleFunc("POST /diapers/{id}", s.handleUpdateDiaper) // no-JS fallback
	mux.HandleFunc("DELETE /diapers/{id}", s.handleDeleteDiaper)
	mux.HandleFunc("POST /diapers/{id}/delete", s.handleDeleteDiaper)   // no-JS fallback
	mux.HandleFunc("POST /diapers/{id}/restore", s.handleRestoreDiaper) // undo

	mux.HandleFunc("GET /sleeps/sheet", s.handleSleepSheet)
	mux.HandleFunc("POST /sleeps", s.handleCreateSleep)
	mux.HandleFunc("GET /sleeps/{id}/sheet", s.handleSleepEditPage)
	mux.HandleFunc("POST /sleeps/{id}/stop", s.handleStopSleep)
	mux.HandleFunc("PATCH /sleeps/{id}", s.handleUpdateSleep)
	mux.HandleFunc("POST /sleeps/{id}", s.handleUpdateSleep) // no-JS fallback
	mux.HandleFunc("DELETE /sleeps/{id}", s.handleDeleteSleep)
	mux.HandleFunc("POST /sleeps/{id}/delete", s.handleDeleteSleep)   // no-JS fallback
	mux.HandleFunc("POST /sleeps/{id}/restore", s.handleRestoreSleep) // undo

	mux.HandleFunc("GET /timeline", s.handleTimelinePage)
	mux.HandleFunc("GET /timeline/partial", s.handleTimelinePartial)
	mux.HandleFunc("DELETE /timeline/{kind}/{id}", s.handleTimelineDelete)
	mux.HandleFunc("POST /timeline/{kind}/{id}/delete", s.handleTimelineDelete)   // no-JS fallback
	mux.HandleFunc("POST /timeline/{kind}/{id}/restore", s.handleTimelineRestore) // undo

	mux.HandleFunc("GET /settings", s.handleSettingsPage)
	mux.HandleFunc("POST /settings", s.handleSaveSettings)
	mux.HandleFunc("POST /settings/baby", s.handleSaveBabyName)
	mux.HandleFunc("GET /export.csv", s.handleExportCSV)
	mux.HandleFunc("GET /export.json", s.handleExportJSON)
	mux.HandleFunc("GET /export.sql", s.handleExportSQL)

	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /signup", s.handleSignupPage)
	mux.HandleFunc("POST /signup", s.handleSignup)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /offline", s.handleOffline)
	mux.HandleFunc("GET /sw.js", s.handleServiceWorker)
	mux.HandleFunc("GET /partials/sync", s.handleSyncChip)

	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))

	return s.recoverer(s.logRequests(s.security(s.limitBody(s.auth.middleware(mux)))))
}

/* ---------- Middleware ---------- */

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(body)
	r.bytes += n
	return n, err
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		level := slog.LevelInfo
		if strings.HasPrefix(r.URL.Path, "/static/") {
			level = slog.LevelDebug
		}
		s.log.Log(r.Context(), level, "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic recovered",
					"panic", recovered,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				http.Error(w, "Something went wrong. Try again.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}

type contextKey string

const nonceContextKey contextKey = "csp-nonce"

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := newNonce()
		if err != nil {
			nonce = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		ctx := context.WithValue(r.Context(), nonceContextKey, nonce)
		r = r.WithContext(ctx)

		w.Header().Set("Content-Security-Policy", fmt.Sprintf(
			"default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'",
			nonce))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		next.ServeHTTP(w, r)
	})
}

func newNonce() (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func nonceFrom(ctx context.Context) string {
	nonce, _ := ctx.Value(nonceContextKey).(string)
	return nonce
}

/* ---------- Static ---------- */

// staticFingerprint hashes the CSS and JS bundle so every deploy references
// fresh URLs. Without this, an updated page can be paired with a stylesheet
// from the service-worker cache and render half-styled.
func staticFingerprint(fsys fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".css", ".js":
		default:
			return nil
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(body)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:10], nil
}

func newStaticHandler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		switch path.Ext(r.URL.Path) {
		case ".woff2":
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case ".webmanifest":
			w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=3600")
		case ".css", ".js", ".svg":
			w.Header().Set("Cache-Control", "public, max-age=300")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	})
}
