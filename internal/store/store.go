package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	turso "turso.tech/database/tursogo"
)

// Options configures how the local database is opened.
type Options struct {
	Path        string
	LocalOnly   bool
	RemoteURL   string
	AuthToken   string
	BusyTimeout time.Duration
	Logger      *slog.Logger
}

// Store owns the local Turso database. Local SQLite-compatible writes are the
// only write path; the optional sync worker pushes them to Turso Cloud.
type Store struct {
	db       *sql.DB
	syncDB   *turso.TursoSyncDb
	log      *slog.Logger
	path     string
	onChange func()

	changeMu sync.RWMutex
}

// Open creates the data directory if needed and opens the local Turso database.
// In sync mode it bootstraps from Turso Cloud when the local file is empty.
func Open(ctx context.Context, opts Options) (*Store, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, fmt.Errorf("store: empty database path")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	busy := opts.BusyTimeout
	if busy <= 0 {
		busy = 5 * time.Second
	}

	if opts.Path != ":memory:" {
		dir := filepath.Dir(opts.Path)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("store: create data directory: %w", err)
		}
	}

	var (
		db     *sql.DB
		syncDB *turso.TursoSyncDb
		err    error
	)

	if opts.LocalOnly || opts.RemoteURL == "" {
		if !opts.LocalOnly {
			return nil, fmt.Errorf("store: sync requested but no remote URL was provided")
		}
		dsn := opts.Path
		if strings.Contains(dsn, "?") {
			dsn += fmt.Sprintf("&_busy_timeout=%d", busy.Milliseconds())
		} else {
			dsn += fmt.Sprintf("?_busy_timeout=%d", busy.Milliseconds())
		}
		db, err = sql.Open("turso", dsn)
		if err != nil {
			return nil, fmt.Errorf("store: open local database: %w", err)
		}
	} else {
		syncDB, err = turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
			Path:              opts.Path,
			RemoteUrl:         opts.RemoteURL,
			AuthToken:         opts.AuthToken,
			ClientName:        "nurtur",
			BusyTimeout:       int(busy.Milliseconds()),
			LongPollTimeoutMs: 5000,
		})
		if err != nil {
			return nil, fmt.Errorf("store: open synced database: %w", err)
		}
		db, err = syncDB.Connect(ctx)
		if err != nil {
			return nil, fmt.Errorf("store: connect synced database: %w", err)
		}
	}

	// One connection serialises writers inside this single-instance app and
	// makes read-then-write transitions deterministic. The partial unique
	// indexes remain the database-level backstop.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(0)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping database: %w", err)
	}

	s := &Store{db: db, syncDB: syncDB, log: opts.Logger, path: opts.Path}
	return s, nil
}

// SetOnChange registers a callback fired after every successful write
// transaction. The sync worker uses it to schedule a debounced push.
func (s *Store) SetOnChange(fn func()) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.onChange = fn
}

func (s *Store) notifyChange() {
	s.changeMu.RLock()
	fn := s.onChange
	s.changeMu.RUnlock()
	if fn != nil {
		fn()
	}
}

// SyncDB returns the synced database handle, or nil in local-only mode.
func (s *Store) SyncDB() *turso.TursoSyncDb { return s.syncDB }

// Path is the local database file path.
func (s *Store) Path() string { return s.path }

// Ping checks the local database.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Close closes the database. Flush and checkpoint the sync worker first.
func (s *Store) Close() error { return s.db.Close() }

// WithTx runs fn in a database/sql transaction. fn must use the supplied *sql.Tx
// for every statement; the store holds a single connection.
func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	s.notifyChange()
	return nil
}

type accountContextKey struct{}

// WithAccount scopes store reads and writes to one account. Installs without an
// account system use account 0, which is also the legacy namespace that the
// first account claims on creation.
func WithAccount(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, accountContextKey{}, id)
}

// AccountID returns the account bound to ctx, or 0 when none is set.
func AccountID(ctx context.Context) int64 {
	id, _ := ctx.Value(accountContextKey{}).(int64)
	return id
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type scanner interface {
	Scan(dest ...any) error
}

func encodeTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func normalizeTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

func decodeTime(raw string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse timestamp %q: %w", raw, err)
	}
	return t.UTC(), nil
}

func decodeNullTime(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	t, err := decodeTime(raw.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

// Migrate applies embedded SQL files in name order. It is idempotent: applied
// versions are recorded in schema_migrations and skipped on later starts.
func (s *Store) Migrate(ctx context.Context, fsys fs.FS, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var applied int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`,
			entry.Name(),
		).Scan(&applied); err != nil {
			return fmt.Errorf("store: check migration %s: %w", entry.Name(), err)
		}
		if applied > 0 {
			continue
		}

		body, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return fmt.Errorf("store: read migration %s: %w", entry.Name(), err)
		}
		statements := splitSQL(string(body))
		if len(statements) == 0 {
			return fmt.Errorf("store: migration %s has no statements", entry.Name())
		}

		err = s.WithTx(ctx, func(tx *sql.Tx) error {
			for _, stmt := range statements {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("apply %s: %w", entry.Name(), err)
				}
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
				entry.Name(), encodeTime(now))
			return err
		})
		if err != nil {
			return fmt.Errorf("store: migrate %s: %w", entry.Name(), err)
		}
		s.log.Info("migration applied", "version", entry.Name())
	}
	return s.repairSettingsSchema(ctx)
}

// repairSettingsSchema fixes databases where migration 0004 was recorded but the
// Turso sync engine did not apply its final table rewrite. Those databases have
// settings_scoped (often with the rows) but no settings table, so every signup
// and settings request fails with "no such table: settings". The repair only
// uses CREATE, INSERT, and DROP, which the sync engine replicates correctly.
func (s *Store) repairSettingsSchema(ctx context.Context) error {
	settingsExists, hasAccountID, err := s.tableShape(ctx, `PRAGMA table_info(settings)`)
	if err != nil {
		return fmt.Errorf("store: inspect settings: %w", err)
	}
	scopedExists, _, err := s.tableShape(ctx, `PRAGMA table_info(settings_scoped)`)
	if err != nil {
		return fmt.Errorf("store: inspect settings_scoped: %w", err)
	}

	if settingsExists && hasAccountID {
		if !scopedExists {
			return nil
		}
		// A normal database has no settings_scoped left. If one is still here,
		// keep any rows it holds and remove the stray table.
		s.log.Warn("repairing stray settings_scoped table")
		return s.WithTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO settings (account_id, key, value)
				 SELECT account_id, key, value FROM settings_scoped`); err != nil {
				return fmt.Errorf("store: merge settings_scoped: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DROP TABLE settings_scoped`); err != nil {
				return fmt.Errorf("store: drop stray settings_scoped: %w", err)
			}
			return nil
		})
	}

	s.log.Warn("repairing settings table after incomplete migration",
		"settings_exists", settingsExists, "settings_scoped_exists", scopedExists)

	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if !scopedExists {
			if settingsExists {
				// The pre-account table survived without its account_id column.
				// Move it into the scoped shape first.
				if _, err := tx.ExecContext(ctx, settingsScopedDDL); err != nil {
					return fmt.Errorf("store: create settings_scoped: %w", err)
				}
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO settings_scoped (account_id, key, value)
					 SELECT COALESCE((SELECT MIN(id) FROM accounts), 0), key, value FROM settings`); err != nil {
					return fmt.Errorf("store: copy legacy settings: %w", err)
				}
				if _, err := tx.ExecContext(ctx, `DROP TABLE settings`); err != nil {
					return fmt.Errorf("store: drop legacy settings: %w", err)
				}
			} else {
				// Both tables are gone; create the expected schema so reads and
				// writes stop failing. There is no data left to recover.
				if _, err := tx.ExecContext(ctx, settingsDDL); err != nil {
					return fmt.Errorf("store: create settings: %w", err)
				}
				return nil
			}
		} else if settingsExists {
			// An unscoped settings table is still around next to the scoped one.
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO settings_scoped (account_id, key, value)
				 SELECT COALESCE((SELECT MIN(id) FROM accounts), 0), key, value FROM settings`); err != nil {
				return fmt.Errorf("store: merge legacy settings: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DROP TABLE settings`); err != nil {
				return fmt.Errorf("store: drop legacy settings: %w", err)
			}
		}

		if _, err := tx.ExecContext(ctx, settingsDDL); err != nil {
			return fmt.Errorf("store: create settings: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (account_id, key, value)
			 SELECT account_id, key, value FROM settings_scoped`); err != nil {
			return fmt.Errorf("store: copy settings_scoped: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE settings_scoped`); err != nil {
			return fmt.Errorf("store: drop settings_scoped: %w", err)
		}
		return nil
	})
}

const settingsDDL = `CREATE TABLE settings (
	account_id INTEGER NOT NULL DEFAULT 0,
	key        TEXT NOT NULL,
	value      TEXT NOT NULL,
	PRIMARY KEY (account_id, key)
)`

const settingsScopedDDL = `CREATE TABLE settings_scoped (
	account_id INTEGER NOT NULL DEFAULT 0,
	key        TEXT NOT NULL,
	value      TEXT NOT NULL,
	PRIMARY KEY (account_id, key)
)`

// tableShape reports whether the table named by the fixed PRAGMA query exists
// and whether it has an account_id column. Callers pass constant queries; no
// user input reaches the SQL string.
func (s *Store) tableShape(ctx context.Context, query string) (exists, hasAccountID bool, err error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return false, false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid, notNull, pk int
			name, typ        string
			defaultValue     sql.NullString
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, false, err
		}
		exists = true
		if name == "account_id" {
			hasAccountID = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, false, err
	}
	return exists, hasAccountID, nil
}

// splitSQL splits a SQLite script on semicolons outside strings and comments.
// tursogo accepts multi-statement Exec, but executing each statement separately
// produces far better errors when a migration fails.
func splitSQL(script string) []string {
	var (
		statements []string
		current    strings.Builder
		inString   bool
		hasContent bool
	)
	flush := func() {
		if hasContent {
			statements = append(statements, strings.TrimSpace(current.String()))
		}
		current.Reset()
		hasContent = false
	}

	for i := 0; i < len(script); i++ {
		c := script[i]
		if inString {
			current.WriteByte(c)
			if c == '\'' {
				if i+1 < len(script) && script[i+1] == '\'' {
					current.WriteByte(script[i+1])
					i++
					continue
				}
				inString = false
			}
			continue
		}
		switch {
		case c == '\'':
			inString = true
			hasContent = true
			current.WriteByte(c)
		case c == '-' && i+1 < len(script) && script[i+1] == '-':
			for i < len(script) && script[i] != '\n' {
				i++
			}
			current.WriteByte('\n')
		case c == '/' && i+1 < len(script) && script[i+1] == '*':
			i += 2
			for i+1 < len(script) && !(script[i] == '*' && script[i+1] == '/') {
				i++
			}
			i++
			current.WriteByte(' ')
		case c == ';':
			flush()
		default:
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				hasContent = true
			}
			current.WriteByte(c)
		}
	}
	flush()
	return statements
}
