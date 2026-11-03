package attendances

import "github.com/haidarrasyad/absensi-api/internal/pkg/apperror"

var (
	ErrReportsIncomplete = apperror.Conflict("reports_incomplete", "Absen pulang belum dapat dilakukan karena masih ada laporan kerja yang belum lengkap. Silakan lengkapi seluruh sesi laporan kerja terlebih dahulu.")
	ErrSelfieRequired    = apperror.BadRequest("selfie_required", "Foto selfie wajib diambil.")
	ErrLocationRequired  = apperror.BadRequest("location_required", "Lokasi wajib diizinkan sebelum absen.")
	ErrNotFound          = apperror.NotFound("attendance_not_found", "Data absen tidak ditemukan.")
	ErrAlreadyCheckedIn  = apperror.Conflict("already_checked_in", "Anda sudah absen masuk hari ini.")
	ErrNotCheckedIn      = apperror.Conflict("not_checked_in", "Silakan absen masuk dulu.")
	ErrAlreadyCheckedOut = apperror.Conflict("already_checked_out", "Anda sudah absen pulang hari ini.")
	ErrCheckOutTooEarly  = apperror.Conflict("check_out_too_early",
		"Absen pulang belum tersedia. Ajukan pulang lebih awal bila perlu.")
	ErrReasonRequired   = &apperror.Error{Status: 422, Code: "reason_required", Message: "Validation Failed", Valid: map[string]string{"reason": "Alasan Wajib Diisi"}}
	ErrEarlyLeaveExists = apperror.Conflict("early_leave_exists", "Permintaan pulang awal sudah dikirim.")
	ErrInvalidLocation  = apperror.BadRequest("invalid_location", "Data lokasi tidak valid.")
	ErrInvalidDateRange = apperror.BadRequest("invalid_date_range", "Rentang tanggal tidak valid. Silakan periksa kembali filter tanggal.")
)
