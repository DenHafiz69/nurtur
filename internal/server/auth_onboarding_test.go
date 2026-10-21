package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"nurtur/internal/store"
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
	for _, want := range []string{"Secret key", "Username", "Baby's name", "date of birth", "Gender", "Confirm password", "Create account"} {
		if !strings.Contains(body, want) {
			t.Errorf("signup page missing %q\n%s", want, body)
		}
	}

	// Wrong key creates nothing.
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"wrong"},
		"username":         {"wife"},
		"baby_name":        {"Aisyah"},
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
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
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
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
	acctCtx := store.WithAccount(context.Background(), acct.ID)
	storedName, ok, err := env.store.Setting(acctCtx, settingBabyName)
	if err != nil || !ok || storedName != "Aisyah" {
		t.Fatalf("stored baby_name = %q, ok=%v, err=%v", storedName, ok, err)
	}
	storedBirthDate, ok, err := env.store.Setting(acctCtx, settingBabyBirthDate)
	if err != nil || !ok || storedBirthDate != "2026-06-20" {
		t.Fatalf("stored baby_birth_date = %q, ok=%v, err=%v", storedBirthDate, ok, err)
	}
	storedGender, ok, err := env.store.Setting(acctCtx, settingBabyGender)
	if err != nil || !ok || storedGender != "girl" {
		t.Fatalf("stored baby_gender = %q, ok=%v, err=%v", storedGender, ok, err)
	}

	// Signup stays open for anyone with the secret key. Each account gets its
	// own baby profile and data, so the form always asks for a name.
	rec = env.do(t, http.MethodGet, "/login?mode=signup", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("open signup = %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"Secret key", "Baby's name"} {
		if !strings.Contains(body, want) {
			t.Fatalf("open signup missing %q:\n%s", want, body)
		}
	}
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"only-for-my-wife"},
		"username":         {"husband"},
		"baby_name":        {"Someone else"},
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
		"password":         {"another password"},
		"confirm_password": {"another password"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("second signup = %d\n%s", rec.Code, rec.Body.String())
	}
	if count, err := env.store.CountAccounts(context.Background()); err != nil || count != 2 {
		t.Fatalf("second signup count=%d err=%v", count, err)
	}
	storedName, ok, err = env.store.Setting(acctCtx, settingBabyName)
	if err != nil || !ok || storedName != "Aisyah" {
		t.Fatalf("first account baby_name = %q, ok=%v, err=%v", storedName, ok, err)
	}
	husband, err := env.store.AccountByUsername(context.Background(), "husband")
	if err != nil {
		t.Fatalf("husband account: %v", err)
	}
	husbandName, ok, err := env.store.Setting(store.WithAccount(context.Background(), husband.ID), settingBabyName)
	if err != nil || !ok || husbandName != "Someone else" {
		t.Fatalf("second account baby_name = %q, ok=%v, err=%v", husbandName, ok, err)
	}

	// A username can only be claimed once.
	rec = env.do(t, http.MethodPost, "/signup", url.Values{
		"secret_key":       {"only-for-my-wife"},
		"username":         {"husband"},
		"baby_name":        {"Another baby"},
		"baby_birth_date":  {"2026-06-20"},
		"baby_gender":      {"girl"},
		"password":         {"another password"},
		"confirm_password": {"another password"},
	}, false)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already taken") {
		t.Fatalf("duplicate signup = %d\n%s", rec.Code, rec.Body.String())
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

func TestAccountsHaveSeparateData(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.SignupKey = "only-for-my-wife"
		cfg.SessionSecret = "test-session-secret"
	})

	signup := func(username, babyName string) *http.Cookie {
		t.Helper()
		rec := env.do(t, http.MethodPost, "/signup", url.Values{
			"secret_key":       {"only-for-my-wife"},
			"username":         {username},
			"baby_name":        {babyName},
			"baby_birth_date":  {"2026-06-20"},
			"baby_gender":      {"girl"},
			"password":         {"correct horse"},
			"confirm_password": {"correct horse"},
		}, false)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("signup %s = %d\n%s", username, rec.Code, rec.Body.String())
		}
		for _, cookie := range rec.Result().Cookies() {
			if cookie.Name == sessionCookieName {
				return cookie
			}
		}
		t.Fatalf("signup %s did not set a session cookie", username)
		return nil
	}

	wife := signup("wife", "Aisyah")
	husband := signup("husband", "Bilal")
	headers := func(cookie *http.Cookie) map[string]string {
		return map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
	}
	get := func(cookie *http.Cookie, path string) *httptest.ResponseRecorder {
		t.Helper()
		return doWith(t, env, http.MethodGet, path, nil, headers(cookie))
	}

	// The wife's feed belongs only to her account.
	rec := doWith(t, env, http.MethodPost, "/feeds", url.Values{"side": {"left"}}, headers(wife))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("wife start feed = %d\n%s", rec.Code, rec.Body.String())
	}

	wifeBody := get(wife, "/").Body.String()
	if !strings.Contains(wifeBody, "Aisyah") || !strings.Contains(wifeBody, `data-feed-id=`) {
		t.Fatalf("wife dashboard missing her own data:\n%s", wifeBody)
	}
	husbandBody := get(husband, "/").Body.String()
	if !strings.Contains(husbandBody, "Bilal") {
		t.Fatalf("husband dashboard missing his baby name:\n%s", husbandBody)
	}
	if strings.Contains(husbandBody, "Aisyah") || strings.Contains(husbandBody, `data-feed-id=`) {
		t.Fatalf("husband dashboard leaked the wife's data:\n%s", husbandBody)
	}
	if !strings.Contains(husbandBody, "Tap a side to start.") {
		t.Fatalf("husband should start with an idle dashboard:\n%s", husbandBody)
	}

	// His own feed is independent of hers.
	rec = doWith(t, env, http.MethodPost, "/feeds", url.Values{"side": {"right"}}, headers(husband))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("husband start feed = %d\n%s", rec.Code, rec.Body.String())
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
			"baby_birth_date":  {"2026-06-20"},
			"baby_gender":      {"girl"},
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

func TestSignupRejectsInvalidBabyDetails(t *testing.T) {
	env := newTestEnvConfig(t, func(cfg *Config) {
		cfg.SignupKey = "only-for-my-wife"
	})

	post := func(birthDate, gender string) *httptest.ResponseRecorder {
		return env.do(t, http.MethodPost, "/signup", url.Values{
			"secret_key":       {"only-for-my-wife"},
			"username":         {"wife"},
			"baby_name":        {"Aisyah"},
			"baby_birth_date":  {birthDate},
			"baby_gender":      {gender},
			"password":         {"correct horse"},
			"confirm_password": {"correct horse"},
		}, false)
	}

	cases := []struct {
		name     string
		birth    string
		gender   string
		contains string
	}{
		{"missing date", "", "girl", "date of birth is required"},
		{"invalid date", "20/06/2026", "girl", "look right"},
		{"future date", "2026-10-21", "girl", "in the future"},
		{"too old", "2021-10-19", "girl", "more than 5 years"},
		{"missing gender", "2026-06-20", "", "Choose the baby"},
		{"invalid gender", "2026-06-20", "other", "Choose Girl or Boy"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(tt.birth, tt.gender)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.contains) {
				t.Fatalf("signup = %d\n%s", rec.Code, rec.Body.String())
			}
		})
	}
	if count, err := env.store.CountAccounts(context.Background()); err != nil || count != 0 {
		t.Fatalf("rejected signup created an account: count=%d err=%v", count, err)
	}
}
