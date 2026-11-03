package employees

import "time"

// Employee adalah data pegawai. NIK dipakai untuk login; nomor HP disimpan sebagai hash kontak dan ciphertext.
type Employee struct {
	HasAttendance bool
	ID            string
	EmployeeID    string

	NIK     string
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
