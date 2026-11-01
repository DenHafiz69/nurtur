package album

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// tinyWebP is a real 1x1 VP8 WebP. Storage never decodes it; it only needs to
// prove that bytes survive a round trip unchanged.
func tinyWebP(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return raw
}

func TestLocalStorageRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := NewStorage(StorageConfig{Driver: "local", Dir: dir, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	body := tinyWebP(t)
	key := "accounts/1/album/0123456789abcdef0123456789abcdef/full.webp"
	if err := st.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "image/webp"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	size, contentType, err := st.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if size != int64(len(body)) || contentType != "image/webp" {
		t.Fatalf("Stat = %d/%q", size, contentType)
	}

	reader, size, err := st.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if size != int64(len(body)) || !bytes.Equal(got, body) {
		t.Fatalf("round trip = %d bytes, want %d", len(got), len(body))
	}

	// A new driver over the same directory proves the bytes are on disk, not
	// only in the process.
	reopened, err := NewStorage(StorageConfig{Driver: "local", Dir: dir, Logger: testLogger()})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reader, _, err = reopened.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	_ = reader.Close()

	if err := st.Remove(ctx, key); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, _, err := st.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Remove error = %v, want ErrNotFound", err)
	}
	if _, _, err := st.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat after Remove error = %v, want ErrNotFound", err)
	}
}

func TestLocalStorageRejectsEscapingKeys(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := NewStorage(StorageConfig{Driver: "local", Dir: dir, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	for _, key := range []string{"../escape.webp", "/etc/passwd", "accounts/../../escape.webp", "."} {
		err := st.Put(ctx, key, bytes.NewReader([]byte("x")), 1, "image/webp")
		if err == nil {
			t.Errorf("Put(%q) accepted an escaping key", key)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.webp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an escaping write landed outside the storage root: %v", err)
	}
}

func TestStorageAutoFallsBackToLocal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := NewStorage(StorageConfig{Driver: "auto", Dir: dir, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	if err := st.Put(ctx, "photos/test.webp", bytes.NewReader(tinyWebP(t)), 42, "image/webp"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "photos", "test.webp")); err != nil {
		t.Fatalf("local fallback did not write into the album dir: %v", err)
	}
}

func TestStorageR2NeedsCredentials(t *testing.T) {
	if _, err := NewStorage(StorageConfig{Driver: "r2", Dir: t.TempDir(), Logger: testLogger()}); err == nil {
		t.Fatal("r2 storage accepted an empty credential block")
	}
}

// TestR2StorageRoundTrip is opt-in because it talks to a real private bucket.
// Run it with NURTUR_R2_TEST=1 and the four R2 values in the environment.
func TestR2StorageRoundTrip(t *testing.T) {
	if os.Getenv("NURTUR_R2_TEST") != "1" {
		t.Skip("set NURTUR_R2_TEST=1 to run the R2 smoke test")
	}
	cfg := R2Config{
		Endpoint:        os.Getenv("NURTUR_R2_ENDPOINT"),
		Bucket:          os.Getenv("NURTUR_R2_BUCKET"),
		AccessKeyID:     os.Getenv("NURTUR_R2_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("NURTUR_R2_SECRET_ACCESS_KEY"),
		Region:          os.Getenv("NURTUR_R2_REGION"),
	}
	if !cfg.Complete() {
		t.Skip("set NURTUR_R2_ENDPOINT, NURTUR_R2_BUCKET, NURTUR_R2_ACCESS_KEY_ID, and NURTUR_R2_SECRET_ACCESS_KEY")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	st, err := NewStorage(StorageConfig{Driver: "r2", R2: cfg, Logger: testLogger()})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	body := tinyWebP(t)
	key := fmt.Sprintf("nurtur-tests/%d/full.webp", time.Now().UnixNano())
	t.Cleanup(func() { _ = st.Remove(context.Background(), key) })

	if err := st.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "image/webp"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	size, contentType, err := st.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if size != int64(len(body)) || contentType != "image/webp" {
		t.Fatalf("Stat = %d/%q", size, contentType)
	}
	reader, _, err := st.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("Get body = %d bytes, err=%v", len(got), err)
	}
	if err := st.Remove(ctx, key); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, _, err := st.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat after Remove error = %v, want ErrNotFound", err)
	}
}
