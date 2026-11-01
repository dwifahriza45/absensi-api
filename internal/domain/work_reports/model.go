package workreports

import "time"

// WorkReport adalah satu foto hasil kerja untuk satu sesi.
type WorkReport struct {
	ID           string
	WorkReportID string
	AttendanceID string
	SessionNo    int
	PhotoID      string
	ReceivedAt   time.Time
}
