package photos

import "time"

// Kind menandai untuk apa foto dipakai.
type Kind string

const (
	KindSelfieIn   Kind = "selfie_in"
	KindSelfieOut  Kind = "selfie_out"
	KindWorkReport Kind = "work_report"
)

// Photo adalah data foto yang tersimpan. File-nya ada di storage dengan StorageKey.
type Photo struct {
	ID          string
	PhotoID     string
	Kind        Kind
	StorageKey  string
	ContentType string
	SizeBytes   int64
	CreatedAt   time.Time
}
