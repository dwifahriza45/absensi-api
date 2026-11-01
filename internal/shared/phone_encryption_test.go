package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestPhoneEncryption(t *testing.T) {
	h := NewPhoneHasher("test-encryption-secret")
	first, err := h.Encrypt("+6281234567890")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.Encrypt("081234567890")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("nonce reused")
	}
	phone, err := h.Decrypt(first, h.Hash("081234567890"))
	if err != nil || phone != "6281234567890" {
		t.Fatal("roundtrip failed")
	}
	if _, err := h.Decrypt(first, h.Hash("081111111111")); err == nil {
		t.Fatal("row swapping accepted")
	}
	if _, err := NewPhoneHasher("wrong-key").Decrypt(first, h.Hash(phone)); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := h.Decrypt(first[:len(first)-2], h.Hash(phone)); err == nil {
		t.Fatal("tampering accepted")
	}
}

func TestLegacyPhoneRemainsReadableAndSearchable(t *testing.T) {
	h := NewPhoneHasher("legacy-phone-secret")
	mac := hmac.New(sha256.New, []byte("legacy-phone-secret"))
	mac.Write([]byte("081234567890"))
	oldHash := hex.EncodeToString(mac.Sum(nil))
	if h.Hash("6281234567890") != oldHash {
		t.Fatal("existing lookup hash changed")
	}
	aead, err := h.phoneCipher()
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	legacy := aead.Seal(nonce, nonce, []byte("081234567890"), []byte(oldHash))
	phone, err := h.Decrypt(base64.RawStdEncoding.EncodeToString(legacy), oldHash)
	if err != nil || phone != "6281234567890" {
		t.Fatalf("legacy phone normalization failed: %v", err)
	}
}
