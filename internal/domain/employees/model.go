package employees

import "time"

// Employee adalah data pegawai. Nomor HP disimpan sebagai hash login dan ciphertext kontak.
type Employee struct {
	HasAttendance bool
	ID            string
	EmployeeID    string

	Name    string
	MitraID string
	Mitra   string

	BidangID       string
	Bidang         string
	PhoneHash      string
	PhoneEncrypted string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
