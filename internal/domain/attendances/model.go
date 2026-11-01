package attendances

import "time"

// Location adalah titik GPS saat absen.
type Location struct {
	Lat       float64
	Lng       float64
	AccuracyM float64
}

// Attendance adalah satu hari kerja seorang pegawai: absen masuk, pulang, dan permintaan pulang awal.
type Attendance struct {
	ID                    string
	AttendanceID          string
	EmployeeID            string
	WorkDate              string // YYYY-MM-DD
	CheckInAt             time.Time
	CheckInLocation       *Location
	CheckInSelfieID       *string
	CheckInFlag           bool // perlu ditinjau PIC
	CheckOutAt            *time.Time
	CheckOutLocation      *Location
	CheckOutSelfieID      *string
	CheckOutReason        *string
	EarlyLeaveRequestedAt *time.Time
	EarlyLeaveReason      *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// SubmittedSession adalah foto hasil kerja yang sudah diterima untuk satu sesi.
type SubmittedSession struct {
	No         int
	PhotoID    string
	ReceivedAt time.Time
}

// AdminRecord adalah absen beserta data pegawai dan foto sesinya, untuk tampilan admin.
type AdminRecord struct {
	Attendance
	EmployeeName string
	EmployeeUUID string
	MitraCode    string
	Mitra        string

	Bidang   string
	Sessions []SubmittedSession
}
