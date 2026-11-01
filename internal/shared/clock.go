package shared

import "time"

// Clock memberi waktu sekarang. Dibuat interface agar mudah diganti saat testing.
type Clock interface {
	Now() time.Time
}

// LocalClock mengembalikan waktu server pada zona waktu perusahaan.
type LocalClock struct {
	loc *time.Location
}

// NewClock membuat LocalClock.
func NewClock(loc *time.Location) LocalClock {
	return LocalClock{loc: loc}
}

// Now mengembalikan waktu sekarang di zona waktu perusahaan.
func (c LocalClock) Now() time.Time {
	return time.Now().In(c.loc)
}

// DateString memformat tanggal kerja menjadi "YYYY-MM-DD".
func DateString(t time.Time) string {
	return t.Format(time.DateOnly)
}
