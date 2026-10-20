package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nurtur/internal/view"
)

const (
	sessionCookieName = "nurtur_session"
	sessionLifetime   = 30 * 24 * time.Hour
	loginWindow       = 15 * time.Minute
	loginMaxAttempts  = 5
)

type authContextKey string

const (
	authCSRFKey authContextKey = "csrf"
)

// auth implements the optional single-user passcode gate. When no passcode is
// configured the middleware is a no-op, which keeps LAN-only installs simple.
type auth struct {
	passcode string
	key      []byte
	secure   bool
	limiter  *loginLimiter
}

func newAuth(passcode, secret string, secure bool) *auth {
	if secret == "" {
		sum := sha256.Sum256([]byte("nurtur-session:" + passcode))
		secret = string(sum[:])
	}
	return &auth{
		passcode: passcode,
		key:      []byte(secret),
		secure:   secure,
		limiter:  newLoginLimiter(),
	}
}

func (a *auth) enabled() bool { return a != nil && a.passcode != "" }

func (a *auth) middleware(next http.Handler) http.Handler {
	if !a.enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAuth(r) {
			next.ServeHTTP(w, r)
			return
		}

		csrf, ok := a.session(r)
		if !ok {
			next := r.URL.RequestURI()
			http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
			return
		}

		if isUnsafeMethod(r.Method) {
			if err := a.checkCSRF(r, csrf); err != nil {
				http.Error(w, "That request could not be verified. Reload and try again.", http.StatusForbidden)
				return
			}
		}

		ctx := context.WithValue(r.Context(), authCSRFKey, csrf)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requiresAuth(r *http.Request) bool {
	path := r.URL.Path
	switch {
	case path == "/login", path == "/logout",
		path == "/healthz",
		path == "/offline",
		path == "/sw.js",
		path == "/favicon.ico":
		return false
	case strings.HasPrefix(path, "/static/"):
		return false
	default:
		return true
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// checkCSRF requires the per-session token for HTMX mutations and a same-origin
// Origin/Sec-Fetch-Site signal for classic form posts, which cannot attach a
// header. Both paths also reject cross-site origins outright.
func (a *auth) checkCSRF(r *http.Request, csrf string) error {
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(parsed.Host, r.Host) {
			return errors.New("cross-origin request")
		}
	} else if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		if site != "same-origin" && site != "none" {
			return errors.New("cross-site request")
		}
	}

	if isHTMX(r) {
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			return errors.New("missing csrf token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(csrf)) != 1 {
			return errors.New("bad csrf token")
		}
	}
	return nil
}

func (a *auth) newSession() (value, csrf string, err error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	csrf = base64.RawURLEncoding.EncodeToString(raw[:])
	payload := strings.Join([]string{
		"v1",
		time.Now().UTC().Add(sessionLifetime).Format(time.RFC3339),
		csrf,
	}, "|")
	return payload + "." + a.sign(payload), csrf, nil
}

func (a *auth) session(r *http.Request) (csrf string, ok bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	payload, sig, found := strings.Cut(cookie.Value, ".")
	if !found || !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return "", false
	}
	parts := strings.Split(payload, "|")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", false
	}
	expires, err := time.Parse(time.RFC3339, parts[1])
	if err != nil || time.Now().UTC().After(expires) {
		return "", false
	}
	return parts[2], true
}

func (a *auth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *auth) setSession(w http.ResponseWriter, r *http.Request, csrf string) {
	if csrf == "" {
		return
	}
	// Rebuild the value so expiry and signature match the token issued above.
	payload := strings.Join([]string{
		"v1",
		time.Now().UTC().Add(sessionLifetime).Format(time.RFC3339),
		csrf,
	}, "|")
	// #nosec G124 -- Secure is requested whenever TLS or auth.secure is in play;
	// plain-HTTP LAN installs intentionally keep it off or the cookie is dropped.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    payload + "." + a.sign(payload),
		Path:     "/",
		MaxAge:   int(sessionLifetime / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secure || r.TLS != nil,
	})
}

func (a *auth) clearSession(w http.ResponseWriter, r *http.Request) {
	// #nosec G124 -- must mirror the attributes used by setSession above.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secure || r.TLS != nil,
	})
}

func (a *auth) matches(candidate string) bool {
	// Hash both sides so the comparison is constant-time regardless of length.
	want := sha256.Sum256([]byte(a.passcode))
	got := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

func csrfFrom(ctx context.Context) string {
	value, _ := ctx.Value(authCSRFKey).(string)
	return value
}

/* ---------- Login rate limiting ---------- */

type loginAttempt struct {
	count int
	reset time.Time
}

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginAttempt
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]*loginAttempt)}
}

func (l *loginLimiter) allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[ip]
	if !ok || now.After(entry.reset) {
		l.entries[ip] = &loginAttempt{reset: now.Add(loginWindow)}
		return true
	}
	return entry.count < loginMaxAttempts
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[ip]
	if !ok || now.After(entry.reset) {
		l.entries[ip] = &loginAttempt{count: 1, reset: now.Add(loginWindow)}
		return
	}
	entry.count++
	// Keep the map from growing without bound on a long-running server.
	if len(l.entries) > 1024 {
		for key, item := range l.entries {
			if now.After(item.reset) {
				delete(l.entries, key)
			}
		}
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

/* ---------- Handlers ---------- */

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.auth.enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := &view.PageData{
		Nonce:       nonceFrom(r.Context()),
		AuthEnabled: true,
		Next:        safeNext(r.URL.Query().Get("next")),
		Version:     s.version,
	}
	s.renderNamed(w, r, http.StatusOK, "login", data)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.auth.enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	ip := clientIP(r)
	if !s.auth.limiter.allowed(ip) {
		data := &view.PageData{
			Nonce:       nonceFrom(r.Context()),
			AuthEnabled: true,
			LoginError:  "Too many tries. Wait a few minutes and try again.",
			Next:        safeNext(r.FormValue("next")),
		}
		s.renderNamed(w, r, http.StatusTooManyRequests, "login", data)
		return
	}
	if !s.auth.matches(r.FormValue("passcode")) {
		s.auth.limiter.fail(ip)
		s.log.Warn("login failed", "ip", ip)
		data := &view.PageData{
			Nonce:       nonceFrom(r.Context()),
			AuthEnabled: true,
			LoginError:  "That passcode isn't right. Try again?",
			Next:        safeNext(r.FormValue("next")),
		}
		s.renderNamed(w, r, http.StatusUnauthorized, "login", data)
		return
	}

	s.auth.limiter.reset(ip)
	_, csrf, err := s.auth.newSession()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.auth.setSession(w, r, csrf)
	// #nosec G710 -- safeNext returns only same-origin paths; absolute,
	// protocol-relative, and backslash-normalised targets are rejected.
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth.enabled() {
		s.auth.clearSession(w, r)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// renderNamed renders a top-level template without the dashboard shell. The
// login and offline pages own their whole document.
func (s *Server) renderNamed(w http.ResponseWriter, r *http.Request, status int, name string, data *view.PageData) {
	var buf bytes.Buffer
	if err := s.renderer.Execute(&buf, name, data); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func safeNext(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	// Browsers normalise backslashes to slashes, so "/\host" would otherwise
	// turn into a protocol-relative redirect.
	if strings.ContainsRune(raw, '\\') {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return u.RequestURI()
}
