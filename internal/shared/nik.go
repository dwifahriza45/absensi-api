package shared

// ValidNIK preserves all sixteen digits, including leading zeroes.
func ValidNIK(value string) bool {
	if len(value) != 16 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
