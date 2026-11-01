// Package shared berisi utilitas bisnis kecil yang dipakai lebih dari satu domain.
package shared

import (
	"crypto/rand"
	"fmt"
	"regexp"
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NewID membuat UUID v4.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// IsID memeriksa format UUID sebelum dikirim ke database.
func IsID(value string) bool {
	return idPattern.MatchString(value)
}

// IsPublicID validates a business identifier before a lookup.
func IsPublicID(value, prefix string) bool {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-[0-9]{8}-[0-9]{6}$`).MatchString(value)
}
