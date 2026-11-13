package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nurtur/internal/intake"
)

const (
	bottleColumns = `id, account_id, logged_at, milk_type, amount_ml, COALESCE(notes, ''), created_at, deleted_at`
	pumpColumns   = `id, account_id, logged_at, side, amount_ml, duration_minutes, COALESCE(notes, ''), created_at, deleted_at`
	solidColumns  = `id, account_id, logged_at, food, portion, reaction, COALESCE(notes, ''), created_at, deleted_at`
)

// CreateBottle logs a bottle feed. If quickTap is true, a matching milk_type +
// amount_ml within intake.DuplicateWindow is treated as a double tap.
func (s *Store) CreateBottle(ctx context.Context, milk intake.MilkType, amountML int, loggedAt, now time.Time, notes string, quickTap bool) (intake.Bottle, bool, error) {
	loggedAt = normalizeTime(loggedAt)
	now = normalizeTime(now)
	if err := intake.ValidateBottle(milk, amountML, loggedAt, now, notes); err != nil {
		return intake.Bottle{}, false, err
	}

	var (
		out     intake.Bottle
		created bool
	)
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if quickTap {
			cutoff := encodeTime(loggedAt.Add(-intake.DuplicateWindow))
			row := tx.QueryRowContext(ctx, `
				SELECT `+bottleColumns+`
				FROM bottles
				WHERE account_id = ? AND deleted_at IS NULL AND milk_type = ? AND amount_ml = ? AND logged_at >= ?
				ORDER BY logged_at DESC, id DESC
				LIMIT 1`, AccountID(ctx), milk, amountML, cutoff)
			existing, err := scanBottle(row)
			if err == nil {
				out = existing
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("store: check duplicate bottle: %w", err)
			}
		}

		res, err := tx.ExecContext(ctx, `
			INSERT INTO bottles (account_id, logged_at, milk_type, amount_ml, notes, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			AccountID(ctx), encodeTime(loggedAt), milk, amountML, nullableString(notes), encodeTime(now))
		if err != nil {
			return fmt.Errorf("store: insert bottle: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: bottle id: %w", err)
		}
		out = intake.Bottle{
			ID:        id,
			AccountID: AccountID(ctx),
			LoggedAt:  loggedAt,
			MilkType:  milk,
			AmountML:  amountML,
			Notes:     notes,
			CreatedAt: now,
		}
		created = true
		return nil
	})
	return out, created, err
}

func (s *Store) GetBottle(ctx context.Context, id int64) (intake.Bottle, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+bottleColumns+` FROM bottles WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		id, AccountID(ctx))
	b, err := scanBottle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return intake.Bottle{}, intake.ErrNotFound
	}
	return b, err
}

func (s *Store) UpdateBottle(ctx context.Context, id int64, milk intake.MilkType, amountML int, loggedAt, now time.Time, notes string) (intake.Bottle, error) {
	loggedAt = normalizeTime(loggedAt)
	now = normalizeTime(now)
	if err := intake.ValidateBottle(milk, amountML, loggedAt, now, notes); err != nil {
		return intake.Bottle{}, err
	}

	var out intake.Bottle
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE bottles SET milk_type = ?, amount_ml = ?, logged_at = ?, notes = ?
			WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			milk, amountML, encodeTime(loggedAt), nullableString(notes), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: update bottle: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return intake.ErrNotFound
		}
		row := tx.QueryRowContext(ctx, `SELECT `+bottleColumns+` FROM bottles WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		out, err = scanBottle(row)
		return err
	})
	return out, err
}

func (s *Store) DeleteBottle(ctx context.Context, id int64, now time.Time) (intake.Bottle, error) {
	now = normalizeTime(now)
	var out intake.Bottle
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT `+bottleColumns+` FROM bottles WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		b, err := scanBottle(row)
		if errors.Is(err, sql.ErrNoRows) {
			return intake.ErrNotFound
		}
		if err != nil {
			return err
		}
		if b.DeletedAt != nil {
			return intake.ErrAlreadyDeleted
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bottles SET deleted_at = ? WHERE id = ? AND account_id = ?`, encodeTime(now), id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: delete bottle: %w", err)
		}
		b.DeletedAt = &now
		out = b
		return nil
	})
	return out, err
}

func (s *Store) RestoreBottle(ctx context.Context, id int64) (intake.Bottle, error) {
	var out intake.Bottle
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE bottles SET deleted_at = NULL WHERE id = ? AND account_id = ? AND deleted_at IS NOT NULL`,
			id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: restore bottle: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return intake.ErrNotFound
		}
		row := tx.QueryRowContext(ctx, `SELECT `+bottleColumns+` FROM bottles WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		out, err = scanBottle(row)
		return err
	})
	return out, err
}

func (s *Store) bottlesByIDs(ctx context.Context, ids []int64) (map[int64]*intake.Bottle, error) {
	out := make(map[int64]*intake.Bottle, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := idPlaceholders(AccountID(ctx), ids)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+bottleColumns+` FROM bottles
		WHERE account_id = ? AND deleted_at IS NULL AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: bottles by ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		b, err := scanBottle(rows)
		if err != nil {
			return nil, err
		}
		copyB := b
		out[b.ID] = &copyB
	}
	return out, rows.Err()
}

func (s *Store) AllBottles(ctx context.Context) ([]intake.Bottle, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+bottleColumns+` FROM bottles WHERE account_id = ? ORDER BY id ASC`, AccountID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: export bottles: %w", err)
	}
	defer rows.Close()
	var out []intake.Bottle
	for rows.Next() {
		b, err := scanBottle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func scanBottle(sc scanner) (intake.Bottle, error) {
	var (
		b         intake.Bottle
		loggedRaw string
		milkRaw   string
		created   string
		deleted   sql.NullString
	)
	if err := sc.Scan(&b.ID, &b.AccountID, &loggedRaw, &milkRaw, &b.AmountML, &b.Notes, &created, &deleted); err != nil {
		return intake.Bottle{}, err
	}
	var err error
	if b.LoggedAt, err = decodeTime(loggedRaw); err != nil {
		return intake.Bottle{}, err
	}
	b.MilkType = intake.MilkType(milkRaw)
	if b.CreatedAt, err = decodeTime(created); err != nil {
		return intake.Bottle{}, err
	}
	if b.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return intake.Bottle{}, err
	}
	return b, nil
}

// CreatePump logs a pumping session.
func (s *Store) CreatePump(ctx context.Context, side intake.Side, amountML, durationMin int, loggedAt, now time.Time, notes string) (intake.Pump, error) {
	loggedAt = normalizeTime(loggedAt)
	now = normalizeTime(now)
	if err := intake.ValidatePump(side, amountML, durationMin, loggedAt, now, notes); err != nil {
		return intake.Pump{}, err
	}

	var out intake.Pump
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO pumps (account_id, logged_at, side, amount_ml, duration_minutes, notes, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			AccountID(ctx), encodeTime(loggedAt), side, amountML, durationMin, nullableString(notes), encodeTime(now))
		if err != nil {
			return fmt.Errorf("store: insert pump: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: pump id: %w", err)
		}
		out = intake.Pump{
			ID:              id,
			AccountID:       AccountID(ctx),
			LoggedAt:        loggedAt,
			Side:            side,
			AmountML:        amountML,
			DurationMinutes: durationMin,
			Notes:           notes,
			CreatedAt:       now,
		}
		return nil
	})
	return out, err
}

func (s *Store) GetPump(ctx context.Context, id int64) (intake.Pump, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+pumpColumns+` FROM pumps WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		id, AccountID(ctx))
	p, err := scanPump(row)
	if errors.Is(err, sql.ErrNoRows) {
		return intake.Pump{}, intake.ErrNotFound
	}
	return p, err
}

func (s *Store) UpdatePump(ctx context.Context, id int64, side intake.Side, amountML, durationMin int, loggedAt, now time.Time, notes string) (intake.Pump, error) {
	loggedAt = normalizeTime(loggedAt)
	now = normalizeTime(now)
	if err := intake.ValidatePump(side, amountML, durationMin, loggedAt, now, notes); err != nil {
		return intake.Pump{}, err
	}

	var out intake.Pump
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE pumps SET side = ?, amount_ml = ?, duration_minutes = ?, logged_at = ?, notes = ?
			WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			side, amountML, durationMin, encodeTime(loggedAt), nullableString(notes), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: update pump: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return intake.ErrNotFound
		}
		row := tx.QueryRowContext(ctx, `SELECT `+pumpColumns+` FROM pumps WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		out, err = scanPump(row)
		return err
	})
	return out, err
}

func (s *Store) DeletePump(ctx context.Context, id int64, now time.Time) (intake.Pump, error) {
	now = normalizeTime(now)
	var out intake.Pump
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT `+pumpColumns+` FROM pumps WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		p, err := scanPump(row)
		if errors.Is(err, sql.ErrNoRows) {
			return intake.ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.DeletedAt != nil {
			return intake.ErrAlreadyDeleted
		}
		if _, err := tx.ExecContext(ctx, `UPDATE pumps SET deleted_at = ? WHERE id = ? AND account_id = ?`, encodeTime(now), id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: delete pump: %w", err)
		}
		p.DeletedAt = &now
		out = p
		return nil
	})
	return out, err
}

func (s *Store) RestorePump(ctx context.Context, id int64) (intake.Pump, error) {
	var out intake.Pump
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE pumps SET deleted_at = NULL WHERE id = ? AND account_id = ? AND deleted_at IS NOT NULL`,
			id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: restore pump: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return intake.ErrNotFound
		}
		row := tx.QueryRowContext(ctx, `SELECT `+pumpColumns+` FROM pumps WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		out, err = scanPump(row)
		return err
	})
	return out, err
}

func (s *Store) pumpsByIDs(ctx context.Context, ids []int64) (map[int64]*intake.Pump, error) {
	out := make(map[int64]*intake.Pump, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := idPlaceholders(AccountID(ctx), ids)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+pumpColumns+` FROM pumps
		WHERE account_id = ? AND deleted_at IS NULL AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: pumps by ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanPump(rows)
		if err != nil {
			return nil, err
		}
		copyP := p
		out[p.ID] = &copyP
	}
	return out, rows.Err()
}

func (s *Store) AllPumps(ctx context.Context) ([]intake.Pump, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pumpColumns+` FROM pumps WHERE account_id = ? ORDER BY id ASC`, AccountID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: export pumps: %w", err)
	}
	defer rows.Close()
	var out []intake.Pump
	for rows.Next() {
		p, err := scanPump(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPump(sc scanner) (intake.Pump, error) {
	var (
		p         intake.Pump
		loggedRaw string
		sideRaw   string
		created   string
		deleted   sql.NullString
	)
	if err := sc.Scan(&p.ID, &p.AccountID, &loggedRaw, &sideRaw, &p.AmountML, &p.DurationMinutes, &p.Notes, &created, &deleted); err != nil {
		return intake.Pump{}, err
	}
	var err error
	if p.LoggedAt, err = decodeTime(loggedRaw); err != nil {
		return intake.Pump{}, err
	}
	p.Side = intake.Side(sideRaw)
	if p.CreatedAt, err = decodeTime(created); err != nil {
		return intake.Pump{}, err
	}
	if p.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return intake.Pump{}, err
	}
	return p, nil
}

// CreateSolid logs a solid meal.
func (s *Store) CreateSolid(ctx context.Context, food string, portion intake.Portion, reaction intake.Reaction, loggedAt, now time.Time, notes string) (intake.Solid, error) {
	food = strings.TrimSpace(food)
	loggedAt = normalizeTime(loggedAt)
	now = normalizeTime(now)
	if err := intake.ValidateSolid(food, portion, reaction, loggedAt, now, notes); err != nil {
		return intake.Solid{}, err
	}

	var out intake.Solid
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO solids (account_id, logged_at, food, portion, reaction, notes, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			AccountID(ctx), encodeTime(loggedAt), food, portion, reaction, nullableString(notes), encodeTime(now))
		if err != nil {
			return fmt.Errorf("store: insert solid: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: solid id: %w", err)
		}
		out = intake.Solid{
			ID:        id,
			AccountID: AccountID(ctx),
			LoggedAt:  loggedAt,
			Food:      food,
			Portion:   portion,
			Reaction:  reaction,
			Notes:     notes,
			CreatedAt: now,
		}
		return nil
	})
	return out, err
}

func (s *Store) GetSolid(ctx context.Context, id int64) (intake.Solid, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+solidColumns+` FROM solids WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
		id, AccountID(ctx))
	sol, err := scanSolid(row)
	if errors.Is(err, sql.ErrNoRows) {
		return intake.Solid{}, intake.ErrNotFound
	}
	return sol, err
}

func (s *Store) UpdateSolid(ctx context.Context, id int64, food string, portion intake.Portion, reaction intake.Reaction, loggedAt, now time.Time, notes string) (intake.Solid, error) {
	food = strings.TrimSpace(food)
	loggedAt = normalizeTime(loggedAt)
	now = normalizeTime(now)
	if err := intake.ValidateSolid(food, portion, reaction, loggedAt, now, notes); err != nil {
		return intake.Solid{}, err
	}

	var out intake.Solid
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE solids SET food = ?, portion = ?, reaction = ?, logged_at = ?, notes = ?
			WHERE id = ? AND account_id = ? AND deleted_at IS NULL`,
			food, portion, reaction, encodeTime(loggedAt), nullableString(notes), id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: update solid: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return intake.ErrNotFound
		}
		row := tx.QueryRowContext(ctx, `SELECT `+solidColumns+` FROM solids WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		out, err = scanSolid(row)
		return err
	})
	return out, err
}

func (s *Store) DeleteSolid(ctx context.Context, id int64, now time.Time) (intake.Solid, error) {
	now = normalizeTime(now)
	var out intake.Solid
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT `+solidColumns+` FROM solids WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		sol, err := scanSolid(row)
		if errors.Is(err, sql.ErrNoRows) {
			return intake.ErrNotFound
		}
		if err != nil {
			return err
		}
		if sol.DeletedAt != nil {
			return intake.ErrAlreadyDeleted
		}
		if _, err := tx.ExecContext(ctx, `UPDATE solids SET deleted_at = ? WHERE id = ? AND account_id = ?`, encodeTime(now), id, AccountID(ctx)); err != nil {
			return fmt.Errorf("store: delete solid: %w", err)
		}
		sol.DeletedAt = &now
		out = sol
		return nil
	})
	return out, err
}

func (s *Store) RestoreSolid(ctx context.Context, id int64) (intake.Solid, error) {
	var out intake.Solid
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE solids SET deleted_at = NULL WHERE id = ? AND account_id = ? AND deleted_at IS NOT NULL`,
			id, AccountID(ctx))
		if err != nil {
			return fmt.Errorf("store: restore solid: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return intake.ErrNotFound
		}
		row := tx.QueryRowContext(ctx, `SELECT `+solidColumns+` FROM solids WHERE id = ? AND account_id = ?`, id, AccountID(ctx))
		out, err = scanSolid(row)
		return err
	})
	return out, err
}

func (s *Store) solidsByIDs(ctx context.Context, ids []int64) (map[int64]*intake.Solid, error) {
	out := make(map[int64]*intake.Solid, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders, args := idPlaceholders(AccountID(ctx), ids)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+solidColumns+` FROM solids
		WHERE account_id = ? AND deleted_at IS NULL AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: solids by ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		sol, err := scanSolid(rows)
		if err != nil {
			return nil, err
		}
		copyS := sol
		out[sol.ID] = &copyS
	}
	return out, rows.Err()
}

func (s *Store) AllSolids(ctx context.Context) ([]intake.Solid, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+solidColumns+` FROM solids WHERE account_id = ? ORDER BY id ASC`, AccountID(ctx))
	if err != nil {
		return nil, fmt.Errorf("store: export solids: %w", err)
	}
	defer rows.Close()
	var out []intake.Solid
	for rows.Next() {
		sol, err := scanSolid(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sol)
	}
	return out, rows.Err()
}

func scanSolid(sc scanner) (intake.Solid, error) {
	var (
		sol       intake.Solid
		loggedRaw string
		porRaw    string
		reactRaw  string
		created   string
		deleted   sql.NullString
	)
	if err := sc.Scan(&sol.ID, &sol.AccountID, &loggedRaw, &sol.Food, &porRaw, &reactRaw, &sol.Notes, &created, &deleted); err != nil {
		return intake.Solid{}, err
	}
	var err error
	if sol.LoggedAt, err = decodeTime(loggedRaw); err != nil {
		return intake.Solid{}, err
	}
	sol.Portion = intake.Portion(porRaw)
	sol.Reaction = intake.Reaction(reactRaw)
	if sol.CreatedAt, err = decodeTime(created); err != nil {
		return intake.Solid{}, err
	}
	if sol.DeletedAt, err = decodeNullTime(deleted); err != nil {
		return intake.Solid{}, err
	}
	return sol, nil
}

func idPlaceholders(accountID int64, ids []int64) (string, []any) {
	var b strings.Builder
	args := make([]any, 0, len(ids)+1)
	args = append(args, accountID)
	for i, id := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args = append(args, id)
	}
	return b.String(), args
}
