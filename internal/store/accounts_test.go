package store

import (
	"context"
	"errors"
	"testing"
)

func TestAccountLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	count, err := st.CountAccounts(ctx)
	if err != nil {
		t.Fatalf("CountAccounts: %v", err)
	}
	if count != 0 {
		t.Fatalf("fresh database has %d accounts, want 0", count)
	}

	acct, err := st.CreateAccount(ctx, "wife", "hash-one", testNow)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acct.ID == 0 || acct.Username != "wife" || !acct.CreatedAt.Equal(testNow) {
		t.Fatalf("created account = %+v", acct)
	}

	got, err := st.AccountByUsername(ctx, "wife")
	if err != nil {
		t.Fatalf("AccountByUsername: %v", err)
	}
	if got.ID != acct.ID || got.PasswordHash != "hash-one" {
		t.Fatalf("looked up account = %+v", got)
	}

	if _, err := st.AccountByUsername(ctx, "missing"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("missing account error = %v, want ErrAccountNotFound", err)
	}

	if _, err := st.CreateAccount(ctx, "second", "hash-two", testNow); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("second account error = %v, want ErrAccountExists", err)
	}

	count, err = st.CountAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("account count = %d, want 1", count)
	}
}

func TestCreateAccountWithSettingsIsAtomic(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	acct, err := st.CreateAccountWithSettings(ctx, "wife", "hash", map[string]string{
		"baby_name": "Aisyah",
	}, testNow)
	if err != nil {
		t.Fatalf("CreateAccountWithSettings: %v", err)
	}
	if acct.Username != "wife" {
		t.Fatalf("account = %+v", acct)
	}
	name, ok, err := st.Setting(ctx, "baby_name")
	if err != nil || !ok || name != "Aisyah" {
		t.Fatalf("baby_name = %q, ok=%v, err=%v", name, ok, err)
	}

	// A second attempt must not overwrite the first account or its settings.
	if _, err := st.CreateAccountWithSettings(ctx, "other", "hash", map[string]string{
		"baby_name": "Someone else",
	}, testNow); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("second account error = %v, want ErrAccountExists", err)
	}
	name, _, err = st.Setting(ctx, "baby_name")
	if err != nil || name != "Aisyah" {
		t.Fatalf("baby_name after rejected signup = %q, err=%v", name, err)
	}
}
