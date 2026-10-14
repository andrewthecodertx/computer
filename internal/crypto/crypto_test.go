package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestPasswordEncryption(t *testing.T) {
	cipher, e := Encrypt("mailbox-password", "stable-secret")
	if e != nil {
		t.Fatal(e)
	}
	p, e := Decrypt(cipher, "stable-secret")
	if e != nil || p != "mailbox-password" {
		t.Fatal(p, e)
	}
	raw, _ := base64.StdEncoding.DecodeString(cipher)
	// v1 format: version(1) + IV(12) + tag(16) + ciphertext
	if len(raw) != 1+12+16+len(p) {
		t.Fatalf("incompatible v1 IV/tag/ciphertext layout: got %d", len(raw))
	}
	if raw[0] != versionByte {
		t.Fatal("ciphertext missing version byte")
	}
	raw[len(raw)-1] ^= 1
	if _, e = Decrypt(base64.StdEncoding.EncodeToString(raw), "stable-secret"); e == nil {
		t.Fatal("accepted tampered ciphertext")
	}
	if _, e = Decrypt(cipher, "changed-secret"); e == nil {
		t.Fatal("accepted wrong secret")
	}
	if _, e = Encrypt("value", ""); e == nil {
		t.Fatal("accepted empty secret")
	}
}

// TestLegacyCiphertextCompatibility verifies Decrypt can read ciphertexts
// created before HKDF/versioning (review SEC-9).
func TestLegacyCiphertextCompatibility(t *testing.T) {
	// Create a legacy ciphertext using the OLD key derivation (raw SHA-256)
	// This simulates a password encrypted before HKDF was added.
	secret := "legacy-secret"
	oldKey := sha256.Sum256([]byte(secret))
	block, _ := aes.NewCipher(oldKey[:])
	gcm, _ := cipher.NewGCM(block)
	iv := make([]byte, ivLength)
	rand.Read(iv)
	seal := gcm.Seal(nil, iv, []byte("legacy-password"), nil)
	legacyRaw := append(iv, seal...) // no version byte
	legacyCipher := base64.StdEncoding.EncodeToString(legacyRaw)

	// Decrypt with the new Decrypt (should auto-detect legacy format via
	// LegacyKey which uses raw SHA-256)
	p, e := Decrypt(legacyCipher, secret)
	if e != nil || p != "legacy-password" {
		t.Fatalf("failed to decrypt legacy ciphertext: %v %v", p, e)
	}

	// Re-encrypting with the same secret produces v1 format (HKDF + version)
	v1Cipher, e := Encrypt(p, secret)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := base64.StdEncoding.DecodeString(v1Cipher)
	if raw[0] != versionByte {
		t.Fatal("re-encrypted ciphertext missing version byte")
	}
}
