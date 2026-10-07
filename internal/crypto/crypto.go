package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// Encryption of IMAP passwords. Porting guide 3.5.
//
// MUST keep the exact byte layout so existing stored IMAP passwords stay readable:
//
//	encrypt(text) = base64( IV(12) || tag(16) || ciphertext )
//
// AES-256-GCM with a random 12-byte IV and 16-byte auth tag. The key is the
// SHA-256 of the NEXTAUTH_SECRET (or AUTH_SECRET), giving 32 bytes.
//
// Changing the secret makes saved IMAP passwords unreadable.

const (
	algorithm  = "aes-256-gcm"
	ivLength   = 12
	tagLength  = 16
)

// Key derives the 32-byte AES key from the secret.
// TODO: fail if the secret is empty (do NOT hard-code a fallback).
func Key(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Encrypt returns base64(IV || tag || ciphertext). Never returns an error path
// that loses data; the caller must ensure the secret is set.
func Encrypt(text, secret string) (string, error) {
	key := Key(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	seal := gcm.Seal(nil, iv, []byte(text), nil)
	// gcm output is ciphertext || tag
	ciphertext := seal[:len(seal)-tagLength]
	tag := seal[len(seal)-tagLength:]
	out := make([]byte, 0, ivLength+tagLength+len(ciphertext))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt. Returns an error on malformed input.
func Decrypt(data, secret string) (string, error) {
	key := Key(secret)
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	if len(raw) < ivLength+tagLength {
		return "", errShortInputError
	}
	iv := raw[:ivLength]
	tag := raw[ivLength : ivLength+tagLength]
	ciphertext := raw[ivLength+tagLength:]
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	// gcm expects ciphertext || tag
	sealed := append([]byte(nil), ciphertext...)
	sealed = append(sealed, tag...)
	plain, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// errShortInputError is returned when the decoded input is too short to contain
// the IV and auth tag.
var errShortInputError = errShortInput{}

type errShortInput struct{}

func (errShortInput) Error() string { return "crypto: input too short" }