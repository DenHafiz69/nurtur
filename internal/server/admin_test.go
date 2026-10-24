package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

const testAdminPassword = "admin-secret"

// adminClient keeps the admin cookie between requests, which the shared test
// helper intentionally does not do.
type adminClient struct {
	t      *testing.T
	env    *testEnv
	cookie *http.Cookie
}

func (c *adminClient) do(method, path string, form url.Values) *httptest.ResponseRecorder {
	c.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.env.handler.ServeHTTP(rec, req)
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name != adminSessionCookieName {
			continue
		}
		if cookie.MaxAge < 0 {
			c.cookie = nil
			continue
		}
		c.cookie = cookie
	}
	return rec
}

func adminEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvConfig(t, func(cfg *Config) {
		cfg.AdminPassword = testAdminPassword
		cfg.PublicURL = "https://nurtur.test"
		cfg.SessionSecret = "test-session-secret"
	})
}

func loginAdmin(t *testing.T, env *testEnv) *adminClient {
	t.Helper()
	client := &adminClient{t: t, env: env}
	rec := client.do(http.MethodPost, "/admin/login", url.Values{
		"username": {adminUsername},
		"password": {testAdminPassword},
		"next":     {"/admin"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin login = %d, body = %s", rec.Code, rec.Body.String())
	}
	if client.cookie == nil {
		t.Fatal("admin login did not set a session cookie")
	}
	return client
}

func extractAdminCSRF(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("no csrf token in admin page:\n%s", body)
	}
	return match[1]
}

func extractInviteURL(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`https://nurtur\.test/signup\?invite=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("no invite link in admin page:\n%s", body)
	}
	return match[0]
}

func TestAdminDisabledReturnsNotFound(t *testing.T) {
	env := newTestEnv(t)

	for _, path := range []string{"/admin", "/admin/login"} {
		rec := env.do(t, http.MethodGet, path, nil, false)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	rec := env.do(t, http.MethodPost, "/admin/login", url.Values{
		"username": {adminUsername},
		"password": {"anything"},
	}, false)
	if rec.Code != http.StatusNotFound {
		t.Errorf("POST /admin/login = %d, want 404", rec.Code)
	}
}

func TestAdminLoginRejectsBadPassword(t *testing.T) {
	env := adminEnv(t)

	rec := env.do(t, http.MethodGet, "/admin", nil, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/login?next=%2Fadmin" {
		t.Fatalf("signed-out /admin = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = env.do(t, http.MethodGet, "/admin/login", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login page = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Nurtur admin") {
		t.Fatalf("admin login page missing title:\n%s", body)
	}

	client := &adminClient{t: t, env: env}
	rec = client.do(http.MethodPost, "/admin/login", url.Values{
		"username": {adminUsername},
		"password": {"wrong-password"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", rec.Code)
	}
	if client.cookie != nil {
		t.Fatal("failed login set a session cookie")
	}
}

func TestAdminInviteFlow(t *testing.T) {
	env := adminEnv(t)
	client := loginAdmin(t, env)

	rec := client.do(http.MethodGet, "/admin", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin page = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Magic link", "Generate link", "Invitations", "Accounts"} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page missing %q", want)
		}
	}
	csrf := extractAdminCSRF(t, body)

	// A mutation without the session CSRf token is rejected.
	rec = client.do(http.MethodPost, "/admin/invites", url.Values{
		"label": {"Aunty Sarah"},
		"days":  {"7"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("csrf-less create = %d, want 403", rec.Code)
	}

	rec = client.do(http.MethodPost, "/admin/invites", url.Values{
		"csrf":  {csrf},
		"label": {"Aunty Sarah"},
		"days":  {"7"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create invite = %d, body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Aunty Sarah") || !strings.Contains(body, "Active") {
		t.Fatalf("admin page does not show the new invite:\n%s", body)
	}
	inviteURL := extractInviteURL(t, body)

	// The invitee opens the link directly and sees no secret-key field.
	rec = env.do(t, http.MethodGet, inviteURL, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("invite signup page = %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"You're invited", "Invitation for Aunty Sarah", `name="invite"`, "Create account"} {
		if !strings.Contains(body, want) {
			t.Errorf("invite signup page missing %q", want)
		}
	}
	if strings.Contains(body, `name="secret_key"`) {
		t.Fatal("invite signup page still asks for the secret key")
	}

	form := url.Values{
		"username":         {"wife"},
		"baby_name":        {"Aisyah"},
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
		"mascot":           {"pip"},
		"password":         {"password123"},
		"confirm_password": {"password123"},
	}
	token := inviteURL[strings.Index(inviteURL, "invite=")+len("invite="):]
	form.Set("invite", token)
	rec = env.do(t, http.MethodPost, "/signup", form, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("invite signup = %d %q, body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if _, err := env.store.AccountByUsername(context.Background(), "wife"); err != nil {
		t.Fatalf("account not created: %v", err)
	}

	// The same link cannot create a second account.
	rec = env.do(t, http.MethodGet, inviteURL, nil, false)
	if body := rec.Body.String(); !strings.Contains(body, "already been used") {
		t.Fatalf("reused invite page missing notice:\n%s", body)
	}
	reuse := url.Values{}
	for key, values := range form {
		reuse[key] = append([]string(nil), values...)
	}
	reuse.Set("username", "second")
	rec = env.do(t, http.MethodPost, "/signup", reuse, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reused invite signup = %d, want 403", rec.Code)
	}
	count, err := env.store.CountAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("account count = %d, want 1", count)
	}

	// Revoking a fresh invite closes the link immediately.
	knownToken := "known-invite-token"
	now := env.clock.Now().UTC()
	invite, err := env.store.CreateInvite(context.Background(), hashInviteToken(knownToken), "Uncle", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	rec = client.do(http.MethodGet, "/admin", nil)
	csrf = extractAdminCSRF(t, rec.Body.String())
	rec = client.do(http.MethodPost, fmt.Sprintf("/admin/invites/%d/revoke", invite.ID), url.Values{
		"csrf": {csrf},
	})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin?notice=revoked" {
		t.Fatalf("revoke = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = env.do(t, http.MethodGet, "/signup?invite="+knownToken, nil, false)
	if body := rec.Body.String(); !strings.Contains(body, "was revoked") {
		t.Fatalf("revoked invite page missing notice:\n%s", body)
	}
}

func TestExpiredInviteIsRejected(t *testing.T) {
	env := adminEnv(t)
	now := env.clock.Now().UTC()

	token := "expired-invite-token"
	if _, err := env.store.CreateInvite(context.Background(), hashInviteToken(token), "", now.Add(-2*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	rec := env.do(t, http.MethodGet, "/signup?invite="+token, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expired invite page = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "has expired") {
		t.Fatalf("expired invite page missing notice:\n%s", body)
	}

	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"invite":           {token},
		"username":         {"wife"},
		"baby_name":        {"Aisyah"},
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
		"mascot":           {"pip"},
		"password":         {"password123"},
		"confirm_password": {"password123"},
	}, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expired invite signup = %d, want 403", rec.Code)
	}
	count, err := env.store.CountAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("account count = %d, want 0", count)
	}
}

func TestAdminLogout(t *testing.T) {
	env := adminEnv(t)
	client := loginAdmin(t, env)

	rec := client.do(http.MethodGet, "/admin", nil)
	csrf := extractAdminCSRF(t, rec.Body.String())
	rec = client.do(http.MethodPost, "/admin/logout", url.Values{"csrf": {csrf}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/login" {
		t.Fatalf("logout = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = client.do(http.MethodGet, "/admin", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("after logout = %d, want 303", rec.Code)
	}
}
