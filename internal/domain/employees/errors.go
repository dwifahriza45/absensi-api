package employees

import "github.com/haidarrasyad/absensi-api/internal/pkg/apperror"

var (
	ErrHasAttendance  = apperror.Conflict("employee_has_attendance", "Karyawan sudah memiliki riwayat absensi dan tidak dapat dihapus.")
	ErrNotFound       = apperror.NotFound("employee_not_found", "Pegawai tidak ditemukan.")
	ErrQueryTooShort  = apperror.BadRequest("query_too_short", "Ketik minimal 3 huruf.")
	ErrImportEmpty    = apperror.Unprocessable("import_empty", "Tidak ada baris yang bisa disimpan. Periksa kolom Nama, NIK 16 digit, dan Bidang.")
	ErrImportTooLarge = apperror.BadRequest("import_too_large", "Maksimal 5.000 baris per import.")
)
