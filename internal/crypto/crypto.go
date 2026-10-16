package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// Encryption of IMAP passwords. Porting guide 3.5.
//
// Ciphertext format (review SEC-9, layout corrected per review CR-20 —
// gcm.Seal emits ciphertext followed by tag):
//	v1: base64( version(1) || IV(12) || ciphertext || tag(16) )
//	legacy (pre-HKDF): base64( IV(12) || ciphertext || tag(16) ) — no version
//	byte; derived with raw SHA-256 for backward compatibility during rotation.
//
// The key is now derived via HKDF-SHA256 with a fixed context string.
// Changing the secret makes saved IMAP passwords unreadable unless the
// legacy fallback is used (handled by Decrypt).
//
// Review CR-20: a legacy record starts with a RANDOM IV byte, which can
// coincidentally equal versionByte — the format cannot be sniffed from the
// first byte alone. Decrypt therefore treats an apparent version byte as a
// hint only: the v1 decode is attempted first and, when GCM authentication
// does not succeed, the WHOLE record is retried as legacy. Only successful
// authenticated decryption ever selects a format/key.

const (
	algorithm       = "aes-256-gcm"
	ivLength        = 12
	tagLength       = 16
	versionByte     = 1
	legacyVersion   = 0
	hkdfContext     = "computer/imap-password/v1"
)

// Key derives the 32-byte AES key from the secret using HKDF-SHA256.
// An empty secret is a programming error: Encrypt/Decrypt guard it, and
// main fails startup without a configured secret.
func Key(secret string) []byte {
	if secret == "" {
		panic("crypto: empty secret")
	}
	k, err := hkdf.Key(sha256.New, []byte(secret), nil, hkdfContext, 32)
	if err != nil {
		panic("hkdf: " + err.Error())
	}
	return k
}

// LegacyKey derives the 32-byte AES key via raw SHA-256 (pre-HKDF).
// Used by Decrypt when encountering a legacy ciphertext (no version byte).
func LegacyKey(secret string) []byte {
	if secret == "" {
		panic("crypto: empty secret")
	}
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// Encrypt returns base64( version(1) || IV(12) || ciphertext || tag(16) ).
// Never returns an error path that loses data; the caller must ensure the
// secret is set.
func Encrypt(text, secret string) (string, error) {
	if secret == "" {
		return "", errors.New("missing encryption secret")
	}
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
	out := make([]byte, 0, 1+ivLength+len(ciphertext)+tagLength)
	out = append(out, versionByte)
	out = append(out, iv...)
	out = append(out, ciphertext...)
	out = append(out, tag...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt. Returns an error on malformed input.
// Supports both v1 (HKDF) and legacy (raw SHA-256) ciphertexts for
// transparent rotation. Review CR-20: the leading-byte check is only a hint —
// a legacy IV byte can collide with versionByte, so a failed v1 decode falls
// back to an authenticated legacy decode of the whole record instead of
// declaring the password unreadable.
func Decrypt(data, secret string) (string, error) {
	if secret == "" {
		return "", errors.New("missing encryption secret")
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	if len(raw) >= 1 && raw[0] == versionByte {
		if plain, e := openGCM(raw[1:], Key(secret)); e == nil {
			return plain, nil
		}
		// Fall through: either a tampered/miskeyed v1 record (the legacy
		// retry will fail authentication too) or a legacy record whose random
		// IV starts with the version byte.
	}
	return openGCM(raw, LegacyKey(secret))
}

// openGCM decodes one record body: IV(12) || ciphertext || tag(16).
func openGCM(raw, key []byte) (string, error) {
	if len(raw) < ivLength+tagLength {
		return "", errShortInputError
	}
	iv := raw[:ivLength]
	// gcm.Seal returns ciphertext || tag, so the tag is the LAST 16 bytes
	tag := raw[len(raw)-tagLength:]
	ciphertext := raw[ivLength : len(raw)-tagLength]
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
