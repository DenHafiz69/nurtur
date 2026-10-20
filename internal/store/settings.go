package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// Settings returns every stored key/value pair. The settings table holds
// user-editable preferences that are not environment configuration.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings ORDER BY key ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list settings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("store: scan setting: %w", err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list settings: %w", err)
	}
	return out, nil
}

// Setting returns one value and whether it exists.
func (s *Store) Setting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get setting %s: %w", key, err)
	}
	return value, true, nil
}

// SetSettings upserts a batch of values in one transaction.
func (s *Store) SetSettings(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, key := range keys {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO settings (key, value) VALUES (?, ?)
				 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
				key, values[key]); err != nil {
				return fmt.Errorf("store: set setting %s: %w", key, err)
			}
		}
		return nil
	})
}
