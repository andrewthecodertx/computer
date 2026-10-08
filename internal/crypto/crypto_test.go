package crypto

import (
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
	if len(raw) != 12+16+len(p) {
		t.Fatal("incompatible IV/tag/ciphertext layout")
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
