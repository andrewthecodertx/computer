package db

import (
	"context"
	"database/sql"

	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/models"
)

// UserRepo backs the auth/user endpoints. Porting guide 2.2, 4.1, 4.2.
type UserRepo struct {
	db *DB
}

// NewUserRepo constructs a repo backed by the given DB.
func NewUserRepo(db *DB) *UserRepo {
	return &UserRepo{db: db}
}

const userCols = `id, name, email, "emailVerified", image, password, role, "createdAt", "updatedAt"`

func scanUser(rows *sql.Rows) (*models.User, error) {
	var u models.User
	var ev, image, password sql.NullString
	if err := rows.Scan(&u.ID, &u.Name, &u.Email, &ev, &image, &password, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	u.EmailVerified = nullStrPtr(ev)
	u.Image = nullStrPtr(image)
	u.Password = nullStrPtr(password)
	return &u, nil
}

func nullStrPtr(s sql.NullString) *string {
	if s.Valid {
		v := s.String
		return &v
	}
	return nil
}

// FindByEmail looks up a user by email (case-sensitive, as stored).
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	rows, err := r.db.QueryContext(ctx, `select `+userCols+` from "User" where email = $1`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanUser(rows)
}

// FindByID looks up a user by id.
func (r *UserRepo) FindByID(ctx context.Context, id string) (*models.User, error) {
	rows, err := r.db.QueryContext(ctx, `select `+userCols+` from "User" where id = $1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanUser(rows)
}

// ExistsAnyNonTest returns true when there is already at least one real
// (non-@example.com) account. Used to decide whether the next signup becomes
// admin. Porting guide 5.2.
func (r *UserRepo) ExistsAnyNonTest(ctx context.Context) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `select count(*) from "User" where email not like '%@example.com'`).Scan(&n)
	return n > 0, err
}

// CountAdmins returns the number of admin accounts (excluding @example.com).
func (r *UserRepo) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `select count(*) from "User" where role = 'ADMIN' and email not like '%@example.com'`).Scan(&n)
	return n, err
}

// Create inserts a user and returns its id. Role defaults to USER; pass ADMIN
// to make someone an admin (first real signup, or explicit promotion).
func (r *UserRepo) Create(ctx context.Context, u *NewUserInput) (string, error) {
	if _, err := r.db.ExecContext(ctx, `insert into "User" (id, name, email, password, role, "createdAt", "updatedAt") values ($1, $2, $3, $4, $5, now(), now())`,
		u.ID, u.Name, u.Email, u.Password, u.Role); err != nil {
		return "", err
	}
	return u.ID, nil
}

// SetRole updates a user's role. Porting guide 4.2.
func (r *UserRepo) SetRole(ctx context.Context, id, role string) error {
	_, err := r.db.ExecContext(ctx, `update "User" set role = $1, "updatedAt" = now() where id = $2`, role, id)
	return err
}

// ListAll returns all users except @example.com, oldest first. Porting guide 4.2.
func (r *UserRepo) ListAll(ctx context.Context) ([]models.User, error) {
	rows, err := r.db.QueryContext(ctx, `select `+userCols+` from "User" where email not like '%@example.com' order by "createdAt"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// IsAdminEmail reports whether email is listed in ADMIN_EMAILS (promotes on
// first check, per porting guide 3.2 / 5.2).
func IsAdminEmail(email string) bool {
	return config.IsAdminEmail(email)
}

// NewUserInput is the fields needed to create a user.
type NewUserInput struct {
	ID       string
	Name     string
	Email    string
	Password string
	Role     string
}
