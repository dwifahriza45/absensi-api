package employees

// SearchItem adalah hasil pencarian nama di layar login pegawai.
// Sengaja tanpa nomor HP atau data pribadi lain.
type SearchItem struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employee_id"`

	Name     string `json:"name"`
	MitraID  string `json:"mitra_id,omitempty"`
	Mitra    string `json:"mitra"`
	BidangID string `json:"bidang_id,omitempty"`
	Bidang   string `json:"bidang"`
}

// Response berisi profil. Phone hanya diisi pada response admin.
type Response struct {
	HasAttendance bool   `json:"has_attendance"`
	CanDelete     bool   `json:"can_delete"`
	DeleteReason  string `json:"delete_reason,omitempty"`
	NIK           string `json:"nik,omitempty"`
	Phone         string `json:"phone,omitempty"`
	ID            string `json:"id"`
	EmployeeID    string `json:"employee_id"`

	Name    string `json:"name"`
	MitraID string `json:"mitra_id,omitempty"`
	Mitra   string `json:"mitra"`

	BidangID string `json:"bidang_id,omitempty"`
	Bidang   string `json:"bidang"`
	IsActive bool   `json:"isActive"`
}

// ToResponse mengubah model menjadi Response.
func ToResponse(e Employee) Response {
	result := Response{HasAttendance: e.HasAttendance, CanDelete: !e.HasAttendance, ID: e.ID, EmployeeID: e.EmployeeID, Name: e.Name, MitraID: e.MitraID, Mitra: e.Mitra, Bidang: e.Bidang, BidangID: e.BidangID, IsActive: e.IsActive}
	if e.HasAttendance {
		result.DeleteReason = ErrHasAttendance.Message
	}
	return result
}

// ListFilter adalah filter daftar pegawai di admin.
type ListFilter struct {
	MitraID         string
	BidangID        string
	IncludeInactive bool
	Mitra           string

	Bidang string
	Query  string
}

// FilterOptions berisi pilihan untuk dropdown filter admin.
type FilterOptions struct {
	Mitra []string `json:"mitra"`

	Bidang []string `json:"bidang"`
}

// ImportRow adalah satu baris dari file Excel.
type ImportRow struct {
	EmployeeID string `json:"employee_id"`
	NIK        string `json:"nik"`
	Name       string `json:"name"`
	Mitra      string `json:"mitra"`

	Bidang string `json:"bidang"`
	Phone  string `json:"phone"`
}

// ImportRequest dikirim admin setelah membaca file Excel.
type ImportRequest struct {
	Rows              []ImportRow `json:"rows" validate:"required,min=1,max=5000" label:"Data Karyawan"`
	DeactivateMissing bool        `json:"deactivateMissing"`
}

// SkippedRow menjelaskan baris yang tidak disimpan.
type SkippedRow struct {
	Row    int    `json:"row"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ImportResult adalah ringkasan hasil import.
type ImportResult struct {
	Created     int          `json:"created"`
	Updated     int          `json:"updated"`
	Deactivated int          `json:"deactivated"`
	Skipped     []SkippedRow `json:"skipped"`
}
