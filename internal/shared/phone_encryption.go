package shared

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// Derive a separate encryption key from the configured phone secret. Keep this
// secret in backups: losing it makes stored contact numbers unrecoverable.
func (h PhoneHasher) phoneCipher() (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("absensi:phone-encryption:v1:"), h.secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (h PhoneHasher) Encrypt(phone string) (string, error) {
	aead, err := h.phoneCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	// Binding ciphertext to its lookup hash prevents swapping numbers between rows.
	sealed := aead.Seal(nonce, nonce, []byte(NormalizePhone(phone)), []byte(h.Hash(phone)))
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}
func (h PhoneHasher) Decrypt(encrypted, phoneHash string) (string, error) {
	if encrypted == "" {
		return "", nil
	} // Legacy hash-only records need re-entry.
	aead, err := h.phoneCipher()
	if err != nil {
		return "", err
	}
	data, err := base64.RawStdEncoding.DecodeString(encrypted)
	if err != nil || len(data) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("invalid encrypted phone")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(phoneHash))
	if err != nil {
		return "", errors.New("cannot decrypt employee phone")
	}
	return NormalizePhone(string(plain)), nil
}
