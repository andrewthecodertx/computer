package scripts

import (
	"context"
	"fmt"
	"github.com/andrew/go-computer/internal/db"
	"golang.org/x/crypto/bcrypt"
	"os"
)

// Seed only creates the requested account; it never clears existing records.
func Seed(ctx context.Context, d *db.DB) error {
	email, password := os.Getenv("SEED_EMAIL"), os.Getenv("SEED_PASSWORD")
	if email == "" || password == "" {
		return fmt.Errorf("SEED_EMAIL and SEED_PASSWORD are required")
	}
	repo := db.NewUserRepo(d)
	if _, e := repo.FindByEmail(ctx, email); e == nil {
		return nil
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
	if e != nil {
		return e
	}
	name := os.Getenv("SEED_NAME")
	if name == "" {
		name = "Admin"
	}
	_, e = repo.Create(ctx, &db.NewUserInput{ID: db.NewID(), Name: name, Email: email, Password: string(hash), Role: "USER"})
	return e
}
