// Package sqlite opens a SQLite database through modernc.org/sqlite: pure Go, so that a service builds without cgo, for
// any target Go has.
package sqlite

import (
	"context"
	"database/sql"
	"errors"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Service embeds a SQLite database, in WAL mode, on one connection.
type Service struct {
	*sql.DB
}

// Dial opens the database at cfg.Path, creating it if need be.
func (s *Service) Dial(ctx context.Context, cfg Config) error {
	db, err := sql.Open("sqlite", "file:"+cfg.Path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	// One writer: SQLite has one anyway.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return errors.Join(err, db.Close())
	}
	s.DB = db

	return nil
}

// Close closes the database, if dialed.
func (s *Service) Close(context.Context) error {
	if s.DB == nil {
		return nil
	}

	return s.DB.Close()
}
