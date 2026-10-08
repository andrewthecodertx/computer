package auth

import (
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
