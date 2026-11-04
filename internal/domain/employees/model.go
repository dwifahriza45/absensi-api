package employees

import "time"

// Employee adalah data pegawai. NIK dipakai untuk login.
type Employee struct {
	HasAttendance bool
	ID            string
	EmployeeID    string

	NIK     string
	Name    string
	MitraID string
	Mitra   string

	BidangID string
	Bidang   string

	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
