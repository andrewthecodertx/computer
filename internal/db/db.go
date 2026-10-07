package db

import "github.com/andrew/go-computer/internal/config"

// DB is the storage interface. All data access goes through it so the port can
// swap Postgres (GORM, sqlc, pgx) for another relational DB without touching
// handlers. Every query is scoped to the effective user; never trust a user ID
// from the client (porting guide 5.1).
//
// TODO: implement against the real database. Keep table and column names so the
// existing `linkos` database can be reused.

type DB struct {
	// TODO: hold the underlying connection (e.g. *sql.DB or a GORM dialector).
}

// New connects to the database using the given config.
func New(cfg *config.Config) (*DB, error) {
	// TODO: open, ping, run migrations.
	return &DB{}, nil
}

// Close closes the connection pool.
func (d *DB) Close() error {
	return nil
}