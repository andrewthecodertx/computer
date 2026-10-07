package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptHash hashes a password with cost 12 (porting guide 3.1).
func bcryptHashImpl(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// bcryptCheck compares a password against a bcrypt hash.
func bcryptCheckImpl(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// cuid generates a CUID-shaped id so the values match the original app's
// format (the DB stores text ids and the web client expects strings).
func cuid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("rand: %v", err))
	}
	return "cmu" + hex.EncodeToString(b)
}
