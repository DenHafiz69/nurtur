package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nurtur/internal/store"
	"nurtur/internal/view"
)

const (
	adminUsername          = "admin"
	adminSessionCookieName = "nurtur_admin"
	adminSessionLifetime   = 12 * time.Hour
	inviteDefaultDays      = 7
	inviteMaxLabelRunes    = 80
	inviteListLimit        = 100
)

// adminAuth is a separate gate from the household account system. It accepts
// the fixed "admin" username and the password configured in the environment.
type adminAuth struct {
	password string
	key      []byte
	secure   bool
}

func newAdminAuth(password, sessionSecret string, secure bool) *adminAuth {
	if password == "" {
		return nil
	}
	// Fold the session secret into the signing key so changing either the
	// password or the secret invalidates existing admin cookies.
	seed := "nurtur-admin:" + password
	if sessionSecret != "" {
		seed = "nurtur-admin:" + sessionSecret + ":" + password
	}
	sum := sha256.Sum256([]byte(seed))
	return &adminAuth{password: password, key: sum[:], secure: secure}
}

func (a *adminAuth) enabled() bool {
	return a != nil
}

func (a *adminAuth) matchesPassword(candidate string) bool {
	want := sha256.Sum256([]byte(a.password))
	got := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}

func (a *adminAuth) matchesUsername(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(adminUsername)) == 1
}

func (a *adminAuth) newSession() (value, csrf string, err error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	csrf = base64.RawURLEncoding.EncodeToString(raw[:])
	payload := strings.Join([]string{
		"v1",
		time.Now().UTC().Add(adminSessionLifetime).Format(time.RFC3339),
		csrf,
	}, "|")
	return payload + "." + a.sign(payload), csrf, nil
}

func (a *adminAuth) session(r *http.Request) (csrf string, ok bool) {
	cookie, err := r.Cookie(adminSessionCookieName)
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

func (a *adminAuth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *adminAuth) setSession(w http.ResponseWriter, r *http.Request) error {
	value, _, err := a.newSession()
	if err != nil {
		return err
	}
	// #nosec G124 -- mirrors the household session: Secure is requested
	// whenever TLS or NURTUR_COOKIE_SECURE is in play.
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(adminSessionLifetime / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secure || r.TLS != nil,
	})
	return nil
}

func (a *adminAuth) clearSession(w http.ResponseWriter, r *http.Request) {
	// #nosec G124 -- must mirror the attributes used by setSession above.
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.secure || r.TLS != nil,
	})
}

type adminContextKey string

const adminCSRFContextKey adminContextKey = "admin-csrf"

func adminCSRFFrom(ctx context.Context) string {
	value, _ := ctx.Value(adminCSRFContextKey).(string)
	return value
}

// adminOnly guards every admin route except the login screens. It uses a
// session-bound CSRF token for mutations, in addition to the same-origin check
// used by the household login.
func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.admin.enabled() {
			http.NotFound(w, r)
			return
		}
		csrf, ok := s.admin.session(r)
		if !ok {
			target := "/admin/login?next=" + url.QueryEscape(r.URL.RequestURI())
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		if isUnsafeMethod(r.Method) {
			if err := s.checkAdminCSRF(r, csrf); err != nil {
				http.Error(w, "That request could not be verified. Reload and try again.", http.StatusForbidden)
				return
			}
		}
		ctx := context.WithValue(r.Context(), adminCSRFContextKey, csrf)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) checkAdminCSRF(r *http.Request, csrf string) error {
	if err := sameOrigin(r); err != nil {
		return err
	}
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("read form: %w", err)
	}
	token := r.PostForm.Get("csrf")
	if token == "" {
		return errors.New("missing csrf token")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(csrf)) != 1 {
		return errors.New("bad csrf token")
	}
	return nil
}

// safeAdminNext keeps the post-login redirect inside the admin area.
func safeAdminNext(raw string) string {
	next := safeNext(raw)
	if next == "/admin" || strings.HasPrefix(next, "/admin/") {
		if next == "/admin/login" {
			return "/admin"
		}
		return next
	}
	return "/admin"
}

/* ---------- Handlers ---------- */

func (s *Server) handleAdminLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.admin.enabled() {
		http.NotFound(w, r)
		return
	}
	if _, ok := s.admin.session(r); ok {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	s.renderNamed(w, r, http.StatusOK, "admin-login", &view.AdminPageData{
		Nonce:   nonceFrom(r.Context()),
		Version: s.version,
		Next:    safeAdminNext(r.URL.Query().Get("next")),
	})
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if !s.admin.enabled() {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	if err := sameOrigin(r); err != nil {
		http.Error(w, "That request could not be verified. Reload and try again.", http.StatusForbidden)
		return
	}

	next := safeAdminNext(r.FormValue("next"))
	ip := clientIP(r)
	limiterKey := "admin:" + ip
	if !s.auth.limiter.allowed(limiterKey) {
		s.renderAdminLogin(w, r, http.StatusTooManyRequests, next,
			"Too many tries. Wait a few minutes and try again.")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	if !s.admin.matchesUsername(username) || !s.admin.matchesPassword(r.FormValue("password")) {
		s.auth.limiter.fail(limiterKey)
		s.log.Warn("admin login failed", "ip", ip)
		s.renderAdminLogin(w, r, http.StatusUnauthorized, next,
			"That username and password don't match. Try again.")
		return
	}

	s.auth.limiter.reset(limiterKey)
	if err := s.admin.setSession(w, r); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) renderAdminLogin(w http.ResponseWriter, r *http.Request, status int, next, message string) {
	s.renderNamed(w, r, status, "admin-login", &view.AdminPageData{
		Nonce:   nonceFrom(r.Context()),
		Version: s.version,
		Next:    next,
		Error:   message,
	})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	s.admin.clearSession(w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/admin":
	case "/admin/":
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	default:
		http.NotFound(w, r)
		return
	}

	data, err := s.adminPageData(r, adminNotice(r.URL.Query().Get("notice")))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderNamed(w, r, http.StatusOK, "admin", data)
}

func (s *Server) handleAdminCreateInvite(w http.ResponseWriter, r *http.Request) {
	label := strings.TrimSpace(r.PostForm.Get("label"))
	if utf8.RuneCountInString(label) > inviteMaxLabelRunes {
		data, err := s.adminPageData(r, "")
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		data.Error = fmt.Sprintf("Label must be %d characters or fewer.", inviteMaxLabelRunes)
		s.renderNamed(w, r, http.StatusBadRequest, "admin", data)
		return
	}

	days := inviteDefaultDays
	if raw := strings.TrimSpace(r.PostForm.Get("days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || !validInviteDays(parsed) {
			data, loadErr := s.adminPageData(r, "")
			if loadErr != nil {
				s.serverError(w, r, loadErr)
				return
			}
			data.Error = "Choose an expiry of 1, 7, or 30 days."
			s.renderNamed(w, r, http.StatusBadRequest, "admin", data)
			return
		}
		days = parsed
	}

	token, err := newInviteToken()
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	now := s.clock.Now().UTC()
	expires := now.Add(time.Duration(days) * 24 * time.Hour)
	invite, err := s.store.CreateInvite(r.Context(), hashInviteToken(token), label, now, expires)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	data, err := s.adminPageData(r, "")
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.NewInviteURL = s.publicBaseURL(r) + "/signup?invite=" + url.QueryEscape(token)
	data.Notice = fmt.Sprintf("Invitation created. It expires %s.",
		invite.ExpiresAt.In(s.loc).Format("Jan 2, 2006 at 15:04"))
	s.log.Info("invite created", "id", invite.ID, "expires", invite.ExpiresAt, "ip", clientIP(r))
	s.renderNamed(w, r, http.StatusOK, "admin", data)
}

func (s *Server) handleAdminRevokeInvite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.badRequest(w, "That invitation couldn't be found.")
		return
	}
	err = s.store.RevokeInvite(r.Context(), id, s.clock.Now().UTC())
	switch {
	case errors.Is(err, store.ErrInviteNotFound):
		http.Redirect(w, r, "/admin?notice=unchanged", http.StatusSeeOther)
	case err != nil:
		s.serverError(w, r, err)
	default:
		s.log.Info("invite revoked", "id", id)
		http.Redirect(w, r, "/admin?notice=revoked", http.StatusSeeOther)
	}
}

func (s *Server) adminPageData(r *http.Request, notice string) (*view.AdminPageData, error) {
	now := s.clock.Now().UTC()
	invites, err := s.store.ListInvites(r.Context(), inviteListLimit)
	if err != nil {
		return nil, err
	}
	accounts, err := s.store.ListAccounts(r.Context())
	if err != nil {
		return nil, err
	}

	data := &view.AdminPageData{
		Nonce:    nonceFrom(r.Context()),
		Version:  s.version,
		CSRF:     adminCSRFFrom(r.Context()),
		Notice:   notice,
		Invites:  make([]view.AdminInviteRow, 0, len(invites)),
		Accounts: make([]view.AdminAccountRow, 0, len(accounts)),
	}
	for _, invite := range invites {
		row := view.AdminInviteRow{
			ID:           invite.ID,
			Label:        invite.Label,
			CreatedLabel: invite.CreatedAt.In(s.loc).Format("Jan 2, 15:04"),
			ExpiresLabel: invite.ExpiresAt.In(s.loc).Format("Jan 2, 15:04"),
		}
		switch {
		case invite.UsedAt != nil:
			row.Status, row.StatusTone = "Used", "muted"
		case invite.RevokedAt != nil:
			row.Status, row.StatusTone = "Revoked", "muted"
		case !now.Before(invite.ExpiresAt):
			row.Status, row.StatusTone = "Expired", "warn"
		default:
			row.Status, row.StatusTone, row.Active = "Active", "ok", true
		}
		data.Invites = append(data.Invites, row)
	}
	for _, acct := range accounts {
		data.Accounts = append(data.Accounts, view.AdminAccountRow{
			Username:     acct.Username,
			CreatedLabel: acct.CreatedAt.In(s.loc).Format("Jan 2, 2006"),
		})
	}
	return data, nil
}

func adminNotice(raw string) string {
	switch raw {
	case "revoked":
		return "Invitation revoked."
	case "unchanged":
		return "That invitation was already used or revoked."
	default:
		return ""
	}
}

func validInviteDays(days int) bool {
	switch days {
	case 1, 7, 30:
		return true
	default:
		return false
	}
}

// newInviteToken returns 256 bits of entropy in a URL-safe form.
func newInviteToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("server: generate invite token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// publicBaseURL prefers an explicit configuration, because a reverse proxy can
// rewrite the Host header. Otherwise it reconstructs the public origin from
// the request and the usual forwarding headers.
func (s *Server) publicBaseURL(r *http.Request) string {
	if s.publicURL != "" {
		return s.publicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		host = forwarded
	}
	return scheme + "://" + host
}
