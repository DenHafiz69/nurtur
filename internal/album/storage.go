package album

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage is the seam between photo metadata and photo bytes. The local driver
// keeps development hermetic; the R2 driver is used in production. Neither
// driver is ever exposed directly to a browser.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Remove(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (size int64, contentType string, err error)
}

// StorageConfig is the resolved configuration for NewStorage.
type StorageConfig struct {
	Driver string
	Dir    string
	R2     R2Config
	Logger *slog.Logger
}

// R2Config is the Cloudflare R2 credentials and endpoint. The bucket stays
// private; Nurtur is the only reader.
type R2Config struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// Complete reports whether every required R2 value is present.
func (c R2Config) Complete() bool {
	return strings.TrimSpace(c.Endpoint) != "" &&
		strings.TrimSpace(c.Bucket) != "" &&
		strings.TrimSpace(c.AccessKeyID) != "" &&
		strings.TrimSpace(c.SecretAccessKey) != ""
}

// NewStorage resolves the driver and opens the local directory. "auto" uses R2
// only when every required value is present; otherwise it falls back to local
// and logs one warning at startup.
func NewStorage(cfg StorageConfig) (Storage, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = "auto"
	}

	switch driver {
	case "auto":
		if cfg.R2.Complete() {
			return newR2Storage(cfg.R2)
		}
		cfg.Logger.Warn("album storage: R2 is not fully configured, using the local driver",
			"dir", cfg.Dir)
		driver = "local"
	case "local", "r2":
	default:
		return nil, fmt.Errorf("album: NURTUR_ALBUM_STORAGE must be auto, local, or r2")
	}

	if driver == "r2" {
		if !cfg.R2.Complete() {
			return nil, fmt.Errorf("album: R2 storage needs an endpoint, bucket, access key, and secret")
		}
		return newR2Storage(cfg.R2)
	}
	return newLocalStorage(cfg.Dir)
}

/* ---------- Local ---------- */

type localStorage struct {
	root string
}

func newLocalStorage(dir string) (*localStorage, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("album: local storage directory is empty")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("album: resolve storage directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("album: create storage directory: %w", err)
	}
	return &localStorage{root: root}, nil
}

func (l *localStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if size < 0 {
		return fmt.Errorf("album: negative object size")
	}
	full, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("album: create object directory: %w", err)
	}

	// Write beside the destination and rename, so an interrupted upload never
	// leaves a half object visible under the final key.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return fmt.Errorf("album: create object: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := io.Copy(tmp, r); err != nil {
		cleanup()
		return fmt.Errorf("album: write object: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("album: close object: %w", err)
	}
	if err := os.Rename(tmpName, full); err != nil {
		cleanup()
		return fmt.Errorf("album: publish object: %w", err)
	}
	return nil
}

func (l *localStorage) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	full, err := l.path(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("album: open object: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("album: stat object: %w", err)
	}
	return f, info.Size(), nil
}

func (l *localStorage) Remove(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	full, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("album: remove object: %w", err)
	}
	return nil
}

func (l *localStorage) Stat(ctx context.Context, key string) (int64, string, error) {
	if err := ctx.Err(); err != nil {
		return 0, "", err
	}
	full, err := l.path(key)
	if err != nil {
		return 0, "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("album: stat object: %w", err)
	}
	return info.Size(), contentTypeForKey(key), nil
}

// path resolves an object key below the storage root. Keys are generated by
// Nurtur, but this check makes a malformed one harmless even if a future
// caller passes it through.
func (l *localStorage) path(key string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(strings.TrimSpace(key), "\\", "/"))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("album: invalid object key")
	}
	full := filepath.Join(l.root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(l.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("album: object key escapes the storage root")
	}
	return full, nil
}

func contentTypeForKey(key string) string {
	switch strings.ToLower(path.Ext(key)) {
	case ".webp":
		return "image/webp"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		if value := mime.TypeByExtension(path.Ext(key)); value != "" {
			return value
		}
		return "application/octet-stream"
	}
}

/* ---------- R2 ---------- */

type r2Storage struct {
	client *minio.Client
	bucket string
}

func newR2Storage(cfg R2Config) (*r2Storage, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("album: NURTUR_R2_ENDPOINT must be an absolute URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("album: NURTUR_R2_ENDPOINT must start with https:// or http://")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("album: NURTUR_R2_ENDPOINT must not include a path")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "auto"
	}
	client, err := minio.New(parsed.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(strings.TrimSpace(cfg.AccessKeyID), strings.TrimSpace(cfg.SecretAccessKey), ""),
		Secure: parsed.Scheme == "https",
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("album: create R2 client: %w", err)
	}
	return &r2Storage{client: client, bucket: strings.TrimSpace(cfg.Bucket)}, nil
}

func (r *r2Storage) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if _, err := r.client.PutObject(ctx, r.bucket, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("album: put object: %w", err)
	}
	return nil
}

func (r *r2Storage) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := r.client.GetObject(ctx, r.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("album: get object: %w", err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if isR2NotFound(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("album: stat object: %w", err)
	}
	return obj, info.Size, nil
}

func (r *r2Storage) Remove(ctx context.Context, key string) error {
	if err := r.client.RemoveObject(ctx, r.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("album: remove object: %w", err)
	}
	return nil
}

func (r *r2Storage) Stat(ctx context.Context, key string) (int64, string, error) {
	info, err := r.client.StatObject(ctx, r.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isR2NotFound(err) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("album: stat object: %w", err)
	}
	return info.Size, info.ContentType, nil
}

func isR2NotFound(err error) bool {
	var response minio.ErrorResponse
	if errors.As(err, &response) {
		return response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NotFound"
	}
	return false
}
