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
		"Nursing &amp; live timer",
		"Bottles in one tap",
		"Pumping sessions",
		"Sleep &amp; wake windows",
		"Diapers at a glance",
		"Solids &amp; first meals",
		"Private baby album",
		`href="/demo"`,
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

func TestDemoModeWithoutLoginAndIsolation(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.SignupKey = "family-only"
		cfg.SessionSecret = "test-session-secret"
	})

	// Unauthenticated visitor cannot hit /timeline directly.
	rec := env.do(t, http.MethodGet, "/timeline", nil, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Fatalf("signed-out /timeline = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// Opening /demo starts a demo session without logging in.
	rec = env.do(t, http.MethodGet, "/demo", nil, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("GET /demo = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	var demoCookie string
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			demoCookie = cookie.Name + "=" + cookie.Value
		}
	}
	if demoCookie == "" {
		t.Fatal("GET /demo did not set a session cookie")
	}
	headers := map[string]string{"Cookie": demoCookie}

	// Demo session lands directly on the full app dashboard with seeded data.
	rec = doWith(t, env, http.MethodGet, "/", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("demo root = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="timer-card"`,
		"Demo mode",
		"Aisyah",
		"Oatmeal &amp; banana",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("demo dashboard missing %q", want)
		}
	}

	// Demo session can use /timeline and /settings without logging in.
	rec = doWith(t, env, http.MethodGet, "/timeline", nil, headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("demo /timeline = %d", rec.Code)
	}

	// Demo session does not create real accounts in the accounts table.
	count, err := env.store.CountAccounts(t.Context())
	if err != nil {
		t.Fatalf("CountAccounts: %v", err)
	}
	if count != 0 {
		t.Fatalf("demo created %d real accounts, want 0", count)
	}

	// Resetting the demo issues a fresh demo cookie and redirects to /.
	rec = doWith(t, env, http.MethodPost, "/demo/reset", nil, headers)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("POST /demo/reset = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// Exiting the demo clears the session and returns to /.
	rec = doWith(t, env, http.MethodPost, "/demo/exit", nil, headers)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("POST /demo/exit = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	var cleared bool
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("POST /demo/exit did not clear session cookie")
	}

	// /landing is always directly reachable.
	rec = env.do(t, http.MethodGet, "/landing", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Try live demo") {
		t.Fatalf("GET /landing = %d", rec.Code)
	}
}
