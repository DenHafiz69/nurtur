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
	"strconv"
	"strings"
	"sync"
	"time"

	"nurtur/internal/account"
	"nurtur/internal/store"
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

// auth implements Nurtur's local gate. It supports the original single
// passcode and the account flow (username + password created with a signup
// key). When neither is configured, the middleware is a no-op, which keeps
// LAN-only installs simple.
type auth struct {
	passcode  string
	signupKey string
	key       []byte
	secure    bool
	limiter   *loginLimiter

	accountsMu sync.RWMutex
	accounts   bool
}

func newAuth(passcode, signupKey, secret string, secure, hasAccounts bool) *auth {
	if secret == "" {
		sum := sha256.Sum256([]byte("nurtur-session:" + passcode + ":" + signupKey))
		secret = string(sum[:])
	}
	return &auth{
		passcode:  passcode,
		signupKey: signupKey,
		key:       []byte(secret),
		secure:    secure,
		limiter:   newLoginLimiter(),
		accounts:  hasAccounts,
	}
}

func (a *auth) enabled() bool {
	if a == nil {
		return false
	}
	return a.passcode != "" || a.signupKey != "" || a.hasAccounts()
}

func (a *auth) hasAccounts() bool {
	a.accountsMu.RLock()
	defer a.accountsMu.RUnlock()
	return a.accounts
}

func (a *auth) setHasAccounts(has bool) {
	a.accountsMu.Lock()
	a.accounts = has
	a.accountsMu.Unlock()
}

// accountLogin reports whether the gate checks the accounts table. The legacy
// passcode form is used only when no account system is configured.
func (a *auth) accountLogin() bool {
	return a.signupKey != "" || a.hasAccounts()
}

// signupOpen reports whether anyone who knows the signup key may create an
// account. Accounts are shared logins for one household, so signup stays open
// while the key is configured rather than closing after the first account.
func (a *auth) signupOpen() bool {
	return a.signupKey != ""
}

func (a *auth) middleware(next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.enabled() {
			next.ServeHTTP(w, r)
			return
		}
		if !requiresAuth(r) {
			next.ServeHTTP(w, r)
			return
		}

		accountID, csrf, ok := a.session(r)
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
		ctx = store.WithAccount(ctx, accountID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requiresAuth(r *http.Request) bool {
	path := r.URL.Path
	switch {
	case path == "/login", path == "/signup", path == "/logout",
		path == "/healthz",
		path == "/offline",
		path == "/sw.js",
		path == "/favicon.ico":
		return false
	case path == "/admin" || strings.HasPrefix(path, "/admin/"):
		// The admin gate is separate from household accounts.
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
	if err := sameOrigin(r); err != nil {
		return err
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

// sameOrigin rejects a request whose Origin or Sec-Fetch-Site says it came from
// another site. Classic form posts cannot attach a custom header, so this is the
// shared baseline for login and admin forms.
func sameOrigin(r *http.Request) error {
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
	return nil
}

func (a *auth) newSession(accountID int64) (value, csrf string, err error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	csrf = base64.RawURLEncoding.EncodeToString(raw[:])
	payload := strings.Join([]string{
		"v2",
		time.Now().UTC().Add(sessionLifetime).Format(time.RFC3339),
		strconv.FormatInt(accountID, 10),
		csrf,
	}, "|")
	return payload + "." + a.sign(payload), csrf, nil
}

func (a *auth) session(r *http.Request) (accountID int64, csrf string, ok bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return 0, "", false
	}
	payload, sig, found := strings.Cut(cookie.Value, ".")
	if !found || !hmac.Equal([]byte(sig), []byte(a.sign(payload))) {
		return 0, "", false
	}
	parts := strings.Split(payload, "|")
	if len(parts) != 4 || parts[0] != "v2" {
		return 0, "", false
	}
	expires, err := time.Parse(time.RFC3339, parts[1])
	if err != nil || time.Now().UTC().After(expires) {
		return 0, "", false
	}
	accountID, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil || accountID < 0 {
		return 0, "", false
	}
	return accountID, parts[3], true
}

func (a *auth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *auth) setSession(w http.ResponseWriter, r *http.Request, accountID int64, csrf string) {
	if csrf == "" {
		return
	}
	// Rebuild the value so expiry and signature match the token issued above.
	payload := strings.Join([]string{
		"v2",
		time.Now().UTC().Add(sessionLifetime).Format(time.RFC3339),
		strconv.FormatInt(accountID, 10),
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

func (a *auth) matchesSignupKey(candidate string) bool {
	if a.signupKey == "" {
		return false
	}
	want := sha256.Sum256([]byte(a.signupKey))
	got := sha256.Sum256([]byte(strings.TrimSpace(candidate)))
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

// authPageData prepares state for the standalone login/onboarding document.
func (s *Server) authPageData(r *http.Request, mode, next string) *view.PageData {
	return &view.PageData{
		Nonce:        nonceFrom(r.Context()),
		Version:      s.version,
		AuthEnabled:  true,
		AuthMode:     mode,
		Next:         next,
		AccountLogin: s.auth.accountLogin(),
		SignupOpen:   s.auth.signupOpen(),
		MaxBirthDate: s.todayISO(),
	}
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if !s.auth.enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	mode := r.URL.Query().Get("mode")
	switch mode {
	case "signin", "signup":
	default:
		// A fresh browser is asked whether it already has an account. Installs
		// using only the legacy passcode go straight to the passcode form.
		if s.auth.accountLogin() {
			mode = "chooser"
		} else {
			mode = "login"
		}
	}
	next := safeNext(r.URL.Query().Get("next"))
	s.renderNamed(w, r, http.StatusOK, "login", s.authPageData(r, mode, next))
}

func (s *Server) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	// A configured admin can invite people even when the shared signup key is
	// not set, so the admin password also keeps this page reachable.
	if !s.auth.enabled() && !s.admin.enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	data := s.authPageData(r, "signup", next)
	if err := s.applyInviteToken(r.Context(), data, r.URL.Query().Get("invite")); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderNamed(w, r, http.StatusOK, "login", data)
}

// applyInviteToken fills the signup page state from an invite link. A missing
// token is a normal signup page; an unusable token gets a calm explanation
// instead of the shared-secret form.
func (s *Server) applyInviteToken(ctx context.Context, data *view.PageData, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	invite, err := s.store.InviteByTokenHash(ctx, hashInviteToken(token))
	switch {
	case errors.Is(err, store.ErrInviteNotFound):
		data.InviteError = "This invitation link isn't valid. Ask the person who invited you for a new one."
		return nil
	case err != nil:
		return err
	}
	now := s.clock.Now().UTC()
	if !invite.Active(now) {
		data.InviteError = inviteUnavailableMessage(invite, now)
		return nil
	}
	data.InviteToken = token
	data.InviteValid = true
	data.InviteLabel = invite.Label
	return nil
}

func inviteUnavailableMessage(invite store.Invite, now time.Time) string {
	switch {
	case invite.UsedAt != nil:
		return "This invitation has already been used. Sign in instead, or ask for a new link."
	case invite.RevokedAt != nil:
		return "This invitation was revoked. Ask the person who invited you for a new link."
	case !now.Before(invite.ExpiresAt):
		return "This invitation link has expired. Ask for a new one."
	default:
		return "This invitation link isn't valid. Ask for a new one."
	}
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
	next := safeNext(r.FormValue("next"))
	ip := clientIP(r)
	limiterKey := "login:" + ip
	if !s.auth.limiter.allowed(limiterKey) {
		data := s.authPageData(r, "login", next)
		data.LoginError = "Too many tries. Wait a few minutes and try again."
		s.renderNamed(w, r, http.StatusTooManyRequests, "login", data)
		return
	}

	if s.auth.accountLogin() {
		rawUsername := strings.TrimSpace(r.FormValue("username"))
		username, err := account.NormalizeUsername(rawUsername)
		var acct store.Account
		if err == nil {
			acct, err = s.store.AccountByUsername(r.Context(), username)
		}
		switch {
		case errors.Is(err, store.ErrAccountNotFound), errors.Is(err, account.ErrInvalidUsername):
			// Fall through to the generic message below.
		case err != nil:
			s.serverError(w, r, err)
			return
		}
		if err != nil || !account.VerifyPassword(acct.PasswordHash, r.FormValue("password")) {
			s.auth.limiter.fail(limiterKey)
			s.log.Warn("account login failed", "ip", ip)
			data := s.authPageData(r, "login", next)
			data.Username = rawUsername
			data.LoginError = "That username and password don't match. Try again."
			s.renderNamed(w, r, http.StatusUnauthorized, "login", data)
			return
		}
		s.auth.setHasAccounts(true)
		s.auth.limiter.reset(limiterKey)
		s.startSession(w, r, acct.ID, next)
		return
	}

	if !s.auth.matches(r.FormValue("passcode")) {
		s.auth.limiter.fail(limiterKey)
		s.log.Warn("login failed", "ip", ip)
		data := s.authPageData(r, "login", next)
		data.LoginError = "That passcode isn't right. Try again?"
		s.renderNamed(w, r, http.StatusUnauthorized, "login", data)
		return
	}

	s.auth.limiter.reset(limiterKey)
	s.startSession(w, r, 0, next)
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	// A configured admin can invite people even when the shared signup key is
	// not set, so the admin password also keeps signup reachable.
	if !s.auth.enabled() && !s.admin.enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, "Couldn't read that form. Try again.")
		return
	}
	next := safeNext(r.FormValue("next"))

	inviteToken := strings.TrimSpace(r.FormValue("invite"))
	var invite store.Invite
	hasInvite := false
	if inviteToken != "" {
		found, err := s.store.InviteByTokenHash(r.Context(), hashInviteToken(inviteToken))
		switch {
		case errors.Is(err, store.ErrInviteNotFound):
			s.renderSignupInviteError(w, r, next,
				"This invitation link isn't valid. Ask the person who invited you for a new one.")
			return
		case err != nil:
			s.serverError(w, r, err)
			return
		}
		now := s.clock.Now().UTC()
		if !found.Active(now) {
			s.renderSignupInviteError(w, r, next, inviteUnavailableMessage(found, now))
			return
		}
		invite = found
		hasInvite = true
	}

	if !hasInvite && !s.auth.signupOpen() {
		data := s.authPageData(r, "signup", next)
		data.SignupError = "Sign-ups are closed on this server. Ask the person who set it up for the secret key."
		s.renderNamed(w, r, http.StatusForbidden, "login", data)
		return
	}

	ip := clientIP(r)
	limiterKey := "signup:" + ip
	if !s.auth.limiter.allowed(limiterKey) {
		data := s.authPageData(r, "signup", next)
		data.SignupError = "Too many tries. Wait a few minutes and try again."
		setInviteData(data, hasInvite, inviteToken, invite.Label)
		s.renderNamed(w, r, http.StatusTooManyRequests, "login", data)
		return
	}

	rawUsername := strings.TrimSpace(r.FormValue("username"))
	rawBabyName := r.FormValue("baby_name")
	rawBirthDate := r.FormValue("baby_birth_date")
	rawGender := r.FormValue("baby_gender")
	rawMascot := r.FormValue("mascot")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")
	reject := func(status int, message string) {
		data := s.authPageData(r, "signup", next)
		data.Username = rawUsername
		data.BabyName = rawBabyName
		data.BabyBirthDate = rawBirthDate
		data.BabyGender = rawGender
		data.MascotName = rawMascot
		data.SignupError = message
		setInviteData(data, hasInvite, inviteToken, invite.Label)
		s.renderNamed(w, r, status, "login", data)
	}

	// An invite replaces the shared secret key for exactly this signup.
	if !hasInvite && !s.auth.matchesSignupKey(r.FormValue("secret_key")) {
		s.auth.limiter.fail(limiterKey)
		s.log.Warn("signup rejected: wrong secret key", "ip", ip)
		reject(http.StatusUnauthorized, "That secret key isn't right.")
		return
	}
	username, err := account.NormalizeUsername(rawUsername)
	if err != nil {
		reject(http.StatusBadRequest, account.ErrInvalidUsername.Error())
		return
	}
	if err := account.ValidatePassword(password); err != nil {
		reject(http.StatusBadRequest, account.ErrInvalidPassword.Error())
		return
	}
	if password != confirm {
		reject(http.StatusBadRequest, "Those passwords don't match.")
		return
	}
	babyName, err := parseBabyName(rawBabyName)
	if err != nil {
		reject(http.StatusBadRequest, babyNameMessage(err))
		return
	}
	birthDate, err := parseBirthDate(rawBirthDate, s.clock.Now(), s.loc)
	if err != nil {
		reject(http.StatusBadRequest, birthDateMessage(err))
		return
	}
	gender, err := parseBabyGender(rawGender)
	if err != nil {
		reject(http.StatusBadRequest, genderMessage(err))
		return
	}
	mascot, err := parseMascot(rawMascot)
	if err != nil {
		reject(http.StatusBadRequest, mascotMessage(err))
		return
	}

	hash, err := account.HashPassword(password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	settings := map[string]string{
		settingBabyName:      babyName,
		settingBabyBirthDate: birthDate.Format("2006-01-02"),
		settingBabyGender:    gender,
		settingMascot:        mascot,
	}
	// A brand-new account starts caught up so onboarding does not open the
	// update sheet for a release it never knew about.
	if seen, ok := s.changelogSeenSetting(); ok {
		settings[settingChangelogSeen] = seen
	}
	now := s.clock.Now()
	var acct store.Account
	if hasInvite {
		acct, err = s.store.CreateAccountWithInvite(
			r.Context(), username, hash, settings, invite.ID, now,
		)
	} else {
		acct, err = s.store.CreateAccountWithSettings(
			r.Context(), username, hash, settings, now,
		)
	}
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInviteUnavailable):
			// Another signup used the link between the page load and submit.
			s.renderSignupInviteError(w, r, next,
				"This invitation has just been used. Ask the person who invited you for a new link.")
			return
		case errors.Is(err, store.ErrAccountExists):
			s.auth.setHasAccounts(true)
			reject(http.StatusConflict, "That username is already taken. Try another.")
			return
		default:
			s.serverError(w, r, err)
			return
		}
	}

	s.auth.setHasAccounts(true)
	s.auth.limiter.reset(limiterKey)
	s.log.Info("account created", "username", username, "id", acct.ID, "ip", ip, "invite", hasInvite)
	s.startSession(w, r, acct.ID, next)
}

func setInviteData(data *view.PageData, hasInvite bool, token, label string) {
	if !hasInvite {
		return
	}
	data.InviteToken = token
	data.InviteValid = true
	data.InviteLabel = label
}

func (s *Server) renderSignupInviteError(w http.ResponseWriter, r *http.Request, next, message string) {
	data := s.authPageData(r, "signup", next)
	data.InviteError = message
	s.renderNamed(w, r, http.StatusForbidden, "login", data)
}

// startSession issues the signed session cookie and returns to the page the
// person originally asked for.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, accountID int64, next string) {
	_, csrf, err := s.auth.newSession(accountID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.auth.setSession(w, r, accountID, csrf)
	// #nosec G710 -- next has passed through safeNext, which allows only
	// same-origin paths and rejects absolute or backslash-normalised targets.
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth.enabled() {
		s.auth.clearSession(w, r)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// renderNamed renders a top-level template without the dashboard shell. The
// login, admin, and offline pages own their whole document.
func (s *Server) renderNamed(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
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
