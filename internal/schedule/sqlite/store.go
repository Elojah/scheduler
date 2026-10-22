// Package sqlite holds a schedule.Store on a SQLite database of package pkg/sqlite.
package sqlite

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/elojah/scheduler/internal/schedule"
	"github.com/elojah/scheduler/pkg/sqlite"
)

var _ schedule.Store = (*Store)(nil)

//go:embed schema.sql
var schema string

// Store is a schedule.Store on Service's database. Init it before any other call.
type Store struct {
	sqlite.Service
}

// Init creates the store's table, if need be.
func (s *Store) Init(ctx context.Context) error {
	if _, err := s.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("sqlite: schema: %w", err)
	}

	return nil
}
