package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/andrew/go-computer/internal/config"
	"github.com/andrew/go-computer/internal/models"
)

type UserRepo struct{ db *DB }

func NewUserRepo(d *DB) *UserRepo { return &UserRepo{d} }
func (r *UserRepo) find(ctx context.Context, field, value string) (*models.User, error) {
	o, err := One(ctx, r.db, `SELECT to_jsonb(u) FROM "User" u WHERE `+Quote(field)+`=$1`, value)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(o)
	var row struct {
		ID            string  `json:"id"`
		Name          *string `json:"name"`
		Email         *string `json:"email"`
		EmailVerified *string `json:"emailVerified"`
		Image         *string `json:"image"`
		Password      *string `json:"password"`
		Role          string  `json:"role"`
	}
	if err = json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	return &models.User{ID: row.ID, Name: row.Name, Email: row.Email, EmailVerified: row.EmailVerified, Image: row.Image, Password: row.Password, Role: row.Role}, nil
}
func (r *UserRepo) FindByEmail(ctx context.Context, e string) (*models.User, error) {
	return r.find(ctx, "email", e)
}
func (r *UserRepo) FindByID(ctx context.Context, id string) (*models.User, error) {
	return r.find(ctx, "id", id)
}
func (r *UserRepo) SetRole(ctx context.Context, id, role string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE "User" SET role=$1,"updatedAt"=now() WHERE id=$2`, role, id)
	return err
}

type NewUserInput struct{ ID, Name, Email, Password, Role string }

// A transaction-scoped lock serializes first-admin selection with signup.
func (r *UserRepo) Create(ctx context.Context, u *NewUserInput) (string, error) {
	err := Transaction(ctx, r.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(927438)`); err != nil {
			return err
		}
		var admins int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM "User" WHERE role='ADMIN' AND email NOT LIKE '%@example.com'`).Scan(&admins); err != nil {
			return err
		}
		role := u.Role
		if role == "" {
			role = "USER"
		}
		if admins == 0 && len(config.AdminEmails()) == 0 && !isTestEmail(u.Email) {
			role = "ADMIN"
		}
		_, err := Insert(ctx, tx, "User", Object{"id": u.ID, "name": u.Name, "email": u.Email, "password": u.Password, "role": role})
		return err
	})
	return u.ID, err
}
func isTestEmail(e string) bool      { return len(e) >= 12 && e[len(e)-12:] == "@example.com" }
func IsAdminEmail(email string) bool { return config.IsAdminEmail(email) }
