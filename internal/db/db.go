package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/andrew/go-computer/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct{ *sql.DB }
type Object = map[string]any

var ErrNotFound = errors.New("not found")

func New(cfg *config.Config) (*DB, error) {
	d, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(20)
	d.SetMaxIdleConns(5)
	d.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = d.PingContext(ctx); err != nil {
		d.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	return &DB{d}, nil
}
func (d *DB) Close() error { return d.DB.Close() }

type contextKey struct{}

func FromContext(ctx context.Context) *DB { d, _ := ctx.Value(contextKey{}).(*DB); return d }
func NewContext(ctx context.Context, d *DB) context.Context {
	return context.WithValue(ctx, contextKey{}, d)
}
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Queryer is also implemented by sql.Tx, so mutations and relation updates
// can be committed atomically without a second connection pool.
type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func One(ctx context.Context, q Queryer, query string, args ...any) (Object, error) {
	var raw []byte
	err := q.QueryRowContext(ctx, query, args...).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var v Object
	err = json.Unmarshal(raw, &v)
	return v, err
}
func Many(ctx context.Context, q Queryer, query string, args ...any) ([]Object, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Object{}
	for rows.Next() {
		var raw []byte
		var v Object
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func Quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Table and column names originate in server-owned allowlists, never request
// parameters. Every value is a PostgreSQL bind parameter.
func Insert(ctx context.Context, q Queryer, table string, data Object) (Object, error) {
	keys := []string{}
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cols, ps := []string{}, []string{}
	args := []any{}
	for i, k := range keys {
		cols = append(cols, Quote(k))
		ps = append(ps, fmt.Sprintf("$%d", i+1))
		args = append(args, data[k])
	}
	return One(ctx, q, `INSERT INTO `+Quote(table)+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ps, ",")+`) RETURNING to_jsonb(`+Quote(table)+`)`, args...)
}
func Update(ctx context.Context, q Queryer, table, id, ownerCol, owner string, data Object) (Object, error) {
	keys := []string{}
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sets, args := []string{}, []any{}
	for i, k := range keys {
		sets = append(sets, Quote(k)+fmt.Sprintf("=$%d", i+1))
		args = append(args, data[k])
	}
	args = append(args, id, owner)
	where := fmt.Sprintf(`id=$%d AND %s=$%d`, len(args)-1, Quote(ownerCol), len(args))
	if len(sets) == 0 {
		return One(ctx, q, `SELECT to_jsonb(t) FROM `+Quote(table)+` t WHERE `+where, args...)
	}
	return One(ctx, q, `UPDATE `+Quote(table)+` SET `+strings.Join(sets, ",")+` WHERE `+where+` RETURNING to_jsonb(`+Quote(table)+`)`, args...)
}
func Transaction(ctx context.Context, d *DB, fn func(*sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
