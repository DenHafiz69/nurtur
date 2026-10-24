package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInviteLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	expires := testNow.Add(24 * time.Hour)

	invite, err := st.CreateInvite(ctx, "hash-one", "Aunty Sarah", testNow, expires)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if invite.ID == 0 || invite.Label != "Aunty Sarah" || !invite.Active(testNow) {
		t.Fatalf("created invite = %+v", invite)
	}

	got, err := st.InviteByTokenHash(ctx, "hash-one")
	if err != nil {
		t.Fatalf("InviteByTokenHash: %v", err)
	}
	if got.ID != invite.ID || got.Label != invite.Label || !got.CreatedAt.Equal(testNow) {
		t.Fatalf("looked up invite = %+v", got)
	}

	if _, err := st.InviteByTokenHash(ctx, "missing"); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("missing invite error = %v, want ErrInviteNotFound", err)
	}

	invites, err := st.ListInvites(ctx, 10)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(invites) != 1 || invites[0].ID != invite.ID {
		t.Fatalf("listed invites = %+v", invites)
	}

	if err := st.RevokeInvite(ctx, invite.ID, testNow); err != nil {
		t.Fatalf("RevokeInvite: %v", err)
	}
	got, err = st.InviteByTokenHash(ctx, "hash-one")
	if err != nil {
		t.Fatalf("InviteByTokenHash after revoke: %v", err)
	}
	if got.RevokedAt == nil || got.Active(testNow) {
		t.Fatalf("revoked invite = %+v", got)
	}
	if err := st.RevokeInvite(ctx, invite.ID, testNow); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("second revoke error = %v, want ErrInviteNotFound", err)
	}
}

func TestCreateAccountWithInviteConsumesOnce(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	invite, err := st.CreateInvite(ctx, "hash-two", "", testNow, testNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	acct, err := st.CreateAccountWithInvite(ctx, "wife", "hash", map[string]string{
		"baby_name": "Aisyah",
	}, invite.ID, testNow)
	if err != nil {
		t.Fatalf("CreateAccountWithInvite: %v", err)
	}

	used, err := st.InviteByTokenHash(ctx, "hash-two")
	if err != nil {
		t.Fatalf("InviteByTokenHash: %v", err)
	}
	if used.UsedAt == nil || used.UsedAccountID == nil || *used.UsedAccountID != acct.ID {
		t.Fatalf("used invite = %+v", used)
	}
	if used.Active(testNow) {
		t.Fatal("used invite is still active")
	}

	if _, err := st.CreateAccountWithInvite(ctx, "other", "hash", nil, invite.ID, testNow); !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("second use error = %v, want ErrInviteUnavailable", err)
	}
	count, err := st.CountAccounts(ctx)
	if err != nil {
		t.Fatalf("CountAccounts: %v", err)
	}
	if count != 1 {
		t.Fatalf("account count = %d, want 1 (failed signup should roll back)", count)
	}
}

func TestCreateAccountWithExpiredInviteFails(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	invite, err := st.CreateInvite(ctx, "hash-three", "", testNow.Add(-2*time.Hour), testNow.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if invite.Active(testNow) {
		t.Fatal("expired invite reports active")
	}

	if _, err := st.CreateAccountWithInvite(ctx, "wife", "hash", nil, invite.ID, testNow); !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("expired invite error = %v, want ErrInviteUnavailable", err)
	}
	count, err := st.CountAccounts(ctx)
	if err != nil {
		t.Fatalf("CountAccounts: %v", err)
	}
	if count != 0 {
		t.Fatalf("account count = %d, want 0", count)
	}
}
