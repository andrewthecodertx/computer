package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/andrew/go-computer/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// DB is the storage interface. All data access goes through it so the port can
// swap Postgres for another relational DB without touching handlers. Every
// query is scoped to the effective user; never trust a user ID from the client
// (porting guide 5.1).
type DB struct {
	*sql.DB
}

// New connects to the database using the given config and runs a readiness ping.
// Connection string: config.DBURL() (DATABASE_URL or DB_URL), e.g.
// postgres://computer:computer@db:5432/computer?sslmode=disable
func New(cfg *config.Config) (*DB, error) {
	dsn := config.DBURL()
	if dsn == "" {
		return nil, fmt.Errorf("no database url set (DATABASE_URL / DB_URL)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	pingCtx, pcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pcancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	log.Println("db: connected")
	return &DB{db}, nil
}

// Close closes the connection pool.
func (d *DB) Close() error {
	return d.Close()
}

// ErrNotFound is returned when a single-row query matches nothing.
var ErrNotFound = errors.New("db: not found")
// CtxKey is the context key carrying the *DB into request contexts.
type CtxKey struct{}

// FromContext pulls the *DB from the request context, or nil.
func FromContext(ctx context.Context) *DB {
	if v := ctx.Value(CtxKey{}); v != nil {
		return v.(*DB)
	}
	return nil
}

// NewContext returns ctx with the DB attached.
func NewContext(ctx context.Context, db *DB) context.Context {
	return context.WithValue(ctx, CtxKey{}, db)
}
