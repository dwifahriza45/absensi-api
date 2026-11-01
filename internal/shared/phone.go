package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// NormalizePhone menyamakan format nomor Indonesia: +62 / 08 / 8xx menjadi 628xx.
func NormalizePhone(raw string) string {
	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	phone := digits.String()

	switch {
	case strings.HasPrefix(phone, "08"):
		return "62" + phone[1:]
	case strings.HasPrefix(phone, "8"):
		return "62" + phone
	default:
		return phone
	}
}

// PhoneHasher menghasilkan lookup HMAC untuk nomor HP.
type PhoneHasher struct {
	secret []byte
}

// NewPhoneHasher membuat PhoneHasher dengan kunci rahasia server.
func NewPhoneHasher(secret string) PhoneHasher {
	return PhoneHasher{secret: []byte(secret)}
}

// Hash menghasilkan kode dari nomor HP (dinormalisasi lebih dulu).
func (h PhoneHasher) Hash(phone string) string {
	mac := hmac.New(sha256.New, h.secret)
	// Keep the original hash representation so existing logins, uniqueness,
	// and ciphertext authentication remain compatible without rehashing data.
	normalized := NormalizePhone(phone)
	if strings.HasPrefix(normalized, "628") {
		normalized = "0" + normalized[2:]
	}
	mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// Matches membandingkan nomor HP dengan kode tersimpan secara aman (waktu konstan).
func (h PhoneHasher) Matches(phone, storedHash string) bool {
	return hmac.Equal([]byte(h.Hash(phone)), []byte(storedHash))
}
