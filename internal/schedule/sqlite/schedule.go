package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/elojah/scheduler/internal/schedule"
)

// Create inserts sc.
func (s *Store) Create(ctx context.Context, sc schedule.Schedule) error {
	options, err := json.Marshal(sc.Options)
	if err != nil {
		return err
	}
	// options as a string: a []byte would be a BLOB, which the STRICT TEXT column refuses.
	_, err = s.ExecContext(ctx, `INSERT INTO schedules (id, rule, operation, options, enabled) VALUES (?, ?, ?, ?, ?)`,
		sc.ID, sc.Rule, sc.Operation, string(options), sc.Enabled)

	return err
}

// Get returns the schedule of id.
func (s *Store) Get(ctx context.Context, id uuid.UUID) (schedule.Schedule, error) {
	row := s.QueryRowContext(ctx, `SELECT id, rule, operation, options, enabled FROM schedules WHERE id = ?`, id)
	sc, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, schedule.NotFoundError{ID: id}
	}

	return sc, err
}

// List returns every schedule, by ID, which as a UUIDv7 is their order of creation.
func (s *Store) List(ctx context.Context) ([]schedule.Schedule, error) {
	rows, err := s.QueryContext(ctx, `SELECT id, rule, operation, options, enabled FROM schedules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	// rows.Err reports what Close would.
	defer func() { _ = rows.Close() }()

	var schedules []schedule.Schedule
	for rows.Next() {
		sc, err := scan(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, sc)
	}

	return schedules, rows.Err()
}

// Update replaces the schedule of sc.ID with sc.
func (s *Store) Update(ctx context.Context, sc schedule.Schedule) error {
	options, err := json.Marshal(sc.Options)
	if err != nil {
		return err
	}
	res, err := s.ExecContext(ctx, `UPDATE schedules SET rule = ?, operation = ?, options = ?, enabled = ? WHERE id = ?`,
		sc.Rule, sc.Operation, string(options), sc.Enabled, sc.ID)
	if err != nil {
		return err
	}

	return found(res, sc.ID)
}

// Delete removes the schedule of id.
func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := s.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}

	return found(res, id)
}

// scan reads a schedule from a row of id, rule, operation, options and enabled.
func scan(row interface{ Scan(dest ...any) error }) (schedule.Schedule, error) {
	var (
		sc      schedule.Schedule
		options []byte
	)
	if err := row.Scan(&sc.ID, &sc.Rule, &sc.Operation, &options, &sc.Enabled); err != nil {
		return schedule.Schedule{}, err
	}
	if err := json.Unmarshal(options, &sc.Options); err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: options of %s: %w", sc.ID, err)
	}

	return sc, nil
}

// found returns a schedule.NotFoundError for id if res affected no row.
func found(res sql.Result, id uuid.UUID) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return schedule.NotFoundError{ID: id}
	}

	return nil
}
