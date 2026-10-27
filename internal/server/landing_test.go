package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLandingForSignedOutAndDashboardForSignedIn(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.SignupKey = "only-for-my-wife"
		cfg.SessionSecret = "test-session-secret"
	})

	// Signed out: the root is a public landing page, not the baby dashboard.
	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed-out root = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	body := rec.Body.String()
	for _, want := range []string{
		"One less thing to remember.",
		"Nursing that keeps up",
		"Diapers in one tap",
		`href="/login?mode=signin"`,
		`href="/login?mode=signup"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("landing missing %q\n%s", want, body)
		}
	}
	for _, unwanted := range []string{`id="timer-card"`, `hx-post="/feeds"`, "Nothing logged yet today."} {
		if strings.Contains(body, unwanted) {
			t.Errorf("landing leaked dashboard markup %q\n%s", unwanted, body)
		}
	}

	// A browser that just signed up goes straight to the dashboard.
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"only-for-my-wife"},
		"username":         {"wife"},
		"baby_name":        {"Aisyah"},
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("signup = %d\n%s", rec.Code, rec.Body.String())
	}
	var session map[string]string
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			session = map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
		}
	}
	if session == nil {
		t.Fatal("signup did not set a session cookie")
	}

	rec = doWith(t, env, http.MethodGet, "/", nil, session)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed-in root = %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, `id="timer-card"`) || !strings.Contains(body, "Aisyah") {
		t.Fatalf("signed-in root is not the dashboard:\n%s", body)
	}
	if strings.Contains(body, "One less thing to remember.") {
		t.Fatalf("signed-in root served the landing page:\n%s", body)
	}

	// HTMX requests are told to redirect instead of receiving a whole landing
	// document where the caller expects a fragment. This lets boosted tab
	// links fall back to a full sign-in navigation.
	rec = env.do(t, http.MethodGet, "/", nil, true)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("HX-Redirect"), "/login") {
		t.Fatalf("signed-out HTMX root = %d %q", rec.Code, rec.Header().Get("HX-Redirect"))
	}
}
