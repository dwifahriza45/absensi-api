package attendances

import (
	"io"
	"time"
)

// Status sesi foto yang dikirim ke aplikasi pegawai.
const (
	SessionDone     = "done"     // foto sudah diterima server
	SessionOpen     = "open"     // sedang dalam jam sesi
	SessionUpcoming = "upcoming" // belum dibuka
	SessionMissed   = "missed"   // jam sesi sudah lewat
)

// LocationDTO adalah titik GPS dalam respons API.
type LocationDTO struct {
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	AccuracyM float64 `json:"accuracyM"`
}

// EarlyLeaveDTO adalah permintaan pulang lebih awal.
type EarlyLeaveDTO struct {
	RequestedAt time.Time `json:"requestedAt"`
	Reason      string    `json:"reason"`
}

// AttendanceDTO adalah data absen dalam respons API.
type AttendanceDTO struct {
	ID               string         `json:"id"`
	AttendanceID     string         `json:"attendance_id"`
	EmployeeID       string         `json:"employee_id"`
	WorkDate         string         `json:"workDate"`
	CheckInAt        time.Time      `json:"checkInAt"`
	CheckInLocation  *LocationDTO   `json:"checkInLocation"`
	CheckInSelfieID  *string        `json:"check_in_selfie_id"`
	NeedsReview      bool           `json:"needsReview"`
	CheckOutAt       *time.Time     `json:"checkOutAt"`
	CheckOutLocation *LocationDTO   `json:"checkOutLocation"`
	CheckOutSelfieID *string        `json:"check_out_selfie_id"`
	CheckOutReason   *string        `json:"checkOutReason"`
	EarlyLeave       *EarlyLeaveDTO `json:"earlyLeave"`
}

// ScheduleDTO adalah jadwal kerja dalam format "08.00".
type ScheduleDTO struct {
	Start      string `json:"start"`
	End        string `json:"end"`
	BreakStart string `json:"breakStart"`
	BreakEnd   string `json:"breakEnd"`
}

// SessionDTO adalah status satu sesi foto hasil kerja.
type SessionDTO struct {
	No         int        `json:"no"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Status     string     `json:"status"`
	ReceivedAt *time.Time `json:"receivedAt"`
}

// TodayResponse adalah semua yang dibutuhkan beranda aplikasi pegawai.
type TodayResponse struct {
	Date                   string         `json:"date"`
	Schedule               ScheduleDTO    `json:"schedule"`
	Attendance             *AttendanceDTO `json:"attendance"`
	Sessions               []SessionDTO   `json:"sessions"`
	CanCheckOut            bool           `json:"canCheckOut"`
	CheckOutReasonRequired bool           `json:"checkOutReasonRequired"`
}

// CheckInInput adalah data absen masuk dari form multipart.
type CheckInInput struct {
	Selfie   io.Reader // nil bila kamera tidak bisa dipakai (absen ditandai untuk ditinjau)
	Location *Location // nil bila lokasi tidak bisa dibaca
}

// CheckOutInput adalah data absen pulang dari form multipart.
type CheckOutInput struct {
	Selfie   io.Reader
	Location *Location
	Reason   string
}

// EarlyLeaveRequest dikirim saat pegawai minta pulang lebih awal.
type EarlyLeaveRequest struct {
	Reason string `json:"reason" validate:"required,notblank,max=200" label:"Alasan"`
}

// AdminFilter adalah filter daftar absen di admin. Tanggal berformat YYYY-MM-DD.
type AdminFilter struct {
	From  string
	To    string
	Mitra string

	Bidang string
	Query  string
}

// AdminEmployeeDTO adalah ringkasan pegawai di daftar admin.
type AdminEmployeeDTO struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employee_id"`
	MitraCode  string `json:"mitra_id,omitempty"`
	Name       string `json:"name"`
	Mitra      string `json:"mitra"`

	Bidang string `json:"bidang"`
}

// AdminSessionDTO adalah foto hasil kerja yang sudah diterima.
type AdminSessionDTO struct {
	No         int       `json:"no"`
	PhotoID    string    `json:"photo_id"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// AdminItem adalah satu baris absen di admin.
type AdminItem struct {
	AttendanceDTO
	Employee AdminEmployeeDTO  `json:"employee"`
	Sessions []AdminSessionDTO `json:"sessions"`
}

// VersionResponse dipakai admin untuk mengecek apakah ada data baru (milidetik Unix).
type VersionResponse struct {
	Version int64 `json:"version"`
}

func toLocationDTO(l *Location) *LocationDTO {
	if l == nil {
		return nil
	}
	return &LocationDTO{Lat: l.Lat, Lng: l.Lng, AccuracyM: l.AccuracyM}
}

func toDTO(a Attendance) AttendanceDTO {
	dto := AttendanceDTO{
		ID:               a.ID,
		AttendanceID:     a.AttendanceID,
		EmployeeID:       a.EmployeeID,
		WorkDate:         a.WorkDate,
		CheckInAt:        a.CheckInAt,
		CheckInLocation:  toLocationDTO(a.CheckInLocation),
		CheckInSelfieID:  a.CheckInSelfieID,
		NeedsReview:      a.CheckInFlag,
		CheckOutAt:       a.CheckOutAt,
		CheckOutLocation: toLocationDTO(a.CheckOutLocation),
		CheckOutSelfieID: a.CheckOutSelfieID,
		CheckOutReason:   a.CheckOutReason,
	}
	if a.EarlyLeaveRequestedAt != nil {
		reason := ""
		if a.EarlyLeaveReason != nil {
			reason = *a.EarlyLeaveReason
		}
		dto.EarlyLeave = &EarlyLeaveDTO{RequestedAt: *a.EarlyLeaveRequestedAt, Reason: reason}
	}
	return dto
}

func toAdminItem(rec AdminRecord) AdminItem {
	sessions := make([]AdminSessionDTO, len(rec.Sessions))
	for i, s := range rec.Sessions {
		sessions[i] = AdminSessionDTO{No: s.No, PhotoID: s.PhotoID, ReceivedAt: s.ReceivedAt}
	}
	return AdminItem{
		AttendanceDTO: toDTO(rec.Attendance),
		Employee: AdminEmployeeDTO{
			ID:         rec.EmployeeUUID,
			EmployeeID: rec.EmployeeID, MitraCode: rec.MitraCode,
			Name:  rec.EmployeeName,
			Mitra: rec.Mitra,

			Bidang: rec.Bidang,
		},
		Sessions: sessions,
	}
}
