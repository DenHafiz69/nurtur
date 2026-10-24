package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"nurtur/internal/account"
	"nurtur/internal/store"
)

func TestExistingAccountCompletesBabyProfileOnce(t *testing.T) {
	env := newTestEnvWithoutProfile(t, func(cfg *Config) {
		cfg.SignupKey = "only-for-my-wife"
		cfg.SessionSecret = "test-session-secret"
	})

	hash, err := account.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// This is what an account created before the profile fields existed looks
	// like: a name, but no birth date or gender.
	if _, err := env.store.CreateAccountWithSettings(
		context.Background(), "wife", hash,
		map[string]string{settingBabyName: "Aisyah"},
		testNow,
	); err != nil {
		t.Fatalf("CreateAccountWithSettings: %v", err)
	}

	rec := env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"wife"},
		"password": {"correct horse"},
	}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login = %d\n%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a session cookie")
	}
	session := map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value}

	// An already signed-in account is redirected before the dashboard renders.
	rec = doWith(t, env, http.MethodGet, "/", nil, session)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/welcome?next=%2F" {
		t.Fatalf("dashboard before profile = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = doWith(t, env, http.MethodGet, "/welcome?next=%2F", nil, session)
	if rec.Code != http.StatusOK {
		t.Fatalf("welcome = %d\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"One more thing", "Aisyah", "date of birth", "Girl", "Boy", "Mascot", "Mochi", "can't be changed"} {
		if !strings.Contains(body, want) {
			t.Errorf("welcome missing %q\n%s", want, body)
		}
	}

	// A future birth date is rejected and nothing is stored.
	rec = doWith(t, env, http.MethodPost, "/welcome", url.Values{
		"baby_birth_date": {"2030-01-01"},
		"baby_gender":     {"girl"},
		"next":            {"/"},
	}, session)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "in the future") {
		t.Fatalf("future date = %d\n%s", rec.Code, rec.Body.String())
	}

	rec = doWith(t, env, http.MethodPost, "/welcome", url.Values{
		"baby_birth_date": {"2026-06-20"},
		"baby_gender":     {"girl"},
		"mascot":          {"cat"},
		"next":            {"/"},
	}, session)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("save welcome = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	acct, err := env.store.AccountByUsername(context.Background(), "wife")
	if err != nil {
		t.Fatalf("AccountByUsername: %v", err)
	}
	acctCtx := store.WithAccount(context.Background(), acct.ID)
	born, ok, err := env.store.Setting(acctCtx, settingBabyBirthDate)
	if err != nil || !ok || born != "2026-06-20" {
		t.Fatalf("stored birth date = %q ok=%v err=%v", born, ok, err)
	}
	gender, ok, err := env.store.Setting(acctCtx, settingBabyGender)
	if err != nil || !ok || gender != "girl" {
		t.Fatalf("stored gender = %q ok=%v err=%v", gender, ok, err)
	}
	mascot, ok, err := env.store.Setting(acctCtx, settingMascot)
	if err != nil || !ok || mascot != "cat" {
		t.Fatalf("stored mascot = %q ok=%v err=%v", mascot, ok, err)
	}

	rec = doWith(t, env, http.MethodGet, "/", nil, session)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard after profile = %d\n%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{"Aisyah", "4 months old", "Girl"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q\n%s", want, body)
		}
	}

	// Replaying the form must not rewrite immutable details.
	rec = doWith(t, env, http.MethodPost, "/welcome", url.Values{
		"baby_birth_date": {"2026-07-01"},
		"baby_gender":     {"boy"},
		"next":            {"/"},
	}, session)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("second save = %d\n%s", rec.Code, rec.Body.String())
	}
	born, _, _ = env.store.Setting(acctCtx, settingBabyBirthDate)
	gender, _, _ = env.store.Setting(acctCtx, settingBabyGender)
	mascot, _, _ = env.store.Setting(acctCtx, settingMascot)
	if born != "2026-06-20" || gender != "girl" || mascot != "cat" {
		t.Fatalf("profile changed on replay: born=%q gender=%q mascot=%q", born, gender, mascot)
	}
}

func TestWelcomePromptWithoutAuth(t *testing.T) {
	env := newTestEnvWithoutProfile(t, nil)

	rec := env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/welcome?next=%2F" {
		t.Fatalf("dashboard = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = env.do(t, http.MethodGet, "/welcome", nil, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "date of birth") {
		t.Fatalf("welcome = %d\n%s", rec.Code, rec.Body.String())
	}

	rec = env.do(t, http.MethodPost, "/welcome", url.Values{
		"baby_birth_date": {"2026-06-20"},
		"baby_gender":     {"boy"},
		"next":            {"/timeline"},
	}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/timeline" {
		t.Fatalf("save welcome = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = env.do(t, http.MethodGet, "/", nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard after profile = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Boy") {
		t.Errorf("dashboard missing gender label:\n%s", rec.Body.String())
	}
	if mascot, ok, err := env.store.Setting(context.Background(), settingMascot); err != nil || !ok || mascot != "pip" {
		t.Fatalf("default mascot = %q ok=%v err=%v", mascot, ok, err)
	}
}

func TestWelcomePageRedirectsWhenProfileIsComplete(t *testing.T) {
	env := newTestEnv(t)

	rec := env.do(t, http.MethodGet, "/welcome", nil, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("welcome with profile = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestParseBirthDate(t *testing.T) {
	now := testNow
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"empty", "", errBirthDateRequired},
		{"garbage", "20/06/2026", errBirthDateInvalid},
		{"future", "2026-10-21", errBirthDateFuture},
		{"too old", "2021-10-19", errBirthDateTooOld},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseBirthDate(tt.raw, now, now.Location()); err != tt.want {
				t.Fatalf("parseBirthDate(%q) = %v, want %v", tt.raw, err, tt.want)
			}
		})
	}
	if _, err := parseBirthDate("2021-10-20", now, now.Location()); err != nil {
		t.Fatalf("exactly five years ago = %v, want nil", err)
	}
	if _, err := parseBabyGender("girl"); err != nil {
		t.Fatalf("parseBabyGender(girl) = %v", err)
	}
	if _, err := parseBabyGender(""); err != errGenderRequired {
		t.Fatalf("parseBabyGender(empty) = %v, want %v", err, errGenderRequired)
	}
	if _, err := parseBabyGender("other"); err != errGenderInvalid {
		t.Fatalf("parseBabyGender(other) = %v, want %v", err, errGenderInvalid)
	}
}
