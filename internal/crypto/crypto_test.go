package crypto

import (
	"bytes"
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
	// v1 format: version(1) + IV(12) + ciphertext + tag(16) (review CR-20
	// corrected the layout comment: gcm.Seal emits ciphertext || tag).
	if len(raw) != 1+12+len(p)+16 {
		t.Fatalf("incompatible v1 IV/ciphertext/tag layout: got %d", len(raw))
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
	if _, e = Decrypt("", "stable-secret"); e == nil {
		t.Fatal("accepted empty input")
	}
	// Too-short bodies are rejected on both format paths.
	short := base64.StdEncoding.EncodeToString([]byte{versionByte, 1, 2, 3})
	if _, e = Decrypt(short, "stable-secret"); e == nil {
		t.Fatal("accepted short versioned input")
	}
	shortLegacy := base64.StdEncoding.EncodeToString([]byte{9, 9, 9})
	if _, e = Decrypt(shortLegacy, "stable-secret"); e == nil {
		t.Fatal("accepted short legacy input")
	}
}

// legacyEncrypt builds a pre-HKDF ciphertext: base64( IV || ciphertext || tag )
// under the raw SHA-256 key, with full control over the IV (review CR-20 asks
// for deterministic fixtures around the version-byte collision).
func legacyEncrypt(t *testing.T, plain, secret string, iv []byte) string {
	t.Helper()
	key := sha256.Sum256([]byte(secret))
	block, e := aes.NewCipher(key[:])
	if e != nil {
		t.Fatal(e)
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		t.Fatal(e)
	}
	seal := gcm.Seal(nil, iv, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(append([]byte(nil), iv...), seal...))
}

// TestLegacyCiphertextCompatibility verifies Decrypt can read ciphertexts
// created before HKDF/versioning (review SEC-9).
func TestLegacyCiphertextCompatibility(t *testing.T) {
	secret := "legacy-secret"
	iv := make([]byte, ivLength)
	rand.Read(iv)
	legacyCipher := legacyEncrypt(t, "legacy-password", secret, iv)

	// Decrypt with the new Decrypt (legacy format via LegacyKey/raw SHA-256).
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

// TestLegacyVersionByteCollision pins review CR-20: a legacy record whose
// random IV starts with the version byte must still decrypt. The old
// first-byte sniff stripped the byte and selected the v1 key, rendering such
// stored credentials permanently unreadable.
func TestLegacyVersionByteCollision(t *testing.T) {
	secret := "legacy-secret"
	iv := make([]byte, ivLength)
	rand.Read(iv)
	iv[0] = versionByte // force the collision, deterministically
	legacyCipher := legacyEncrypt(t, "collision-password", secret, iv)

	p, e := Decrypt(legacyCipher, secret)
	if e != nil || p != "collision-password" {
		t.Fatalf("legacy ciphertext colliding with the version byte failed to decrypt: %v %v", p, e)
	}

	// Tampering must still fail authentication on the fallback path.
	raw, _ := base64.StdEncoding.DecodeString(legacyCipher)
	raw[len(raw)-1] ^= 1
	if _, e = Decrypt(base64.StdEncoding.EncodeToString(raw), secret); e == nil {
		t.Fatal("accepted tampered colliding legacy ciphertext")
	}
	// A wrong secret never authenticates, collision or not.
	if _, e = Decrypt(legacyCipher, "other-secret"); e == nil {
		t.Fatal("accepted wrong secret for legacy ciphertext")
	}
	// Sanity: the fixture really starts with the version byte.
	if !bytes.HasPrefix(mustDecode(t, legacyCipher), []byte{versionByte}) {
		t.Fatal("fixture does not exercise the collision")
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	raw, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
