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

	second, err := st.CreateAccount(ctx, "second", "hash-two", testNow)
	if err != nil {
		t.Fatalf("second account: %v", err)
	}
	if second.ID == acct.ID {
		t.Fatalf("second account reused id %d", acct.ID)
	}

	if _, err := st.CreateAccount(ctx, "wife", "hash-three", testNow); !errors.Is(err, ErrAccountExists) {
		t.Fatalf("duplicate username error = %v, want ErrAccountExists", err)
	}

	count, err = st.CountAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("account count = %d, want 2", count)
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
	acctCtx := WithAccount(ctx, acct.ID)
	name, ok, err := st.Setting(acctCtx, "baby_name")
	if err != nil || !ok || name != "Aisyah" {
		t.Fatalf("baby_name = %q, ok=%v, err=%v", name, ok, err)
	}

	// An additional account has its own settings namespace.
	other, err := st.CreateAccountWithSettings(ctx, "other", "hash", map[string]string{
		"baby_name": "Someone else",
	}, testNow)
	if err != nil {
		t.Fatalf("additional account: %v", err)
	}
	name, _, err = st.Setting(acctCtx, "baby_name")
	if err != nil || name != "Aisyah" {
		t.Fatalf("first account baby_name = %q, err=%v", name, err)
	}
	name, ok, err = st.Setting(WithAccount(ctx, other.ID), "baby_name")
	if err != nil || !ok || name != "Someone else" {
		t.Fatalf("second account baby_name = %q, ok=%v, err=%v", name, ok, err)
	}
}
