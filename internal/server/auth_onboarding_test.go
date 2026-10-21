package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOnboardingSignupThenLogin(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.SignupKey = "only-for-my-wife"
		cfg.SessionSecret = "test-session-secret"
	})

	// An unauthenticated browser is asked the onboarding question.
	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Fatalf("root = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = env.do(t, http.MethodGet, "/login", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("onboarding status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Do you already have an account?",
		"/login?mode=signin",
		"/login?mode=signup",
		"Yes, sign in",
		"No, I'm new here",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("onboarding missing %q\n%s", want, body)
		}
	}

	// The signup form asks for the secret key before the account details.
	rec = env.do(t, http.MethodGet, "/login?mode=signup", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("signup page = %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"Secret key", "Username", "Baby's name", "Confirm password", "Create account"} {
		if !strings.Contains(body, want) {
			t.Errorf("signup page missing %q\n%s", want, body)
		}
	}

	// Wrong key creates nothing.
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"wrong"},
		"username":         {"wife"},
		"baby_name":        {"Aisyah"},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
	}, false)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "secret key") {
		t.Fatalf("wrong key = %d\n%s", rec.Code, rec.Body.String())
	}
	if count, err := env.store.CountAccounts(context.Background()); err != nil || count != 0 {
		t.Fatalf("wrong key created an account: count=%d err=%v", count, err)
	}

	// A valid key, matching passwords, and a normalized username create the
	// account and sign this browser in.
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"only-for-my-wife"},
		"username":         {"  Wife  "},
		"baby_name":        {"  Aisyah  "},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
		"next":             {"/settings"},
	}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings" {
		t.Fatalf("signup = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if count, err := env.store.CountAccounts(context.Background()); err != nil || count != 1 {
		t.Fatalf("signup count=%d err=%v", count, err)
	}
	acct, err := env.store.AccountByUsername(context.Background(), "wife")
	if err != nil {
		t.Fatalf("stored account: %v", err)
	}
	if strings.Contains(acct.PasswordHash, "correct horse") {
		t.Fatal("password was stored in plain text")
	}
	storedName, ok, err := env.store.Setting(context.Background(), settingBabyName)
	if err != nil || !ok || storedName != "Aisyah" {
		t.Fatalf("stored baby_name = %q, ok=%v, err=%v", storedName, ok, err)
	}

	// The signup is now closed: the form explains that an account exists.
	rec = env.do(t, http.MethodGet, "/login?mode=signup", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("closed signup = %d\n%s", rec.Code, rec.Body.String())
	}
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"only-for-my-wife"},
		"username":         {"intruder"},
		"baby_name":        {"Intruder"},
		"password":         {"another password"},
		"confirm_password": {"another password"},
	}, false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second signup = %d", rec.Code)
	}

	// A new browser can choose "Yes, sign in" and log in with the account.
	rec = env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"WIFE"},
		"password": {"wrong"},
	}, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d", rec.Code)
	}
	rec = env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"WIFE"},
		"password": {"correct horse"},
	}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("login = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a session cookie")
	}

	rec = doWith(t, env, http.MethodGet, "/", nil, map[string]string{
		"Cookie": cookies[0].Name + "=" + cookies[0].Value,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated dashboard = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Aisyah") {
		t.Fatalf("dashboard missing the onboarded baby name:\n%s", rec.Body.String())
	}
}

func TestOnboardingRejectsWeakCredentials(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.SignupKey = "only-for-my-wife"
	})

	post := func(username, password, confirm, babyName string) *httptest.ResponseRecorder {
		return env.do(t, http.MethodPost, "/signup", url.Values{
			"secret_key":       {"only-for-my-wife"},
			"username":         {username},
			"baby_name":        {babyName},
			"password":         {password},
			"confirm_password": {confirm},
		}, false)
	}

	if rec := post("ab", "correct horse", "correct horse", "Aisyah"); rec.Code != http.StatusBadRequest {
		t.Fatalf("short username = %d", rec.Code)
	}
	if rec := post("wife", "short", "short", "Aisyah"); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password = %d", rec.Code)
	}
	rec := post("wife", "correct horse", "different horse", "Aisyah")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "match") {
		t.Fatalf("password mismatch = %d\n%s", rec.Code, rec.Body.String())
	}
	rec = post("wife", "correct horse", "correct horse", "   ")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "name is required") {
		t.Fatalf("missing baby name = %d\n%s", rec.Code, rec.Body.String())
	}
	if count, err := env.store.CountAccounts(context.Background()); err != nil || count != 0 {
		t.Fatalf("rejected signup created an account: count=%d err=%v", count, err)
	}
}
