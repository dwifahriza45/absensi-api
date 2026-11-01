package workreports

import "github.com/haidarrasyad/absensi-api/internal/pkg/apperror"

var (
	ErrNotFound          = apperror.NotFound("work_report_not_found", "Foto hasil kerja tidak ditemukan.")
	ErrNotCheckedIn      = apperror.Conflict("not_checked_in", "Silakan absen masuk dulu.")
	ErrAlreadyCheckedOut = apperror.Conflict("already_checked_out", "Anda sudah absen pulang hari ini.")
	ErrInvalidSession    = apperror.BadRequest("invalid_session", "Sesi foto tidak dikenal.")
	ErrSessionNotOpen    = apperror.Conflict("session_not_open", "Sesi foto ini belum dibuka.")
	ErrSessionClosed     = apperror.Conflict("session_closed", "Jam sesi foto ini sudah habis.")
	ErrPhotoRequired     = apperror.BadRequest("photo_required", "Foto hasil kerja wajib dikirim.")
)
