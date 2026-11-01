package shared

import "testing"

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"0812-3456-7890":    "6281234567890",
		"+62 812 3456 7890": "6281234567890",
		"6281234567890":     "6281234567890",
		"81234567890":       "6281234567890",
	}
	for raw, want := range cases {
		if got := NormalizePhone(raw); got != want {
			t.Errorf("NormalizePhone(%q) = %q, mau %q", raw, got, want)
		}
	}
}

func TestPhoneHasherMatches(t *testing.T) {
	hasher := NewPhoneHasher("rahasia-uji")
	stored := hasher.Hash("081234567890")

	if !hasher.Matches("+62 812-3456-7890", stored) {
		t.Error("format nomor berbeda seharusnya tetap cocok")
	}
	if hasher.Matches("081200000000", stored) {
		t.Error("nomor berbeda seharusnya tidak cocok")
	}
}
